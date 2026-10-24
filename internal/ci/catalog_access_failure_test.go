package ci_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func testAccessCatalogFailures(t *testing.T, executable string, buildEnvironment []string, project, assets, archive string) {
	cases := []struct {
		name, diagnostic string
		edit             func(map[string]any)
		file, body       string
	}{
		{name: "missing Pack", diagnostic: "missing", edit: func(m map[string]any) {
			accessCatalogSkill(m, "leaf")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "absent:leaf"}}
		}},
		{name: "missing skill", diagnostic: "missing", edit: func(m map[string]any) {
			accessCatalogSkill(m, "leaf")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "alpha:absent"}}
		}},
		{name: "cross-Pack cycle", diagnostic: "cycle", edit: func(m map[string]any) {
			accessCatalogSkill(m, "leaf")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "alpha:root"}}
		}},
		{name: "incompatible dependency", diagnostic: "reviewed", edit: func(m map[string]any) { delete(accessCatalogSkill(m, "leaf"), "on_demand") }},
		{name: "excluded dependency", diagnostic: "excluded", edit: func(m map[string]any) {
			r := accessCatalogSkill(m, "leaf")
			r["bindings"] = slices.DeleteFunc(r["bindings"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			r["variants"] = slices.DeleteFunc(r["variants"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			r["surface_exclusions"] = []any{map[string]any{"surface": "codex", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed for Codex"}}
		}},
		{name: "invalid access reference", diagnostic: "directive", file: "packs/beta/skills/leaf-codex/ACCESS.md", body: "{{packy.access:alpha::root}}\n"},
		{name: "unsupported vocabulary in unrelated Pack", diagnostic: "unknown field", edit: func(m map[string]any) {
			accessCatalogSkill(m, "root")["on_demand"].(map[string]any)["future_protocol"] = true
		}},
		{name: "malformed manifest", diagnostic: "decode", file: "packs/beta/pack.json", body: "{"},
		{name: "incompatible complete index", diagnostic: "schema"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := catalogBuildArchive(t, readIssue798File(t, filepath.Join(assets, "catalog-snapshot.tar.gz")))
			pack := "beta"
			if strings.Contains(tc.name, "unrelated") {
				pack = "zulu"
			}
			if tc.edit != nil {
				path := "packs/" + pack + "/pack.json"
				var m map[string]any
				if err := json.Unmarshal(files[path], &m); err != nil {
					t.Fatal(err)
				}
				tc.edit(m)
				data, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				files[path] = data
			}
			if tc.file != "" {
				files[tc.file] = []byte(tc.body)
			}
			if tc.name == "excluded dependency" {
				for path := range files {
					if strings.HasPrefix(path, "packs/beta/skills/leaf-codex/") {
						delete(files, path)
					}
				}
			}
			invalid := writeAccessArchive(t, files, tc.name == "incompatible complete index")
			// Authoring validates these same malformed declarations before any build.
			if tc.name != "incompatible complete index" {
				invalidProject := t.TempDir()
				for name, data := range files {
					if strings.HasPrefix(name, "packs/") {
						mode := os.FileMode(0644)
						if strings.HasSuffix(name, ".sh") {
							mode = 0755
						}
						writeAccessFile(t, filepath.Join(invalidProject, filepath.FromSlash(name)), data, mode)
					}
				}
				before := catalogTreeState(t, []string{invalidProject})
				for _, verb := range []string{"validate", "build"} {
					args := []string{"catalog", verb, "--project", invalidProject, "--json"}
					output := filepath.Join(t.TempDir(), "output")
					if verb == "build" {
						args = append(args, "--source-repository", "yersonargotev/packy-catalog", "--source-commit", strings.Repeat("d", 40), "--out-dir", output)
					}
					out, diagnostic, err := runCatalogCLI(t, buildEnvironment, t.TempDir(), executable, args...)
					schema := "catalog-validation"
					if verb == "build" {
						schema = "catalog-build"
					}
					validateCatalogCLISchema(t, schema, out)
					if err == nil || !strings.Contains(strings.ToLower(string(out)+string(diagnostic)), tc.diagnostic) {
						t.Fatalf("%s invalid catalog: %v %s %s", verb, err, out, diagnostic)
					}
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatalf("failed build exposed output: %v", err)
					}
					if !reflect.DeepEqual(before, catalogTreeState(t, []string{invalidProject})) {
						t.Fatal("validation/build changed source")
					}
				}
			}
			environment := testprocess.Env(t, "PATH="+t.TempDir())
			directory := t.TempDir()
			workspace := filepath.Join(t.TempDir(), "workspace")
			run := func(selected string, args ...string) ([]byte, []byte, error) {
				return runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", "candidate", "--snapshot", selected, "--workspace", workspace, "--"}, args...)...)
			}
			for _, host := range []string{"claude", "codex"} {
				out, diagnostic, err := run(assets, "get", "skill", "alpha:root", "--surface", host, "--json")
				if err != nil {
					t.Fatalf("baseline: %v %s %s", err, out, diagnostic)
				}
			}
			before := catalogTreeState(t, []string{workspace, project})
			for _, operation := range [][]string{{"get", "skill", "alpha:root", "--surface", "codex", "--json"}, {"transition", "--expect-archive-sha256", archive, "--json"}} {
				out, diagnostic, err := run(invalid, operation...)
				if err == nil || bytes.Contains(out, []byte(`"prepared"`)) || !strings.Contains(strings.ToLower(string(out)+string(diagnostic)), tc.diagnostic) {
					t.Fatalf("invalid complete candidate %v: %v %s %s", operation, err, out, diagnostic)
				}
				if operation[0] == "transition" {
					validateCatalogCLISchema(t, "catalog-candidate-transition", out)
				}
				if !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace, project})) {
					t.Fatal("failed candidate changed prior usable state")
				}
			}
			for _, host := range []string{"claude", "codex"} {
				out, diagnostic, err := run(assets, "get", "skill", "alpha:root", "--surface", host, "--json")
				if err != nil {
					t.Fatalf("prior selection unusable: %v %s %s", err, out, diagnostic)
				}
			}
		})
	}
}

// Repack an adversarial publication with consistent transport and per-file
// integrity. This deliberately bypasses the builder, so graph tests reach the
// consumer's semantic validation rather than failing on stale checksums/modes.
func writeAccessArchive(t *testing.T, files map[string][]byte, incompatible bool) string {
	t.Helper()
	var index cataloglayout.CatalogSnapshotIndex
	if err := json.Unmarshal(files["catalog-index.json"], &index); err != nil {
		t.Fatal(err)
	}
	digest := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	modes := map[string]int64{"catalog-index.json": 0644}
	for i := range index.Packs {
		pack := &index.Packs[i]
		pack.Files = slices.DeleteFunc(pack.Files, func(file cataloglayout.FileRecord) bool { _, ok := files["packs/"+pack.ID+"/"+file.Path]; return !ok })
		hash := sha256.New()
		for j := range pack.Files {
			file := &pack.Files[j]
			path := "packs/" + pack.ID + "/" + file.Path
			file.SHA256 = digest(files[path])
			fmt.Fprintf(hash, "%s\x00%s\x00%s\n", file.Path, file.Mode, file.SHA256)
			modes[path] = 0644
			if file.Mode == "100755" {
				modes[path] = 0755
			}
			if file.Path == "pack.json" {
				pack.ManifestSHA256 = file.SHA256
			}
		}
		pack.ClosureSHA256 = hex.EncodeToString(hash.Sum(nil))
	}
	encoded, err := json.Marshal(index.Packs)
	if err != nil {
		t.Fatal(err)
	}
	index.CatalogSHA256 = digest(encoded)
	index.Source.Commit = strings.Repeat("d", 40)
	if incompatible {
		index.SchemaVersion = 999
	}
	files["catalog-index.json"], err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		data := files[name]
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: modes[name], Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	assets := t.TempDir()
	writeAccessFile(t, filepath.Join(assets, "catalog-snapshot.tar.gz"), buffer.Bytes(), 0644)
	writeAccessFile(t, filepath.Join(assets, "SHA256SUMS"), []byte(digest(buffer.Bytes())+"  catalog-snapshot.tar.gz\n"), 0644)
	return assets
}

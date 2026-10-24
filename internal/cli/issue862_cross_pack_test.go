package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/skillaccess"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func crossPackAccessProject(t *testing.T, revision string) string {
	t.Helper()
	project := dependencyAccessProject(t, revision, nil)
	original := filepath.Join(project, "packs/reviewed")
	for _, id := range []string{"other", "third"} {
		root := filepath.Join(project, "packs", id)
		if err := os.CopyFS(root, os.DirFS(original)); err != nil {
			t.Fatal(err)
		}
		// CopyFS deliberately uses non-executable defaults; restore reviewed modes.
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".sh") {
				return os.Chmod(path, 0755)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		editCrossPackManifest(t, project, id, func(m map[string]any) { m["id"] = id; m["version"] = "2.0.0" })
	}
	editCrossPackManifest(t, project, "reviewed", func(m map[string]any) {
		accessResource(m, "guide")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "other:grilling"}, map[string]any{"type": "skill", "target": "third:grilling"}}
	})
	for _, id := range []string{"other", "third"} {
		editCrossPackManifest(t, project, id, func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "reviewed:shared"}}
		})
	}
	for _, host := range []string{"common", "claude", "codex"} {
		path := filepath.Join(original, "skills", host, "ACCESS.md")
		body := "Read [other]({{packy.access:other:grilling}}), [third]({{packy.access:third:grilling}}), [local]({{packy.access:grilling}}), [again]({{packy.access:other:grilling}}).\n"
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"other", "third"} {
		for _, host := range []string{"common", "claude", "codex"} {
			source := "grilling-" + host
			if host == "common" {
				source = "grilling"
			}
			body := id + " " + host + " " + revision + "\n" + accessSupportingLinks + "Read [diamond]({{packy.access:reviewed:shared}}) and [own]({{packy.access:shared}}).\n"
			if err := os.WriteFile(filepath.Join(project, "packs", id, "skills", source, "ACCESS.md"), []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return project
}

func editCrossPackManifest(t *testing.T, project, id string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(project, "packs", id, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	edit(manifest)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestIssue862CrossPackExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	if override := os.Getenv("PACKY_ISSUE862_BINARY"); override != "" {
		binary = override
	}
	project := crossPackAccessProject(t, "first")
	firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
	assets := dependencyAccessAssets(t, project, firstID)
	repository, _ := filepath.Abs(filepath.Join("..", ".."))

	t.Run("effective dependency declarations", func(t *testing.T) {
		variantProject := crossPackAccessProject(t, "variant")
		editCrossPackManifest(t, variantProject, "reviewed", func(m map[string]any) {
			guide := accessResource(m, "guide")
			for _, v := range guide["variants"].([]any) {
				variant := v.(map[string]any)
				host := variant["surface"].(string)
				owner := "other"
				if host == "claude" {
					owner = "third"
				}
				encoded, _ := json.Marshal(guide["on_demand"])
				var contract map[string]any
				if err := json.Unmarshal(encoded, &contract); err != nil {
					t.Fatal(err)
				}
				contract["dependencies"] = []any{map[string]any{"type": "skill", "target": owner + ":grilling"}}
				variant["on_demand"] = contract
				if err := os.WriteFile(filepath.Join(variantProject, "packs/reviewed/skills", host, "ACCESS.md"), []byte("Read [effective]({{packy.access:"+owner+":grilling}}).\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
		variantAssets := dependencyAccessAssets(t, variantProject, strings.Repeat("e", 40))
		for _, host := range []string{"codex", "claude"} {
			out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), "catalog", "candidate", "--snapshot", variantAssets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "get", "skill", "reviewed:guide", "--surface", host, "--json")
			if err != nil {
				t.Fatalf("effective graph: %v %s %s", err, out, stderr)
			}
			var result skillaccess.Result
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			owner, absent := "other", "third"
			if host == "claude" {
				owner, absent = "third", "other"
			}
			if len(result.Dependencies) != 5 {
				t.Fatalf("effective closure: %+v", result.Dependencies)
			}
			for _, d := range result.Dependencies {
				if strings.HasPrefix(d.QualifiedSkill, absent+":") {
					t.Fatalf("used another host's dependency: %+v", d)
				}
			}
			body, err := os.ReadFile(followAccessLink(t, result.EntryDocument, "effective"))
			if err != nil || !bytes.HasPrefix(body, []byte(owner+" "+host+" variant\n")) {
				t.Fatalf("effective target: %s %v", body, err)
			}
		}
	})
	for _, host := range []string{"codex", "claude"} {
		t.Run(host, func(t *testing.T) {
			envList := testprocess.Env(t, "PATH="+t.TempDir())
			env := adoptionEnvironmentMap(envList)
			directory := t.TempDir()
			acquireSkillAccessCatalog(t, env, directory, assets, firstID, "init")
			protected := map[string]string{}
			for _, relative := range []string{".packy/packs.json", ".agents/skills/grilling/SKILL.md", ".claude/skills/grilling/SKILL.md"} {
				path := filepath.Join(env["HOME"], relative)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("installed sentinel\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{directory, env["XDG_CONFIG_HOME"], filepath.Join(env["HOME"], ".packy"), filepath.Join(env["HOME"], ".agents"), filepath.Join(env["HOME"], ".claude"), project} {
				protected[path] = snapshotTree(t, path)
			}
			get := func(identity, snapshot string) skillaccess.Result {
				t.Helper()
				args := []string{"get", "skill", identity, "--surface", host, "--json"}
				if snapshot != "" {
					args = append(args, "--snapshot", snapshot)
				}
				out, stderr, err := runSkillAccessCLI(t, binary, directory, envList, args...)
				if err != nil {
					t.Fatalf("get: %v %s %s", err, out, stderr)
				}
				assertStructuredOutput(t, repository, "skill-access.schema.json", string(out))
				var result skillaccess.Result
				if err := json.Unmarshal(out, &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			first := get("reviewed:guide", "")
			identities := []string{}
			for _, d := range first.Dependencies {
				identities = append(identities, d.QualifiedSkill)
			}
			want := []string{"other:grilling", "other:shared", "reviewed:domain-modeling", "reviewed:grilling", "reviewed:shared", "third:grilling", "third:shared"}
			if !slices.Equal(identities, want) {
				t.Fatalf("dependencies: %v", identities)
			}
			for _, d := range append([]skillaccess.Skill{first.Skill}, first.Dependencies...) {
				version := "1.0.0"
				if !strings.HasPrefix(d.QualifiedSkill, "reviewed:") {
					version = "2.0.0"
				}
				if d.PackVersion != version || string(d.Surface) != host || d.SnapshotID != firstID || strings.Join(d.Prerequisites, ",") != "missing-fixture-runtime,missing-pack-runtime" {
					t.Fatalf("identity: %+v", d)
				}
				assertAccessFile(t, filepath.Join(d.ResourceRoot, "../../assets/data.bin"), []byte{0, 255, 7, 10}, 0644)
				owner := strings.Split(d.QualifiedSkill, ":")[0]
				manifest, err := os.ReadFile(filepath.Join(project, "packs", owner, "pack.json"))
				if err != nil {
					t.Fatal(err)
				}
				assertAccessFile(t, filepath.Join(d.ResourceRoot, "../../pack.json"), manifest, 0644)

				notice, err := os.ReadFile(filepath.Join(project, "packs", owner, "notices/reviewed-mit"))
				if err != nil {
					t.Fatal(err)
				}
				assertAccessFile(t, filepath.Join(d.ResourceRoot, "../../notices/reviewed-mit"), notice, 0644)
				if d.QualifiedSkill != "reviewed:guide" {
					id := strings.Split(d.QualifiedSkill, ":")[1]
					assertAccessFile(t, followAccessLink(t, d.EntryDocument, "Detail"), []byte("Complete supporting detail for "+id+"\n"), 0644)
					assertAccessFile(t, filepath.Join(d.ResourceRoot, "scripts/run.sh"), []byte("#!/bin/sh\n# {{packy.access:do-not-render}} /grilling $grilling Skill(\"grilling\")\nprintf executed > \"$HOME/sentinel-executed\"\n"), 0755)
				}
			}
			rootBody, err := os.ReadFile(first.EntryDocument)
			if err != nil {
				t.Fatal(err)
			}

			expectedRoot := "Read [other](../../../other/skills/grilling-" + host + "/ACCESS.md), [third](../../../third/skills/grilling-" + host + "/ACCESS.md), [local](../grilling-" + host + "/ACCESS.md), [again](../../../other/skills/grilling-" + host + "/ACCESS.md).\n"
			assertAccessFile(t, first.EntryDocument, []byte(expectedRoot), 0644)
			assertAccessFile(t, filepath.Join(first.ResourceRoot, "SKILL.md"), []byte("---\nname: guide\ndescription: Native wrapper.\n---\nNative /{{packy.skill:grilling}} call.\n"), 0644)
			for _, label := range []string{"other", "third"} {
				linked := followAccessLink(t, first.EntryDocument, label)
				// Compare complete effective bodies, including both qualified and local links.
				body, err := os.ReadFile(linked)
				if err != nil {
					t.Fatal(err)
				}
				prefix := label + " " + host + " first\n" + accessSupportingLinks
				if string(body) != prefix+"Read [diamond](../../../reviewed/skills/shared-"+host+"/ACCESS.md) and [own](../shared-"+host+"/ACCESS.md).\n" {
					t.Fatalf("effective body: %s", body)
				}
				assertAccessFile(t, followAccessLink(t, linked, "diamond"), []byte(host+" shared first\n"+accessSupportingLinks), 0644)
				assertAccessFile(t, followAccessLink(t, linked, "own"), []byte(host+" shared first\n"+accessSupportingLinks), 0644)
			}
			if followAccessLink(t, first.EntryDocument, "other") != followAccessLink(t, first.EntryDocument, "again") {
				t.Fatal("repeated target differs")
			}

			readable, stderr, err := runSkillAccessCLI(t, binary, directory, envList, "get", "skill", "reviewed:guide", "--surface", host)
			if err != nil {
				t.Fatalf("readable: %v %s", err, stderr)
			}
			for _, dependency := range first.Dependencies {
				for _, want := range []string{dependency.QualifiedSkill, "Pack " + dependency.PackVersion, dependency.EntryDocument, dependency.ResourceRoot, dependency.SnapshotID, strings.Join(dependency.Prerequisites, " ")} {
					if !bytes.Contains(readable, []byte(want)) {
						t.Fatalf("readable omitted %s: %s", want, readable)
					}
				}
			}
			again := get("reviewed:guide", "")
			if again.EntryDocument != first.EntryDocument {
				t.Fatal("reuse moved")
			}
			for path, before := range protected {
				if snapshotTree(t, path) != before {
					t.Fatalf("modified %s", path)
				}
			}
			if _, err := os.Stat(filepath.Join(env["HOME"], "sentinel-executed")); !os.IsNotExist(err) {
				t.Fatal("executed catalog script")
			}
			next := dependencyAccessAssets(t, crossPackAccessProject(t, "next"), nextID)
			acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
			nested := get("other:grilling", first.SnapshotID)
			assertAccessFile(t, followAccessLink(t, nested.EntryDocument, "diamond"), []byte(host+" shared first\n"+accessSupportingLinks), 0644)
			current := get("reviewed:guide", "")
			if current.SnapshotID != nextID {
				t.Fatal("refresh ignored")
			}
			assertAccessFile(t, first.EntryDocument, rootBody, 0644)
			assertAccessFile(t, followAccessLink(t, first.EntryDocument, "local"), []byte(host+" grilling first\n"+accessSupportingLinks+"Read [shared](../shared-"+host+"/ACCESS.md).\n"), 0644)
			if err := os.WriteFile(first.Dependencies[0].EntryDocument, []byte("consumer drift\n"), 0644); err != nil {
				t.Fatal(err)
			}
			out, _, err := runSkillAccessCLI(t, binary, directory, envList, "get", "skill", "reviewed:guide", "--surface", host, "--snapshot", firstID, "--json")
			if err == nil || !bytes.Contains(out, []byte("preparation_failed")) {
				t.Fatalf("drift: %v %s", err, out)
			}
			assertAccessFile(t, first.Dependencies[0].EntryDocument, []byte("consumer drift\n"), 0644)
			retained := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog/snapshots", firstID, "packs/other/skills/grilling-"+host+"/ACCESS.md")
			if err := os.Remove(retained); err != nil {
				t.Fatal(err)
			}
			out, _, err = runSkillAccessCLI(t, binary, directory, envList, "get", "skill", "other:grilling", "--surface", host, "--snapshot", firstID, "--json")
			if err == nil || !bytes.Contains(out, []byte("snapshot_unavailable")) {
				t.Fatalf("missing retained substituted: %v %s", err, out)
			}
			get("reviewed:guide", "")

		})
	}
}

func TestIssue862InvalidCrossPackCatalogExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	for _, test := range []struct {
		name, diagnostic string
		edit             func(map[string]any)
	}{
		{"missing Pack", "missing", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "absent:guide"}}
		}},
		{"missing skill", "missing", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "reviewed:absent"}}
		}},
		{"cycle", "cycle", func(m map[string]any) {
			accessResource(m, "shared")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "reviewed:guide"}}
		}},
		{"unsupported type", "type", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "workflow", "target": "reviewed:shared"}}
		}},
		{"malformed target", "qualified", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "../reviewed:shared"}}
		}},
		{"unknown vocabulary", "unknown field", func(m map[string]any) { accessResource(m, "grilling")["on_demand"].(map[string]any)["future"] = true }},
		{"null dependencies", "null", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = nil
		}},
		{"foreign source ownership", "source", func(m map[string]any) { accessResource(m, "grilling")["source"] = "../reviewed/skills/grilling" }},
		{"incompatible", "reviewed", func(m map[string]any) { delete(accessResource(m, "grilling"), "on_demand") }},
		{"excluded", "excluded", func(m map[string]any) {
			r := accessResource(m, "grilling")
			r["bindings"] = slices.DeleteFunc(r["bindings"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			r["variants"] = slices.DeleteFunc(r["variants"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			r["surface_exclusions"] = append(r["surface_exclusions"].([]any), map[string]any{"surface": "codex", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed"})
		}},
		{"undeclared qualified reference", "absent", func(m map[string]any) {
			accessResource(m, "grilling")["on_demand"].(map[string]any)["dependencies"] = []any{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := crossPackAccessProject(t, "invalid")
			editCrossPackManifest(t, project, "other", test.edit)
			before := snapshotTree(t, project)
			out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), "catalog", "validate", "--project", project, "--json")
			if err == nil || !strings.Contains(strings.ToLower(string(out)+stderr), test.diagnostic) {
				t.Fatalf("invalid whole catalog: %v %s %s", err, out, stderr)
			}
			if snapshotTree(t, project) != before {
				t.Fatal("validation mutated catalog")
			}
		})
	}
	for _, token := range []string{"{{packy.access:other::grilling}}", "{{packy.access:Other:grilling}}", "{{packy.access:other:grilling"} {
		t.Run(token, func(t *testing.T) {
			project := crossPackAccessProject(t, "malformed")
			if err := os.WriteFile(filepath.Join(project, "packs/other/skills/grilling-codex/ACCESS.md"), []byte(token), 0644); err != nil {
				t.Fatal(err)
			}
			out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "validate", "--project", project, "--json")
			if err == nil || !strings.Contains(strings.ToLower(string(out)+stderr), "directive") {
				t.Fatalf("malformed qualified reference: %v %s %s", err, out, stderr)
			}
		})
	}

}

// Construct an internally consistent but semantically invalid publication at the
// external catalog boundary; production builders must never admit these bytes.
func uncheckedCrossPackAssets(t *testing.T, project, id string) string {
	t.Helper()
	validation := cataloglayout.CatalogValidation{}
	index := cataloglayout.CatalogSnapshotIndex{SchemaVersion: 3, Source: cataloglayout.CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: id}, Builder: "yersonargotev/packy@" + strings.Repeat("e", 40)}
	for _, packID := range []string{"other", "reviewed", "third"} {
		v, err := cataloglayout.ValidateCatalogPack(context.Background(), project, packID, nil)
		if err != nil {
			t.Fatal(err)
		}
		validation.Packs = append(validation.Packs, v)
		index.Packs = append(index.Packs, cataloglayout.CatalogSnapshotPack{ID: packID, Version: v.Manifest.Version, ManifestSHA256: v.ManifestSHA256, ClosureSHA256: v.ClosureSHA256, Files: v.Files})
	}
	assets := filepath.Join(t.TempDir(), "invalid-builder")
	if _, err := cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: index.Source.Repository, Commit: id, Builder: index.Builder}, assets); err == nil {
		t.Fatal("builder admitted an invalid complete graph")
	}
	encoded, err := json.Marshal(index.Packs)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	index.CatalogSHA256 = digest(encoded)
	encoded, err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	writer := tar.NewWriter(gzipWriter)
	write := func(name string, data []byte, mode int64) {
		t.Helper()
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("catalog-index.json", encoded, 0644)
	for _, pack := range index.Packs {
		for _, file := range pack.Files {
			name := "packs/" + pack.ID + "/" + file.Path
			data, err := os.ReadFile(filepath.Join(project, name))
			if err != nil {
				t.Fatal(err)
			}
			mode := int64(0644)
			if file.Mode == "100755" {
				mode = 0755
			}
			write(name, data, mode)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	assets = t.TempDir()
	if err := os.WriteFile(filepath.Join(assets, "catalog-snapshot.tar.gz"), archive.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "SHA256SUMS"), []byte(digest(archive.Bytes())+"  catalog-snapshot.tar.gz\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return assets
}

func TestIssue862InvalidUnselectedPackPreservesSelection(t *testing.T) {
	project := crossPackAccessProject(t, "first")
	firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
	assets := dependencyAccessAssets(t, project, firstID)
	envList := testprocess.Env(t)
	env := adoptionEnvironmentMap(envList)
	directory := t.TempDir()
	acquireSkillAccessCatalog(t, env, directory, assets, firstID, "init")
	// third:guide is not part of reviewed:guide's procedure.
	editCrossPackManifest(t, project, "third", func(m map[string]any) {
		accessResource(m, "guide")["on_demand"].(map[string]any)["dependencies"] = []any{map[string]any{"type": "skill", "target": "absent:guide"}}
	})
	invalid := uncheckedCrossPackAssets(t, project, nextID)
	release := catalogstore.Release{Repository: "yersonargotev/packy-catalog", Tag: "catalog-" + nextID, Commit: nextID, Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true}
	for _, name := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(invalid, name))
		if err != nil {
			t.Fatal(err)
		}
		release.Assets = append(release.Assets, catalogstore.Asset{Name: name, Data: data, SHA256: catalogFixtureDigest(data)})
	}
	opts := Options{Env: env, Getwd: func() (string, error) { return directory, nil }, CatalogSource: &catalogSourceFixture{release: release}}
	store := catalogstore.New(catalogstore.DefaultDataRoot(env["HOME"]), nil)
	before, err := store.Selected()
	if err != nil {
		t.Fatal(err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh")
	if err == nil || !strings.Contains(out+err.Error(), "absent") {
		t.Fatalf("invalid refresh: %v %s", err, out)
	}
	after, err := store.Selected()
	if err != nil || after.ID != before.ID {
		t.Fatalf("failed refresh changed selection: %+v %v", after, err)
	}
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	stdout, stderr, err := runSkillAccessCLI(t, binary, directory, envList, "catalog", "candidate", "--snapshot", invalid, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
	if err == nil || !strings.Contains(string(stdout)+stderr, "absent") || bytes.Contains(stdout, []byte(`"prepared"`)) {
		t.Fatalf("invalid unselected Pack consumed: %v %s %s", err, stdout, stderr)
	}
}

func TestIssue862NativeLifecycleKeepsPackLocalOwnership(t *testing.T) {
	for _, host := range []string{"codex", "claude"} {
		t.Run(host, func(t *testing.T) {
			project := crossPackAccessProject(t, "native")
			for _, id := range []string{"reviewed", "other", "third"} {
				editCrossPackManifest(t, project, id, func(m map[string]any) {
					m["external_requirements"] = []string{}
					for _, value := range m["resources"].([]any) {
						r := value.(map[string]any)
						if r["kind"] == "skill" {
							r["requires"] = slices.DeleteFunc(r["requires"].([]any), func(v any) bool { return v == "asset:guide-data" })
						}
					}
				})
			}
			id := strings.Repeat("d", 40)
			assets := dependencyAccessAssets(t, project, id)
			env := adoptionEnvironmentMap(testprocess.Env(t))
			directory := t.TempDir()
			writeTestGitWorktree(t, directory)
			acquireSkillAccessCatalog(t, env, directory, assets, id, "init")
			opts := Options{Env: env, Getwd: func() (string, error) { return directory, nil }, Runner: &fakeRunner{}, Terminal: &fakeTerminal{interactive: true, approve: true}}
			before := snapshotTree(t, project)
			for _, verb := range []string{"activate", "install"} {
				out, err := executeCommand(t, NewRootCommand(opts), verb, "reviewed", "--surface", host, "--resource", "skill:guide", "--alias", "skill:grilling=local-grill")
				if err != nil {
					t.Fatalf("native %s: %v %s", verb, err, out)
				}
			}
			nativeRoot := filepath.Join(env["HOME"], ".agents/skills")
			if host == "claude" {
				nativeRoot = filepath.Join(env["HOME"], ".claude/skills")
			}
			assertAccessFile(t, filepath.Join(nativeRoot, "guide/SKILL.md"), []byte("---\nname: guide\ndescription: Native wrapper.\n---\nNative /local-grill call.\n"), 0644)
			for _, file := range []string{filepath.Join(directory, "packy.lock.json"), filepath.Join(env["HOME"], ".packy/packs.json")} {
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var document struct {
					Receipts []struct {
						Pack struct {
							ID string `json:"id"`
						} `json:"pack"`
					} `json:"receipts"`
				}
				if err := json.Unmarshal(data, &document); err != nil {
					t.Fatal(err)
				}
				if len(document.Receipts) != 1 || document.Receipts[0].Pack.ID != "reviewed" {
					t.Fatalf("foreign receipt: %s", data)
				}
			}

			entries, err := os.ReadDir(nativeRoot)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !slices.Equal(names, []string{"domain-modeling", "guide", "local-grill", "shared"}) {
				t.Fatalf("native ownership: %v", names)
			}
			if snapshotTree(t, project) != before {
				t.Fatal("native operation modified source catalog")
			}
			for _, verb := range []string{"uninstall", "deactivate"} {
				if out, err := executeCommand(t, NewRootCommand(opts), verb, "reviewed", "--surface", host); err != nil {
					t.Fatalf("%s: %v %s", verb, err, out)
				}
			}
		})
	}
}

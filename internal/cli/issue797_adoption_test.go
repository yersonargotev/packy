package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

type adoptionSentinel struct {
	bytes []byte
	mode  os.FileMode
}

func TestIssue797CleanAdoptionRehearsesPreviousCLIHandoffWithoutTouchingPersonalData(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("adoption")
	bundleRoot := t.TempDir()
	for _, group := range []string{"engineering", "productivity"} {
		if err := os.MkdirAll(filepath.Join(bundleRoot, "skills", group), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(bundleRoot, "skills", "in-progress", "loop-me")
	if err := os.MkdirAll(sentinel, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sentinel, "SKILL.md"), []byte("# Adoption rehearsal sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pack.WriteBundle(bundleRoot); err != nil {
		t.Fatal(err)
	}
	environment := testprocess.Env(t, "PACKY_SKILLS_SOURCE="+filepath.Join(bundleRoot, "skills"))
	env := adoptionEnvironmentMap(environment)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts := Options{Env: env, Getwd: func() (string, error) { return project, nil }, Runner: &fakeRunner{}, Terminal: terminal, skillSourceRoot: filepath.Join(bundleRoot, "skills")}

	protectedContent := map[string]string{
		filepath.Join(env["HOME"], "personal.txt"):                  "personal\n",
		filepath.Join(env["HOME"], ".credentials", "token"):         "secret\n",
		filepath.Join(env["HOME"], ".engram", "memory", "facts.md"): "memory\n",
		filepath.Join(env["HOME"], ".codex", "foreign.toml"):        "foreign host config\n",
		filepath.Join(project, "foreign-project.txt"):               "foreign project config\n",
	}
	protected := make(map[string]adoptionSentinel, len(protectedContent))
	for path, content := range protectedContent {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		protected[path] = adoptionSentinel{bytes: []byte(content), mode: info.Mode()}
	}

	packID := pack.ID()
	for _, args := range [][]string{
		{"activate", packID, "--surface", "codex"},
		{"install", packID, "--surface", "codex"},
		{"activate", packID, "--surface", "codex", "--project"},
	} {
		if output, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("seed previous installation %v: %v\n%s", args, err, output)
		}
	}

	executableDir := t.TempDir()
	catalogSnapshotCLI := filepath.Join(executableDir, "packy-catalog-snapshot-mode")
	build := exec.Command("go", "build", "-o", catalogSnapshotCLI, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build previous CLI: %v\n%s", err, output)
	}
	run := func(binary string, args ...string) (string, error) {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = project
		command.Env = append([]string(nil), environment...)
		output, err := command.CombinedOutput()
		return string(output), err
	}

	// A packaged CLI ignores the former source override and stays closed until
	// an official Catalog Snapshot has been selected.
	if output, err := run(catalogSnapshotCLI, "list"); err == nil || !strings.Contains(output, "run `packy init`") {
		t.Fatalf("packaged CLI accepted PACKY_SKILLS_SOURCE without a snapshot: %v\n%s", err, output)
	}

	for _, args := range [][]string{
		{"deactivate", packID, "--surface", "codex", "--project"},
		{"uninstall", packID, "--surface", "codex"},
		{"deactivate", packID, "--surface", "codex"},
	} {
		if output, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("apply previous handoff %v: %v\n%s", args, err, output)
		}
	}
	if _, err := os.Stat(bundleRoot); err != nil {
		t.Fatalf("old referenced source was removed during handoff: %v", err)
	}
	assertAdoptionProtectedFiles(t, protected)

	// The same packaged artifact resolves only the selected snapshot even while
	// the obsolete environment variable remains set.
	opts.skillSourceRoot = ""
	snapshotID := strings.Repeat("f", 40)
	opts.CatalogSource = &catalogSourceFixture{release: catalogReleaseFixture(t, snapshotID, pack)}
	if output, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil || !strings.Contains(output, "selected official Catalog Snapshot") {
		t.Fatalf("initialize current Catalog Snapshot: %v\n%s", err, output)
	}
	for _, args := range [][]string{
		{"activate", packID, "--surface", "codex"},
		{"install", packID, "--surface", "codex"},
		{"activate", packID, "--surface", "codex", "--project"},
	} {
		if output, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("apply current installation %v: %v\n%s", args, err, output)
		}
	}
	if output, err := run(catalogSnapshotCLI, "status", "--project"); err != nil || !strings.Contains(output, packID) {
		t.Fatalf("current CLI omitted reinstalled project Pack:\n%s", output)
	}
	assertAdoptionCatalogSnapshot(t, filepath.Join(env["HOME"], ".packy", "packs.json"), snapshotID)
	assertAdoptionCatalogSnapshot(t, filepath.Join(project, "packy.lock.json"), snapshotID)
	snapshotRoot := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", snapshotID)
	assertAdoptionProjectionSymlinksUseSnapshot(t, filepath.Join(env["HOME"], ".packy", "packs.json"), "", snapshotRoot)
	assertAdoptionProjectionSymlinksUseSnapshot(t, filepath.Join(project, "packy.lock.json"), project, snapshotRoot)
	if _, err := os.Stat(bundleRoot); err != nil {
		t.Fatalf("old source disappeared after Catalog Snapshot reinstall: %v", err)
	}
	assertAdoptionProtectedFiles(t, protected)
}

func adoptionEnvironmentMap(environment []string) MapEnv {
	result := MapEnv{}
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func assertAdoptionProtectedFiles(t *testing.T, files map[string]adoptionSentinel) {
	t.Helper()
	for path, want := range files {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(want.bytes) {
			t.Fatalf("protected file %s = %q, %v; want %q", path, data, err, want.bytes)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode() != want.mode {
			t.Fatalf("protected file %s mode = %v, %v; want %v", path, infoMode(info), err, want.mode)
		}
	}
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode()
}

func assertAdoptionCatalogSnapshot(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	found := 0
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "catalog_snapshot" {
					found++
					if child != want {
						t.Errorf("%s catalog_snapshot = %v, want %s", path, child, want)
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(document)
	if found == 0 {
		t.Fatalf("%s contains no catalog_snapshot", path)
	}
}

func assertAdoptionProjectionSymlinksUseSnapshot(t *testing.T, receiptPath, targetRoot, snapshotRoot string) {
	t.Helper()
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Receipts []struct {
			Projections []struct {
				Target string `json:"target"`
			} `json:"projections"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode projection receipts %s: %v", receiptPath, err)
	}
	for _, receipt := range document.Receipts {
		for _, projection := range receipt.Projections {
			target := filepath.FromSlash(projection.Target)
			if !filepath.IsAbs(target) {
				target = filepath.Join(targetRoot, target)
			}
			info, err := os.Lstat(target)
			if err != nil {
				t.Fatalf("inspect current projection %s: %v", target, err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			resolved, err := filepath.EvalSymlinks(target)
			if err != nil {
				t.Fatal(err)
			}
			relative, err := filepath.Rel(snapshotRoot, resolved)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				t.Errorf("current projection symlink %s resolves outside retained snapshot %s: %s", target, snapshotRoot, resolved)
			}
		}
	}
}

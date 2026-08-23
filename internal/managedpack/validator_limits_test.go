package managedpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProjectRejectsFileLargerThanIndexLimit(t *testing.T) {
	project, origin := writeValidProject(t)
	path := filepath.Join(project, "skills", "guide", "SKILL.md")
	if err := os.Truncate(path, (8<<20)+1); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum file size") {
		t.Fatalf("error = %v, want maximum file size rejection", err)
	}
}

func TestValidateProjectStopsWhenContextIsCanceled(t *testing.T) {
	project, origin := writeValidProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ValidateProject(ctx, project, originResolver{"upstream": origin})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestValidateProjectStopsWhenContextIsCanceledByOriginResolver(t *testing.T) {
	project, origin := writeValidProject(t)
	ctx, cancel := context.WithCancel(context.Background())

	_, err := ValidateProject(ctx, project, cancelingOriginResolver{root: origin, cancel: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

type cancelingOriginResolver struct {
	root   string
	cancel context.CancelFunc
}

func (r cancelingOriginResolver) Resolve(_ context.Context, _ Origin) (string, error) {
	r.cancel()
	return r.root, nil
}

func TestValidateProjectRejectsTooManyIndexedEntries(t *testing.T) {
	project, origin := writeValidProject(t)
	for index := 0; index < 1024; index++ {
		name := fmt.Sprintf("file-%04d", index)
		writeFile(t, filepath.Join(project, "skills", "guide", name), "x", 0o644)
		writeFile(t, filepath.Join(origin, "guide", name), "x", 0o644)
	}

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum entry count") {
		t.Fatalf("error = %v, want maximum entry count rejection", err)
	}
}

func TestValidateProjectRejectsIndexedPathThatIsTooDeep(t *testing.T) {
	project, origin := writeValidProject(t)
	components := make([]string, 30)
	for index := range components {
		components[index] = fmt.Sprintf("level-%02d", index)
	}
	projectPath := filepath.Join(append([]string{project, "skills", "guide"}, components...)...)
	originPath := filepath.Join(append([]string{origin, "guide"}, components...)...)
	writeFile(t, filepath.Join(projectPath, "deep.txt"), "deep", 0o644)
	writeFile(t, filepath.Join(originPath, "deep.txt"), "deep", 0o644)

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum path depth") {
		t.Fatalf("error = %v, want maximum path depth rejection", err)
	}
}

func TestValidateProjectRejectsAggregateBytesAboveIndexLimit(t *testing.T) {
	project, origin := writeValidProject(t)
	for index := 0; index < 9; index++ {
		name := fmt.Sprintf("large-%02d", index)
		for _, path := range []string{
			filepath.Join(project, "skills", "guide", name),
			filepath.Join(origin, "guide", name),
		} {
			writeFile(t, path, "", 0o644)
			if err := os.Truncate(path, 8<<20); err != nil {
				t.Fatal(err)
			}
		}
	}

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum aggregate size") {
		t.Fatalf("error = %v, want maximum aggregate size rejection", err)
	}
}

func TestMaterializeClosureCopiesExactValidatedFilesIntoBundleLayout(t *testing.T) {
	project, origin := writeValidProject(t)
	skillPath := filepath.Join(project, "skills", "guide", "SKILL.md")
	if err := os.Chmod(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()

	if err := MaterializeClosure(context.Background(), project, destination, validation); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(destination, "packs", "example", "pack.json"), validManifest)
	assertFileContent(t, filepath.Join(destination, "skills", "guide", "SKILL.md"), "managed guidance\n")
	assertFileContent(t, filepath.Join(destination, "notices", "mit"), "MIT notice\n")
	if _, err := os.Lstat(filepath.Join(destination, "pack.json")); !os.IsNotExist(err) {
		t.Fatalf("root pack.json error = %v, want not exist", err)
	}
	info, err := os.Lstat(filepath.Join(destination, "skills", "guide", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := canonicalMode(info.Mode()); got != "100755" {
		t.Fatalf("skill mode = %s, want 100755", got)
	}
}

func TestMaterializeClosureRejectsSourceDrift(t *testing.T) {
	project, origin := writeValidProject(t)
	validation, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "changed after validation\n", 0o644)

	err = MaterializeClosure(context.Background(), project, t.TempDir(), validation)
	if err == nil || !strings.Contains(err.Error(), "drifted from validated SHA-256") {
		t.Fatalf("error = %v, want source drift rejection", err)
	}
}

func TestMaterializeClosureRejectsValidatedManifestDrift(t *testing.T) {
	project, origin := writeValidProject(t)
	validation, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}
	validation.Manifest.Version = "9.9.9"

	err = MaterializeClosure(context.Background(), project, t.TempDir(), validation)
	if err == nil || !strings.Contains(err.Error(), "drifted from validated manifest") {
		t.Fatalf("error = %v, want validated manifest drift rejection", err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

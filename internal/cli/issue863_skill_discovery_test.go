package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue863SkillDiscoveryExecutable(t *testing.T) {
	binary := os.Getenv("PACKY_ISSUE863_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "packy")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
		build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	project := crossPackAccessProject(t, "discovery")
	assets := dependencyAccessAssets(t, project, strings.Repeat("a", 40))
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	directory := t.TempDir() // Outside Git, with no content runtimes or network tools.
	env := adoptionEnvironmentMap(environment)
	acquireSkillAccessCatalog(t, env, directory, assets, strings.Repeat("a", 40), "init")
	out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "show", "reviewed", "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("discover: %v %s %s", err, out, stderr)
	}
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	assertStructuredOutput(t, repository, "skill-discovery.schema.json", string(out))
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	if report["schema_version"] != float64(1) || report["report"] != "skill-discovery" || report["pack_id"] != "reviewed" || report["pack_version"] != "1.0.0" || report["surface"] != "codex" {
		t.Fatalf("identity: %s", out)
	}
	skills := report["skills"].([]any)
	if len(skills) != 4 {
		t.Fatalf("candidates: %s", out)
	}
	guide := skills[2].(map[string]any)
	if guide["qualified_skill"] != "reviewed:guide" || guide["compatible"] != true || guide["description"] != "Reviewed variant guide" {
		t.Fatalf("guide: %s", out)
	}
	if !reflect.DeepEqual(guide["prerequisites"], []any{"missing-fixture-runtime", "missing-pack-runtime"}) {
		t.Fatalf("prerequisites: %s", out)
	}
	if !reflect.DeepEqual(guide["on_demand"].(map[string]any)["dependencies"], []any{map[string]any{"type": "skill", "target": "other:grilling"}, map[string]any{"type": "skill", "target": "third:grilling"}}) {
		t.Fatalf("access dependencies: %s", out)
	}
	if bytes.Contains(out, []byte("Native wrapper")) {
		t.Fatal("discovery dumped entry content")
	}
}

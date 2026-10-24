package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func testCandidateAccessWithoutGit(t *testing.T, executable, assets string) {
	for _, damage := range []string{"missing Git", "missing HEAD", "empty HEAD", "missing objects", "foreign projection"} {
		t.Run(damage, func(t *testing.T) {
			environment := testprocess.Env(t, "PATH="+t.TempDir())
			enclosing, directory, foreign := t.TempDir(), t.TempDir(), t.TempDir()
			if out, diagnostic, err := runCatalogCLI(t, testprocess.Env(t), enclosing, "git", "init", "-q"); err != nil {
				t.Fatalf("enclosing Git: %v %s %s", err, out, diagnostic)
			}
			workspace := filepath.Join(enclosing, "workspace")
			run := func(args ...string) ([]byte, []byte, error) {
				return runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}, args...)...)
			}
			if out, diagnostic, err := run("list", "--json"); err != nil {
				t.Fatalf("open: %v %s %s", err, out, diagnostic)
			}
			gitRoot := filepath.Join(workspace, "project", ".git")
			switch damage {
			case "missing Git":
				if err := os.RemoveAll(gitRoot); err != nil {
					t.Fatal(err)
				}
			case "missing HEAD":
				if err := os.Remove(filepath.Join(gitRoot, "HEAD")); err != nil {
					t.Fatal(err)
				}
			case "empty HEAD":
				writeAccessFile(t, filepath.Join(gitRoot, "HEAD"), nil, 0644)
			case "missing objects":
				if err := os.RemoveAll(filepath.Join(gitRoot, "objects")); err != nil {
					t.Fatal(err)
				}
			case "foreign projection":
				if err := os.MkdirAll(filepath.Join(workspace, "project", ".agents"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, filepath.Join(workspace, "project", ".agents", "skills")); err != nil {
					t.Fatal(err)
				}
			}
			protected := []string{filepath.Join(enclosing, ".git"), filepath.Join(workspace, "project"), filepath.Join(workspace, "config"), filepath.Join(workspace, "snapshots"), directory, foreign}
			before := catalogTreeState(t, protected)
			for _, operation := range [][]string{{"list", "--json"}, {"show", "alpha", "--surface", "codex", "--json"}, {"get", "skill", "alpha:root", "--surface", "codex", "--json"}} {
				out, diagnostic, err := run(operation...)
				if damage == "foreign projection" {
					if err == nil {
						t.Fatalf("unsafe access accepted: %s %s", out, diagnostic)
					}
				} else {
					if err != nil {
						t.Fatalf("access requires Git: %v %s %s", err, out, diagnostic)
					}
					schema := "pack-list"
					if operation[0] == "show" {
						schema = "skill-discovery"
					}
					if operation[0] == "get" {
						schema = "skill-access"
					}
					validateCatalogCLISchema(t, schema, out)
				}
			}
			// Preparation is allowed only beneath the owned home data root.
			if !reflect.DeepEqual(before, catalogTreeState(t, protected)) {
				t.Fatal("access changed project, enclosing Git, or foreign state")
			}
			for _, operation := range [][]string{{"install", "alpha", "--surface", "codex", "--dry-run", "--json"}, {"activate", "alpha", "--surface", "codex", "--dry-run", "--json"}, {"uninstall", "alpha", "--surface", "codex", "--json"}} {
				beforeNative := catalogTreeState(t, []string{enclosing, directory, foreign})
				out, diagnostic, err := run(operation...)
				if err == nil || !strings.Contains(string(diagnostic), "candidate workspace") {
					t.Fatalf("native Git/ownership guard bypassed: %v %s %s", err, out, diagnostic)
				}
				if !reflect.DeepEqual(beforeNative, catalogTreeState(t, []string{enclosing, directory, foreign})) {
					t.Fatal("native operation changed enclosing or foreign state")
				}
			}
		})
	}
}

func testAccessIncompatibleDiscovery(t *testing.T, executable string, buildEnvironment []string, commit string) {
	project := writeAccessCatalog(t, "first", commit)
	editAccessManifest(t, project, "zulu", func(m map[string]any) {
		r := accessCatalogSkill(m, "root")
		delete(r, "on_demand")
		r["bindings"] = r["bindings"].([]any)[:1] // Claude remains natively supported.
		r["variants"] = r["variants"].([]any)[:1]
		r["surface_exclusions"] = []any{map[string]any{"surface": "codex", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed for Codex"}}
	})
	if err := os.RemoveAll(filepath.Join(project, "packs/zulu/skills/root-codex")); err != nil {
		t.Fatal(err)
	}
	assets, _ := buildAccessCatalog(t, executable, buildEnvironment, project, strings.Repeat("c", 40))
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	directory := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	run := func(args ...string) ([]byte, []byte, error) {
		return runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}, args...)...)
	}
	for _, host := range []string{"claude", "codex"} {
		out, diagnostic, err := run("show", "zulu", "--surface", host, "--json")
		if err != nil {
			t.Fatalf("incompatible discovery: %v %s %s", err, out, diagnostic)
		}
		validateCatalogCLISchema(t, "skill-discovery", out)
		var report capabilitypack.SkillDiscoveryReport
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Skills) != 4 {
			t.Fatalf("lost incompatible candidate: %s", out)
		}
		root := report.Skills[3]
		if root.QualifiedSkill != "zulu:root" || root.Compatible || root.OnDemand != nil || root.Reason == "" || !slices.Equal(root.SupportedSurfaces, []capabilitypack.Surface{"claude"}) {
			t.Fatalf("incompatible metadata: %+v", root)
		}
		code, message := "incompatible_resource", "reviewed"
		if host == "codex" {
			code, message = "unsupported_surface", "excluded"
			expected := &capabilitypack.SurfaceExclusion{Surface: "codex", Mode: "optional", Code: "unreviewed", Reason: "Not reviewed for Codex"}
			if !reflect.DeepEqual(root.Exclusion, expected) || !strings.Contains(root.Reason, expected.Reason) {
				t.Fatalf("excluded metadata: %+v", root)
			}
		} else if root.Exclusion != nil {
			t.Fatal("Claude inherited Codex exclusion")
		}
		out, diagnostic, err = run("get", "skill", "zulu:root", "--surface", host, "--json")
		assertAccessFailure(t, out, diagnostic, err, code, message)
	}
}

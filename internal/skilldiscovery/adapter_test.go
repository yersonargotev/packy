package skilldiscovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func putSkill(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: fixture\n---\n"+body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "helper.txt"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNativeAndCompatibilityDiscoveryInBothOrders(t *testing.T) {
	for _, other := range []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceClaude} {
		for _, reverse := range []bool{false, true} {
			t.Run(string(other)+map[bool]string{false: "-forward", true: "-reverse"}[reverse], func(t *testing.T) {
				root := t.TempDir()
				roots := map[capabilitypack.Surface]string{capabilitypack.SurfaceCodex: filepath.Join(root, "home/.agents/skills"), capabilitypack.SurfaceClaude: filepath.Join(root, "home/.claude/skills"), capabilitypack.SurfaceOpenCode: filepath.Join(root, "config/opencode/skills")}
				desiredHost, installedHost := capabilitypack.SurfaceOpenCode, other
				if reverse {
					desiredHost, installedHost = other, capabilitypack.SurfaceOpenCode
				}
				source := filepath.Join(root, "source")
				putSkill(t, source, "guide", "desired")
				target := filepath.Join(roots[desiredHost], "guide")
				existing := filepath.Join(roots[installedHost], "guide")
				putSkill(t, existing, "guide", "installed")
				observer := Observer{Surface: desiredHost, GlobalRoots: roots}
				observation := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{ID: "skill:guide", Goal: capabilitypack.ProjectionPresent, Action: capabilitypack.ProjectionAction{Source: source, Target: target}}}}
				facts, err := observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, observation)
				if err != nil || len(facts) != 1 || facts[0].Fingerprint == facts[0].OtherFingerprint {
					t.Fatalf("facts=%+v err=%v", facts, err)
				}
				putSkill(t, existing, "guide", "desired")
				facts, err = observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, observation)
				if err != nil || len(facts) != 1 || facts[0].Fingerprint != facts[0].OtherFingerprint {
					t.Fatalf("equal facts=%+v err=%v", facts, err)
				}
				if err := os.WriteFile(filepath.Join(existing, "helper.txt"), []byte("changed auxiliary"), 0644); err != nil {
					t.Fatal(err)
				}
				facts, err = observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, observation)
				if err != nil || facts[0].Fingerprint == facts[0].OtherFingerprint {
					t.Fatalf("auxiliary facts=%+v err=%v", facts, err)
				}
				putSkill(t, existing, "guide", "desired")
				if err := os.Chmod(filepath.Join(existing, "helper.txt"), 0700); err != nil {
					t.Fatal(err)
				}
				facts, err = observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, observation)
				if err != nil || facts[0].Fingerprint == facts[0].OtherFingerprint {
					t.Fatalf("executable mode facts=%+v err=%v", facts, err)
				}

			})
		}
	}
}

func TestDiscoveryUsesFrontmatterAndStopsAtWorktreeBoundary(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	nested := filepath.Join(project, "src")
	for _, dir := range []string{filepath.Join(project, ".git"), nested} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "source")
	putSkill(t, source, "guide", "desired")
	outside := filepath.Join(root, ".opencode/skills/guide")
	putSkill(t, outside, "guide", "outside")
	observer := Observer{Surface: capabilitypack.SurfaceCodex, WorkingDirectory: nested}
	inspection := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{ID: "skill:guide", Goal: capabilitypack.ProjectionPresent, Action: capabilitypack.ProjectionAction{Source: source, Target: filepath.Join(root, "home/.agents/skills/renamed")}}}}
	facts, err := observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, inspection)
	if err != nil || len(facts) != 0 {
		t.Fatalf("escaped worktree: %+v %v", facts, err)
	}
	foreign := filepath.Join(project, ".opencode/skills/another-directory")
	putSkill(t, foreign, "guide", "foreign")
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("---\ndescription: 'literal --- delimiter'\nname: guide\n---\nforeign\n"), 0644); err != nil {
		t.Fatal(err)
	}
	facts, err = observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, inspection)
	if err != nil || len(facts) != 1 || facts[0].OtherTarget != foreign {
		t.Fatalf("native frontmatter collision missed: %+v %v", facts, err)
	}
}

func TestDesiredSameNameTreesCannotHideBehindDifferentTargets(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "source-a"), filepath.Join(root, "source-b")
	putSkill(t, a, "same", "one")
	putSkill(t, b, "same", "two")
	observer := Observer{Surface: capabilitypack.SurfaceCodex}
	facts, err := observer.Observe(context.Background(), capabilitypack.SurfaceTransition{}, capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{
		{ID: "skill:one", Goal: capabilitypack.ProjectionPresent, Action: capabilitypack.ProjectionAction{Source: a, Target: filepath.Join(root, "skills/one")}},
		{ID: "skill:two", Goal: capabilitypack.ProjectionPresent, Action: capabilitypack.ProjectionAction{Source: b, Target: filepath.Join(root, "skills/two")}},
	}})
	if err != nil || len(facts) != 1 || facts[0].Fingerprint == facts[0].OtherFingerprint {
		t.Fatalf("same-name desired trees: %+v %v", facts, err)
	}
}

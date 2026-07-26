package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestVercelLifecycleExercisesEveryCodexWriteBoundaryAndExactDiff(t *testing.T) {
	for failed := 0; failed < 9; failed++ {
		t.Run(twoDigit(failed+1), func(t *testing.T) {
			root := t.TempDir()
			bundle := filepath.Join(root, "bundle")
			skills := filepath.Join(root, "home", ".agents", "skills")
			materializeVercelFixture(t, bundle)
			adapter := NewSurfaceAdapter(bundle, skills, filepath.Join(root, "home", ".codex", "AGENTS.md"))
			inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: vercelacceptance.Canonical().Pack})
			if err != nil {
				t.Fatal(err)
			}
			actions := projectionActions(inspection.Projections)
			if len(actions) != 9 {
				t.Fatalf("Codex write boundaries = %d, want 9", len(actions))
			}
			before := filesystemFacts(t, filepath.Join(root, "home"))
			broken := append([]capabilitypack.ProjectionAction(nil), actions...)
			broken[failed].Source = filepath.Join(root, "missing-source")
			if err := adapter.ApplyProjections(context.Background(), broken); err == nil || err.ID != broken[failed].ID {
				t.Fatalf("boundary %s failure = %v", broken[failed].ID, err)
			}
			if got := filesystemFacts(t, filepath.Join(root, "home")); !reflect.DeepEqual(got, before) {
				t.Fatalf("boundary %s left mutations:\n got %#v\nwant %#v", broken[failed].ID, got, before)
			}
			if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
				t.Fatalf("boundary %s recovery: %v", broken[failed].ID, err)
			}
			assertExactCodexDiff(t, filepath.Join(root, "home"), before, actions)
			assertCodexCommitRollbackAndRecovery(t, adapter, filepath.Join(root, "home"), actions, failed)
		})
	}
}

func TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	skills := filepath.Join(root, "home", ".agents", "skills")
	materializeVercelFixture(t, bundle)
	fixture := vercelacceptance.Canonical()
	adapter := NewSurfaceAdapter(
		bundle,
		skills,
		filepath.Join(root, "home", ".codex", "AGENTS.md"),
	)

	before, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skills); !os.IsNotExist(err) {
		t.Fatalf("preview mutated Codex skills root: %v", err)
	}
	if len(before.Projections) != 9 {
		t.Fatalf("Codex projections = %d, want 9", len(before.Projections))
	}

	wantNames := codexSkillBindings(t, fixture.Pack)
	gotNames := make([]string, 0, len(before.Projections))
	actions := make([]capabilitypack.ProjectionAction, 0, len(before.Projections))
	for _, projection := range before.Projections {
		if projection.Exists || projection.Goal != capabilitypack.ProjectionPresent {
			t.Fatalf("unexpected initial projection state: %#v", projection)
		}
		if projection.Action.Kind != capabilitypack.ActionSkillLink {
			t.Fatalf("%s action kind = %s", projection.ID, projection.Action.Kind)
		}
		name := strings.TrimPrefix(projection.ID, "skill:")
		gotNames = append(gotNames, name)
		if filepath.Base(projection.Action.Target) != name {
			t.Fatalf("%s target = %s", projection.ID, projection.Action.Target)
		}
		if projection.DesiredFingerprint == "" || projection.DesiredFingerprint == "missing" {
			t.Fatalf("%s did not fingerprint its complete source tree", projection.ID)
		}
		actions = append(actions, projection.Action)
	}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Codex names = %v, want %v", gotNames, wantNames)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}

	applied, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range applied.Projections {
		if !projection.Exists || projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("%s did not converge as a complete tree", projection.ID)
		}
		resolved, err := filepath.EvalSymlinks(projection.Action.Target)
		if err != nil {
			t.Fatal(err)
		}
		wantSource, err := filepath.EvalSymlinks(projection.Action.Source)
		if err != nil {
			t.Fatal(err)
		}
		if resolved != wantSource {
			t.Fatalf("%s resolves to %s, want %s", projection.ID, resolved, wantSource)
		}
	}

	remove, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(remove.Projections) != 9 {
		t.Fatalf("removal projections = %d, want 9", len(remove.Projections))
	}
	removeActions := make([]capabilitypack.ProjectionAction, 0, len(remove.Projections))
	for _, projection := range remove.Projections {
		if projection.Goal != capabilitypack.ProjectionAbsent ||
			projection.Action.Mode != capabilitypack.ProjectionDeleteTarget {
			t.Fatalf("non-inert removal candidate: %#v", projection)
		}
		removeActions = append(removeActions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), removeActions); err != nil {
		t.Fatal(err)
	}
	for _, name := range wantNames {
		if _, err := os.Lstat(filepath.Join(skills, name)); !os.IsNotExist(err) {
			t.Fatalf("Codex skill %s survived deactivation: %v", name, err)
		}
	}
}

func assertExactCodexDiff(t *testing.T, home string, before map[string]string, actions []capabilitypack.ProjectionAction) {
	t.Helper()
	got := filesystemFacts(t, home)
	want := make(map[string]string, len(before)+len(actions))
	for path, fact := range before {
		want[path] = fact
	}
	for _, action := range actions {
		addExpectedDirectories(t, home, filepath.Dir(action.Target), want)
		relative, err := filepath.Rel(home, action.Target)
		if err != nil {
			t.Fatal(err)
		}
		want[filepath.ToSlash(relative)] = symlinkFact(t, action.Target, action.Source)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex filesystem diff:\n got %#v\nwant %#v", got, want)
	}
}

func assertCodexCommitRollbackAndRecovery(t *testing.T, adapter *SurfaceAdapter, home string, actions []capabilitypack.ProjectionAction, failed int) {
	t.Helper()
	baseline := filesystemFacts(t, home)
	action := actions[failed]
	stage := filepath.Join(filepath.Dir(action.Target), ".packy-stage-"+localprojection.FingerprintBytes([]byte(string(action.Kind) + ":" + action.ID))[:12])
	blocker := stage + ".backup"
	if err := os.MkdirAll(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocker, "operator"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err == nil || err.ID != action.ID {
		t.Fatalf("commit boundary %s failure = %v", action.ID, err)
	}
	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if got := filesystemFacts(t, home); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("commit boundary %s did not roll back exactly:\n got %#v\nwant %#v", action.ID, got, baseline)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatalf("commit boundary %s recovery: %v", action.ID, err)
	}
	if got := filesystemFacts(t, home); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("commit boundary %s recovery changed exact facts", action.ID)
	}
}

func filesystemFacts(t *testing.T, root string) map[string]string {
	t.Helper()
	facts := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			facts[key] = "directory:" + info.Mode().Perm().String()
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			info, err = os.Lstat(path)
			if err != nil {
				return err
			}
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			facts[key] = "symlink:" + info.Mode().Perm().String() + ":" + target
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		facts[key] = "file:" + info.Mode().Perm().String() + ":" + hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return facts
}

func symlinkFact(t *testing.T, target, source string) string {
	t.Helper()
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	return "symlink:" + info.Mode().Perm().String() + ":" + source
}

func addExpectedDirectories(t *testing.T, root, dir string, facts map[string]string) {
	t.Helper()
	for current := dir; current != root; current = filepath.Dir(current) {
		relative, err := filepath.Rel(root, current)
		if err != nil || strings.HasPrefix(relative, "..") {
			t.Fatalf("target parent %s escapes %s: %v", current, root, err)
		}
		facts[filepath.ToSlash(relative)] = "directory:-rwx------"
	}
}

func twoDigit(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func materializeVercelFixture(t *testing.T, root string) {
	t.Helper()
	files, err := vercelacceptance.InspectExactArchive()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, file.Content, os.FileMode(file.Mode)&0o777); err != nil {
			t.Fatal(err)
		}
	}
}

func codexSkillBindings(t *testing.T, pack capabilitypack.Pack) []string {
	t.Helper()
	names := make([]string, 0, 9)
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface != capabilitypack.SurfaceCodex {
				continue
			}
			if binding.Projection != "skill" || binding.Invocation != "$"+binding.Name ||
				binding.Mode != "native" || binding.Sharing != "exclusive" {
				t.Fatalf("non-native Codex binding for %s: %#v", resource.ID, binding)
			}
			names = append(names, binding.Name)
		}
	}
	sort.Strings(names)
	return names
}

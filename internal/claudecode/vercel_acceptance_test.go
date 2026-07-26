package claudecode

import (
	"bytes"
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

func TestVercelLifecycleExercisesEveryClaudeWriteBoundaryAndExactDiff(t *testing.T) {
	for failed := 0; failed < 9; failed++ {
		t.Run(twoDigit(failed+1), func(t *testing.T) {
			root := t.TempDir()
			bundle := filepath.Join(root, "bundle")
			home := filepath.Join(root, "home")
			layout := NewCanonicalLayout(home)
			materializeClaudeVercelFixture(t, bundle)
			adapter := NewSurfaceAdapter(bundle, layout, filepath.Join(root, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(NewOwnershipSnapshot()))
			inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: vercelacceptance.Canonical().Pack})
			if err != nil {
				t.Fatal(err)
			}
			actions := projectionActions(inspection)
			if len(actions) != 9 {
				t.Fatalf("Claude write boundaries = %d, want 9", len(actions))
			}
			before := filesystemFacts(t, home)
			broken := append([]capabilitypack.ProjectionAction(nil), actions...)
			if broken[failed].Kind == ActionSkillTree {
				broken[failed].Content = "{}"
			} else {
				broken[failed].Source = filepath.Join(root, "missing-source")
			}
			if err := adapter.ApplyProjections(context.Background(), broken); err == nil {
				t.Fatalf("boundary %s failure = %v", broken[failed].ID, err)
			}
			if got := filesystemFacts(t, home); !reflect.DeepEqual(got, before) {
				t.Fatalf("boundary %s left mutations:\n got %#v\nwant %#v", broken[failed].ID, got, before)
			}
			if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
				t.Fatalf("boundary %s recovery: %v", broken[failed].ID, err)
			}
			assertExactClaudeDiff(t, home, before, actions)
			owned := vercelOwnedAdapter(t, bundle, layout, filepath.Join(root, "state"), inspection)
			assertClaudeCommitRollbackAndRecovery(t, owned, home, actions, failed)
		})
	}
}

func TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	home := filepath.Join(root, "home")
	layout := NewCanonicalLayout(home)
	materializeClaudeVercelFixture(t, bundle)
	fixture := vercelacceptance.Canonical()
	adapter := NewSurfaceAdapter(
		bundle,
		layout,
		filepath.Join(root, "state"),
		"claude",
		&recordingRunner{result: Result{Stdout: "2.1.203"}},
		StaticOwnershipSnapshot(NewOwnershipSnapshot()),
	)

	before, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.SkillsDir); !os.IsNotExist(err) {
		t.Fatalf("preview mutated Claude skills root: %v", err)
	}
	if len(before.Projections) != 9 {
		t.Fatalf("Claude projections = %d, want 9", len(before.Projections))
	}

	wantNames := claudeSkillBindings(t, fixture.Pack)
	gotNames := make([]string, 0, len(before.Projections))
	actions := make([]capabilitypack.ProjectionAction, 0, len(before.Projections))
	compositeCount := 0
	for _, projection := range before.Projections {
		if projection.Exists || projection.Goal != capabilitypack.ProjectionPresent {
			t.Fatalf("unexpected initial projection state: %#v", projection)
		}
		if projection.Action.Kind != ActionSkillLink && projection.Action.Kind != ActionSkillTree {
			t.Fatalf("%s action kind = %s", projection.ID, projection.Action.Kind)
		}
		if projection.Action.Kind == ActionSkillTree {
			compositeCount++
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
		t.Fatalf("Claude names = %v, want %v", gotNames, wantNames)
	}
	if compositeCount != 2 {
		t.Fatalf("Claude composite guideline trees = %d, want 2", compositeCount)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}

	applied, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	records := make([]OwnershipRecord, 0, len(applied.Projections))
	for _, projection := range applied.Projections {
		if !projection.Exists || projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("%s did not converge as a complete tree", projection.ID)
		}
		contributor := "pack:vercel:" + strings.TrimPrefix(projection.ID, "skill:")
		record := OwnershipRecord{
			StateOwner: "capabilitypack", ContributorID: contributor, Contributors: []string{contributor},
			ID: projection.ID, Kind: string(projection.Action.Kind), Target: projection.Action.Target,
			Fingerprint: projection.DesiredFingerprint, DeletionAuthorized: true,
		}
		if projection.Action.Kind == ActionSkillTree {
			provenance, err := decodeCompositeOwnership(projection.Action.AdapterProvenance)
			if err != nil {
				t.Fatal(err)
			}
			record.Composite = provenance
			if _, err := os.Stat(filepath.Join(projection.Action.Target, "references")); err != nil {
				t.Fatalf("%s omitted sealed dependency assets: %v", projection.ID, err)
			}
			skill, err := os.ReadFile(filepath.Join(projection.Action.Target, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(skill, []byte("../../references/")) || !bytes.Contains(skill, []byte("references/vercel-")) {
				t.Fatalf("%s did not preserve its sealed dependency through the Claude-local path", projection.ID)
			}
		} else {
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
			record.Skill = SkillIdentity{Surface: "claude", ProjectionID: projection.ID, Path: projection.Action.Target, SymlinkType: "directory", ResolvedTarget: wantSource, ExpectedSource: wantSource, SourceTreeFingerprint: projection.DesiredFingerprint}
		}
		records = append(records, record)
	}

	owned := NewSurfaceAdapter(bundle, layout, filepath.Join(root, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(NewOwnershipSnapshot(records...)))
	remove, err := owned.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(remove.Projections) != 9 {
		t.Fatalf("removal projections = %d, want 9", len(remove.Projections))
	}
	removeActions := make([]capabilitypack.ProjectionAction, 0, len(remove.Projections))
	for _, projection := range remove.Projections {
		if projection.Goal != capabilitypack.ProjectionAbsent || projection.Action.Mode != capabilitypack.ProjectionDeleteTarget {
			t.Fatalf("non-inert removal candidate: %#v", projection)
		}
		removeActions = append(removeActions, projection.Action)
	}
	if err := owned.ApplyProjections(context.Background(), removeActions); err != nil {
		t.Fatal(err)
	}
	for _, name := range wantNames {
		if _, err := os.Lstat(filepath.Join(layout.SkillsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("Claude skill %s survived deactivation: %v", name, err)
		}
	}
}

func projectionActions(inspection capabilitypack.SurfaceInspection) []capabilitypack.ProjectionAction {
	actions := make([]capabilitypack.ProjectionAction, 0, len(inspection.Projections))
	for _, projection := range inspection.Projections {
		actions = append(actions, projection.Action)
	}
	return actions
}

func assertExactClaudeDiff(t *testing.T, home string, before map[string]string, actions []capabilitypack.ProjectionAction) {
	t.Helper()
	got := filesystemFacts(t, home)
	for path, fact := range before {
		if got[path] != fact {
			t.Fatalf("Claude changed pre-existing path %s", path)
		}
		delete(got, path)
	}
	seen := make(map[string]bool, len(actions))
	for path := range got {
		absolute := filepath.Join(home, filepath.FromSlash(path))
		matched := false
		for _, action := range actions {
			if absolute == action.Target ||
				strings.HasPrefix(absolute, action.Target+string(filepath.Separator)) ||
				strings.HasPrefix(action.Target, absolute+string(filepath.Separator)) {
				seen[action.ID] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("Claude wrote outside exact action targets: %s", path)
		}
	}
	for _, action := range actions {
		if !seen[action.ID] {
			t.Fatalf("Claude action %s produced no exact filesystem diff", action.ID)
		}
	}
}

func vercelOwnedAdapter(t *testing.T, bundle string, layout CanonicalLayout, stateRoot string, inspection capabilitypack.SurfaceInspection) *SurfaceAdapter {
	t.Helper()
	records := make([]OwnershipRecord, 0, len(inspection.Projections))
	for _, projection := range inspection.Projections {
		name := strings.TrimPrefix(projection.ID, "skill:")
		contributor := "pack:vercel:" + name
		record := OwnershipRecord{
			StateOwner: "capabilitypack", ContributorID: contributor, Contributors: []string{contributor},
			ID: projection.ID, Kind: string(projection.Action.Kind), Target: projection.Action.Target,
			Fingerprint: projection.DesiredFingerprint, DeletionAuthorized: true,
		}
		if projection.Action.Kind == ActionSkillTree {
			provenance, err := decodeCompositeOwnership(projection.Action.AdapterProvenance)
			if err != nil {
				t.Fatal(err)
			}
			record.Composite = provenance
		} else {
			expected, err := canonicalPath(projection.Action.Source)
			if err != nil {
				t.Fatal(err)
			}
			record.Skill = SkillIdentity{
				Surface: "claude", ProjectionID: projection.ID, Path: projection.Action.Target,
				SymlinkType: "directory", ResolvedTarget: expected, ExpectedSource: expected,
				SourceTreeFingerprint: projection.DesiredFingerprint,
			}
		}
		records = append(records, record)
	}
	return NewSurfaceAdapter(bundle, layout, stateRoot, "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(NewOwnershipSnapshot(records...)))
}

func assertClaudeCommitRollbackAndRecovery(t *testing.T, adapter *SurfaceAdapter, home string, actions []capabilitypack.ProjectionAction, failed int) {
	t.Helper()
	action := actions[failed]
	baseline := filesystemFacts(t, home)
	suffix := localprojection.FingerprintBytes([]byte(action.ID + "\x00" + filepath.Clean(action.Target)))[:12]
	blocker := filepath.Join(filepath.Dir(action.Target), ".packy-batch-stage-"+suffix+".backup")
	if err := os.MkdirAll(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocker, "operator"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err == nil {
		t.Fatalf("Claude commit boundary %s unexpectedly succeeded", action.ID)
	}
	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if got := filesystemFacts(t, home); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("Claude commit boundary %s did not roll back exact facts", action.ID)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatalf("Claude commit boundary %s recovery: %v", action.ID, err)
	}
	if got := filesystemFacts(t, home); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("Claude commit boundary %s recovery changed exact facts", action.ID)
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

func twoDigit(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func materializeClaudeVercelFixture(t *testing.T, root string) {
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

func claudeSkillBindings(t *testing.T, pack capabilitypack.Pack) []string {
	t.Helper()
	names := make([]string, 0, 9)
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface != capabilitypack.SurfaceClaude {
				continue
			}
			if binding.Projection != "skill" || binding.Invocation != "/"+binding.Name || binding.Mode != "native" || binding.Sharing != "exclusive" {
				t.Fatalf("non-native Claude binding for %s: %#v", resource.ID, binding)
			}
			names = append(names, binding.Name)
		}
	}
	sort.Strings(names)
	return names
}

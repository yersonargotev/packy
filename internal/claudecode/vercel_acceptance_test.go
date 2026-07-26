package claudecode

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

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

package opencode

import (
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
	skills := filepath.Join(root, "home", ".agents", "skills")
	materializeVercelFixture(t, bundle)
	fixture := vercelacceptance.Canonical()
	adapter := NewSurfaceAdapter(bundle, skills, filepath.Join(root, "home", ".config", "opencode", "opencode.json"), filepath.Join(root, "home", ".config", "opencode", "AGENTS.md"))

	before, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: fixture.Pack})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skills); !os.IsNotExist(err) {
		t.Fatalf("preview mutated OpenCode skills root: %v", err)
	}

	wantNames := opencodeSkillBindings(t, fixture.Pack)
	gotNames := make([]string, 0, len(before.Projections))
	actions := make([]capabilitypack.ProjectionAction, 0, len(before.Projections))
	assetCount := 0
	for _, projection := range before.Projections {
		if !strings.HasPrefix(projection.ID, "skill:") {
			if !strings.HasPrefix(projection.ID, "asset:skill:") ||
				projection.Action.Kind != capabilitypack.ActionOpenCodeAssetFile {
				t.Fatalf("unexpected OpenCode Vercel projection: %#v", projection)
			}
			assetCount++
			actions = append(actions, projection.Action)
			continue
		}
		if projection.Exists || projection.Goal != capabilitypack.ProjectionPresent {
			t.Fatalf("unexpected initial projection state: %#v", projection)
		}
		if projection.Action.Kind != capabilitypack.ActionOpenCodeSkillLink {
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
	if len(before.Projections) != 11 || assetCount != 2 {
		t.Fatalf("OpenCode projections = %d with %d assets, want 11 with 2 assets", len(before.Projections), assetCount)
	}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("OpenCode names = %v, want %v", gotNames, wantNames)
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
			t.Fatalf("%s did not converge", projection.ID)
		}
		if !strings.HasPrefix(projection.ID, "skill:") {
			continue
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
			t.Fatalf("OpenCode skill %s survived deactivation: %v", name, err)
		}
	}
	for _, projection := range remove.Projections {
		if _, err := os.Lstat(projection.Action.Target); !os.IsNotExist(err) {
			t.Fatalf("OpenCode projection %s survived deactivation: %v", projection.ID, err)
		}
	}
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

func opencodeSkillBindings(t *testing.T, pack capabilitypack.Pack) []string {
	t.Helper()
	names := make([]string, 0, 9)
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface != capabilitypack.SurfaceOpenCode {
				continue
			}
			if binding.Projection != "skill" || binding.Invocation != binding.Name ||
				binding.Mode != "native" || binding.Sharing != "exclusive" {
				t.Fatalf("non-native OpenCode binding for %s: %#v", resource.ID, binding)
			}
			names = append(names, binding.Name)
		}
	}
	sort.Strings(names)
	return names
}

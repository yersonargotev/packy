package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestSkillTemplateRendersGlobalAndProjectTrees(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"helper":   "---\nname: helper\n---\nhelper\n",
		"consumer": "---\nname: consumer # keep\n---\nUse {{packy.skill:helper}}; keep /helper.\n",
	} {
		dir := filepath.Join(root, "catalog", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	capability := capabilitypack.SurfaceCapability{Type: capabilitypack.SurfaceCapabilitySkillTemplate, SkillTemplate: &capabilitypack.SkillTemplateCapability{Naming: capabilitypack.SkillTemplateNamingPack}}
	pack := capabilitypack.ResolvePackForSurface(capabilitypack.Pack{ID: "fixture", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "helper", Source: "helper", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "helper", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
		{Kind: "skill", ID: "consumer", Source: "consumer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "consumer", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
	}}, capabilitypack.SurfaceClaude)
	layout := NewCanonicalLayout(filepath.Join(root, "home"))
	adapter := NewSurfaceAdapter(filepath.Join(root, "catalog"), layout, filepath.Join(root, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	legacy := capabilitypack.Pack{ID: "fixture", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "helper", Source: "helper", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "helper"}}},
		{Kind: "skill", ID: "consumer", Source: "consumer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "consumer"}}},
	}}
	migration, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: legacy, Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	retired := claudeTemplateProjection(t, migration, "skill:helper")
	if retired.Goal != capabilitypack.ProjectionAbsent || retired.Action.Kind != ActionSkillLink || retired.Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("legacy projection was not retired: %+v", retired)
	}

	global, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	consumer := claudeTemplateProjection(t, global, "skill:fixture-consumer")
	assertClaudeTemplateTree(t, consumer.Action, capabilitypack.ActionSkillTree)
	actions := make([]capabilitypack.ProjectionAction, len(global.Projections))
	for i, projection := range global.Projections {
		actions[i] = projection.Action
	}
	if applyErr := adapter.ApplyProjections(context.Background(), actions); applyErr != nil {
		t.Fatal(applyErr)
	}
	if body, err := os.ReadFile(filepath.Join(layout.SkillsDir, "fixture-consumer", "SKILL.md")); err != nil || !strings.Contains(string(body), "Use fixture-helper; keep /helper.") {
		t.Fatalf("materialized template = %q err=%v", body, err)
	}

	project, err := adapter.inspectProject(context.Background(), pack, filepath.Join(root, "project"))
	if err != nil {
		t.Fatal(err)
	}
	projectConsumer := claudeTemplateProjection(t, project, "skill:consumer")
	assertClaudeTemplateTree(t, projectConsumer.Action, capabilitypack.ActionClaudeProjectSkillTree)
}

func claudeTemplateProjection(t *testing.T, inspection capabilitypack.SurfaceInspection, id string) capabilitypack.ObservedProjection {
	t.Helper()
	for _, projection := range inspection.Projections {
		if projection.ID == id {
			return projection
		}
	}
	t.Fatalf("projection %q missing: %+v", id, inspection.Projections)
	return capabilitypack.ObservedProjection{}
}

func assertClaudeTemplateTree(t *testing.T, action capabilitypack.ProjectionAction, kind capabilitypack.ProjectionActionKind) {
	t.Helper()
	if action.Kind != kind || action.Source != "" || len(action.TreeFiles) == 0 {
		t.Fatalf("template action = %+v", action)
	}
	for _, file := range action.TreeFiles {
		if file.Path == "SKILL.md" {
			body := string(file.Content)
			if !strings.Contains(body, "name: fixture-consumer # keep") || !strings.Contains(body, "Use fixture-helper; keep /helper.") {
				t.Fatalf("rendered SKILL.md = %q", body)
			}
			return
		}
	}
	t.Fatal("rendered action has no SKILL.md")
}

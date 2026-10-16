package opencode

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
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	capability := capabilitypack.SurfaceCapability{Type: capabilitypack.SurfaceCapabilitySkillTemplate, SkillTemplate: &capabilitypack.SkillTemplateCapability{Naming: capabilitypack.SkillTemplateNamingPackSurface}}
	pack := capabilitypack.ResolvePackForSurface(capabilitypack.Pack{ID: "fixture", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "helper", Source: "helper", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceOpenCode, Projection: "skill", Name: "helper", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
		{Kind: "skill", ID: "consumer", Source: "consumer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceOpenCode, Projection: "skill", Name: "consumer", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
	}}, capabilitypack.SurfaceOpenCode)
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "home", "skills"), filepath.Join(root, "home", "opencode.json"), filepath.Join(root, "home", "prompt.md"))

	global, err := adapter.inspectDesired(context.Background(), pack, nil)
	if err != nil {
		t.Fatal(err)
	}
	consumer := openCodeTemplateProjection(t, global, "skill:fixture-opencode-consumer")
	assertOpenCodeTemplateTree(t, consumer.Action, capabilitypack.ActionSkillTree)
	if applyErr := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{global.Projections[0].Action, global.Projections[1].Action}); applyErr != nil {
		t.Fatal(applyErr)
	}
	converged, err := adapter.inspectDesired(context.Background(), pack, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range converged.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("global template did not converge: %+v", projection)
		}
	}

	project, err := adapter.inspectProject(context.Background(), pack, filepath.Join(root, "project"))
	if err != nil {
		t.Fatal(err)
	}
	projectConsumer := openCodeTemplateProjection(t, project, "skill:consumer")
	assertOpenCodeTemplateTree(t, projectConsumer.Action, capabilitypack.ActionOpenCodeProjectSkillTree)
}

func openCodeTemplateProjection(t *testing.T, inspection capabilitypack.SurfaceInspection, id string) capabilitypack.ObservedProjection {
	t.Helper()
	for _, projection := range inspection.Projections {
		if projection.ID == id {
			return projection
		}
	}
	t.Fatalf("projection %q missing: %+v", id, inspection.Projections)
	return capabilitypack.ObservedProjection{}
}

func assertOpenCodeTemplateTree(t *testing.T, action capabilitypack.ProjectionAction, kind capabilitypack.ProjectionActionKind) {
	t.Helper()
	if action.Kind != kind || action.Source != "" || len(action.TreeFiles) == 0 {
		t.Fatalf("template action = %+v", action)
	}
	for _, file := range action.TreeFiles {
		if file.Path == "SKILL.md" {
			body := string(file.Content)
			if !strings.Contains(body, "name: fixture-opencode-consumer # keep") || !strings.Contains(body, "Use fixture-opencode-helper; keep /helper.") {
				t.Fatalf("rendered SKILL.md = %q", body)
			}
			return
		}
	}
	t.Fatal("rendered action has no SKILL.md")
}

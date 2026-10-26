package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestSkillTemplateRendersExactGlobalAndProjectTrees(t *testing.T) {
	root := t.TempDir()
	writeAgentsTemplateSkill(t, root, "helper", "---\nname: helper\n---\nhelper\n")
	writeAgentsTemplateSkill(t, root, "consumer", "---\nname: consumer # keep\n---\nUse {{packy.skill:helper}}; keep $helper.\n")
	pack := capabilitypack.ResolvePackForSurface(agentsTemplatePack(root), capabilitypack.SurfaceAgents)
	skills := filepath.Join(root, "home", "skills")
	adapter := NewSurfaceAdapter(root, skills, filepath.Join(root, "home", "AGENTS.md"))

	inspection, err := adapter.inspectDesired(context.Background(), pack, nil)
	if err != nil {
		t.Fatal(err)
	}
	consumer := agentsProjection(t, inspection, "skill:fixture-consumer")
	assertAgentsRenderedTree(t, consumer.Action, "fixture-consumer", "fixture-helper")
	if consumer.Action.Kind != capabilitypack.ActionSkillTree || consumer.Action.Source != "" {
		t.Fatalf("global template action = %+v", consumer.Action)
	}
	if applyErr := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{inspection.Projections[0].Action, inspection.Projections[1].Action}); applyErr != nil {
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
	projectConsumer := agentsProjection(t, project, "skill:consumer")
	assertAgentsRenderedTree(t, projectConsumer.Action, "fixture-consumer", "fixture-helper")
	if projectConsumer.Action.Kind != capabilitypack.ActionAgentsProjectSkillTree || projectConsumer.Action.FileMode != 0o755 {
		t.Fatalf("project template action = %+v", projectConsumer.Action)
	}

	removal, err := adapter.inspectPriorTransition(context.Background(), pack, capabilitypack.Pack{ID: "fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed := agentsProjection(t, removal, "skill:fixture-consumer")
	if removed.Action.Kind != capabilitypack.ActionSkillTree || removed.Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("template removal = %+v", removed.Action)
	}
}

func agentsTemplatePack(root string) capabilitypack.Pack {
	capability := capabilitypack.SurfaceCapability{Type: capabilitypack.SurfaceCapabilitySkillTemplate, SkillTemplate: &capabilitypack.SkillTemplateCapability{Naming: capabilitypack.SkillTemplateNamingPack}}
	return capabilitypack.Pack{ID: "fixture", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "helper", Source: "helper", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "skill", Name: "helper", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
		{Kind: "skill", ID: "consumer", Source: "consumer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceAgents, Projection: "skill", Name: "consumer", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
	}}
}

func writeAgentsTemplateSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func agentsProjection(t *testing.T, inspection capabilitypack.SurfaceInspection, id string) capabilitypack.ObservedProjection {
	t.Helper()
	for _, projection := range inspection.Projections {
		if projection.ID == id {
			return projection
		}
	}
	t.Fatalf("projection %q missing: %+v", id, inspection.Projections)
	return capabilitypack.ObservedProjection{}
}

func assertAgentsRenderedTree(t *testing.T, action capabilitypack.ProjectionAction, self, reference string) {
	t.Helper()
	for _, file := range action.TreeFiles {
		if file.Path == "SKILL.md" {
			body := string(file.Content)
			if !strings.Contains(body, "name: "+self+" # keep") || !strings.Contains(body, "Use "+reference+"; keep $helper.") {
				t.Fatalf("rendered SKILL.md = %q", body)
			}
			return
		}
	}
	t.Fatal("rendered action has no SKILL.md")
}

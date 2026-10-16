package codex

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
	writeCodexTemplateSkill(t, root, "helper", "---\nname: helper\n---\nhelper\n")
	writeCodexTemplateSkill(t, root, "consumer", "---\nname: consumer # keep\n---\nUse {{packy.skill:helper}}; keep $helper.\n")
	pack := capabilitypack.ResolvePackForSurface(codexTemplatePack(root), capabilitypack.SurfaceCodex)
	skills := filepath.Join(root, "home", "skills")
	adapter := NewSurfaceAdapter(root, skills, filepath.Join(root, "home", "AGENTS.md"))

	inspection, err := adapter.inspectDesired(context.Background(), pack, nil)
	if err != nil {
		t.Fatal(err)
	}
	consumer := codexProjection(t, inspection, "skill:fixture-codex-consumer")
	assertCodexRenderedTree(t, consumer.Action, "fixture-codex-consumer", "fixture-codex-helper")
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
	projectConsumer := codexProjection(t, project, "skill:consumer")
	assertCodexRenderedTree(t, projectConsumer.Action, "fixture-codex-consumer", "fixture-codex-helper")
	if projectConsumer.Action.Kind != capabilitypack.ActionCodexProjectSkillTree || projectConsumer.Action.FileMode != 0o755 {
		t.Fatalf("project template action = %+v", projectConsumer.Action)
	}

	removal, err := adapter.inspectPriorTransition(context.Background(), pack, capabilitypack.Pack{ID: "fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed := codexProjection(t, removal, "skill:fixture-codex-consumer")
	if removed.Action.Kind != capabilitypack.ActionSkillTree || removed.Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("template removal = %+v", removed.Action)
	}
}

func codexTemplatePack(root string) capabilitypack.Pack {
	capability := capabilitypack.SurfaceCapability{Type: capabilitypack.SurfaceCapabilitySkillTemplate, SkillTemplate: &capabilitypack.SkillTemplateCapability{Naming: capabilitypack.SkillTemplateNamingPackSurface}}
	return capabilitypack.Pack{ID: "fixture", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "helper", Source: "helper", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "helper", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
		{Kind: "skill", ID: "consumer", Source: "consumer", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "consumer", Capabilities: []capabilitypack.SurfaceCapability{capability}}}},
	}}
}

func writeCodexTemplateSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func codexProjection(t *testing.T, inspection capabilitypack.SurfaceInspection, id string) capabilitypack.ObservedProjection {
	t.Helper()
	for _, projection := range inspection.Projections {
		if projection.ID == id {
			return projection
		}
	}
	t.Fatalf("projection %q missing: %+v", id, inspection.Projections)
	return capabilitypack.ObservedProjection{}
}

func assertCodexRenderedTree(t *testing.T, action capabilitypack.ProjectionAction, self, reference string) {
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

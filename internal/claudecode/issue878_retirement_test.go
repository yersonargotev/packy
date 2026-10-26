package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestClaudeRetiredResourcesCannotUseSkillProjection(t *testing.T) {
	for _, kind := range []string{"command", "mcp_server", "lifecycle"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			if err := os.MkdirAll(source, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("retired"), 0600); err != nil {
				t.Fatal(err)
			}
			layout := NewCanonicalLayout(filepath.Join(root, "home"))
			adapter := NewSurfaceAdapter(root, layout, "", "", nil, nil)
			pack := capabilitypack.Pack{ID: "retired", Resources: []capabilitypack.Resource{{Kind: kind, ID: "retired", Source: "source", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "retired"}}}}}
			if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack}); err == nil {
				t.Fatal("retired resource accepted as a skill")
			}
			if _, err := os.Stat(layout.ConfigDir); !os.IsNotExist(err) {
				t.Fatalf("inspection mutated configuration: %v", err)
			}
		})
	}
}

func TestClaudeContentInspectionIgnoresUnrelatedRuntimeConfiguration(t *testing.T) {
	root := t.TempDir()
	layout := NewCanonicalLayout(filepath.Join(root, "home"))
	if err := os.MkdirAll(layout.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(layout.ConfigDir, "settings.json"), filepath.Join(layout.Home, ".claude.json")} {
		if err := os.WriteFile(path, []byte("unrelated malformed config"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "guide.md"), []byte("Review guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(root, layout, "", "", nil, nil)
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "guide"}}}}}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 1 || inspection.Readiness.Authorized || inspection.Readiness.Usable {
		t.Fatalf("inspection=%+v", inspection)
	}
	for _, path := range []string{filepath.Join(layout.ConfigDir, "settings.json"), filepath.Join(layout.Home, ".claude.json")} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "unrelated malformed config" {
			t.Fatalf("unrelated configuration changed: %s %q %v", path, data, err)
		}
	}
}

func TestClaudeAgentDocumentRetainsSupportingAssetClosure(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			root := t.TempDir()
			for path, body := range map[string]string{"reviewer.md": "---\nname: reviewer\ndescription: Reviewed role\n---\nReview the changes.\n", "support.md": "Required supporting guidance."} {
				if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			pack := capabilitypack.Pack{ID: "reviewed", Resources: []capabilitypack.Resource{
				{Kind: "agent", ID: "reviewer", Source: "reviewer.md", Requires: []string{"asset:support"}, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "reviewer", Capabilities: []capabilitypack.SurfaceCapability{{Type: capabilitypack.SurfaceCapabilityClaudeAgentDocument, ClaudeAgentDocument: &capabilitypack.ClaudeAgentDocumentCapability{Authority: capabilitypack.AgentAuthority{PermissionMode: "default"}}}}}}},
				{Kind: "asset", ID: "support", Source: "support.md"},
			}}
			transition := capabilitypack.SurfaceTransition{Desired: pack}
			if scope == "project" {
				transition.ProjectRoot = t.TempDir()
			}
			adapter := NewSurfaceAdapter(root, NewCanonicalLayout(filepath.Join(root, "home")), "", "", nil, nil)
			inspection, err := adapter.InspectSurface(context.Background(), transition)
			if err != nil {
				t.Fatal(err)
			}
			var content string
			for _, projection := range inspection.Projections {
				if projection.ID == "agent:reviewer" {
					content = projection.Action.Content
				}
			}
			if !strings.Contains(content, "Required supporting guidance.") || !strings.Contains(content, "does not establish runtime authorization or observed usability") {
				t.Fatalf("agent omitted required supporting content or policy boundary: %s", content)
			}
		})
	}
}

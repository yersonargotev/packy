package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestLockedProjectRuntimeKeepsOpenCodeConsentAndSecretsHostOwned(t *testing.T) {
	project := t.TempDir()
	config := filepath.Join(project, "opencode.json")
	if err := os.WriteFile(config, []byte("{\n  \"mcp\": {\n    \"memory\": {\"type\": \"local\", \"command\": [\"engram\", \"mcp\"], \"enabled\": true}\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(project, ".opencode", "packy-hooks", "memory.json")
	if err := os.MkdirAll(filepath.Dir(hook), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("{\"id\":\"memory\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcp, err := InspectMCPProjection(config, "memory", "engram", []string{"mcp"})
	if err != nil {
		t.Fatal(err)
	}
	hookDigest := localprojection.FingerprintBytes([]byte("{\"id\":\"memory\"}\n"))
	installation := capabilitypack.ProjectInstallation{
		Manifest: capabilitypack.ProjectContractProposal{Packs: []capabilitypack.ProjectManifestPack{{ID: "memory", Version: "1.0.0", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}}}},
		Lock: capabilitypack.ProjectLockProposal{
			Source: capabilitypack.ProjectPackSourceIdentity{PackID: "memory", PackVersion: "1.0.0"},
			Sensitive: []capabilitypack.ProjectSensitiveDisclosure{
				{Category: capabilitypack.ProjectActivationMCP, Surface: capabilitypack.SurfaceOpenCode, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "mcp_server"},
				{Category: capabilitypack.ProjectActivationHooks, Surface: capabilitypack.SurfaceOpenCode, Resource: capabilitypack.ResourceIdentity{Kind: "lifecycle", ID: "memory"}, Detail: "command_hook"},
				{Category: capabilitypack.ProjectActivationAuthentication, Surface: capabilitypack.SurfaceOpenCode, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "runtime:oauth"},
			},
			Bindings: []capabilitypack.LifecycleBinding{
				{Surface: capabilitypack.SurfaceOpenCode, Kind: "mcp_server", ID: "memory", Projection: "mcp_server", Name: "memory"},
				{Surface: capabilitypack.SurfaceOpenCode, Kind: "lifecycle", ID: "memory", Projection: "command_hook", Name: "memory"},
			},
			Projections: []capabilitypack.ProjectProjectionPlan{
				{Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Target: "opencode.json", DesiredFingerprint: mcp.DesiredFingerprint, Contributor: "surface:opencode:pack:memory", Command: "engram", Args: []string{"mcp"}},
				{Resource: capabilitypack.ResourceIdentity{Kind: "lifecycle", ID: "memory"}, Target: ".opencode/packy-hooks/memory.json", DesiredFingerprint: hookDigest, Contributor: "surface:opencode:pack:memory"},
			},
		},
	}
	adapter := NewSurfaceAdapter("", "", "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 2 || len(inspection.ProjectActivationActions) != 0 || inspection.Readiness.AuthorizationObserved || inspection.Readiness.UsabilityObserved {
		t.Fatalf("OpenCode runtime inspection = %#v", inspection)
	}
	for _, want := range []string{"approve the locked OpenCode MCP definition", "approve the locked OpenCode hook", "complete the host-owned OpenCode authentication"} {
		if !containsOpenCodeDetail(inspection.Readiness.PendingHumanActions, want) {
			t.Fatalf("pending actions = %#v; missing %q", inspection.Readiness.PendingHumanActions, want)
		}
	}
	if !containsOpenCodeDetail(inspection.Readiness.Evidence, "OpenCode project definitions match the lock") || !containsOpenCodeDetail(inspection.Readiness.Evidence, "hook artifact remains inert") {
		t.Fatalf("runtime evidence = %#v", inspection.Readiness.Evidence)
	}
	if data, readErr := os.ReadFile(config); readErr != nil || strings.Contains(string(data), "token") {
		t.Fatalf("project MCP declaration changed or included credentials: %q, %v", data, readErr)
	}
	if err := os.WriteFile(hook, []byte("{\"id\":\"memory\",\"changed\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionPresent})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Readiness.AuthorizationObserved || changed.Readiness.Authorized || !containsOpenCodeDetail(changed.Readiness.PendingHumanActions, "restore the exact locked OpenCode project definition lifecycle:memory") || !containsOpenCodeDetail(changed.Readiness.Evidence, "definitions differ from the lock") {
		t.Fatalf("changed OpenCode hook was treated as runnable: %#v", changed.Readiness)
	}
}

func containsOpenCodeDetail(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func TestProjectInspectionRepresentsEverySupportedOpenCodeResourceKind(t *testing.T) {
	root := t.TempDir()
	bundle, project := filepath.Join(root, "bundle"), filepath.Join(root, "project")
	for path, content := range map[string]string{
		"skills/one/SKILL.md": "# One\n", "instructions/one.md": "Project guidance\n", "agents/one.md": "Agent prompt\n",
		"commands/one.md": "Command prompt\n", "assets/one.md": "Reference\n",
	} {
		full := filepath.Join(bundle, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	binding := func(projection, name string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceOpenCode, Projection: projection, Name: name, Mode: "native", Sharing: "exclusive"}}
	}
	pack := capabilitypack.Pack{ID: "complete", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "one", Source: "skills/one", Bindings: binding("skill", "one")},
		{Kind: "instruction", ID: "one", Source: "instructions/one.md", Bindings: binding("instruction", "one")},
		{Kind: "agent", ID: "one", Source: "agents/one.md", Bindings: binding("agent", "one")},
		{Kind: "command", ID: "one", Source: "commands/one.md", Bindings: binding("command", "one")},
		{Kind: "mcp_server", ID: "one", Command: "server", Args: []string{"--stdio"}, Bindings: binding("mcp_server", "one")},
		{Kind: "lifecycle", ID: "one", Bindings: binding("lifecycle", "one")},
		{Kind: "asset", ID: "one", Source: "assets/one.md"},
	}}
	adapter := NewSurfaceAdapter(bundle, "", "", "")
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Unrepresentable) != 0 || len(inspection.Projections) != 7 {
		t.Fatalf("inspection = %#v", inspection)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range inspection.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".agents/skills/one/SKILL.md", "AGENTS.md", ".opencode/agents/one.md", ".opencode/commands/one.md", "opencode.json", ".opencode/packy-hooks/one.json", ".opencode/assets/one/one.md"} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
}

func TestOpenCodeProjectInstructionUsesSharedSurfaceNeutralContribution(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "instructions", "guide.md"), []byte("Shared guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "guide", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "instructions/guide.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceOpenCode, Projection: "instruction", Name: "guide"}}}}}
	inspection, err := NewSurfaceAdapter(bundle, "", "", "").InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 1 {
		t.Fatalf("instruction projections = %#v", inspection.Projections)
	}
	projection := inspection.Projections[0]
	if !projection.Shared || len(projection.DiscoverableBy) != 1 || projection.DiscoverableBy[0] != capabilitypack.SurfaceCodex || !strings.Contains(projection.Action.Content, "<!-- packy:project:instruction:guide:start -->") {
		t.Fatalf("shared instruction projection = %#v", projection)
	}
}

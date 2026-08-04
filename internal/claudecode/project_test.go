package claudecode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestClaudeProjectAdapterStructurallyMergesInstructionsAndMCP(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bundle, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "instructions", "memory.md"), []byte("Use durable memory.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "assets", "checklist.md"), []byte("# Checklist\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("# Team instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{\n  \"foreign\": true\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "portable", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "memory", Source: "instructions/memory.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "instruction", Name: "memory"}}},
		{Kind: "mcp_server", ID: "memory", Command: "engram", Args: []string{"mcp"}, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "mcp_server", Name: "memory"}}},
		{Kind: "lifecycle", ID: "session", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "command_hook", Name: "session", Hook: &capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Matcher: "", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 10, Failure: "warn"}}}},
		{Kind: "asset", ID: "checklist", Source: "assets/checklist.md"},
	}}
	adapter := NewSurfaceAdapter(bundle, NewCanonicalLayout(""), "", "", nil, nil)
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ProjectRoot: project})
	if err != nil {
		t.Fatal(err)
	}
	actions := make([]capabilitypack.ProjectionAction, len(inspection.Projections))
	for i := range inspection.Projections {
		actions[i] = inspection.Projections[i].Action
		actions[i].PreviewOnly = false
	}
	if actionErr := adapter.ApplyProjections(context.Background(), actions); actionErr != nil {
		t.Fatal(actionErr)
	}
	instructions, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(instructions), "# Team instructions") || !strings.Contains(string(instructions), "pack:portable:memory") {
		t.Fatalf("instructions = %q, err=%v", instructions, err)
	}
	mcpBytes, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mcp map[string]any
	if err := json.Unmarshal(mcpBytes, &mcp); err != nil || mcp["foreign"] != true || mcp["mcpServers"].(map[string]any)["memory"] == nil {
		t.Fatalf("mcp = %#v, err=%v", mcp, err)
	}
	hookDefinition := filepath.Join(project, ".claude", "packy-hooks", "session.json")
	hookBytes, err := os.ReadFile(hookDefinition)
	if err != nil || !strings.Contains(string(hookBytes), "SessionStart") {
		t.Fatalf("inert hook definition = %q, err=%v", hookBytes, err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("project install activated the hook through shared Claude settings: %v", err)
	}

	lock := capabilitypack.ProjectLockProposal{Bindings: []capabilitypack.LifecycleBinding{
		{Kind: "instruction", ID: "memory", Projection: "instruction", Name: "memory"},
		{Kind: "mcp_server", ID: "memory", Projection: "mcp_server", Name: "memory"},
		{Kind: "lifecycle", ID: "session", Projection: "command_hook", Name: "session"},
	}}
	for _, projection := range inspection.Projections {
		relative, relErr := filepath.Rel(project, projection.Action.Target)
		if relErr != nil {
			t.Fatal(relErr)
		}
		lock.Projections = append(lock.Projections, capabilitypack.ProjectProjectionPlan{
			Resource: capabilitypack.ResourceIdentity{Kind: strings.SplitN(projection.ID, ":", 2)[0], ID: strings.SplitN(projection.ID, ":", 2)[1]},
			Target:   filepath.ToSlash(relative), DesiredFingerprint: projection.DesiredFingerprint, Command: projection.Action.Command, Args: projection.Action.Args,
			Contributors: []string{"surface:claude:pack:portable"},
		})
	}
	installation := capabilitypack.ProjectInstallation{Manifest: capabilitypack.ProjectContractProposal{Packs: []capabilitypack.ProjectManifestPack{{ID: "portable", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude}}}}, Lock: lock}
	tampered := installation
	tampered.Lock.Projections = append([]capabilitypack.ProjectProjectionPlan(nil), lock.Projections...)
	for i := range tampered.Lock.Projections {
		if tampered.Lock.Projections[i].Resource.Kind == "asset" {
			tampered.Lock.Projections[i].Target = ".claude/other/checklist.md"
		}
	}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &tampered}); err == nil {
		t.Fatal("Claude locked inspection trusted a tampered asset target")
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation, ProjectGoal: capabilitypack.ProjectionAbsent})
	if err != nil {
		t.Fatal(err)
	}
	removeActions := make([]capabilitypack.ProjectionAction, len(removal.Projections))
	for i := range removal.Projections {
		removeActions[i] = removal.Projections[i].Action
	}
	if actionErr := adapter.ApplyProjections(context.Background(), removeActions); actionErr != nil {
		t.Fatal(actionErr)
	}
	instructions, err = os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil || string(instructions) != "# Team instructions\n" {
		t.Fatalf("removed instructions = %q, err=%v", instructions, err)
	}
	mcpBytes, err = os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil || json.Unmarshal(mcpBytes, &mcp) != nil || mcp["foreign"] != true {
		t.Fatalf("removed MCP = %q, err=%v", mcpBytes, err)
	}
	if _, err := os.Lstat(hookDefinition); !os.IsNotExist(err) {
		t.Fatalf("uninstall retained the inert hook definition: %v", err)
	}

	removed, err := removeProjectMCP(mcpBytes, "memory")
	if err != nil || json.Unmarshal(removed, &mcp) != nil || mcp["foreign"] != true {
		t.Fatalf("removed MCP = %q, err=%v", removed, err)
	}
}

func TestLockedClaudeProjectInspectionRequiresNativeEvidenceForExactHookIdentity(t *testing.T) {
	project := t.TempDir()
	layout := NewCanonicalLayout(t.TempDir())
	if err := os.MkdirAll(filepath.Join(project, ".claude", "packy-hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	hook := capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 10, Failure: "warn"}
	definition, err := json.MarshalIndent(struct {
		ID      string                 `json:"id"`
		Binding capabilitypack.Binding `json:"binding"`
	}{ID: "session", Binding: capabilitypack.Binding{Surface: capabilitypack.SurfaceClaude, Projection: "command_hook", Name: "session", Hook: &hook}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	definition = append(definition, '\n')
	hookPath := filepath.Join(project, ".claude", "packy-hooks", "session.json")
	if err := os.WriteFile(hookPath, definition, 0o600); err != nil {
		t.Fatal(err)
	}
	lock := capabilitypack.ProjectLockProposal{
		Bindings:    []capabilitypack.LifecycleBinding{{Surface: capabilitypack.SurfaceClaude, Kind: "lifecycle", ID: "session", Projection: "command_hook", Name: "session"}},
		Projections: []capabilitypack.ProjectProjectionPlan{{Resource: capabilitypack.ResourceIdentity{Kind: "lifecycle", ID: "session"}, Target: ".claude/packy-hooks/session.json", DesiredFingerprint: Fingerprint(definition), Contributors: []string{"surface:claude:pack:portable"}}},
	}
	installation := capabilitypack.ProjectInstallation{Manifest: capabilitypack.ProjectContractProposal{Packs: []capabilitypack.ProjectManifestPack{{ID: "portable", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude}}}}, Lock: lock}
	a := NewSurfaceAdapterWithAuthorization("", layout, "", "", nil, nil, AuthorizationObserverFunc(func(context.Context) AuthorizationObservation {
		return AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	}))
	inspection, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.ProjectActivationActions) != 0 || !inspection.Readiness.AuthorizationObserved || !inspection.Readiness.Authorized || inspection.Readiness.UsabilityObserved || len(inspection.Readiness.PendingHumanActions) == 0 {
		t.Fatalf("inspection=%+v", inspection)
	}
	identity := Fingerprint([]byte("lifecycle:session=" + Fingerprint(definition)))
	a.WithRuntimeEvidence(staticRuntimeEvidence([]RuntimeEvidence{{Kind: claudeProjectRuntimeEvidenceKind, ID: "project_runtime:claude", Signal: "usable", Revision: identity}}))
	inspection, err = a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation})
	if err != nil || !inspection.Readiness.AuthorizationObserved || !inspection.Readiness.Authorized || !inspection.Readiness.UsabilityObserved || !inspection.Readiness.Usable || len(inspection.ProjectActivationActions) != 0 {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime inspection wrote personal state into the project: %v", err)
	}
	if err := os.WriteFile(hookPath, append([]byte("changed"), definition...), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err = a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{ProjectRoot: project, ProjectInstallation: &installation})
	if err != nil || inspection.Readiness.Authorized || len(inspection.ProjectActivationActions) != 0 || len(inspection.Readiness.PendingHumanActions) == 0 {
		t.Fatalf("changed hook inspection=%+v err=%v", inspection, err)
	}
}

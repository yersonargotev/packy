package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue461ProjectRuntimeCompositionUsesOneHumanAndStructuredVocabulary(t *testing.T) {
	effect := capabilitypack.ProjectRuntimeEffectStatus{
		Category: capabilitypack.ProjectActivationMCP, Resource: capabilitypack.ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "mcp_server",
		Coverage: capabilitypack.ProjectRuntimeCoverageInheritedGlobal, GlobalVersion: "1.0.0",
	}
	status := capabilitypack.JSONProjectStatusReport{SchemaVersion: capabilitypack.ProjectStatusSchemaVersion, Report: "project-status", ProjectRoot: "<project-root>", Packs: []capabilitypack.JSONProjectPackStatus{{
		Pack: capabilitypack.ProjectManifestPack{ID: "runtime-pack", Version: "1.0.0"}, Surface: capabilitypack.SurfaceOpenCode,
		Installation: capabilitypack.ProjectInstallationInstalled, Runtime: capabilitypack.ProjectRuntimeInheritedGlobal, RuntimeRequired: true,
		RuntimeEffects: []capabilitypack.ProjectRuntimeEffectStatus{effect}, Projections: []capabilitypack.ProjectProjectionStatus{}, Blockers: []capabilitypack.ProjectInstallBlocker{}, PendingHumanActions: []string{}, Evidence: []string{},
	}}}
	var human bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&human)
	if err := renderProjectStatus(cmd, status); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Runtime activation: inherited-global", "coverage=inherited-global", "global_version=1.0.0"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("human status omitted %q:\n%s", want, human.String())
		}
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema_version":3`, `"runtime":"inherited-global"`, `"coverage":"inherited-global"`, `"global_version":"1.0.0"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("structured status omitted %q: %s", want, encoded)
		}
	}
}

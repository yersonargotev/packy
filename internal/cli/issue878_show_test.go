package cli

import (
	"encoding/json"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue878HumanShowDisclosesClaudeRoleClientAndScopes(t *testing.T) {
	pack := testsupport.CapabilityRich("reviewed-show").WithSurfaces(testsupport.SurfaceClaude)
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	output, err := executeCommand(t, NewRootCommand(fixture.options), "show", pack.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Custom-agent projection: agent:reviewer client=claude scopes=global, project") {
		t.Fatalf("human show omitted reviewed client/scopes: %s", output)
	}
}

func TestIssue878ProjectPreviewDisclosesReviewedRoleTarget(t *testing.T) {
	pack := testsupport.CapabilityRich("project-role-preview").WithSurfaces(testsupport.SurfaceClaude)
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	output, err := executeCommand(t, NewRootCommand(fixture.options), "install", pack.ID(), "--surface", "claude", "--resource", "agent:reviewer", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Custom-agent projection: agent:reviewer client=claude scope=project supported_scopes=global, project") {
		t.Fatalf("project preview omitted reviewed role target: %s", output)
	}
	output, err = executeCommand(t, NewRootCommand(fixture.options), "install", pack.ID(), "--surface", "claude", "--resource", "agent:reviewer", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(output), &preview); err != nil {
		t.Fatal(err)
	}
	for _, projection := range preview.Projections {
		if projection.Resource.Kind == "agent" {
			if projection.ClientProjection == nil || projection.ClientProjection.Client != "claude" || strings.Join(projection.ClientProjection.Scopes, ",") != "global,project" {
				t.Fatalf("project JSON omitted reviewed role target: %+v", projection)
			}
			return
		}
	}
	t.Fatal("project preview omitted required agent")
}

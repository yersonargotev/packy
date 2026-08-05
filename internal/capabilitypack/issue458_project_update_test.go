package capabilitypack

import (
	"strings"
	"testing"
)

func TestIssue458ClassifiesSensitiveVersionChangesForFreshPersonalActivation(t *testing.T) {
	oldHook := ProjectSensitiveDisclosure{Category: ProjectActivationHooks, Surface: SurfaceCodex, Resource: ResourceIdentity{Kind: "lifecycle", ID: "memory"}, Detail: "hook:memory"}
	newMCP := ProjectSensitiveDisclosure{Category: ProjectActivationMCP, Surface: SurfaceOpenCode, Resource: ResourceIdentity{Kind: "mcp_server", ID: "memory"}, Detail: "mcp:memory"}
	prior := ProjectLockProposal{Source: ProjectPackSourceIdentity{PackID: "memory", PackVersion: "1.0.0"}, Sensitive: []ProjectSensitiveDisclosure{oldHook}}
	desired := ProjectLockProposal{Source: ProjectPackSourceIdentity{PackID: "memory", PackVersion: "2.0.0"}, Sensitive: []ProjectSensitiveDisclosure{oldHook, newMCP}}

	changes := projectSensitiveChanges(prior, desired)
	if len(changes) != 2 || changes[0].Change != "changed" || changes[0].Resource != oldHook.Resource || changes[1].Change != "added" || changes[1].Resource != newMCP.Resource {
		t.Fatalf("sensitive changes = %#v", changes)
	}
	for _, change := range changes {
		if !strings.Contains(change.Detail, "fresh personal project activation") {
			t.Fatalf("sensitive change omitted reactivation disclosure: %#v", change)
		}
	}
}

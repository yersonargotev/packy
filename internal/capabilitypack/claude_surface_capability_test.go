package capabilitypack

import (
	"strings"
	"testing"
)

func TestCurrentPackAcceptsClosedClaudeCompositionCapabilities(t *testing.T) {
	if err := validateCurrentPack(validClaudeCapabilityPack()); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentPackRejectsMalformedClaudeCompositionBeforeUse(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Pack)
		want string
	}{
		{"skill outside closure", func(pack *Pack) { pack.Resources[0].Requires = []string{} }, "direct dependency"},
		{"missing reference", func(pack *Pack) { pack.Resources = pack.Resources[:2] }, "does not exist"},
		{"unsupported authority", func(pack *Pack) {
			pack.Resources[0].Bindings[0].Capabilities[0].ClaudeAgentDocument.Authority.PermissionMode = "bypass"
		}, "permission_mode must be default"},
		{"conflicting payload", func(pack *Pack) {
			pack.Resources[0].Bindings[0].Capabilities[0].ClaudeCompositeSkill = &ClaudeCompositeSkillCapability{Dependencies: []ResourceIdentity{}, References: []ResourceIdentity{}}
		}, "does not accept other capability data"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pack := validClaudeCapabilityPack()
			test.edit(&pack)
			err := validateCurrentPack(pack)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func validClaudeCapabilityPack() Pack {
	asset := Resource{Kind: "asset", ID: "guide", Source: "references/guide.md", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}}
	skill := Resource{
		Kind: "skill", ID: "workflow", Source: "skills/workflow", Description: "Workflow",
		Requires: []string{"asset:guide"}, Conflicts: []string{}, SurfaceExclusions: []SurfaceExclusion{},
		Bindings: []Binding{{
			Surface: SurfaceClaude, Projection: "skill", Name: "workflow", Invocation: "/workflow", Mode: "native", Sharing: "exclusive",
			Capabilities: []SurfaceCapability{{
				Type: SurfaceCapabilityClaudeCompositeSkill,
				ClaudeCompositeSkill: &ClaudeCompositeSkillCapability{
					Dependencies: []ResourceIdentity{},
					References:   []ResourceIdentity{{Kind: "asset", ID: "guide"}},
				},
			}},
		}},
	}
	agent := Resource{
		Kind: "agent", ID: "reviewer", Source: "agents/reviewer.md", Description: "Reviewer", Mode: "subagent",
		Tools: []string{}, Permissions: []string{}, Requires: []string{"skill:workflow"}, Conflicts: []string{}, SurfaceExclusions: []SurfaceExclusion{},
		Bindings: []Binding{{
			Surface: SurfaceClaude, Projection: "agent", Name: "reviewer", Invocation: "@reviewer", Mode: "native", Sharing: "exclusive",
			Capabilities: []SurfaceCapability{{
				Type: SurfaceCapabilityClaudeAgentDocument,
				ClaudeAgentDocument: &ClaudeAgentDocumentCapability{
					Skills:    []ResourceIdentity{{Kind: "skill", ID: "workflow"}},
					Authority: AgentAuthority{PermissionMode: "default", Authorities: []AuthorityRecord{}},
				},
			}},
		}},
	}
	return Pack{
		ID: "synthetic", Version: "1.0.0", Description: "Synthetic", Selectable: true,
		Surfaces: []Surface{SurfaceClaude}, ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization},
		Requires: Requirements{Tools: []string{}}, Resources: []Resource{agent, asset, skill}, Contract: Contract{OptionalModes: []OptionalMode{}},
	}
}

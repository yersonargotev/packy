package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestRuntimeModeHumanRendererUsesCompleteNormalizedFacts(t *testing.T) {
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	state := capabilitypack.RuntimeModeUnverified
	mode := capabilitypack.RuntimeModeResult{
		ResourceID: "vercel-deploy", ModeID: "preview", Role: capabilitypack.RuntimeModePrimary,
		State: capabilitypack.RuntimeModeUnavailable,
		Requirements: []capabilitypack.RuntimeRequirement{{
			Kind: capabilitypack.RuntimeRequirementTool, ID: "vercel", Version: ">=53.0.0",
		}},
		Authorities: []capabilitypack.RuntimeAuthority{{
			Kind: capabilitypack.RuntimeAuthorityNetwork, Scope: capabilitypack.RuntimeScopeVercelAccount,
		}},
		Effects: []capabilitypack.RuntimeEffect{{
			Kind: capabilitypack.RuntimeEffectPreviewDeployment, Scope: capabilitypack.RuntimeScopeDeploymentPayload,
		}},
		Fallback:      capabilitypack.RuntimeFallback{Kind: capabilitypack.RuntimeFallbackMode, Mode: "local"},
		FallbackState: &state, OnUnavailable: capabilitypack.RuntimeFailBeforeEffects,
		Evidence: capabilitypack.RuntimeEvidence{
			Requirements: []capabilitypack.RuntimeRequirementObservation{{
				Kind: capabilitypack.RuntimeRequirementTool, ID: "vercel",
				RuntimeObservation: capabilitypack.RuntimeObservation{
					State: capabilitypack.ObservationUnavailable, Reason: capabilitypack.ObservationReasonNotFound,
					ObservedAt: "2026-07-25T12:00:00Z", ObserverRevision: "codex-host-v1", RedactedIdentity: "vercel",
				},
			}},
			Authorities: []capabilitypack.RuntimeAuthorityObservation{{
				Kind: capabilitypack.RuntimeAuthorityNetwork, Scope: capabilitypack.RuntimeScopeVercelAccount,
				RuntimeObservation: capabilitypack.RuntimeObservation{
					State: capabilitypack.ObservationUnverified, Reason: capabilitypack.ObservationReasonObserverError,
					ObservedAt: "2026-07-25T12:00:00Z", ObserverRevision: "codex-host-v1",
				},
			}},
		},
		Affected: []string{"requirement:tool:vercel"},
	}
	if err := renderRuntimeModes(cmd, []capabilitypack.RuntimeModeResult{mode}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		"resource=vercel-deploy mode=preview role=primary state=unavailable",
		"on_unavailable=fail_before_effects",
		"fallback=mode fallback_mode=local fallback_state=unverified",
		"Requirement: kind=tool id=vercel version=>=53.0.0",
		"Authority: kind=network scope=vercel_account",
		"Effect: kind=preview_deployment scope=deployment_payload",
		"Requirement evidence: kind=tool id=vercel state=unavailable reason=not_found",
		"Authority evidence: kind=network scope=vercel_account state=unverified reason=observer_error",
		"observer_revision=codex-host-v1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime human output missing %q:\n%s", want, got)
		}
	}
}

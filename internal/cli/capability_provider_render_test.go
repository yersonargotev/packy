package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackLifecycleHumanPreviewRendersProviderFactsFromPlan(t *testing.T) {
	consumer := capabilitypack.ResourceIdentity{Kind: "instruction", ID: "deploy"}
	provider := capabilitypack.ResourceIdentity{Kind: "skill", ID: "vercel"}
	fact := capabilitypack.CapabilityRequirementFact{
		ConsumerPack: "consumer", ConsumerResource: &consumer, Capability: "deployment:vercel",
		ProviderPack: "provider", ProviderResource: &provider,
		RequiredTools: []string{"vercel-cli"}, RequiredAuthority: []string{"network:vercel-project"},
		ResultingReadiness: capabilitypack.ReadinessStatus{Configured: true, Authorized: true},
	}
	var out bytes.Buffer
	if err := renderCapabilityRequirement(&out, fact); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"consumer=consumer/instruction:deploy",
		"capability=deployment:vercel",
		"provider=provider/skill:vercel",
		"tools=vercel-cli",
		"authority=network:vercel-project",
		"readiness=configured:true,authorized:true,usable:false",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("provider fact output %q missing %q", out.String(), want)
		}
	}
}

package agents

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func validateCodexRole(resource capabilitypack.Resource) error {
	for _, binding := range resource.Bindings {
		if binding.Surface != capabilitypack.SurfaceAgents || binding.Projection != "agent" {
			continue
		}
		projection := binding.ClientProjection
		if projection != nil && projection.Client == "codex" && slices.Contains(projection.Scopes, "global") {
			return nil
		}
	}
	return fmt.Errorf("agent %s requires a reviewed codex/global client projection", resource.ID)
}

// renderCodexRole produces only a native role document. Its instructions do not
// establish client authorization, enforce a sandbox, or prove delegation.
func renderCodexRole(resource capabilitypack.Resource, name string, prompt []byte) string {
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	values := func(items []string) string {
		encoded := make([]string, len(items))
		for i, item := range items {
			encoded[i] = quote(item)
		}
		return "[" + strings.Join(encoded, ", ") + "]"
	}
	policy := fmt.Sprintf("Packy agent policy (required): mode=%s; tools=%s; permissions=%s. Preserve these constraints when executing.\n\n", resource.Mode, values(resource.Tools), values(resource.Permissions))
	return fmt.Sprintf("name = %s\ndescription = %s\ndeveloper_instructions = %s\n", quote(name), quote(resource.Description), quote(policy+string(prompt)))
}

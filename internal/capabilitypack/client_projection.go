package capabilitypack

import "fmt"

// validateSelectedClient runs before any surface observation or mutation. A
// required native role cannot be silently dropped from the selected closure.
func validateSelectedClient(pack Pack, selection ResourceSelection, surface Surface, scope string) error {
	if selection.Client != "" && surface != SurfaceAgents {
		return fmt.Errorf("client selection %q is unsupported on %s", selection.Client, surface)
	}
	if selection.Client != "" && scope != "global" {
		return fmt.Errorf("client selection is supported only for reviewed native roles at global scope")
	}
	if selection.Client != "" && !hasSelectedRole(pack, surface) {
		return fmt.Errorf("client selection requires a selected reviewed native role; shared skills do not target a runtime client")
	}
	for _, resource := range pack.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			if surface == SurfaceAgents && resource.Kind == "instruction" && scope != "project" {
				return fmt.Errorf("Agents instructions are project-only; install %s in a project or select only skills and reviewed roles", resource.ID)
			}
			if surface != SurfaceAgents || resource.Kind != "agent" {
				continue
			}
			if err := validateClientProjection(resource.Kind, binding); err != nil {
				return err
			}
			projection := binding.ClientProjection
			if scope != "global" {
				return fmt.Errorf("required role %s targets codex at global scope; project Codex roles are unsupported", resource.ID)
			}
			if selection.Client != projection.Client {
				return fmt.Errorf("required role %s targets client %s at %s scope; explicitly select --client %s", resource.ID, projection.Client, scope, projection.Client)
			}
		}
	}
	return nil
}

func hasSelectedRole(pack Pack, surface Surface) bool {
	if surface != SurfaceAgents {
		return false
	}
	for _, resource := range pack.Resources {
		if resource.Kind == "agent" && resourceHasSurfaceBinding(resource, surface) {
			return true
		}
	}
	return false
}

func cloneClientProjection(projection *ClientProjection) *ClientProjection {
	if projection == nil {
		return nil
	}
	return &ClientProjection{Client: projection.Client, Scopes: append([]string{}, projection.Scopes...)}
}

func validateReceiptClient(receipt installedPackReceipt, scope string) error {
	if receipt.Selection.Client != "" && receipt.Surface != SurfaceAgents {
		return fmt.Errorf("receipt client selection is unsupported on %s", receipt.Surface)
	}
	if receipt.Selection.Client != "" && scope != "global" {
		return fmt.Errorf("project receipt client selection is unsupported")
	}
	roleSelected := false
	for _, resource := range receipt.Resources {
		if receipt.Surface != SurfaceAgents {
			continue
		}
		switch resource.Kind {
		case "agent":
			roleSelected = true
			if scope != "global" || receipt.Selection.Client != "codex" {
				return fmt.Errorf("Agents role receipt requires explicit codex client at global scope")
			}
		case "instruction":
			if scope != "project" {
				return fmt.Errorf("Agents instruction receipt is project-only")
			}
		case "mcp_server", "command", "lifecycle":
			return fmt.Errorf("Agents %s receipt is unsupported", resource.Kind)
		}
	}
	if receipt.Selection.Client != "" && !roleSelected {
		return fmt.Errorf("receipt client selection requires an installed native role")
	}
	return nil
}

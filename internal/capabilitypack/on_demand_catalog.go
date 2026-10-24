package capabilitypack

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// AccessResource retains Pack-local file and version ownership while a procedure
// traverses the effective resources of one complete catalog.
type AccessResource struct {
	Pack     Pack
	Resource Resource
}

// ResolveOnDemand resolves a deterministic procedure, root first, without
// changing the native Requires graph or granting foreign projection ownership.
func ResolveOnDemand(packs []Pack, qualified string, surface Surface) ([]AccessResource, error) {
	return resolveOnDemand(packs, qualified, surface, true)
}

func resolveOnDemand(packs []Pack, qualified string, surface Surface, selectableRoot bool) ([]AccessResource, error) {
	effective := map[string]Pack{}
	for _, pack := range packs {
		effective[pack.ID] = ResolvePackForSurface(pack, surface)
	}
	state := map[string]int{}
	result := []AccessResource{}
	var visit func(string, string) error
	visit = func(packID, identity string) error {
		key := packID + ":" + identity
		if state[key] == 1 {
			return fmt.Errorf("on-demand dependency cycle at %s", key)
		}
		if state[key] == 2 {
			return nil
		}
		pack, ok := effective[packID]
		if !ok || !pack.Selectable && (selectableRoot || packID != strings.Split(qualified, ":")[0]) {
			return fmt.Errorf("missing or non-selectable Pack %s; inspect packy list", packID)
		}
		var resource Resource
		found := false
		for _, candidate := range pack.Resources {
			if candidate.Kind+":"+candidate.ID == identity {
				resource = candidate
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("missing resource %s in Pack %s; inspect packy show %s", identity, packID, packID)
		}
		state[key] = 1
		switch resource.Kind {
		case "skill":
			binding := bindingForSurface(resource, surface)
			if binding == nil {
				return fmt.Errorf("skill %s:%s is excluded or unbound for %s", packID, resource.ID, surface)
			}
			if resource.OnDemand == nil || resource.OnDemand.Protocol != "skill-tree-v1" {
				return fmt.Errorf("skill %s:%s needs a reviewed on_demand skill-tree-v1 contract", packID, resource.ID)
			}
			for _, capability := range binding.Capabilities {
				if (capability.Type == SurfaceCapabilitySkillTemplate || capability.Type == SurfaceCapabilityClaudeCompositeSkill) && resource.OnDemand.AccessEntry() == "SKILL.md" {
					return fmt.Errorf("skill %s:%s requires a reviewed standalone entry_document separate from native rendering or composition", packID, resource.ID)
				}
			}
		case "asset", "notice":
		default:
			return fmt.Errorf("unsupported on-demand dependency %s", key)
		}
		result = append(result, AccessResource{Pack: pack, Resource: resource})
		for _, dependency := range append(slices.Clone(resource.Requires), resource.Notices...) {
			if err := visit(packID, dependency); err != nil {
				return err
			}
		}
		if resource.OnDemand != nil {
			for _, dependency := range resource.OnDemand.Dependencies {
				parts := strings.Split(dependency.Target, ":")
				if dependency.Type != "skill" || len(parts) != 2 || !ValidQualifiedSkill(dependency.Target) {
					return fmt.Errorf("invalid on_demand dependency %q", dependency.Target)
				}
				if err := visit(parts[0], "skill:"+parts[1]); err != nil {
					return err
				}
			}
		}
		state[key] = 2
		return nil
	}
	if !ValidQualifiedSkill(qualified) {
		return nil, fmt.Errorf("invalid qualified skill %q", qualified)
	}
	parts := strings.Split(qualified, ":")
	if err := visit(parts[0], "skill:"+parts[1]); err != nil {
		return nil, err
	}
	return result, nil
}

// ValidateOnDemandCatalog validates all declared procedures and reviewed access
// references as inert data, including declarations in unselected Packs.
func ValidateOnDemandCatalog(packs []Pack) error {
	for _, pack := range packs {
		for _, surface := range []Surface{SurfaceCodex, SurfaceClaude} {
			effective := ResolvePackForSurface(pack, surface)
			for _, resource := range effective.Resources {
				if resource.OnDemand == nil || bindingForSurface(resource, surface) == nil {
					continue
				}
				closure, err := resolveOnDemand(packs, pack.ID+":"+resource.ID, surface, false)
				if err != nil {
					return err
				}
				targets := map[string]string{}
				for _, owned := range closure {
					r := owned.Resource
					if r.Kind == "skill" {
						targets[owned.Pack.ID+":"+r.ID] = path.Join("packs", owned.Pack.ID, r.Source, r.OnDemand.AccessEntry())
					}
				}
				root := resource.CatalogRootOr("")
				entry := filepath.Join(root, resource.Source, resource.OnDemand.AccessEntry())
				if info, err := os.Lstat(entry); err != nil || !info.Mode().IsRegular() {
					return fmt.Errorf("skill %s:%s entry_document %s is missing or not regular", pack.ID, resource.ID, entry)
				}
				for _, relative := range resource.OnDemand.ReferenceFiles {
					file := filepath.Join(root, resource.Source, relative)
					info, err := os.Lstat(file)
					if err != nil || !info.Mode().IsRegular() {
						return fmt.Errorf("skill %s:%s reference file %s is missing or not regular", pack.ID, resource.ID, relative)
					}
					data, err := os.ReadFile(file)
					if err != nil {
						return err
					}
					if _, err := RenderAccessReferences(pack.ID, resource, relative, data, targets); err != nil {
						return fmt.Errorf("skill %s:%s reference file %s: %w", pack.ID, resource.ID, relative, err)
					}
				}
			}
		}
	}
	return nil
}

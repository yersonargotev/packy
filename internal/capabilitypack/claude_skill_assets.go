package capabilitypack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateClaudeSkillAssetClosures qualifies a Pack's effective Claude skill
// layouts. Retained-source integrity is separate: an old snapshot can still
// supply unchanged receipt-owned content without authorizing a new projection.
func ValidateClaudeSkillAssetClosures(pack Pack, fallbackRoot string) error {
	effective := ResolvePackForSurface(pack, SurfaceClaude)
	for _, resource := range effective.Resources {
		if resource.Kind != "skill" || bindingForSurface(resource, SurfaceClaude) == nil {
			continue
		}
		if err := ValidateClaudeSkillAssetClosure(effective, resource, fallbackRoot); err != nil {
			return err
		}
	}
	return nil
}

// ValidateClaudeSkillAssetClosure verifies that an effective Claude skill retains
// its required assets in its native tree. External assets require explicitly
// reviewed composite references; a dependency alone does not choose their paths.
func ValidateClaudeSkillAssetClosure(pack Pack, skill Resource, fallbackRoot string) error {
	capability, composite := skill.SurfaceCapability(SurfaceClaude, SurfaceCapabilityClaudeCompositeSkill)
	if composite && skill.RequestsSurfaceCapability(SurfaceClaude, SurfaceCapabilitySkillTemplate) {
		return fmt.Errorf("Claude skill %s cannot combine skill-template and claude-composite-skill", skill.ID)
	}
	closure, _, err := resolveResourceClosure(withSurfaceCapabilityDependencies(pack, SurfaceClaude), []ResourceIdentity{identityFromResource(skill)})
	if err != nil {
		return err
	}
	source, err := filepath.EvalSymlinks(filepath.Join(skill.CatalogRootOr(fallbackRoot), skill.Source))
	if err != nil {
		return err
	}
	references := map[ResourceIdentity]bool{}
	if composite {
		for _, reference := range capability.ClaudeCompositeSkill.References {
			references[reference] = true
		}
	}
	destinations := map[string]string{}
	for _, asset := range closure {
		if asset.Kind != "asset" {
			continue
		}
		path, err := filepath.EvalSymlinks(filepath.Join(asset.CatalogRootOr(fallbackRoot), asset.Source))
		if err != nil {
			return err
		}
		if references[identityFromResource(asset)] {
			destination := filepath.Join("references", filepath.Base(asset.Source))
			if prior, exists := destinations[destination]; exists {
				return fmt.Errorf("Claude skill %s reference destination %q collides between assets %s and %s", skill.ID, destination, prior, asset.ID)
			}
			destinations[destination] = asset.ID
			if _, err := os.Lstat(filepath.Join(source, destination)); !os.IsNotExist(err) {
				if err != nil {
					return fmt.Errorf("Claude skill %s reference destination %q: %w", skill.ID, destination, err)
				}
				return fmt.Errorf("Claude skill %s reference destination %q already exists in its source tree", skill.ID, destination)
			}
			continue
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if composite {
				return fmt.Errorf("Claude skill %s must declare external dependency asset %s in claude-composite-skill references", skill.ID, asset.ID)
			}
			return fmt.Errorf("Claude skill %s dependency asset %s is outside its complete source tree; declare reviewed claude-composite-skill references", skill.ID, asset.ID)
		}
	}
	return nil
}

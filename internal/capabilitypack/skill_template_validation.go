package capabilitypack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/yersonargotev/packy/internal/skilltemplate"
)

func validateSkillTemplateSources(pack Pack, fallbackRoot string) error {
	for _, surface := range pack.Surfaces {
		effective := ResolvePackForSurface(pack, surface)
		names := SkillTemplateNames(effective, surface)
		for _, resource := range effective.Resources {
			if resource.Source == "" || bindingForSurface(resource, surface) == nil {
				continue
			}
			root := resource.CatalogRootOr(fallbackRoot)
			references, regularTree, err := referencedSkillsInResourceSource(root, resource)
			if err != nil {
				return fmt.Errorf("surface %q resource %q skill-template source: %w", surface, resource.Kind+":"+resource.ID, err)
			}
			capability, optedIn := resource.SurfaceCapability(surface, SurfaceCapabilitySkillTemplate)
			if !optedIn {
				if len(references) != 0 {
					return fmt.Errorf("surface %q resource %q contains a skill template directive without the %q capability", surface, resource.Kind+":"+resource.ID, SurfaceCapabilitySkillTemplate)
				}
				continue
			}
			if capability.SkillTemplate == nil {
				// Structural validation reports this first. Keep this function safe
				// for direct callers and partially assembled test values.
				return fmt.Errorf("surface %q resource %q has no skill_template capability data", surface, resource.Kind+":"+resource.ID)
			}
			for _, target := range references {
				if _, ok := names[target]; !ok {
					return fmt.Errorf("surface %q resource %q references absent, excluded, or unbound skill %q", surface, resource.Kind+":"+resource.ID, target)
				}
				if target != resource.ID && !containsString(resource.Requires, "skill:"+target) {
					return fmt.Errorf("surface %q resource %q references skill %q without a direct requires dependency", surface, resource.Kind+":"+resource.ID, target)
				}
			}
			if regularTree {
				source := filepath.Join(root, filepath.Clean(resource.Source))
				if _, err := skilltemplate.RenderTree(source, resource.ID, names); err != nil {
					return fmt.Errorf("surface %q resource %q skill-template render: %w", surface, resource.Kind+":"+resource.ID, err)
				}
			}
		}
	}
	return nil
}

func bindingForSurface(resource Resource, surface Surface) *Binding {
	for i := range resource.Bindings {
		if resource.Bindings[i].Surface == surface {
			return &resource.Bindings[i]
		}
	}
	return nil
}

func referencedSkillsInResourceSource(root string, resource Resource) ([]string, bool, error) {
	source := filepath.Join(root, filepath.Clean(resource.Source))
	seen := map[string]bool{}
	regularTree := true
	read := func(path, relative string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", relative, err)
		}
		references, err := skilltemplate.ReferencedSkills(data)
		if err != nil {
			return fmt.Errorf("file %q: %w", relative, err)
		}
		for _, reference := range references {
			seen[reference] = true
		}
		return nil
	}
	if resource.Kind != "skill" {
		if err := read(source, filepath.ToSlash(resource.Source)); err != nil {
			return nil, false, err
		}
	} else {
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == source || entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if !info.Mode().IsRegular() {
				// Declared Pack Closure validation owns special-file and symlink
				// diagnostics. Template validation covers only regular source
				// bytes so it does not mask those more precise errors.
				regularTree = false
				return nil
			}
			return read(path, relative)
		})
		if err != nil {
			return nil, false, err
		}
	}
	result := make([]string, 0, len(seen))
	for reference := range seen {
		result = append(result, reference)
	}
	sort.Strings(result)
	return result, regularTree, nil
}

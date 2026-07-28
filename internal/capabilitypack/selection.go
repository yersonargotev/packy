package capabilitypack

import (
	"fmt"
	"sort"
	"strings"
)

type SelectionMode string

const (
	SelectionAll    SelectionMode = "all"
	SelectionCustom SelectionMode = "custom"
)

type ResourceIdentity struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (r ResourceIdentity) String() string { return r.Kind + ":" + r.ID }

func ParseResourceIdentity(value string) (ResourceIdentity, error) {
	parts := strings.Split(value, ":")
	kinds := map[string]bool{"skill": true, "instruction": true, "mcp_server": true, "agent": true, "command": true, "lifecycle": true, "asset": true, "notice": true}
	if len(parts) != 2 || !kinds[parts[0]] || !idPattern.MatchString(parts[1]) {
		return ResourceIdentity{}, fmt.Errorf("resource identity %q must be canonical kind:id", value)
	}
	identity := ResourceIdentity{Kind: parts[0], ID: parts[1]}
	if identity.String() != value {
		return ResourceIdentity{}, fmt.Errorf("resource identity %q must be canonical kind:id", value)
	}
	return identity, nil
}

type ResourceSelection struct {
	Mode  SelectionMode      `json:"mode"`
	Roots []ResourceIdentity `json:"roots"`
}

func cloneSelection(selection ResourceSelection) ResourceSelection {
	if selection.Roots != nil {
		selection.Roots = append([]ResourceIdentity{}, selection.Roots...)
	}
	return selection
}

func canonicalSelection(selection ResourceSelection) (ResourceSelection, error) {
	if selection.Mode == "" && len(selection.Roots) == 0 {
		return ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, nil
	}
	if selection.Mode != SelectionAll && selection.Mode != SelectionCustom {
		return ResourceSelection{}, fmt.Errorf("resource selection mode %q is unsupported", selection.Mode)
	}
	if selection.Mode == SelectionAll {
		if len(selection.Roots) != 0 {
			return ResourceSelection{}, fmt.Errorf("resource selection mode all forbids roots")
		}
		return ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, nil
	}
	roots := map[string]ResourceIdentity{}
	for _, candidate := range selection.Roots {
		root, err := ParseResourceIdentity(candidate.String())
		if err != nil {
			return ResourceSelection{}, err
		}
		roots[root.String()] = root
	}
	if len(roots) != 1 {
		return ResourceSelection{}, fmt.Errorf("custom resource selection requires exactly one distinct root")
	}
	var root ResourceIdentity
	for _, candidate := range roots {
		root = candidate
	}
	return ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{root}}, nil
}

func selectPackResources(pack Pack, selection ResourceSelection) (Pack, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return Pack{}, err
	}
	if selection.Mode == SelectionAll {
		return clonePack(pack), nil
	}
	if pack.manifestVersion != manifestSchemaV4 {
		return Pack{}, fmt.Errorf("custom resource selection requires manifest schema_version 4")
	}
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	closure := map[string]bool{}
	var visit func(string) error
	visit = func(identity string) error {
		resource, ok := resources[identity]
		if !ok {
			return fmt.Errorf("custom resource selection root or dependency %q does not exist in pack %q", identity, pack.ID)
		}
		if closure[identity] {
			return nil
		}
		closure[identity] = true
		for _, dependency := range resource.Requires {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range selection.Roots {
		resource, ok := resources[root.String()]
		if !ok {
			return Pack{}, fmt.Errorf("custom resource selection root %q does not exist in pack %q", root.String(), pack.ID)
		}
		if resource.Kind == "asset" || resource.Kind == "notice" {
			return Pack{}, fmt.Errorf("custom resource selection root %q is not operational", root.String())
		}
		if err := visit(root.String()); err != nil {
			return Pack{}, err
		}
	}
	ordered := make([]Resource, 0, len(closure))
	emitted := map[string]bool{}
	for len(emitted) < len(closure) {
		ready := make([]string, 0)
		for identity := range closure {
			if emitted[identity] {
				continue
			}
			resource := resources[identity]
			ok := true
			for _, dependency := range resource.Requires {
				if closure[dependency] && !emitted[dependency] {
					ok = false
					break
				}
			}
			if ok {
				ready = append(ready, identity)
			}
		}
		sort.Strings(ready)
		if len(ready) == 0 {
			return Pack{}, fmt.Errorf("custom resource selection dependency cycle")
		}
		identity := ready[0]
		ordered = append(ordered, resources[identity])
		emitted[identity] = true
	}
	noticeSet := map[string]bool{}
	for identity := range closure {
		for _, notice := range resources[identity].Notices {
			noticeSet[notice] = true
		}
	}
	noticeIDs := make([]string, 0, len(noticeSet))
	for identity := range noticeSet {
		noticeIDs = append(noticeIDs, identity)
	}
	sort.Strings(noticeIDs)
	for _, identity := range noticeIDs {
		ordered = append(ordered, resources[identity])
	}
	selected := clonePack(pack)
	selected.Resources = ordered
	return selected, nil
}

func resourceSelectionFacts(pack Pack, selection ResourceSelection, active bool) []ResourceSelectionStatus {
	selection, _ = canonicalSelection(selection)
	selected := map[string]bool{}
	if selection.Mode == SelectionCustom {
		selected[selection.Roots[0].String()] = true
	}
	result := make([]ResourceSelectionStatus, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		result = append(result, ResourceSelectionStatus{Resource: identity, Selected: active && (selection.Mode == SelectionAll || selected[identity.String()])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

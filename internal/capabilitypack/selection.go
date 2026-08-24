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
	if len(roots) == 0 {
		return ResourceSelection{}, fmt.Errorf("custom resource selection requires at least one distinct root")
	}
	identities := make([]string, 0, len(roots))
	for identity := range roots {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	canonicalRoots := make([]ResourceIdentity, 0, len(identities))
	for _, identity := range identities {
		canonicalRoots = append(canonicalRoots, roots[identity])
	}
	return ResourceSelection{Mode: SelectionCustom, Roots: canonicalRoots}, nil
}

func mergeCustomSelections(current, requested ResourceSelection) (ResourceSelection, error) {
	current, err := canonicalSelection(current)
	if err != nil {
		return ResourceSelection{}, err
	}
	requested, err = canonicalSelection(requested)
	if err != nil {
		return ResourceSelection{}, err
	}
	if current.Mode == SelectionAll {
		return current, nil
	}
	roots := append([]ResourceIdentity{}, current.Roots...)
	roots = append(roots, requested.Roots...)
	return canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: roots})
}

func removeResourceSelectionRoots(pack Pack, selection ResourceSelection, removals []ResourceIdentity) (ResourceSelection, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return ResourceSelection{}, err
	}
	if len(removals) == 0 {
		return ResourceSelection{}, fmt.Errorf("resource-scoped deactivation requires at least one resource")
	}
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	requested := make(map[string]ResourceIdentity, len(removals))
	for _, candidate := range removals {
		identity, parseErr := ParseResourceIdentity(candidate.String())
		if parseErr != nil {
			return ResourceSelection{}, parseErr
		}
		resource, ok := resources[identity.String()]
		if !ok {
			return ResourceSelection{}, fmt.Errorf("resource %q does not exist in pack %q", identity.String(), pack.ID)
		}
		if resource.Kind == "asset" || resource.Kind == "notice" {
			return ResourceSelection{}, fmt.Errorf("resource %q is not an operational selection root", identity.String())
		}
		requested[identity.String()] = identity
	}

	roots, err := resourceSelectionRoots(pack, selection)
	if err != nil {
		return ResourceSelection{}, err
	}
	rootSet := make(map[string]bool, len(roots))
	for _, root := range roots {
		rootSet[root.String()] = true
	}
	if selection.Mode == SelectionCustom {
		_, chains, closureErr := resolveResourceClosure(pack, roots)
		if closureErr != nil {
			return ResourceSelection{}, closureErr
		}
		identities := make([]string, 0, len(requested))
		for identity := range requested {
			identities = append(identities, identity)
		}
		sort.Strings(identities)
		for _, identity := range identities {
			if rootSet[identity] {
				continue
			}
			chain, selected := chains[identity]
			if !selected {
				return ResourceSelection{}, fmt.Errorf("resource %q is not selected by pack %q", identity, pack.ID)
			}
			consumers := []string{}
			for _, root := range roots {
				_, rootChains, rootErr := resolveResourceClosure(pack, []ResourceIdentity{root})
				if rootErr != nil {
					return ResourceSelection{}, rootErr
				}
				rootChain, rootSelected := rootChains[identity]
				if rootSelected && len(rootChain) > 1 {
					consumers = append(consumers, root.String())
				}
			}
			sort.Strings(consumers)
			if len(consumers) == 0 {
				consumers = append(consumers, chain[0].String())
			}
			return ResourceSelection{}, fmt.Errorf("resource %q is dependency-only; remove consuming root(s) %s instead", identity, strings.Join(consumers, ", "))
		}
	}
	remaining := make([]ResourceIdentity, 0, len(roots))
	for _, root := range roots {
		if _, removed := requested[root.String()]; !removed {
			remaining = append(remaining, root)
		}
	}
	sort.Slice(remaining, func(i, j int) bool { return remaining[i].String() < remaining[j].String() })
	return ResourceSelection{Mode: SelectionCustom, Roots: remaining}, nil
}

func selectPackResources(pack Pack, selection ResourceSelection) (Pack, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return Pack{}, err
	}
	return selectPackResourceClosure(pack, selection)
}

func selectPackResourceClosure(pack Pack, selection ResourceSelection) (Pack, error) {
	roots, err := resourceSelectionRoots(pack, selection)
	if err != nil {
		return Pack{}, err
	}
	resources, _, err := resolveResourceClosure(pack, roots)
	if err != nil {
		return Pack{}, err
	}
	selected := clonePack(pack)
	selected.Resources = resources
	return selected, nil
}

func resourceSelectionRoots(pack Pack, selection ResourceSelection) ([]ResourceIdentity, error) {
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	if selection.Mode == SelectionAll {
		roots := make([]ResourceIdentity, 0, len(pack.Resources))
		for _, resource := range pack.Resources {
			if resource.Kind != "asset" && resource.Kind != "notice" {
				roots = append(roots, ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
			}
		}
		sort.Slice(roots, func(i, j int) bool { return roots[i].String() < roots[j].String() })
		return roots, nil
	}
	roots := make([]ResourceIdentity, 0, len(selection.Roots))
	for _, root := range selection.Roots {
		resource, ok := resources[root.String()]
		if !ok {
			return nil, fmt.Errorf("custom resource selection root %q does not exist in pack %q", root.String(), pack.ID)
		}
		if resource.Kind == "asset" || resource.Kind == "notice" {
			return nil, fmt.Errorf("custom resource selection root %q is not operational", root.String())
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func resolveResourceClosure(pack Pack, roots []ResourceIdentity) ([]Resource, map[string][]ResourceIdentity, error) {
	resources := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[(ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()] = resource
	}
	chains := map[string][]ResourceIdentity{}
	rootIDs := map[string]bool{}
	for _, root := range roots {
		rootIDs[root.String()] = true
	}
	var visit func(string, []ResourceIdentity) error
	visit = func(identity string, chain []ResourceIdentity) error {
		resource, ok := resources[identity]
		if !ok {
			return fmt.Errorf("custom resource selection root or dependency %q does not exist in pack %q", identity, pack.ID)
		}
		if rootIDs[identity] && len(chain) > 0 {
			return nil
		}
		candidate := append(append([]ResourceIdentity{}, chain...), ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
		if prior, exists := chains[identity]; exists && !identityChainLess(candidate, prior) {
			return nil
		}
		chains[identity] = candidate
		for _, dependency := range resource.Requires {
			if err := visit(dependency, candidate); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := visit(root.String(), nil); err != nil {
			return nil, nil, err
		}
	}
	ordered := make([]Resource, 0, len(chains))
	emitted := map[string]bool{}
	for len(emitted) < len(chains) {
		ready := make([]string, 0)
		for identity := range chains {
			if emitted[identity] {
				continue
			}
			resource := resources[identity]
			ok := true
			for _, dependency := range resource.Requires {
				if _, selected := chains[dependency]; selected && !emitted[dependency] {
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
			return nil, nil, fmt.Errorf("custom resource selection dependency cycle")
		}
		identity := ready[0]
		ordered = append(ordered, resources[identity])
		emitted[identity] = true
	}
	noticeChains := map[string][]ResourceIdentity{}
	for identity, chain := range chains {
		for _, notice := range resources[identity].Notices {
			resource, ok := resources[notice]
			if !ok {
				return nil, nil, fmt.Errorf("resource %q notice %q does not exist in pack %q", identity, notice, pack.ID)
			}
			candidate := append(append([]ResourceIdentity{}, chain...), ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
			if prior, exists := noticeChains[notice]; !exists || identityChainLess(candidate, prior) {
				noticeChains[notice] = candidate
			}
		}
	}
	noticeIDs := make([]string, 0, len(noticeChains))
	for identity := range noticeChains {
		noticeIDs = append(noticeIDs, identity)
	}
	sort.Strings(noticeIDs)
	for _, identity := range noticeIDs {
		ordered = append(ordered, resources[identity])
		chains[identity] = noticeChains[identity]
	}
	return ordered, chains, nil
}

func resourceSelectionFacts(pack Pack, selection ResourceSelection, surface Surface, active bool) []ResourceSelectionStatus {
	pack = withSurfaceCapabilityDependencies(pack, surface)
	selection, _ = canonicalSelection(selection)
	selected := map[string]bool{}
	if active {
		if roots, err := resourceSelectionRoots(pack, selection); err == nil {
			if _, chains, err := resolveResourceClosure(pack, roots); err == nil {
				for identity := range chains {
					selected[identity] = true
				}
			}
		}
	}
	result := make([]ResourceSelectionStatus, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		identity := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		result = append(result, ResourceSelectionStatus{Resource: identity, Selected: selected[identity.String()]})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

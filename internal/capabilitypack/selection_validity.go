package capabilitypack

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type SelectionReasonCode string

const (
	SelectionReasonRootExcluded          SelectionReasonCode = "root-excluded"
	SelectionReasonDependencyUnavailable SelectionReasonCode = "dependency-unavailable"
	SelectionReasonMandatoryIncomplete   SelectionReasonCode = "mandatory-incomplete"
	SelectionReasonResourceConflict      SelectionReasonCode = "resource-conflict"
	SelectionReasonOptionalExclusion     SelectionReasonCode = "optional-exclusion"
)

type SelectionValidityReason struct {
	Code                       SelectionReasonCode `json:"code"`
	Resource                   ResourceIdentity    `json:"resource"`
	Role                       ResourceRole        `json:"role"`
	DependencyChain            []ResourceIdentity  `json:"dependency_chain"`
	ConflictingResource        string              `json:"conflicting_resource,omitempty"`
	ConflictingRole            ResourceRole        `json:"conflicting_role,omitempty"`
	ConflictingDependencyChain []ResourceIdentity  `json:"conflicting_dependency_chain"`
	Surface                    Surface             `json:"surface"`
	Detail                     string              `json:"detail"`
	Remediation                string              `json:"remediation"`
}

type SelectionAvailability struct {
	Available          bool                      `json:"available"`
	Reasons            []SelectionValidityReason `json:"reasons"`
	OptionalExclusions []SelectionValidityReason `json:"optional_exclusions"`
}

type ResourceSelectability struct {
	Resource  ResourceIdentity          `json:"resource"`
	Available bool                      `json:"available"`
	Reasons   []SelectionValidityReason `json:"reasons"`
}

type SelectionValidity struct {
	All   SelectionAvailability   `json:"all"`
	Roots []ResourceSelectability `json:"roots"`
}

func (availability SelectionAvailability) MarshalJSON() ([]byte, error) {
	reasons := availability.Reasons
	if reasons == nil {
		reasons = []SelectionValidityReason{}
	}
	exclusions := availability.OptionalExclusions
	if exclusions == nil {
		exclusions = []SelectionValidityReason{}
	}
	return json.Marshal(struct {
		Available          bool                      `json:"available"`
		Reasons            []SelectionValidityReason `json:"reasons"`
		OptionalExclusions []SelectionValidityReason `json:"optional_exclusions"`
	}{availability.Available, reasons, exclusions})
}

func (root ResourceSelectability) MarshalJSON() ([]byte, error) {
	reasons := root.Reasons
	if reasons == nil {
		reasons = []SelectionValidityReason{}
	}
	return json.Marshal(struct {
		Resource  ResourceIdentity          `json:"resource"`
		Available bool                      `json:"available"`
		Reasons   []SelectionValidityReason `json:"reasons"`
	}{root.Resource, root.Available, reasons})
}

func (validity SelectionValidity) MarshalJSON() ([]byte, error) {
	roots := validity.Roots
	if roots == nil {
		roots = []ResourceSelectability{}
	}
	return json.Marshal(struct {
		All   SelectionAvailability   `json:"all"`
		Roots []ResourceSelectability `json:"roots"`
	}{validity.All, roots})
}

func SelectionValidityFor(pack Pack, surface Surface) SelectionValidity {
	all, _ := selectionAvailability(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, surface)
	result := SelectionValidity{All: all, Roots: []ResourceSelectability{}}
	if pack.manifestVersion != manifestSchemaV4 {
		return result
	}
	for _, resource := range pack.Resources {
		if resource.Kind == "asset" || resource.Kind == "notice" {
			continue
		}
		root := ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
		availability, err := selectionAvailability(pack, ResourceSelection{
			Mode: SelectionCustom, Roots: []ResourceIdentity{root},
		}, surface)
		if err != nil {
			availability = unavailableSelection(SelectionValidityReason{
				Code: SelectionReasonMandatoryIncomplete, Resource: root, Role: ResourceRoleRoot,
				DependencyChain: []ResourceIdentity{root}, ConflictingDependencyChain: []ResourceIdentity{},
				Surface: surface, Detail: err.Error(), Remediation: "choose a root whose complete mandatory closure is valid",
			})
		}
		result.Roots = append(result.Roots, ResourceSelectability{
			Resource: root, Available: availability.Available, Reasons: availability.Reasons,
		})
	}
	sort.Slice(result.Roots, func(i, j int) bool {
		return result.Roots[i].Resource.String() < result.Roots[j].Resource.String()
	})
	return result
}

func selectionAvailability(pack Pack, selection ResourceSelection, surface Surface) (SelectionAvailability, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return SelectionAvailability{}, err
	}
	if pack.manifestVersion != manifestSchemaV4 {
		return SelectionAvailability{Available: true, Reasons: []SelectionValidityReason{}, OptionalExclusions: []SelectionValidityReason{}}, nil
	}
	roots, optional, err := surfaceSelectionRoots(pack, selection, surface)
	if err != nil {
		return SelectionAvailability{}, err
	}
	_, chains, err := resolveResourceClosure(pack, roots)
	if err != nil {
		return SelectionAvailability{}, err
	}
	resources := resourceMap(pack)
	reasons := []SelectionValidityReason{}
	identities := make([]string, 0, len(chains))
	for identity := range chains {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		resource := resources[identity]
		if resource.Kind == "asset" || resource.Kind == "notice" {
			continue
		}
		chain := cloneIdentityChain(chains[identity])
		exclusion, excluded := exclusionFor(resource, surface)
		hasBinding := resourceHasSurfaceBinding(resource, surface)
		if !excluded && hasBinding {
			continue
		}
		code := SelectionReasonDependencyUnavailable
		detail := fmt.Sprintf("mandatory dependency %s via %s is unavailable on %s", identity, renderSelectionChain(chain), surface)
		remediation := fmt.Sprintf("choose a root whose mandatory closure is available on %s", surface)
		if len(chain) == 1 {
			code = SelectionReasonRootExcluded
			detail = fmt.Sprintf("requested root %s is excluded on %s", identity, surface)
			remediation = fmt.Sprintf("choose a root available on %s", surface)
		}
		if !excluded {
			code = SelectionReasonMandatoryIncomplete
			detail = fmt.Sprintf("resource %s declares no outcome on %s", identity, surface)
		} else if exclusion.Code != "" {
			detail += ": " + exclusion.Code + " — " + exclusion.Reason
		}
		reasons = append(reasons, SelectionValidityReason{
			Code: code, Resource: identityFromResource(resource), Role: roleForChain(resource, chain),
			DependencyChain: chain, ConflictingDependencyChain: []ResourceIdentity{}, Surface: surface,
			Detail: detail, Remediation: remediation,
		})
	}
	reasons = append(reasons, conflictReasons(resources, chains, surface)...)
	sortSelectionReasons(reasons)
	sortSelectionReasons(optional)
	return SelectionAvailability{
		Available: len(reasons) == 0, Reasons: reasons, OptionalExclusions: optional,
	}, nil
}

func unavailableSelection(reason SelectionValidityReason) SelectionAvailability {
	return SelectionAvailability{
		Available: false, Reasons: []SelectionValidityReason{reason}, OptionalExclusions: []SelectionValidityReason{},
	}
}

func surfaceSelectionRoots(pack Pack, selection ResourceSelection, surface Surface) ([]ResourceIdentity, []SelectionValidityReason, error) {
	roots, err := resourceSelectionRoots(pack, selection)
	if err != nil {
		return nil, nil, err
	}
	optional := []SelectionValidityReason{}
	if selection.Mode != SelectionAll || pack.manifestVersion != manifestSchemaV4 {
		return roots, optional, nil
	}
	resources := resourceMap(pack)
	included := make([]ResourceIdentity, 0, len(roots))
	for _, root := range roots {
		resource := resources[root.String()]
		exclusion, excluded := exclusionFor(resource, surface)
		if excluded && exclusion.Mode == "optional" {
			optional = append(optional, SelectionValidityReason{
				Code: SelectionReasonOptionalExclusion, Resource: root, Role: ResourceRoleRoot,
				DependencyChain: []ResourceIdentity{root}, ConflictingDependencyChain: []ResourceIdentity{},
				Surface:     surface,
				Detail:      fmt.Sprintf("optional root %s is excluded on %s: %s — %s", root, surface, exclusion.Code, exclusion.Reason),
				Remediation: fmt.Sprintf("use custom selection on a surface where %s is available", root),
			})
			continue
		}
		included = append(included, root)
	}
	return included, optional, nil
}

func selectPackResourcesForSurface(pack Pack, selection ResourceSelection, surface Surface) (Pack, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return Pack{}, err
	}
	if selection.Mode != SelectionAll || pack.manifestVersion != manifestSchemaV4 {
		return selectPackResources(pack, selection)
	}
	roots, _, err := surfaceSelectionRoots(pack, selection, surface)
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

func selectionBlockers(pack Pack, selection ResourceSelection, surface Surface) ([]PlanBlocker, SelectionValidity, error) {
	availability, err := selectionAvailability(pack, selection, surface)
	if err != nil {
		return nil, SelectionValidity{}, err
	}
	validity := SelectionValidityFor(pack, surface)
	if selection.Mode == SelectionAll {
		validity.All = availability
	}
	blockers := make([]PlanBlocker, 0, len(availability.Reasons))
	for _, reason := range availability.Reasons {
		kind := BlockerSelectionUnavailable
		subject := reason.Resource.String()
		if reason.Code == SelectionReasonResourceConflict {
			kind = BlockerResourceConflict
			subject = reason.Resource.String() + "," + reason.ConflictingResource
		}
		blockers = append(blockers, PlanBlocker{Kind: kind, Subject: subject, Detail: reason.Detail + "; remediation: " + reason.Remediation})
	}
	sortBlockers(blockers)
	return blockers, validity, nil
}

func conflictReasons(resources map[string]Resource, chains map[string][]ResourceIdentity, surface Surface) []SelectionValidityReason {
	reasons := []SelectionValidityReason{}
	identities := make([]string, 0, len(chains))
	for identity := range chains {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		resource := resources[identity]
		for _, conflict := range resource.Conflicts {
			if identity >= conflict {
				continue
			}
			otherChain, selected := chains[conflict]
			if !selected {
				continue
			}
			leftChain, rightChain := cloneIdentityChain(chains[identity]), cloneIdentityChain(otherChain)
			leftRole, rightRole := roleForChain(resource, leftChain), roleForChain(resources[conflict], rightChain)
			leftRoot, rightRoot := leftChain[0].String(), rightChain[0].String()
			reasons = append(reasons, SelectionValidityReason{
				Code: SelectionReasonResourceConflict, Resource: identityFromResource(resource), Role: leftRole,
				DependencyChain: leftChain, ConflictingResource: conflict, ConflictingRole: rightRole,
				ConflictingDependencyChain: rightChain, Surface: surface,
				Detail: fmt.Sprintf(
					"resources %s (%s via %s) and %s (%s via %s) conflict",
					identity, leftRole, renderSelectionChain(leftChain), conflict, rightRole, renderSelectionChain(rightChain),
				),
				Remediation: fmt.Sprintf("remove one explicit root: %s or %s", leftRoot, rightRoot),
			})
		}
	}
	return reasons
}

func resourceMap(pack Pack) map[string]Resource {
	result := make(map[string]Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		result[resource.Kind+":"+resource.ID] = resource
	}
	return result
}

func resourceHasSurfaceBinding(resource Resource, surface Surface) bool {
	for _, binding := range resource.Bindings {
		if binding.Surface == surface {
			return true
		}
	}
	return false
}

func exclusionFor(resource Resource, surface Surface) (SurfaceExclusion, bool) {
	for _, exclusion := range resource.SurfaceExclusions {
		if exclusion.Surface == surface {
			return exclusion, true
		}
	}
	return SurfaceExclusion{}, false
}

func identityFromResource(resource Resource) ResourceIdentity {
	return ResourceIdentity{Kind: resource.Kind, ID: resource.ID}
}

func roleForChain(resource Resource, chain []ResourceIdentity) ResourceRole {
	switch {
	case resource.Kind == "asset":
		return ResourceRoleAsset
	case resource.Kind == "notice":
		return ResourceRoleNotice
	case len(chain) == 1:
		return ResourceRoleRoot
	default:
		return ResourceRoleDependency
	}
}

func cloneIdentityChain(chain []ResourceIdentity) []ResourceIdentity {
	return append([]ResourceIdentity{}, chain...)
}

func renderSelectionChain(chain []ResourceIdentity) string {
	values := make([]string, 0, len(chain))
	for _, identity := range chain {
		values = append(values, identity.String())
	}
	return strings.Join(values, " -> ")
}

func sortSelectionReasons(reasons []SelectionValidityReason) {
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].Code != reasons[j].Code {
			return reasons[i].Code < reasons[j].Code
		}
		if reasons[i].Resource.String() != reasons[j].Resource.String() {
			return reasons[i].Resource.String() < reasons[j].Resource.String()
		}
		return reasons[i].ConflictingResource < reasons[j].ConflictingResource
	})
	for i := range reasons {
		if reasons[i].DependencyChain == nil {
			reasons[i].DependencyChain = []ResourceIdentity{}
		}
		if reasons[i].ConflictingDependencyChain == nil {
			reasons[i].ConflictingDependencyChain = []ResourceIdentity{}
		}
	}
}

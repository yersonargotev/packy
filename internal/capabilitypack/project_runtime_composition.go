package capabilitypack

import (
	"context"
	"fmt"
	"sort"
)

type projectRuntimeComposition struct {
	effects        []ProjectRuntimeEffectStatus
	conflict       bool
	conflictDetail string
}

func (f Facade) composeProjectRuntime(ctx context.Context, installation ProjectInstallation, surface Surface) (projectRuntimeComposition, error) {
	pack := installation.Manifest.Packs[0]
	categories := projectActivationCategories(installation.Lock, surface)
	result := projectRuntimeComposition{effects: pendingProjectRuntimeEffects(categories)}
	if len(result.effects) == 0 || f.activation == nil || f.activation.store == nil {
		return result, nil
	}
	state, err := loadActivationState(ctx, f.activation.store, surface)
	if err != nil {
		return result, fmt.Errorf("load global activation for project runtime composition: %w", err)
	}
	intent, active := intentForPack(state, pack.ID, surface)
	if !active || !intent.Active {
		return result, nil
	}
	globalPack, err := f.catalog.ResolveIntentPack(intent.PackID, intent.Version)
	if err != nil {
		return result, fmt.Errorf("resolve global activation contract %s@%s: %w", intent.PackID, intent.Version, err)
	}
	globalSelected, err := selectPackResources(globalPack, intent.Selection)
	if err != nil {
		return result, fmt.Errorf("resolve global activation resource selection: %w", err)
	}
	selected := make(map[ResourceIdentity]bool, len(globalSelected.Resources))
	for _, resource := range globalSelected.Resources {
		selected[ResourceIdentity{Kind: resource.Kind, ID: resource.ID}] = true
	}
	projectBindings := projectRuntimeBindings(installation.Lock.Bindings, surface)
	globalContract := LifecycleContractFor(globalSelected, surface, intent.Aliases)
	globalBindings := globalContract.Bindings
	for i := range globalBindings {
		globalBindings[i].Surface = surface
	}
	globalBindingMap := projectRuntimeBindings(globalBindings, surface)
	globalDetails := projectRuntimeDisclosureSet(globalSelected, surface, globalBindings)
	conflicts := make([]string, 0)
	for i := range result.effects {
		effect := &result.effects[i]
		if !selected[effect.Resource] {
			continue
		}
		effect.GlobalVersion = intent.Version
		if intent.Version != pack.Version {
			effect.Coverage = ProjectRuntimeCoverageConflict
			effect.Conflict = fmt.Sprintf("global %s@%s and project %s@%s select %s with different exact contracts", intent.PackID, intent.Version, pack.ID, pack.Version, effect.Resource)
			conflicts = append(conflicts, effect.Conflict)
			continue
		}
		if digestJSON(projectBindings[effect.Resource]) != digestJSON(globalBindingMap[effect.Resource]) {
			effect.Coverage = ProjectRuntimeCoverageConflict
			effect.Conflict = fmt.Sprintf("global and project bindings or aliases differ for %s", effect.Resource)
			conflicts = append(conflicts, effect.Conflict)
			continue
		}
		key := projectRuntimeDisclosureKey(effect.Category, effect.Resource, effect.Detail)
		if !globalDetails[key] {
			effect.Coverage = ProjectRuntimeCoverageConflict
			effect.Conflict = fmt.Sprintf("global and project sensitive definitions differ for %s", effect.Resource)
			conflicts = append(conflicts, effect.Conflict)
			continue
		}
		effect.Coverage = ProjectRuntimeCoverageInheritedGlobal
	}
	for _, effect := range result.effects {
		result.conflict = result.conflict || effect.Coverage == ProjectRuntimeCoverageConflict
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		result.conflictDetail = stringsJoinUnique(conflicts, "; ")
	}
	return result, nil
}

func projectRuntimeBindings(bindings []LifecycleBinding, surface Surface) map[ResourceIdentity][]LifecycleBinding {
	result := map[ResourceIdentity][]LifecycleBinding{}
	for _, binding := range bindings {
		if binding.Surface != "" && binding.Surface != surface {
			continue
		}
		binding.Surface = surface
		identity := ResourceIdentity{Kind: binding.Kind, ID: binding.ID}
		result[identity] = append(result[identity], binding)
	}
	for identity := range result {
		sort.Slice(result[identity], func(i, j int) bool { return digestJSON(result[identity][i]) < digestJSON(result[identity][j]) })
	}
	return result
}

func projectRuntimeDisclosureSet(pack Pack, surface Surface, bindings []LifecycleBinding) map[string]bool {
	lock := ProjectLockProposal{Bindings: bindings, Sensitive: projectSensitiveDisclosures(pack, surface)}
	result := map[string]bool{}
	for _, category := range projectActivationCategories(lock, surface) {
		for _, detail := range category.Details {
			result[projectRuntimeDisclosureKey(category.Kind, detail.Resource, detail.Detail)] = true
		}
	}
	return result
}

func projectRuntimeDisclosureKey(category ProjectActivationCategory, resource ResourceIdentity, detail string) string {
	return string(category) + "\x00" + resource.String() + "\x00" + detail
}

func stringsJoinUnique(values []string, separator string) string {
	result := ""
	for i, value := range values {
		if i > 0 && value == values[i-1] {
			continue
		}
		if result != "" {
			result += separator
		}
		result += value
	}
	return result
}

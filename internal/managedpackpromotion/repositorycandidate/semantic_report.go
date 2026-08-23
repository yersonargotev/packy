package repositorycandidate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

type semanticReport struct {
	previousVersion  string
	candidateVersion string
	actual           versionLevel
	floor            versionLevel
	floorReasons     []semanticReason
	changes          []semanticChange
	humanJudgment    []semanticChange
}

type semanticReason struct {
	level  versionLevel
	detail string
}

type semanticChange struct {
	section  string
	identity string
	detail   string
}

func compareSemanticChanges(current managedpack.Manifest, currentFiles []managedpack.FileRecord, candidate managedpack.Manifest, candidateFiles []managedpack.FileRecord) semanticReport {
	report := semanticReport{
		previousVersion: current.Version, candidateVersion: candidate.Version,
		actual: semanticChangeLevel(current.Version, candidate.Version), floor: patchLevel,
		changes: []semanticChange{}, floorReasons: []semanticReason{}, humanJudgment: []semanticChange{},
	}
	compareStringCollection(&report, "Pack contract", "supported surface", stringsOf(current.Surfaces), stringsOf(candidate.Surfaces), minorLevel, majorLevel)
	compareStringCollection(&report, "Pack contract", "readiness obligation", stringsOf(current.ReadinessObligations), stringsOf(candidate.ReadinessObligations), minorLevel, majorLevel)
	compareStringCollection(&report, "Pack contract", "external requirement", current.ExternalRequirements, candidate.ExternalRequirements, majorLevel, patchLevel)
	if current.Description != candidate.Description {
		detail := fmt.Sprintf("Pack description changed from %s to %s.", markdownCode(current.Description), markdownCode(candidate.Description))
		report.addChange("Metadata", "pack:description", detail)
		report.addReason(patchLevel, "changed Pack description")
		report.addHuman("Metadata", "pack:description", "Pack description changed; review its meaning.")
	}
	if current.Selectable != candidate.Selectable {
		report.addChange("Pack contract", "selectability", fmt.Sprintf("Selectability changed from `%t` to `%t`.", current.Selectable, candidate.Selectable))
		report.addReason(majorLevel, fmt.Sprintf("changed selectability from %t to %t", current.Selectable, candidate.Selectable))
	}
	compareOrigins(&report, current.Origins, candidate.Origins)
	compareResources(&report, current.Resources, currentFiles, candidate.Resources, candidateFiles)
	report.sort()
	return report
}

func compareOrigins(report *semanticReport, previous, candidate []managedpack.Origin) {
	oldOrigins := originMap(previous)
	newOrigins := originMap(candidate)
	for identity, origin := range newOrigins {
		if _, exists := oldOrigins[identity]; exists {
			continue
		}
		detail := fmt.Sprintf("Origin `%s` added: %s.", identity, originCoordinates(origin))
		report.addChange("Origins", identity, detail)
		report.addReason(patchLevel, "added origin "+identity)
		report.addHuman("Origins", identity, strings.TrimSuffix(detail, ".")+"; review its provenance.")
	}
	for identity, origin := range oldOrigins {
		if _, exists := newOrigins[identity]; exists {
			continue
		}
		detail := fmt.Sprintf("Origin `%s` removed: %s.", identity, originCoordinates(origin))
		report.addChange("Origins", identity, detail)
		report.addReason(patchLevel, "removed origin "+identity)
		report.addHuman("Origins", identity, strings.TrimSuffix(detail, ".")+"; review its provenance.")
	}
	for identity, oldOrigin := range oldOrigins {
		newOrigin, exists := newOrigins[identity]
		if !exists {
			continue
		}
		compareOriginField(report, identity, "repository", oldOrigin.Repository, newOrigin.Repository)
		compareOriginField(report, identity, "commit", oldOrigin.Commit, newOrigin.Commit)
		compareOriginField(report, identity, "revision", oldOrigin.Revision, newOrigin.Revision)
	}
}

func originCoordinates(origin managedpack.Origin) string {
	result := fmt.Sprintf("repository %s, commit %s", markdownCode(origin.Repository), markdownCode(origin.Commit))
	if origin.Revision != "" {
		result += fmt.Sprintf(", revision %s", markdownCode(origin.Revision))
	}
	return result
}

func originMap(values []managedpack.Origin) map[string]managedpack.Origin {
	result := make(map[string]managedpack.Origin, len(values))
	for _, origin := range values {
		result[origin.ID] = origin
	}
	return result
}

func compareOriginField(report *semanticReport, identity, field, previous, candidate string) {
	if previous == candidate {
		return
	}
	detail := fmt.Sprintf("Origin `%s` %s changed from %s to %s.", identity, field, markdownCode(previous), markdownCode(candidate))
	report.addChange("Origins", identity+":"+field, detail)
	report.addReason(patchLevel, "changed origin "+field+" of "+identity)
	report.addHuman("Origins", identity+":"+field, strings.TrimSuffix(detail, ".")+"; review its provenance.")
}

func compareResources(report *semanticReport, previous []managedpack.Resource, previousFiles []managedpack.FileRecord, candidate []managedpack.Resource, candidateFiles []managedpack.FileRecord) {
	oldResources := resourceMap(previous)
	newResources := resourceMap(candidate)
	compareGraph(report, previous, candidate)
	for identity, resource := range newResources {
		if _, exists := oldResources[identity]; exists {
			continue
		}
		report.addChange("Resources", identity, fmt.Sprintf("Resource `%s` added.", identity))
		if resourceHasMandatoryContract(resource) {
			report.addReason(majorLevel, "added mandatory resource "+identity)
		} else {
			report.addReason(minorLevel, "added isolated resource "+identity)
		}
		compareBindings(report, identity, nil, resource.Bindings, false)
		describeResourcePresence(report, identity, resource, candidateFiles, true)
	}
	for identity, resource := range oldResources {
		if _, exists := newResources[identity]; exists {
			continue
		}
		report.addChange("Resources", identity, fmt.Sprintf("Resource `%s` removed.", identity))
		report.addReason(majorLevel, "removed resource "+identity)
		compareBindings(report, identity, resource.Bindings, nil, false)
		describeResourcePresence(report, identity, resource, previousFiles, false)
	}
	for identity, oldResource := range oldResources {
		newResource, exists := newResources[identity]
		if !exists {
			continue
		}
		compareProjection(report, identity, oldResource, newResource)
		compareBindings(report, identity, oldResource.Bindings, newResource.Bindings, true)
		compareResourceMetadata(report, identity, oldResource, newResource)
		contentChanged := compareResourceContent(report, identity, oldResource, previousFiles, newResource, candidateFiles)
		if canonicalResource(oldResource) != canonicalResource(newResource) ||
			resourceGraphFingerprint(identity, previous) != resourceGraphFingerprint(identity, candidate) || contentChanged {
			report.addChange("Resources", identity, fmt.Sprintf("Resource `%s` modified.", identity))
		}
	}
}

func describeResourcePresence(report *semanticReport, identity string, resource managedpack.Resource, files []managedpack.FileRecord, added bool) {
	action := "added"
	if !added {
		action = "removed"
	}
	provenance := fmt.Sprintf("Resource `%s` %s as authored.", identity, action)
	if resource.Origin != nil {
		provenance = fmt.Sprintf("Resource `%s` %s as derived from %s (%s).", identity, action, markdownCode(resource.Origin.ID+":"+resource.Origin.Path), markdownCode(string(resource.Origin.Relationship)))
	}
	report.addChange("Provenance", identity+":"+action, provenance)
	report.addReason(patchLevel, action+" provenance of "+identity)
	report.addHuman("Provenance", identity+":"+action, strings.TrimSuffix(provenance, ".")+"; review its provenance.")

	if resource.License != "" {
		detail := fmt.Sprintf("Resource `%s` license added as %s.", identity, markdownCode(resource.License))
		if !added {
			detail = fmt.Sprintf("Resource `%s` license %s removed.", identity, markdownCode(resource.License))
		}
		report.addChange("Legal", identity+":license:"+action, detail)
		report.addReason(patchLevel, action+" license metadata of "+identity)
		report.addHuman("Legal", identity+":license:"+action, strings.TrimSuffix(detail, ".")+"; review the declared legal metadata.")
	}
	if resource.Attribution != "" {
		detail := fmt.Sprintf("Resource `%s` attribution added as %s.", identity, markdownCode(resource.Attribution))
		if !added {
			detail = fmt.Sprintf("Resource `%s` attribution %s removed.", identity, markdownCode(resource.Attribution))
		}
		report.addChange("Legal", identity+":attribution:"+action, detail)
		report.addReason(patchLevel, action+" attribution metadata of "+identity)
		report.addHuman("Legal", identity+":attribution:"+action, strings.TrimSuffix(detail, ".")+"; review the declared legal metadata.")
	}
	if added {
		compareNoticeAssociations(report, identity, nil, resource.Notices)
	} else {
		compareNoticeAssociations(report, identity, resource.Notices, nil)
	}
	if len(resourceFiles(resource, files)) == 0 {
		return
	}
	noun := "Resource"
	section := "Content"
	judgment := "review it without inferring behavioral compatibility."
	if resource.Kind == "notice" {
		noun = "Notice resource"
		section = "Legal"
		judgment = "review the content without inferring a legal conclusion."
	}
	detail := fmt.Sprintf("%s `%s` content %s.", noun, identity, action)
	report.addChange(section, identity+":content:"+action, detail)
	report.addReason(patchLevel, action+" content of "+identity)
	report.addHuman(section, identity+":content:"+action, strings.TrimSuffix(detail, ".")+"; "+judgment)
}

func compareResourceMetadata(report *semanticReport, identity string, previous, candidate managedpack.Resource) {
	if previous.Description != candidate.Description {
		detail := fmt.Sprintf("Resource `%s` description changed from %s to %s.", identity, markdownCode(previous.Description), markdownCode(candidate.Description))
		report.addChange("Metadata", identity+":description", detail)
		report.addReason(patchLevel, "changed description of "+identity)
		report.addHuman("Metadata", identity+":description", strings.TrimSuffix(detail, ".")+"; review its meaning.")
	}
	compareResourceReviewMetadata(report, identity, previous, candidate)
}

func compareResourceReviewMetadata(report *semanticReport, identity string, previous, candidate managedpack.Resource) {
	if previous.License != candidate.License {
		detail := fmt.Sprintf("Resource `%s` license changed from %s to %s.", identity, markdownCode(previous.License), markdownCode(candidate.License))
		report.addChange("Legal", identity+":license", detail)
		report.addReason(patchLevel, "changed license metadata of "+identity)
		report.addHuman("Legal", identity+":license", strings.TrimSuffix(detail, ".")+"; review the declared legal metadata.")
	}
	if previous.Attribution != candidate.Attribution {
		detail := fmt.Sprintf("Resource `%s` attribution changed from %s to %s.", identity, markdownCode(previous.Attribution), markdownCode(candidate.Attribution))
		report.addChange("Legal", identity+":attribution", detail)
		report.addReason(patchLevel, "changed attribution metadata of "+identity)
		report.addHuman("Legal", identity+":attribution", strings.TrimSuffix(detail, ".")+"; review the declared legal metadata.")
	}
	compareNoticeAssociations(report, identity, previous.Notices, candidate.Notices)
	compareResourceOrigin(report, identity, previous.Origin, candidate.Origin)
}

func compareNoticeAssociations(report *semanticReport, identity string, previous, candidate []string) {
	oldNotices := stringSet(previous)
	newNotices := stringSet(candidate)
	for notice := range newNotices {
		if _, exists := oldNotices[notice]; exists {
			continue
		}
		detail := fmt.Sprintf("Resource `%s` notice association `%s` added.", identity, notice)
		report.addChange("Legal", identity+":notice:"+notice, detail)
		report.addReason(patchLevel, "added notice association "+identity+" → "+notice)
		report.addHuman("Legal", identity+":notice:"+notice, strings.TrimSuffix(detail, ".")+"; review notice coverage.")
	}
	for notice := range oldNotices {
		if _, exists := newNotices[notice]; exists {
			continue
		}
		detail := fmt.Sprintf("Resource `%s` notice association `%s` removed.", identity, notice)
		report.addChange("Legal", identity+":notice:"+notice, detail)
		report.addReason(patchLevel, "removed notice association "+identity+" → "+notice)
		report.addHuman("Legal", identity+":notice:"+notice, strings.TrimSuffix(detail, ".")+"; review notice coverage.")
	}
}

func compareResourceOrigin(report *semanticReport, identity string, previous, candidate *managedpack.ResourceOrigin) {
	if previous == nil && candidate == nil {
		return
	}
	if previous == nil {
		detail := fmt.Sprintf("Resource `%s` changed from authored to derived (%s, %s).", identity, markdownCode(candidate.ID+":"+candidate.Path), markdownCode(string(candidate.Relationship)))
		report.addProvenance(identity, "authorship", detail, "changed "+identity+" from authored to derived")
		return
	}
	if candidate == nil {
		detail := fmt.Sprintf("Resource `%s` changed from derived (%s, %s) to authored.", identity, markdownCode(previous.ID+":"+previous.Path), markdownCode(string(previous.Relationship)))
		report.addProvenance(identity, "authorship", detail, "changed "+identity+" from derived to authored")
		return
	}
	if previous.ID != candidate.ID {
		detail := fmt.Sprintf("Resource `%s` origin changed from %s to %s.", identity, markdownCode(previous.ID), markdownCode(candidate.ID))
		report.addProvenance(identity, "origin", detail, "changed resource origin of "+identity)
	}
	if previous.Path != candidate.Path {
		detail := fmt.Sprintf("Resource `%s` origin path changed from %s to %s.", identity, markdownCode(previous.Path), markdownCode(candidate.Path))
		report.addProvenance(identity, "path", detail, "changed resource origin path of "+identity)
	}
	if previous.Relationship != candidate.Relationship {
		detail := fmt.Sprintf("Resource `%s` relationship changed from %s to %s.", identity, markdownCode(string(previous.Relationship)), markdownCode(string(candidate.Relationship)))
		report.addProvenance(identity, "relationship", detail, "changed provenance relationship of "+identity)
	}
}

func (report *semanticReport) addProvenance(identity, field, detail, reason string) {
	report.addChange("Provenance", identity+":"+field, detail)
	report.addReason(patchLevel, reason)
	report.addHuman("Provenance", identity+":"+field, strings.TrimSuffix(detail, ".")+"; review its provenance.")
}

func compareResourceContent(report *semanticReport, identity string, previous managedpack.Resource, previousFiles []managedpack.FileRecord, candidate managedpack.Resource, candidateFiles []managedpack.FileRecord) bool {
	if canonicalJSON(resourceFiles(previous, previousFiles)) == canonicalJSON(resourceFiles(candidate, candidateFiles)) {
		return false
	}
	if previous.Kind != "notice" && candidate.Kind != "notice" {
		detail := fmt.Sprintf("Resource `%s` content changed.", identity)
		report.addChange("Content", identity+":content", detail)
		report.addReason(patchLevel, "changed content of "+identity)
		report.addHuman("Content", identity+":content", strings.TrimSuffix(detail, ".")+"; review it without inferring behavioral compatibility.")
		return true
	}
	detail := fmt.Sprintf("Notice resource `%s` content changed.", identity)
	report.addChange("Legal", identity+":content", detail)
	report.addReason(patchLevel, "changed notice content of "+identity)
	report.addHuman("Legal", identity+":content", strings.TrimSuffix(detail, ".")+"; review the content without inferring a legal conclusion.")
	return true
}

func resourceFiles(resource managedpack.Resource, files []managedpack.FileRecord) map[string]map[string]managedpack.FileRecord {
	roots := map[string]bool{}
	if resource.Source != "" {
		roots[resource.Source] = true
	}
	for _, binding := range resource.Bindings {
		for _, root := range binding.ReferencedSourcePaths() {
			if root != "" {
				roots[root] = true
			}
		}
	}
	result := map[string]map[string]managedpack.FileRecord{}
	for root := range roots {
		tree := map[string]managedpack.FileRecord{}
		for _, file := range files {
			if file.Path != root && !strings.HasPrefix(file.Path, root+"/") {
				continue
			}
			relative := strings.TrimPrefix(strings.TrimPrefix(file.Path, root), "/")
			if relative == "" {
				relative = "."
			}
			copy := file
			copy.Path = relative
			tree[relative] = copy
		}
		if len(tree) > 0 {
			result[root] = tree
		}
	}
	return result
}

func canonicalResource(value managedpack.Resource) string {
	value.Requires = nil
	value.Conflicts = nil
	value.Args = append([]string{}, value.Args...)
	value.Tools = sortedStrings(value.Tools)
	value.Permissions = sortedStrings(value.Permissions)
	value.Notices = sortedStrings(value.Notices)
	value.SurfaceExclusions = sortedSurfaceExclusions(value.SurfaceExclusions)
	value.Bindings = append([]capabilitypack.Binding{}, value.Bindings...)
	for i := range value.Bindings {
		if value.Bindings[i].Hook != nil {
			copy := *value.Bindings[i].Hook
			copy.Args = append([]string{}, copy.Args...)
			copy.Authorities = sortedStrings(copy.Authorities)
			value.Bindings[i].Hook = &copy
		}
		value.Bindings[i].Capabilities = append([]capabilitypack.SurfaceCapability{}, value.Bindings[i].Capabilities...)
		for capabilityIndex := range value.Bindings[i].Capabilities {
			value.Bindings[i].Capabilities[capabilityIndex] = normalizedCapability(value.Bindings[i].Capabilities[capabilityIndex])
		}
		sort.Slice(value.Bindings[i].Capabilities, func(left, right int) bool {
			return canonicalCapability(value.Bindings[i].Capabilities[left]) < canonicalCapability(value.Bindings[i].Capabilities[right])
		})
	}
	sort.Slice(value.Bindings, func(i, j int) bool {
		return string(value.Bindings[i].Surface) < string(value.Bindings[j].Surface)
	})
	return canonicalJSON(value)
}

func resourceGraphFingerprint(identity string, resources []managedpack.Resource) string {
	var edges []string
	for edge := range requiresEdges(resources) {
		if strings.HasPrefix(edge, identity+" → ") {
			edges = append(edges, "requires:"+edge)
		}
	}
	for edge := range conflictEdges(resources) {
		left, right, _ := strings.Cut(edge, " — ")
		if left == identity || right == identity {
			edges = append(edges, "conflict:"+edge)
		}
	}
	sort.Strings(edges)
	return strings.Join(edges, "\n")
}

func compareGraph(report *semanticReport, previous, candidate []managedpack.Resource) {
	compareEdges(report, "requires", requiresEdges(previous), requiresEdges(candidate))
	compareEdges(report, "conflict", conflictEdges(previous), conflictEdges(candidate))
}

func requiresEdges(resources []managedpack.Resource) map[string]bool {
	result := map[string]bool{}
	for _, resource := range resources {
		from := resource.Kind + ":" + resource.ID
		for _, target := range resource.Requires {
			result[from+" → "+target] = true
		}
	}
	return result
}

func conflictEdges(resources []managedpack.Resource) map[string]bool {
	result := map[string]bool{}
	for _, resource := range resources {
		left := resource.Kind + ":" + resource.ID
		for _, target := range resource.Conflicts {
			right := target
			if right < left {
				left, right = right, left
			}
			result[left+" — "+right] = true
			left = resource.Kind + ":" + resource.ID
		}
	}
	return result
}

func compareEdges(report *semanticReport, kind string, previous, candidate map[string]bool) {
	noun := sentenceCase(kind) + " edge"
	for edge := range candidate {
		if previous[edge] {
			continue
		}
		report.addChange("Graph", kind+":"+edge+":added", fmt.Sprintf("%s `%s` added.", noun, edge))
		report.addReason(majorLevel, "added "+kind+" edge "+edge)
	}
	for edge := range previous {
		if candidate[edge] {
			continue
		}
		detail := fmt.Sprintf("%s `%s` removed.", noun, edge)
		report.addChange("Graph", kind+":"+edge+":removed", detail)
		report.addReason(patchLevel, "removed "+kind+" edge "+edge)
		report.addHuman("Graph", kind+":"+edge, strings.TrimSuffix(detail, ".")+"; review its meaning.")
	}
}

func compareProjection(report *semanticReport, identity string, previous, candidate managedpack.Resource) {
	compareProjectionValue(report, identity, "source", previous.Source, candidate.Source)
	compareProjectionValue(report, identity, "command", previous.Command, candidate.Command)
	compareProjectionValue(report, identity, "arguments", canonicalJSON(previous.Arguments), canonicalJSON(candidate.Arguments))
	compareProjectionValue(report, identity, "args", canonicalJSON(append([]string{}, previous.Args...)), canonicalJSON(append([]string{}, candidate.Args...)))
	compareProjectionValue(report, identity, "mode", previous.Mode, candidate.Mode)
	compareProjectionValue(report, identity, "tools", canonicalStringSet(previous.Tools), canonicalStringSet(candidate.Tools))
	compareProjectionValue(report, identity, "permissions", canonicalStringSet(previous.Permissions), canonicalStringSet(candidate.Permissions))
	compareProjectionValue(report, identity, "surface exclusions", canonicalSurfaceExclusions(previous.SurfaceExclusions), canonicalSurfaceExclusions(candidate.SurfaceExclusions))
}

func compareProjectionValue(report *semanticReport, identity, field, previous, candidate string) {
	if previous == candidate {
		return
	}
	report.addChange("Resources", identity+":"+field, fmt.Sprintf("Resource `%s` %s changed from %s to %s.", identity, field, markdownCode(previous), markdownCode(candidate)))
	report.addReason(majorLevel, "changed projection "+field+" of "+identity)
}

func compareBindings(report *semanticReport, resourceIdentity string, previous, candidate []capabilitypack.Binding, classifyFloor bool) {
	oldBindings := bindingMap(previous)
	newBindings := bindingMap(candidate)
	for key, binding := range newBindings {
		old, exists := oldBindings[key]
		identity := resourceIdentity + "/" + key
		if !exists {
			report.addChange("Bindings", identity, fmt.Sprintf("Binding `%s` added.", identity))
			if classifyFloor {
				report.addReason(majorLevel, "added binding "+identity)
			}
		} else if canonicalBinding(old) != canonicalBinding(binding) {
			report.addChange("Bindings", identity, fmt.Sprintf("Binding `%s` changed from %s to %s.", identity, markdownCode(canonicalBinding(old)), markdownCode(canonicalBinding(binding))))
			if classifyFloor {
				report.addReason(majorLevel, "changed binding "+identity)
			}
		}
	}
	for key := range oldBindings {
		if _, exists := newBindings[key]; exists {
			continue
		}
		identity := resourceIdentity + "/" + key
		report.addChange("Bindings", identity, fmt.Sprintf("Binding `%s` removed.", identity))
		if classifyFloor {
			report.addReason(majorLevel, "removed binding "+identity)
		}
	}
	compareCapabilities(report, resourceIdentity, previous, candidate, classifyFloor)
}

func bindingMap(values []capabilitypack.Binding) map[string]capabilitypack.Binding {
	result := make(map[string]capabilitypack.Binding, len(values))
	for _, binding := range values {
		result[string(binding.Surface)] = binding
	}
	return result
}

func canonicalBinding(value capabilitypack.Binding) string {
	value.Capabilities = nil
	if value.Hook != nil {
		copy := *value.Hook
		copy.Args = append([]string{}, copy.Args...)
		copy.Authorities = sortedStrings(copy.Authorities)
		value.Hook = &copy
	}
	return canonicalJSON(value)
}

func compareCapabilities(report *semanticReport, resourceIdentity string, previous, candidate []capabilitypack.Binding, classifyFloor bool) {
	oldCapabilities := capabilityMap(previous)
	newCapabilities := capabilityMap(candidate)
	for key, capability := range newCapabilities {
		identity := resourceIdentity + "/" + key
		old, exists := oldCapabilities[key]
		if !exists {
			report.addChange("Capabilities", identity, fmt.Sprintf("Capability `%s` added.", identity))
			if classifyFloor {
				report.addReason(majorLevel, "added capability "+identity)
			}
		} else if canonicalCapability(old) != canonicalCapability(capability) {
			report.addChange("Capabilities", identity, fmt.Sprintf("Capability `%s` changed from %s to %s.", identity, markdownCode(canonicalCapability(old)), markdownCode(canonicalCapability(capability))))
			if classifyFloor {
				report.addReason(majorLevel, "changed capability "+identity)
			}
		}
	}
	for key := range oldCapabilities {
		if _, exists := newCapabilities[key]; exists {
			continue
		}
		identity := resourceIdentity + "/" + key
		report.addChange("Capabilities", identity, fmt.Sprintf("Capability `%s` removed.", identity))
		if classifyFloor {
			report.addReason(majorLevel, "removed capability "+identity)
		}
	}
}

func capabilityMap(bindings []capabilitypack.Binding) map[string]capabilitypack.SurfaceCapability {
	result := map[string]capabilitypack.SurfaceCapability{}
	for _, binding := range bindings {
		for _, capability := range binding.Capabilities {
			result[string(binding.Surface)+"/"+string(capability.Type)] = capability
		}
	}
	return result
}

func canonicalCapability(value capabilitypack.SurfaceCapability) string {
	return canonicalJSON(normalizedCapability(value))
}

func normalizedCapability(value capabilitypack.SurfaceCapability) capabilitypack.SurfaceCapability {
	if value.ClaudeCompositeSkill != nil {
		copy := *value.ClaudeCompositeSkill
		copy.Dependencies = sortedResourceIdentities(copy.Dependencies)
		copy.References = sortedResourceIdentities(copy.References)
		value.ClaudeCompositeSkill = &copy
	}
	if value.ClaudeAgentDocument != nil {
		copy := *value.ClaudeAgentDocument
		copy.Skills = sortedResourceIdentities(copy.Skills)
		copy.Authority.Authorities = append([]capabilitypack.AuthorityRecord{}, copy.Authority.Authorities...)
		for i := range copy.Authority.Authorities {
			copy.Authority.Authorities[i].Declarations = sortedStrings(copy.Authority.Authorities[i].Declarations)
			copy.Authority.Authorities[i].ClaudeTools = sortedStrings(copy.Authority.Authorities[i].ClaudeTools)
		}
		sort.Slice(copy.Authority.Authorities, func(i, j int) bool {
			return canonicalJSON(copy.Authority.Authorities[i]) < canonicalJSON(copy.Authority.Authorities[j])
		})
		value.ClaudeAgentDocument = &copy
	}
	return value
}

func sortedResourceIdentities(values []capabilitypack.ResourceIdentity) []capabilitypack.ResourceIdentity {
	result := append([]capabilitypack.ResourceIdentity{}, values...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Kind+":"+result[i].ID < result[j].Kind+":"+result[j].ID
	})
	return result
}

func canonicalSurfaceExclusions(values []capabilitypack.SurfaceExclusion) string {
	return canonicalJSON(sortedSurfaceExclusions(values))
}

func sortedSurfaceExclusions(values []capabilitypack.SurfaceExclusion) []capabilitypack.SurfaceExclusion {
	result := append([]capabilitypack.SurfaceExclusion{}, values...)
	sort.Slice(result, func(i, j int) bool { return canonicalJSON(result[i]) < canonicalJSON(result[j]) })
	return result
}

func canonicalStringSet(values []string) string {
	return canonicalJSON(sortedStrings(values))
}

func sortedStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func canonicalJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func markdownCode(value string) string {
	escaped := strings.NewReplacer("\r", "\\r", "\n", "\\n", "\t", "\\t").Replace(value)
	if escaped == "" {
		return "`\"\"`"
	}
	longest := 0
	for _, run := range strings.FieldsFunc(escaped, func(value rune) bool { return value != '`' }) {
		if len(run) > longest {
			longest = len(run)
		}
	}
	delimiter := strings.Repeat("`", longest+1)
	boundarySpaces := strings.HasPrefix(escaped, " ") && strings.HasSuffix(escaped, " ") && strings.Trim(escaped, " ") != ""
	if strings.HasPrefix(escaped, "`") || strings.HasSuffix(escaped, "`") || boundarySpaces {
		escaped = " " + escaped + " "
	}
	return delimiter + escaped + delimiter
}

func semanticChangeLevel(previous, candidate string) versionLevel {
	from, fromErr := semver.StrictNewVersion(previous)
	to, toErr := semver.StrictNewVersion(candidate)
	if fromErr != nil || toErr != nil {
		return patchLevel
	}
	return changeLevel(from, to)
}

func stringsOf[T ~string](values []T) []string {
	result := make([]string, len(values))
	for i := range values {
		result[i] = string(values[i])
	}
	return result
}

func compareStringCollection(report *semanticReport, section, noun string, previous, candidate []string, added, removed versionLevel) {
	old := stringSet(previous)
	newValues := stringSet(candidate)
	for value := range newValues {
		if _, exists := old[value]; !exists {
			report.addChange(section, noun+":"+value, fmt.Sprintf("%s `%s` added.", sentenceCase(noun), value))
			report.addReason(added, "added "+noun+" "+value)
		}
	}
	for value := range old {
		if _, exists := newValues[value]; !exists {
			detail := fmt.Sprintf("%s `%s` removed.", sentenceCase(noun), value)
			report.addChange(section, noun+":"+value, detail)
			report.addReason(removed, "removed "+noun+" "+value)
			if removed == patchLevel {
				report.addHuman(section, noun+":"+value, strings.TrimSuffix(detail, ".")+"; review its meaning.")
			}
		}
	}
}

func sentenceCase(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (report *semanticReport) addChange(section, identity, detail string) {
	report.changes = append(report.changes, semanticChange{section: section, identity: identity, detail: detail})
}

func (report *semanticReport) addHuman(section, identity, detail string) {
	report.humanJudgment = append(report.humanJudgment, semanticChange{section: section, identity: identity, detail: detail})
}

func (report *semanticReport) addReason(level versionLevel, detail string) {
	if level > report.floor {
		report.floor = level
	}
	report.floorReasons = append(report.floorReasons, semanticReason{level: level, detail: detail})
}

func (report *semanticReport) sort() {
	sort.Slice(report.changes, func(i, j int) bool {
		return semanticChangeKey(report.changes[i]) < semanticChangeKey(report.changes[j])
	})
	sort.Slice(report.humanJudgment, func(i, j int) bool {
		return semanticChangeKey(report.humanJudgment[i]) < semanticChangeKey(report.humanJudgment[j])
	})
	sort.Slice(report.floorReasons, func(i, j int) bool {
		return report.floorReasons[i].detail < report.floorReasons[j].detail
	})
}

func semanticChangeKey(change semanticChange) string {
	return change.section + "\x00" + change.identity + "\x00" + change.detail
}

func (report semanticReport) renderMarkdown() string {
	var output strings.Builder
	output.WriteString("## Semantic changes\n")
	lastSection := ""
	for _, change := range report.changes {
		if change.section != lastSection {
			fmt.Fprintf(&output, "\n### %s\n\n", change.section)
			lastSection = change.section
		}
		fmt.Fprintf(&output, "- %s\n", change.detail)
	}
	if len(report.changes) == 0 {
		output.WriteString("\n- None.\n")
	}
	fmt.Fprintf(&output, "\n### Compatibility\n\n- Version: `%s` → `%s` (`%s`).\n- Mechanical floor: `%s`.\n", report.previousVersion, report.candidateVersion, report.actual, report.floor)
	if len(report.floorReasons) == 0 {
		output.WriteString("- Reasons: no mechanically classified structural changes.\n")
	} else {
		reasons := make([]string, len(report.floorReasons))
		for i := range report.floorReasons {
			reasons[i] = report.floorReasons[i].detail
		}
		fmt.Fprintf(&output, "- Reasons: %s.\n", strings.Join(reasons, "; "))
	}
	output.WriteString("\n### Human judgment\n\n")
	if len(report.humanJudgment) == 0 {
		output.WriteString("- None.\n")
	} else {
		for _, item := range report.humanJudgment {
			fmt.Fprintf(&output, "- %s\n", item.detail)
		}
	}
	return output.String()
}

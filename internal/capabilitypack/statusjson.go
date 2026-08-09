package capabilitypack

import "sort"

const StatusSchemaVersion = 9

type JSONIntent struct {
	State     string            `json:"state"`
	Active    *bool             `json:"active"`
	Revision  *int              `json:"revision"`
	Version   string            `json:"version,omitempty"`
	Selection ResourceSelection `json:"selection"`
}

type JSONReadiness = ReadinessStatus

type JSONOptionalAuthority struct {
	ModeID    string                 `json:"mode_id"`
	Authority string                 `json:"authority"`
	State     OptionalAuthorityState `json:"state"`
	Fallback  string                 `json:"fallback"`
}

type JSONProjectionSummary struct {
	Verified  int `json:"verified"`
	Missing   int `json:"missing"`
	Drifted   int `json:"drifted"`
	Ambiguous int `json:"ambiguous"`
	Unmanaged int `json:"unmanaged"`
}

type JSONProjectionStatus struct {
	ID                  string           `json:"id"`
	Target              string           `json:"target"`
	Owner               string           `json:"owner"`
	Health              ProjectionHealth `json:"health"`
	ObservedFingerprint string           `json:"observed_fingerprint"`
	DesiredFingerprint  string           `json:"desired_fingerprint"`
}

type JSONResourceSelectionStatus struct {
	Resource        ResourceIdentity   `json:"resource"`
	Selected        bool               `json:"selected"`
	Role            ResourceRole       `json:"role"`
	DependencyChain []ResourceIdentity `json:"dependency_chain"`
}

type JSONResourceStatus struct {
	Resource        ResourceIdentity      `json:"resource"`
	Role            ResourceRole          `json:"role"`
	DependencyChain []ResourceIdentity    `json:"dependency_chain"`
	Readiness       JSONReadiness         `json:"readiness"`
	Conditions      []ReadinessCondition  `json:"conditions"`
	Projections     JSONProjectionSummary `json:"projection_summary"`
	Blockers        []string              `json:"blockers"`
}

type JSONStatusRequirement struct {
	Resource  *ResourceIdentity `json:"resource,omitempty"`
	Readiness string            `json:"readiness"`
	Satisfied bool              `json:"satisfied"`
}

type JSONStatusEntry struct {
	Pack                string                        `json:"pack"`
	PackVersion         string                        `json:"pack_version"`
	Surface             Surface                       `json:"surface"`
	Intent              JSONIntent                    `json:"intent"`
	UpdateAvailable     bool                          `json:"update_available"`
	Projections         JSONProjectionSummary         `json:"projection_summary"`
	ProjectionDetails   []JSONProjectionStatus        `json:"projection_details"`
	ResourceSelections  []JSONResourceSelectionStatus `json:"resource_selections"`
	Resources           []JSONResourceStatus          `json:"resources"`
	Contract            LifecycleContract             `json:"contract"`
	Readiness           JSONReadiness                 `json:"readiness"`
	Conditions          []ReadinessCondition          `json:"conditions"`
	OptionalAuthorities []JSONOptionalAuthority       `json:"optional_authorities"`
	RuntimeModes        []RuntimeModeResult           `json:"runtime_modes,omitempty"`
	Blockers            []string                      `json:"blockers"`
	Evidence            []string                      `json:"evidence"`
	PendingHumanActions []string                      `json:"pending_human_actions"`
	ActivationRole      ActivationRole                `json:"activation_role"`
	LifecycleState      PackLifecycleState            `json:"lifecycle_state"`
}

type JSONStatusReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Report        string                 `json:"report"`
	Entries       []JSONStatusEntry      `json:"entries"`
	Focused       *JSONResourceStatus    `json:"focused_resource,omitempty"`
	Requirement   *JSONStatusRequirement `json:"requirement,omitempty"`
}

func (report StatusReport) JSONReport(targeted bool) JSONStatusReport {
	kind := "pack-status-overview"
	if targeted {
		kind = "pack-status"
	}
	entries := make([]JSONStatusEntry, 0, len(report.Entries))
	for _, entry := range report.Entries {
		intent := JSONIntent{State: "absent", Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}}
		if entry.IntentPresent {
			active, revision := entry.Intent.Active, entry.Intent.Revision
			selection, _ := canonicalSelection(entry.Intent.Selection)
			intent = JSONIntent{State: "known", Active: &active, Revision: &revision, Version: entry.Intent.Version, Selection: selection}
		}
		entries = append(entries, JSONStatusEntry{
			Pack: entry.Pack.ID, PackVersion: entry.Pack.Version, Surface: entry.Surface,
			Intent: intent, UpdateAvailable: entry.UpdateAvailable, Projections: JSONProjectionSummary{Verified: entry.Projections.Verified, Missing: entry.Projections.Missing, Drifted: entry.Projections.Drifted, Ambiguous: entry.Projections.Ambiguous, Unmanaged: entry.Projections.Unmanaged},
			Readiness:           entry.Readiness,
			Conditions:          append([]ReadinessCondition{}, entry.Conditions...),
			OptionalAuthorities: jsonOptionalAuthorities(entry.OptionalAuthorities),
			RuntimeModes:        sortedRuntimeModeResults(entry.RuntimeModes),
			ProjectionDetails:   jsonProjectionDetails(entry.ProjectionDetails), Contract: entry.Contract,
			ResourceSelections: jsonResourceSelectionDetails(entry.ResourceSelections),
			Resources:          jsonResourceStatuses(entry.Resources),
			Blockers:           sortedCopy(entry.Blockers), Evidence: sortedCopy(entry.Evidence), PendingHumanActions: sortedCopy(entry.PendingHumanActions),
			ActivationRole: statusActivationRole(entry), LifecycleState: normalizedLifecycleState(entry),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pack != entries[j].Pack {
			return entries[i].Pack < entries[j].Pack
		}
		return entries[i].Surface < entries[j].Surface
	})
	result := JSONStatusReport{SchemaVersion: StatusSchemaVersion, Report: kind, Entries: entries}
	if report.Focused != nil {
		focused := jsonResourceStatus(*report.Focused)
		result.Focused = &focused
	}
	if report.Requirement != nil {
		requirement := &JSONStatusRequirement{Readiness: report.Requirement.Readiness, Satisfied: report.Requirement.Satisfied}
		if report.Requirement.Resource.Kind != "" {
			resource := report.Requirement.Resource
			requirement.Resource = &resource
		}
		result.Requirement = requirement
	}
	return result
}

func normalizedLifecycleState(entry StatusEntry) PackLifecycleState {
	if entry.LifecycleState != "" {
		return entry.LifecycleState
	}
	if entry.IntentPresent && entry.Intent.Active {
		return PackLifecycleActive
	}
	return PackLifecycleInactiveClean
}

func statusActivationRole(entry StatusEntry) ActivationRole {
	if !entry.IntentPresent || !entry.Intent.Active {
		return ActivationInactive
	}
	return entry.ActivationRole
}

func jsonResourceStatuses(values []ResourceStatus) []JSONResourceStatus {
	result := make([]JSONResourceStatus, 0, len(values))
	for _, value := range values {
		result = append(result, jsonResourceStatus(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

func jsonResourceStatus(value ResourceStatus) JSONResourceStatus {
	return JSONResourceStatus{
		Resource: value.Resource, Role: value.Role, DependencyChain: append([]ResourceIdentity{}, value.DependencyChain...),
		Readiness: value.Readiness, Conditions: append([]ReadinessCondition{}, value.Conditions...),
		Projections: JSONProjectionSummary{Verified: value.Projections.Verified, Missing: value.Projections.Missing, Drifted: value.Projections.Drifted, Ambiguous: value.Projections.Ambiguous, Unmanaged: value.Projections.Unmanaged},
		Blockers:    sortedCopy(value.Blockers),
	}
}

func jsonResourceSelectionDetails(values []ResourceSelectionStatus) []JSONResourceSelectionStatus {
	result := make([]JSONResourceSelectionStatus, 0, len(values))
	for _, value := range values {
		result = append(result, JSONResourceSelectionStatus{
			Resource: value.Resource, Selected: value.Selected, Role: value.Role,
			DependencyChain: append([]ResourceIdentity{}, value.DependencyChain...),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Resource.String() < result[j].Resource.String() })
	return result
}

func jsonOptionalAuthorities(values []OptionalAuthorityObservation) []JSONOptionalAuthority {
	result := make([]JSONOptionalAuthority, 0, len(values))
	for _, value := range values {
		result = append(result, JSONOptionalAuthority{
			ModeID: value.ModeID, Authority: value.Authority, State: value.State, Fallback: value.Fallback,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ModeID != result[j].ModeID {
			return result[i].ModeID < result[j].ModeID
		}
		return result[i].Authority < result[j].Authority
	})
	return result
}

func jsonProjectionDetails(values []ProjectionStatus) []JSONProjectionStatus {
	result := make([]JSONProjectionStatus, 0, len(values))
	for _, value := range values {
		result = append(result, JSONProjectionStatus{ID: value.ID, Target: value.Target, Owner: value.Owner, Health: value.Health,
			ObservedFingerprint: value.ObservedFingerprint, DesiredFingerprint: value.DesiredFingerprint})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedCopy(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

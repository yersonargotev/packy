package capabilitypack

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RuntimeModeEvidence binds sanitized runtime evidence to one declared mode.
// ResourceID and ModeID are portable manifest identities, never host paths or
// probe data.
type RuntimeModeEvidence struct {
	ResourceID string          `json:"resource_id"`
	ModeID     string          `json:"mode_id"`
	Evidence   RuntimeEvidence `json:"evidence"`
}

// RuntimeModeState is the deterministic truth derived from all facts for a
// mode. Unavailable takes precedence over Unverified.
type RuntimeModeState = ObservationState

const (
	RuntimeModeAvailable   RuntimeModeState = ObservationAvailable
	RuntimeModeUnavailable RuntimeModeState = ObservationUnavailable
	RuntimeModeUnverified  RuntimeModeState = ObservationUnverified
)

// RuntimeModeResult preserves the complete declaration alongside normalized
// evidence. A fallback's truth is reported, but fallback selection is left to
// the caller.
type RuntimeModeResult struct {
	ResourceID    string                   `json:"resource_id"`
	ModeID        string                   `json:"mode_id"`
	Role          RuntimeModeRole          `json:"role"`
	State         RuntimeModeState         `json:"state"`
	Requirements  []RuntimeRequirement     `json:"requirements"`
	Authorities   []RuntimeAuthority       `json:"authorities"`
	Effects       []RuntimeEffect          `json:"effects"`
	Fallback      RuntimeFallback          `json:"fallback"`
	OnUnavailable RuntimeUnavailablePolicy `json:"on_unavailable"`
	FallbackState *RuntimeModeState        `json:"fallback_state,omitempty"`
	Evidence      RuntimeEvidence          `json:"evidence"`
	Affected      []string                 `json:"affected"`
}

const runtimeEvidenceFreshness = 5 * time.Minute

// UnverifiedRuntimeModeEvidence gives adapters the exact portable evidence
// skeleton for every declared mode. It is a safe default for observers that
// cannot establish runtime facts; no adapter needs to duplicate mode-policy
// iteration.
func UnverifiedRuntimeModeEvidence(pack Pack, observedAt time.Time, observerRevision string) ([]RuntimeModeEvidence, error) {
	if observedAt.IsZero() || observerRevision == "" {
		return nil, RuntimeEvidenceError{Problem: "an observation time and observer revision are required"}
	}
	stamp := observedAt.UTC().Format(time.RFC3339)
	records := make([]RuntimeModeEvidence, 0)
	for _, resource := range pack.Resources {
		for _, mode := range resource.RuntimeModes {
			evidence := RuntimeEvidence{Requirements: []RuntimeRequirementObservation{}, Authorities: []RuntimeAuthorityObservation{}}
			for _, requirement := range mode.Requirements {
				evidence.Requirements = append(evidence.Requirements, RuntimeRequirementObservation{
					Kind: requirement.Kind, ID: requirement.ID,
					RuntimeObservation: RuntimeObservation{State: ObservationUnverified, Reason: ObservationReasonObserverError, ObservedAt: stamp, ObserverRevision: observerRevision},
				})
			}
			for _, authority := range mode.Authorities {
				evidence.Authorities = append(evidence.Authorities, RuntimeAuthorityObservation{
					Kind: authority.Kind, Scope: authority.Scope,
					RuntimeObservation: RuntimeObservation{State: ObservationUnverified, Reason: ObservationReasonObserverError, ObservedAt: stamp, ObserverRevision: observerRevision},
				})
			}
			records = append(records, RuntimeModeEvidence{ResourceID: resource.ID, ModeID: mode.ID, Evidence: evidence})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return runtimeModeEvidenceKey(records[i].ResourceID, records[i].ModeID) < runtimeModeEvidenceKey(records[j].ResourceID, records[j].ModeID)
	})
	return records, nil
}

func cloneRuntimeModeEvidence(values []RuntimeModeEvidence) []RuntimeModeEvidence {
	result := make([]RuntimeModeEvidence, len(values))
	copy(result, values)
	for i := range result {
		result[i].Evidence = cloneRuntimeEvidence(result[i].Evidence)
	}
	return result
}

func cloneRuntimeModeResults(values []RuntimeModeResult) []RuntimeModeResult {
	result := make([]RuntimeModeResult, len(values))
	copy(result, values)
	for i := range result {
		result[i].Requirements = append([]RuntimeRequirement{}, result[i].Requirements...)
		result[i].Authorities = append([]RuntimeAuthority{}, result[i].Authorities...)
		result[i].Effects = append([]RuntimeEffect{}, result[i].Effects...)
		result[i].Evidence = cloneRuntimeEvidence(result[i].Evidence)
		result[i].Affected = append([]string{}, result[i].Affected...)
		if result[i].FallbackState != nil {
			state := *result[i].FallbackState
			result[i].FallbackState = &state
		}
	}
	return result
}

func sortedRuntimeModeResults(values []RuntimeModeResult) []RuntimeModeResult {
	result := cloneRuntimeModeResults(values)
	sort.Slice(result, func(i, j int) bool {
		return runtimeModeEvidenceKey(result[i].ResourceID, result[i].ModeID) <
			runtimeModeEvidenceKey(result[j].ResourceID, result[j].ModeID)
	})
	return result
}

func cloneRuntimeEvidence(value RuntimeEvidence) RuntimeEvidence {
	value.Requirements = append([]RuntimeRequirementObservation{}, value.Requirements...)
	value.Authorities = append([]RuntimeAuthorityObservation{}, value.Authorities...)
	return value
}

// RuntimeEvidenceError reports only portable declared identities.
type RuntimeEvidenceError struct {
	Problem  string
	Affected []string
}

func (e RuntimeEvidenceError) Error() string {
	if len(e.Affected) == 0 {
		return "runtime evidence: " + e.Problem
	}
	return fmt.Sprintf("runtime evidence: %s: %s", e.Problem, strings.Join(e.Affected, ", "))
}

// RuntimePreflightError means the requested mode must fail before effects.
type RuntimePreflightError struct {
	ResourceID string
	ModeID     string
	State      RuntimeModeState
	Affected   []string
}

func (e RuntimePreflightError) Error() string {
	return fmt.Sprintf("runtime preflight failed before effects for %s:%s (%s): %s",
		e.ResourceID, e.ModeID, e.State, strings.Join(e.Affected, ", "))
}

// EvaluateRuntimeModes validates exact evidence coverage and evaluates every
// declared runtime mode. maxAge must be positive; an observation older
// than maxAge is normalized to unverified/stale.
func EvaluateRuntimeModes(pack Pack, records []RuntimeModeEvidence, now time.Time, maxAge time.Duration) ([]RuntimeModeResult, error) {
	if now.IsZero() || maxAge <= 0 {
		return nil, RuntimeEvidenceError{Problem: "now and a positive max age are required"}
	}

	declared := make(map[string]RuntimeMode)
	keys := make([]string, 0)
	for _, resource := range pack.Resources {
		for _, mode := range resource.RuntimeModes {
			key := runtimeModeEvidenceKey(resource.ID, mode.ID)
			if _, exists := declared[key]; exists {
				return nil, RuntimeEvidenceError{Problem: "ambiguous declared mode", Affected: []string{portableModeIdentity(resource.ID, mode.ID)}}
			}
			declared[key] = mode
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	provided := make(map[string]RuntimeEvidence, len(records))
	for _, record := range records {
		key := runtimeModeEvidenceKey(record.ResourceID, record.ModeID)
		identity := portableModeIdentity(record.ResourceID, record.ModeID)
		if _, exists := provided[key]; exists {
			return nil, RuntimeEvidenceError{Problem: "duplicate mode evidence", Affected: []string{identity}}
		}
		if _, exists := declared[key]; !exists {
			return nil, RuntimeEvidenceError{Problem: "extra or mismatched mode evidence", Affected: []string{identity}}
		}
		if err := ValidateRuntimeEvidence(record.Evidence); err != nil {
			return nil, RuntimeEvidenceError{Problem: "invalid evidence for " + identity + ": " + err.Error()}
		}
		provided[key] = record.Evidence
	}
	missing := make([]string, 0)
	for _, key := range keys {
		if _, exists := provided[key]; !exists {
			resourceID, modeID := splitRuntimeModeEvidenceKey(key)
			missing = append(missing, portableModeIdentity(resourceID, modeID))
		}
	}
	if len(missing) != 0 {
		return nil, RuntimeEvidenceError{Problem: "missing mode evidence", Affected: missing}
	}

	results := make([]RuntimeModeResult, 0, len(keys))
	resultIndex := make(map[string]int, len(keys))
	for _, key := range keys {
		mode := declared[key]
		resourceID, modeID := splitRuntimeModeEvidenceKey(key)
		evidence := normalizeRuntimeEvidence(provided[key], now, maxAge)
		affected, err := exactRuntimeFactCoverage(mode, evidence)
		if err != nil {
			return nil, RuntimeEvidenceError{Problem: err.Error(), Affected: affected}
		}
		state, affected := deriveRuntimeModeState(evidence)
		resultIndex[key] = len(results)
		results = append(results, RuntimeModeResult{
			ResourceID: resourceID, ModeID: modeID, Role: mode.Role, State: state,
			Requirements: append([]RuntimeRequirement(nil), mode.Requirements...),
			Authorities:  append([]RuntimeAuthority(nil), mode.Authorities...),
			Effects:      append([]RuntimeEffect(nil), mode.Effects...),
			Fallback:     mode.Fallback, OnUnavailable: mode.OnUnavailable, Evidence: evidence, Affected: affected,
		})
	}
	for i := range results {
		if results[i].Fallback.Kind != RuntimeFallbackMode {
			continue
		}
		fallbackKey := runtimeModeEvidenceKey(results[i].ResourceID, results[i].Fallback.Mode)
		if fallbackIndex, ok := resultIndex[fallbackKey]; ok {
			state := results[fallbackIndex].State
			results[i].FallbackState = &state
		}
	}
	return results, nil
}

// PreflightRuntimeMode evaluates the entire evidence set, then fails closed
// for an unavailable or unverified requested mode. It never selects fallback.
func PreflightRuntimeMode(pack Pack, resourceID, modeID string, records []RuntimeModeEvidence, now time.Time, maxAge time.Duration) (RuntimeModeResult, error) {
	results, err := EvaluateRuntimeModes(pack, records, now, maxAge)
	if err != nil {
		return RuntimeModeResult{}, err
	}
	for _, result := range results {
		if result.ResourceID != resourceID || result.ModeID != modeID {
			continue
		}
		if result.State != RuntimeModeAvailable {
			return result, RuntimePreflightError{
				ResourceID: resourceID, ModeID: modeID, State: result.State,
				Affected: append([]string(nil), result.Affected...),
			}
		}
		return result, nil
	}
	return RuntimeModeResult{}, RuntimeEvidenceError{
		Problem:  "requested mode is not declared",
		Affected: []string{portableModeIdentity(resourceID, modeID)},
	}
}

func exactRuntimeFactCoverage(mode RuntimeMode, evidence RuntimeEvidence) ([]string, error) {
	wantRequirements := make(map[string]bool, len(mode.Requirements))
	for _, requirement := range mode.Requirements {
		wantRequirements[runtimeScopedKey(requirement.Kind, requirement.ID)] = true
	}
	wantAuthorities := make(map[string]bool, len(mode.Authorities))
	for _, authority := range mode.Authorities {
		wantAuthorities[runtimeScopedKey(authority.Kind, authority.Scope)] = true
	}
	affected := make([]string, 0)
	for _, observation := range evidence.Requirements {
		key := runtimeScopedKey(observation.Kind, observation.ID)
		if !wantRequirements[key] {
			affected = append(affected, portableRequirementIdentity(observation.Kind, observation.ID))
		}
		delete(wantRequirements, key)
	}
	for _, observation := range evidence.Authorities {
		key := runtimeScopedKey(observation.Kind, observation.Scope)
		if !wantAuthorities[key] {
			affected = append(affected, portableAuthorityIdentity(observation.Kind, observation.Scope))
		}
		delete(wantAuthorities, key)
	}
	for key := range wantRequirements {
		parts := strings.Split(key, "\x00")
		affected = append(affected, portableRequirementIdentity(RuntimeRequirementKind(parts[0]), parts[1]))
	}
	for key := range wantAuthorities {
		parts := strings.Split(key, "\x00")
		affected = append(affected, portableAuthorityIdentity(RuntimeAuthorityKind(parts[0]), RuntimeScope(parts[1])))
	}
	if len(affected) != 0 {
		sort.Strings(affected)
		return affected, errors.New("evidence facts do not exactly match the mode declaration")
	}
	return nil, nil
}

func normalizeRuntimeEvidence(evidence RuntimeEvidence, now time.Time, maxAge time.Duration) RuntimeEvidence {
	normalized := RuntimeEvidence{
		Requirements: append([]RuntimeRequirementObservation(nil), evidence.Requirements...),
		Authorities:  append([]RuntimeAuthorityObservation(nil), evidence.Authorities...),
	}
	for i := range normalized.Requirements {
		normalizeRuntimeObservation(&normalized.Requirements[i].RuntimeObservation, now, maxAge)
	}
	for i := range normalized.Authorities {
		normalizeRuntimeObservation(&normalized.Authorities[i].RuntimeObservation, now, maxAge)
	}
	return normalized
}

func normalizeRuntimeObservation(observation *RuntimeObservation, now time.Time, maxAge time.Duration) {
	observedAt, _ := time.Parse(time.RFC3339, observation.ObservedAt)
	if observedAt.After(now) || now.Sub(observedAt) > maxAge {
		observation.State = ObservationUnverified
		observation.Reason = ObservationReasonStale
	}
}

func deriveRuntimeModeState(evidence RuntimeEvidence) (RuntimeModeState, []string) {
	state := RuntimeModeAvailable
	affected := make([]string, 0)
	for _, observation := range evidence.Requirements {
		if observation.State == ObservationUnavailable {
			state = RuntimeModeUnavailable
		} else if observation.State == ObservationUnverified && state != RuntimeModeUnavailable {
			state = RuntimeModeUnverified
		}
	}
	for _, observation := range evidence.Authorities {
		if observation.State == ObservationUnavailable {
			state = RuntimeModeUnavailable
		} else if observation.State == ObservationUnverified && state != RuntimeModeUnavailable {
			state = RuntimeModeUnverified
		}
	}
	for _, observation := range evidence.Requirements {
		if observation.State != ObservationAvailable {
			affected = append(affected, portableRequirementIdentity(observation.Kind, observation.ID))
		}
	}
	for _, observation := range evidence.Authorities {
		if observation.State != ObservationAvailable {
			affected = append(affected, portableAuthorityIdentity(observation.Kind, observation.Scope))
		}
	}
	sort.Strings(affected)
	return state, affected
}

func runtimeModeEvidenceKey(resourceID, modeID string) string { return resourceID + "\x00" + modeID }
func splitRuntimeModeEvidenceKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	return parts[0], parts[1]
}
func portableModeIdentity(resourceID, modeID string) string {
	return "mode:" + resourceID + ":" + modeID
}
func portableRequirementIdentity(kind RuntimeRequirementKind, id string) string {
	return "requirement:" + string(kind) + ":" + id
}
func portableAuthorityIdentity(kind RuntimeAuthorityKind, scope RuntimeScope) string {
	return "authority:" + string(kind) + ":" + string(scope)
}

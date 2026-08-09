package capabilitypack

import (
	"fmt"
	"sort"
	"time"
)

type ReadinessValue string

const (
	ReadinessTrue    ReadinessValue = "true"
	ReadinessFalse   ReadinessValue = "false"
	ReadinessUnknown ReadinessValue = "unknown"
)

type ReadinessDimension string

const (
	ReadinessConfigured ReadinessDimension = "configured"
	ReadinessAuthorized ReadinessDimension = "authorized"
	ReadinessUsable     ReadinessDimension = "usable"
)

type ReadinessConditionType string

const (
	ConditionProjectionIntegrity  ReadinessConditionType = "projection-integrity"
	ConditionExternalRequirement  ReadinessConditionType = "external-requirement"
	ConditionSurfaceAuthorization ReadinessConditionType = "surface-authorization"
	ConditionRuntimeUsability     ReadinessConditionType = "runtime-usability"
)

type ReadinessReason string

const (
	ReasonProjectionVerified      ReadinessReason = "projection-verified"
	ReasonProjectionMissing       ReadinessReason = "projection-missing"
	ReasonProjectionDrifted       ReadinessReason = "projection-drifted"
	ReasonProjectionAmbiguous     ReadinessReason = "projection-ambiguous"
	ReasonProjectionUnmanaged     ReadinessReason = "projection-unmanaged"
	ReasonRequirementAvailable    ReadinessReason = "requirement-available"
	ReasonRequirementMissing      ReadinessReason = "requirement-missing"
	ReasonRequirementUnobservable ReadinessReason = "requirement-unobservable"
	ReasonAuthorizationConfirmed  ReadinessReason = "authorization-confirmed"
	ReasonAuthorizationDenied     ReadinessReason = "authorization-denied"
	ReasonAuthorizationUnknown    ReadinessReason = "authorization-unobservable"
	ReasonRuntimeConfirmed        ReadinessReason = "runtime-confirmed"
	ReasonRuntimeRejected         ReadinessReason = "runtime-rejected"
	ReasonRuntimeUnobservable     ReadinessReason = "runtime-unobservable"
	ReasonRuntimeCheckStale       ReadinessReason = "runtime-check-stale"
)

type ReadinessScopeKind string

const (
	ReadinessScopeGlobal  ReadinessScopeKind = "global"
	ReadinessScopeProject ReadinessScopeKind = "project"
)

type ReadinessScope struct {
	Kind     ReadinessScopeKind `json:"kind"`
	Pack     string             `json:"pack"`
	Surface  Surface            `json:"surface"`
	Resource *ResourceIdentity  `json:"resource,omitempty"`
}

type ReadinessFreshness struct {
	ObservedAt       string `json:"observed_at"`
	ValidityIdentity string `json:"validity_identity"`
}

type ReadinessCondition struct {
	Type      ReadinessConditionType `json:"type"`
	Scope     ReadinessScope         `json:"scope"`
	Dimension ReadinessDimension     `json:"dimension"`
	Value     ReadinessValue         `json:"value"`
	Reason    ReadinessReason        `json:"reason"`
	Message   string                 `json:"message"`
	Evidence  []string               `json:"evidence"`
	Freshness ReadinessFreshness     `json:"freshness"`
}

type ReadinessStatus struct {
	Configured ReadinessValue `json:"configured"`
	Authorized ReadinessValue `json:"authorized"`
	Usable     ReadinessValue `json:"usable"`
}

func (status ReadinessStatus) SatisfiesUsable() bool {
	return status.Configured == ReadinessTrue && status.Authorized == ReadinessTrue && status.Usable == ReadinessTrue
}

func readinessValue(observed, value bool) ReadinessValue {
	if !observed {
		return ReadinessUnknown
	}
	if value {
		return ReadinessTrue
	}
	return ReadinessFalse
}

func configuredReadiness(value bool) ReadinessValue {
	if value {
		return ReadinessTrue
	}
	return ReadinessFalse
}

type readinessEvaluation struct {
	Pack                   Pack
	Surface                Surface
	Scope                  ReadinessScopeKind
	Resource               *ResourceIdentity
	Projections            []ProjectionStatus
	Resolutions            []ExecutableResolution
	UnobservedRequirements []string
	Observation            ReadinessObservation
	Revision               string
	ObservedAt             time.Time
	ControlledCheck        *ControlledCheckStatus
}

func evaluateReadiness(input readinessEvaluation) (ReadinessStatus, []ReadinessCondition) {
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC().Truncate(time.Second)
	}
	baseScope := ReadinessScope{Kind: input.Scope, Pack: input.Pack.ID, Surface: input.Surface, Resource: input.Resource}
	validity := fmt.Sprintf("%s@%s/%s/%s/%s", input.Pack.ID, input.Pack.Version, input.Surface, input.Scope, input.Revision)
	conditions := make([]ReadinessCondition, 0, len(input.Projections)+len(input.Resolutions)+len(input.UnobservedRequirements)+len(input.Pack.ReadinessObligations))
	freshness := func(suffix string) ReadinessFreshness {
		return ReadinessFreshness{ObservedAt: input.ObservedAt.Format(time.RFC3339), ValidityIdentity: validity + "/" + suffix}
	}
	if len(input.Projections) == 0 {
		conditions = append(conditions, ReadinessCondition{
			Type: ConditionProjectionIntegrity, Scope: baseScope, Dimension: ReadinessConfigured, Value: ReadinessFalse,
			Reason: ReasonProjectionMissing, Message: "no required Pack projection was observed", Evidence: []string{}, Freshness: freshness("projection:none"),
		})
	}
	for _, projection := range input.Projections {
		value, reason := ReadinessFalse, projectionReason(projection.Health)
		if projection.Health == ProjectionVerified {
			value = ReadinessTrue
		}
		conditions = append(conditions, ReadinessCondition{
			Type: ConditionProjectionIntegrity, Scope: baseScope, Dimension: ReadinessConfigured, Value: value,
			Reason: reason, Message: fmt.Sprintf("Pack projection %s is %s", projection.ID, projection.Health),
			Evidence: []string{"projection:" + projection.ID}, Freshness: freshness("projection:" + projection.ID),
		})
	}
	for _, resolution := range input.Resolutions {
		value, reason, message := ReadinessTrue, ReasonRequirementAvailable, fmt.Sprintf("external requirement %s is available", resolution.Tool)
		if !resolution.Available {
			value, reason, message = ReadinessFalse, ReasonRequirementMissing, fmt.Sprintf("external requirement %s is missing", resolution.Tool)
		}
		conditions = append(conditions, ReadinessCondition{
			Type: ConditionExternalRequirement, Scope: baseScope, Dimension: ReadinessUsable, Value: value,
			Reason: reason, Message: message, Evidence: []string{"executable:" + resolution.Tool}, Freshness: freshness("requirement:" + resolution.Tool + ":" + resolution.Precondition),
		})
	}
	for _, tool := range input.UnobservedRequirements {
		conditions = append(conditions, ReadinessCondition{
			Type: ConditionExternalRequirement, Scope: baseScope, Dimension: ReadinessUsable, Value: ReadinessUnknown,
			Reason: ReasonRequirementUnobservable, Message: fmt.Sprintf("external requirement %s cannot be observed", tool),
			Evidence: []string{"executable:" + tool}, Freshness: freshness("requirement:" + tool + ":unobservable"),
		})
	}
	for _, obligation := range input.Pack.ReadinessObligations {
		switch obligation {
		case ReadinessSurfaceAuthorization:
			value, reason, message := observedReadiness(input.Observation.AuthorizationObserved, input.Observation.Authorized, ReasonAuthorizationConfirmed, ReasonAuthorizationDenied, ReasonAuthorizationUnknown, "surface authorization is confirmed", "surface authorization was denied", "surface authorization cannot be observed")
			conditions = append(conditions, ReadinessCondition{Type: ConditionSurfaceAuthorization, Scope: baseScope, Dimension: ReadinessAuthorized, Value: value, Reason: reason, Message: message, Evidence: readinessEvidence(input.Observation.Evidence, input.Revision), Freshness: freshness(string(obligation))})
		case ReadinessRuntimeUsability:
			value, reason, message, evidence, checkFreshness := controlledCheckReadiness(input.ControlledCheck)
			if input.ControlledCheck != nil && input.ControlledCheck.State == ControlledCheckStale {
				// A stale human result is deliberately not mixed with a fresh
				// adapter fact. It remains visible as actionable unknown evidence.
			} else if input.ControlledCheck == nil || input.ControlledCheck.State != ControlledCheckCurrent {
				value, reason, message = observedReadiness(input.Observation.UsabilityObserved, input.Observation.Usable, ReasonRuntimeConfirmed, ReasonRuntimeRejected, ReasonRuntimeUnobservable, "runtime usability is confirmed", "runtime usability was rejected", "runtime usability cannot be observed")
				evidence, checkFreshness = readinessEvidence(input.Observation.Evidence, input.Revision), freshness(string(obligation))
			}
			conditions = append(conditions, ReadinessCondition{Type: ConditionRuntimeUsability, Scope: baseScope, Dimension: ReadinessUsable, Value: value, Reason: reason, Message: message, Evidence: evidence, Freshness: checkFreshness})
		}
	}
	sort.SliceStable(conditions, func(i, j int) bool {
		if readinessDimensionRank(conditions[i].Dimension) != readinessDimensionRank(conditions[j].Dimension) {
			return readinessDimensionRank(conditions[i].Dimension) < readinessDimensionRank(conditions[j].Dimension)
		}
		if conditions[i].Type != conditions[j].Type {
			return conditions[i].Type < conditions[j].Type
		}
		return conditions[i].Freshness.ValidityIdentity < conditions[j].Freshness.ValidityIdentity
	})
	return ReadinessStatus{
		Configured: aggregateReadinessDimension(conditions, ReadinessConfigured),
		Authorized: aggregateReadinessDimension(conditions, ReadinessAuthorized),
		Usable:     aggregateReadinessDimension(conditions, ReadinessUsable),
	}, conditions
}

func controlledCheckReadiness(check *ControlledCheckStatus) (ReadinessValue, ReadinessReason, string, []string, ReadinessFreshness) {
	if check == nil || check.State == ControlledCheckUnknown {
		return ReadinessUnknown, ReasonRuntimeUnobservable, "runtime usability cannot be observed", nil, ReadinessFreshness{}
	}
	freshness := ReadinessFreshness{ObservedAt: check.ObservedAt, ValidityIdentity: check.ValidityIdentity}
	if check.State == ControlledCheckStale {
		return ReadinessUnknown, ReasonRuntimeCheckStale, "controlled runtime check evidence is stale; rerun the controlled check", []string{"controlled-check:" + check.ValidityIdentity}, freshness
	}
	if check.Result == ReadinessTrue {
		return ReadinessTrue, ReasonRuntimeConfirmed, "controlled runtime check succeeded", []string{"controlled-check:" + check.ValidityIdentity}, freshness
	}
	return ReadinessFalse, ReasonRuntimeRejected, "controlled runtime check failed; rerun the check after fixing the host behavior", []string{"controlled-check:" + check.ValidityIdentity}, freshness
}

func aggregateReadinessDimension(conditions []ReadinessCondition, dimension ReadinessDimension) ReadinessValue {
	result, found := ReadinessTrue, false
	for _, condition := range conditions {
		if condition.Dimension != dimension {
			continue
		}
		found = true
		if condition.Value == ReadinessFalse {
			return ReadinessFalse
		}
		if condition.Value == ReadinessUnknown {
			result = ReadinessUnknown
		}
	}
	if !found {
		return ReadinessTrue
	}
	return result
}

func projectionReason(health ProjectionHealth) ReadinessReason {
	switch health {
	case ProjectionVerified:
		return ReasonProjectionVerified
	case ProjectionDrifted:
		return ReasonProjectionDrifted
	case ProjectionAmbiguous:
		return ReasonProjectionAmbiguous
	case ProjectionUnmanaged:
		return ReasonProjectionUnmanaged
	default:
		return ReasonProjectionMissing
	}
}

func observedReadiness(observed, value bool, positive, negative, unknown ReadinessReason, positiveMessage, negativeMessage, unknownMessage string) (ReadinessValue, ReadinessReason, string) {
	if !observed {
		return ReadinessUnknown, unknown, unknownMessage
	}
	if value {
		return ReadinessTrue, positive, positiveMessage
	}
	return ReadinessFalse, negative, negativeMessage
}

func readinessEvidence(evidence []string, revision string) []string {
	if len(evidence) == 0 {
		return []string{"surface-observation:" + revision}
	}
	result := append([]string(nil), evidence...)
	sort.Strings(result)
	return result
}

func readinessDimensionRank(value ReadinessDimension) int {
	switch value {
	case ReadinessConfigured:
		return 0
	case ReadinessAuthorized:
		return 1
	default:
		return 2
	}
}

func cloneReadinessConditions(values []ReadinessCondition) []ReadinessCondition {
	result := append([]ReadinessCondition(nil), values...)
	for i := range result {
		result[i].Evidence = append([]string(nil), result[i].Evidence...)
		if result[i].Scope.Resource != nil {
			resource := *result[i].Scope.Resource
			result[i].Scope.Resource = &resource
		}
	}
	return result
}

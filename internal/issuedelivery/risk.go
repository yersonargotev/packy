package issuedelivery

import (
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type CandidateEffect string

const (
	EffectPassive           CandidateEffect = "passive"
	EffectOrdinaryBehavior  CandidateEffect = "ordinary-behavior"
	EffectInstallation      CandidateEffect = "installation"
	EffectRealConfiguration CandidateEffect = "real-configuration"
	EffectSecurity          CandidateEffect = "security"
	EffectPublication       CandidateEffect = "publication"
	EffectMigration         CandidateEffect = "migration"
	EffectPersistentFormat  CandidateEffect = "persistent-format"
	EffectGovernance        CandidateEffect = "governance"
	EffectDestructive       CandidateEffect = "destructive-effect"
)

type SensitiveBoundary string

const (
	BoundaryInstallation      SensitiveBoundary = "installation"
	BoundaryRealConfiguration SensitiveBoundary = "real-configuration"
	BoundarySecurity          SensitiveBoundary = "security"
	BoundaryPublication       SensitiveBoundary = "publication"
	BoundaryMigration         SensitiveBoundary = "migration"
	BoundaryPersistentFormat  SensitiveBoundary = "persistent-format"
	BoundaryGovernance        SensitiveBoundary = "governance"
	BoundaryDestructive       SensitiveBoundary = "destructive-effect"
)

type EffectObservation struct {
	Effect   CandidateEffect `json:"effect"`
	Evidence string          `json:"evidence"`
	Complete bool            `json:"complete"`
}

type RiskAssessment struct {
	Profile     deliveryevidence.DeliveryRiskProfile `json:"profile"`
	Effects     []EffectObservation                  `json:"effects"`
	Boundaries  []SensitiveBoundary                  `json:"boundaries,omitempty"`
	Specialists []string                             `json:"specialists,omitempty"`
	Complete    bool                                 `json:"complete"`
}

func mechanicalProfileFloor(observations []EffectObservation) RiskAssessment {
	assessment := RiskAssessment{
		Profile:     deliveryevidence.RiskLow,
		Boundaries:  []SensitiveBoundary{},
		Specialists: []string{},
		Complete:    len(observations) > 0,
	}
	boundaries := map[SensitiveBoundary]struct{}{}
	effects := map[string]EffectObservation{}

	for _, observation := range observations {
		observation.Evidence = strings.TrimSpace(observation.Evidence)
		key := string(observation.Effect) + "\x00" + observation.Evidence
		if existing, found := effects[key]; found {
			observation.Complete = observation.Complete && existing.Complete
		}
		effects[key] = observation

		profile, boundary, known := profileForEffect(observation.Effect)
		assessment.Profile = maxRiskProfile(assessment.Profile, profile)
		if !known || !observation.Complete || observation.Evidence == "" {
			assessment.Complete = false
			assessment.Profile = deliveryevidence.RiskHigh
			boundaries[BoundaryGovernance] = struct{}{}
		}
		if boundary != "" {
			boundaries[boundary] = struct{}{}
		}
	}

	if !assessment.Complete {
		fallback := EffectObservation{
			Effect:   EffectGovernance,
			Evidence: "risk assessment incomplete; conservative governance review required",
			Complete: false,
		}
		effects[string(fallback.Effect)+"\x00"+fallback.Evidence] = fallback
		boundaries[BoundaryGovernance] = struct{}{}
		assessment.Profile = deliveryevidence.RiskHigh
	}

	assessment.Effects = make([]EffectObservation, 0, len(effects))
	for _, observation := range effects {
		assessment.Effects = append(assessment.Effects, observation)
	}
	sort.Slice(assessment.Effects, func(i, j int) bool {
		if assessment.Effects[i].Effect != assessment.Effects[j].Effect {
			return assessment.Effects[i].Effect < assessment.Effects[j].Effect
		}
		if assessment.Effects[i].Evidence != assessment.Effects[j].Evidence {
			return assessment.Effects[i].Evidence < assessment.Effects[j].Evidence
		}
		return !assessment.Effects[i].Complete && assessment.Effects[j].Complete
	})

	for boundary := range boundaries {
		assessment.Boundaries = append(assessment.Boundaries, boundary)
	}
	sort.Slice(assessment.Boundaries, func(i, j int) bool {
		return assessment.Boundaries[i] < assessment.Boundaries[j]
	})
	for _, boundary := range assessment.Boundaries {
		assessment.Specialists = append(assessment.Specialists, specialistForBoundary(boundary))
	}
	sort.Strings(assessment.Specialists)
	return assessment
}

func profileForEffect(effect CandidateEffect) (deliveryevidence.DeliveryRiskProfile, SensitiveBoundary, bool) {
	switch effect {
	case EffectPassive:
		return deliveryevidence.RiskLow, "", true
	case EffectOrdinaryBehavior:
		return deliveryevidence.RiskStandard, "", true
	case EffectInstallation:
		return deliveryevidence.RiskHigh, BoundaryInstallation, true
	case EffectRealConfiguration:
		return deliveryevidence.RiskHigh, BoundaryRealConfiguration, true
	case EffectSecurity:
		return deliveryevidence.RiskHigh, BoundarySecurity, true
	case EffectPublication:
		return deliveryevidence.RiskHigh, BoundaryPublication, true
	case EffectMigration:
		return deliveryevidence.RiskHigh, BoundaryMigration, true
	case EffectPersistentFormat:
		return deliveryevidence.RiskHigh, BoundaryPersistentFormat, true
	case EffectGovernance:
		return deliveryevidence.RiskHigh, BoundaryGovernance, true
	case EffectDestructive:
		return deliveryevidence.RiskHigh, BoundaryDestructive, true
	default:
		return deliveryevidence.RiskHigh, BoundaryGovernance, false
	}
}

func specialistForBoundary(boundary SensitiveBoundary) string {
	switch boundary {
	case BoundaryInstallation:
		return "installation-specialist"
	case BoundaryRealConfiguration:
		return "configuration-specialist"
	case BoundarySecurity:
		return "security-specialist"
	case BoundaryPublication:
		return "publication-specialist"
	case BoundaryMigration:
		return "migration-specialist"
	case BoundaryPersistentFormat:
		return "persistent-format-specialist"
	case BoundaryGovernance:
		return "governance-specialist"
	case BoundaryDestructive:
		return "destructive-effect-specialist"
	default:
		return "governance-specialist"
	}
}

func maxRiskProfile(left, right deliveryevidence.DeliveryRiskProfile) deliveryevidence.DeliveryRiskProfile {
	leftRank, leftKnown := riskProfileRank(left)
	rightRank, rightKnown := riskProfileRank(right)
	if !leftKnown || !rightKnown {
		return deliveryevidence.RiskHigh
	}
	if leftRank >= rightRank {
		return left
	}
	return right
}

func riskProfileRank(profile deliveryevidence.DeliveryRiskProfile) (int, bool) {
	switch profile {
	case deliveryevidence.RiskLow:
		return 0, true
	case deliveryevidence.RiskStandard:
		return 1, true
	case deliveryevidence.RiskHigh:
		return 2, true
	default:
		return 2, false
	}
}

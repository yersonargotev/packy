package capabilitypack

import (
	"context"
	"fmt"
)

// SkillDiscoveryFact compares one intended tree with a same-name tree in a
// native or compatibility discovery root. It grants no ownership.
type SkillDiscoveryFact struct {
	Host                                                     Surface
	Name, Target, OtherTarget, Fingerprint, OtherFingerprint string
}

func skillDiscoveryBlockers(facts []SkillDiscoveryFact) []PlanBlocker {
	var blockers []PlanBlocker
	for _, fact := range facts {
		if fact.Fingerprint == fact.OtherFingerprint && fact.Fingerprint != "unverified" {
			continue
		}
		detail := fmt.Sprintf("%s discovers divergent or unverified skill %q at %s and %s; deterministic coexistence is not verified. Keep only one definition in these discovery roots, or use distinct native skill names.", fact.Host, fact.Name, fact.Target, fact.OtherTarget)
		if fact.Host == SurfaceOpenCode {
			detail += " OPENCODE_DISABLE_EXTERNAL_SKILLS=1 isolates an explicitly controlled OpenCode launch only; Packy cannot verify every launch context and does not accept its own environment as persistent isolation."
		}
		blockers = append(blockers, PlanBlocker{Kind: BlockerHostDiscovery, Subject: "skill:" + fact.Name, Detail: detail})
	}
	sortBlockers(blockers)
	return blockers
}

// SkillDiscoveryObserver supplies native filesystem discovery facts without
// granting ownership or choosing lifecycle policy.
type SkillDiscoveryObserver func(context.Context, SurfaceTransition, SurfaceInspection) ([]SkillDiscoveryFact, error)

type discoverySurfaceAdapter struct {
	SurfaceAdapter
	observe SkillDiscoveryObserver
}

func WithSkillDiscoveryObserver(adapter SurfaceAdapter, observe SkillDiscoveryObserver) SurfaceAdapter {
	return discoverySurfaceAdapter{SurfaceAdapter: adapter, observe: observe}
}

func (a discoverySurfaceAdapter) InspectSurface(ctx context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	observation, err := inspectSurface(ctx, a.SurfaceAdapter, transition)
	if err != nil {
		return observation, err
	}
	observation.SkillDiscovery, err = a.observe(ctx, transition, observation)
	return observation, err
}

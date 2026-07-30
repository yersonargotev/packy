package corelifecycle

// DecisionGuidanceView is the owner-provided risk, recovery, and retry policy
// for a classic lifecycle plan.
type DecisionGuidanceView struct {
	Risks       []string
	Recovery    []string
	NextCommand string
}

func (plan Plan) DecisionGuidance() DecisionGuidanceView {
	risks := make([]string, 0, len(plan.pending)+len(plan.blockers)+len(plan.warnings))
	for _, value := range plan.pending {
		risks = append(risks, "prerequisite: "+value)
	}
	for _, value := range plan.blockers {
		risks = append(risks, "blocker: "+value)
	}
	for _, value := range plan.warnings {
		risks = append(risks, "warning: "+value)
	}
	command := "packy " + string(plan.operation)
	nextCommand := command
	switch {
	case len(plan.blockers) != 0:
		nextCommand = "resolve blockers above, then run " + command
	case len(plan.recovery) != 0:
		nextCommand = "follow recovery guidance above, then run " + command
	}
	return DecisionGuidanceView{
		Risks:       risks,
		Recovery:    append([]string(nil), plan.recovery...),
		NextCommand: nextCommand,
	}
}

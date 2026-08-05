package capabilitypack

const ProjectLifecycleJSONSchemaVersion = 1

// JSONProjectFailure is the stable terminal event for a failed project
// lifecycle command. A preview already written to the event stream remains the
// complete description of the attempted plan.
type JSONProjectFailure struct {
	SchemaVersion     int    `json:"schema_version"`
	Report            string `json:"report"`
	Operation         string `json:"operation"`
	Stage             string `json:"stage"`
	Error             string `json:"error"`
	ActionsExecuted   *int   `json:"actions_executed,omitempty"`
	ApprovalRequested *bool  `json:"approval_requested,omitempty"`
}

// JSONProjectRecovery records the explicit recovery phase that completes
// before Packy previews new project intent.
type JSONProjectRecovery struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Operation     string `json:"operation"`
	Status        string `json:"status"`
	NextCommand   string `json:"next_command"`
}

func JSONProjectFailureFor(operation, stage string, err error) JSONProjectFailure {
	result := JSONProjectFailure{
		SchemaVersion: ProjectLifecycleJSONSchemaVersion,
		Report:        "project-lifecycle-failure",
		Operation:     operation,
		Stage:         stage,
		Error:         err.Error(),
	}
	if stage == "preview" || stage == "blocked" {
		no, zero := false, 0
		result.ApprovalRequested, result.ActionsExecuted = &no, &zero
	}
	return result
}

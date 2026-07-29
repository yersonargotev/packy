package packsync

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (plan Plan) CanonicalJSON() ([]byte, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (plan Plan) Human() string {
	var output strings.Builder
	fmt.Fprintf(&output, "source: %s\n", plan.SourceID)
	fmt.Fprintf(&output, "candidate: %s\n", plan.Candidate.Commit)
	if plan.Candidate.Release != nil {
		fmt.Fprintf(&output, "candidate_release: %s\n", plan.Candidate.Release.Tag)
	}
	fmt.Fprintf(&output, "source_lock: current=%s proposed=%s\n", plan.Preconditions.SourceLockSHA256, plan.SourceLockSHA256)
	fmt.Fprintf(&output, "plan_id: %s\n", plan.PlanID)
	fmt.Fprintf(&output, "status: %s\n", plan.Status)
	fmt.Fprintf(&output, "authoritative: %t\n", plan.Authoritative)
	fmt.Fprintf(&output, "resources: %d\nfiles: %d\n", plan.Counts.Resources, plan.Counts.Files)
	fmt.Fprintf(&output, "changes: added=%d removed=%d moved=%d modified=%d\n", plan.Counts.Added, plan.Counts.Removed, plan.Counts.Moved, plan.Counts.Modified)
	fmt.Fprintf(&output, "unselected discoveries: %d\n", plan.Counts.Discoveries)
	for _, change := range plan.Changes {
		fmt.Fprintf(&output, "  %s %s\n", change.Kind, change.Path)
	}
	for _, impact := range plan.AffectedPacks {
		fmt.Fprintf(&output, "affected_pack: %s current=%s floor=%s\n", impact.PackID, impact.CurrentVersion, impact.MechanicalFloor)
		for _, reason := range impact.Reasons {
			fmt.Fprintf(&output, "  reason: %s\n", reason)
		}
		if impact.Contract != nil {
			fmt.Fprintf(&output, "  manifest: schema=%d current_sha256=%s baseline_sha256=%s\n", impact.Contract.SchemaVersion, impact.Contract.CurrentManifestSHA256, impact.Contract.BaselineManifestSHA256)
			for _, migration := range impact.Contract.RootMigrations {
				fmt.Fprintf(&output, "  migration: %s -> %s\n", migration.From, migration.To)
			}
			for _, notice := range impact.Contract.NoticeAssociations {
				fmt.Fprintf(&output, "  notice: %s -> %s\n", notice.Resource, notice.Notice)
			}
		}
	}
	if len(plan.Blockers) > 0 {
		output.WriteString("blockers:\n")
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(&output, "  - %s\n", blocker)
		}
	}
	return output.String()
}

func seal(plan Plan) (string, error) {
	plan.PlanID = ""
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	return "pack-sync-" + hashBytes(data), nil
}

func (plan Plan) VerifySeal() bool {
	want, err := seal(plan)
	return err == nil && plan.PlanID == want
}

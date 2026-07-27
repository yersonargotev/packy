package packsyncworkflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/packy/internal/packsync"
)

// BundleReviewBrief is the v3-native, Pack-scoped authorization evidence
// rendered into the managed pull request. It never adapts the complete set to
// a member-wise v1/v2 dispatch.
type BundleReviewBrief struct {
	SchemaVersion           int                         `json:"schema_version"`
	Actor                   string                      `json:"actor"`
	RunID                   string                      `json:"run_id"`
	RunAttempt              string                      `json:"run_attempt"`
	RunURL                  string                      `json:"run_url"`
	Repository              string                      `json:"repository"`
	Request                 BundleDispatchRequest       `json:"request"`
	Identity                BundleArtifactIdentity      `json:"identity"`
	ClassificationSHA256    string                      `json:"classification_sha256"`
	HeadSHA                 string                      `json:"head_sha"`
	ResultTreeSHA           string                      `json:"result_tree_sha"`
	Branch                  string                      `json:"branch"`
	PullRequest             int                         `json:"pull_request,omitempty"`
	SelectedResources       []packsync.ResourceEvidence `json:"selected_resources"`
	PreviousSnapshotSHA256  string                      `json:"previous_snapshot_sha256"`
	ProposedSnapshotSHA256  string                      `json:"proposed_snapshot_sha256"`
	ApplyStatus             string                      `json:"apply_status"`
	Validation              ValidationGates             `json:"validation"`
	UpstreamContentExecuted bool                        `json:"upstream_content_executed"`
	Blockers                []string                    `json:"blockers"`
	DecisionReady           bool                        `json:"decision_ready"`
	AutoMerge               bool                        `json:"auto_merge"`
	ManualMergeRequired     bool                        `json:"manual_merge_required"`
	InvalidationConditions  []string                    `json:"invalidation_conditions"`
	Recovery                []string                    `json:"recovery"`
}

func (brief *BundleReviewBrief) PreparePublication(proposal Proposal) {
	brief.ResultTreeSHA = proposal.ResultTreeSHA
	brief.Validation = proposal.Validation
	brief.DecisionReady = false
	brief.Blockers = []string{"Publication remains blocked until the exact post-write pull request identity is reobserved."}
	brief.InvalidationConditions = proposal.InvalidationConditions
}

func (brief *BundleReviewBrief) FinalizePublication(proposal Proposal, observed PRState) {
	brief.PullRequest = observed.Number
	brief.HeadSHA = observed.HeadSHA
	brief.Validation = proposal.Validation
	brief.Blockers = nil
	brief.DecisionReady = true
	brief.InvalidationConditions = proposal.InvalidationConditions
}

func (brief BundleReviewBrief) CanonicalJSON() ([]byte, error) {
	repository, runID, validRun := parseActionsRunURL(brief.RunURL)
	if brief.SchemaVersion != 3 || brief.Request.Validate() != nil || brief.Identity.Validate() != nil ||
		brief.Request.PackID != brief.Identity.PackID ||
		brief.Request.RegistrationBundleSHA256 != brief.Identity.RegistrationBundleSHA256 ||
		brief.Request.ProposedVersion != brief.Identity.ProposedVersion ||
		brief.Request.ProposedManifestSHA256 != brief.Identity.ProposedManifestSHA256 ||
		requireSHA256("classification", brief.ClassificationSHA256) != nil ||
		requireFullSHA("head", brief.HeadSHA) != nil || requireFullSHA("result tree", brief.ResultTreeSHA) != nil ||
		brief.Branch != "sync/"+brief.Identity.PackID || len(brief.SelectedResources) == 0 ||
		requireSHA256("previous snapshot", brief.PreviousSnapshotSHA256) != nil ||
		requireSHA256("proposed snapshot", brief.ProposedSnapshotSHA256) != nil ||
		!brief.Validation.Complete() || brief.UpstreamContentExecuted || brief.AutoMerge || !brief.ManualMergeRequired ||
		!validRun || repository != brief.Repository || runID != brief.RunID {
		return nil, errors.New("v3 bundle review brief is incomplete or contradictory")
	}
	canonical, err := json.MarshalIndent(brief, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(canonical, '\n'), nil
}

func (brief BundleReviewBrief) Markdown() (string, error) {
	canonical, err := brief.CanonicalJSON()
	if err != nil {
		return "", err
	}
	status := "blocked"
	if brief.DecisionReady {
		status = "decision-ready"
	}
	return fmt.Sprintf("## Packy composite Pack registration\n\n- Pack: `%s`\n- Members: `%s`\n- Plan: `%s`\n- Base/head/tree: `%s` / `%s` / `%s`\n- State: **%s**\n- Auto-merge: disabled; manual merge required.\n\nAuthorization-Exception: automation\nAuthorization-Record: %s\n\n<details><summary>Canonical v3 composite admission evidence</summary>\n\n```json\n%s```\n</details>\n", brief.Identity.PackID, strings.Join(brief.Identity.SourceIDs, ", "), brief.Identity.PlanID, brief.Identity.BaseSHA, brief.HeadSHA, brief.ResultTreeSHA, status, brief.RunURL, string(canonical)), nil
}

// ManagedMarkdown keeps the pull request below GitHub's transport ceiling.
// Full v3 evidence remains available as the canonical run artifact.
func (brief BundleReviewBrief) ManagedMarkdown() (string, error) {
	if _, err := brief.CanonicalJSON(); err != nil {
		return "", err
	}
	status := "blocked"
	if brief.DecisionReady {
		status = "decision-ready"
	}
	return fmt.Sprintf("## Packy composite Pack registration\n\n- Pack: `%s`\n- Members: `%s`\n- Plan: `%s`\n- Base/head/tree: `%s` / `%s` / `%s`\n- State: **%s**\n- Complete canonical evidence: [workflow run](%s)\n- Auto-merge: disabled; manual merge required.\n\nAuthorization-Exception: automation\nAuthorization-Record: %s\n", brief.Identity.PackID, strings.Join(brief.Identity.SourceIDs, ", "), brief.Identity.PlanID, brief.Identity.BaseSHA, brief.HeadSHA, brief.ResultTreeSHA, status, brief.RunURL, brief.RunURL), nil
}

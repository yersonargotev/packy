package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

func TestGovernanceDriftContractAndSeededStates(t *testing.T) {
	root := repositoryRoot(t)
	var contract governancedrift.Contract
	data, err := os.ReadFile(filepath.Join(root, "docs", "governance", "expected-state.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{
		"actions-policy",
		"credential-metadata",
		"immutable-releases",
		"installed-app-authority",
		"latest-release",
		"main-protection",
		"protected-environments",
		"repository-settings",
		"residual-owner-authority",
		"tag-rules",
		"workflow-identities",
		"workflow-policy",
	}
	gotIDs := make([]string, 0, len(contract.Controls))
	observed := make([]governancedrift.ObservedControl, 0, len(contract.Controls))
	type credentialSecret struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	for _, control := range contract.Controls {
		gotIDs = append(gotIDs, control.ID)
		if control.ID == "credential-metadata" {
			var credentialMetadata struct {
				RepositoryActions struct {
					TotalCount int                `json:"total_count"`
					Secrets    []credentialSecret `json:"secrets"`
				} `json:"repository_actions"`
			}
			if err := json.Unmarshal([]byte(control.Expected), &credentialMetadata); err != nil {
				t.Fatal(err)
			}
			want := []credentialSecret{{
				Name:      "GOVERNANCE_READ_TOKEN",
				CreatedAt: "2026-07-24T16:36:34Z",
				UpdatedAt: "2026-07-24T16:36:34Z",
			}}
			if credentialMetadata.RepositoryActions.TotalCount != 1 ||
				!reflect.DeepEqual(credentialMetadata.RepositoryActions.Secrets, want) {
				t.Fatalf("repository governance credential metadata = %+v", credentialMetadata.RepositoryActions)
			}
		}
		observed = append(observed, governancedrift.ObservedControl{
			ID:     control.ID,
			State:  governancedrift.ObservationObserved,
			Actual: control.Expected,
		})
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("contract controls = %v, want %v", gotIDs, wantIDs)
	}
	sha := strings.Repeat("a", 40)
	observation := governancedrift.Observation{
		SchemaVersion: governancedrift.ObservationSchemaVersion,
		Identity: governancedrift.EvidenceIdentity{
			Repository:  "yersonargotev/packy",
			Ref:         "refs/heads/main",
			CommitSHA:   sha,
			WorkflowSHA: strings.Repeat("b", 40),
			CollectedAt: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC),
		},
		Controls: observed,
	}
	evaluation, err := governancedrift.Evaluate(contract, observation)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.State != governancedrift.StateClean {
		t.Fatalf("seeded expected state = %s", evaluation.State)
	}
}

func TestGovernanceDriftWorkflowSeparatesObservationReportingAndGates(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readFile(t, filepath.Join(root, ".github", "workflows", "governance-drift.yml"))
	for _, required := range []string{
		"name: Governance drift",
		"schedule:",
		"cron: '43 8 * * 1'",
		"permissions: {}",
		"name: Observe expected governance state",
		"actions: read",
		"contents: read",
		"collect-governance-drift.sh",
		"--mode evaluate",
		"Retain durable governance evidence",
		"name: Maintain canonical drift issue",
		"issues: write",
		"--mode issue-decision",
		"packy-governance-drift-v1",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("governance drift workflow missing %q", required)
		}
	}
	observe := strings.Split(strings.Split(workflow, "  observe:\n")[1], "\n  report:")[0]
	if strings.Contains(observe, "issues: write") {
		t.Fatal("read-only observer has issue mutation authority")
	}
	if !strings.Contains(observe, "GH_TOKEN: ${{ secrets.GOVERNANCE_READ_TOKEN }}") ||
		!strings.Contains(observe, "GH_METADATA_TOKEN: ${{ github.token }}") ||
		strings.Contains(observe, "GH_TOKEN: ${{ github.token }}") {
		t.Fatal("read-only observer must separate dedicated governance and built-in metadata credentials")
	}
	report := strings.Split(workflow, "\n  report:")[1]
	if !strings.Contains(report, "GH_TOKEN: ${{ github.token }}") ||
		strings.Contains(report, "secrets.GOVERNANCE_READ_TOKEN") {
		t.Fatal("issue reporter must retain only its narrow built-in token")
	}
	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"deployments: write",
		"packages: write",
		"pages: write",
		"secrets:",
		"security-advisories",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("governance drift workflow contains forbidden authority %q", forbidden)
		}
	}

	release := readFile(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	sync := readFile(t, filepath.Join(root, ".github", "workflows", "sync-pack-source.yml"))
	for _, check := range []struct {
		content  string
		boundary string
	}{
		{
			content:  workflowSection(t, release, "  governance-drift:", "  build:"),
			boundary: "--boundary publication",
		},
		{
			content:  workflowSection(t, sync, "  governance-drift:", "  inspect:"),
			boundary: "--boundary promotion",
		},
	} {
		if !strings.Contains(check.content, "gate-governance-drift.sh") ||
			!strings.Contains(check.content, check.boundary) ||
			!strings.Contains(check.content, "GH_TOKEN: ${{ secrets.GOVERNANCE_READ_TOKEN }}") ||
			!strings.Contains(check.content, "GH_METADATA_TOKEN: ${{ github.token }}") ||
			strings.Contains(check.content, "GH_TOKEN: ${{ github.token }}") {
			t.Fatalf("affected workflow lacks current fail-closed %s gate", check.boundary)
		}
	}
	if !strings.Contains(release, "needs: [normalize, governance-drift]") ||
		!strings.Contains(sync, "needs: [admit, governance-drift]") ||
		!strings.Contains(sync, "needs.governance-drift.result == 'success'") {
		t.Fatal("affected workflows do not block their first action on governance drift")
	}
	gate := readFile(t, filepath.Join(root, "scripts", "gate-governance-drift.sh"))
	for _, required := range []string{
		"--recovery-tag",
		"--mode recovery-contract",
		"--release-tag \"$recovery_tag\"",
		"--contract \"$contract\"",
	} {
		if !strings.Contains(gate, required) {
			t.Fatalf("recovery-aware governance gate missing %q", required)
		}
	}
}

func TestGovernanceDriftAdaptersContainNoSelfCorrectionPath(t *testing.T) {
	root := repositoryRoot(t)
	content := readFile(t, filepath.Join(root, "scripts", "collect-governance-drift.sh")) +
		readFile(t, filepath.Join(root, "scripts", "gate-governance-drift.sh")) +
		readFile(t, filepath.Join(root, "scripts", "project-governance-drift-issues.sh"))
	for _, forbidden := range []string{
		"--method",
		" api --input",
		" issue create",
		" issue edit",
		" issue close",
		" issue comment",
		"gh secret",
		"gh release",
		"gh variable",
		"git push",
		"PATCH",
		"POST",
		"PUT",
		"DELETE",
		"security-advisories",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("read-only adapter contains self-correction boundary %q", forbidden)
		}
	}
}

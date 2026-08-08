package packsyncworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestDispatchV23RequiresCompleteSingleSourceAdmissionIntent(t *testing.T) {
	registration := packsync.SourceConfig{
		ID: "orchestrate-source", Provider: "github", Repository: "example/orchestrate",
		Selector:  packsync.Selector{Mode: packsync.SelectorStableRelease},
		Resources: []packsync.Binding{{PackID: "orchestrate", Kind: "skill", ResourceID: "orchestrate", UpstreamPath: "orchestrate"}},
	}
	registrationDigest, err := CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	manifest, manifestDigest, err := CanonicalProposedManifest(json.RawMessage(`{"id":"orchestrate","version":"1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	request := DispatchRequest{
		SchemaVersion: 2, Operation: OperationRegister, SourceID: registration.ID,
		Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: "initial admission",
		Registration: &registration, RegistrationSHA256: registrationDigest,
		ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: manifestDigest,
		LegalAdmission: &packsync.CompositeLegalAdmission{EvidenceReference: "docs/evidence.json", EvidenceSHA256: strings.Repeat("a", 64), Disposition: packsync.RedistributableDisposition},
		ClosingIssue:   "https://github.com/example/packy/issues/544",
	}
	if err := request.Validate(); err != nil || !request.IsSingleSourceAdmission() {
		t.Fatalf("complete v2.3 dispatch rejected: initial=%v err=%v", request.IsSingleSourceAdmission(), err)
	}
	if err := ValidateIssueBoundSingleSourceAdmission(request); err != nil {
		t.Fatalf("issue-bound admission rejected: %v", err)
	}
	issueLess := request
	issueLess.ClosingIssue = ""
	if err := issueLess.Validate(); err != nil {
		t.Fatalf("immutable v2.3 dispatch contract changed: %v", err)
	}
	if err := ValidateIssueBoundSingleSourceAdmission(issueLess); err == nil {
		t.Fatal("private admission policy accepted a request without a closing issue")
	}
	partial := request
	partial.ProposedVersion = ""
	if err := partial.Validate(); err == nil {
		t.Fatal("partial initial admission dispatch accepted")
	}
	legacy := request
	legacy.ProposedVersion = ""
	legacy.ProposedManifest = nil
	legacy.ProposedManifestSHA256 = ""
	legacy.LegalAdmission = nil
	if err := legacy.Validate(); err != nil || legacy.IsSingleSourceAdmission() {
		t.Fatalf("legacy v2 register changed: initial=%v err=%v", legacy.IsSingleSourceAdmission(), err)
	}
}

func TestIssueBoundInitialAdmissionBriefUsesClosingIssueWithoutActionsIdentity(t *testing.T) {
	registration := packsync.SourceConfig{
		ID: "orchestrate-source", Provider: "github", Repository: "example/orchestrate",
		Selector:  packsync.Selector{Mode: packsync.SelectorStableRelease},
		Resources: []packsync.Binding{{PackID: "orchestrate", Kind: "skill", ResourceID: "orchestrate", UpstreamPath: "orchestrate"}},
	}
	registrationDigest, err := CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	manifest, manifestDigest, err := CanonicalProposedManifest(json.RawMessage(`{"id":"orchestrate","version":"1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	issue := "https://github.com/example/packy/issues/545"
	request := DispatchRequest{
		SchemaVersion: 2, Operation: OperationRegister, SourceID: registration.ID,
		Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: "initial admission",
		Registration: &registration, RegistrationSHA256: registrationDigest,
		ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: manifestDigest,
		LegalAdmission: &packsync.CompositeLegalAdmission{EvidenceReference: "docs/evidence.json", EvidenceSHA256: strings.Repeat("a", 64), Disposition: packsync.RedistributableDisposition},
		ClosingIssue:   issue,
	}
	brief := ReviewBrief{
		SchemaVersion: 1, RunURL: issue, Repository: "example/packy", Request: request,
		Candidate: packsync.Candidate{Commit: strings.Repeat("b", 40)}, PlanID: "plan", BaseSHA: strings.Repeat("c", 40), HeadSHA: strings.Repeat("d", 40), ResultTreeSHA: strings.Repeat("e", 40),
		Branch: "sync/orchestrate-source", SelectedResources: []packsync.ResourceEvidence{{SHA256: strings.Repeat("f", 64)}}, ProposedSnapshotSHA256: strings.Repeat("1", 64),
		ApplyStatus: "applied", Validation: ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true},
		DecisionReady: true, ManualMergeRequired: true, InvalidationConditions: DecisionReadyInvalidationConditions(),
	}
	canonical, err := brief.CanonicalJSON()
	if err != nil || !strings.Contains(string(canonical), `"run_url": "`+issue+`"`) {
		t.Fatalf("manual admission brief=%s err=%v", canonical, err)
	}
	managed, err := brief.ManagedMarkdown()
	if err != nil || !strings.Contains(managed, "Authorization-Record: "+issue) || !strings.Contains(managed, "[authorization record]("+issue+")") || strings.Contains(managed, "[workflow run]") {
		t.Fatalf("manual admission markdown=%q err=%v", managed, err)
	}
}

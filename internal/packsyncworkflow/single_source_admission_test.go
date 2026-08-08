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

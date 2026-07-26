package packsyncworkflow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestBundleDispatchRequiresCanonicalCompleteOrderedRegistrationSet(t *testing.T) {
	registrations := bundleRegistrations()
	digest, err := CanonicalRegistrationBundleSHA256("vercel", registrations)
	if err != nil {
		t.Fatal(err)
	}
	manifest := canonicalVercelManifest(t)
	manifestSum := sha256.Sum256(manifest)
	request := BundleDispatchRequest{SchemaVersion: 3, Operation: OperationRegisterBundle, PackID: "vercel", ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: fmt.Sprintf("%x", manifestSum), Registrations: registrations, RegistrationBundleSHA256: digest, ClassificationMode: ClassificationAI, RequestReason: "Admit the complete Pack."}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid bundle request: %v", err)
	}

	for name, mutate := range map[string]func(*BundleDispatchRequest){
		"v2 shape":      func(r *BundleDispatchRequest) { r.SchemaVersion = 2 },
		"single member": func(r *BundleDispatchRequest) { r.Registrations = r.Registrations[:1] },
		"stale seal":    func(r *BundleDispatchRequest) { r.RegistrationBundleSHA256 = strings.Repeat("0", 64) },
		"mixed pack":    func(r *BundleDispatchRequest) { r.Registrations[1].Registration.Resources[0].PackID = "other" },
		"unordered members": func(r *BundleDispatchRequest) {
			r.Registrations[0], r.Registrations[1] = r.Registrations[1], r.Registrations[0]
		},
		"moving selector": func(r *BundleDispatchRequest) {
			r.Registrations[0].Registration.Selector = packsync.Selector{Mode: packsync.SelectorStableRelease}
		},
		"missing legal grant": func(r *BundleDispatchRequest) { r.Registrations[0].LegalAdmission.Disposition = "private" },
		"mixed manifest pack": func(r *BundleDispatchRequest) {
			r.ProposedManifest = json.RawMessage(`{"id":"other","version":"1.0.0"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			changed.Registrations = bundleRegistrations()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("invalid v3 dispatch accepted")
			}
		})
	}
}

func TestV3DispatchCannotConsumeV1OrV2Fields(t *testing.T) {
	var request BundleDispatchRequest
	if err := json.Unmarshal([]byte(`{"schema_version":3,"operation":"register_bundle","pack_id":"vercel","source_id":"one","registrations":[],"registration_bundle_sha256":"`+strings.Repeat("1", 64)+`","classification_mode":"ai","request_reason":"bad"}`), &request); err == nil {
		t.Fatal("v3 request accepted a v1/v2 source_id field")
	}
	var legacy DispatchRequest
	if err := json.Unmarshal([]byte(`{"schema_version":3,"operation":"register_bundle","pack_id":"vercel","registrations":[],"registration_bundle_sha256":"`+strings.Repeat("1", 64)+`","classification_mode":"ai","request_reason":"bad"}`), &legacy); err == nil {
		t.Fatal("v1/v2 request accepted v3 fields")
	}
}

func TestV3HumanClassificationUsesInspectionThenExactEvidenceDispatch(t *testing.T) {
	registrations := bundleRegistrations()
	digest, err := CanonicalRegistrationBundleSHA256("vercel", registrations)
	if err != nil {
		t.Fatal(err)
	}
	manifest := canonicalVercelManifest(t)
	manifestSum := sha256.Sum256(manifest)
	request := BundleDispatchRequest{
		SchemaVersion: 3, Operation: OperationRegisterBundle, PackID: "vercel",
		ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: fmt.Sprintf("%x", manifestSum),
		Registrations: registrations, RegistrationBundleSHA256: digest,
		ClassificationMode: ClassificationHuman, RequestReason: "Inspect before human classification.",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid human inspection dispatch: %v", err)
	}

	evidence := request
	evidence.RequestReason = "Supply exact composite evidence."
	evidence.ExpectedPlanID = "bundle-plan"
	evidence.ExpectedBaseSHA = baseA
	evidence.HumanEvidence = json.RawMessage(`{"schema_version":1,"plan_id":"bundle-plan","pack_id":"vercel","evidence":{}}`)
	if err := evidence.Validate(); err != nil {
		t.Fatalf("valid human evidence dispatch: %v", err)
	}

	invalid := []BundleDispatchRequest{
		func() BundleDispatchRequest {
			value := evidence
			value.ClassificationMode = ClassificationAI
			return value
		}(),
		func() BundleDispatchRequest { value := evidence; value.ExpectedPlanID = ""; return value }(),
		func() BundleDispatchRequest { value := evidence; value.ExpectedBaseSHA = ""; return value }(),
		func() BundleDispatchRequest {
			value := evidence
			value.HumanEvidence = json.RawMessage(`{"schema_version":1,"plan_id":"other","pack_id":"vercel","evidence":{}}`)
			return value
		}(),
		func() BundleDispatchRequest {
			value := evidence
			value.HumanEvidence = json.RawMessage(`{"schema_version":1,"plan_id":"bundle-plan","pack_id":"other","evidence":{}}`)
			return value
		}(),
	}
	for _, value := range invalid {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid human dispatch accepted: %#v", value)
		}
	}
}

func TestV3ArtifactsRejectStaleAndMixedMembers(t *testing.T) {
	identity := bundleArtifactIdentity()
	artifacts := []interface{ Validate() error }{
		BundleInspectionArtifact{SchemaVersion: 3, BundleArtifactIdentity: identity},
		BundleClassificationArtifact{SchemaVersion: 3, BundleArtifactIdentity: identity, ClassificationSHA256: strings.Repeat("7", 64)},
		BundleValidationArtifact{SchemaVersion: 3, BundleArtifactIdentity: identity, ResultTreeSHA: treeA, Validation: completeValidationGates()},
		BundleFailureArtifact{SchemaVersion: 3, State: "blocked", BundleFailureIdentity: plannedBundleFailureIdentity(), Blockers: []string{"member failed"}, Recovery: []string{"fresh Inspect"}},
	}
	for _, artifact := range artifacts {
		if err := artifact.Validate(); err != nil {
			t.Fatalf("valid artifact %T: %v", artifact, err)
		}
	}

	stale := identity
	stale.Members = append([]BundleArtifactMember(nil), identity.Members...)
	stale.Members[1].CandidateSHA = candidateA
	if err := stale.Matches(identity); err == nil {
		t.Fatal("stale member candidate accepted")
	}
	mixed := identity
	mixed.SourceIDs = []string{"legal", "tools"}
	if err := mixed.Matches(identity); err == nil {
		t.Fatal("mixed source ids and member set accepted")
	}
	legacy := BundleInspectionArtifact{SchemaVersion: 2, BundleArtifactIdentity: identity}
	if err := legacy.Validate(); err == nil {
		t.Fatal("v3 artifact accepted a v2 schema")
	}
}

func TestV3FailureIdentityAllowsPreCheckButRejectsPartialPlan(t *testing.T) {
	precheck := BundleFailureArtifact{SchemaVersion: 3, State: "blocked", BundleFailureIdentity: precheckBundleFailureIdentity(), Blockers: []string{"acquisition failed"}, Recovery: []string{"fresh Inspect"}}
	if err := precheck.Validate(); err != nil {
		t.Fatalf("valid pre-Check failure: %v", err)
	}
	if _, err := precheck.PlannedIdentity(); err == nil {
		t.Fatal("pre-Check failure produced a plan identity")
	}

	partial := precheck
	partial.Plan = &BundleFailurePlanIdentity{PlanID: "partial"}
	if err := partial.Validate(); err == nil {
		t.Fatal("partial plan/result failure identity accepted")
	}

	planned := BundleFailureArtifact{SchemaVersion: 3, State: "blocked", BundleFailureIdentity: plannedBundleFailureIdentity(), Blockers: []string{"validation failed"}, Recovery: []string{"fresh Inspect"}}
	if err := planned.Validate(); err != nil {
		t.Fatalf("valid planned failure: %v", err)
	}
	if err := planned.Matches(bundleArtifactIdentity()); err != nil {
		t.Fatalf("planned failure did not match exact prior identity: %v", err)
	}
	planned.Plan.Members[1].CandidateSHA = candidateA
	if err := planned.Matches(bundleArtifactIdentity()); err == nil {
		t.Fatal("stale planned failure matched prior identity")
	}

	unsafe := precheck
	unsafe.ContainsSecrets = true
	if err := unsafe.Validate(); err == nil {
		t.Fatal("failure containing secrets accepted")
	}
}

func TestV3PublicationIdentityIsPackScoped(t *testing.T) {
	artifact := BundlePublicationArtifact{
		SchemaVersion: 3, BundleArtifactIdentity: bundleArtifactIdentity(),
		HeadSHA: headA, ResultTreeSHA: treeA, BranchName: "sync/vercel", PRNumber: 7, PRStateSHA256: strings.Repeat("8", 64),
		ProvenanceSHA256: strings.Repeat("9", 64), ManagedTitle: "register(vercel): composite",
		ManagedMetadataHash: strings.Repeat("a", 64), Validation: completeValidationGates(),
		DecisionReady: true, ManualMergeRequired: true, InvalidationConditions: DecisionReadyInvalidationConditions(),
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("valid publication: %v", err)
	}
	artifact.BranchName = "sync/runtime"
	if err := artifact.Validate(); err == nil {
		t.Fatal("source-scoped bundle publication accepted")
	}
}

func TestV3PreparationProvesReadOnlyNonAuthoritativeState(t *testing.T) {
	artifact := BundlePreparationArtifact{
		SchemaVersion: 3, BundleArtifactIdentity: bundleArtifactIdentity(),
		HeadSHA: headA, ResultTreeSHA: treeA, BranchName: "sync/vercel",
		ProvenanceSHA256: strings.Repeat("9", 64), ManagedTitle: "sync(vercel): composite registration",
		ManagedMetadataHash: strings.Repeat("a", 64), ObservedBaseSHA: baseA,
		Validation: completeValidationGates(), ObservationsStable: true,
	}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("valid preparation: %v", err)
	}
	for name, mutate := range map[string]func(*BundlePreparationArtifact){
		"mutated repository": func(a *BundlePreparationArtifact) { a.RepositoryMutated = true },
		"decision ready":     func(a *BundlePreparationArtifact) { a.DecisionReady = true },
		"stale base":         func(a *BundlePreparationArtifact) { a.ObservedBaseSHA = candidateA },
		"unstable state":     func(a *BundlePreparationArtifact) { a.ObservationsStable = false },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := artifact
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("invalid preparation claimed read-only proof")
			}
		})
	}
}

func bundleRegistrations() []BundleRegistration {
	return []BundleRegistration{
		{
			Registration:   packsync.SourceConfig{ID: "legal", Provider: "github", Repository: "example/legal", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: candidateA}, Resources: []packsync.Binding{{PackID: "vercel", Kind: "notice", ResourceID: "license", UpstreamPath: "LICENSE"}}},
			LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "https://example.test/legal", EvidenceSHA256: strings.Repeat("1", 64), Disposition: packsync.RedistributableDisposition},
		},
		{
			Registration:   packsync.SourceConfig{ID: "runtime", Provider: "github", Repository: "example/runtime", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: candidateB}, Resources: []packsync.Binding{{PackID: "vercel", Kind: "skill", ResourceID: "deploy", UpstreamPath: "skills/deploy"}}},
			LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "https://example.test/runtime", EvidenceSHA256: strings.Repeat("2", 64), Disposition: packsync.RedistributableDisposition},
		},
	}
}

func bundleArtifactIdentity() BundleArtifactIdentity {
	return BundleArtifactIdentity{
		PackID: "vercel", ProposedVersion: "1.0.0", ProposedManifestSHA256: strings.Repeat("c", 64), SourceIDs: []string{"legal", "runtime"}, RegistrationBundleSHA256: strings.Repeat("3", 64),
		Members: []BundleArtifactMember{
			{SourceID: "legal", CandidateSHA: candidateA, SourceLockSHA256: strings.Repeat("1", 64), LegalEvidenceRef: "https://example.test/legal", LegalEvidenceSHA256: strings.Repeat("2", 64)},
			{SourceID: "runtime", CandidateSHA: candidateB, SourceLockSHA256: strings.Repeat("4", 64), LegalEvidenceRef: "https://example.test/runtime", LegalEvidenceSHA256: strings.Repeat("5", 64)},
		},
		PlanID: "bundle-plan", BaseSHA: baseA, ConfigSHA256: strings.Repeat("6", 64), ManifestsSHA256: strings.Repeat("7", 64), LockSetSHA256: strings.Repeat("8", 64), ResultBundleSHA256: strings.Repeat("b", 64),
	}
}

func precheckBundleFailureIdentity() BundleFailureIdentity {
	return BundleFailureIdentity{
		PackID: "vercel", ProposedVersion: "1.0.0", ProposedManifestSHA256: strings.Repeat("c", 64), SourceIDs: []string{"legal", "runtime"}, RegistrationBundleSHA256: strings.Repeat("3", 64),
		Members: []BundleFailureMember{
			{SourceID: "legal", CandidateSHA: candidateA, LegalEvidenceRef: "https://example.test/legal", LegalEvidenceSHA256: strings.Repeat("2", 64)},
			{SourceID: "runtime", CandidateSHA: candidateB, LegalEvidenceRef: "https://example.test/runtime", LegalEvidenceSHA256: strings.Repeat("5", 64)},
		},
	}
}

func plannedBundleFailureIdentity() BundleFailureIdentity {
	identity := bundleArtifactIdentity()
	failure := precheckBundleFailureIdentity()
	failure.Plan = &BundleFailurePlanIdentity{
		PlanID: identity.PlanID, BaseSHA: identity.BaseSHA, ConfigSHA256: identity.ConfigSHA256,
		ManifestsSHA256: identity.ManifestsSHA256, LockSetSHA256: identity.LockSetSHA256,
		ResultBundleSHA256: identity.ResultBundleSHA256, Members: append([]BundleArtifactMember(nil), identity.Members...),
	}
	return failure
}

func completeValidationGates() ValidationGates {
	return ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true}
}

func canonicalVercelManifest(t *testing.T) json.RawMessage {
	t.Helper()
	manifest, err := packsync.CanonicalCompositePackManifest(json.RawMessage(`{"schema_version":4,"id":"vercel","version":"1.0.0","resources":[{"kind":"skill","id":"deploy","source":"skills/deploy"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

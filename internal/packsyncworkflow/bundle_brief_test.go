package packsyncworkflow

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestBundleReviewBriefBindsExactActionsRunAndCompleteV3Identity(t *testing.T) {
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
		ClassificationMode: ClassificationAI, RequestReason: "Admit complete Pack.",
	}
	identity := BundleArtifactIdentity{
		PackID: "vercel", SourceIDs: []string{"source-a", "source-b"},
		RegistrationBundleSHA256: digest, ProposedVersion: request.ProposedVersion,
		ProposedManifestSHA256: request.ProposedManifestSHA256,
		Members: []BundleArtifactMember{
			{SourceID: "source-a", CandidateSHA: baseA, SourceLockSHA256: strings.Repeat("1", 64), LegalEvidenceRef: "evidence/a", LegalEvidenceSHA256: strings.Repeat("2", 64)},
			{SourceID: "source-b", CandidateSHA: baseB, SourceLockSHA256: strings.Repeat("3", 64), LegalEvidenceRef: "evidence/b", LegalEvidenceSHA256: strings.Repeat("4", 64)},
		},
		PlanID: "bundle-plan", BaseSHA: baseA, ConfigSHA256: strings.Repeat("5", 64),
		ManifestsSHA256: strings.Repeat("6", 64), LockSetSHA256: strings.Repeat("7", 64),
		ResultBundleSHA256: strings.Repeat("8", 64),
	}
	brief := BundleReviewBrief{
		SchemaVersion: 3, Actor: "maintainer", RunID: "37", RunAttempt: "1",
		RunURL: "https://github.com/owner/repo/actions/runs/37", Repository: "owner/repo",
		Request: request, Identity: identity, ClassificationSHA256: strings.Repeat("9", 64),
		HeadSHA: baseB, ResultTreeSHA: baseB, Branch: "sync/vercel",
		SelectedResources:      []packsync.ResourceEvidence{{SHA256: strings.Repeat("a", 64)}},
		PreviousSnapshotSHA256: strings.Repeat("b", 64), ProposedSnapshotSHA256: identity.ResultBundleSHA256,
		ApplyStatus: "applied", Validation: completeValidationGates(), ManualMergeRequired: true,
	}
	if _, err := brief.Markdown(); err != nil {
		t.Fatalf("valid v3 review brief: %v", err)
	}
	brief.Repository = "owner/other"
	if _, err := brief.Markdown(); err == nil {
		t.Fatal("v3 review brief accepted a run URL for another repository")
	}
}

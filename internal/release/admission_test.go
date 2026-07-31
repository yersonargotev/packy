package release

import (
	"strings"
	"testing"
)

func TestAdmitReleaseClassifiesFreshDryRunAndRecovery(t *testing.T) {
	sha := func(s string) string { return strings.Repeat(s, 40) }
	base := AdmissionObservation{
		Repository: PackyRepository, Tag: "v0.4.0", TagCommit: sha("a"),
		EventCommit: sha("a"), CurrentMain: sha("a"), LatestVersion: "v0.3.9",
		ReleaseState: "absent",
	}
	fresh := base
	fresh.EventName, fresh.EventRef = "push", "refs/tags/v0.4.0"
	got, err := AdmitRelease(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AdmissionFresh || got.AttestationSourceRef != "refs/tags/v0.4.0" {
		t.Fatalf("fresh admission = %+v", got)
	}

	dry := base
	dry.EventName, dry.EventRef, dry.RequestedMode, dry.TagInMain = "workflow_dispatch", PackyMainRef, "dry-run", true
	if got, err = AdmitRelease(dry); err != nil || got.Mode != AdmissionDryRun {
		t.Fatalf("dry-run admission = %+v, %v", got, err)
	}

	recovery := dry
	recovery.RequestedMode, recovery.ReleasePresent = "recovery", true
	recovery.ReleaseState = "draft"
	recovery.ReleaseTag, recovery.ReleaseCommit = recovery.Tag, recovery.TagCommit
	recovery.ReleaseSchemaVersion = 1
	recovery.ReleaseCandidateID = strings.Repeat("c", 64)
	recovery.ReleaseAttestationSourceRef = "refs/tags/v0.4.0"
	recovery.OriginalRunID, recovery.CandidateLocator = "123", "candidate-abc"
	if got, err = AdmitRelease(recovery); err != nil {
		t.Fatal(err)
	}
	if got.Mode != AdmissionRecovery || got.OriginalRunID != "123" || got.CandidateLocator != "candidate-abc" {
		t.Fatalf("recovery admission = %+v", got)
	}
}

func TestAdmitReleaseRejectsUnsafeObservations(t *testing.T) {
	sha := strings.Repeat("a", 40)
	valid := AdmissionObservation{
		EventName: "push", EventRef: "refs/tags/v0.4.0", Repository: PackyRepository,
		Tag: "v0.4.0", TagCommit: sha, EventCommit: sha, CurrentMain: sha, LatestVersion: "v0.3.9",
		ReleaseState: "absent",
	}
	tests := map[string]func(*AdmissionObservation){
		"wrong repository":  func(o *AdmissionObservation) { o.Repository = "attacker/fork" },
		"malformed version": func(o *AdmissionObservation) { o.Tag = "v1.4.0" },
		"non-monotonic":     func(o *AdmissionObservation) { o.LatestVersion = "v0.4.0" },
		"mismatched event":  func(o *AdmissionObservation) { o.EventCommit = strings.Repeat("b", 40) },
		"occupied fresh":    func(o *AdmissionObservation) { o.ReleasePresent = true },
		"manual historical ref": func(o *AdmissionObservation) {
			o.EventName, o.EventRef, o.RequestedMode, o.TagInMain = "workflow_dispatch", PackyMainRef, "dry-run", false
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observed := valid
			mutate(&observed)
			if _, err := AdmitRelease(observed); err == nil {
				t.Fatal("unsafe observation admitted")
			}
		})
	}
}

func TestAdmitReleaseRejectsRecoveryWithoutExistingSealedRelease(t *testing.T) {
	sha := strings.Repeat("a", 40)
	observed := AdmissionObservation{
		EventName: "workflow_dispatch", EventRef: PackyMainRef, RequestedMode: "recovery",
		Repository: PackyRepository, Tag: "v0.4.0", TagCommit: sha, EventCommit: sha,
		CurrentMain: sha, LatestVersion: "v0.4.0", TagInMain: true, ReleaseState: "absent",
	}
	if _, err := AdmitRelease(observed); err == nil || !strings.Contains(err.Error(), "existing release") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdmitReleaseRequiresExactRecoveryLocator(t *testing.T) {
	sha := strings.Repeat("a", 40)
	valid := AdmissionObservation{
		EventName: "workflow_dispatch", EventRef: PackyMainRef, RequestedMode: "recovery",
		Repository: PackyRepository, Tag: "v0.4.0", TagCommit: sha, EventCommit: sha,
		CurrentMain: sha, LatestVersion: "v0.4.0", TagInMain: true,
		ReleasePresent: true, ReleaseState: "published", ReleaseTag: "v0.4.0",
		ReleaseCommit: sha, ReleaseSchemaVersion: 1, ReleaseCandidateID: strings.Repeat("c", 64),
		ReleaseAttestationSourceRef: "refs/tags/v0.4.0", OriginalRunID: "123",
		CandidateLocator: "packy-release-v0.4.0",
	}
	tests := map[string]func(*AdmissionObservation){
		"missing run":      func(o *AdmissionObservation) { o.OriginalRunID = "" },
		"zero run":         func(o *AdmissionObservation) { o.OriginalRunID = "0" },
		"leading zero run": func(o *AdmissionObservation) { o.OriginalRunID = "0123" },
		"non-decimal run":  func(o *AdmissionObservation) { o.OriginalRunID = "run-123" },
		"overflow run":     func(o *AdmissionObservation) { o.OriginalRunID = "18446744073709551616" },
		"missing locator":  func(o *AdmissionObservation) { o.CandidateLocator = "" },
		"unsafe locator":   func(o *AdmissionObservation) { o.CandidateLocator = "candidate $(unsafe)" },
		"long locator":     func(o *AdmissionObservation) { o.CandidateLocator = strings.Repeat("a", 129) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observed := valid
			mutate(&observed)
			if _, err := AdmitRelease(observed); err == nil {
				t.Fatal("ambiguous recovery locator admitted")
			}
		})
	}
}

func TestAdmitReleaseDerivesSealedRecoveryFromMetadataFacts(t *testing.T) {
	sha := strings.Repeat("a", 40)
	valid := AdmissionObservation{
		EventName: "workflow_dispatch", EventRef: PackyMainRef, RequestedMode: "recovery",
		Repository: PackyRepository, Tag: "v0.4.0", TagCommit: sha, EventCommit: sha,
		CurrentMain: sha, LatestVersion: "v0.4.0", TagInMain: true,
		ReleasePresent: true, ReleaseState: "published", ReleaseTag: "v0.4.0",
		ReleaseCommit: sha, ReleaseSchemaVersion: 1, ReleaseCandidateID: strings.Repeat("c", 64),
		ReleaseAttestationSourceRef: "refs/tags/v0.4.0", OriginalRunID: "123",
		CandidateLocator: "packy-release-v0.4.0",
	}
	if _, err := AdmitRelease(valid); err != nil {
		t.Fatalf("valid sealed metadata rejected: %v", err)
	}
	tests := map[string]func(*AdmissionObservation){
		"wrong schema":     func(o *AdmissionObservation) { o.ReleaseSchemaVersion = 2 },
		"empty candidate":  func(o *AdmissionObservation) { o.ReleaseCandidateID = "" },
		"wrong source ref": func(o *AdmissionObservation) { o.ReleaseAttestationSourceRef = PackyMainRef },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observed := valid
			mutate(&observed)
			if _, err := AdmitRelease(observed); err == nil {
				t.Fatal("unsealed recovery metadata admitted")
			}
		})
	}
}

func TestVerifyRefStateAllowsMainAdvancementAndRejectsMovedTag(t *testing.T) {
	releaseCommit, main := strings.Repeat("a", 40), strings.Repeat("b", 40)
	valid := RefStateObservation{
		Tag: "v0.4.0", ExpectedTagCommit: releaseCommit, RemoteTagCommit: releaseCommit,
		ReleaseCommit: releaseCommit, CurrentMain: main, ReleaseInMain: true,
	}
	got, err := VerifyRefState(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Verified || got.CurrentMain != main || got.ReleaseCommit != releaseCommit {
		t.Fatalf("verified state = %+v", got)
	}
	moved := valid
	moved.RemoteTagCommit = main
	if _, err := VerifyRefState(moved); err == nil {
		t.Fatal("moved tag verified")
	}
	outside := valid
	outside.ReleaseInMain = false
	if _, err := VerifyRefState(outside); err == nil {
		t.Fatal("release outside main verified")
	}
}

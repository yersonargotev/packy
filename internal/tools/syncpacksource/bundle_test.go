package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

type compositeAdapterLegalEvidence struct {
	SchemaVersion    int                              `json:"schema_version"`
	EvidenceID       string                           `json:"evidence_id"`
	DurableReference string                           `json:"durable_reference"`
	Issuer           string                           `json:"issuer"`
	EvidenceOrigin   string                           `json:"evidence_origin"`
	Decision         string                           `json:"decision"`
	Candidate        packsync.LegalAdmissionCandidate `json:"candidate"`
	Disposition      string                           `json:"disposition"`
	Rights           []string                         `json:"rights"`
	Obligations      []string                         `json:"obligations"`
	Disclosures      []string                         `json:"disclosures"`
	Scope            packsync.LegalAdmissionScope     `json:"scope"`
	Validity         string                           `json:"validity"`
	Invalidation     string                           `json:"invalidation"`
}

func writeAdapterLegalEvidence(t *testing.T, repository string, member packsyncworkflow.BundleRegistration) string {
	t.Helper()
	selected := make([]string, 0, len(member.Registration.Resources))
	for _, binding := range member.Registration.Resources {
		selected = append(selected, binding.UpstreamPath)
	}
	evidence := compositeAdapterLegalEvidence{
		SchemaVersion: 1, EvidenceID: "synthetic-" + member.Registration.ID,
		DurableReference: member.LegalAdmission.EvidenceReference,
		Issuer:           "Packy synthetic fixture", EvidenceOrigin: "issue-256 adapter tracer",
		Decision: "synthetic redistribution admitted",
		Candidate: packsync.LegalAdmissionCandidate{
			Repository: member.Registration.Repository, Commit: member.Registration.Selector.Ref,
			READMEBlob: strings.Repeat("c", 40), READMELength: 1, READMESHA256: strings.Repeat("d", 64),
		},
		Disposition: packsync.RedistributableDisposition,
		Rights:      []string{"copy"}, Obligations: []string{"preserve notice"}, Disclosures: []string{"synthetic fixture only"},
		Scope:    packsync.LegalAdmissionScope{SelectedRoots: selected, Exclusions: []string{}},
		Validity: "exact candidate and selected roots", Invalidation: "candidate, scope, or evidence digest changes",
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	target := filepath.Join(repository, filepath.FromSlash(member.LegalAdmission.EvidenceReference))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return sha256Text(string(raw))
}

type compositeAdapterSource struct {
	roots      map[string]string
	candidates map[string]packsync.Candidate
	resolves   map[string]int
}

func (s *compositeAdapterSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return nil, nil
}
func (s *compositeAdapterSource) ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error) {
	return packsync.Candidate{}, nil
}
func (s *compositeAdapterSource) ResolveCommit(_ context.Context, registration packsync.SourceConfig, _ string) (packsync.Candidate, error) {
	s.resolves[registration.ID]++
	return s.candidates[registration.ID], nil
}
func (s *compositeAdapterSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	target := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeErrorForTest(s.roots[candidate.Commit], target); err != nil {
		return err
	}
	err := visit(target)
	cleanup := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanup
}

func TestBundleDispatchDecodesV3EnvironmentWithoutCrossDecoding(t *testing.T) {
	clearPackyEnvironment(t)
	sha, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	var manifestValue any
	if err := json.Unmarshal([]byte(`{"schema_version":1,"id":"composite","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/one"},{"kind":"reference","id":"two","source":"references/two.md"}]}`), &manifestValue); err != nil {
		t.Fatal(err)
	}
	manifestBytes, _ := json.MarshalIndent(manifestValue, "", "  ")
	manifest := append(json.RawMessage(nil), append(manifestBytes, '\n')...)
	members := []packsyncworkflow.BundleRegistration{
		{Registration: packsync.SourceConfig{ID: "source-a", Provider: "github", Repository: "example/a", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: sha}, Resources: []packsync.Binding{{PackID: "composite", Kind: "skill", ResourceID: "one", UpstreamPath: "skill"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/a", EvidenceSHA256: hash, Disposition: packsync.RedistributableDisposition}},
		{Registration: packsync.SourceConfig{ID: "source-b", Provider: "github", Repository: "example/b", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: sha}, Resources: []packsync.Binding{{PackID: "composite", Kind: "reference", ResourceID: "two", UpstreamPath: "notice.md"}}}, LegalAdmission: packsync.CompositeLegalAdmission{EvidenceReference: "evidence/b", EvidenceSHA256: hash, Disposition: packsync.RedistributableDisposition}},
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationBundleSHA256("composite", members)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256Text(string(manifest))
	encoded, _ := json.Marshal(members)
	t.Setenv("PACKY_OPERATION", "register_bundle")
	t.Setenv("PACKY_PACK_ID", "composite")
	t.Setenv("PACKY_PROPOSED_VERSION", "1.0.0")
	t.Setenv("PACKY_PROPOSED_MANIFEST_JSON", string(manifest))
	t.Setenv("PACKY_PROPOSED_MANIFEST_SHA256", manifestDigest)
	t.Setenv("PACKY_REGISTRATIONS_JSON", string(encoded))
	t.Setenv("PACKY_REGISTRATION_BUNDLE_SHA256", registrationDigest)
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "synthetic tracer")
	request, err := bundleDispatch(options{})
	if err != nil {
		t.Fatal(err)
	}
	if request.SchemaVersion != 3 || request.PackID != "composite" || len(request.Registrations) != 2 {
		t.Fatalf("v3 environment dispatch = %#v", request)
	}

	v2 := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(v2, []byte(`{"schema_version":2,"operation":"synchronize","source_id":"source","selector":"latest-stable","classification_mode":"ai","request_reason":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := isBundleDispatch(options{requestPath: v2})
	if err != nil || bundle {
		t.Fatalf("v2 request crossed into v3 dispatch: bundle=%v err=%v", bundle, err)
	}
}

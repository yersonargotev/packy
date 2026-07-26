package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func clearPackyEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, "PACKY_") {
			continue
		}
		value := os.Getenv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("clear inherited %s: %v", name, err)
		}
		t.Cleanup(func() {
			if err := os.Setenv(name, value); err != nil {
				t.Errorf("restore inherited %s: %v", name, err)
			}
		})
	}
}

func TestV1AndV2FixturesIgnoreHostileValidV3Environment(t *testing.T) {
	repository := t.TempDir()
	copyTreeForTest(t, filepath.Join(repositoryRootForTest(t), "bundle"), filepath.Join(repository, "bundle"))
	manifest, err := vercelacceptance.CanonicalManifestJSON()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = packsync.CanonicalCompositePackManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repository, "bundle", "packs", "vercel", "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	setHostileValidV3Environment(t)
	t.Setenv("PACKY_FUTURE_DISPATCH_FIELD", "hostile")
	if request, err := bundleDispatch(options{}); err != nil || request.SchemaVersion != 3 {
		t.Fatalf("hostile v3 fixture is invalid: request=%#v err=%v", request, err)
	}
	err = run(
		context.Background(),
		[]string{"--phase", "inspect", "--repository-root", repository, "--output", t.TempDir()},
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), `target Pack "vercel" already exists`) {
		t.Fatalf("hostile v3 dispatch did not observe staged Vercel Pack: %v", err)
	}

	clearPackyEnvironment(t)
	if _, present := os.LookupEnv("PACKY_FUTURE_DISPATCH_FIELD"); present {
		t.Fatal("inherited PACKY_* entry survived fixture isolation")
	}
	if bundle, err := isBundleDispatch(options{}); err != nil || bundle {
		t.Fatalf("v1 fixture routed as bundle: bundle=%v err=%v", bundle, err)
	}
	v1, check, err := inspectRequest(options{repositoryRoot: repository, sourceID: "legacy-source"})
	if err != nil || v1.SchemaVersion != 1 || check.SourceID != "legacy-source" {
		t.Fatalf("v1 fixture routing: request=%#v check=%#v err=%v", v1, check, err)
	}

	t.Setenv("PACKY_SOURCE_ID", "source")
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "isolated v2 fixture")
	v2, check, err := inspectRequest(options{repositoryRoot: repository})
	if err != nil || v2.SchemaVersion != 2 || v2.Operation != packsyncworkflow.OperationSynchronize || check.SourceID != "source" {
		t.Fatalf("v2 fixture routing: request=%#v check=%#v err=%v", v2, check, err)
	}
}

func setHostileValidV3Environment(t *testing.T) {
	t.Helper()
	clearPackyEnvironment(t)
	fixture := vercelacceptance.Canonical()
	canonicalManifest, err := vercelacceptance.CanonicalManifestJSON()
	if err != nil {
		t.Fatal(err)
	}
	canonicalManifest, err = packsync.CanonicalCompositePackManifest(canonicalManifest)
	if err != nil {
		t.Fatal(err)
	}
	legal := map[string]packsync.CompositeLegalAdmission{
		"vercel-agent-skills":             {EvidenceReference: "docs/research/evidence/vercel-agent-skills-legal-admission.json", EvidenceSHA256: "e98ea93b2fc7ee5e4b49364ab0fc4e13fe4b0801d6439bd7e07180a7751e6dc3", Disposition: packsync.RedistributableDisposition},
		"vercel-web-interface-guidelines": {EvidenceReference: "docs/research/evidence/vercel-web-interface-guidelines-legal-admission.json", EvidenceSHA256: "f53f20a752db7bcb91f3ed1044fe1c4a49603599d9c25936994761994fcc8cc4", Disposition: packsync.RedistributableDisposition},
		"vercel-writing-guidelines":       {EvidenceReference: "docs/research/evidence/vercel-writing-guidelines-legal-admission.json", EvidenceSHA256: "0e6e060ab7a7b4980d671de99a2516f713ea3584be175af3bb28e9773aeb9966", Disposition: packsync.RedistributableDisposition},
	}
	members := make([]packsyncworkflow.BundleRegistration, 0, len(fixture.Sources.Sources))
	for _, registration := range fixture.Sources.Sources {
		members = append(members, packsyncworkflow.BundleRegistration{Registration: registration, LegalAdmission: legal[registration.ID]})
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationBundleSHA256("vercel", members)
	if err != nil {
		t.Fatal(err)
	}
	encodedMembers, err := json.Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"PACKY_OPERATION":                  "register_bundle",
		"PACKY_PACK_ID":                    "vercel",
		"PACKY_PROPOSED_VERSION":           "1.0.0",
		"PACKY_PROPOSED_MANIFEST_JSON":     string(canonicalManifest),
		"PACKY_PROPOSED_MANIFEST_SHA256":   sha256Text(string(canonicalManifest)),
		"PACKY_REGISTRATIONS_JSON":         string(encodedMembers),
		"PACKY_REGISTRATION_BUNDLE_SHA256": registrationDigest,
		"PACKY_CLASSIFICATION_MODE":        "ai",
		"PACKY_REQUEST_REASON":             "hostile inherited v3 fixture",
	} {
		t.Setenv(name, value)
	}
}

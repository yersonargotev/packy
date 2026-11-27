package catalogstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
)

const (
	demoRepository = "actions/attest-demo"
	demoWorkflow   = demoRepository + "/.github/workflows/build-python.yml"
	demoSubject    = "ae57936def59bc4c75edd3a837d89bcefc6d3a5e31d55a6fa7a71624f92c3c3b"
)

func readSigstoreFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/sigstore/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func pinnedGitHubTrustedRoot(t *testing.T) func(context.Context) (root.TrustedMaterial, error) {
	trusted, err := root.NewTrustedRootFromJSON(readSigstoreFixture(t, "github-trusted_root.json"))
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context) (root.TrustedMaterial, error) { return trusted, nil }
}

func unloadedTrustedRoot(t *testing.T, instance string) func(context.Context) (root.TrustedMaterial, error) {
	return func(context.Context) (root.TrustedMaterial, error) {
		t.Errorf("the %s trust root was loaded", instance)
		return nil, errors.New(instance + " trust root is unavailable")
	}
}

func TestGitHubInstanceVerifiesPrivateRepositoryProvenance(t *testing.T) {
	verifier := githubAttestationVerifier{instances: map[string]sigstoreInstance{
		"GitHub, Inc.": {trustedRoot: pinnedGitHubTrustedRoot(t), options: sigstoreInstances["GitHub, Inc."].options},
		"sigstore.dev": {trustedRoot: unloadedTrustedRoot(t, "public-good")},
	}}
	bundles := []json.RawMessage{readSigstoreFixture(t, "attest-demo.sigstore.json")}
	if err := verifier.Verify(context.Background(), bundles, demoRepository, demoSubject, demoWorkflow); err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	for _, test := range []struct{ name, repository, subject, workflow string }{
		{"another workflow", demoRepository, demoSubject, demoRepository + "/.github/workflows/other.yml"},
		{"another archive", demoRepository, strings.Repeat("0", 64), demoWorkflow},
		{"another repository", "actions/other", demoSubject, "actions/other/.github/workflows/build-python.yml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := verifier.Verify(context.Background(), bundles, test.repository, test.subject, test.workflow); !errors.Is(err, ErrNoAttestationVerified) {
				t.Fatalf("Verify() = %v, want a rejection", err)
			}
		})
	}
}

func TestAttestationVerifierSelectsTheInstanceByCertificateIssuer(t *testing.T) {
	demo := readSigstoreFixture(t, "attest-demo.sigstore.json")
	unavailable := errors.New("public-good trust root is unavailable")
	publicGoodLoads := 0
	verifier := githubAttestationVerifier{instances: map[string]sigstoreInstance{
		"GitHub, Inc.": {trustedRoot: pinnedGitHubTrustedRoot(t), options: sigstoreInstances["GitHub, Inc."].options},
		"sigstore.dev": {trustedRoot: func(context.Context) (root.TrustedMaterial, error) {
			publicGoodLoads++
			return nil, unavailable
		}},
	}}

	err := verifier.Verify(context.Background(), []json.RawMessage{withLeafIssuer(t, demo, "Example, Inc.")}, demoRepository, demoSubject, demoWorkflow)
	if !errors.Is(err, ErrNoAttestationVerified) || !strings.Contains(err.Error(), `issuer "Example, Inc." is not a Sigstore instance`) || publicGoodLoads != 0 {
		t.Fatalf("Verify() of an unknown issuer = %v after %d public-good loads", err, publicGoodLoads)
	}

	publicGood := withLeafIssuer(t, demo, "sigstore.dev")
	err = verifier.Verify(context.Background(), []json.RawMessage{publicGood, publicGood}, demoRepository, demoSubject, demoWorkflow)
	if !errors.Is(err, unavailable) || errors.Is(err, ErrNoAttestationVerified) || publicGoodLoads != 1 {
		t.Fatalf("Verify() without the public-good trust root = %v after %d loads", err, publicGoodLoads)
	}

	if err := verifier.Verify(context.Background(), []json.RawMessage{publicGood, demo}, demoRepository, demoSubject, demoWorkflow); err != nil {
		t.Fatalf("Verify() of a GitHub bundle after an unavailable instance = %v", err)
	}
}

// withLeafIssuer replaces the bundle's leaf certificate with a self-signed one
// that names organization as its issuer.
func withLeafIssuer(t *testing.T, raw []byte, organization string) json.RawMessage {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{Organization: []string{organization}}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["verificationMaterial"].(map[string]any)["certificate"] = map[string]any{"rawBytes": base64.StdEncoding.EncodeToString(certificate)}
	replaced, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return replaced
}

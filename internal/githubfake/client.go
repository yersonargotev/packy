package githubfake

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yersonargotev/packy/internal/catalogstore"
)

var routedHosts = map[string]bool{"api.github.com": true, AssetHost: true}

// Address locates a running fake and carries the certificate its clients trust.
type Address struct {
	URL         string `json:"url"`
	Certificate []byte `json:"certificate"`
}

// AddressOf returns the Address of a server started with httptest.NewTLSServer.
func AddressOf(server *httptest.Server) Address {
	return Address{URL: server.URL, Certificate: server.Certificate().Raw}
}

// Write records the address in a file that NewClient reads. The file appears
// complete, so a process that waits for it never reads a partial address.
func (a Address) Write(path string) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".addr-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// NewClient returns a client that sends GitHub API and asset hosts to the fake
// named by addressFile over TLS, and refuses every other host so that nothing
// reaches the real network. It reads addressFile on the first request.
func NewClient(addressFile string) *http.Client {
	return &http.Client{Timeout: 2 * time.Minute, Transport: &redirectTransport{load: sync.OnceValues(func() (*redirectTarget, error) {
		return loadTarget(addressFile)
	})}}
}

type redirectTransport struct {
	load func() (*redirectTarget, error)
}

type redirectTarget struct {
	host      string
	transport *http.Transport
}

func loadTarget(addressFile string) (*redirectTarget, error) {
	if addressFile == "" {
		return nil, errors.New("githubfake: address file is not configured")
	}
	data, err := os.ReadFile(addressFile)
	if err != nil {
		return nil, fmt.Errorf("githubfake: read address: %w", err)
	}
	var address Address
	if err := json.Unmarshal(data, &address); err != nil {
		return nil, fmt.Errorf("githubfake: decode address: %w", err)
	}
	target, err := url.Parse(address.URL)
	if err != nil {
		return nil, fmt.Errorf("githubfake: parse address: %w", err)
	}
	certificate, err := x509.ParseCertificate(address.Certificate)
	if err != nil {
		return nil, fmt.Errorf("githubfake: parse certificate: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return &redirectTarget{host: target.Host, transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}, nil
}

func (t *redirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || !routedHosts[request.URL.Host] {
		return nil, fmt.Errorf("githubfake: %s is not a routed GitHub host", request.URL.Redacted())
	}
	target, err := t.load()
	if err != nil {
		return nil, err
	}
	routed := request.Clone(request.Context())
	routed.Host = request.URL.Host
	routed.URL.Host = target.host
	return target.transport.RoundTrip(routed)
}

// AttestationVerifier stands in for Sigstore verification of GitHub build
// provenance. It accepts an archive when the fake serves an attestation for its
// digest that names the expected signer workflow.
type AttestationVerifier struct {
	client *http.Client
}

// NewAttestationVerifier returns a verifier that reads attestations through client.
func NewAttestationVerifier(client *http.Client) *AttestationVerifier {
	return &AttestationVerifier{client: client}
}

var _ catalogstore.AttestationVerifier = (*AttestationVerifier)(nil)

func (v *AttestationVerifier) Verify(ctx context.Context, repository, subjectSHA256, signerWorkflow string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repository+"/attestations/sha256:"+subjectSHA256, nil)
	if err != nil {
		return err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch fixture attestations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("fetch fixture attestations: GitHub returned %s", response.Status)
	}
	var wire struct {
		Attestations []struct {
			Bundle attestation `json:"bundle"`
		} `json:"attestations"`
	}
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return fmt.Errorf("decode fixture attestations: %w", err)
	}
	for _, candidate := range wire.Attestations {
		if candidate.Bundle.SubjectSHA256 == subjectSHA256 && candidate.Bundle.SignerWorkflow == signerWorkflow {
			return nil
		}
	}
	return fmt.Errorf("%w: no fixture attestation for %s was signed by %s", catalogstore.ErrNoAttestationVerified, repository, signerWorkflow)
}

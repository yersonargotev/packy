package catalogstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const githubOIDCIssuer = "https://token.actions.githubusercontent.com"

// NewGitHubAttestationVerifier constructs the production verifier for GitHub
// build provenance. Trust roots are refreshed from Sigstore for this explicit
// network operation and are not written into the user's configuration paths.
func NewGitHubAttestationVerifier(client *http.Client) AttestationVerifier {
	if client == nil {
		client = http.DefaultClient
	}
	return &githubAttestationVerifier{client: client}
}

type githubAttestationVerifier struct{ client *http.Client }

func (v *githubAttestationVerifier) Verify(ctx context.Context, repository, subjectSHA256, signerWorkflow string) error {
	digestBytes, err := hex.DecodeString(subjectSHA256)
	if err != nil || len(digestBytes) != sha256.Size {
		return errors.New("artifact attestation subject has an invalid SHA-256")
	}
	attestations, err := v.fetch(ctx, repository, subjectSHA256)
	if err != nil {
		return err
	}

	options := tuf.DefaultOptions()
	options.DisableLocalCache = true
	options.WithContext(ctx)
	client, err := tuf.New(options)
	if err != nil {
		return fmt.Errorf("initialize Sigstore trust metadata: %w", err)
	}
	trusted, err := root.GetTrustedRoot(client)
	if err != nil {
		return fmt.Errorf("load Sigstore trusted root: %w", err)
	}
	verifier, err := verify.NewVerifier(trusted,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
	if err != nil {
		return fmt.Errorf("initialize Sigstore verifier: %w", err)
	}
	sanPattern := "^" + regexp.QuoteMeta("https://github.com/"+signerWorkflow+"@refs/heads/main") + "$"
	identity, err := verify.NewShortCertificateIdentity(githubOIDCIssuer, "", "", sanPattern)
	if err != nil {
		return fmt.Errorf("construct catalog publisher identity: %w", err)
	}
	policy := verify.NewPolicy(verify.WithArtifactDigest("sha256", digestBytes), verify.WithCertificateIdentity(identity))
	expectedRepository := "https://github.com/" + repository
	var lastErr error
	for _, attestation := range attestations {
		result, verifyErr := verifier.Verify(attestation, policy)
		if verifyErr != nil {
			lastErr = verifyErr
			continue
		}
		cert := result.Signature.Certificate
		if cert == nil || !strings.EqualFold(cert.SourceRepositoryURI, expectedRepository) || cert.SourceRepositoryRef != "refs/heads/main" || cert.RunnerEnvironment != "github-hosted" {
			lastErr = errors.New("verified attestation does not match the official repository, main ref, and GitHub-hosted runner")
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no build-provenance attestations were returned")
	}
	return fmt.Errorf("no official catalog attestation verified: %w", lastErr)
}

func (v *githubAttestationVerifier) fetch(ctx context.Context, repository, digest string) ([]*bundle.Bundle, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repository+"/attestations/sha256:"+digest, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	response, err := v.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog artifact attestations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("fetch catalog artifact attestations: GitHub returned %s", response.Status)
	}
	var wire struct {
		Attestations []struct {
			Bundle json.RawMessage `json:"bundle"`
		} `json:"attestations"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode catalog artifact attestations: %w", err)
	}
	result := make([]*bundle.Bundle, 0, len(wire.Attestations))
	for _, candidate := range wire.Attestations {
		var parsed bundle.Bundle
		if err := parsed.UnmarshalJSON(candidate.Bundle); err != nil {
			continue
		}
		result = append(result, &parsed)
	}
	return result, nil
}

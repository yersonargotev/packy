package catalogstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const githubOIDCIssuer = "https://token.actions.githubusercontent.com"

// ErrNoAttestationVerified reports that a verifier read the catalog's
// attestations and none verified for the expected signer workflow. Transport
// and Sigstore trust-root failures do not wrap it.
var ErrNoAttestationVerified = errors.New("no catalog attestation verified")

// NewGitHubAttestationVerifier constructs the production verifier for GitHub
// build provenance. Trust roots are refreshed from Sigstore for this explicit
// network operation and are not written into the user's configuration paths.
func NewGitHubAttestationVerifier() AttestationVerifier {
	return githubAttestationVerifier{}
}

type githubAttestationVerifier struct{}

func (githubAttestationVerifier) Verify(ctx context.Context, bundles []json.RawMessage, repository, subjectSHA256, signerWorkflow string) error {
	digestBytes, err := hex.DecodeString(subjectSHA256)
	if err != nil || len(digestBytes) != sha256.Size {
		return errors.New("artifact attestation subject has an invalid SHA-256")
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
	workflow := strings.TrimPrefix(signerWorkflow, repository+"/")
	// Repository is lowercase, and the certificate keeps the repository's GitHub case.
	sanPattern := "^(?i:" + regexp.QuoteMeta("https://github.com/"+repository+"/") + ")" + regexp.QuoteMeta(workflow+"@refs/heads/main") + "$"
	identity, err := verify.NewShortCertificateIdentity(githubOIDCIssuer, "", "", sanPattern)
	if err != nil {
		return fmt.Errorf("construct catalog publisher identity: %w", err)
	}
	policy := verify.NewPolicy(verify.WithArtifactDigest("sha256", digestBytes), verify.WithCertificateIdentity(identity))
	expectedRepository := "https://github.com/" + repository
	var lastErr error
	for _, raw := range bundles {
		var attestation bundle.Bundle
		if err := attestation.UnmarshalJSON(raw); err != nil {
			continue
		}
		result, verifyErr := verifier.Verify(&attestation, policy)
		if verifyErr != nil {
			lastErr = verifyErr
			continue
		}
		cert := result.Signature.Certificate
		if cert == nil || !strings.EqualFold(cert.SourceRepositoryURI, expectedRepository) || cert.SourceRepositoryRef != "refs/heads/main" || cert.RunnerEnvironment != "github-hosted" {
			lastErr = errors.New("verified attestation does not match the catalog repository, main ref, and GitHub-hosted runner")
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no build-provenance attestations were returned")
	}
	return fmt.Errorf("%w: %w", ErrNoAttestationVerified, lastErr)
}

package catalogstore

import (
	"context"
	"crypto/sha256"
	_ "embed"
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

// ErrNoAttestationVerified reports that none of the attestation bundles passed
// to a verifier verified for the expected signer workflow. Sigstore trust-root
// failures do not wrap it.
var ErrNoAttestationVerified = errors.New("no catalog attestation verified")

//go:embed sigstore/tuf-repo.github.com/root.json
var githubTUFRoot []byte

type sigstoreInstance struct {
	trustedRoot func(context.Context) (root.TrustedMaterial, error)
	options     []verify.VerifierOption
}

// sigstoreInstances maps the issuer organization of an attestation's leaf
// certificate to the instance that signed it. GitHub signs attestations of
// private repositories with its own instance, which timestamps signatures
// with its own authority instead of a public transparency log.
var sigstoreInstances = map[string]sigstoreInstance{
	"sigstore.dev": {
		trustedRoot: func(ctx context.Context) (root.TrustedMaterial, error) {
			return trustedRootFromTUF(ctx, tuf.DefaultOptions())
		},
		options: []verify.VerifierOption{verify.WithSignedCertificateTimestamps(1), verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1)},
	},
	"GitHub, Inc.": {
		trustedRoot: func(ctx context.Context) (root.TrustedMaterial, error) {
			options := tuf.DefaultOptions().WithRoot(githubTUFRoot).WithRepositoryBaseURL("https://tuf-repo.github.com")
			return trustedRootFromTUF(ctx, options)
		},
		options: []verify.VerifierOption{verify.WithSignedTimestamps(1)},
	},
}

func trustedRootFromTUF(ctx context.Context, options *tuf.Options) (root.TrustedMaterial, error) {
	options.DisableLocalCache = true
	options.WithContext(ctx)
	client, err := tuf.New(options)
	if err != nil {
		return nil, fmt.Errorf("initialize Sigstore trust metadata from %s: %w", options.RepositoryBaseURL, err)
	}
	trusted, err := root.GetTrustedRoot(client)
	if err != nil {
		return nil, fmt.Errorf("load Sigstore trusted root from %s: %w", options.RepositoryBaseURL, err)
	}
	return trusted, nil
}

// NewGitHubAttestationVerifier constructs the production verifier for GitHub
// build provenance. It refreshes the trust root of each Sigstore instance that
// an attestation names, for this explicit network operation only, and writes
// nothing into the user's configuration paths.
func NewGitHubAttestationVerifier() AttestationVerifier {
	return githubAttestationVerifier{instances: sigstoreInstances}
}

type githubAttestationVerifier struct {
	instances map[string]sigstoreInstance
}

func (v githubAttestationVerifier) Verify(ctx context.Context, bundles []json.RawMessage, repository, subjectSHA256, signerWorkflow string) error {
	digestBytes, err := hex.DecodeString(subjectSHA256)
	if err != nil || len(digestBytes) != sha256.Size {
		return errors.New("artifact attestation subject has an invalid SHA-256")
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
	verifiers := map[string]*verify.Verifier{}
	var lastErr, trustErr error
	for _, raw := range bundles {
		var attestation bundle.Bundle
		if err := attestation.UnmarshalJSON(raw); err != nil {
			lastErr = fmt.Errorf("decode attestation bundle: %w", err)
			continue
		}
		issuer, err := leafIssuer(&attestation)
		if err != nil {
			lastErr = err
			continue
		}
		verifier, initialized := verifiers[issuer]
		if !initialized {
			instance, known := v.instances[issuer]
			if !known {
				lastErr = fmt.Errorf("attestation certificate issuer %q is not a Sigstore instance of GitHub build provenance", issuer)
				continue
			}
			verifier, err = instance.verifier(ctx)
			if err != nil {
				trustErr = err
			}
			verifiers[issuer] = verifier
		}
		if verifier == nil {
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
	if trustErr != nil {
		return trustErr
	}
	if lastErr == nil {
		lastErr = errors.New("no build-provenance attestations were returned")
	}
	return fmt.Errorf("%w: %w", ErrNoAttestationVerified, lastErr)
}

func (instance sigstoreInstance) verifier(ctx context.Context) (*verify.Verifier, error) {
	trusted, err := instance.trustedRoot(ctx)
	if err != nil {
		return nil, err
	}
	verifier, err := verify.NewVerifier(trusted, instance.options...)
	if err != nil {
		return nil, fmt.Errorf("initialize Sigstore verifier: %w", err)
	}
	return verifier, nil
}

func leafIssuer(attestation *bundle.Bundle) (string, error) {
	content, err := attestation.VerificationContent()
	if err != nil {
		return "", fmt.Errorf("read attestation verification material: %w", err)
	}
	certificate := content.Certificate()
	if certificate == nil {
		return "", errors.New("attestation is not signed with a certificate")
	}
	if len(certificate.Issuer.Organization) != 1 {
		return "", errors.New("attestation certificate issuer names no single organization")
	}
	return certificate.Issuer.Organization[0], nil
}

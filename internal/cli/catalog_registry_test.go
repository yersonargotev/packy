package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yersonargotev/packy/internal/catalogstore"
)

type failingVerifier struct{ err error }

func (v failingVerifier) Verify(context.Context, string, string, string) error { return v.err }

func TestRejectionMarkingVerifierMarksOnlyUnverifiedAttestations(t *testing.T) {
	unverified := fmt.Errorf("%w: signed by another workflow", catalogstore.ErrNoAttestationVerified)
	err := rejectionMarkingVerifier{failingVerifier{unverified}}.Verify(context.Background(), "acme/packs", "", "")
	var rejected attestationRejection
	if !errors.As(err, &rejected) || rejected.repository != "acme/packs" || !errors.Is(err, catalogstore.ErrNoAttestationVerified) {
		t.Fatalf("unverified attestation = %#v", err)
	}
	for _, fetch := range []error{
		errors.New("fetch catalog artifact attestations: GitHub returned 503 Service Unavailable"),
		fmt.Errorf("initialize Sigstore trust metadata: %w", context.DeadlineExceeded),
	} {
		err := rejectionMarkingVerifier{failingVerifier{fetch}}.Verify(context.Background(), "acme/packs", "", "")
		if errors.As(err, &rejected) || !errors.Is(err, fetch) {
			t.Fatalf("fetch failure %v was marked as a rejection: %#v", fetch, err)
		}
	}
}

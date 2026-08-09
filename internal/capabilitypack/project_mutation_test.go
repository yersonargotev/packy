package capabilitypack

import (
	"context"
	"errors"
	"testing"
)

func TestProjectMutationErrorReportsOnlyADomainVerifiedRollback(t *testing.T) {
	cause := errors.New("second project write failed")
	err := rollbackProjectMutation(context.Background(), &fakeSurfaceAdapter{}, nil, cause)
	var mutationErr ProjectMutationError
	if !errors.As(err, &mutationErr) || !mutationErr.RollbackVerified || !errors.Is(err, cause) {
		t.Fatalf("verified rollback error = %#v", err)
	}

	failed := rollbackProjectMutation(context.Background(), &fakeSurfaceAdapter{applyErr: errors.New("restore failed")}, []ProjectionAction{{ID: "restore"}}, cause)
	if errors.As(failed, &mutationErr) && mutationErr.RollbackVerified {
		t.Fatalf("failed rollback reported verification: %v", failed)
	}
}

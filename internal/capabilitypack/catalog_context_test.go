package capabilitypack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/catalogtransaction"
)

func TestCatalogListCurrentHonorsCancellationWhileWaitingForBundle(t *testing.T) {
	repository := t.TempDir()
	guard, err := catalogtransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = (Catalog{catalogRoot: repository}).ListCurrent(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListCurrent error = %v; want context deadline", err)
	}
}

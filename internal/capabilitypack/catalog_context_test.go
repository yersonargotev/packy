package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

func TestCatalogListCurrentHonorsCancellationWhileWaitingForBundle(t *testing.T) {
	repository := t.TempDir()
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = (Catalog{bundleRoot: filepath.Join(repository, "bundle")}).ListCurrent(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListCurrent error = %v; want context deadline", err)
	}
}

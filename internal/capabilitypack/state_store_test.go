package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileActivationStoreExplainsCompareAndSwapStaleRevision(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 1}}
	if err := store.Save(context.Background(), 0, state); err != nil {
		t.Fatal(err)
	}

	err := store.Save(context.Background(), 0, ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 2}})
	if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "changed from 0 to 1 before persistence") || !strings.Contains(err.Error(), "rerun activation") {
		t.Fatalf("error = %v", err)
	}
}

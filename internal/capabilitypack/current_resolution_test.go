package capabilitypack

import (
	"context"
	"strings"
	"testing"
)

func TestRetainedCatalogRejectsReceiptWithoutSnapshotIdentity(t *testing.T) {
	catalog := Catalog{snapshotID: strings.Repeat("a", 40)}
	_, err := catalog.resolveIntentPackAt(context.Background(), "legacy", "1.0.0", "")
	if err == nil || !strings.Contains(err.Error(), "predates Catalog Snapshots") {
		t.Fatalf("resolveIntentPackAt() error = %v", err)
	}
}

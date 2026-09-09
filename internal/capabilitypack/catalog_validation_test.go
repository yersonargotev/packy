package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

func TestValidatedCatalogRejectsSourceBeforeManifestDiscovery(t *testing.T) {
	root := t.TempDir()
	rejected := errors.New("unadmitted source")
	_, err := DiscoverValidatedForDurableIntents(context.Background(), filepath.Join(root, "bundle"), func(context.Context) error { return rejected })
	if !errors.Is(err, rejected) {
		t.Fatalf("discovery error = %v; want source rejection before missing catalog", err)
	}
}

func TestValidatedCatalogRevalidatesEveryObservation(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "packs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var validationErr error
	catalog, err := DiscoverValidatedForDurableIntents(context.Background(), bundle, func(context.Context) error { return validationErr })
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("source changed since discovery")
	validationErr = rejected
	for name, observe := range map[string]func() error{
		"list":    func() error { _, err := catalog.ListCurrent(context.Background()); return err },
		"details": func() error { _, err := catalog.ListDetails(context.Background()); return err },
		"show":    func() error { _, err := catalog.Show(context.Background(), "missing"); return err },
		"facade": func() error {
			_, err := withBundleObservation(context.Background(), NewFacade(catalog), func(Facade) (bool, error) { t.Fatal("invalid source reached facade"); return false, nil })
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := observe(); !errors.Is(err, rejected) {
				t.Fatalf("observation error = %v", err)
			}
		})
	}
}

func TestValidatedCatalogKeepsValidationAndConsumptionInOneTransaction(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "packs"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertLocked := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		guard, err := bundletransaction.Acquire(ctx, root)
		if guard != nil {
			guard.Release()
			t.Fatal("source replacement acquired observation lock")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("competing transaction error = %v", err)
		}
	}
	checks := 0
	catalog, err := DiscoverValidatedForDurableIntents(context.Background(), bundle, func(context.Context) error {
		checks++
		assertLocked()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = withBundleObservation(context.Background(), NewFacade(catalog), func(facade Facade) (bool, error) {
		assertLocked()
		if facade.catalog.validateSource == nil {
			t.Fatal("refresh dropped source validator")
		}
		if _, err := facade.catalog.ListCurrent(context.Background()); err != nil {
			return false, err
		}
		assertLocked()
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Fatalf("validated %d times; want once per outer observation", checks)
	}
	// The retained catalog must acquire and validate again on its next operation.
	if _, err := catalog.ListDetails(context.Background()); err != nil {
		t.Fatal(err)
	}
	if checks != 3 {
		t.Fatalf("next observation skipped validation: %d", checks)
	}
}

package engrambin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquirerReportsSupportedHomebrewAcquisition(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	resolution, err := NewAcquirer(prefix).ResolveAcquisition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Command != "brew" || !strings.Contains(strings.Join(resolution.Args, " "), Formula) {
		t.Fatalf("missing resolution = %+v", resolution)
	}
	if resolution.Path != filepath.Join(prefix, "bin", "engram") {
		t.Fatalf("missing path = %q", resolution.Path)
	}
}

func TestAcquirerSealsReadOnlyFormulaSourceAndVersion(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	calls := 0
	acquirer := NewAcquirer(prefix).WithFormulaInspector(
		func(_ context.Context, formula string) (FormulaMetadata, error) {
			calls++
			if formula != Formula {
				t.Fatalf("formula = %q", formula)
			}
			return FormulaMetadata{Source: Formula, Version: "0.4.2"}, nil
		},
	)
	resolution, err := acquirer.ResolveAcquisition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || resolution.Source != Formula || resolution.Version != "0.4.2" {
		t.Fatalf("sealed acquisition resolution = %+v calls=%d", resolution, calls)
	}
}

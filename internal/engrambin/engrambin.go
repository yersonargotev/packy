package engrambin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const (
	Formula        = "gentleman-programming/tap/engram"
	FormulaVersion = "0.4.2"
)

// Acquirer resolves the reviewed Homebrew acquisition facts for the explicit
// Engram executable-acquisition capability. PATH availability is observed
// separately by the generic tool observer.
type Acquirer struct {
	HomebrewPrefixEnv string
	FormulaInspector  func(context.Context, string) (FormulaMetadata, error)
}

type FormulaMetadata struct {
	Source  string
	Version string
}

func NewAcquirer(homebrewPrefixEnv string) Acquirer {
	return Acquirer{HomebrewPrefixEnv: homebrewPrefixEnv}
}

func (a Acquirer) WithFormulaInspector(inspector func(context.Context, string) (FormulaMetadata, error)) Acquirer {
	a.FormulaInspector = inspector
	return a
}

func (a Acquirer) ResolveAcquisition(ctx context.Context) (capabilitypack.ExecutableAcquisition, error) {
	acquisitionSupported := strings.TrimSpace(a.HomebrewPrefixEnv) != ""
	acquisitionCommand := ""
	var acquisitionArgs []string
	if acquisitionSupported {
		acquisitionCommand = "brew"
		acquisitionArgs = []string{"install", Formula}
	}
	acquisitionSource := Formula
	acquisitionVersion := FormulaVersion
	if acquisitionSupported && a.FormulaInspector != nil {
		metadata, err := a.FormulaInspector(ctx, Formula)
		if err != nil {
			return capabilitypack.ExecutableAcquisition{}, fmt.Errorf("inspect Homebrew formula %s: %w", Formula, err)
		}
		acquisitionSource = strings.TrimSpace(metadata.Source)
		acquisitionVersion = strings.TrimSpace(metadata.Version)
	}
	return capabilitypack.ExecutableAcquisition{
		Path:    filepath.Join(a.HomebrewPrefixEnv, "bin", "engram"),
		Command: acquisitionCommand,
		Args:    acquisitionArgs,
		Source:  acquisitionSource,
		Version: acquisitionVersion,
	}, nil
}

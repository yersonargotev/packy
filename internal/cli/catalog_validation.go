package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/cataloglayout"
)

type catalogValidationReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Report        string                  `json:"report"`
	Result        string                  `json:"result"`
	Packs         []catalogValidatedPack  `json:"packs,omitempty"`
	FitnessRows   int                     `json:"fitness_rows"`
	Error         *catalogValidationError `json:"error,omitempty"`
}

type catalogValidatedPack struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifest_sha256"`
	ClosureSHA256  string `json:"closure_sha256"`
	Files          int    `json:"files"`
}

type catalogValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newCatalogValidateCommand(opts Options) *cobra.Command {
	var project, baseline string
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		if jsonOutput {
			report := catalogValidationReport{SchemaVersion: 1, Report: "catalog-validation", Result: "invalid", Error: &catalogValidationError{Code: code, Message: err.Error()}}
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	command := &cobra.Command{
		Use: "validate", Short: "Validate a complete inert Catalog Project and optional baseline",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolver, cleanup, err := resolveCatalogOrigins(opts)
			if err != nil {
				return failure(cmd, "origin_setup_failed", err)
			}
			defer cleanup()
			validation, err := cataloglayout.ValidateCatalogProject(cmd.Context(), project, baseline, resolver)
			if err != nil {
				return failure(cmd, "validation_failed", err)
			}
			report := catalogValidationReport{SchemaVersion: 1, Report: "catalog-validation", Result: "valid", FitnessRows: validation.FitnessRows}
			for _, pack := range validation.Packs {
				report.Packs = append(report.Packs, catalogValidatedPack{ID: pack.Manifest.ID, Version: pack.Manifest.Version, ManifestSHA256: pack.ManifestSHA256, ClosureSHA256: pack.ClosureSHA256, Files: len(pack.Files)})
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "validated Catalog Project packs=%d fitness_rows=%d\n", len(report.Packs), report.FitnessRows); err != nil {
				return err
			}
			for _, pack := range report.Packs {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s@%s manifest_sha256=%s closure_sha256=%s files=%d\n", pack.ID, pack.Version, pack.ManifestSHA256, pack.ClosureSHA256, pack.Files); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&project, "project", ".", "Catalog Project root")
	command.Flags().StringVar(&baseline, "baseline", "", "baseline Catalog Project root for independent Pack version validation")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	configureCatalogArguments(command, &jsonOutput, failure)
	return command
}

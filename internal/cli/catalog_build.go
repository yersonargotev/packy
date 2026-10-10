package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

type catalogBuildReport struct {
	SchemaVersion int                                 `json:"schema_version"`
	Report        string                              `json:"report"`
	Result        string                              `json:"result"`
	Index         *cataloglayout.CatalogSnapshotIndex `json:"index,omitempty"`
	ArchivePath   string                              `json:"archive_path,omitempty"`
	ChecksumPath  string                              `json:"checksum_path,omitempty"`
	ArchiveSHA256 string                              `json:"archive_sha256,omitempty"`
	Error         *catalogValidationError             `json:"error,omitempty"`
}

func newCatalogBuildCommand(opts Options) *cobra.Command {
	var project, baseline, repository, sourceCommit, outputDir string
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		if jsonOutput {
			report := catalogBuildReport{SchemaVersion: 1, Report: "catalog-build", Result: "failed", Error: &catalogValidationError{Code: code, Message: err.Error()}}
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	command := &cobra.Command{
		DisableFlagParsing: true,
		Use:                "build", Short: "Build a complete validated Catalog Snapshot with this executable's identity",

		Args: func(cmd *cobra.Command, args []string) error {
			// Discover output mode across unknown flags before strict parsing can stop.
			// Both passes use pflag's actual flag/value rules, including the -- boundary.
			flags := cmd.Flags()
			flags.ParseErrorsAllowlist.UnknownFlags = true
			requestedJSON := false
			_ = flags.ParseAll(args, func(flag *pflag.Flag, value string) error {
				if flag.Name == "json" {
					if requested, err := strconv.ParseBool(value); err == nil {
						requestedJSON = requested
					}
				}
				return nil
			})
			flags.ParseErrorsAllowlist.UnknownFlags = false
			if err := flags.Parse(args); err != nil {
				jsonOutput = requestedJSON
				return failure(cmd, "invalid_arguments", err)
			}
			if help, _ := flags.GetBool("help"); help {
				return nil
			}
			if err := cobra.NoArgs(cmd, flags.Args()); err != nil {
				return failure(cmd, "invalid_arguments", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if help, _ := cmd.Flags().GetBool("help"); help {
				return cmd.Help()
			}
			resolver, cleanup, err := resolveCatalogOrigins(opts)
			if err != nil {
				return failure(cmd, "origin_setup_failed", err)
			}
			defer cleanup()
			validation, err := cataloglayout.ValidateCatalogProject(cmd.Context(), project, baseline, resolver)
			if err != nil {
				return failure(cmd, "validation_failed", err)
			}
			result, err := cataloglayout.BuildCatalogSnapshot(cmd.Context(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: repository, Commit: sourceCommit, Builder: "yersonargotev/packy@" + packyversion.SourceCommit}, outputDir)
			if err != nil {
				return failure(cmd, "build_failed", err)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(catalogBuildReport{SchemaVersion: 1, Report: "catalog-build", Result: "built", Index: &result.Index, ArchivePath: result.ArchivePath, ChecksumPath: result.ChecksumPath, ArchiveSHA256: result.ArchiveSHA256})
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "built Catalog Snapshot source=%s@%s builder=%s catalog_sha256=%s archive_sha256=%s packs=%d\n", result.Index.Source.Repository, result.Index.Source.Commit, result.Index.Builder, result.Index.CatalogSHA256, result.ArchiveSHA256, len(result.Index.Packs)); err != nil {
				return err
			}
			for _, pack := range result.Index.Packs {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s@%s manifest_sha256=%s closure_sha256=%s files=%d\n", pack.ID, pack.Version, pack.ManifestSHA256, pack.ClosureSHA256, len(pack.Files)); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&project, "project", ".", "Catalog Project root")
	command.Flags().StringVar(&baseline, "baseline", "", "baseline Catalog Project root for independent Pack version validation")
	command.Flags().StringVar(&repository, "source-repository", "", "Catalog Project owner/repository label")
	command.Flags().StringVar(&sourceCommit, "source-commit", "", "Catalog Project full commit label")
	command.Flags().StringVar(&outputDir, "out-dir", "dist", "new output directory")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return command
}

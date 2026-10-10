package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/catalogpublish"
)

type catalogPublicationReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Report        string                  `json:"report"`
	Result        string                  `json:"result"`
	Repository    string                  `json:"repository,omitempty"`
	Commit        string                  `json:"commit,omitempty"`
	Tag           string                  `json:"tag,omitempty"`
	ArchiveSHA256 string                  `json:"archive_sha256,omitempty"`
	Error         *catalogValidationError `json:"error,omitempty"`
}

func newCatalogPublishCommand() *cobra.Command {
	var request catalogpublish.Request
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		if jsonOutput {
			report := catalogPublicationReport{SchemaVersion: 1, Report: "catalog-publication", Result: "failed", Error: &catalogValidationError{Code: code, Message: err.Error()}}
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	command := &cobra.Command{
		Use: "publish", Short: "Publish or verify prepared immutable Catalog Snapshot assets",
		Long: "Publish prepared assets using authenticated gh on PATH. The official workflow owns reviewed preparation and artifact attestation; this command does not issue attestations.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := catalogpublish.Publish(cmd.Context(), request)
			if err != nil {
				code := "publication_failed"
				var publicationError *catalogpublish.Error
				if errors.As(err, &publicationError) {
					code = publicationError.Code
				}
				return failure(cmd, code, err)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(catalogPublicationReport{SchemaVersion: 1, Report: "catalog-publication", Result: result.Outcome, Repository: result.Repository, Commit: result.Commit, Tag: result.Tag, ArchiveSHA256: result.ArchiveSHA256})
			}
			messages := map[string]string{"published": "published immutable Catalog Snapshot", "resumed": "completed interrupted Catalog Snapshot publication", "unchanged": "Catalog Snapshot already published unchanged"}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s repository=%s archive_sha256=%s\n", messages[result.Outcome], result.Tag, result.Repository, result.ArchiveSHA256)
			return err
		},
	}
	command.Flags().StringVar(&request.Repository, "repository", "", "intended owner/repository")
	command.Flags().StringVar(&request.Commit, "commit", "", "intended full Catalog Project commit")
	command.Flags().StringVar(&request.Dist, "dist", "", "directory containing prepared catalog-snapshot.tar.gz and SHA256SUMS")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	configureCatalogArguments(command, &jsonOutput, failure)
	return command
}

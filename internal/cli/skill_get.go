package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/skillaccess"
	"github.com/yersonargotev/packy/internal/workstation"
)

type skillAccessReport struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Outcome       string `json:"result"`
	*skillaccess.Result
	Error *skillaccess.Error `json:"error,omitempty"`
}

func newGetCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	get := &cobra.Command{Use: "get", Short: "Obtain reviewed resources without installation"}
	var surface, snapshotID string
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		problem := &skillaccess.Error{Code: code, Message: err.Error()}
		var domainError *skillaccess.Error
		if errors.As(err, &domainError) {
			problem = domainError
		}
		if jsonOutput {
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(skillAccessReport{SchemaVersion: 1, Report: "skill-access", Outcome: "failed", Error: problem}); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	skill := &cobra.Command{
		Use: "skill <pack>:<skill>", Short: "Prepare one reviewed skill from the local Catalog Snapshot",
		Long: "Prepare a reviewed standalone skill tree for an explicit host, without installing or executing it. Uses local retained data only. Read the returned entry document, check prerequisites, and follow its procedure and approvals. Keep the target project and writable scratch/output separate from these immutable resource locations.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := capabilitypack.Surface(surface)
			if err := skillaccess.ValidateRequest(args[0], host); err != nil {
				return failure(cmd, "invalid_arguments", err)
			}
			station, err := resolver.Resolve(workstation.Options{})
			if err != nil {
				return failure(cmd, "catalog_unavailable", err)
			}
			dataRoot := catalogstore.DefaultDataRoot(station.Home())
			var root, identity string
			if opts.localCandidate != nil {
				identity = snapshotID
				if identity == "" {
					identity = opts.localCandidate.Snapshot.ID
				}
				root, err = opts.localCandidate.Resolve(cmd.Context(), identity)
			} else {
				store := catalogstore.New(dataRoot, nil)
				if snapshotID != "" {
					identity = snapshotID
					root, err = store.Resolve(identity)
				} else {
					var selected catalogstore.Snapshot
					selected, err = store.Selected()
					root, identity = selected.Root, selected.ID
				}
			}
			if err != nil {
				if snapshotID != "" {
					return failure(cmd, "snapshot_unavailable", fmt.Errorf("required retained snapshot %s is unavailable; restore its exact content, without substituting a newer revision: %w", snapshotID, err))
				}
				return failure(cmd, "catalog_unavailable", fmt.Errorf("local Catalog Snapshot unavailable; run `packy init` or `packy catalog refresh` to acquire it explicitly: %w", err))
			}
			result, err := skillaccess.Prepare(cmd.Context(), root, identity, dataRoot, args[0], host)
			if err != nil {
				return failure(cmd, "preparation_failed", err)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(skillAccessReport{SchemaVersion: 1, Report: "skill-access", Outcome: "prepared", Result: &result})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Prepared skill %s (Pack %s, surface %s)\nCatalog Snapshot: %s\nEntry document: %s\nResource root: %s\nKnown prerequisites (not verified): %v\nResources only: no native registration or workflow execution. Check prerequisites and follow the skill's procedure and approvals; use separate target and writable scratch/output locations.\n", result.QualifiedSkill, result.PackVersion, result.Surface, result.SnapshotID, result.EntryDocument, result.ResourceRoot, result.Prerequisites)
			if err != nil {
				return err
			}
			for _, dependency := range result.Dependencies {
				if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Dependency: %s (Pack %s, surface %s, snapshot %s)\n  Entry document: %s\n  Resource root: %s\n  Known prerequisites (not verified): %v\n", dependency.QualifiedSkill, dependency.PackVersion, dependency.Surface, dependency.SnapshotID, dependency.EntryDocument, dependency.ResourceRoot, dependency.Prerequisites); err != nil {
					return err
				}
			}
			return nil
		},
	}
	skill.Flags().StringVar(&surface, "surface", "", "explicit host: codex or claude")
	skill.Flags().StringVar(&snapshotID, "snapshot", "", "reuse this exact retained Catalog Snapshot instead of the selected revision")
	skill.Flags().BoolVar(&jsonOutput, "json", false, "Emit versioned skill-access JSON")
	configureCatalogArguments(skill, &jsonOutput, failure)
	get.AddCommand(skill)
	return get
}

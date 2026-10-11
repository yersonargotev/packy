package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/catalogcandidate"
	"github.com/yersonargotev/packy/internal/claudecode"
)

func newCatalogCandidateCommand(opts Options) *cobra.Command {
	var assetsRoot, workspaceRoot string
	command := &cobra.Command{
		Use:   "candidate --snapshot <assets-directory> --workspace <new-directory> -- <operation> [args...]",
		Short: "Exercise a local candidate in an isolated authoring workspace",
		Long:  "Exercise local snapshot assets without official publisher authority. Only list, show, project install, verify, and uninstall are supported. A new disposable workspace is required; reuse is limited to the same candidate bytes. Normal interactive approval is required for Apply; JSON is never consent. Host execution and personal/global activation are unavailable.",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 0 || len(args) == 0 {
				return errors.New("candidate requires -- followed by a supported operation")
			}
			switch args[0] {
			case "list", "show", "install", "verify", "uninstall", "update", "transition":
				return nil
			default:
				return fmt.Errorf("unsupported candidate operation %q; use list, show, install, verify, or uninstall", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "transition" {
				return executeCandidateTransition(cmd, assetsRoot, workspaceRoot, args[1:])
			}
			workspace, err := catalogcandidate.Open(cmd.Context(), assetsRoot, workspaceRoot)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Local authoring candidate: snapshot=%s archive_sha256=%s; not an official Catalog Publication; project=%s\n", workspace.Snapshot.ID, workspace.ArchiveSHA256, workspace.Project); err != nil {
				return err
			}
			isolated := opts
			isolated.Env = MapEnv{"HOME": workspace.Home, "XDG_CONFIG_HOME": workspace.ConfigurationHome}
			isolated.Getwd = func() (string, error) { return workspace.Project, nil }
			isolated.Runner = candidateRunner{}
			isolated.ClaudeRunner = claudeRunner{runner: isolated.Runner}
			isolated.ClaudeLookPath = claudecode.LookPath(isolated.Runner.LookPath)
			isolated.localCandidate = &workspace
			isolated.catalogRootOverride = ""
			resolver := newWorkstationResolver(isolated)
			root := &cobra.Command{Use: "candidate", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(newPackListCommand(isolated, resolver), newPackShowCommand(isolated, resolver), newPackInstallCommand(isolated, resolver), newProjectVerifyCommand(isolated.Getwd), newPackUninstallCommand(isolated, resolver), newPackUpdateCommand(isolated, resolver))
			root.SetIn(cmd.InOrStdin())
			root.SetOut(cmd.OutOrStdout())
			root.SetErr(cmd.ErrOrStderr())
			root.SetArgs(args)
			return root.ExecuteContext(cmd.Context())
		},
	}
	command.Flags().StringVar(&assetsRoot, "snapshot", "", "absolute directory containing catalog-snapshot.tar.gz and SHA256SUMS")
	command.Flags().StringVar(&workspaceRoot, "workspace", "", "absolute disposable workspace path (must not exist on first use)")
	_ = command.MarkFlagRequired("snapshot")
	_ = command.MarkFlagRequired("workspace")
	return command
}

type candidateTransitionReport struct {
	SchemaVersion         int                     `json:"schema_version"`
	Report                string                  `json:"report"`
	Result                string                  `json:"result"`
	Provenance            string                  `json:"provenance"`
	SnapshotID            string                  `json:"snapshot_id,omitempty"`
	ArchiveSHA256         string                  `json:"archive_sha256,omitempty"`
	PreviousArchiveSHA256 string                  `json:"previous_archive_sha256,omitempty"`
	Error                 *catalogValidationError `json:"error,omitempty"`
}

func executeCandidateTransition(parent *cobra.Command, assetsRoot, workspaceRoot string, args []string) error {
	var expected string
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		if jsonOutput {
			report := candidateTransitionReport{SchemaVersion: 1, Report: "catalog-candidate-transition", Result: "failed", Provenance: "local-candidate", Error: &catalogValidationError{Code: code, Message: err.Error()}}
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	command := &cobra.Command{Use: "transition", SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, _ []string) error {
		workspace, err := catalogcandidate.Transition(cmd.Context(), assetsRoot, workspaceRoot, expected)
		if err != nil {
			return failure(cmd, "transition_failed", err)
		}
		report := candidateTransitionReport{SchemaVersion: 1, Report: "catalog-candidate-transition", Result: "transitioned", Provenance: "local-candidate", SnapshotID: workspace.Snapshot.ID, ArchiveSHA256: workspace.ArchiveSHA256, PreviousArchiveSHA256: expected}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Local authoring candidate transitioned: snapshot=%s archive_sha256=%s; not an official Catalog Publication\n", report.SnapshotID, report.ArchiveSHA256)
		return err
	}}
	command.Flags().StringVar(&expected, "expect-archive-sha256", "", "observed current candidate archive digest")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	configureCatalogArguments(command, &jsonOutput, failure)
	command.SetIn(parent.InOrStdin())
	command.SetOut(parent.OutOrStdout())
	command.SetErr(parent.ErrOrStderr())
	command.SetArgs(args)
	return command.ExecuteContext(parent.Context())
}

type candidateRunner struct{}

func (candidateRunner) LookPath(name string) (string, error) {
	return "", fmt.Errorf("host executable %q is unavailable in candidate workspaces", name)
}

func (candidateRunner) Run(context.Context, string, ...string) error {
	return errors.New("host execution is unavailable in candidate workspaces")
}

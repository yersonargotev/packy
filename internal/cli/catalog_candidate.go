package cli

import (
	"context"
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
			case "list", "show", "install", "verify", "uninstall":
				return nil
			default:
				return fmt.Errorf("unsupported candidate operation %q; use list, show, install, verify, or uninstall", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
			isolated.localCandidate = &workspace.Snapshot
			isolated.catalogRootOverride = ""
			resolver := newWorkstationResolver(isolated)
			root := &cobra.Command{Use: "candidate", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(newPackListCommand(isolated, resolver), newPackShowCommand(isolated, resolver), newPackInstallCommand(isolated, resolver), newProjectVerifyCommand(isolated.Getwd), newPackUninstallCommand(isolated, resolver))
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

type candidateRunner struct{}

func (candidateRunner) LookPath(name string) (string, error) {
	return "", fmt.Errorf("host executable %q is unavailable in candidate workspaces", name)
}

func (candidateRunner) Run(context.Context, string, ...string) error {
	return errors.New("host execution is unavailable in candidate workspaces")
}

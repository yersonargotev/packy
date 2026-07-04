package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.0.0-dev"

// Options carries injectable process boundaries for tests and future command
// implementations. The zero value uses the real OS environment and runner.
type Options struct {
	Env    Env
	Runner Runner
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	return o
}

// NewRootCommand constructs the Matty CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()

	root := &cobra.Command{
		Use:           "matty",
		Short:         "Install and configure the Matty AI coding workflow",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newInstallCommand(opts),
		newDoctorCommand(opts),
		newUpdateCommand(opts),
		newUninstallCommand(opts),
	)

	return root
}

func newInstallCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Matty-managed global workflow configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			if dryRun {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty install dry-run: would use state %s and skills %s\n", paths.StateFile, paths.AgentSkillsDir)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty install scaffold: state %s\n", paths.StateFile)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed changes without writing files")
	return cmd
}

func newDoctorCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check Matty setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "HOME=%s\nCONFIG_HOME=%s\nMATTY_STATE=%s\nAGENT_SKILLS=%s\n", paths.HomeDir, paths.ConfigHome, paths.StateFile, paths.AgentSkillsDir)
			return err
		},
	}
}

func newUpdateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Refresh Matty-managed tools and configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty update scaffold: state %s\n", paths.StateFile)
			return err
		},
	}
}

func newUninstallCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove only Matty-managed artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty uninstall scaffold: state %s\n", paths.StateFile)
			return err
		},
	}
}

package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/skillbundle"
	packyversion "github.com/yersonargotev/packy/internal/version"
	"github.com/yersonargotev/packy/internal/workstation"
)

// Options carries injectable process boundaries for tests and future command
// implementations. The zero value uses the real OS environment and runner.
type Options struct {
	Env                   Env
	Getwd                 func() (string, error)
	Runner                Runner
	Clock                 func() time.Time
	Terminal              Terminal
	SurfaceAdapters       map[capabilitypack.Surface]capabilitypack.SurfaceAdapter
	EngramFacts           engrambin.Facts
	SetupHealthDiagnose   func() (setuphealth.Report, error)
	ClaudeRunner          claudecode.Runner
	ClaudeLookPath        claudecode.LookPath
	ClaudeAuthorization   claudecode.AuthorizationObserver
	ClaudeRuntimeEvidence claudecode.RuntimeEvidenceObserver
}

func (o Options) withDefaults() Options {
	runnerInjected := o.Runner != nil
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.ClaudeRunner == nil {
		if runnerInjected {
			o.ClaudeRunner = claudeRunner{runner: o.Runner}
		} else {
			o.ClaudeRunner = execClaudeRunner{}
		}
	}
	if o.ClaudeLookPath == nil {
		o.ClaudeLookPath = claudecode.LookPath(o.Runner.LookPath)
	}
	if o.Getwd == nil {
		o.Getwd = os.Getwd
	}
	if o.Clock == nil {
		o.Clock = time.Now
	}
	o.EngramFacts = o.EngramFacts.WithDefaults()
	if o.Terminal == nil {
		o.Terminal = processTerminal{}
	}
	return o
}

// NewRootCommand constructs the Packy CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()
	workstationResolver := newWorkstationResolver(opts)

	root := &cobra.Command{
		Use:           "packy",
		Short:         "Manage Packy capability packs and sources",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       packyversion.Value,
	}

	root.AddCommand(
		newVersionCommand(),
		newPackCommand(opts, workstationResolver),
		newInitCommand(opts, workstationResolver),
		newDoctorCommand(opts, workstationResolver),
	)

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Packy version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy version %s\n", packyversion.Value)
			return err
		},
	}
}

func newInitCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var (
		homeFlag      string
		sourceRoot    string
		repositoryURL string
		repositoryRef string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Packy's package-installed source checkout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := workstationResolver.Resolve(workstation.Options{Home: strings.TrimSpace(homeFlag)})
			if err != nil {
				return err
			}
			installedSource, err := bootstrap.ResolveInstalledSource(snapshot, sourceRoot)
			if err != nil {
				return err
			}

			result, err := bootstrap.EnsureInstalledSource(bootstrap.BootstrapOptions{
				InstalledSource: installedSource,
				RepositoryURL:   repositoryURL,
				RepositoryRef:   defaultInitRepositoryRef(repositoryRef, packyversion.Value),
				HomeDir:         snapshot.Home(),
				ConfigHome:      snapshot.ConfigurationHome(),
				ReportProgress: func(message string) error {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy init: %s\n", message)
					return err
				},
			})
			if err != nil {
				return err
			}
			switch {
			case result.Cloned:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: initialized Installed Source at %s\n", installedSource.Root())
			case result.Updated:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: updated Installed Source at %s\n", installedSource.Root())
			default:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: Installed Source already initialized at %s\n", installedSource.Root())
			}
			return err
		},
	}

	cmd.Flags().StringVar(&homeFlag, "home", "", "home directory used to resolve the default Installed Source")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "Installed Source root (default ~/.local/share/packy)")
	cmd.Flags().StringVar(&repositoryURL, "repository-url", bootstrap.DefaultRepositoryURL, "Packy Source of Truth Git URL")
	cmd.Flags().StringVar(&repositoryRef, "repository-ref", "", "optional Packy Source of Truth Git ref to clone or check out")
	return cmd
}

func newWorkstationResolver(opts Options) *workstation.Resolver {
	return workstation.NewResolver(func() (workstation.Inputs, error) {
		cwd, err := opts.Getwd()
		return workstation.Inputs{
			Home:                 opts.Env.Getenv("HOME"),
			ConfigurationHome:    opts.Env.Getenv("XDG_CONFIG_HOME"),
			ExecutableSearchPath: opts.Env.Getenv("PATH"),
			HomebrewPrefix:       opts.Env.Getenv("HOMEBREW_PREFIX"),
			CurrentDirectory:     cwd,
			CurrentDirectoryErr:  err,
		}, nil
	})
}

func defaultInitRepositoryRef(explicitRef, currentVersion string) string {
	if strings.TrimSpace(explicitRef) != "" {
		return explicitRef
	}
	if strings.HasPrefix(currentVersion, "v") {
		return currentVersion
	}
	return ""
}

func newDoctorCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Packy setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				report setuphealth.Report
				err    error
			)
			if opts.SetupHealthDiagnose != nil {
				report, err = opts.SetupHealthDiagnose()
			} else {
				report, err = diagnoseSetupHealth(cmd.Context(), opts, workstationResolver)
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := renderSetupHealthJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else if err := renderSetupHealthHuman(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			return setupHealthError(report)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func diagnoseSetupHealth(ctx context.Context, opts Options, resolver *workstation.Resolver) (setuphealth.Report, error) {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return setuphealth.Report{}, err
	}
	store := capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(snapshot.PackyHome()).File())
	intentObservation := capabilitypack.ObserveActiveIntents(ctx, store)
	observation := setuphealth.Observation{}
	for _, surface := range intentObservation.FailedSurfaces {
		observation.FailedStateSurfaces = append(observation.FailedStateSurfaces, string(surface))
	}
	if len(intentObservation.Intents) == 0 {
		return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
	}
	facade, err := activationFacade(opts, resolver)
	if err != nil {
		observation.ActivePacks = failedActivePackObservations(intentObservation.Intents)
		return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
	}
	status, err := facade.ActiveStatus(ctx)
	if err != nil {
		observation.ActivePacks = failedActivePackObservations(intentObservation.Intents)
		return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
	}
	for _, surface := range status.ObservationFailures {
		value := string(surface)
		if !slices.Contains(observation.FailedStateSurfaces, value) {
			observation.FailedStateSurfaces = append(observation.FailedStateSurfaces, value)
		}
	}
	observation.ActivePacks = make([]setuphealth.ActivePack, 0, len(status.Entries))
	for _, entry := range status.Entries {
		observation.ActivePacks = append(observation.ActivePacks, setuphealth.ActivePack{
			ID:                  entry.Pack.ID,
			Surface:             string(entry.Surface),
			InspectionFailed:    entry.InspectionFailed,
			RecoveryRequired:    entry.LifecycleState == capabilitypack.PackLifecycleRecoveryRequired,
			UpdateAvailable:     entry.UpdateAvailable,
			ProjectionProblems:  entry.Projections.Missing + entry.Projections.Drifted + entry.Projections.Ambiguous + entry.Projections.Unmanaged,
			MissingRequirements: len(entry.MissingRequirements),
			ReadinessPending:    !entry.Readiness.Configured || !entry.Readiness.Authorized || !entry.Readiness.Usable,
			PendingHumanActions: len(entry.PendingHumanActions),
		})
	}
	return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
}

func failedActivePackObservations(intents []capabilitypack.ActivationIntent) []setuphealth.ActivePack {
	result := make([]setuphealth.ActivePack, 0, len(intents))
	for _, intent := range intents {
		result = append(result, setuphealth.ActivePack{ID: intent.PackID, Surface: string(intent.Surface), InspectionFailed: true})
	}
	return result
}

type invocationSources struct {
	installed bootstrap.InstalledSource
	skills    skillbundle.Source
}

func resolveInvocationSources(opts Options, snapshot workstation.Snapshot) (invocationSources, error) {
	installed, err := bootstrap.ResolveInstalledSource(snapshot, "")
	if err != nil {
		return invocationSources{}, err
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return invocationSources{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	skills, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    opts.Env.Getenv("PACKY_SKILLS_SOURCE"),
		RepositoryStart: currentDirectory,
		InstalledSource: installed,
	})
	if err != nil {
		return invocationSources{}, err
	}
	return invocationSources{installed: installed, skills: skills}, nil
}

package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
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
	Env                    Env
	Getwd                  func() (string, error)
	Runner                 Runner
	Clock                  func() time.Time
	Terminal               Terminal
	SurfaceAdapters        map[capabilitypack.Surface]capabilitypack.SurfaceAdapter
	EngramFormulaInspector func(context.Context, string) (engrambin.FormulaMetadata, error)
	SetupHealthDiagnose    func() (setuphealth.Report, error)
	ClaudeRunner           claudecode.Runner
	ClaudeLookPath         claudecode.LookPath
	ClaudeAuthorization    claudecode.AuthorizationObserver
	ClaudeRuntimeEvidence  claudecode.RuntimeEvidenceObserver
	TUIRunner              func(context.Context, Options, io.Reader, io.Writer) error
	CatalogSource          catalogstore.Source
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
	if o.Terminal == nil {
		o.Terminal = processTerminal{}
	}
	if o.TUIRunner == nil {
		o.TUIRunner = RunTUI
	}
	return o
}

// NewRootCommand constructs the Packy CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()
	workstationResolver := newWorkstationResolver(opts)

	root := &cobra.Command{
		Use:   "packy",
		Short: "Packy is an installer and configurator for reviewed capability Packs",
		Long: `Packy is an installer and configurator for reviewed capability Packs on Claude Code, Codex, and OpenCode.

Lifecycle commands preview an immutable plan before interactive Apply. Approvals
are requested separately for each consent kind. A verified Apply can succeed while
login, trust, permissions, reload, or runtime loading remain pending; use targeted
status with --require usable as the separate automation gate.

After a stale plan, repeat the original lifecycle verb to inspect fresh state
and receive a new Preview. Packy never retries it automatically.

` + projectLifecycleHelp,
		Example: `  packy list
  packy audit --json
  packy show matty
  packy show engram --json
  packy status
  packy status engram --surface claude
  packy status engram --surface claude --require usable --json
  packy status engram --surface codex
  packy status engram --surface codex --require usable
  packy check orchestrate --surface codex --dry-run
  packy check orchestrate --surface codex --result positive
  packy install matty --surface codex --dry-run
  packy uninstall matty --dry-run
  packy activate matty --surface codex --dry-run
  packy activate example-pack --surface codex --resource skill:ask-matt --dry-run
  packy deactivate example-pack --surface codex --resource skill:ask-matt --dry-run
  packy activate engram --surface claude --dry-run --json
  packy activate matty --surface codex
  packy update matty --surface codex
  packy deactivate matty --surface codex`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       packyversion.Value,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !opts.Terminal.InteractiveSession(cmd.InOrStdin(), cmd.OutOrStdout()) {
				return cmd.Help()
			}
			return opts.TUIRunner(cmd.Context(), opts, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}

	root.AddCommand(
		newVersionCommand(),
		newInitCommand(opts, workstationResolver),
		newCatalogCommand(opts, workstationResolver),
		newDoctorCommand(opts, workstationResolver),
		newAuditCommand(opts, workstationResolver),
		newPackListCommand(opts, workstationResolver),
		newPackShowCommand(opts, workstationResolver),
		newProjectVerifyCommand(opts.Getwd),
		newPackStatusCommand(opts, workstationResolver),
		newControlledCheckCommand(opts, workstationResolver),
		newPackInstallCommand(opts, workstationResolver),
		newPackUninstallCommand(opts, workstationResolver),
		newPackActivateCommand(opts, workstationResolver),
		newPackUpdateCommand(opts, workstationResolver),
		newPackDeactivateCommand(opts, workstationResolver),
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
	var homeFlag string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Packy's official Catalog Snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return initializeCatalog(cmd.Context(), opts, workstationResolver, initializationRequest{
				Home: strings.TrimSpace(homeFlag),
				ReportProgress: func(message string) error {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy init: %s\n", message)
					return err
				},
			})
		},
	}

	cmd.Flags().StringVar(&homeFlag, "home", "", "home directory used to store the official Catalog Snapshot")
	return cmd
}

type initializationRequest struct {
	Home           string
	ReportProgress func(string) error
}

func initializeCatalog(ctx context.Context, opts Options, resolver *workstation.Resolver, request initializationRequest) error {
	snapshot, err := resolver.Resolve(workstation.Options{Home: request.Home})
	if err != nil {
		return err
	}
	result, err := newCatalogStore(opts, snapshot).AcquireLatest(ctx)
	if err != nil {
		return err
	}
	if request.ReportProgress != nil {
		return request.ReportProgress(fmt.Sprintf("selected official Catalog Snapshot %s", result.ID))
	}
	return nil
}

func newCatalogCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	command := &cobra.Command{Use: "catalog", Short: "Manage the official reviewed Pack catalog", Args: cobra.NoArgs}
	command.AddCommand(&cobra.Command{
		Use: "refresh", Short: "Acquire and select the latest official Catalog Snapshot", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return initializeCatalog(cmd.Context(), opts, resolver, initializationRequest{ReportProgress: func(message string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy catalog refresh: %s\n", message)
				return err
			}})
		},
	})
	return command
}

func newCatalogStore(opts Options, snapshot workstation.Snapshot) *catalogstore.Store {
	source := opts.CatalogSource
	if source == nil {
		client := &http.Client{Timeout: 2 * time.Minute}
		source = catalogstore.NewGitHubSource(client, catalogstore.NewGitHubAttestationVerifier(client))
	}
	return catalogstore.New(catalogstore.DefaultDataRoot(snapshot.Home()), source)
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
	facade, err := activationFacade(ctx, opts, resolver)
	if err != nil {
		observation.CatalogFailure = err.Error()
		observation.ActivePacks = failedActivePackObservations(intentObservation.Intents)
		return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
	}
	status, err := facade.ActiveStatus(ctx)
	if err != nil {
		observation.CatalogFailure = err.Error()
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
			ID:                        entry.Pack.ID,
			Surface:                   string(entry.Surface),
			InspectionFailed:          entry.InspectionFailed,
			HistoricalEvidenceMessage: entry.HistoricalEvidence.Message,
			IntentVersion:             entry.Intent.Version, CatalogVersion: entry.Pack.Version,
			UpdateAvailable:         entry.UpdateAvailable,
			UpdateActionUnavailable: entry.UpdateAvailable && !entry.UpdateActionAvailable,
			ProjectionProblems:      entry.Projections.Missing + entry.Projections.Drifted + entry.Projections.Ambiguous + entry.Projections.Unmanaged,
			MissingRequirements:     len(entry.MissingRequirements),
			PendingHumanActions:     len(entry.PendingHumanActions),
			Conditions:              setupHealthConditions(entry.Conditions),
			ControlledCheckState:    string(entry.ControlledCheck.State), ControlledCheckResult: string(entry.ControlledCheck.Result), ControlledCheckObserved: entry.ControlledCheck.ObservedAt, ControlledCheckIdentity: entry.ControlledCheck.ValidityIdentity,
		})
	}
	return setuphealth.Diagnose(snapshot.Home(), snapshot.ConfigurationHome(), observation), nil
}

func setupHealthConditions(conditions []capabilitypack.ReadinessCondition) []setuphealth.ReadinessCondition {
	result := make([]setuphealth.ReadinessCondition, 0, len(conditions))
	for _, condition := range conditions {
		result = append(result, setuphealth.ReadinessCondition{
			Type:      string(condition.Type),
			Dimension: string(condition.Dimension),
			Value:     string(condition.Value),
			Reason:    string(condition.Reason),
			Message:   condition.Message,
		})
	}
	return result
}

func failedActivePackObservations(intents []capabilitypack.ActivationIntent) []setuphealth.ActivePack {
	result := make([]setuphealth.ActivePack, 0, len(intents))
	for _, intent := range intents {
		result = append(result, setuphealth.ActivePack{ID: intent.PackID, Surface: string(intent.Surface), InspectionFailed: true})
	}
	return result
}

type invocationSources struct {
	skills   skillbundle.Source
	snapshot catalogstore.Snapshot
	store    *catalogstore.Store
}

func resolveInvocationSources(ctx context.Context, opts Options, snapshot workstation.Snapshot) (invocationSources, error) {
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return invocationSources{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	explicit := strings.TrimSpace(opts.Env.Getenv("PACKY_SKILLS_SOURCE"))
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(currentDirectory, explicit)
		}
		return invocationSources{skills: skillbundle.Source{Root: filepath.Clean(explicit)}}, nil
	}
	store := newCatalogStore(opts, snapshot)
	selected, err := store.Selected()
	if err != nil {
		return invocationSources{}, fmt.Errorf("official Catalog Snapshot is unavailable; run `packy init`: %w", err)
	}
	return invocationSources{
		skills:   skillbundle.Source{Root: selected.SkillRoot(), IsDefault: true},
		snapshot: selected,
		store:    store,
	}, nil
}

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
	"github.com/yersonargotev/packy/internal/workstation"
)

func newPackCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Discover and manage capability packs",
		Long: `Discover and manage opt-in capability packs independently on Claude Code, Codex, and OpenCode.

Lifecycle commands preview an immutable plan before interactive Apply. Approvals
are requested separately for each consent kind. A verified Apply can succeed while
login, trust, permissions, reload, or runtime loading remain pending; use targeted
status with --require usable as the separate automation gate.

After a stale plan or recovery-required attempt, repeat the original lifecycle
verb to inspect fresh state and receive a new Preview. Packy never retries it
automatically.`,
		Example: `  packy pack list
  packy pack show matty
  packy pack show engram --json
  packy pack status
  packy pack status engram --surface claude
  packy pack status engram --surface claude --require usable --json
  packy pack status engram --surface codex
  packy pack status engram --surface codex --require usable
  packy pack install matty --surface codex --dry-run
  packy pack uninstall matty --dry-run
  packy pack activate matty --surface codex --dry-run
  packy pack activate example-pack --surface codex --resource skill:ask-matt --dry-run
  packy pack deactivate example-pack --surface codex --resource skill:ask-matt --dry-run
  packy pack activate engram --surface claude --dry-run --json
  packy pack activate matty --surface codex
  packy pack update matty --surface codex
  packy pack reconcile matty --surface codex
  packy pack reconcile --surface codex
  packy pack deactivate matty --surface codex`,
	}
	cmd.AddCommand(newPackListCommand(opts, workstationResolver), newPackShowCommand(opts, workstationResolver), newPackStatusCommand(opts, workstationResolver), newPackInstallCommand(opts, workstationResolver), newPackUninstallCommand(opts, workstationResolver), newPackActivateCommand(opts, workstationResolver), newPackUpdateCommand(opts, workstationResolver), newPackDeactivateCommand(opts, workstationResolver), newPackReconcileCommand(opts, workstationResolver))
	return cmd
}

func newPackUninstallCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "uninstall <pack>",
		Short: "Uninstall exact owned projections from the current Git project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := workstationResolver.Resolve(workstation.Options{})
			if err != nil {
				return err
			}
			cwd, err := snapshot.CurrentDirectory()
			if err != nil {
				return fmt.Errorf("resolve current directory: %w", err)
			}
			projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
			if err != nil {
				return err
			}
			adapter := projectOfflineAdapter(capabilitypack.Surface(surface))
			pendingRecovery, err := capabilitypack.ProjectInstallRecoveryPending(snapshot.PackyHome(), projectRoot)
			if err != nil {
				return err
			}
			if dryRun && pendingRecovery {
				return errors.New("project installation is recovery-required; rerun `packy pack uninstall` without --dry-run before requesting another preview")
			}
			if !dryRun {
				recovered, recoveryErr := capabilitypack.NewFacade(capabilitypack.Catalog{}).RecoverProjectInstall(cmd.Context(), capabilitypack.ProjectInstallRecoveryRequest{ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter})
				if recoveryErr != nil {
					return recoveryErr
				}
				if recovered && !jsonOutput {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Recovered the prior project mutation before preview"); err != nil {
						return err
					}
				}
			}
			report, err := capabilitypack.PreviewProjectUninstall(cmd.Context(), capabilitypack.ProjectUninstallRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot}, adapter)
			if err != nil {
				return err
			}
			if jsonOutput {
				outputReport := report
				outputReport.DryRun = dryRun
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(outputReport); err != nil {
					return err
				}
			} else if err := renderProjectUninstallPreview(cmd, report, dryRun); err != nil {
				return err
			}
			if report.Disposition == capabilitypack.ProjectInstallBlocked {
				return capabilitypack.ProjectUninstallNotActionableError{Disposition: report.Disposition}
			}
			if dryRun {
				return nil
			}
			if !opts.Terminal.Interactive(cmd.InOrStdin()) {
				return capabilitypack.ErrInteractiveRequired
			}
			prompt := fmt.Sprintf("Approve project surface uninstall for exact preview %s?", report.Observation)
			if report.Scope == capabilitypack.ProjectUninstallPack {
				prompt = fmt.Sprintf("Approve complete project pack uninstall for exact preview %s?", report.Observation)
			}
			if !jsonOutput {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), prompt); err != nil {
					return err
				}
			}
			approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), prompt)
			if err != nil {
				return err
			}
			if !approved {
				return errors.New("project uninstall was not approved")
			}
			result, err := capabilitypack.ApplyProjectUninstall(cmd.Context(), capabilitypack.ProjectUninstallApplyRequest{Preview: report, PackyHome: snapshot.PackyHome(), Adapter: adapter})
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Verified project uninstall")
			return err
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "Remove only one installed CLI surface (codex)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview every exact removal without mutation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func renderProjectUninstallPreview(cmd *cobra.Command, report capabilitypack.JSONProjectUninstallPreview, dryRun bool) error {
	header := "PROJECT SURFACE UNINSTALL PREVIEW"
	scope := "surface " + string(report.Surface)
	if report.Scope == capabilitypack.ProjectUninstallPack {
		header, scope = "COMPLETE PROJECT PACK UNINSTALL PREVIEW", "complete pack"
	}
	if dryRun {
		header = strings.Replace(header, "PREVIEW", "DRY-RUN", 1)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\nProject root: %s\nPack: %s %s\nScope: %s\n", header, report.ProjectRoot, report.Pack.ID, report.Pack.Version, scope); err != nil {
		return err
	}
	for _, projection := range report.Projections {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Remove projection: %s -> %s mode=%s health=%s\n", projection.Resource, projection.Target, projection.Mode, projection.Health); err != nil {
			return err
		}
	}
	for _, contract := range report.Contracts {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Remove project contract contribution: %s\n", contract); err != nil {
			return err
		}
	}
	if len(report.Blockers) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Blockers: none"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Blockers:"); err != nil {
			return err
		}
		for _, blocker := range report.Blockers {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s; remediation: %s\n", blocker.Code, blocker.Detail, blocker.Remediation); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Disposition: %s\n", report.Disposition)
	return err
}

func newPackInstallCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var jsonOutput bool
	var aliasValues []string
	var resourceValues []string
	var providerValues []string
	cmd := &cobra.Command{
		Use:   "install [pack]",
		Short: "Install a capability pack in the current Git project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			selection := capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll}
			if len(resourceValues) > 0 {
				selection.Mode = capabilitypack.SelectionCustom
				for _, value := range resourceValues {
					resource, parseErr := capabilitypack.ParseResourceIdentity(value)
					if parseErr != nil {
						return parseErr
					}
					selection.Roots = append(selection.Roots, resource)
				}
			}
			providerChoices, err := parseProviderChoices(providerValues)
			if err != nil {
				return err
			}
			snapshot, err := workstationResolver.Resolve(workstation.Options{})
			if err != nil {
				return err
			}
			cwd, err := snapshot.CurrentDirectory()
			if err != nil {
				return fmt.Errorf("resolve current directory: %w", err)
			}
			projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
			if err != nil {
				return err
			}
			if len(args) == 1 && surface == "" {
				return errors.New("--surface is required when installing a pack")
			}
			if len(args) == 0 && surface != "" {
				return errors.New("--surface is not accepted when reconciling the complete project manifest")
			}
			if len(args) == 0 && (len(aliasValues) > 0 || len(resourceValues) > 0 || len(providerValues) > 0) {
				return errors.New("--resource, --alias, and --provider are accepted only when installing a pack")
			}
			offlineAdapter := projectOfflineAdapter("")
			pendingRecovery, err := capabilitypack.ProjectInstallRecoveryPending(snapshot.PackyHome(), projectRoot)
			if err != nil {
				return err
			}
			if dryRun && pendingRecovery {
				return errors.New("project installation is recovery-required; rerun `packy pack install` without --dry-run before requesting another preview")
			}
			if !dryRun {
				recoveryFacade := capabilitypack.NewFacade(capabilitypack.Catalog{})
				recovered, recoveryErr := recoveryFacade.RecoverProjectInstall(cmd.Context(), capabilitypack.ProjectInstallRecoveryRequest{ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: offlineAdapter})
				if recoveryErr != nil {
					return recoveryErr
				}
				if recovered && !jsonOutput {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Recovered the prior project installation attempt before preview"); err != nil {
						return err
					}
				}
			}
			if len(args) == 0 {
				status, statusErr := capabilitypack.InspectProjectStatus(cmd.Context(), capabilitypack.ProjectStatusRequest{
					ProjectRoot: projectRoot,
					Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
						capabilitypack.SurfaceClaude: projectOfflineAdapter(capabilitypack.SurfaceClaude), capabilitypack.SurfaceCodex: projectOfflineAdapter(capabilitypack.SurfaceCodex), capabilitypack.SurfaceOpenCode: projectOfflineAdapter(capabilitypack.SurfaceOpenCode),
					},
				})
				if statusErr != nil {
					return statusErr
				}
				converged := len(status.Packs) > 0
				for _, pack := range status.Packs {
					converged = converged && pack.Installation == capabilitypack.ProjectInstallationInstalled
				}
				if converged {
					if jsonOutput {
						return json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.ProjectInstallApplyResult{SchemaVersion: 1, Report: "project-install-apply", Status: "no-op"})
					}
					_, err := fmt.Fprintln(cmd.OutOrStdout(), "Verified no-op: the exact project installation is already present")
					return err
				}
			}
			composition, err := resolvePackComposition(opts, workstationResolver)
			if err != nil {
				return err
			}
			facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(composition.state.File()), nil))
			adapter := projectInstallAdapter(capabilitypack.Surface(surface), composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
			var report capabilitypack.JSONProjectInstallPreview
			if len(args) == 0 {
				report, err = facade.PreviewProjectReconcile(cmd.Context(), projectRoot, adapter)
			} else {
				report, err = facade.PreviewProjectInstall(cmd.Context(), capabilitypack.ProjectInstallRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot, Selection: selection, Aliases: aliases, ProviderChoices: providerChoices}, adapter)
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				outputReport := report
				outputReport.DryRun = dryRun
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(outputReport); err != nil {
					return err
				}
			} else if err := renderProjectInstallPreview(cmd, report, dryRun); err != nil {
				return err
			}
			if report.Disposition == capabilitypack.ProjectInstallBlocked {
				return capabilitypack.ProjectInstallNotActionableError{Disposition: report.Disposition}
			}
			if dryRun {
				return nil
			}
			if report.Disposition == capabilitypack.ProjectInstallConverged {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Verified no-op: the exact project installation is already present")
				return err
			}
			if !opts.Terminal.Interactive(cmd.InOrStdin()) {
				return capabilitypack.ErrInteractiveRequired
			}
			if !jsonOutput {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Approve project installation for exact preview %s?\n", report.Observation); err != nil {
					return err
				}
			}
			approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve project installation for exact preview %s?", report.Observation))
			if err != nil {
				return err
			}
			if !approved {
				return errors.New("project installation was not approved")
			}
			result, err := facade.ApplyProjectInstall(cmd.Context(), capabilitypack.ProjectInstallApplyRequest{Preview: report, PackyHome: snapshot.PackyHome(), Adapter: adapter})
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			if result.Status == "no-op" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Verified no-op: the exact project installation is already present")
			} else {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Verified project installation")
			}
			if err != nil || jsonOutput || result.Status == "no-op" || len(args) == 0 {
				return err
			}
			return offerProjectActivation(cmd, opts, facade, report, projectRoot, snapshot.PackyHome(), projectRuntimeAdapter(opts, report.Surface, snapshot))
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the project contract and projections without mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a project surface alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().StringArrayVar(&resourceValues, "resource", nil, "Select one operational project resource (<kind>:<id>); repeatable")
	cmd.Flags().StringArrayVar(&providerValues, "provider", nil, "Select a project capability provider (<capability>=<pack>[/<kind>:<id>]); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func offerProjectActivation(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, install capabilitypack.JSONProjectInstallPreview, projectRoot, packyHome string, adapter capabilitypack.SurfaceAdapter) error {
	preview, err := facade.PreviewProjectActivation(cmd.Context(), capabilitypack.ProjectActivationRequest{
		PackID: install.Pack.ID, Surface: install.Surface, ProjectRoot: projectRoot, PackyHome: packyHome, Adapter: adapter,
	})
	if err != nil {
		return err
	}
	if !preview.RuntimeRequired || preview.Disposition == capabilitypack.ProjectActivationConverged {
		return nil
	}
	if preview.Disposition == capabilitypack.ProjectActivationInheritedGlobal || preview.Disposition == capabilitypack.ProjectActivationBlocked {
		if err := renderProjectActivationPreview(cmd, preview); err != nil {
			return err
		}
		if preview.Disposition == capabilitypack.ProjectActivationBlocked {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Project installation remains installed; personal runtime readiness is blocked by the activation scope conflict")
			return err
		}
		return nil
	}
	if err := renderProjectActivationPreview(cmd, preview); err != nil {
		return err
	}
	approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), "Continue with the separately previewed personal project activation?")
	if err != nil {
		return err
	}
	if !approved {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Project installation remains installed; activate later with `packy pack activate %s --surface %s --project`\n", preview.Pack.ID, preview.Surface)
		return err
	}
	return approveAndApplyProjectActivation(cmd, opts, facade, preview, adapter, false, "project installation succeeded but activation was cancelled")
}

func renderProjectInstallPreview(cmd *cobra.Command, report capabilitypack.JSONProjectInstallPreview, dryRun bool) error {
	header := "Project install preview"
	if dryRun {
		header = "Project install dry-run"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\nProject root: %s\nPack: %s %s\nSurface: %s\nSelection: %s (%d resources)\nManifest: %s (schema %d)\nLock: %s (schema %d)\nNotices: %s (%d contributions)\n", header, report.ProjectRoot, report.Pack.ID, report.Pack.Version, report.Surface, report.Selection.Mode, len(report.Selection.Resources), report.Manifest.Path, report.Manifest.SchemaVersion, report.Lock.Path, report.Lock.SchemaVersion, report.Notices.Path, len(report.Notices.Contributions)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Admitted source: %s repository=%s commit=%s tree=%s lock_sha256=%s\nLock graph: %d resources, %d projections\n", report.Lock.Source.SourceID, report.Lock.Source.Repository, report.Lock.Source.Commit, report.Lock.Source.Tree, report.Lock.Source.SourceLockSHA256, len(report.Lock.ResourceGraph.Resources), len(report.Lock.Projections)); err != nil {
		return err
	}
	for _, projection := range report.Projections {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Projection: %s -> %s mode=%s observed=%s\n", projection.Resource, projection.Target, projection.Mode, projection.ObservedState); err != nil {
			return err
		}
		for _, discovered := range projection.DiscoverableBy {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  Incidentally discoverable by %s; no installation or activation intent recorded\n", discovered); err != nil {
				return err
			}
		}
	}
	for _, projection := range report.Retirements {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Retirement: %s -> %s observed=%s\n", projection.Resource, projection.Target, projection.ObservedState); err != nil {
			return err
		}
	}
	for _, change := range report.SensitiveChanges {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Sensitive change: %s %s on %s (%s); %s\n", change.Change, change.Resource, change.Surface, change.Category, change.Detail); err != nil {
			return err
		}
	}
	for _, contribution := range report.Notices.Contributions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Legal contribution: %s license=%s attribution=%s\n", contribution.Resource, contribution.License, contribution.Attribution); err != nil {
			return err
		}
	}
	requirements := report.Requirements
	if len(requirements) == 0 {
		requirements = []string{"none"}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Requirements: %s\n", strings.Join(requirements, ", ")); err != nil {
		return err
	}
	if len(report.Blockers) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Blockers: none"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Blockers:"); err != nil {
			return err
		}
		for _, blocker := range report.Blockers {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s; remediation: %s\n", blocker.Code, blocker.Detail, blocker.Remediation); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Disposition: %s\n", report.Disposition)
	return err
}

func newPackReconcileCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "reconcile [pack]", Short: "Repair active capability packs on one CLI surface", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(aliasValues) > 0 && len(args) == 0 {
				return fmt.Errorf("--alias is valid only for targeted reconcile of one pack")
			}
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			plan, err := facade.PreviewReconcile(cmd.Context(), capabilitypack.ReconcileRequest{PackID: packID, Surface: capabilitypack.Surface(surface), Aliases: aliases})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackDeactivateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var resourceValues []string
	var jsonOutput bool
	cmd := &cobra.Command{Use: "deactivate <pack>", Short: "Deactivate a capability pack on one CLI surface", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		resources := make([]capabilitypack.ResourceIdentity, 0, len(resourceValues))
		for _, value := range resourceValues {
			resource, err := capabilitypack.ParseResourceIdentity(value)
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			resources = append(resources, resource)
		}
		facade, err := activationFacade(opts, workstationResolver)
		if err != nil {
			return err
		}
		plan, err := facade.PreviewDeactivate(cmd.Context(), capabilitypack.DeactivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Resources: resources})
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
		}
		if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
			return err
		}
		return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
	}}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&resourceValues, "resource", nil, "Remove a manifest-v4 operational resource root (<kind>:<id>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackUpdateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	var project bool
	var version string
	cmd := &cobra.Command{
		Use: "update <pack>", Short: "Update an active capability pack to the catalog-current version", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if project {
				if surface != "" {
					return errors.New("--surface is not accepted for project update")
				}
				if len(aliasValues) > 0 {
					return errors.New("--alias is not accepted for project update")
				}
				if version == "" {
					return errors.New("--version is required for project update")
				}
				return runProjectPackUpdate(cmd, opts, workstationResolver, args[0], version, dryRun, jsonOutput)
			}
			if version != "" {
				return errors.New("--version is accepted only for project update")
			}
			if surface == "" {
				return errors.New("--surface is required for global update")
			}
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			plan, err := facade.PreviewUpdate(cmd.Context(), capabilitypack.UpdateRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	cmd.Flags().BoolVar(&project, "project", false, "Update the shared project installation across every installed surface")
	cmd.Flags().StringVar(&version, "version", "", "Exact admitted project pack version")
	return cmd
}

func runProjectPackUpdate(cmd *cobra.Command, opts Options, workstationResolver *workstation.Resolver, packID, version string, dryRun, jsonOutput bool) error {
	snapshot, err := workstationResolver.Resolve(workstation.Options{})
	if err != nil {
		return err
	}
	cwd, err := snapshot.CurrentDirectory()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
	if err != nil {
		return err
	}
	adapter := projectOfflineAdapter("")
	pendingRecovery, err := capabilitypack.ProjectInstallRecoveryPending(snapshot.PackyHome(), projectRoot)
	if err != nil {
		return err
	}
	if dryRun && pendingRecovery {
		return errors.New("project installation is recovery-required; rerun the project update without --dry-run before requesting another preview")
	}
	if !dryRun {
		recovered, recoveryErr := capabilitypack.NewFacade(capabilitypack.Catalog{}).RecoverProjectInstall(cmd.Context(), capabilitypack.ProjectInstallRecoveryRequest{ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter})
		if recoveryErr != nil {
			return recoveryErr
		}
		if recovered && !jsonOutput {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Recovered the prior project installation attempt before preview"); err != nil {
				return err
			}
		}
	}
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return err
	}
	facade := capabilitypack.NewFacade(composition.catalog)
	adapter = projectInstallAdapter("", composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
	report, err := facade.PreviewProjectUpdate(cmd.Context(), capabilitypack.ProjectUpdateRequest{PackID: packID, Version: version, ProjectRoot: projectRoot}, adapter)
	if err != nil {
		return err
	}
	if jsonOutput {
		output := report
		output.DryRun = dryRun
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(output); err != nil {
			return err
		}
	} else if err := renderProjectInstallPreview(cmd, report, dryRun); err != nil {
		return err
	}
	if report.Disposition == capabilitypack.ProjectInstallBlocked {
		return capabilitypack.ProjectInstallNotActionableError{Disposition: report.Disposition}
	}
	if dryRun {
		return nil
	}
	if report.Disposition == capabilitypack.ProjectInstallConverged {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Verified no-op: the exact project installation is already present")
		return err
	}
	if !opts.Terminal.Interactive(cmd.InOrStdin()) {
		return capabilitypack.ErrInteractiveRequired
	}
	approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve project update for exact preview %s?", report.Observation))
	if err != nil {
		return err
	}
	if !approved {
		return errors.New("project update was not approved")
	}
	destructiveCleanupApproved := false
	if len(report.Retirements) > 0 {
		destructiveCleanupApproved, err = opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve destructive-cleanup phase for exact project update preview %s?", report.Observation))
		if err != nil {
			return err
		}
		if !destructiveCleanupApproved {
			return errors.New("project update destructive-cleanup phase was not approved")
		}
	}
	result, err := facade.ApplyProjectInstall(cmd.Context(), capabilitypack.ProjectInstallApplyRequest{Preview: report, PackyHome: snapshot.PackyHome(), Adapter: adapter, DestructiveCleanupApproved: destructiveCleanupApproved})
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Verified project update")
	return err
}

func applyPackPlan(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, plan capabilitypack.ReconciliationPlan, dryRun, jsonOutput bool) error {
	if !plan.Applicable() {
		return lifecycleFailure(cmd, jsonOutput, "blocked", capabilitypack.PlanNotActionableError{Disposition: plan.Disposition()}, &plan)
	}
	if dryRun || plan.NoOp() {
		return nil
	}
	interactive := opts.Terminal.Interactive(cmd.InOrStdin())
	if !interactive {
		_, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Interactive: false})
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "apply-noninteractive", err, &plan)
		}
		return nil
	}
	var receipts []capabilitypack.ApprovalReceipt
	for _, phase := range plan.Phases() {
		if !phase.ApprovalRequired {
			continue
		}
		approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve %s phase for exact plan %s?", phase.Kind, plan.ID()))
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "approval", err, &plan)
		}
		if !approved {
			operation := string(plan.Operation())
			if plan.Operation() == capabilitypack.OperationActivate {
				operation = "activation"
			}
			return lifecycleFailure(cmd, jsonOutput, "approval", fmt.Errorf("%s cancelled; plan %s was not approved", operation, plan.ID()), &plan)
		}
		receipts = append(receipts, facade.Approve(plan, phase.Kind))
	}
	result, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Approvals: receipts, Interactive: true})
	if err != nil {
		return lifecycleFailure(cmd, jsonOutput, "apply", err, &plan)
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.JSONApplyResultFor(plan, result))
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified plan %s: %d %s projections owned by %s\n", result.PlanID, result.Projections, surfaceName(plan.Surface()), plan.Pack().ID); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Apply result facts: verified=%s projections=%d\n", yesNo(result.Verified), result.Projections); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Readiness: configured=%s, authorized=%s, usable=%s\n", readinessValue(result.ReadinessObserved.Configured, result.Readiness.Configured), readinessValue(result.ReadinessObserved.Authorization, result.Readiness.Authorized), readinessValue(result.ReadinessObserved.Usability, result.Readiness.Usable)); err != nil {
		return err
	}
	if len(result.PendingHumanActions) > 0 {
		if _, err = fmt.Fprintln(cmd.OutOrStdout(), "Pending human actions:"); err != nil {
			return err
		}
		for _, action := range result.PendingHumanActions {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

func newPackActivateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var project bool
	var aliasValues []string
	var resourceValues []string
	var providerValues []string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "activate <pack>", Short: "Activate a capability pack on one CLI surface", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if project {
				if len(aliasValues) > 0 || len(resourceValues) > 0 || len(providerValues) > 0 {
					return errors.New("--alias, --resource, and --provider are not accepted with --project; project activation consumes the exact installed lock")
				}
				snapshot, err := workstationResolver.Resolve(workstation.Options{})
				if err != nil {
					return err
				}
				cwd, err := snapshot.CurrentDirectory()
				if err != nil {
					return fmt.Errorf("resolve current directory: %w", err)
				}
				projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
				if err != nil {
					return err
				}
				facade := capabilitypack.NewFacade(capabilitypack.Catalog{})
				store := capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(snapshot.PackyHome()).File())
				global := capabilitypack.ObserveActiveIntents(cmd.Context(), store)
				globalRelevant := slices.Contains(global.FailedSurfaces, capabilitypack.Surface(surface))
				for _, intent := range global.Intents {
					globalRelevant = globalRelevant || intent.PackID == args[0] && intent.Surface == capabilitypack.Surface(surface)
				}
				if globalRelevant {
					facade, err = activationFacade(opts, workstationResolver)
					if err != nil {
						return err
					}
				}
				adapter := projectRuntimeAdapter(opts, capabilitypack.Surface(surface), snapshot)
				preview, err := facade.PreviewProjectActivation(cmd.Context(), capabilitypack.ProjectActivationRequest{
					PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot,
					PackyHome: snapshot.PackyHome(), Adapter: adapter,
				})
				if err != nil {
					return err
				}
				if jsonOutput {
					outputPreview := preview
					outputPreview.DryRun = dryRun
					if err := json.NewEncoder(cmd.OutOrStdout()).Encode(outputPreview); err != nil {
						return err
					}
				} else if err := renderProjectActivationPreview(cmd, preview); err != nil {
					return err
				}
				if !preview.RuntimeRequired || dryRun || preview.Disposition == capabilitypack.ProjectActivationConverged || preview.Disposition == capabilitypack.ProjectActivationInheritedGlobal {
					return nil
				}
				if !opts.Terminal.Interactive(cmd.InOrStdin()) {
					return capabilitypack.ErrInteractiveRequired
				}
				return approveAndApplyProjectActivation(cmd, opts, facade, preview, adapter, jsonOutput, "project activation cancelled")
			}
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			selection := capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll}
			if len(resourceValues) > 0 {
				selection.Mode = capabilitypack.SelectionCustom
				for _, value := range resourceValues {
					resource, err := capabilitypack.ParseResourceIdentity(value)
					if err != nil {
						return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
					}
					selection.Roots = append(selection.Roots, resource)
				}
			}
			providerChoices, err := parseProviderChoices(providerValues)
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			plan, err := facade.Preview(cmd.Context(), capabilitypack.ActivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases, Selection: selection, ProviderChoices: providerChoices})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().BoolVar(&project, "project", false, "Activate personal runtime effects from the current project installation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().StringArrayVar(&resourceValues, "resource", nil, "Select one manifest-v4 operational resource (<kind>:<id>); repeatable")
	cmd.Flags().StringArrayVar(&providerValues, "provider", nil, "Select a capability provider (<capability>=<pack>[/<kind>:<id>]); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func projectRuntimeAdapter(opts Options, surface capabilitypack.Surface, snapshot workstation.Snapshot) capabilitypack.SurfaceAdapter {
	if adapter := opts.SurfaceAdapters[surface]; adapter != nil {
		return adapter
	}
	home := snapshot.Home()
	if surface == capabilitypack.SurfaceCodex && home != "" {
		layout := codex.NewCanonicalLayout(home)
		return codex.NewSurfaceAdapterWithConfig("", "", layout.PromptFile(), layout.ConfigFile())
	}
	if surface == capabilitypack.SurfaceClaude && home != "" {
		executable, _ := opts.ClaudeLookPath("claude")
		layout := claudecode.NewCanonicalLayout(home)
		var adapter *claudecode.SurfaceAdapter
		if opts.ClaudeAuthorization != nil {
			adapter = claudecode.NewSurfaceAdapterWithAuthorization("", layout, snapshot.PackyHome(), executable, opts.ClaudeRunner, nil, opts.ClaudeAuthorization)
		} else {
			adapter = claudecode.NewSurfaceAdapter("", layout, snapshot.PackyHome(), executable, opts.ClaudeRunner, nil)
		}
		if opts.ClaudeRuntimeEvidence != nil {
			adapter = adapter.WithRuntimeEvidence(opts.ClaudeRuntimeEvidence)
		}
		return adapter
	}
	return projectOfflineAdapter(surface)
}

func approveAndApplyProjectActivation(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, preview capabilitypack.JSONProjectActivationPreview, adapter capabilitypack.SurfaceAdapter, jsonOutput bool, cancellation string) error {
	approvals := make([]capabilitypack.ProjectActivationApproval, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve project %s for exact activation %s?", category.Kind, preview.Digest))
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("%s; %s was not approved", cancellation, category.Kind)
		}
		approvals = append(approvals, facade.ApproveProjectActivation(preview, category.Kind))
	}
	result, err := facade.ApplyProjectActivation(cmd.Context(), capabilitypack.ProjectActivationApplyRequest{Preview: preview, Approvals: approvals, Adapter: adapter, Interactive: true})
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified personal project activation %s\n", result.Digest)
	return err
}

func renderProjectActivationPreview(cmd *cobra.Command, preview capabilitypack.JSONProjectActivationPreview) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Project activation preview\nProject root: %s\nPack: %s %s\nSurface: %s\nRuntime activation: %s\nSensitive lock identity: %s\n", preview.ProjectRoot, preview.Pack.ID, preview.Pack.Version, preview.Surface, preview.Disposition, preview.SensitiveLockIdentity); err != nil {
		return err
	}
	for _, category := range preview.Categories {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Approval category: %s (%d declared facts)\n", category.Kind, len(category.Details)); err != nil {
			return err
		}
		for _, detail := range category.Details {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", detail.Resource, detail.Detail); err != nil {
				return err
			}
		}
	}
	for _, effect := range preview.RuntimeEffects {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Runtime effect: %s %s %s coverage=%s", effect.Category, effect.Resource, effect.Detail, effect.Coverage); err != nil {
			return err
		}
		if effect.GlobalVersion != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " global_version=%s", effect.GlobalVersion); err != nil {
				return err
			}
		}
		if effect.Conflict != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " conflict=%s", effect.Conflict); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
			return err
		}
	}
	for _, effect := range preview.Effects {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Personal effect: %s -> %s identity=%s\n", effect.Action, effect.Target, effect.Identity); err != nil {
			return err
		}
	}
	return nil
}

func parseProviderChoices(values []string) ([]capabilitypack.ProviderChoice, error) {
	if values == nil {
		return nil, nil
	}
	result := make([]capabilitypack.ProviderChoice, 0, len(values))
	for _, value := range values {
		capability, provider, ok := strings.Cut(value, "=")
		if !ok || capability == "" || provider == "" {
			return nil, fmt.Errorf("provider choice %q must be <capability>=<pack>[/<kind>:<id>]", value)
		}
		packID, resourceValue, hasResource := strings.Cut(provider, "/")
		if packID == "" {
			return nil, fmt.Errorf("provider choice %q has an empty provider pack", value)
		}
		choice := capabilitypack.ProviderChoice{Capability: capability, ProviderPack: packID}
		if hasResource {
			resource, err := capabilitypack.ParseResourceIdentity(resourceValue)
			if err != nil {
				return nil, fmt.Errorf("provider choice %q: %w", value, err)
			}
			choice.ProviderResource = &resource
		}
		result = append(result, choice)
	}
	return result, nil
}

func lifecycleFailure(cmd *cobra.Command, jsonOutput bool, stage string, err error, plan *capabilitypack.ReconciliationPlan) error {
	err = capabilitypack.ReportSafeError(err, plan)
	if jsonOutput {
		var approval *bool
		var actions *int
		if stage == "preview" || stage == "blocked" {
			no, zero := false, 0
			approval, actions = &no, &zero
		} else if stage == "approval" {
			yes, zero := true, 0
			approval, actions = &yes, &zero
		} else if stage == "apply-noninteractive" {
			no := false
			approval = &no
		} else if stage == "apply" {
			yes := true
			approval = &yes
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.JSONFailureFor(stage, err, plan, approval, actions))
	}
	return err
}

func surfaceName(surface capabilitypack.Surface) string {
	if surface == capabilitypack.SurfaceClaude {
		return "Claude Code"
	}
	if surface == capabilitypack.SurfaceOpenCode {
		return "OpenCode"
	}
	return "Codex"
}

func parseSurfaceAliases(values []string) ([]capabilitypack.SurfaceAlias, error) {
	if values == nil {
		return nil, nil
	}
	aliases := make([]capabilitypack.SurfaceAlias, 0, len(values))
	for _, value := range values {
		if strings.Count(value, "=") != 1 {
			return nil, fmt.Errorf("invalid --alias %q: expected <kind>:<logical-id>=<host-name>", value)
		}
		identity, name, _ := strings.Cut(value, "=")
		kind, id, ok := strings.Cut(identity, ":")
		if !ok || kind == "" || id == "" || name == "" {
			return nil, fmt.Errorf("invalid --alias %q: expected <kind>:<logical-id>=<host-name>", value)
		}
		aliases = append(aliases, capabilitypack.SurfaceAlias{Kind: kind, ID: id, Name: name})
	}
	return aliases, nil
}

func readinessValue(observed, value bool) string {
	if !observed {
		return "unknown"
	}
	return yesNo(value)
}

func activationFacade(opts Options, workstationResolver *workstation.Resolver) (capabilitypack.Facade, error) {
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return capabilitypack.Facade{}, err
	}
	codexAdapter := codex.NewSurfaceAdapterWithConfig(composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile())
	openCodeAdapter := opencode.NewSurfaceAdapter(composition.bundleRoot, composition.skills.Root(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
	store := capabilitypack.NewFileActivationStore(composition.state.File())
	claudeLayout := composition.claude
	claudeExecutable, _ := opts.ClaudeLookPath("claude")
	claudePacks := make(map[string]capabilitypack.Pack)
	for _, pack := range composition.catalog.List() {
		if slices.Contains(pack.Surfaces, capabilitypack.SurfaceClaude) {
			claudePacks[pack.ID] = pack
			claudePacks[pack.ID+"@"+pack.Version] = pack
		}
	}
	claudeState, err := store.Load(context.Background(), capabilitypack.SurfaceClaude)
	if err != nil {
		return capabilitypack.Facade{}, fmt.Errorf("load Claude activation contracts: %w", err)
	}
	claudeIntents := claudeState.Intents
	if len(claudeIntents) == 0 && claudeState.Intent.PackID != "" {
		claudeIntents = []capabilitypack.ActivationIntent{claudeState.Intent}
	}
	for _, intent := range claudeIntents {
		if !intent.Active || intent.Surface != capabilitypack.SurfaceClaude {
			continue
		}
		pack, resolveErr := composition.catalog.ResolveIntentPack(intent.PackID, intent.Version)
		if resolveErr != nil {
			return capabilitypack.Facade{}, fmt.Errorf("resolve Claude activation contract %s@%s: %w", intent.PackID, intent.Version, resolveErr)
		}
		claudePacks[intent.PackID+"@"+intent.Version] = pack
	}
	ownership := claudecode.NewCapabilityPackOwnershipProvider(store, claudePacks, claudeLayout, composition.bundleRoot)
	var claudeAdapter *claudecode.SurfaceAdapter
	if opts.ClaudeAuthorization != nil {
		claudeAdapter = claudecode.NewSurfaceAdapterWithAuthorization(composition.bundleRoot, claudeLayout, filepath.Dir(composition.state.File()), claudeExecutable, opts.ClaudeRunner, ownership, opts.ClaudeAuthorization)
	} else {
		claudeAdapter = claudecode.NewSurfaceAdapter(composition.bundleRoot, claudeLayout, filepath.Dir(composition.state.File()), claudeExecutable, opts.ClaudeRunner, ownership)
	}
	if opts.ClaudeRuntimeEvidence != nil {
		claudeAdapter = claudeAdapter.WithRuntimeEvidence(opts.ClaudeRuntimeEvidence)
	}
	adapters := opts.SurfaceAdapters
	if adapters == nil {
		adapters = map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceCodex:    codexAdapter,
			capabilitypack.SurfaceOpenCode: openCodeAdapter,
			capabilitypack.SurfaceClaude:   claudeAdapter,
		}
	}
	return capabilitypack.NewFacade(composition.catalog,
		capabilitypack.WithActivation(store, adapters),
		capabilitypack.WithExternalEffects(
			composition.engram,
			runnerExternalExecutor{runner: opts.Runner},
		),
	), nil
}

type outputRunner interface {
	RunOutput(context.Context, string, ...string) (string, string, int, error)
}

type claudeRunner struct{ runner Runner }

func (r claudeRunner) Run(ctx context.Context, command claudecode.Command) claudecode.Result {
	if runner, ok := r.runner.(outputRunner); ok {
		stdout, stderr, exitCode, err := runner.RunOutput(ctx, command.Executable, command.Args...)
		return claudecode.Result{Stdout: stdout, Stderr: stderr, ExitCode: exitCode, Err: err}
	}
	err := r.runner.Run(ctx, command.Executable, command.Args...)
	return claudecode.Result{Err: err}
}

func renderActivationPlan(cmd *cobra.Command, plan capabilitypack.ReconciliationPlan, dryRun bool) error {
	prefix := "Activation plan"
	if plan.Operation() == capabilitypack.OperationUpdate {
		prefix = "Update plan"
	} else if plan.Operation() == capabilitypack.OperationDeactivate {
		prefix = "Deactivation plan"
	} else if plan.Operation() == capabilitypack.OperationReconcile {
		prefix = "Reconcile plan"
	}
	if dryRun {
		prefix = strings.TrimSuffix(prefix, " plan") + " dry-run plan"
	}
	packLabel := plan.Pack().ID
	if plan.Operation() != capabilitypack.OperationUpdate && plan.Operation() != capabilitypack.OperationDeactivate && plan.Operation() != capabilitypack.OperationReconcile {
		packLabel += " " + plan.Pack().Version
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDigest: %s\nPack: %s\nSurface: %s\n", prefix, plan.ID(), plan.Digest(), packLabel, plan.Surface()); err != nil {
		return err
	}
	if history := plan.HistoricalAttempt(); history != nil {
		guidance := plan.RecoveryGuidance()
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Recovery: fresh %s Preview toward the already-approved intent; historical plan %s is not replayed.\nOriginating operation: %s\nHistorical outcome: %s\nHistorical digest: %s\nAffected resources: %s\nConsumers: %s\nCompleted: %s\nFailed: %s — %s\nNot started: %s\nNext explicit lifecycle command: `%s`\nTo recover, repeat `%s`; a new Preview and approvals are required.\n", plan.Operation(), history.PlanID, guidance.OriginatingOperation, history.Outcome, history.PlanDigest, renderRecoveryResources(guidance.AffectedResources), renderRecoveryConsumers(guidance.Consumers), joinFacts(guidance.Completed), guidance.FailedAction, guidance.FailureDetail, joinFacts(guidance.NotStarted), guidance.NextCommand, guidance.NextCommand); err != nil {
			return err
		}
	}
	if plan.Operation() == capabilitypack.OperationUpdate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s -> %s (catalog-current)\nIntent revision: %d\n", plan.OldVersion(), plan.Pack().Version, plan.IntentRevision()); err != nil {
			return err
		}
	} else if plan.Operation() == capabilitypack.OperationDeactivate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Active version: %s\nIntent revision: %d\n", plan.OldVersion(), plan.IntentRevision()); err != nil {
			return err
		}
	} else if plan.Operation() == capabilitypack.OperationReconcile {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Scope: %s\nIntent revision: %d (unchanged)\n", plan.ReconcileScope(), plan.IntentRevision()); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\nIntent revision: %d\n", plan.Pack().Version, plan.IntentRevision()); err != nil {
		return err
	}
	selection := plan.Selection()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Selection mode: %s\n", selection.Mode); err != nil {
		return err
	}
	for _, root := range selection.Roots {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Selection root: %s\n", root); err != nil {
			return err
		}
	}
	for _, activation := range plan.Activations() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Activation: %s %s %s\n", activation.Role, activation.Pack.ID, activation.Pack.Version); err != nil {
			return err
		}
	}
	for _, requirement := range plan.CapabilityRequirements() {
		if err := renderCapabilityRequirement(cmd.OutOrStdout(), requirement); err != nil {
			return err
		}
	}
	for _, choice := range plan.ProviderChoices() {
		if err := renderProviderChoice(cmd.OutOrStdout(), choice); err != nil {
			return err
		}
	}
	for _, alias := range plan.Aliases() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Alias: %s:%s=%s\n", alias.Kind, alias.ID, alias.Name); err != nil {
			return err
		}
	}
	for _, origin := range plan.JSONReport(dryRun).SensitiveEffects {
		for _, authority := range origin.PromptAuthorities {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Authority origin: %s pack=%s resource=%s root=%s dependency_chain=%s\n",
				authority, origin.Pack, origin.Resource, origin.Root, renderIdentityChain(origin.DependencyChain)); err != nil {
				return err
			}
		}
		for _, authority := range origin.RuntimeAuthorities {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Runtime authority origin: mode=%s kind=%s scope=%s pack=%s resource=%s root=%s dependency_chain=%s\n",
				authority.ModeID, authority.Kind, factOrNone(string(authority.Scope)), origin.Pack, origin.Resource, origin.Root, renderIdentityChain(origin.DependencyChain)); err != nil {
				return err
			}
		}
		for _, effect := range origin.RuntimeEffects {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Sensitive effect origin: mode=%s kind=%s scope=%s pack=%s resource=%s root=%s dependency_chain=%s\n",
				effect.ModeID, effect.Kind, factOrNone(string(effect.Scope)), origin.Pack, origin.Resource, origin.Root, renderIdentityChain(origin.DependencyChain)); err != nil {
				return err
			}
		}
	}
	if err := renderPackContract(cmd, plan.LifecycleContract()); err != nil {
		return err
	}
	for _, resource := range plan.JSONReport(dryRun).ResourceGraph.Resources {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resource graph: resource=%s role=%s dependency_chain=%s requires=%s notices=%s\n",
			resource.Resource, resource.Role, renderIdentityChain(resource.DependencyChain),
			renderIdentityChain(resource.Requires), renderIdentityChain(resource.Notices)); err != nil {
			return err
		}
	}
	if err := renderRuntimeModes(cmd, plan.RuntimeModeResults()); err != nil {
		return err
	}
	readiness, observed := plan.Readiness(), plan.ReadinessObserved()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Expected readiness: configured=%s, authorized=%s, usable=%s\nObserved evidence: %s\nPending evidence: %s\n", readinessValue(observed.Configured, readiness.Configured), readinessValue(observed.Authorization, readiness.Authorized), readinessValue(observed.Usability, readiness.Usable), renderPendingAction(plan.Evidence()), renderPendingAction(plan.PendingEvidence())); err != nil {
		return err
	}
	structured := plan.JSONReport(dryRun)
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contract diff: added=%s changed=%s removed=%s retained=%s\n", joinFacts(structured.ContractDiff.Added), joinFacts(structured.ContractDiff.Changed), joinFacts(structured.ContractDiff.Removed), joinFacts(structured.ContractDiff.Retained)); err != nil {
		return err
	}
	for _, migration := range structured.Migrations {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Migration: %s\n", migration); err != nil {
			return err
		}
	}
	disposition := plan.Disposition()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan disposition: %s\n", disposition); err != nil {
		return err
	}
	if disposition == capabilitypack.PlanBlocked || disposition == capabilitypack.PlanMixed {
		operation := "activation"
		if plan.Operation() == capabilitypack.OperationUpdate {
			operation = "update"
		} else if plan.Operation() == capabilitypack.OperationDeactivate {
			operation = "deactivation"
		} else if plan.Operation() == capabilitypack.OperationReconcile {
			operation = "reconcile"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Cannot apply %s: %d blockers\nPreserved or blocked projections:\n", operation, len(plan.Blockers())); err != nil {
			return err
		}
		for _, blocker := range plan.Blockers() {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Blocker: %s %s — %s\n", blocker.Kind, blocker.Subject, blocker.Detail); err != nil {
				return err
			}
		}
		if disposition == capabilitypack.PlanBlocked {
			return nil
		}
	}
	if disposition == capabilitypack.PlanMixed {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Applicable actions (not applied while required blockers remain):"); err != nil {
			return err
		}
	}
	if plan.NoOp() {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Already converged: no approval or Apply required.")
		return err
	}
	contributors := plan.Contributors()
	removed := plan.RemovedContributors()
	removedKeys := make([]string, 0, len(removed))
	for id := range removed {
		removedKeys = append(removedKeys, id)
	}
	sort.Strings(removedKeys)
	for _, id := range removedKeys {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contributor removed: %s <- %s\n", id, removed[id]); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(contributors))
	for id := range contributors {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contributors: %s <- %s\n", id, strings.Join(contributors[id], ", ")); err != nil {
			return err
		}
	}
	for _, retained := range plan.RetainedProjections() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Retained shared projection: %s <- %s (no rewrite)\n", retained.ID, strings.Join(retained.Contributors, ", ")); err != nil {
			return err
		}
	}
	for _, shared := range structured.SharedProjections {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Shared projection: %s key=%s discoverable_by=%s — %s\n", shared.ID, shared.ProjectionKey, joinSurfaces(shared.DiscoverableBy), shared.DiscoveryNotice); err != nil {
			return err
		}
	}
	for _, resolution := range plan.Resolutions() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Requirement: %s available=%s path=%s origin=%s acquisition_source=%s acquisition_version=%s\n",
			resolution.Tool, yesNo(resolution.Available), resolution.Path, resolution.Origin,
			factOrNone(resolution.AcquisitionSource), factOrNone(resolution.AcquisitionVersion)); err != nil {
			return err
		}
	}
	for _, phase := range structured.Phases {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Phase: %s (%s)\n", phase.Kind, phase.Digest); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Phase approval required: %s\n", yesNo(phase.ApprovalRequired)); err != nil {
			return err
		}
		for _, action := range phase.Actions {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action.Description); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(),
				"    Action facts: id=%s kind=%s consent=%s source=%s target=%s command=%s args=%s version=%s consequences=%s rollback_limits=%s mode=%s adapter_provenance=%s\n",
				action.ID, factOrNone(string(action.Kind)), factOrNone(string(action.Consent)), factOrNone(action.Source),
				factOrNone(action.Target), factOrNone(action.Command), joinFacts(action.Args), factOrNone(action.Version),
				factOrNone(action.Consequences), factOrNone(action.RollbackLimits), factOrNone(string(action.Mode)),
				factOrNone(action.AdapterProvenance),
			); err != nil {
				return err
			}
		}
	}
	if pending := plan.PendingHumanActions(); len(pending) > 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Pending human actions:"); err != nil {
			return err
		}
		for _, action := range pending {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderRecoveryResources(values []capabilitypack.RecoveryAffectedResource) string {
	facts := make([]string, 0, len(values))
	for _, value := range values {
		facts = append(facts, value.Pack+"/"+value.Resource.String())
	}
	return joinFacts(facts)
}

func renderRecoveryConsumers(values []capabilitypack.RecoveryConsumer) string {
	facts := make([]string, 0, len(values))
	for _, value := range values {
		fact := value.Pack
		if value.Resource != nil {
			fact += "/" + value.Resource.String()
		}
		if value.Capability != "" {
			fact += " (" + value.Capability + ")"
		}
		facts = append(facts, fact)
	}
	return joinFacts(facts)
}

func renderActivationPlanOutput(cmd *cobra.Command, plan capabilitypack.ReconciliationPlan, dryRun, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan.JSONReport(dryRun))
	}
	return renderActivationPlan(cmd, plan, dryRun)
}

func renderCapabilityRequirement(out io.Writer, requirement capabilitypack.CapabilityRequirementFact) error {
	consumer, provider := "all", "all"
	if requirement.ConsumerResource != nil {
		consumer = requirement.ConsumerResource.String()
	}
	if requirement.ProviderResource != nil {
		provider = requirement.ProviderResource.String()
	}
	readiness := requirement.ResultingReadiness
	_, err := fmt.Fprintf(out, "Capability requirement: consumer=%s/%s capability=%s provider=%s/%s tools=%s authority=%s readiness=configured:%t,authorized:%t,usable:%t\n",
		requirement.ConsumerPack, consumer, requirement.Capability, requirement.ProviderPack, provider,
		joinFacts(requirement.RequiredTools), joinFacts(requirement.RequiredAuthority),
		readiness.Configured, readiness.Authorized, readiness.Usable)
	return err
}

func renderProviderChoice(out io.Writer, choice capabilitypack.ProviderChoice) error {
	provider := "all"
	if choice.ProviderResource != nil {
		provider = choice.ProviderResource.String()
	}
	_, err := fmt.Fprintf(out, "Provider choice: capability=%s provider=%s/%s\n", choice.Capability, choice.ProviderPack, provider)
	return err
}

func renderPackContract(cmd *cobra.Command, contract capabilitypack.LifecycleContract) error {
	return renderPackShowContract(cmd.OutOrStdout(), contract)
}

func joinFacts(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func factOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

type runnerExternalExecutor struct{ runner Runner }

func (e runnerExternalExecutor) Execute(ctx context.Context, action capabilitypack.ProjectionAction) error {
	return e.runner.Run(ctx, action.Command, action.Args...)
}

func newPackStatusCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var resource string
	var require string
	var jsonOutput bool
	var project bool
	cmd := &cobra.Command{
		Use: "status [pack]", Short: "Inspect capability pack status", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			if project {
				if resource != "" {
					return errors.New("--resource is not yet supported with --project")
				}
				if require != "" && require != "installed" && require != "usable" {
					return errors.New("--require with --project accepts installed or usable")
				}
				if require == "usable" && (packID == "" || surface == "") {
					return errors.New("--require usable with --project requires one pack and --surface")
				}
				snapshot, err := workstationResolver.Resolve(workstation.Options{})
				if err != nil {
					return err
				}
				cwd, err := snapshot.CurrentDirectory()
				if err != nil {
					return fmt.Errorf("resolve current directory: %w", err)
				}
				projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
				if err != nil {
					return err
				}
				statusRequest := capabilitypack.ProjectStatusRequest{
					ProjectRoot: projectRoot, PackID: packID, Surface: capabilitypack.Surface(surface), RequireInstalled: require == "installed", RequireUsable: require == "usable", PackyHome: snapshot.PackyHome(),
					Adapters: map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
						capabilitypack.SurfaceClaude:   projectRuntimeAdapter(opts, capabilitypack.SurfaceClaude, snapshot),
						capabilitypack.SurfaceCodex:    projectRuntimeAdapter(opts, capabilitypack.SurfaceCodex, snapshot),
						capabilitypack.SurfaceOpenCode: projectRuntimeAdapter(opts, capabilitypack.SurfaceOpenCode, snapshot),
					},
				}
				report, err := capabilitypack.InspectProjectStatus(cmd.Context(), statusRequest)
				if err != nil {
					return err
				}
				store := capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(snapshot.PackyHome()).File())
				global := capabilitypack.ObserveActiveIntents(cmd.Context(), store)
				globalRelevant := packID == "" && (len(global.FailedSurfaces) > 0 || len(global.Intents) > 0)
				for _, failed := range global.FailedSurfaces {
					globalRelevant = globalRelevant || surface == "" || failed == capabilitypack.Surface(surface)
				}
				for _, intent := range global.Intents {
					globalRelevant = globalRelevant || intent.PackID == packID && (surface == "" || intent.Surface == capabilitypack.Surface(surface))
				}
				if globalRelevant {
					facade, facadeErr := activationFacade(opts, workstationResolver)
					if facadeErr != nil {
						return facadeErr
					}
					report, err = facade.InspectProjectStatus(cmd.Context(), statusRequest)
					if err != nil {
						return err
					}
				}
				if jsonOutput {
					if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
						return err
					}
				} else if err := renderProjectStatus(cmd, report); err != nil {
					return err
				}
				for _, status := range report.Packs {
					if require == "installed" && !status.RequirementSatisfied {
						return fmt.Errorf("pack %q on %s is not installed", status.Pack.ID, status.Surface)
					}
					if require == "usable" && !status.RequirementSatisfied {
						return fmt.Errorf("pack %q on %s is not usable", status.Pack.ID, status.Surface)
					}
				}
				if require == "installed" && len(report.Packs) == 0 {
					return errors.New("project has no installed capability packs")
				}
				if require == "usable" && len(report.Packs) == 0 {
					return errors.New("project has no usable capability packs")
				}
				return nil
			}
			if require != "" && (require != "usable" || packID == "" || surface == "") {
				return fmt.Errorf("--require usable is valid only for status of one pack and surface")
			}
			if resource != "" && (packID == "" || surface == "") {
				return fmt.Errorf("--resource is valid only for status of one pack and surface")
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			report, err := facade.Status(cmd.Context(), capabilitypack.StatusRequest{
				PackID: packID, Surface: capabilitypack.Surface(surface),
				Resource: resource, RequireUsable: require == "usable",
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report.JSONReport(packID != "")); err != nil {
					return err
				}
			} else if packID == "" {
				return renderPackStatusOverview(cmd, report)
			} else if err := renderPackStatusDetail(cmd, report.Entries[0], report.Focused); err != nil {
				return err
			}
			if report.Requirement != nil && !report.Requirement.Satisfied {
				if report.Requirement.Resource.Kind != "" {
					return fmt.Errorf("resource %q in pack %q on %s is not freshly observed usable", report.Requirement.Resource, packID, surface)
				}
				return fmt.Errorf("pack %q on %s is not freshly observed usable", packID, surface)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().StringVar(&resource, "resource", "", "Inspect one selected resource (<kind>:<id>)")
	cmd.Flags().StringVar(&require, "require", "", "Require a readiness dimension (usable)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	cmd.Flags().BoolVar(&project, "project", false, "Inspect the shared project installation and personal runtime axes")
	return cmd
}

func projectOfflineAdapter(surface capabilitypack.Surface) capabilitypack.SurfaceAdapter {
	if surface == "" {
		return capabilitypack.NewProjectSurfaceAdapterSet(map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceClaude: claudeProjectAdapter(""), capabilitypack.SurfaceCodex: codex.NewSurfaceAdapterWithConfig("", "", "", ""), capabilitypack.SurfaceOpenCode: opencode.NewSurfaceAdapter("", "", "", ""),
		}, capabilitypack.SurfaceCodex)
	}
	if surface == capabilitypack.SurfaceClaude {
		return claudeProjectAdapter("")
	}
	if surface == capabilitypack.SurfaceOpenCode {
		return opencode.NewSurfaceAdapter("", "", "", "")
	}
	return codex.NewSurfaceAdapterWithConfig("", "", "", "")
}

func projectInstallAdapter(surface capabilitypack.Surface, bundleRoot, skillsRoot, codexPrompt, codexConfig, openCodeConfig, openCodePrompt string) capabilitypack.SurfaceAdapter {
	if surface == "" {
		return capabilitypack.NewProjectSurfaceAdapterSet(map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceClaude:   claudeProjectAdapter(bundleRoot),
			capabilitypack.SurfaceCodex:    codex.NewSurfaceAdapterWithConfig(bundleRoot, skillsRoot, codexPrompt, codexConfig),
			capabilitypack.SurfaceOpenCode: opencode.NewSurfaceAdapter(bundleRoot, skillsRoot, openCodeConfig, openCodePrompt),
		}, capabilitypack.SurfaceCodex)
	}
	if surface == capabilitypack.SurfaceClaude {
		return claudeProjectAdapter(bundleRoot)
	}
	if surface == capabilitypack.SurfaceOpenCode {
		return opencode.NewSurfaceAdapter(bundleRoot, skillsRoot, openCodeConfig, openCodePrompt)
	}
	return codex.NewSurfaceAdapterWithConfig(bundleRoot, skillsRoot, codexPrompt, codexConfig)
}

func claudeProjectAdapter(bundleRoot string) capabilitypack.SurfaceAdapter {
	return claudecode.NewSurfaceAdapter(bundleRoot, claudecode.NewCanonicalLayout(""), "", "", nil, nil)
}

func renderProjectStatus(cmd *cobra.Command, report capabilitypack.JSONProjectStatusReport) error {
	for _, status := range report.Packs {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s (project)\nInstallation: %s\nRuntime activation: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nProjections: %d\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", status.Pack.ID, status.Pack.Version, status.Surface, status.Installation, status.Runtime, yesNo(status.Readiness.Configured), yesNo(status.Readiness.Authorized), yesNo(status.Readiness.Usable), len(status.Projections), renderProjectInstallBlockers(status.Blockers), renderPendingAction(status.PendingHumanActions), renderPendingAction(status.Evidence)); err != nil {
			return err
		}
		for _, effect := range status.RuntimeEffects {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Runtime effect: %s %s %s coverage=%s", effect.Category, effect.Resource, effect.Detail, effect.Coverage); err != nil {
				return err
			}
			if effect.GlobalVersion != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), " global_version=%s", effect.GlobalVersion); err != nil {
					return err
				}
			}
			if effect.Conflict != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), " conflict=%s", effect.Conflict); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderProjectInstallBlockers(blockers []capabilitypack.ProjectInstallBlocker) string {
	if len(blockers) == 0 {
		return "none"
	}
	values := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		values = append(values, blocker.Code+": "+blocker.Detail)
	}
	return strings.Join(values, "; ")
}

func renderPackStatusOverview(cmd *cobra.Command, report capabilitypack.StatusReport) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PACK\tSURFACE\tINTENT\tATTEMPT\tCONFIGURED\tAUTHORIZED\tUSABLE\tACTION")
	for _, entry := range report.Entries {
		configured := readinessValue(entry.ReadinessObserved.Configured, entry.Readiness.Configured)
		authorized := readinessValue(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized)
		usable := readinessValue(entry.ReadinessObserved.Usability, entry.Readiness.Usable)
		intent := renderIntent(entry.Intent)
		if entry.LifecycleState == capabilitypack.PackLifecycleInactiveWithResiduals {
			intent = string(entry.LifecycleState)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.Pack.ID, entry.Surface, intent, renderAttempt(entry.LatestAttempt), configured, authorized, usable, renderStatusAction(entry))
	}
	return writer.Flush()
}

func renderPackStatusDetail(cmd *cobra.Command, entry capabilitypack.StatusEntry, focused *capabilitypack.ResourceStatus) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\nIntent: %s\nLifecycle state: %s\nUpdate available: %s\nLatest attempt: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nProjections: %d verified; %d drifted; %d ambiguous; %d missing; %d unmanaged\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, renderIntent(entry.Intent), entry.LifecycleState, renderUpdateAvailability(entry), renderAttempt(entry.LatestAttempt), readinessValue(entry.ReadinessObserved.Configured, entry.Readiness.Configured), readinessValue(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized), readinessValue(entry.ReadinessObserved.Usability, entry.Readiness.Usable), entry.Projections.Verified, entry.Projections.Drifted, entry.Projections.Ambiguous, entry.Projections.Missing, entry.Projections.Unmanaged, renderPendingAction(entry.Blockers), renderPendingAction(entry.PendingHumanActions), renderPendingAction(entry.Evidence)); err != nil {
		return err
	}
	if entry.Intent.Active {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Activation role: %s\n", entry.ActivationRole); err != nil {
			return err
		}
		for _, choice := range entry.Intent.ProviderChoices {
			if err := renderProviderChoice(cmd.OutOrStdout(), choice); err != nil {
				return err
			}
		}
		for _, consumer := range entry.Consumers {
			resource := "all"
			if consumer.ConsumerResource != nil {
				resource = consumer.ConsumerResource.String()
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Capability consumer: consumer=%s/%s capability=%s\n", consumer.ConsumerPack, resource, consumer.Capability); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Selection mode: %s\n", entry.Intent.Selection.Mode); err != nil {
			return err
		}
		for _, selection := range entry.ResourceSelections {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resource selection: %s role=%s dependency_chain=%s\n",
				selection.Resource, selection.Role, renderIdentityChain(selection.DependencyChain)); err != nil {
				return err
			}
		}
	}
	for _, resource := range entry.Resources {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resource readiness: %s role=%s dependency_chain=%s configured=%s authorized=%s usable=%s projections=%d/%d blockers=%s\n",
			resource.Resource, resource.Role, renderIdentityChain(resource.DependencyChain),
			readinessValue(resource.ReadinessObserved.Configured, resource.Readiness.Configured),
			readinessValue(resource.ReadinessObserved.Authorization, resource.Readiness.Authorized),
			readinessValue(resource.ReadinessObserved.Usability, resource.Readiness.Usable),
			resource.Projections.Verified, projectionCount(resource.Projections), renderPendingAction(resource.Blockers)); err != nil {
			return err
		}
	}
	if focused != nil {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Focused resource: %s configured=%s authorized=%s usable=%s\n",
			focused.Resource,
			readinessValue(focused.ReadinessObserved.Configured, focused.Readiness.Configured),
			readinessValue(focused.ReadinessObserved.Authorization, focused.Readiness.Authorized),
			readinessValue(focused.ReadinessObserved.Usability, focused.Readiness.Usable)); err != nil {
			return err
		}
	}
	optionalAuthorities := append([]capabilitypack.OptionalAuthorityObservation(nil), entry.OptionalAuthorities...)
	sort.Slice(optionalAuthorities, func(i, j int) bool {
		if optionalAuthorities[i].ModeID != optionalAuthorities[j].ModeID {
			return optionalAuthorities[i].ModeID < optionalAuthorities[j].ModeID
		}
		return optionalAuthorities[i].Authority < optionalAuthorities[j].Authority
	})
	for _, authority := range optionalAuthorities {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Optional authority: mode=%s authority=%s state=%s fallback=%s\n", authority.ModeID, authority.Authority, authority.State, authority.Fallback); err != nil {
			return err
		}
	}
	for _, projection := range entry.ProjectionDetails {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Projection: %s target=%s owner=%s health=%s observed=%s desired=%s contributors=%s shared=%s discoverable_by=%s discovery_notice=%s\n", projection.ID, projection.Target, projection.Owner, projection.Health, projection.ObservedFingerprint, projection.DesiredFingerprint, joinFacts(projection.Contributors), yesNo(projection.Shared), joinSurfaces(projection.DiscoverableBy), factOrNone(projection.DiscoveryNotice)); err != nil {
			return err
		}
	}
	if err := renderPackContract(cmd, entry.Contract); err != nil {
		return err
	}
	return renderRuntimeModes(cmd, entry.RuntimeModes)
}

func projectionCount(summary capabilitypack.ProjectionSummary) int {
	return summary.Verified + summary.Missing + summary.Drifted + summary.Ambiguous + summary.Unmanaged
}

func renderIdentityChain(chain []capabilitypack.ResourceIdentity) string {
	if len(chain) == 0 {
		return "none"
	}
	values := make([]string, 0, len(chain))
	for _, identity := range chain {
		values = append(values, identity.String())
	}
	return strings.Join(values, " -> ")
}

func renderRuntimeModes(cmd *cobra.Command, modes []capabilitypack.RuntimeModeResult) error {
	for _, mode := range modes {
		fallbackState := "none"
		if mode.FallbackState != nil {
			fallbackState = string(*mode.FallbackState)
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Runtime mode: resource=%s mode=%s role=%s state=%s on_unavailable=%s fallback=%s fallback_mode=%s fallback_state=%s affected=%s\n",
			mode.ResourceID, mode.ModeID, mode.Role, mode.State, mode.OnUnavailable,
			mode.Fallback.Kind, factOrNone(mode.Fallback.Mode), fallbackState, joinFacts(mode.Affected),
		); err != nil {
			return err
		}
		for _, requirement := range mode.Requirements {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  Requirement: kind=%s id=%s version=%s\n", requirement.Kind, requirement.ID, factOrNone(requirement.Version)); err != nil {
				return err
			}
		}
		for _, authority := range mode.Authorities {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  Authority: kind=%s scope=%s\n", authority.Kind, factOrNone(string(authority.Scope))); err != nil {
				return err
			}
		}
		for _, effect := range mode.Effects {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  Effect: kind=%s scope=%s\n", effect.Kind, factOrNone(string(effect.Scope))); err != nil {
				return err
			}
		}
		for _, observation := range mode.Evidence.Requirements {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"  Requirement evidence: kind=%s id=%s state=%s reason=%s observed_at=%s observer_revision=%s redacted_identity=%s\n",
				observation.Kind, observation.ID, observation.State, observation.Reason, observation.ObservedAt,
				observation.ObserverRevision, factOrNone(observation.RedactedIdentity),
			); err != nil {
				return err
			}
		}
		for _, observation := range mode.Evidence.Authorities {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"  Authority evidence: kind=%s scope=%s state=%s reason=%s observed_at=%s observer_revision=%s redacted_identity=%s\n",
				observation.Kind, observation.Scope, observation.State, observation.Reason, observation.ObservedAt,
				observation.ObserverRevision, factOrNone(observation.RedactedIdentity),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderStatusAction(entry capabilitypack.StatusEntry) string {
	if entry.UpdateAvailable {
		return "update to " + entry.Pack.Version
	}
	return renderPendingAction(entry.PendingHumanActions)
}

func renderUpdateAvailability(entry capabilitypack.StatusEntry) string {
	if !entry.UpdateAvailable {
		return "no"
	}
	return fmt.Sprintf("yes (%s -> %s)", entry.Intent.Version, entry.Pack.Version)
}

func renderIntent(intent capabilitypack.IntentStatus) string {
	if !intent.Active {
		return "inactive"
	}
	if intent.Version == "" {
		return fmt.Sprintf("active at revision %d", intent.Revision)
	}
	return fmt.Sprintf("active at version %s, revision %d", intent.Version, intent.Revision)
}

func renderAttempt(attempt *capabilitypack.AttemptStatus) string {
	if attempt == nil {
		return "none"
	}
	if attempt.PlanID == "" {
		return attempt.Outcome
	}
	return fmt.Sprintf("%s (%s)", attempt.Outcome, attempt.PlanID)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func renderPendingAction(actions []string) string {
	if len(actions) == 0 {
		return "none"
	}
	return strings.Join(actions, "; ")
}

func newPackListCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List available capability packs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog, err := discoverPackCatalog(opts, workstationResolver)
			if err != nil {
				return err
			}
			packs, err := catalog.ListCurrent()
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "PACK\tVERSION\tDESCRIPTION\tAVAILABLE ON")
			for _, pack := range packs {
				surfaces := make([]string, len(pack.Surfaces))
				for i, surface := range pack.Surfaces {
					surfaces[i] = string(surface)
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", pack.ID, pack.Version, pack.Description, strings.Join(surfaces, ", "))
			}
			return writer.Flush()
		},
	}
}

func newPackShowCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "show <pack>", Short: "Show a capability pack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			report, err := facade.Show(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return renderPackShowJSON(cmd.OutOrStdout(), report)
			}
			return renderPackShowHuman(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

type packComposition struct {
	catalog    capabilitypack.Catalog
	state      capabilitypack.StateLayout
	skills     skillbundle.GlobalLayout
	bundleRoot string
	codex      codex.CanonicalLayout
	openCode   opencode.CanonicalLayout
	claude     claudecode.CanonicalLayout
	engram     engrambin.Resolver
}

func resolvePackComposition(opts Options, workstationResolver *workstation.Resolver) (packComposition, error) {
	snapshot, err := workstationResolver.Resolve(workstation.Options{})
	if err != nil {
		return packComposition{}, err
	}
	sources, err := resolveInvocationSources(opts, snapshot)
	if err != nil {
		return packComposition{}, err
	}
	if err := skillbundle.ValidateSource(sources.skills.Root, sources.skills.MissingHint); err != nil {
		return packComposition{}, err
	}
	bundleRoot := skillbundle.BundleRoot(sources.skills.Root)
	catalog, err := capabilitypack.DiscoverForDurableIntents(bundleRoot)
	if err != nil {
		return packComposition{}, err
	}
	engramResolver := engrambin.NewResolver(snapshot.HomebrewPrefix(), opts.Runner.LookPath)
	if opts.EngramFormulaInspector != nil {
		engramResolver = engramResolver.WithFormulaInspector(opts.EngramFormulaInspector)
	}
	return packComposition{
		catalog:    catalog,
		state:      capabilitypack.NewStateLayout(snapshot.PackyHome()),
		skills:     skillbundle.NewGlobalLayout(snapshot.Home()),
		bundleRoot: bundleRoot,
		codex:      codex.NewCanonicalLayout(snapshot.Home()),
		openCode:   opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
		claude:     claudecode.NewCanonicalLayout(snapshot.Home()),
		engram:     engramResolver,
	}, nil
}

func discoverPackCatalog(opts Options, workstationResolver *workstation.Resolver) (capabilitypack.Catalog, error) {
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return capabilitypack.Catalog{}, err
	}
	return composition.catalog, nil
}

func joinSurfaces(values []capabilitypack.Surface) string {
	items := make([]string, len(values))
	for i, value := range values {
		items[i] = string(value)
	}
	return strings.Join(items, ", ")
}
func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
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
	"github.com/yersonargotev/packy/internal/reportredaction"
	"github.com/yersonargotev/packy/internal/skillbundle"
	"github.com/yersonargotev/packy/internal/toolbin"
	packyversion "github.com/yersonargotev/packy/internal/version"
	"github.com/yersonargotev/packy/internal/workstation"
)

const projectLifecycleHelp = `Project installation writes the shared, version-controlled project contract.
Personal runtime activation is a separate phase selected with --project; cloning
or installing never transfers personal trust, credentials, or runtime consent.`

func newPackUninstallCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "uninstall <pack>",
		Short: "Uninstall exact owned projections from the current Git project",
		Args: func(cmd *cobra.Command, args []string) error {
			return projectLifecycleArgs(cmd, args, cobra.ExactArgs(1), jsonOutput, true, "uninstall")
		},
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if runErr != nil {
					runErr = projectLifecycleFailure(cmd, jsonOutput, "uninstall", "command", runErr)
				}
			}()
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
			if err := deactivateBeforeProjectUninstall(cmd, opts, snapshot, args[0], capabilitypack.Surface(surface), projectRoot, dryRun, jsonOutput); err != nil {
				return err
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

func deactivateBeforeProjectUninstall(cmd *cobra.Command, opts Options, snapshot workstation.Snapshot, packID string, selected capabilitypack.Surface, projectRoot string, dryRun, jsonOutput bool) error {
	installation, err := capabilitypack.LoadProjectInstallation(projectRoot)
	if err != nil {
		return err
	}
	var surfaces []capabilitypack.Surface
	for _, pack := range installation.Manifest.Packs {
		if pack.ID == packID {
			surfaces = append(surfaces, pack.Surfaces...)
			break
		}
	}
	if len(surfaces) == 0 {
		return fmt.Errorf("capability pack %q is not declared by this project installation", packID)
	}
	if selected != "" {
		surfaces = []capabilitypack.Surface{selected}
	}
	for _, projectSurface := range surfaces {
		adapter := projectRuntimeAdapter(opts, projectSurface, snapshot)
		preview, previewErr := capabilitypack.PreviewProjectDeactivation(cmd.Context(), capabilitypack.ProjectDeactivationRequest{
			PackID: packID, Surface: projectSurface, ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter,
		})
		if previewErr != nil {
			return previewErr
		}
		if preview.Disposition == capabilitypack.ProjectDeactivationConverged {
			continue
		}
		if jsonOutput {
			output := preview
			output.DryRun = dryRun
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(output); err != nil {
				return err
			}
		} else if err := renderProjectDeactivationPreview(cmd, preview, dryRun); err != nil {
			return err
		}
		if preview.Disposition == capabilitypack.ProjectDeactivationBlocked {
			return errors.New("personal project deactivation is blocked")
		}
		if dryRun {
			continue
		}
		if err := approveAndApplyProjectDeactivation(cmd, opts, preview, adapter, jsonOutput); err != nil {
			return err
		}
	}
	return nil
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
	cmd := &cobra.Command{
		Use:   "install <pack>",
		Short: "Install a capability pack in the current Git project",
		Args: func(cmd *cobra.Command, args []string) error {
			return projectLifecycleArgs(cmd, args, cobra.ExactArgs(1), jsonOutput, true, "install")
		},
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if runErr != nil {
					runErr = projectLifecycleFailure(cmd, jsonOutput, "install", "command", runErr)
				}
			}()
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
			if surface == "" {
				return errors.New("--surface is required when installing a pack")
			}
			composition, err := resolvePackComposition(opts, workstationResolver)
			if err != nil {
				return err
			}
			facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithClock(opts.Clock), capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(composition.state.File()), nil), capabilitypack.WithExternalEffects(composition.tools, nil, nil))
			adapter := projectInstallAdapter(capabilitypack.Surface(surface), composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
			report, err := facade.PreviewProjectInstall(cmd.Context(), capabilitypack.ProjectInstallRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot, Selection: selection, Aliases: aliases}, adapter)
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
				if jsonOutput {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.ProjectInstallApplyResult{
						SchemaVersion: 2,
						Report:        "project-install-apply",
						Status:        "no-op",
						Observation:   report.Observation,
						Readiness:     report.ExpectedReadiness,
						Conditions:    report.Conditions,
					})
				}
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
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Readiness: configured=%s, authorized=%s, usable=%s\n", result.Readiness.Configured, result.Readiness.Authorized, result.Readiness.Usable); err != nil {
				return err
			}
			if err = renderReadinessConditions(cmd.OutOrStdout(), result.Conditions); err != nil {
				return err
			}
			return offerProjectActivation(cmd, opts, facade, report, projectRoot, snapshot.PackyHome(), projectRuntimeAdapter(opts, report.Surface, snapshot))
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the project contract and projections without mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a project surface alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().StringArrayVar(&resourceValues, "resource", nil, "Select one operational project resource (<kind>:<id>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func offerProjectActivation(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, install capabilitypack.JSONProjectInstallPreview, projectRoot, packyHome string, adapter capabilitypack.SurfaceAdapter) error {
	if !capabilitypack.ProjectPackRequiresActivation(install.Lock, install.Pack.ID, install.Surface) {
		return nil
	}
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
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Project installation remains installed; activate later with `packy activate %s --surface %s --project`\n", preview.Pack.ID, preview.Surface)
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
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Reviewed Pack: %s@%s\nLock receipt: %d resources, %d projections\n", report.Pack.ID, report.Pack.Version, len(report.Lock.ResourceGraph.Resources), len(report.Lock.Projections)); err != nil {
		return err
	}
	for _, projection := range report.Projections {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Projection: %s -> %s mode=%s observed=%s\n", projection.Resource, projection.Target, projection.Mode, projection.ObservedState); err != nil {
			return err
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
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Expected readiness: configured=%s, authorized=%s, usable=%s\n", report.ExpectedReadiness.Configured, report.ExpectedReadiness.Authorized, report.ExpectedReadiness.Usable); err != nil {
		return err
	}
	if err := renderReadinessConditions(cmd.OutOrStdout(), report.Conditions); err != nil {
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

func newPackDeactivateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var resourceValues []string
	var jsonOutput bool
	var project bool
	var force bool
	cmd := &cobra.Command{Use: "deactivate <pack>", Short: "Deactivate a capability pack on one CLI surface", Args: func(cmd *cobra.Command, args []string) error {
		return projectLifecycleArgs(cmd, args, cobra.ExactArgs(1), jsonOutput, project, "deactivate")
	}, RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			if project && runErr != nil {
				runErr = projectLifecycleFailure(cmd, jsonOutput, "deactivate", "command", runErr)
			}
		}()
		if project {
			if force {
				return errors.New("--force is accepted only for global deactivation")
			}
			if len(resourceValues) > 0 {
				return errors.New("--resource is not accepted with --project; personal project deactivation consumes exact receipts")
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
			adapter := projectRuntimeAdapter(opts, capabilitypack.Surface(surface), snapshot)
			preview, err := capabilitypack.PreviewProjectDeactivation(cmd.Context(), capabilitypack.ProjectDeactivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot, PackyHome: snapshot.PackyHome(), Adapter: adapter})
			if err != nil {
				return err
			}
			if jsonOutput {
				output := preview
				output.DryRun = dryRun
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(output); err != nil {
					return err
				}
			} else if err := renderProjectDeactivationPreview(cmd, preview, dryRun); err != nil {
				return err
			}
			if preview.Disposition == capabilitypack.ProjectDeactivationBlocked {
				return errors.New("personal project deactivation is blocked")
			}
			if dryRun || preview.Disposition == capabilitypack.ProjectDeactivationConverged {
				return nil
			}
			return approveAndApplyProjectDeactivation(cmd, opts, preview, adapter, jsonOutput)
		}
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
		plan, err := facade.PreviewDeactivate(cmd.Context(), capabilitypack.DeactivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Resources: resources, Force: force})
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
	cmd.Flags().BoolVar(&project, "project", false, "Deactivate exact personal runtime effects for the current project")
	cmd.Flags().BoolVar(&force, "force", false, "Remove drifted paths proven to belong to this installed Pack receipt")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func renderProjectDeactivationPreview(cmd *cobra.Command, preview capabilitypack.JSONProjectDeactivationPreview, dryRun bool) error {
	header := "PERSONAL PROJECT DEACTIVATION PREVIEW"
	if dryRun {
		header = "PERSONAL PROJECT DEACTIVATION DRY-RUN"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\nProject root: %s\nPack: %s %s\nSurface: %s\nRuntime activation: %s\n", header, preview.ProjectRoot, preview.Pack.ID, preview.Pack.Version, preview.Surface, preview.Runtime); err != nil {
		return err
	}
	for _, effect := range preview.Effects {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Remove personal effect: %s target=%s identity=%s consent=%s adapter_provenance=%s\n", effect.Action, effect.Target, effect.Identity, effect.Consent, effect.AdapterProvenance); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Blockers: %s\nDisposition: %s\n", renderProjectInstallBlockers(preview.Blockers), preview.Disposition); err != nil {
		return err
	}
	return nil
}

func approveAndApplyProjectDeactivation(cmd *cobra.Command, opts Options, preview capabilitypack.JSONProjectDeactivationPreview, adapter capabilitypack.SurfaceAdapter, jsonOutput bool) error {
	if !opts.Terminal.Interactive(cmd.InOrStdin()) {
		return capabilitypack.ErrInteractiveRequired
	}
	prompt := fmt.Sprintf("Approve personal project deactivation for exact preview %s?", preview.Digest)
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
		return errors.New("personal project deactivation was not approved")
	}
	result, err := capabilitypack.ApplyProjectDeactivation(cmd.Context(), capabilitypack.ProjectDeactivationApplyRequest{Preview: preview, Adapter: adapter, DestructiveCleanupApproved: true})
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Verified personal project deactivation")
	return err
}

func newPackUpdateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	var project bool
	var force bool
	cmd := &cobra.Command{
		Use: "update <pack>", Short: "Update an active capability pack to the catalog-current version", Args: func(cmd *cobra.Command, args []string) error {
			return projectLifecycleArgs(cmd, args, cobra.ExactArgs(1), jsonOutput, project, "update")
		},
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if project && runErr != nil {
					runErr = projectLifecycleFailure(cmd, jsonOutput, "update", "command", runErr)
				}
			}()
			if project {
				if surface == "" {
					return errors.New("--surface is required for project update")
				}
				if len(aliasValues) > 0 {
					return errors.New("--alias is not accepted for project update")
				}
				return runProjectPackUpdate(cmd, opts, workstationResolver, args[0], capabilitypack.Surface(surface), force, dryRun, jsonOutput)
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
			plan, err := facade.PreviewUpdate(cmd.Context(), capabilitypack.UpdateRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases, Force: force})
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
	cmd.Flags().BoolVar(&project, "project", false, "Update the shared project installation for one CLI surface")
	cmd.Flags().BoolVar(&force, "force", false, "Replace drifted paths proven to belong to this installed Pack receipt")
	return cmd
}

func runProjectPackUpdate(cmd *cobra.Command, opts Options, workstationResolver *workstation.Resolver, packID string, surface capabilitypack.Surface, force, dryRun, jsonOutput bool) error {
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
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return err
	}
	facade := capabilitypack.NewFacade(composition.catalog, capabilitypack.WithClock(opts.Clock))
	adapter := projectInstallAdapter(surface, composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
	report, err := facade.PreviewProjectUpdate(cmd.Context(), capabilitypack.ProjectUpdateRequest{PackID: packID, Surface: surface, ProjectRoot: projectRoot, Force: force}, adapter)
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
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Readiness: configured=%s, authorized=%s, usable=%s\n", readinessValue(result.Readiness.Configured), readinessValue(result.Readiness.Authorized), readinessValue(result.Readiness.Usable)); err != nil {
		return err
	}
	if err := renderReadinessConditions(cmd.OutOrStdout(), result.Conditions); err != nil {
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
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "activate <pack>", Short: "Activate a capability pack on one CLI surface", Args: func(cmd *cobra.Command, args []string) error {
			return projectLifecycleArgs(cmd, args, cobra.ExactArgs(1), jsonOutput, project, "activate")
		},
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if project && runErr != nil {
					runErr = projectLifecycleFailure(cmd, jsonOutput, "activate", "command", runErr)
				}
			}()
			if project {
				if len(aliasValues) > 0 || len(resourceValues) > 0 {
					return errors.New("--alias and --resource are not accepted with --project; project activation consumes the exact installed lock")
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
				facade := capabilitypack.NewFacade(capabilitypack.Catalog{}, capabilitypack.WithExternalEffects(projectExecutableResolver(opts, snapshot), nil, nil))
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
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			plan, err := facade.Preview(cmd.Context(), capabilitypack.ActivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases, Selection: selection})
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
		return withControlledCheckFacts(opts, surface, codex.NewSurfaceAdapterWithConfig("", "", layout.PromptFile(), layout.ConfigFile()))
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
		return withControlledCheckFacts(opts, surface, adapter)
	}
	return withControlledCheckFacts(opts, surface, projectOfflineAdapter(surface))
}

func projectStatusAdapters(opts Options, snapshot workstation.Snapshot) map[capabilitypack.Surface]capabilitypack.SurfaceAdapter {
	return map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
		capabilitypack.SurfaceClaude:   projectRuntimeAdapter(opts, capabilitypack.SurfaceClaude, snapshot),
		capabilitypack.SurfaceCodex:    projectRuntimeAdapter(opts, capabilitypack.SurfaceCodex, snapshot),
		capabilitypack.SurfaceOpenCode: projectRuntimeAdapter(opts, capabilitypack.SurfaceOpenCode, snapshot),
	}
}

func projectExecutableResolver(opts Options, snapshot workstation.Snapshot) capabilitypack.ExecutableResolver {
	return toolbin.NewPATHResolver(opts.Runner.LookPath)
}

func projectStatusFacade(opts Options, snapshot workstation.Snapshot) capabilitypack.Facade {
	adapters := projectStatusAdapters(opts, snapshot)
	return capabilitypack.NewFacade(capabilitypack.Catalog{},
		capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(snapshot.PackyHome()).File()), adapters),
		capabilitypack.WithExternalEffects(projectExecutableResolver(opts, snapshot), nil, nil),
		capabilitypack.WithControlledCheckEvidence(capabilitypack.NewFileControlledCheckStore(snapshot.PackyHome())),
	)
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
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified personal project activation %s\nReadiness: configured=%s, authorized=%s, usable=%s\n", result.Digest, result.Readiness.Configured, result.Readiness.Authorized, result.Readiness.Usable); err != nil {
		return err
	}
	return renderReadinessConditions(cmd.OutOrStdout(), result.Conditions)
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
	if err := renderProjectRuntimeEffects(cmd.OutOrStdout(), preview.RuntimeEffects); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Expected readiness: configured=%s, authorized=%s, usable=%s\n", preview.ExpectedReadiness.Configured, preview.ExpectedReadiness.Authorized, preview.ExpectedReadiness.Usable); err != nil {
		return err
	}
	if err := renderReadinessConditions(cmd.OutOrStdout(), preview.Conditions); err != nil {
		return err
	}
	for _, effect := range preview.Effects {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Personal effect: %s -> %s identity=%s\n", effect.Action, effect.Target, effect.Identity); err != nil {
			return err
		}
	}
	return nil
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

func projectLifecycleFailure(cmd *cobra.Command, jsonOutput bool, operation, stage string, err error) error {
	err = reportredaction.Error(err, nil, projectLifecycleArgumentValues(cmd))
	if jsonOutput {
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.JSONProjectFailureFor(operation, stage, err))
	}
	return err
}

func projectLifecycleArgs(cmd *cobra.Command, args []string, validate cobra.PositionalArgs, jsonOutput, project bool, operation string) error {
	err := validate(cmd, args)
	if err == nil || !project {
		return err
	}
	return projectLifecycleFailure(cmd, jsonOutput, operation, "command", err)
}

func projectLifecycleArgumentValues(cmd *cobra.Command) []string {
	values := append([]string(nil), cmd.Flags().Args()...)
	for _, name := range []string{"alias", "resource"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}
		entries, err := cmd.Flags().GetStringArray(name)
		if err == nil {
			values = append(values, entries...)
		}
	}
	for _, name := range []string{"surface", "version"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}
		value, err := cmd.Flags().GetString(name)
		if err == nil {
			values = append(values, value)
		}
	}
	return values
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

func readinessValue(value capabilitypack.ReadinessValue) string {
	return string(value)
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
	claudeState, err := store.LoadSnapshot(context.Background(), capabilitypack.SurfaceClaude)
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
			capabilitypack.SurfaceCodex:    withControlledCheckFacts(opts, capabilitypack.SurfaceCodex, codexAdapter),
			capabilitypack.SurfaceOpenCode: withControlledCheckFacts(opts, capabilitypack.SurfaceOpenCode, openCodeAdapter),
			capabilitypack.SurfaceClaude:   withControlledCheckFacts(opts, capabilitypack.SurfaceClaude, claudeAdapter),
		}
	}
	return capabilitypack.NewFacade(composition.catalog,
		capabilitypack.WithActivation(store, adapters),
		capabilitypack.WithControlledCheckEvidence(capabilitypack.NewFileControlledCheckStore(filepath.Dir(composition.state.File()))),
		capabilitypack.WithExternalEffects(
			composition.tools,
			externalToolAcquirers(composition.engram),
			runnerExternalExecutor{runner: opts.Runner},
		),
	), nil
}

func withControlledCheckFacts(opts Options, surface capabilitypack.Surface, adapter capabilitypack.SurfaceAdapter) capabilitypack.SurfaceAdapter {
	return capabilitypack.WithControlledCheckDescriptor(adapter, capabilitypack.ControlledCheckDescriptor{
		AdapterVersion: "packy/" + packyversion.Value + "/" + string(surface) + "-surface/v1",
		HostVersion:    observableSurfaceVersion(context.Background(), opts.Runner, string(surface)),
		Instructions:   []string{fmt.Sprintf("In a fresh %s session, exercise the selected Pack behavior and record whether it succeeds.", surface)},
	})
}

func observableSurfaceVersion(ctx context.Context, runner Runner, command string) string {
	path, err := runner.LookPath(command)
	if err != nil {
		return "unobservable"
	}
	output, ok := runner.(outputRunner)
	if !ok {
		return "unobservable"
	}
	stdout, stderr, exitCode, err := output.RunOutput(ctx, path, "--version")
	if err != nil || exitCode != 0 {
		return "unobservable"
	}
	outputText := strings.TrimSpace(stdout)
	if outputText == "" {
		outputText = strings.TrimSpace(stderr)
	}
	if index := strings.IndexByte(outputText, '\n'); index >= 0 {
		outputText = outputText[:index]
	}
	version := observableVersionPattern.FindString(outputText)
	if version == "" {
		return "unobservable"
	}
	return command + "/" + version
}

var observableVersionPattern = regexp.MustCompile(`\bv?[0-9]+(?:\.[0-9]+)+(?:[-+][0-9A-Za-z.-]+)?\b`)

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
	}
	if dryRun {
		prefix = strings.TrimSuffix(prefix, " plan") + " dry-run plan"
	}
	packLabel := plan.Pack().ID
	if plan.Operation() != capabilitypack.OperationUpdate && plan.Operation() != capabilitypack.OperationDeactivate {
		packLabel += " " + plan.Pack().Version
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDigest: %s\nPack: %s\nSurface: %s\n", prefix, plan.ID(), plan.Digest(), packLabel, plan.Surface()); err != nil {
		return err
	}
	if plan.Operation() == capabilitypack.OperationUpdate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s -> %s (catalog-current)\nIntent revision: %d\n", plan.OldVersion(), plan.Pack().Version, plan.IntentRevision()); err != nil {
			return err
		}
	} else if plan.Operation() == capabilitypack.OperationDeactivate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Active version: %s\nIntent revision: %d\n", plan.OldVersion(), plan.IntentRevision()); err != nil {
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
	readiness := plan.Readiness()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Expected readiness: configured=%s, authorized=%s, usable=%s\nObserved evidence: %s\nPending evidence: %s\n", readinessValue(readiness.Configured), readinessValue(readiness.Authorized), readinessValue(readiness.Usable), renderPendingAction(plan.Evidence()), renderPendingAction(plan.PendingEvidence())); err != nil {
		return err
	}
	if err := renderReadinessConditions(cmd.OutOrStdout(), plan.Conditions()); err != nil {
		return err
	}
	structured := plan.JSONReport(dryRun)
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contract diff: added=%s changed=%s removed=%s retained=%s\n", joinFacts(structured.ContractDiff.Added), joinFacts(structured.ContractDiff.Changed), joinFacts(structured.ContractDiff.Removed), joinFacts(structured.ContractDiff.Retained)); err != nil {
		return err
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

func renderActivationPlanOutput(cmd *cobra.Command, plan capabilitypack.ReconciliationPlan, dryRun, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan.JSONReport(dryRun))
	}
	return renderActivationPlan(cmd, plan, dryRun)
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

func newControlledCheckCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface, result string
	var project, dryRun bool
	var resourceValues []string
	cmd := &cobra.Command{
		Use:   "check <pack>",
		Short: "Record a controlled runtime check separately from Pack activation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selection := capabilitypack.ResourceSelection{Mode: capabilitypack.SelectionAll}
			if len(resourceValues) > 0 {
				if project {
					return errors.New("--resource is not supported with --project; the installed project closure is checked exactly")
				}
				selection.Mode = capabilitypack.SelectionCustom
				for _, value := range resourceValues {
					resource, err := capabilitypack.ParseResourceIdentity(value)
					if err != nil {
						return err
					}
					selection.Roots = append(selection.Roots, resource)
				}
			}
			if !dryRun && result != "positive" && result != "negative" {
				return errors.New("--result must be positive or negative when recording a controlled runtime check")
			}
			snapshot, err := workstationResolver.Resolve(workstation.Options{})
			if err != nil {
				return err
			}
			projectRoot := ""
			adapter := capabilitypack.SurfaceAdapter(nil)
			if project {
				cwd, cwdErr := snapshot.CurrentDirectory()
				if cwdErr != nil {
					return fmt.Errorf("resolve current directory: %w", cwdErr)
				}
				projectRoot, err = capabilitypack.DiscoverProjectRoot(cwd)
				if err != nil {
					return err
				}
				adapter = projectRuntimeAdapter(opts, capabilitypack.Surface(surface), snapshot)
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			preview, err := facade.PreviewControlledCheck(cmd.Context(), capabilitypack.ControlledCheckRequest{
				PackID: args[0], Surface: capabilitypack.Surface(surface), ProjectRoot: projectRoot,
				PackyHome: snapshot.PackyHome(), Selection: selection, Adapter: adapter,
			})
			if err != nil {
				return err
			}
			if err := renderControlledCheckPreview(cmd.OutOrStdout(), preview, dryRun); err != nil {
				return err
			}
			if dryRun {
				return nil
			}
			if !opts.Terminal.Interactive(cmd.InOrStdin()) {
				return capabilitypack.ErrInteractiveRequired
			}
			prompt := fmt.Sprintf("Record a %s controlled runtime result for exact identity %s?", result, preview.ValidityIdentity)
			approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), prompt)
			if err != nil {
				return err
			}
			if !approved {
				return errors.New("controlled runtime check result was not approved")
			}
			value := capabilitypack.ReadinessTrue
			if result == "negative" {
				value = capabilitypack.ReadinessFalse
			}
			evidence, err := facade.RecordControlledCheck(cmd.Context(), preview, value)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s controlled runtime evidence at %s\n", result, evidence.ObservedAt)
			return err
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().StringVar(&result, "result", "", "Observed result to record (positive or negative)")
	cmd.Flags().BoolVar(&project, "project", false, "Check the current project's installed Pack closure")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the exact controlled check without recording evidence")
	cmd.Flags().StringArrayVar(&resourceValues, "resource", nil, "Select one Pack resource root (<kind>:<id>); repeatable")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func renderControlledCheckPreview(w io.Writer, preview capabilitypack.ControlledCheckPreview, dryRun bool) error {
	header := "CONTROLLED RUNTIME CHECK PREVIEW"
	if dryRun {
		header = "CONTROLLED RUNTIME CHECK DRY-RUN"
	}
	if _, err := fmt.Fprintf(w, "%s\nPack: %s %s\nSurface: %s\nScope: %s\nSelected resource closure: %s\nProjection revision: %s\nAdapter version: %s\nObservable host version: %s\nValidity identity: %s\nExisting evidence: %s\n", header, preview.Pack, preview.PackVersion, preview.Surface, preview.Scope, renderIdentityChain(preview.Resources), preview.ProjectionRevision, preview.AdapterVersion, preview.HostVersion, preview.ValidityIdentity, preview.CurrentEvidence.State); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Check instructions:"); err != nil {
		return err
	}
	for _, instruction := range preview.Instructions {
		if _, err := fmt.Fprintf(w, "  - %s\n", instruction); err != nil {
			return err
		}
	}
	return nil
}

func newPackStatusCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var resource string
	var require string
	var jsonOutput bool
	var project bool
	cmd := &cobra.Command{
		Use: "status [pack]", Short: "Inspect capability pack status", Args: func(cmd *cobra.Command, args []string) error {
			return projectLifecycleArgs(cmd, args, cobra.MaximumNArgs(1), jsonOutput, project, "status")
		},
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if project && runErr != nil {
					runErr = projectLifecycleFailure(cmd, jsonOutput, "status", "command", runErr)
				}
			}()
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
					Adapters: projectStatusAdapters(opts, snapshot), Resolver: projectExecutableResolver(opts, snapshot),
				}
				facade := projectStatusFacade(opts, snapshot)
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
					fullFacade, facadeErr := activationFacade(opts, workstationResolver)
					if facadeErr != nil {
						return facadeErr
					}
					facade = fullFacade
				}
				report, err := facade.InspectProjectStatus(cmd.Context(), statusRequest)
				if err != nil {
					return err
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

func newProjectVerifyCommand(getwd func() (string, error)) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the current project's committed Pack contract",
		Long:  "Verify packy.json, packy.lock.json, required notices, and every locked project projection without reading or changing personal runtime state.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := getwd()
			if err != nil {
				return fmt.Errorf("resolve current directory: %w", err)
			}
			projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
			if err != nil {
				return err
			}
			report := capabilitypack.VerifyProject(cmd.Context(), projectRoot, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
				capabilitypack.SurfaceClaude:   projectOfflineAdapter(capabilitypack.SurfaceClaude),
				capabilitypack.SurfaceCodex:    projectOfflineAdapter(capabilitypack.SurfaceCodex),
				capabilitypack.SurfaceOpenCode: projectOfflineAdapter(capabilitypack.SurfaceOpenCode),
			})
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
					return err
				}
			} else if err := renderProjectVerification(cmd, report); err != nil {
				return err
			}
			if report.Result == capabilitypack.ProjectVerificationFailed {
				return errors.New("project Pack verification failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func renderProjectVerification(cmd *cobra.Command, report capabilitypack.ProjectVerificationReport) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Project Pack verification: %s\nPacks: %d; surfaces: %d; projections: %d/%d verified; findings: %d\n", report.Result, report.Summary.Packs, report.Summary.Surfaces, report.Summary.Verified, report.Summary.Projections, report.Summary.Findings); err != nil {
		return err
	}
	for _, finding := range report.Findings {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Finding: %s: %s\nRemediation: %s\n", finding.Code, finding.Detail, finding.Remediation); err != nil {
			return err
		}
	}
	for _, entry := range report.Entries {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s: %s; projections=%d/%d verified\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, entry.Installation, entry.Verified, entry.Projections); err != nil {
			return err
		}
		for _, finding := range entry.Findings {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  Finding: %s: %s\n  Remediation: %s\n", finding.Code, finding.Detail, finding.Remediation); err != nil {
				return err
			}
		}
	}
	return nil
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
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s (project)\nInstallation: %s\nRuntime activation: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nControlled runtime check: %s result=%s observed_at=%s\nProjections: %d\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", status.Pack.ID, status.Pack.Version, status.Surface, status.Installation, status.Runtime, status.Readiness.Configured, status.Readiness.Authorized, status.Readiness.Usable, status.ControlledCheck.State, status.ControlledCheck.Result, status.ControlledCheck.ObservedAt, len(status.Projections), renderProjectInstallBlockers(status.Blockers), renderPendingAction(status.PendingHumanActions), renderPendingAction(status.Evidence)); err != nil {
			return err
		}
		if err := renderProjectRuntimeEffects(cmd.OutOrStdout(), status.RuntimeEffects); err != nil {
			return err
		}
		if err := renderReadinessConditions(cmd.OutOrStdout(), status.Conditions); err != nil {
			return err
		}
	}
	return nil
}

func renderProjectRuntimeEffects(w io.Writer, effects []capabilitypack.ProjectRuntimeEffectStatus) error {
	for _, effect := range effects {
		if _, err := fmt.Fprintf(w, "Runtime effect: %s %s %s coverage=%s", effect.Category, effect.Resource, effect.Detail, effect.Coverage); err != nil {
			return err
		}
		if effect.GlobalVersion != "" {
			if _, err := fmt.Fprintf(w, " global_version=%s", effect.GlobalVersion); err != nil {
				return err
			}
		}
		if effect.Conflict != "" {
			if _, err := fmt.Fprintf(w, " conflict=%s", effect.Conflict); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
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
	fmt.Fprintln(writer, "PACK\tSURFACE\tINTENT\tCONFIGURED\tAUTHORIZED\tUSABLE\tACTION")
	for _, entry := range report.Entries {
		configured := readinessValue(entry.Readiness.Configured)
		authorized := readinessValue(entry.Readiness.Authorized)
		usable := readinessValue(entry.Readiness.Usable)
		intent := renderIntent(entry.Intent)
		if entry.LifecycleState == capabilitypack.PackLifecycleInactiveWithResiduals {
			intent = string(entry.LifecycleState)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.Pack.ID, entry.Surface, intent, configured, authorized, usable, renderStatusAction(entry))
	}
	return writer.Flush()
}

func renderPackStatusDetail(cmd *cobra.Command, entry capabilitypack.StatusEntry, focused *capabilitypack.ResourceStatus) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\nIntent: %s\nLifecycle state: %s\nUpdate available: %s\nResources: %d selected\nReadiness: configured=%s, authorized=%s, usable=%s\nControlled runtime check: %s result=%s observed_at=%s\nReceipt ownership: %d projected paths\nDrift: %d projections\nProjections: %d verified; %d drifted; %d ambiguous; %d missing; %d unmanaged\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, renderIntent(entry.Intent), entry.LifecycleState, renderUpdateAvailability(entry), len(entry.Resources), readinessValue(entry.Readiness.Configured), readinessValue(entry.Readiness.Authorized), readinessValue(entry.Readiness.Usable), entry.ControlledCheck.State, entry.ControlledCheck.Result, entry.ControlledCheck.ObservedAt, receiptOwnershipCount(entry.ProjectionDetails), receiptDriftCount(entry.ProjectionDetails), entry.Projections.Verified, entry.Projections.Drifted, entry.Projections.Ambiguous, entry.Projections.Missing, entry.Projections.Unmanaged, renderPendingAction(entry.Blockers), renderPendingAction(entry.PendingHumanActions), renderPendingAction(entry.Evidence)); err != nil {
		return err
	}
	if entry.Intent.Active {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Activation role: %s\n", entry.ActivationRole); err != nil {
			return err
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
			readinessValue(resource.Readiness.Configured),
			readinessValue(resource.Readiness.Authorized),
			readinessValue(resource.Readiness.Usable),
			resource.Projections.Verified, projectionCount(resource.Projections), renderPendingAction(resource.Blockers)); err != nil {
			return err
		}
	}
	if focused != nil {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Focused resource: %s configured=%s authorized=%s usable=%s\n",
			focused.Resource,
			readinessValue(focused.Readiness.Configured),
			readinessValue(focused.Readiness.Authorized),
			readinessValue(focused.Readiness.Usable)); err != nil {
			return err
		}
	}
	if err := renderReadinessConditions(cmd.OutOrStdout(), entry.Conditions); err != nil {
		return err
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
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Projection: %s target=%s owner=%s health=%s observed=%s desired=%s\n", projection.ID, projection.Target, projection.Owner, projection.Health, projection.ObservedFingerprint, projection.DesiredFingerprint); err != nil {
			return err
		}
	}
	if err := renderPackContract(cmd, entry.Contract); err != nil {
		return err
	}
	return renderRuntimeModes(cmd, entry.RuntimeModes)
}

func renderReadinessConditions(w io.Writer, conditions []capabilitypack.ReadinessCondition) error {
	for _, condition := range conditions {
		if _, err := fmt.Fprintf(w, "Readiness condition: type=%s scope=%s pack=%s surface=%s dimension=%s value=%s reason=%s message=%s evidence=%s observed_at=%s validity_identity=%s\n", condition.Type, condition.Scope.Kind, condition.Scope.Pack, condition.Scope.Surface, condition.Dimension, condition.Value, condition.Reason, condition.Message, renderPendingAction(condition.Evidence), condition.Freshness.ObservedAt, condition.Freshness.ValidityIdentity); err != nil {
			return err
		}
	}
	return nil
}

func receiptOwnershipCount(projections []capabilitypack.ProjectionStatus) int {
	count := 0
	for _, projection := range projections {
		if projection.Owner == "packy" {
			count++
		}
	}
	return count
}

func receiptDriftCount(projections []capabilitypack.ProjectionStatus) int {
	count := 0
	for _, projection := range projections {
		if projection.Owner == "packy" && projection.Health != capabilitypack.ProjectionVerified {
			count++
		}
	}
	return count
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
	tools      toolbin.PATHResolver
	engram     engrambin.Acquirer
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
	engramAcquirer := engrambin.NewAcquirer(snapshot.HomebrewPrefix())
	if opts.EngramFormulaInspector != nil {
		engramAcquirer = engramAcquirer.WithFormulaInspector(opts.EngramFormulaInspector)
	}
	return packComposition{
		catalog:    catalog,
		state:      capabilitypack.NewStateLayout(snapshot.PackyHome()),
		skills:     skillbundle.NewGlobalLayout(snapshot.Home()),
		bundleRoot: bundleRoot,
		codex:      codex.NewCanonicalLayout(snapshot.Home()),
		openCode:   opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
		claude:     claudecode.NewCanonicalLayout(snapshot.Home()),
		tools:      toolbin.NewPATHResolver(opts.Runner.LookPath),
		engram:     engramAcquirer,
	}, nil
}

func externalToolAcquirers(engram engrambin.Acquirer) map[capabilitypack.SurfaceCapabilityType]capabilitypack.ExecutableAcquirer {
	return map[capabilitypack.SurfaceCapabilityType]capabilitypack.ExecutableAcquirer{
		capabilitypack.SurfaceCapabilityExternalExecutableAcquisition: engram,
	}
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

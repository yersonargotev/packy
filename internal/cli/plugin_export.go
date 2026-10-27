package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/agents"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/skilllayout"
	"github.com/yersonargotev/packy/internal/workstation"
)

type pluginExportReport struct {
	SchemaVersion int    `json:"schema_version"`
	Report        string `json:"report"`
	Result        string `json:"result"`
	*capabilitypack.PluginExportResult
	Error *catalogValidationError `json:"error,omitempty"`
}

func newPluginExportCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	var surface, snapshotID, output string
	var skills []string
	var jsonOutput bool
	failure := func(cmd *cobra.Command, code string, err error) error {
		if jsonOutput {
			if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(pluginExportReport{SchemaVersion: 1, Report: "plugin-export", Result: "failed", Error: &catalogValidationError{Code: code, Message: err.Error()}}); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	command := &cobra.Command{Use: "export <pack>", Short: "Export reviewed skills as a standalone local plugin", Long: "Export one reviewed Pack as an Agent Plugins package with --surface agents or a Claude plugin with --surface claude. Defaults to all format-eligible skills; --skill selects logical skill IDs and their complete native dependency closure. Uses the selected or explicitly retained local Catalog Snapshot without acquisition, installation, registration, or content execution. The output must be a new or empty directory whose parent already exists. Native clients own plugin installation and enablement; declared prerequisites are not verified.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if (surface != "agents" && surface != "claude") || output == "" {
			return failure(cmd, "invalid_arguments", fmt.Errorf("export requires --surface agents or claude and --output <new-or-empty-directory>"))
		}
		for _, skill := range skills {
			if skill == "" {
				return failure(cmd, "invalid_arguments", fmt.Errorf("--skill must name a logical skill ID"))
			}
		}
		station, err := resolver.Resolve(workstation.Options{})
		if err != nil {
			return failure(cmd, "catalog_unavailable", err)
		}
		var root, identity string
		if opts.localCandidate != nil {
			identity = snapshotID
			if identity == "" {
				identity = opts.localCandidate.Snapshot.ID
			}
			root, err = opts.localCandidate.Resolve(cmd.Context(), identity)
		} else {
			store := catalogstore.New(catalogstore.DefaultDataRoot(station.Home()), nil)
			if snapshotID != "" {
				identity = snapshotID
				root, err = store.Resolve(identity)
			} else {
				var snapshot catalogstore.Snapshot
				snapshot, err = store.Selected()
				root, identity = snapshot.Root, snapshot.ID
			}
		}
		if err != nil {
			code := "catalog_unavailable"
			if snapshotID != "" {
				code = "snapshot_unavailable"
			}
			return failure(cmd, code, fmt.Errorf("local retained Catalog Snapshot unavailable; acquire explicitly with packy init or packy catalog refresh, or restore the exact requested snapshot: %w", err))
		}
		destination := output
		if !filepath.IsAbs(destination) {
			directory, err := opts.Getwd()
			if err != nil {
				return failure(cmd, "export_failed", err)
			}
			destination = filepath.Join(directory, destination)
		}
		// Keep Packy-managed inputs/state and native client content outside caller output.
		protected := []string{root, catalogstore.DefaultDataRoot(station.Home()), station.PackyHome()}
		protected = append(protected, pluginClientContentDirectories(station.Home())...)
		directory, wdErr := opts.Getwd()
		if wdErr != nil {
			return failure(cmd, "export_failed", wdErr)
		}
		protected = append(protected, pluginClientContentDirectories(directory)...)
		if opts.localCandidate != nil {
			protected = append(protected, filepath.Dir(opts.localCandidate.Home))
		}
		for _, boundary := range protected {
			if err := checkPluginOutputBoundary(destination, boundary); err != nil {
				return failure(cmd, "export_failed", err)
			}
		}
		result, err := capabilitypack.ExportPlugin(cmd.Context(), root, identity, args[0], capabilitypack.Surface(surface), skills, destination)
		if err != nil {
			return failure(cmd, "export_failed", err)
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(pluginExportReport{SchemaVersion: 1, Report: "plugin-export", Result: "exported", PluginExportResult: &result})
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported %s@%s as %s (%s)\nCatalog Snapshot: %s\nOutput directory: %s\nSelected skills: %v\nContent SHA256: %s\nDeclared prerequisites (not verified): %v\nPackage generated; installation and enablement belong to the consuming client.\n", result.PackID, result.PackVersion, result.PluginName, result.Format, result.SnapshotID, result.OutputDirectory, result.SelectedSkills, result.ContentSHA256, result.Prerequisites)
		return err
	}}
	command.Flags().StringVar(&surface, "surface", "", "explicit projection convention: agents (Agent Plugins) or claude (Claude plugin)")
	command.Flags().StringVar(&snapshotID, "snapshot", "", "reuse this exact retained Catalog Snapshot")
	command.Flags().StringVar(&output, "output", "", "caller-selected new or empty output directory")
	command.Flags().StringSliceVar(&skills, "skill", nil, "logical skill IDs to export (repeat or comma-separate; default: all eligible)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit versioned plugin-export JSON")
	configureCatalogArguments(command, &jsonOutput, failure)
	return command
}

func checkPluginOutputBoundary(output, boundary string) error {
	canonical := func(value string) string {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return filepath.Clean(value)
		}
		suffix := []string{}
		for {
			resolved, err := filepath.EvalSymlinks(absolute)
			if err == nil {
				for i := len(suffix) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, suffix[i])
				}
				return resolved
			}
			if !os.IsNotExist(err) || filepath.Dir(absolute) == absolute {
				return absolute
			}
			suffix = append(suffix, filepath.Base(absolute))
			absolute = filepath.Dir(absolute)
		}
	}
	output, boundary = canonical(output), canonical(boundary)
	contains := func(root, target string) bool {
		relative, err := filepath.Rel(root, target)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	if contains(boundary, output) || contains(output, boundary) {
		return fmt.Errorf("export output must be separate from managed input and client state %s", boundary)
	}
	return nil
}

// Obtain convention roots from their owners instead of duplicating host layouts.
func pluginClientContentDirectories(root string) []string {
	return []string{filepath.Dir(skilllayout.NewGlobalLayout(root).Root()), claudecode.NewCanonicalLayout(root).ConfigDir, filepath.Dir(agents.NewCanonicalLayout(root).RolesDir())}
}

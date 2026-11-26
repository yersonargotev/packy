package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/workstation"
)

type catalogListReport struct {
	SchemaVersion int                `json:"schema_version"`
	Report        string             `json:"report"`
	Catalogs      []catalogListEntry `json:"catalogs"`
}

type catalogListEntry struct {
	Repository string  `json:"repository"`
	Official   bool    `json:"official"`
	Trust      string  `json:"trust"`
	Workflow   string  `json:"workflow,omitempty"`
	SnapshotID *string `json:"snapshot_id"`
}

type catalogRefreshReport struct {
	SchemaVersion int                   `json:"schema_version"`
	Report        string                `json:"report"`
	Catalogs      []catalogRefreshEntry `json:"catalogs"`
}

type catalogRefreshEntry struct {
	Repository string  `json:"repository"`
	Result     string  `json:"result"`
	SnapshotID *string `json:"snapshot_id"`
	Error      string  `json:"error,omitempty"`
}

func newCatalogAddCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	var trust, workflow string
	command := &cobra.Command{
		Use: "add <owner/repo>", Short: "Register a GitHub catalog and select its first Catalog Snapshot", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := catalogstore.ParseRepository(args[0])
			if err != nil {
				return err
			}
			if trust == "access-controlled" {
				return fmt.Errorf("catalog %s: --trust access-controlled arrives with private catalog support; this Packy registers only --trust attested catalogs", repository)
			}
			policy, err := catalogstore.ParseTrustPolicy(trust, workflow)
			if err != nil {
				return err
			}
			registration := catalogstore.Registration{Repository: repository, Trust: policy}
			store, _, err := registryStore(opts, resolver)
			if err != nil {
				return err
			}
			snapshot, err := store.Register(cmd.Context(), registration)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy catalog add: registered %s (attested by %s) and selected Catalog Snapshot %s\n", repository, workflow, snapshot.ID)
			return err
		},
	}
	command.Flags().StringVar(&trust, "trust", "attested", "trust policy of the catalog, which is attested")
	command.Flags().StringVar(&workflow, "workflow", catalogstore.DefaultWorkflow, "publication workflow that must sign attested Catalog Snapshots")
	return command
}

func newCatalogRemoveCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	return &cobra.Command{
		Use: "remove <owner/repo>", Short: "Unregister a catalog that no installed Pack uses; retained Catalog Snapshots stay", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := catalogstore.ParseRepository(args[0])
			if err != nil {
				return err
			}
			store, station, err := registryStore(opts, resolver)
			if err != nil {
				return err
			}
			receipts := capabilitypack.NewFileActivationStore(capabilitypack.NewStateLayout(station.PackyHome()).File())
			inUse := func(ctx context.Context, repository catalogstore.Repository) error {
				installed, err := capabilitypack.InstalledFromCatalog(ctx, receipts, repository.String())
				if err != nil {
					return fmt.Errorf("check installed Pack receipts before removing catalog %s: %w", repository, err)
				}
				activated, err := capabilitypack.ProjectActivationsFromCatalog(station.PackyHome(), repository.String())
				if err != nil {
					return fmt.Errorf("check personal project activations before removing catalog %s: %w", repository, err)
				}
				if len(installed) == 0 && len(activated) == 0 {
					return nil
				}
				names := make([]string, 0, len(installed)+len(activated))
				for _, intent := range installed {
					names = append(names, fmt.Sprintf("%s on %s", intent.PackID, intent.Surface))
				}
				for _, intent := range activated {
					names = append(names, fmt.Sprintf("%s on %s in a project", intent.PackID, intent.Surface))
				}
				return fmt.Errorf("catalog %s is in use by installed Packs %s; deactivate them, then remove the catalog", repository, strings.Join(names, ", "))
			}
			if err := store.Unregister(cmd.Context(), repository, inUse); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy catalog remove: removed catalog %s; its retained Catalog Snapshots stay on disk\n", repository)
			return err
		},
	}
}

func newCatalogListCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "list", Short: "List registered catalogs with their trust policy and selected Catalog Snapshot", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, _, err := registryStore(opts, resolver)
			if err != nil {
				return err
			}
			registrations, err := store.Registrations()
			if err != nil {
				return err
			}
			report := catalogListReport{SchemaVersion: 1, Report: "catalog-list", Catalogs: []catalogListEntry{}}
			for _, registration := range registrations {
				entry := catalogListEntry{Repository: registration.Repository.String(), Official: registration.Repository == catalogstore.Official().Repository}
				entry.Trust, entry.Workflow = catalogstore.TrustPolicyName(registration.Trust)
				entry.SnapshotID, err = selectedSnapshotID(store, registration.Repository)
				if err != nil {
					return err
				}
				report.Catalogs = append(report.Catalogs, entry)
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}
			return renderCatalogList(cmd.OutOrStdout(), report)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit versioned catalog-list JSON")
	return command
}

func newCatalogRefreshCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "refresh [<owner/repo>]", Short: "Acquire and select the latest Catalog Snapshot of every registered catalog, or of one", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := registryStore(opts, resolver)
			if err != nil {
				return err
			}
			var repositories []catalogstore.Repository
			if len(args) == 1 {
				repository, err := catalogstore.ParseRepository(args[0])
				if err != nil {
					return err
				}
				repositories = append(repositories, repository)
			} else {
				registrations, err := store.Registrations()
				if err != nil {
					return err
				}
				for _, registration := range registrations {
					repositories = append(repositories, registration.Repository)
				}
			}
			report := catalogRefreshReport{SchemaVersion: 1, Report: "catalog-refresh", Catalogs: []catalogRefreshEntry{}}
			failed := 0
			for _, repository := range repositories {
				entry := catalogRefreshEntry{Repository: repository.String(), Result: "selected"}
				snapshot, refreshErr := store.AcquireLatest(cmd.Context(), repository)
				if refreshErr == nil {
					entry.SnapshotID = &snapshot.ID
				} else {
					failed++
					entry.Result, entry.Error = "failed", refreshErr.Error()
					previous, err := selectedSnapshotID(store, repository)
					if err != nil {
						entry.Error += "; the previous selection is unreadable: " + err.Error()
					}
					entry.SnapshotID = previous
				}
				report.Catalogs = append(report.Catalogs, entry)
				if !jsonOutput {
					if err := renderCatalogRefresh(cmd.OutOrStdout(), entry); err != nil {
						return err
					}
				}
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d catalogs failed to refresh and keep their previous Catalog Snapshot", failed, len(repositories))
			}
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit versioned catalog-refresh JSON")
	return command
}

func registryStore(opts Options, resolver *workstation.Resolver) (*catalogstore.Store, workstation.Snapshot, error) {
	station, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return nil, workstation.Snapshot{}, err
	}
	return newCatalogStore(opts, station), station, nil
}

// selectedSnapshotID reports no selection when the catalog has never selected
// a Catalog Snapshot, and fails on any other unreadable selection.
func selectedSnapshotID(store *catalogstore.Store, repository catalogstore.Repository) (*string, error) {
	snapshot, err := store.Selected(repository)
	if errors.Is(err, fs.ErrNotExist) && !errors.Is(err, catalogstore.ErrEarlierLayout) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snapshot.ID, nil
}

func renderCatalogList(w io.Writer, report catalogListReport) error {
	writer := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "CATALOG\tTRUST\tWORKFLOW\tSNAPSHOT")
	for _, entry := range report.Catalogs {
		workflow, snapshot := entry.Workflow, "none"
		if workflow == "" {
			workflow = "-"
		}
		if entry.SnapshotID != nil {
			snapshot = *entry.SnapshotID
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", entry.Repository, entry.Trust, workflow, snapshot)
	}
	return writer.Flush()
}

func renderCatalogRefresh(w io.Writer, entry catalogRefreshEntry) error {
	if entry.Error == "" {
		_, err := fmt.Fprintf(w, "packy catalog refresh: selected Catalog Snapshot %s of %s\n", *entry.SnapshotID, entry.Repository)
		return err
	}
	if entry.SnapshotID == nil {
		_, err := fmt.Fprintf(w, "packy catalog refresh: %s failed: %s\n", entry.Repository, entry.Error)
		return err
	}
	_, err := fmt.Fprintf(w, "packy catalog refresh: %s failed and keeps Catalog Snapshot %s: %s\n", entry.Repository, *entry.SnapshotID, entry.Error)
	return err
}

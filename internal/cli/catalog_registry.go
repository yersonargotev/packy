package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
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
			if err := addCatalog(cmd, opts, resolver, args[0], trust, workflow); err != nil {
				return fmt.Errorf("packy catalog add: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&trust, "trust", "attested", "trust policy of the catalog: attested, or access-controlled for a private repository without artifact attestations")
	command.Flags().StringVar(&workflow, "workflow", "", "publication workflow that must sign attested Catalog Snapshots (default "+catalogstore.DefaultWorkflow+")")
	return command
}

func addCatalog(cmd *cobra.Command, opts Options, resolver *workstation.Resolver, name, trust, workflow string) error {
	repository, err := catalogstore.ParseRepository(name)
	if err != nil {
		return err
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
	var rejected *catalogstore.AttestationRejectedError
	if errors.As(err, &rejected) && rejected.Repository == repository.String() {
		_, publication := catalogstore.TrustPolicyName(policy)
		return fmt.Errorf("no artifact attestation of %s verified for publication workflow %s\nif %s publishes from another workflow, run `packy catalog add %s --workflow <path>`\ncause: %w", repository, publication, repository, repository, rejected.Err)
	}
	if err != nil {
		return err
	}
	var evidence string
	switch policy := policy.(type) {
	case catalogstore.Attested:
		evidence = "attested by " + policy.Workflow
	case catalogstore.AccessControlled:
		evidence = accessControlledName + ", " + accessControlledNote
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy catalog add: registered %s (%s) and selected Catalog Snapshot %s\n", repository, evidence, snapshot.ID)
	return err
}

func newCatalogRemoveCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	return &cobra.Command{
		Use: "remove <owner/repo>", Short: "Unregister a catalog that no receipt, personal project activation, or lock of the current project names; retained Catalog Snapshots stay", Args: cobra.ExactArgs(1),
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
				locked, err := currentProjectLockFromCatalog(station, repository.String())
				if err != nil {
					return fmt.Errorf("check the current project lock before removing catalog %s: %w", repository, err)
				}
				if len(installed) == 0 && len(activated) == 0 && len(locked) == 0 {
					return nil
				}
				names := make([]string, 0, len(installed)+len(activated)+len(locked))
				for _, intent := range installed {
					names = append(names, fmt.Sprintf("%s on %s", intent.PackID, intent.Surface))
				}
				for _, intent := range activated {
					names = append(names, fmt.Sprintf("%s on %s in a project", intent.PackID, intent.Surface))
				}
				for _, intent := range locked {
					names = append(names, fmt.Sprintf("%s on %s in this project's lock", intent.PackID, intent.Surface))
				}
				packs, them := "Packs", "them"
				if len(names) == 1 {
					packs, them = "Pack", "it"
				}
				return fmt.Errorf("catalog %s is in use by installed %s %s; deactivate or uninstall %s, then remove the catalog", repository, packs, strings.Join(names, ", "), them)
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
				id, err := store.SelectedID(registration.Repository)
				if err != nil {
					return err
				}
				if id != "" {
					entry.SnapshotID = &id
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
			registrations, err := store.Registrations()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				repository, err := catalogstore.ParseRepository(args[0])
				if err != nil {
					return err
				}
				index, err := registeredIndex(registrations, repository)
				if err != nil {
					if jsonOutput {
						report := catalogRefreshReport{SchemaVersion: 1, Report: "catalog-refresh", Catalogs: []catalogRefreshEntry{{Repository: repository.String(), Result: "failed", Error: err.Error()}}}
						if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); encodeErr != nil {
							return encodeErr
						}
					}
					return err
				}
				registrations = registrations[index : index+1]
			}
			report := catalogRefreshReport{SchemaVersion: 1, Report: "catalog-refresh", Catalogs: []catalogRefreshEntry{}}
			failed := 0
			err = store.RefreshCatalogs(cmd.Context(), registrations, func(catalogstore.Repository) {}, func(refresh catalogstore.Refresh) error {
				entry := catalogRefreshEntry{Repository: refresh.Repository.String(), Result: "selected"}
				if refresh.SnapshotID != "" {
					entry.SnapshotID = &refresh.SnapshotID
				}
				if refresh.Err != nil {
					failed++
					entry.Result, entry.Error = "failed", refresh.Err.Error()
				}
				report.Catalogs = append(report.Catalogs, entry)
				if jsonOutput {
					return nil
				}
				return renderCatalogRefresh(cmd.OutOrStdout(), entry)
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d catalogs failed to refresh and keep their previous Catalog Snapshot", failed, len(registrations))
			}
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit versioned catalog-refresh JSON")
	return command
}

func newCatalogRestoreCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	return &cobra.Command{
		Use: "restore <owner/repo> <snapshot>", Short: "Download one exact Catalog Snapshot again and restore its retained files if they changed; the selection stays", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := catalogstore.ParseRepository(args[0])
			if err != nil {
				return err
			}
			store, _, err := registryStore(opts, resolver)
			if err != nil {
				return err
			}
			registrations, err := store.Registrations()
			if err != nil {
				return err
			}
			if _, err := registeredIndex(registrations, repository); err != nil {
				return err
			}
			snapshot, restored, err := store.Restore(cmd.Context(), repository, args[1])
			if err != nil {
				return err
			}
			if !restored {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy catalog restore: Catalog Snapshot %s of %s is intact\n", snapshot.ID, repository)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy catalog restore: restored Catalog Snapshot %s of %s\n", snapshot.ID, repository)
			return err
		},
	}
}

func registeredIndex(registrations []catalogstore.Registration, repository catalogstore.Repository) (int, error) {
	index := slices.IndexFunc(registrations, func(registration catalogstore.Registration) bool { return registration.Repository == repository })
	if index < 0 {
		return -1, fmt.Errorf("catalog %s is not registered; run `packy catalog list` to see registered catalogs, or `packy catalog add %s` to register it", repository, repository)
	}
	return index, nil
}

func accessControlledCatalogs(opts Options, resolver *workstation.Resolver) (map[string]bool, error) {
	store, _, err := registryStore(opts, resolver)
	if err != nil {
		return nil, err
	}
	registrations, err := store.Registrations()
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, registration := range registrations {
		if _, ok := registration.Trust.(catalogstore.AccessControlled); ok {
			result[registration.Repository.String()] = true
		}
	}
	return result, nil
}

func currentProjectLockFromCatalog(station workstation.Snapshot, repository string) ([]capabilitypack.ActivationIntent, error) {
	cwd, err := station.CurrentDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve current directory: %w", err)
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(cwd)
	if errors.As(err, new(capabilitypack.ProjectNotFoundError)) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return capabilitypack.ProjectLockFromCatalog(projectRoot, repository)
}

func registryStore(opts Options, resolver *workstation.Resolver) (*catalogstore.Store, workstation.Snapshot, error) {
	station, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return nil, workstation.Snapshot{}, err
	}
	return newCatalogStore(opts, station), station, nil
}

const accessControlledNote = "trusts GitHub access to a private repository in place of build provenance, a weaker trust level"

var accessControlledName, _ = catalogstore.TrustPolicyName(catalogstore.AccessControlled{})

func renderCatalogList(w io.Writer, report catalogListReport) error {
	writer := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "CATALOG\tTRUST\tWORKFLOW\tSNAPSHOT")
	weaker := false
	for _, entry := range report.Catalogs {
		weaker = weaker || entry.Trust == accessControlledName
		workflow, snapshot := entry.Workflow, "none"
		if workflow == "" {
			workflow = "-"
		}
		if entry.SnapshotID != nil {
			snapshot = *entry.SnapshotID
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", entry.Repository, entry.Trust, workflow, snapshot)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return renderAccessControlledNote(w, weaker)
}

func renderAccessControlledNote(w io.Writer, shown bool) error {
	if !shown {
		return nil
	}
	_, err := fmt.Fprintf(w, "\n%s %s.\n", accessControlledName, accessControlledNote)
	return err
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

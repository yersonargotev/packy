package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/capabilitypack"
)

func newPackCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "pack", Short: "Discover and manage capability packs"}
	cmd.AddCommand(newPackListCommand(opts), newPackShowCommand(opts))
	return cmd
}

func newPackListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List available capability packs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "PACK\tVERSION\tDESCRIPTION\tAVAILABLE ON")
			for _, pack := range catalog.List() {
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

func newPackShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "show <pack>", Short: "Show a capability pack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			pack, err := catalog.Show(args[0])
			if err != nil {
				return err
			}
			counts := pack.ResourceCounts()
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDescription: %s\nSupported CLI surfaces: %s\nProvides capabilities: %s\nRequires capabilities: %s\nRequires global tools: %s\nConflicts with capabilities: %s\nResources: %d skill, %d instruction, %d mcp_server, %d lifecycle\n",
				pack.ID, pack.Version, pack.Description, joinSurfaces(pack.Surfaces), joinOrNone(pack.Provides), joinOrNone(pack.Requires.Capabilities), joinOrNone(pack.Requires.Tools), joinOrNone(pack.Conflicts), counts.Skills, counts.Instructions, counts.MCPServers, counts.Lifecycles)
			return nil
		},
	}
}

func discoverPackCatalog(opts Options) (capabilitypack.Catalog, error) {
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		return capabilitypack.Catalog{}, err
	}
	return capabilitypack.Discover(paths.BundleSourceRoot)
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

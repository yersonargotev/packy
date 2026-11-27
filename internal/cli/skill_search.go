package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/workstation"
)

func newSkillSearchCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "search <query>...",
		Short: "Search current catalog skills by text",
		Long:  "Search the skills of every current Pack for a surface. A skill matches when every query term is a case-insensitive substring of its qualified skill, native name, or effective description. Results are sorted by qualified skill, carry no score, and include incompatible candidates with their reason. Uses local retained data only. Inspect a candidate with `packy show <pack> --surface <surface>` and obtain it with `packy get skill`.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			search, err := capabilitypack.NewSkillSearch(args, capabilitypack.Surface(surface))
			if err != nil {
				return err
			}
			catalog, err := discoverPackCatalog(cmd.Context(), opts, workstationResolver)
			if err != nil {
				return err
			}
			report, err := catalog.SearchSkills(cmd.Context(), search)
			if err != nil {
				return err
			}
			return renderSkillSearch(cmd.OutOrStdout(), report, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "Search the effective skills of agents or claude")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func renderSkillSearch(w io.Writer, report capabilitypack.SkillSearchReport, structured bool) error {
	if structured {
		return json.NewEncoder(w).Encode(struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			capabilitypack.SkillSearchReport
		}{1, "skill-search", report})
	}
	writer := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SKILL\tCATALOG\tCOMPATIBLE\tDESCRIPTION")
	for _, skill := range report.Skills {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", skill.QualifiedSkill, packCatalog(capabilitypack.CatalogRef{Repository: skill.Catalog}), yesNo(skill.Compatible), skill.Description)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, report.Limitation)
	return err
}

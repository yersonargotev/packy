package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

func newVersionCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "version", Short: "Print the Packy version and structured build identity", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					SchemaVersion int    `json:"schema_version"`
					Report        string `json:"report"`
					Version       string `json:"version"`
					SourceCommit  string `json:"source_commit"`
				}{1, "packy-version", packyversion.Value, packyversion.SourceCommit})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy version %s\n", packyversion.Value)
			return err
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON with the source commit")
	return command
}

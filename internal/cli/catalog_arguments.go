package cli

import (
	"errors"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// configureCatalogArguments preserves the requested output format even when
// strict argument parsing stops before reaching the final --json flag.
func configureCatalogArguments(command *cobra.Command, jsonOutput *bool, failure func(*cobra.Command, string, error) error) {
	validate := command.Args
	if validate == nil {
		validate = cobra.NoArgs
	}
	command.DisableFlagParsing = true
	command.Args = func(cmd *cobra.Command, args []string) error {
		// Discover output mode across unknown flags before strict parsing can stop.
		// Both passes use pflag's actual flag/value rules, including the -- boundary.
		flags := cmd.Flags()
		flags.ParseErrorsAllowlist.UnknownFlags = true
		requestedJSON := false

		discoveryArgs := slices.Clone(args)
		for {
			err := flags.ParseAll(discoveryArgs, func(flag *pflag.Flag, value string) error {
				if flag.Name == "json" {
					if requested, err := strconv.ParseBool(value); err == nil {
						requestedJSON = requested
					}
				}
				return nil
			})
			var syntaxError *pflag.InvalidSyntaxError
			if !errors.As(err, &syntaxError) {
				break
			}
			// Replace only malformed tokens in discovery; strict parsing retains the
			// original arguments and error. Empty positional values preserve arity.
			for i, argument := range discoveryArgs {
				if argument == syntaxError.GetSpecifiedFlag() {
					discoveryArgs[i] = ""
				}
			}
		}
		flags.ParseErrorsAllowlist.UnknownFlags = false
		if err := flags.Parse(args); err != nil {
			*jsonOutput = requestedJSON
			return failure(cmd, "invalid_arguments", err)
		}
		if help, _ := flags.GetBool("help"); help {
			return nil
		}
		if err := validate(cmd, flags.Args()); err != nil {
			return failure(cmd, "invalid_arguments", err)
		}
		return nil
	}
	run := command.RunE
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if help, _ := cmd.Flags().GetBool("help"); help {
			return cmd.Help()
		}
		return run(cmd, cmd.Flags().Args())
	}
}

// Command packcontentvalidate validates Packy's inert bundle content.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/packsync"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("packcontentvalidate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var repositoryRoot string
	flags.StringVar(&repositoryRoot, "repository-root", ".", "Packy repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return err
	}
	return packsync.ValidateContent(filepath.Join(root, "bundle"))
}

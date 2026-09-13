// Command catalogvalidate validates an inert Catalog Project and, when given a
// baseline, enforces independent Pack version changes.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/tools/catalogorigin"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("catalogvalidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", ".", "Catalog Project root")
	baseline := flags.String("baseline", "", "baseline Catalog Project root for Pack version validation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "catalogvalidate accepts flags only")
		return 2
	}

	resolver, err := catalogorigin.New()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer resolver.Close()

	validation, err := managedpack.ValidateCatalogProject(
		context.Background(), *project, *baseline,
		resolver,
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "validated Catalog Project packs=%d fitness_rows=%d\n", len(validation.Packs), validation.FitnessRows)
	for _, pack := range validation.Packs {
		fmt.Fprintf(stdout, "%s@%s manifest_sha256=%s closure_sha256=%s files=%d\n",
			pack.Manifest.ID, pack.Manifest.Version, pack.ManifestSHA256, pack.ClosureSHA256, len(pack.Files))
	}
	return 0
}

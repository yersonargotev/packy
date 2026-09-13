// Command catalogsnapshot builds a deterministic immutable Catalog Snapshot
// from one validated Catalog Project commit.
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
	os.Exit(execute())
}

func execute() int {
	resolver, err := catalogorigin.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resolver.Close()
	return run(os.Args[1:], os.Stdout, os.Stderr, resolver)
}

func run(args []string, stdout, stderr io.Writer, resolver managedpack.OriginResolver) int {
	flags := flag.NewFlagSet("catalogsnapshot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", ".", "Catalog Project root")
	sourceRepository := flags.String("source-repository", "", "reviewed Catalog Project owner/repository")
	sourceCommit := flags.String("source-commit", "", "reviewed Catalog Project full commit")
	builder := flags.String("builder", "", "exact builder identity owner/repository@commit")
	outDir := flags.String("out-dir", "dist", "new output directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *sourceRepository == "" || *sourceCommit == "" || *builder == "" {
		fmt.Fprintln(stderr, "catalogsnapshot requires --source-repository, --source-commit, and --builder")
		return 2
	}

	validation, err := managedpack.ValidateCatalogProject(context.Background(), *project, "", resolver)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	result, err := managedpack.BuildCatalogSnapshot(context.Background(), *project, validation, managedpack.CatalogSnapshotSource{
		Repository: *sourceRepository,
		Commit:     *sourceCommit,
		Builder:    *builder,
	}, *outDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "built Catalog Snapshot source=%s@%s catalog_sha256=%s archive_sha256=%s packs=%d\n",
		result.Index.Source.Repository, result.Index.Source.Commit, result.Index.CatalogSHA256, result.ArchiveSHA256, len(result.Index.Packs))
	for _, pack := range result.Index.Packs {
		fmt.Fprintf(stdout, "%s@%s manifest_sha256=%s closure_sha256=%s files=%d\n",
			pack.ID, pack.Version, pack.ManifestSHA256, pack.ClosureSHA256, len(pack.Files))
	}
	return 0
}

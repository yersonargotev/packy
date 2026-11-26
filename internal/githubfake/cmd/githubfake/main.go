package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/yersonargotev/packy/internal/cli"
	"github.com/yersonargotev/packy/internal/githubfake"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

const fixtureBuilder = "2222222222222222222222222222222222222222"

// seeds are the fixture catalogs of a new fixture directory. They are the
// official catalog, a second public attested catalog, and a catalog whose
// snapshot reuses an official Pack ID.
var seeds = []struct {
	repository, commit string
	pack               githubfake.Pack
}{
	{"yersonargotev/packy-catalog", "1111111111111111111111111111111111111111", githubfake.Pack{ID: "fixture", Version: "1.0.0"}},
	{"acme/packs", "3333333333333333333333333333333333333333", githubfake.Pack{ID: "acme", Version: "1.0.0"}},
	{"evil/packs", "4444444444444444444444444444444444444444", githubfake.Pack{ID: "fixture", Version: "9.0.0"}},
}

func main() {
	fixtures := flag.String("fixtures", "", "fixture directory; an absent directory is created with the official, acme/packs, and evil/packs fixture catalogs")
	addressFile := flag.String("addr-file", "", "file that receives the server address for PACKY_GITHUBFAKE_ADDR_FILE")
	flag.Parse()
	if *fixtures == "" || *addressFile == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *fixtures, *addressFile); err != nil {
		fmt.Fprintln(os.Stderr, "githubfake:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, fixtures, addressFile string) error {
	if _, err := os.Stat(fixtures); errors.Is(err, fs.ErrNotExist) {
		if err := seed(ctx, fixtures); err != nil {
			return fmt.Errorf("seed fixture catalogs: %w", err)
		}
	} else if err != nil {
		return err
	}
	server := httptest.NewTLSServer(githubfake.Handler(fixtures))
	defer server.Close()
	if err := githubfake.AddressOf(server).Write(addressFile); err != nil {
		return err
	}
	fmt.Printf("serving %s at %s\n", fixtures, server.URL)
	<-ctx.Done()
	return nil
}

func seed(ctx context.Context, fixtures string) error {
	work, err := os.MkdirTemp("", "githubfake-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	// Consumers accept only a snapshot built by a Packy commit, and go run stamps none.
	if packyversion.SourceCommit == "unknown" {
		packyversion.SourceCommit = fixtureBuilder
	}
	for _, catalog := range seeds {
		project, dist := filepath.Join(work, catalog.repository, "project"), filepath.Join(work, catalog.repository, "dist")
		if err := githubfake.WriteCatalogProject(project, catalog.pack); err != nil {
			return err
		}
		build := cli.NewRootCommand(cli.Options{})
		build.SetArgs([]string{"catalog", "build", "--project", project, "--source-repository", catalog.repository, "--source-commit", catalog.commit, "--out-dir", dist})
		if err := build.ExecuteContext(ctx); err != nil {
			return err
		}
		if err := githubfake.Publish(fixtures, catalog.repository, catalog.commit, dist); err != nil {
			return err
		}
	}
	return nil
}

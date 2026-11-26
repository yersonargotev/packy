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

const (
	officialRepository = "yersonargotev/packy-catalog"
	officialCommit     = "1111111111111111111111111111111111111111"
	fixtureBuilder     = "2222222222222222222222222222222222222222"
)

func main() {
	fixtures := flag.String("fixtures", "", "fixture directory; an absent directory is created with the official fixture catalog")
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
		if err := seedOfficial(ctx, fixtures); err != nil {
			return fmt.Errorf("seed official fixture catalog: %w", err)
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

func seedOfficial(ctx context.Context, fixtures string) error {
	work, err := os.MkdirTemp("", "githubfake-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	project, dist := filepath.Join(work, "project"), filepath.Join(work, "dist")
	if err := githubfake.WriteCatalogProject(project, githubfake.Pack{ID: "fixture", Version: "1.0.0"}); err != nil {
		return err
	}
	// Consumers accept only a snapshot built by a Packy commit, and go run stamps none.
	if packyversion.SourceCommit == "unknown" {
		packyversion.SourceCommit = fixtureBuilder
	}
	build := cli.NewRootCommand(cli.Options{})
	build.SetArgs([]string{"catalog", "build", "--project", project, "--source-repository", officialRepository, "--source-commit", officialCommit, "--out-dir", dist})
	if err := build.ExecuteContext(ctx); err != nil {
		return err
	}
	return githubfake.Publish(fixtures, officialRepository, officialCommit, dist)
}

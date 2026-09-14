package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/cli"
)

const officialCatalog = "yersonargotev/packy-catalog"

type releaseFileSource struct{}

func (releaseFileSource) Latest(_ context.Context, repository string) (catalogstore.Release, error) {
	if repository != officialCatalog {
		return catalogstore.Release{}, fmt.Errorf("unsupported catalog repository %q", repository)
	}
	path := os.Getenv("PACKY_CATALOG_RELEASE")
	if path == "" {
		return catalogstore.Release{}, errors.New("PACKY_CATALOG_RELEASE is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return catalogstore.Release{}, err
	}
	var release catalogstore.Release
	if err := json.Unmarshal(data, &release); err != nil {
		return catalogstore.Release{}, err
	}
	return release, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	command := cli.NewRootCommand(cli.Options{CatalogSource: releaseFileSource{}})
	err := command.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

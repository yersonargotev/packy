package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"

	"github.com/yersonargotev/packy/internal/cli"
	"github.com/yersonargotev/packy/internal/githubfake"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

const (
	fixtureBuilder = "2222222222222222222222222222222222222222"
	privateToken   = "packy-fixture-token"
)

// seeds are the fixture catalogs of a new fixture directory. They are the
// official catalog, a second public attested catalog, a catalog whose snapshot
// reuses an official Pack ID, and two private catalogs that only privateToken
// reads, one for each trust policy.
var seeds = []struct {
	repository, commit, token string
	pack                      githubfake.Pack
}{
	{"yersonargotev/packy-catalog", "1111111111111111111111111111111111111111", "", githubfake.Pack{ID: "fixture", Version: "1.0.0"}},
	{"acme/packs", "3333333333333333333333333333333333333333", "", githubfake.Pack{ID: "acme", Version: "1.0.0"}},
	{"evil/packs", "4444444444444444444444444444444444444444", "", githubfake.Pack{ID: "fixture", Version: "9.0.0"}},
	{"team/private-packs", "5555555555555555555555555555555555555555", privateToken, githubfake.Pack{ID: "team", Version: "1.0.0", Portable: true}},
	{"team/attested-packs", "6666666666666666666666666666666666666666", privateToken, githubfake.Pack{ID: "vault", Version: "1.0.0"}},
}

func main() {
	fixtures := flag.String("fixtures", "", "fixture directory; an absent directory is created with the official, acme/packs, evil/packs, team/private-packs, and team/attested-packs fixture catalogs, and GH_TOKEN="+privateToken+" reads the private ones")
	addressFile := flag.String("addr-file", "", "file that receives the server address for PACKY_GITHUBFAKE_ADDR_FILE")
	requestLog := flag.String("request-log", "", "file that receives one JSON line per request with its host, path, and header names")
	flag.Parse()
	if *fixtures == "" || *addressFile == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *fixtures, *addressFile, *requestLog); err != nil {
		fmt.Fprintln(os.Stderr, "githubfake:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, fixtures, addressFile, requestLog string) error {
	if _, err := os.Stat(fixtures); errors.Is(err, fs.ErrNotExist) {
		if err := seed(ctx, fixtures); err != nil {
			return fmt.Errorf("seed fixture catalogs: %w", err)
		}
	} else if err != nil {
		return err
	}
	handler := githubfake.Handler(fixtures)
	if requestLog != "" {
		log, err := os.OpenFile(requestLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer log.Close()
		handler = logRequests(handler, log)
	}
	server := httptest.NewTLSServer(handler)
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
		if catalog.token != "" {
			if err := githubfake.WriteRepository(fixtures, catalog.repository, githubfake.Repository{Token: catalog.token}); err != nil {
				return err
			}
		}
	}
	return nil
}

// logRequests records header names and never header values, so the log shows
// which requests carried Authorization without the token.
func logRequests(next http.Handler, log io.Writer) http.Handler {
	var mu sync.Mutex
	encoder := json.NewEncoder(log)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, 0, len(r.Header))
		for name := range r.Header {
			names = append(names, name)
		}
		sort.Strings(names)
		mu.Lock()
		_ = encoder.Encode(map[string]any{"method": r.Method, "host": r.Host, "path": r.URL.Path, "headers": names})
		mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

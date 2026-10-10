package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func testCatalogPublicationContract(t *testing.T, executable string) {
	commit := strings.Repeat("a", 40)
	fakeGH, releaseRoot := catalogPublicationFixture(t)
	dist := t.TempDir()
	archive := []byte("deterministic snapshot bytes\n")
	checksum := []byte("b6095fc720133eb0a6381f02ee0afc9b33812946b391f3086b2e3f8a49f09e2c  catalog-snapshot.tar.gz\n")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dist, "catalog-snapshot.tar.gz"), archive)
	write(filepath.Join(dist, "SHA256SUMS"), checksum)
	// Only transport and shell dependencies are available; Packy runs outside its source tree.
	environment := func(additions ...string) []string {
		return testprocess.Env(t, append([]string{"PATH=" + filepath.Dir(fakeGH) + string(os.PathListSeparator) + os.Getenv("PATH"), "FAKE_RELEASE_ROOT=" + releaseRoot, "FAKE_CATALOG_COMMIT=" + commit}, additions...)...)
	}
	args := []string{"catalog", "publish", "--repository", "example/catalog", "--commit", commit, "--dist", dist, "--json"}
	run := func(env []string, expected, code string, extra ...string) {
		t.Helper()
		before := catalogTreeState(t, []string{dist})
		stdout, stderr, err := runCatalogCLI(t, env, t.TempDir(), executable, append(append([]string(nil), args...), extra...)...)
		validateCatalogCLISchema(t, "catalog-publication", stdout)
		var result struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			Result        string `json:"result"`
			Repository    string `json:"repository"`
			Commit        string `json:"commit"`
			Tag           string `json:"tag"`
			ArchiveSHA256 string `json:"archive_sha256"`
			Error         *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if decodeErr := json.Unmarshal(stdout, &result); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if result.SchemaVersion != 1 || result.Report != "catalog-publication" || result.Result != expected {
			t.Fatalf("unexpected report: %s %s %v", stdout, stderr, err)
		}
		if code != "" {
			if err == nil || result.Error == nil || result.Error.Code != code || result.Error.Message == "" {
				t.Fatalf("expected failure %s: %s %s %v", code, stdout, stderr, err)
			}
		} else if err != nil || result.Repository != "example/catalog" || result.Commit != commit || result.Tag != "catalog-"+commit || result.ArchiveSHA256 != "b6095fc720133eb0a6381f02ee0afc9b33812946b391f3086b2e3f8a49f09e2c" {
			t.Fatalf("publication identity: %s %s %v", stdout, stderr, err)
		}
		if after := catalogTreeState(t, []string{dist}); !reflect.DeepEqual(before, after) {
			t.Fatalf("publication changed prepared inputs: %v", after)
		}
	}
	run(environment(), "published", "")
	run(environment(), "unchanged", "")
	release := filepath.Join(releaseRoot, "release")
	t.Run("target conflict", func(t *testing.T) {
		write(filepath.Join(release, "target"), []byte(strings.Repeat("b", 40)+"\n"))
		run(environment(), "failed", "publication_conflict")
		write(filepath.Join(release, "target"), []byte(commit+"\n"))
	})
	t.Run("unexpected asset", func(t *testing.T) {
		write(filepath.Join(release, "unexpected"), []byte("extra"))
		before := catalogTreeState(t, []string{release})
		run(environment(), "failed", "publication_conflict")
		if after := catalogTreeState(t, []string{release}); !reflect.DeepEqual(before, after) {
			t.Fatal("conflict mutated published release")
		}
		if err := os.Remove(filepath.Join(release, "unexpected")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("immutable missing asset", func(t *testing.T) {
		if err := os.Remove(filepath.Join(release, "SHA256SUMS")); err != nil {
			t.Fatal(err)
		}
		run(environment(), "failed", "publication_conflict")
		write(filepath.Join(release, "SHA256SUMS"), checksum)
	})
	t.Run("transport download failure", func(t *testing.T) { run(environment("FAKE_DOWNLOAD_FAILURE=true"), "failed", "transport_failed") })
	t.Run("invalid assets before transport", func(t *testing.T) {
		write(filepath.Join(dist, "catalog-snapshot.tar.gz"), []byte("changed"))
		run(environment("FAKE_API_FAILURE=true"), "failed", "assets_invalid")
		write(filepath.Join(dist, "catalog-snapshot.tar.gz"), archive)
		write(filepath.Join(dist, "SHA256SUMS"), append(append([]byte(nil), checksum...), checksum...))
		run(environment(), "failed", "assets_invalid")
		write(filepath.Join(dist, "SHA256SUMS"), checksum)
	})
	t.Run("argument errors", func(t *testing.T) {
		for _, extra := range [][]string{{"--commit", "bad"}, {"--unknown", "--json"}, {"--repository", "bad"}, {"--help=bad", "--json"}, {"unexpected"}} {
			run(environment(), "failed", "invalid_arguments", extra...)
		}
	})
	t.Run("interrupted upload then recovery", func(t *testing.T) {
		if err := os.RemoveAll(release); err != nil {
			t.Fatal(err)
		}
		run(environment("FAKE_UPLOAD_FAILURE=catalog-snapshot.tar.gz"), "failed", "transport_failed")
		if string(readIssue798File(t, filepath.Join(release, "draft"))) != "true\n" {
			t.Fatal("failed upload published draft")
		}
		if string(readIssue798File(t, filepath.Join(release, "SHA256SUMS"))) != string(checksum) {
			t.Fatal("first uploaded asset missing")
		}
		run(environment(), "resumed", "")
		run(environment(), "unchanged", "")
	})
	t.Run("native immutability required", func(t *testing.T) {
		if err := os.RemoveAll(release); err != nil {
			t.Fatal(err)
		}
		run(environment("FAKE_PUBLISHED_IMMUTABLE=false"), "failed", "publication_conflict")
	})
}

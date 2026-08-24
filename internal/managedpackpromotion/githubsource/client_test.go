package githubsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCommitSHA   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testTagSHA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRootTreeSHA = "3e789fa096677c9452ac15ac9cd9140dab504db3"
	testBinTreeSHA  = "d4403ccc5d91efb43e2cb62e3c549b8d66fbc114"
	testManifestSHA = "483c739e8692a3c310366499996d0e2cc362fa59"
	testScriptSHA   = "039e4d0069c5c26909f86c505b9de66182e6d1f3"
	testSymlinkSHA  = "31f9e593bd4dd6c82bd12859a91d5ca952099db2"
)

func TestClientAcquiresImmutableReleaseAndVerifiedTreeWithoutCredentials(t *testing.T) {
	requested := map[string]int{}
	server := newTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		requested[request.URL.Path]++
		switch request.URL.Path {
		case "/repos/example/managed/releases":
			writeTestJSON(writer, []map[string]any{{
				"id": 202, "tag_name": "pack-v1.2.3", "immutable": true,
				"published_at": "2026-01-02T03:04:05Z", "draft": false, "prerelease": false,
			}})
		case "/repos/example/managed":
			writeTestJSON(writer, map[string]any{
				"id": 101, "full_name": "example/managed", "visibility": "public", "private": false,
			})
		case "/repos/example/managed/git/ref/tags/pack-v1.2.3":
			writeTestJSON(writer, map[string]any{"object": map[string]any{"sha": testTagSHA, "type": "tag"}})
		case "/repos/example/managed/git/tags/" + testTagSHA:
			writeTestJSON(writer, map[string]any{
				"sha": testTagSHA, "object": map[string]any{"sha": testCommitSHA, "type": "commit"},
			})
		case "/repos/example/managed/git/commits/" + testCommitSHA:
			writeTestJSON(writer, map[string]any{
				"sha": testCommitSHA, "tree": map[string]any{"sha": testRootTreeSHA},
			})
		case "/repos/example/managed/git/trees/" + testRootTreeSHA:
			if request.URL.Query().Get("recursive") != "1" {
				t.Fatalf("recursive query = %q", request.URL.RawQuery)
			}
			writeTestJSON(writer, validTreeResponse())
		case "/example/managed/" + testCommitSHA + "/bin/script.sh":
			assertRawRequest(t, request)
			_, _ = writer.Write([]byte("#!/bin/sh\nexit 0\n"))
		case "/example/managed/" + testCommitSHA + "/link":
			assertRawRequest(t, request)
			_, _ = writer.Write([]byte("bin/script.sh"))
		case "/example/managed/" + testCommitSHA + "/pack.json":
			assertRawRequest(t, request)
			_, _ = writer.Write([]byte("{\"id\":\"example\"}\n"))
		default:
			http.NotFound(writer, request)
		}
	})
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	releases, err := client.Releases(context.Background(), "example/managed")
	if err != nil || len(releases) != 1 {
		t.Fatalf("Releases() = %#v, %v", releases, err)
	}
	if releases[0].ID != 202 || releases[0].Tag != "pack-v1.2.3" || !releases[0].Immutable || releases[0].PublishedAt.IsZero() {
		t.Fatalf("release = %#v", releases[0])
	}
	candidate, err := client.ResolveRelease(context.Background(), "example/managed", releases[0])
	if err != nil {
		t.Fatalf("ResolveRelease() error = %v", err)
	}
	if candidate.RepositoryID != 101 || !candidate.Public || candidate.Commit != testCommitSHA || candidate.Tree != testRootTreeSHA || candidate.TagRefSHA != testTagSHA || len(candidate.TagObjects) != 1 {
		t.Fatalf("candidate = %#v", candidate)
	}

	temporary := t.TempDir()
	err = client.WithGitTreeSnapshot(context.Background(), candidate, temporary, func(root string) error {
		assertTestFile(t, filepath.Join(root, "pack.json"), "{\"id\":\"example\"}\n", 0o644)
		assertTestFile(t, filepath.Join(root, "bin", "script.sh"), "#!/bin/sh\nexit 0\n", 0o755)
		for _, inert := range []string{"link", "module"} {
			if _, err := os.Lstat(filepath.Join(root, inert)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("inert Git entry %q was materialized: %v", inert, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithGitTreeSnapshot() error = %v", err)
	}
	assertTestEmpty(t, temporary)
	if requested["/example/managed/"+testCommitSHA+"/module"] != 0 {
		t.Fatal("submodule content was requested")
	}
	for path := range requested {
		if strings.Contains(path, "/tarball/") || strings.Contains(path, "/git/blobs/") {
			t.Fatalf("unexpected GitHub acquisition request %q", path)
		}
	}
}

func TestClientPaginatesReleasesAndPreservesHTTPFailureMetadata(t *testing.T) {
	requestedSecondPage := false
	server := newTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/repos/o/r/releases" {
			switch request.URL.Query().Get("page") {
			case "1":
				items := make([]map[string]any, 100)
				for index := range items {
					items[index] = map[string]any{"id": index + 1, "tag_name": "preview", "published_at": "2026-01-01T00:00:00Z", "prerelease": true}
				}
				writeTestJSON(writer, items)
			case "2":
				requestedSecondPage = true
				writeTestJSON(writer, []map[string]any{{"id": 101, "tag_name": "pack-v1.0.0", "published_at": "2026-01-01T00:00:00Z"}})
			default:
				http.NotFound(writer, request)
			}
			return
		}
		writer.Header().Set("Retry-After", "9")
		writer.Header().Set("X-RateLimit-Remaining", "0")
		writer.Header().Set("X-RateLimit-Reset", "4102444800")
		writer.WriteHeader(http.StatusTooManyRequests)
	})
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	releases, err := client.Releases(context.Background(), "o/r")
	if err != nil || len(releases) != 101 || !requestedSecondPage || releases[100].Tag != "pack-v1.0.0" {
		t.Fatalf("Releases() = %d releases, second-page=%t, error=%v", len(releases), requestedSecondPage, err)
	}
	_, err = client.Releases(context.Background(), "missing/r")
	var responseError HTTPError
	if !errors.As(err, &responseError) || responseError.StatusCode != http.StatusTooManyRequests || responseError.RetryAfter != "9" || responseError.RateLimitRemaining != "0" || responseError.RateLimitReset != "4102444800" {
		t.Fatalf("HTTP error = %#v, %v", responseError, err)
	}
}

func TestClientRejectsInexactCommitAndUnboundedTagChain(t *testing.T) {
	client := New(nil)
	if _, err := client.ResolveCommit(context.Background(), "example/managed", "abc"); err == nil {
		t.Fatal("abbreviated SHA accepted")
	}

	requests := 0
	server := newTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/example/managed":
			writeTestJSON(writer, map[string]any{"id": 1, "full_name": "example/managed", "visibility": "public"})
		case request.URL.Path == "/repos/example/managed/git/ref/tags/pack-v1.2.3":
			writeTestJSON(writer, map[string]any{"object": map[string]any{"sha": fmt.Sprintf("%040x", 1), "type": "tag"}})
		case strings.Contains(request.URL.Path, "/git/tags/"):
			requests++
			current := strings.TrimPrefix(request.URL.Path, "/repos/example/managed/git/tags/")
			writeTestJSON(writer, map[string]any{
				"sha": current, "object": map[string]any{"sha": fmt.Sprintf("%040x", requests+1), "type": "tag"},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	defer server.Close()

	client = newClient(server.Client(), server.URL)
	_, err := client.ResolveRelease(context.Background(), "example/managed", Release{Tag: "pack-v1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "tag object limit") {
		t.Fatalf("ResolveRelease() error = %v", err)
	}
	if requests != maxAnnotatedTagObjects {
		t.Fatalf("tag object requests = %d, want %d", requests, maxAnnotatedTagObjects)
	}
}

func TestClientRejectsUnprovableTreeAndCleans(t *testing.T) {
	tests := []struct {
		name     string
		response func() map[string]any
		blob     string
		want     string
	}{
		{
			name: "mismatched root",
			response: func() map[string]any {
				response := validTreeResponse()
				response["sha"] = strings.Repeat("f", 40)
				return response
			},
			want: "different root tree",
		},
		{
			name: "truncated",
			response: func() map[string]any {
				response := validTreeResponse()
				response["truncated"] = true
				return response
			},
			want: "truncated",
		},
		{name: "wrong blob", response: validTreeResponse, blob: "#!/bin/sh\nexit 1\n", want: "blob SHA"},
		{
			name: "portable collision",
			response: func() map[string]any {
				response := validTreeResponse()
				response["tree"] = []map[string]any{
					{"path": "Straße", "mode": "100644", "type": "blob", "sha": testManifestSHA, "size": 17},
					{"path": "STRASSE", "mode": "100644", "type": "blob", "sha": testManifestSHA, "size": 17},
				}
				return response
			},
			want: "portable path collision",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/repos/example/managed/git/trees/" + testRootTreeSHA:
					writeTestJSON(writer, test.response())
				case "/example/managed/" + testCommitSHA + "/bin/script.sh":
					value := "#!/bin/sh\nexit 0\n"
					if test.blob != "" {
						value = test.blob
					}
					_, _ = writer.Write([]byte(value))
				case "/example/managed/" + testCommitSHA + "/link":
					_, _ = writer.Write([]byte("bin/script.sh"))
				case "/example/managed/" + testCommitSHA + "/pack.json":
					_, _ = writer.Write([]byte("{\"id\":\"example\"}\n"))
				default:
					http.NotFound(writer, request)
				}
			})
			defer server.Close()
			client := newClient(server.Client(), server.URL)
			temporary := t.TempDir()
			visited := false
			err := client.WithGitTreeSnapshot(context.Background(), testCandidate(), temporary, func(string) error {
				visited = true
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("WithGitTreeSnapshot() error = %v, want %q", err, test.want)
			}
			if visited {
				t.Fatal("visitor called for unproved Git tree")
			}
			assertTestEmpty(t, temporary)
		})
	}
}

func validTreeResponse() map[string]any {
	return map[string]any{
		"sha": testRootTreeSHA, "truncated": false,
		"tree": []map[string]any{
			{"path": "bin", "mode": "040000", "type": "tree", "sha": testBinTreeSHA},
			{"path": "bin/script.sh", "mode": "100755", "type": "blob", "sha": testScriptSHA, "size": 17},
			{"path": "link", "mode": "120000", "type": "blob", "sha": testSymlinkSHA, "size": 13},
			{"path": "module", "mode": "160000", "type": "commit", "sha": strings.Repeat("a", 40)},
			{"path": "pack.json", "mode": "100644", "type": "blob", "sha": testManifestSHA, "size": 17},
		},
	}
}

func testCandidate() Candidate {
	return Candidate{Repository: "example/managed", Commit: testCommitSHA, Tree: testRootTreeSHA}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public acquisition sent credentials")
		}
		handler(writer, request)
	}))
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func assertRawRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Header.Get("Accept") != "application/octet-stream" {
		t.Fatalf("Accept = %q", request.Header.Get("Accept"))
	}
	if request.Header.Get("X-GitHub-Api-Version") != "" {
		t.Fatalf("raw content request carried REST API headers: %#v", request.Header)
	}
}

func assertTestFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %04o, want %04o", path, info.Mode().Perm(), mode)
	}
}

func assertTestEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		data, _ := json.Marshal(entries)
		t.Fatalf("temporary root not empty: %s", data)
	}
}

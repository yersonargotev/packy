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

	"github.com/yersonargotev/packy/internal/packsync"
)

const (
	managedCommitSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	managedRootTree  = "3e789fa096677c9452ac15ac9cd9140dab504db3"
	managedBinTree   = "d4403ccc5d91efb43e2cb62e3c549b8d66fbc114"
	manifestBlobSHA  = "483c739e8692a3c310366499996d0e2cc362fa59"
	scriptBlobSHA    = "039e4d0069c5c26909f86c505b9de66182e6d1f3"
	symlinkBlobSHA   = "31f9e593bd4dd6c82bd12859a91d5ca952099db2"
)

func TestWithGitTreeSnapshotVerifiesAndMaterializesExactCommitTree(t *testing.T) {
	requested := map[string]int{}
	server := newGitTreeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		requested[request.URL.Path]++
		switch request.URL.Path {
		case "/repos/example/managed/git/trees/" + managedRootTree:
			if request.URL.Query().Get("recursive") != "1" {
				t.Fatalf("recursive query = %q", request.URL.RawQuery)
			}
			writeJSON(writer, validGitTreeResponse())
		case "/example/managed/" + managedCommitSHA + "/bin/script.sh":
			assertExactRawRequest(t, request)
			_, _ = writer.Write([]byte("#!/bin/sh\nexit 0\n"))
		case "/example/managed/" + managedCommitSHA + "/link":
			assertExactRawRequest(t, request)
			_, _ = writer.Write([]byte("bin/script.sh"))
		case "/example/managed/" + managedCommitSHA + "/pack.json":
			assertExactRawRequest(t, request)
			_, _ = writer.Write([]byte("{\"id\":\"example\"}\n"))
		default:
			http.NotFound(writer, request)
		}
	})
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	temporary := t.TempDir()
	err := client.WithGitTreeSnapshot(context.Background(), managedCandidate(), temporary, func(root string) error {
		assertSnapshotFile(t, filepath.Join(root, "pack.json"), "{\"id\":\"example\"}\n", 0o644)
		assertSnapshotFile(t, filepath.Join(root, "bin", "script.sh"), "#!/bin/sh\nexit 0\n", 0o755)
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
	assertEmpty(t, temporary)
	if requested["/example/managed/"+managedCommitSHA+"/module"] != 0 {
		t.Fatal("submodule content was requested")
	}
	for path := range requested {
		if strings.Contains(path, "/tarball/") {
			t.Fatalf("managed acquisition requested generated tarball %q", path)
		}
		if strings.Contains(path, "/git/blobs/") {
			t.Fatalf("managed acquisition spent one REST request per blob %q", path)
		}
	}
	visitorErr := errors.New("visitor failed")
	err = client.WithGitTreeSnapshot(context.Background(), managedCandidate(), temporary, func(string) error { return visitorErr })
	if !errors.Is(err, visitorErr) {
		t.Fatalf("visitor error = %v, want %v", err, visitorErr)
	}
	assertEmpty(t, temporary)
}

func TestWithGitTreeSnapshotRejectsUnprovableTreesAndCleans(t *testing.T) {
	tests := []struct {
		name       string
		response   func() map[string]any
		blobPath   string
		blob       string
		configure  func(*Client)
		cancelBlob bool
		want       string
	}{
		{
			name: "mismatched root tree",
			response: func() map[string]any {
				response := validGitTreeResponse()
				response["sha"] = strings.Repeat("f", 40)
				return response
			},
			want: "different root tree",
		},
		{
			name: "truncated recursive tree",
			response: func() map[string]any {
				response := validGitTreeResponse()
				response["truncated"] = true
				return response
			},
			want: "truncated",
		},
		{
			name:     "wrong blob",
			response: validGitTreeResponse,
			blobPath: "/example/managed/" + managedCommitSHA + "/bin/script.sh",
			blob:     "#!/bin/sh\nexit 1\n",
			want:     "blob SHA",
		},
		{
			name: "wrong nested tree",
			response: func() map[string]any {
				response := validGitTreeResponse()
				entries := response["tree"].([]map[string]any)
				entries[0]["sha"] = strings.Repeat("e", 40)
				return response
			},
			want: "tree SHA",
		},
		{
			name:      "entry limit",
			response:  validGitTreeResponse,
			configure: func(client *Client) { client.archiveLimits.maxEntries = 4 },
			want:      "entry count",
		},
		{
			name:      "file size limit",
			response:  validGitTreeResponse,
			configure: func(client *Client) { client.archiveLimits.maxFileBytes = 4 },
			want:      "file size",
		},
		{
			name:      "aggregate byte limit",
			response:  validGitTreeResponse,
			configure: func(client *Client) { client.archiveLimits.maxExpandedBytes = 25 },
			want:      "expanded size",
		},
		{
			name: "path depth limit",
			response: func() map[string]any {
				response := validGitTreeResponse()
				response["tree"] = []map[string]any{{
					"path": strings.Repeat("d/", maxArchivePathDepth) + "file",
					"mode": "100644", "type": "blob", "sha": manifestBlobSHA, "size": 17,
				}}
				return response
			},
			want: "path depth",
		},
		{
			name: "Unicode case-fold collision",
			response: func() map[string]any {
				response := validGitTreeResponse()
				response["tree"] = []map[string]any{
					{"path": "Straße", "mode": "100644", "type": "blob", "sha": manifestBlobSHA, "size": 17},
					{"path": "STRASSE", "mode": "100644", "type": "blob", "sha": manifestBlobSHA, "size": 17},
				}
				return response
			},
			want: "portable path collision",
		},
		{
			name:       "cancellation during blob acquisition",
			response:   validGitTreeResponse,
			cancelBlob: true,
			want:       context.Canceled.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sawTarball := false
			server := newGitTreeServer(t, func(writer http.ResponseWriter, request *http.Request) {
				if strings.Contains(request.URL.Path, "/tarball/") {
					sawTarball = true
					http.NotFound(writer, request)
					return
				}
				switch request.URL.Path {
				case "/repos/example/managed/git/trees/" + managedRootTree:
					writeJSON(writer, test.response())
				case "/example/managed/" + managedCommitSHA + "/bin/script.sh":
					if test.cancelBlob {
						cancel()
					}
					value := "#!/bin/sh\nexit 0\n"
					if test.blobPath == request.URL.Path {
						value = test.blob
					}
					_, _ = writer.Write([]byte(value))
				case "/example/managed/" + managedCommitSHA + "/link":
					_, _ = writer.Write([]byte("bin/script.sh"))
				case "/example/managed/" + managedCommitSHA + "/pack.json":
					_, _ = writer.Write([]byte("{\"id\":\"example\"}\n"))
				default:
					http.NotFound(writer, request)
				}
			})
			defer server.Close()

			client := newClient(server.Client(), server.URL)
			if test.configure != nil {
				test.configure(client)
			}
			temporary := t.TempDir()
			visited := false
			err := client.WithGitTreeSnapshot(ctx, managedCandidate(), temporary, func(string) error {
				visited = true
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("WithGitTreeSnapshot() error = %v, want %q", err, test.want)
			}
			if visited {
				t.Fatal("visitor called for unproved Git tree")
			}
			assertEmpty(t, temporary)
			if sawTarball {
				t.Fatal("managed acquisition fell back to a tarball")
			}
		})
	}
}

func TestResolveReleaseCapsAnnotatedTagTraversal(t *testing.T) {
	requests := 0
	server := newGitTreeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/example/managed":
			writeJSON(writer, map[string]any{"id": 1, "full_name": "example/managed", "visibility": "public", "private": false, "owner": map[string]any{"login": "example"}})
		case request.URL.Path == "/repos/example/managed/git/ref/tags/pack-v1.2.3":
			writeJSON(writer, map[string]any{"object": map[string]any{"sha": fmt.Sprintf("%040x", 1), "type": "tag"}})
		case strings.Contains(request.URL.Path, "/git/tags/"):
			requests++
			current := strings.TrimPrefix(request.URL.Path, "/repos/example/managed/git/tags/")
			next := fmt.Sprintf("%040x", requests+1)
			writeJSON(writer, map[string]any{"sha": current, "tag": "nested", "object": map[string]any{"sha": next, "type": "tag"}, "verification": map[string]any{"verified": false}})
		default:
			http.NotFound(writer, request)
		}
	})
	defer server.Close()

	client := newClient(server.Client(), server.URL)
	_, err := client.ResolveRelease(context.Background(), packsync.SourceConfig{Repository: "example/managed"}, packsync.Release{Tag: "pack-v1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "tag object limit") {
		t.Fatalf("ResolveRelease() error = %v", err)
	}
	if requests != maxAnnotatedTagObjects {
		t.Fatalf("tag object requests = %d, want %d", requests, maxAnnotatedTagObjects)
	}
}

func validGitTreeResponse() map[string]any {
	return map[string]any{
		"sha":       managedRootTree,
		"truncated": false,
		"tree": []map[string]any{
			{"path": "bin", "mode": "040000", "type": "tree", "sha": managedBinTree},
			{"path": "bin/script.sh", "mode": "100755", "type": "blob", "sha": scriptBlobSHA, "size": 17},
			{"path": "link", "mode": "120000", "type": "blob", "sha": symlinkBlobSHA, "size": 13},
			{"path": "module", "mode": "160000", "type": "commit", "sha": strings.Repeat("a", 40)},
			{"path": "pack.json", "mode": "100644", "type": "blob", "sha": manifestBlobSHA, "size": 17},
		},
	}
}

func managedCandidate() packsync.Candidate {
	return packsync.Candidate{Repository: "example/managed", Commit: managedCommitSHA, Tree: managedRootTree}
}

func newGitTreeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatal("public Git tree acquisition sent credentials")
		}
		handler(writer, request)
	}))
}

func assertExactRawRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Header.Get("Accept") != "application/octet-stream" {
		t.Fatalf("Accept = %q", request.Header.Get("Accept"))
	}
	if request.Header.Get("X-GitHub-Api-Version") != "" {
		t.Fatalf("raw content request carried REST API headers: %#v", request.Header)
	}
}

func assertSnapshotFile(t *testing.T, path, want string, mode os.FileMode) {
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

func assertEmpty(t *testing.T, root string) {
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

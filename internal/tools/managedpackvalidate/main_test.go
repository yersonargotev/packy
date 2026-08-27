package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestRunValidatesProjectWithLocalOrigin(t *testing.T) {
	project := t.TempDir()
	origin := t.TempDir()
	writeToolFile(t, filepath.Join(project, "notices", "mit"), "MIT notice\n")
	writeToolFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(origin, "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(project, "pack.json"), toolManifest)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", project, "--origin", "upstream=" + origin}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "validated example@1.0.0") || !strings.Contains(output, "files=3") || !strings.Contains(output, "fitness_rows=2") {
		t.Fatalf("stdout = %q", output)
	}
}

func TestRunRejectsRuntimeProjectionCollisions(t *testing.T) {
	project := t.TempDir()
	origin := t.TempDir()
	writeToolFile(t, filepath.Join(project, "notices", "mit"), "MIT notice\n")
	writeToolFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(project, "skills", "other", "SKILL.md"), "other\n")
	writeToolFile(t, filepath.Join(origin, "guide", "SKILL.md"), "guidance\n")
	writeToolFile(t, filepath.Join(project, "pack.json"), toolManifestWithProjectionCollision(t))

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--project", project, "--origin", "upstream=" + origin}, &stdout, &stderr)
	if exit != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "runtime fitness") || !strings.Contains(stderr.String(), "projection collision") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
}

func TestResolverClonesAndChecksOutExactCommit(t *testing.T) {
	originRoot, firstCommit, _ := writeOriginRepository(t)
	resolved, err := (resolver{
		local:     map[string]string{},
		temporary: t.TempDir(),
		repositoryURL: func(managedpack.Origin) string {
			return originRoot
		},
	}).Resolve(context.Background(), managedpack.Origin{
		ID: "upstream", Repository: "example/upstream", Commit: firstCommit.String(),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(resolved, "guide.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first\n" {
		t.Fatalf("checked-out content = %q, want first commit", content)
	}
}

func TestResolverRejectsUnknownCheckout(t *testing.T) {
	originRoot, _, _ := writeOriginRepository(t)
	_, err := (resolver{
		local:     map[string]string{},
		temporary: t.TempDir(),
		repositoryURL: func(managedpack.Origin) string {
			return originRoot
		},
	}).Resolve(context.Background(), managedpack.Origin{
		ID: "upstream", Repository: "example/upstream", Commit: strings.Repeat("f", 40),
	})
	if err == nil || !strings.Contains(err.Error(), "checkout origin commit") {
		t.Fatalf("Resolve() error = %v, want checkout rejection", err)
	}
}

func TestResolverRejectsMalformedRepositoryObject(t *testing.T) {
	originRoot, _, headCommit := writeOriginRepository(t)
	objectPath := filepath.Join(originRoot, ".git", "objects", headCommit.String()[:2], headCommit.String()[2:])
	if err := os.Chmod(objectPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte("malformed object\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	_, err := (resolver{
		local:     map[string]string{},
		temporary: t.TempDir(),
		repositoryURL: func(managedpack.Origin) string {
			return originRoot
		},
	}).Resolve(context.Background(), managedpack.Origin{
		ID: "upstream", Repository: "example/upstream", Commit: headCommit.String(),
	})
	if err == nil || !strings.Contains(err.Error(), "clone public origin") {
		t.Fatalf("Resolve() error = %v, want malformed-object clone rejection", err)
	}
}

func TestResolverFollowsRedirectWithoutCredentials(t *testing.T) {
	originRoot, _, headCommit := writeOriginRepository(t)
	projectRoot := t.TempDir()
	if _, err := git.PlainClone(filepath.Join(projectRoot, "upstream.git"), true, &git.CloneOptions{URL: originRoot}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "--exec-path")
	command.Env = testprocess.Env(t)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("resolve Git executable path: %v", err)
	}
	backend := filepath.Join(strings.TrimSpace(string(output)), "git-http-backend")

	var serverMu sync.Mutex
	sinkRequests := 0
	redirectRequests := 0
	requestedPaths := []string{}
	originAuthorizations := []string{}
	sinkAuthorizations := []string{}
	backendHandler := &cgi.Handler{
		Path: backend,
		Env: append(testprocess.Env(t),
			"GIT_HTTP_EXPORT_ALL=true",
			"GIT_PROJECT_ROOT="+projectRoot,
		),
	}
	sink := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverMu.Lock()
		sinkRequests++
		requestedPaths = append(requestedPaths, request.URL.RequestURI())
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			sinkAuthorizations = append(sinkAuthorizations, authorization)
		}
		serverMu.Unlock()
		backendHandler.ServeHTTP(writer, request)
	}))
	defer sink.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serverMu.Lock()
		redirectRequests++
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			originAuthorizations = append(originAuthorizations, authorization)
		}
		serverMu.Unlock()
		http.Redirect(writer, request, sink.URL+request.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	authenticatedOrigin, err := url.Parse(redirect.URL)
	if err != nil {
		t.Fatal(err)
	}
	authenticatedOrigin.Host = net.JoinHostPort("localhost", authenticatedOrigin.Port())
	authenticatedOrigin.User = url.UserPassword("packy-test", "fixture-password")

	resolved, err := (resolver{
		local:     map[string]string{},
		temporary: t.TempDir(),
		repositoryURL: func(managedpack.Origin) string {
			return authenticatedOrigin.String() + "/upstream.git"
		},
	}).Resolve(context.Background(), managedpack.Origin{
		ID: "upstream", Repository: "example/upstream", Commit: headCommit.String(),
	})
	serverMu.Lock()
	originRequestCount := redirectRequests
	sinkRequestCount := sinkRequests
	paths := append([]string(nil), requestedPaths...)
	originCredentialCount := len(originAuthorizations)
	sinkCredentialCount := len(sinkAuthorizations)
	serverMu.Unlock()
	if err != nil {
		t.Fatalf("Resolve() error = %v; requested paths = %v", err, paths)
	}
	if originRequestCount == 0 || sinkRequestCount == 0 {
		t.Fatalf("redirected clone requests: origin = %d, sink = %d, paths = %v", originRequestCount, sinkRequestCount, paths)
	}
	if originCredentialCount == 0 {
		t.Fatal("origin did not receive fixture authentication")
	}
	if sinkCredentialCount != 0 {
		t.Fatalf("redirect target received %d Authorization headers", sinkCredentialCount)
	}
	if content, err := os.ReadFile(filepath.Join(resolved, "guide.txt")); err != nil || string(content) != "second\n" {
		t.Fatalf("redirected clone content = %q, %v", content, err)
	}
}

func toolManifestWithProjectionCollision(t *testing.T) string {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal([]byte(toolManifest), &manifest); err != nil {
		t.Fatal(err)
	}
	resources := manifest["resources"].([]any)
	encoded, err := json.Marshal(resources[1])
	if err != nil {
		t.Fatal(err)
	}
	var other map[string]any
	if err := json.Unmarshal(encoded, &other); err != nil {
		t.Fatal(err)
	}
	other["id"] = "other"
	other["source"] = "skills/other"
	delete(other, "origin")
	delete(other, "notices")
	manifest["resources"] = append(resources, other)
	encoded, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded) + "\n"
}

func writeToolFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeOriginRepository(t *testing.T) (string, plumbing.Hash, plumbing.Hash) {
	t.Helper()
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, filepath.Join(root, "guide.txt"), "first\n")
	if _, err := worktree.Add("guide.txt"); err != nil {
		t.Fatal(err)
	}
	signature := &object.Signature{Name: "Packy Test", Email: "packy@example.invalid", When: time.Unix(1, 0).UTC()}
	firstCommit, err := worktree.Commit("first", &git.CommitOptions{Author: signature})
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, filepath.Join(root, "guide.txt"), "second\n")
	if _, err := worktree.Add("guide.txt"); err != nil {
		t.Fatal(err)
	}
	secondCommit, err := worktree.Commit("second", &git.CommitOptions{Author: signature})
	if err != nil {
		t.Fatal(err)
	}
	return root, firstCommit, secondCommit
}

const toolManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [{"id":"upstream","repository":"example/upstream","commit":"0123456789abcdef0123456789abcdef01234567"}],
  "resources": [
    {
      "kind":"notice","id":"mit","source":"notices/mit","description":"MIT notice","license":"MIT","attribution":"Copyright Example",
      "requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]
    },
    {
      "kind":"skill","id":"guide","source":"skills/guide","description":"Guidance","requires":[],"conflicts":[],"notices":["notice:mit"],
      "origin":{"id":"upstream","path":"guide","relationship":"exact-copy"},
      "bindings":[{"surface":"codex","projection":"skill","name":"guide","invocation":"$guide","mode":"native","sharing":"exclusive","capabilities":[]}],
      "surface_exclusions":[]
    }
  ]
}
`

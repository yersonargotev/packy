package catalogorigin_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogtransaction"
	"github.com/yersonargotev/packy/internal/tools/catalogorigin"
)

type originLoader struct {
	repo      *git.Repository
	root      string
	origins   map[string]storer.Storer
	downloads atomic.Int32
}

func (l *originLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	l.downloads.Add(1)
	if s := l.origins[ep.String()]; s != nil {
		return s, nil
	}
	return nil, transport.ErrRepositoryNotFound
}
func originFixture(t *testing.T) (*originLoader, cataloglayout.Origin) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("SSL_CERT_FILE", "")
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide"), []byte("first revision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("guide"); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	loader := &originLoader{repo: repo, root: root, origins: map[string]storer.Storer{"https://github.com/example/origin.git": repo.Storer}}
	previous := client.Protocols["https"]
	client.InstallProtocol("https", server.NewClient(loader))
	t.Cleanup(func() { client.InstallProtocol("https", previous) })
	return loader, cataloglayout.Origin{Repository: "example/origin", Commit: hash.String()}
}
func resolve(t *testing.T, origin cataloglayout.Origin) (string, *catalogorigin.Resolver) {
	t.Helper()
	r, err := catalogorigin.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	root, err := r.Resolve(context.Background(), origin)
	if err != nil {
		t.Fatal(err)
	}
	return root, r
}
func TestIndependentResolversReuseExactSourceAndOwnPrivateCopies(t *testing.T) {
	loader, origin := originFixture(t)
	first, r := resolve(t, origin)
	if err := os.WriteFile(filepath.Join(first, "guide"), []byte("consumer change"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	second, _ := resolve(t, origin)
	data, err := os.ReadFile(filepath.Join(second, "guide"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first revision\n" {
		t.Fatalf("source = %q", data)
	}
	if got := loader.downloads.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1 across independent resolvers", got)
	}
}

func cacheRoot(t *testing.T) string {
	t.Helper()
	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "packy", "catalog-origins-v1")
}
func cacheEntries(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(cacheRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		paths = append(paths, filepath.Join(cacheRoot(t), entry.Name()))
	}
	return paths
}
func TestCorruptRetainedSourcesAreRejectedBeforeUse(t *testing.T) {
	for _, damage := range []string{"bytes", "git metadata", "symlink", "missing file", "extra file", "identity"} {
		t.Run(damage, func(t *testing.T) {
			loader, origin := originFixture(t)
			resolve(t, origin)
			entry := cacheEntries(t)[0]
			path := filepath.Join(entry, "checkout", "guide")
			var err error
			switch damage {
			case "bytes":
				err = os.WriteFile(path, []byte("tampered\n"), 0600)
			case "git metadata":
				err = os.WriteFile(filepath.Join(entry, "checkout", ".git", "HEAD"), []byte("bad"), 0600)
			case "symlink":
				err = os.Remove(path)
				if err == nil {
					err = os.Symlink("/outside", path)
				}
			case "missing file":
				err = os.Remove(path)
			case "extra file":
				err = os.WriteFile(filepath.Join(entry, "checkout", "injected"), []byte("bad"), 0600)
			case "identity":
				err = os.WriteFile(filepath.Join(entry, "seal.json"), []byte(`{"repository":"another/origin"}`), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			r, err := catalogorigin.New()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			root, err := r.Resolve(context.Background(), origin)
			if root != "" || err == nil || !strings.Contains(err.Error(), "remove this entry") {
				t.Fatalf("root=%q error=%v", root, err)
			}
			if loader.downloads.Load() != 1 {
				t.Fatal("unexpected automatic repair download")
			}
			if err := os.RemoveAll(entry); err != nil {
				t.Fatal(err)
			}
			resolve(t, origin)
			if loader.downloads.Load() != 2 {
				t.Fatal("explicit deletion did not reacquire exact origin")
			}
		})
	}
}
func TestConcurrentResolversPublishOnlyOneCompleteOrigin(t *testing.T) {
	loader, origin := originFixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := catalogorigin.New()
			if err != nil {
				results <- err
				return
			}
			defer r.Close()
			root, err := r.Resolve(context.Background(), origin)
			if err != nil {
				results <- err
				return
			}
			data, err := os.ReadFile(filepath.Join(root, "guide"))
			if err == nil && string(data) != "first revision\n" {
				err = fmt.Errorf("incomplete source: %q", data)
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if loader.downloads.Load() != 1 {
		t.Fatalf("downloads=%d", loader.downloads.Load())
	}
}
func TestFailedAndCanceledAcquisitionDoesNotPublish(t *testing.T) {
	loader, origin := originFixture(t)
	r, err := catalogorigin.New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	missing := origin
	missing.Repository = "example/missing"
	if root, err := r.Resolve(context.Background(), missing); err == nil || root != "" {
		t.Fatal("missing repository accepted")
	}
	missing = origin
	missing.Commit = strings.Repeat("a", 40)
	if root, err := r.Resolve(context.Background(), missing); err == nil || root != "" {
		t.Fatal("missing commit accepted")
	}
	if len(cacheEntries(t)) != 0 {
		t.Fatal("failed acquisition left reusable state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Resolve(ctx, origin); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	before := loader.downloads.Load()
	guard, err := catalogtransaction.Acquire(context.Background(), cacheRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = r.Resolve(ctx, origin)
	guard.Release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock deadline=%v", err)
	}
	if loader.downloads.Load() != before {
		t.Fatal("canceled waiter downloaded")
	}
	stage := filepath.Join(cacheRoot(t), "stage-interrupted")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "partial"), []byte("incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	resolve(t, origin)
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted stage not cleaned")
	}
	if len(cacheEntries(t)) != 1 {
		t.Fatal("unexpected retained state")
	}
}
func TestDistinctOriginsAndHistoricalCommitsRemainIndependent(t *testing.T) {
	loader, old := originFixture(t)
	if err := os.WriteFile(filepath.Join(loader.root, "guide"), []byte("second revision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	wt, err := loader.repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("guide"); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("second", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(2, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	newer := old
	newer.Commit = hash.String()
	loader.origins["https://github.com/other/origin.git"] = loader.repo.Storer
	other := old
	other.Repository = "other/origin"
	for _, item := range []struct {
		origin cataloglayout.Origin
		want   string
	}{{newer, "second revision\n"}, {old, "first revision\n"}, {other, "first revision\n"}, {newer, "second revision\n"}, {old, "first revision\n"}} {
		root, _ := resolve(t, item.origin)
		data, err := os.ReadFile(filepath.Join(root, "guide"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != item.want {
			t.Fatalf("origin %v = %q", item.origin, data)
		}
	}
	if loader.downloads.Load() != 3 {
		t.Fatalf("downloads=%d", loader.downloads.Load())
	}
}
func TestRetentionEvictsOldEntriesWithoutInvalidatingPrivateCopies(t *testing.T) {
	loader, origin := originFixture(t)
	private, _ := resolve(t, origin)
	first := cacheEntries(t)[0]
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(first, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	resolve(t, origin)
	if loader.downloads.Load() != 2 {
		t.Fatal("expired origin was reused")
	}
	for i := 0; i < 9; i++ {
		next := origin
		next.Repository = fmt.Sprintf("example/origin-%d", i)
		loader.origins["https://github.com/"+next.Repository+".git"] = loader.repo.Storer
		resolve(t, next)
	}
	if len(cacheEntries(t)) != 8 {
		t.Fatalf("entries=%d", len(cacheEntries(t)))
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("least recently used entry retained")
	}
	data, err := os.ReadFile(filepath.Join(private, "guide"))
	if err != nil || string(data) != "first revision\n" {
		t.Fatalf("eviction affected consumer: %q %v", data, err)
	}
}

type stalledTransport struct {
	transport.Transport
	entered chan struct{}
}

func (s stalledTransport) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	session, err := s.Transport.NewUploadPackSession(ep, auth)
	if err != nil {
		return nil, err
	}
	return stalledSession{session, s.entered}, nil
}

type stalledSession struct {
	transport.UploadPackSession
	entered chan struct{}
}

func (s stalledSession) UploadPack(ctx context.Context, _ *packp.UploadPackRequest) (*packp.UploadPackResponse, error) {
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestCancellationDuringDownloadLeavesNoReusableEntry(t *testing.T) {
	_, origin := originFixture(t)
	upstream := client.Protocols["https"]
	entered := make(chan struct{})
	client.InstallProtocol("https", stalledTransport{upstream, entered})
	r, err := catalogorigin.New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := r.Resolve(ctx, origin); result <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("download never started")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("download cancellation=%v", err)
	}
	if len(cacheEntries(t)) != 0 {
		t.Fatal("canceled download left reusable state")
	}
	client.InstallProtocol("https", upstream)
	resolve(t, origin)
}

func TestCancellationDuringPrivateCopyDoesNotPublish(t *testing.T) {
	loader, origin := originFixture(t)
	// A large tracked file keeps the post-download filesystem copy observable.
	if err := os.WriteFile(filepath.Join(loader.root, "large"), make([]byte, 32<<20), 0600); err != nil {
		t.Fatal(err)
	}
	wt, err := loader.repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("large"); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("large source", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(2, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	origin.Commit = hash.String()
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	r, err := catalogorigin.New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := r.Resolve(ctx, origin); result <- err }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			t.Fatalf("resolution completed before cancellation injection: %v", err)
		case <-deadline:
			t.Fatal("private copy did not start")
		case <-ticker.C:
			copies, err := filepath.Glob(filepath.Join(temporary, "packy-catalog-origins-*", "checkout-*", "large"))
			if err != nil {
				t.Fatal(err)
			}
			if len(copies) == 0 {
				continue
			}
			cancel()
			err = <-result
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("copy cancellation=%v", err)
			}
			if strings.Contains(err.Error(), "invalid") {
				t.Fatalf("cancellation misreported as corruption: %v", err)
			}
			if len(cacheEntries(t)) != 0 {
				t.Fatal("canceled cold copy published a reusable entry")
			}
			return
		}
	}
}

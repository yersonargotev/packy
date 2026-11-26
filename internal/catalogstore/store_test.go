package catalogstore_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

var official = catalogstore.Official().Repository

func TestStoreAcquiresValidatesAndSelectsAnImmutableOfficialSnapshot(t *testing.T) {
	commit := strings.Repeat("a", 40)
	release := validRelease(t, commit, validManifest("example", "1.2.3"))
	dataRoot := t.TempDir()
	store := catalogstore.New(dataRoot, staticSource{release: release})

	snapshot, err := store.AcquireLatest(context.Background(), official)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != commit || snapshot.Commit != commit || snapshot.Tag != "catalog-"+commit {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	catalog := filepath.Join(dataRoot, "catalog", "v2", "yersonargotev", "packy-catalog")
	if snapshot.Root != filepath.Join(catalog, "snapshots", commit) {
		t.Fatalf("snapshot root = %s, want below %s", snapshot.Root, catalog)
	}
	if _, err := os.Stat(filepath.Join(catalog, "selected.json")); err != nil {
		t.Fatal(err)
	}
	selected, err := store.Selected(official)
	if err != nil || selected.ID != commit {
		t.Fatalf("Selected() = %#v, %v", selected, err)
	}
	root, err := store.Resolve(official, commit)
	if err != nil || root != snapshot.Root {
		t.Fatalf("Resolve() = %q, %v; want %q", root, err, snapshot.Root)
	}
}

func TestFailedAcquisitionPreservesPreviousSelection(t *testing.T) {
	firstCommit := strings.Repeat("a", 40)
	root := t.TempDir()
	store := catalogstore.New(root, staticSource{release: validRelease(t, firstCommit, validManifest("example", "1.2.3"))})
	if _, err := store.AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*catalogstore.Release)
		want   string
	}{
		{"unofficial publisher", func(r *catalogstore.Release) { r.Publisher = "someone" }, "publisher metadata"},
		{"release of another repository", func(r *catalogstore.Release) { r.Repository = "acme/packs" }, "publisher metadata"},
		{"index of another repository", func(r *catalogstore.Release) {
			*r = catalogRelease(t, official.String(), "acme/packs", r.Commit, validManifest("newer", "2.0.0"))
		}, "index names acme/packs@"},
		{"mutable release", func(r *catalogstore.Release) { r.Immutable = false }, "immutable"},
		{"unverified attestation", func(r *catalogstore.Release) { r.AttestationVerified = false }, "attestation"},
		{"unexpected asset", func(r *catalogstore.Release) { r.Assets = append(r.Assets, catalogstore.Asset{Name: "extra"}) }, "exactly"},
		{"API digest mismatch", func(r *catalogstore.Release) { r.Assets[0].SHA256 = strings.Repeat("0", 64) }, "API SHA-256"},
		{"checksum mismatch", func(r *catalogstore.Release) {
			r.Assets[1].Data = []byte(strings.Repeat("0", 64) + "  catalog-snapshot.tar.gz\n")
			r.Assets[1].SHA256 = digest(r.Assets[1].Data)
		}, "checksum"},
		{"retired manifest generation", func(r *catalogstore.Release) {
			replaceManifest(t, r, strings.Replace(validManifest("newer", "2.0.0"), `"schema_version":3`, `"schema_version":2`, 1))
		}, "newer Packy engine"},
		{"unknown engine vocabulary", func(r *catalogstore.Release) {
			replaceManifest(t, r, strings.Replace(validManifest("newer", "2.0.0"), `"resources":[`, `"future_engine_field":true,"resources":[`, 1))
		}, "newer Packy engine"},
		{"unsupported capability", func(r *catalogstore.Release) {
			replaceManifest(t, r, strings.Replace(validManifest("newer", "2.0.0"), `"capabilities":[]`, `"capabilities":[{"type":"future-capability"}]`, 1))
		}, "newer Packy engine"},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validRelease(t, fmt.Sprintf("%040x", i+2), validManifest("newer", "2.0.0"))
			test.mutate(&candidate)
			failed := catalogstore.New(root, staticSource{release: candidate})
			if _, err := failed.AcquireLatest(context.Background(), official); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("AcquireLatest() error = %v, want %q", err, test.want)
			}
			selected, err := failed.Selected(official)
			if err != nil || selected.ID != firstCommit {
				t.Fatalf("selection after failure = %#v, %v", selected, err)
			}
		})
	}
}

func TestConcurrentRepeatAcquisitionRetainsOneValidSnapshot(t *testing.T) {
	commit := strings.Repeat("a", 40)
	release := validRelease(t, commit, validManifest("example", "1.2.3"))
	root := t.TempDir()
	stores := []*catalogstore.Store{catalogstore.New(root, staticSource{release: release}), catalogstore.New(root, staticSource{release: release})}
	var wait sync.WaitGroup
	errors := make(chan error, len(stores))
	for _, store := range stores {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.AcquireLatest(context.Background(), official)
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	selected, err := stores[0].Selected(official)
	if err != nil || selected.ID != commit {
		t.Fatalf("Selected() = %#v, %v", selected, err)
	}
}

func TestConcurrentAcquisitionFetchesLatestReleaseInsideStoreLock(t *testing.T) {
	oldCommit := strings.Repeat("a", 40)
	newCommit := strings.Repeat("b", 40)
	source := &orderedSource{
		releases: []catalogstore.Release{
			validRelease(t, oldCommit, validManifest("example", "1.2.3")),
			validRelease(t, newCommit, validManifest("example", "1.2.4")),
		},
		firstStarted:  make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		secondStarted: make(chan struct{}),
	}
	root := t.TempDir()
	first := catalogstore.New(root, source)
	second := catalogstore.New(root, source)
	firstDone := make(chan error, 1)
	go func() {
		_, err := first.AcquireLatest(context.Background(), official)
		firstDone <- err
	}()
	<-source.firstStarted

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := second.AcquireLatest(cancelled, official); err == nil || !strings.Contains(err.Error(), "lock catalog store") {
		t.Fatalf("cancelled concurrent acquisition error = %v, want store lock failure", err)
	}
	select {
	case <-source.secondStarted:
		t.Fatal("second Latest started while the first acquisition held the store lock")
	default:
	}

	close(source.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if _, err := second.AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}
	selected, err := second.Selected(official)
	if err != nil || selected.ID != newCommit {
		t.Fatalf("Selected() = %#v, %v; want new release %s", selected, err, newCommit)
	}
}

func TestRetainedSnapshotReadsFailClosedAfterLocalTampering(t *testing.T) {
	commit := strings.Repeat("a", 40)
	store := catalogstore.New(t.TempDir(), staticSource{release: validRelease(t, commit, validManifest("example", "1.2.3"))})
	snapshot, err := store.AcquireLatest(context.Background(), official)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot.Root, "packs", "example", "skills", "example", "SKILL.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Selected(official); err == nil || !strings.Contains(err.Error(), "indexed identity") {
		t.Fatalf("Selected() error = %v, want integrity failure", err)
	}
	if _, err := store.Resolve(official, commit); err == nil || !strings.Contains(err.Error(), "indexed identity") {
		t.Fatalf("Resolve() error = %v, want integrity failure", err)
	}
}

func TestRefreshOverATamperedRetainedSnapshotFailsAndKeepsTheSelection(t *testing.T) {
	commit := strings.Repeat("a", 40)
	release := validRelease(t, commit, validManifest("example", "1.2.3"))
	store := catalogstore.New(t.TempDir(), staticSource{release: release})
	snapshot, err := store.AcquireLatest(context.Background(), official)
	if err != nil {
		t.Fatal(err)
	}
	selection := filepath.Join(filepath.Dir(filepath.Dir(snapshot.Root)), "selected.json")
	before, err := os.ReadFile(selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot.Root, "packs", "example", "pack.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLatest(context.Background(), official); err == nil || !strings.Contains(err.Error(), "conflicts with release") {
		t.Fatalf("AcquireLatest() error = %v, want retained conflict", err)
	}
	if after, err := os.ReadFile(selection); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("selection changed after a refused refresh: %s %v", after, err)
	}
}

func TestFailedSourceKeepsThePreviousSelection(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("a", 40)
	if _, err := catalogstore.New(root, staticSource{release: validRelease(t, commit, validManifest("example", "1.2.3"))}).AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}
	failing := catalogstore.New(root, staticSource{err: fmt.Errorf("GitHub returned 500")})
	if _, err := failing.AcquireLatest(context.Background(), official); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("AcquireLatest() error = %v", err)
	}
	if selected, err := failing.Selected(official); err != nil || selected.ID != commit {
		t.Fatalf("Selected() = %#v, %v", selected, err)
	}
}

func TestRegistrationLifecycleKeepsRetainedSnapshots(t *testing.T) {
	root := t.TempDir()
	acme := mustRepository(t, "Acme/Packs")
	private := mustRepository(t, "acme/private")
	commit := strings.Repeat("c", 40)
	unattested := catalogRelease(t, "acme/private", "acme/private", commit, validManifest("secret", "1.0.0"))
	unattested.AttestationVerified = false
	source := catalogSource{
		official.String(): validRelease(t, strings.Repeat("a", 40), validManifest("example", "1.2.3")),
		"acme/packs":      catalogRelease(t, "acme/packs", "ACME/packs", strings.Repeat("b", 40), validManifest("tools", "1.0.0")),
		"acme/private":    unattested,
	}
	store := catalogstore.New(root, source)
	if _, err := store.AcquireLatest(context.Background(), acme); err == nil || !strings.Contains(err.Error(), "catalog acme/packs is not registered") {
		t.Fatalf("AcquireLatest() of an unregistered catalog = %v", err)
	}
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: acme, Trust: catalogstore.Attested{Workflow: ".github/workflows/publish.yml"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: private, Trust: catalogstore.AccessControlled{}}); err != nil {
		t.Fatal(err)
	}
	unattested.Repository, unattested.Commit, unattested.Tag = "acme/unverified", strings.Repeat("d", 40), "catalog-"+strings.Repeat("d", 40)
	source["acme/unverified"] = unattested
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: mustRepository(t, "acme/unverified"), Trust: catalogstore.Attested{Workflow: ".github/workflows/publish.yml"}}); err == nil || !strings.Contains(err.Error(), "attestation is not verified") {
		t.Fatalf("Register() of an attested catalog without attestation = %v", err)
	}
	registrations, err := store.Registrations()
	if err != nil {
		t.Fatal(err)
	}
	want := []catalogstore.Registration{
		catalogstore.Official(),
		{Repository: acme, Trust: catalogstore.Attested{Workflow: ".github/workflows/publish.yml"}},
		{Repository: private, Trust: catalogstore.AccessControlled{}},
	}
	if fmt.Sprint(registrations) != fmt.Sprint(want) {
		t.Fatalf("Registrations() = %v, want %v", registrations, want)
	}
	selected, err := store.Selected(acme)
	if err != nil || selected.ID != strings.Repeat("b", 40) {
		t.Fatalf("Selected(acme/packs) = %#v, %v", selected, err)
	}
	if _, err := store.Register(context.Background(), want[1]); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("second Register() = %v", err)
	}
	if _, err := store.Register(context.Background(), catalogstore.Official()); err == nil || !strings.Contains(err.Error(), "built in") {
		t.Fatalf("Register(official) = %v", err)
	}
	if err := store.Unregister(context.Background(), official); err == nil || !strings.Contains(err.Error(), "cannot be removed") {
		t.Fatalf("Unregister(official) = %v", err)
	}
	if err := store.Unregister(context.Background(), acme); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(selected.Root); err != nil {
		t.Fatalf("Unregister() removed the retained snapshot: %v", err)
	}
	if _, err := store.Selected(acme); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Selected() after Unregister() = %v", err)
	}
	if _, err := store.Resolve(acme, selected.ID); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Resolve() after Unregister() = %v", err)
	}
	if registrations, err := store.Registrations(); err != nil || len(registrations) != 2 {
		t.Fatalf("Registrations() after Unregister() = %v, %v", registrations, err)
	}
	if _, err := store.Register(context.Background(), want[1]); err != nil {
		t.Fatalf("Register() over retained snapshots: %v", err)
	}
}

func TestPackIDCollisionIsRejectedWithoutResidue(t *testing.T) {
	root := t.TempDir()
	acme := mustRepository(t, "acme/packs")
	officialCommit := strings.Repeat("a", 40)
	source := catalogSource{
		official.String(): validRelease(t, officialCommit, validManifest("example", "1.2.3")),
		"acme/packs":      catalogRelease(t, "acme/packs", "acme/packs", strings.Repeat("b", 40), validManifest("example", "9.0.0")),
	}
	store := catalogstore.New(root, source)
	if _, err := store.AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}
	registration := catalogstore.Registration{Repository: acme, Trust: catalogstore.Attested{Workflow: ".github/workflows/publish.yml"}}
	if _, err := store.Register(context.Background(), registration); err == nil || !strings.Contains(err.Error(), `introduces Pack "example"`) || !strings.Contains(err.Error(), official.String()) {
		t.Fatalf("Register() with a colliding Pack ID = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog", "v2", "acme")); !os.IsNotExist(err) {
		t.Fatalf("rejected registration left residue: %v", err)
	}
	if registrations, err := store.Registrations(); err != nil || len(registrations) != 1 {
		t.Fatalf("Registrations() = %v, %v", registrations, err)
	}

	source["acme/packs"] = catalogRelease(t, "acme/packs", "acme/packs", strings.Repeat("b", 40), validManifest("tools", "1.0.0"))
	if _, err := store.Register(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	source[official.String()] = validRelease(t, strings.Repeat("c", 40), validManifest("tools", "2.0.0"))
	if _, err := store.AcquireLatest(context.Background(), official); err == nil || !strings.Contains(err.Error(), `introduces Pack "tools"`) {
		t.Fatalf("official refresh with a colliding Pack ID = %v", err)
	}
	if selected, err := store.Selected(official); err != nil || selected.ID != officialCommit {
		t.Fatalf("official selection after collision = %#v, %v", selected, err)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog", "v2", "yersonargotev", "packy-catalog", "snapshots", strings.Repeat("c", 40))); !os.IsNotExist(err) {
		t.Fatalf("colliding snapshot was retained: %v", err)
	}
}

func TestEarlierStoreLayoutIsRejectedWithAdoptionGuidance(t *testing.T) {
	root := t.TempDir()
	earlier := filepath.Join(root, "catalog")
	commit := strings.Repeat("a", 40)
	files := map[string]string{
		"selected.json": `{"schema_version":1,"snapshot_id":"` + commit + `"}` + "\n",
		filepath.Join("snapshots", commit, "catalog-index.json"): "{}\n",
	}
	for name, content := range files {
		path := filepath.Join(earlier, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := catalogstore.New(root, staticSource{release: validRelease(t, strings.Repeat("b", 40), validManifest("example", "1.2.3"))})
	if _, err := store.Selected(official); err == nil || !strings.Contains(err.Error(), "docs/catalog-adoption.md") {
		t.Fatalf("Selected() over the earlier layout = %v", err)
	}
	if _, err := store.Resolve(official, commit); err == nil || !strings.Contains(err.Error(), "docs/catalog-adoption.md") {
		t.Fatalf("Resolve() over the earlier layout = %v", err)
	}
	if _, err := store.AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}
	if selected, err := store.Selected(official); err != nil || selected.ID != strings.Repeat("b", 40) {
		t.Fatalf("Selected() after refresh = %#v, %v", selected, err)
	}
	for name, content := range files {
		if data, err := os.ReadFile(filepath.Join(earlier, name)); err != nil || string(data) != content {
			t.Fatalf("earlier layout file %s changed: %q %v", name, data, err)
		}
	}
}

func TestSelectionMustNameItsCatalog(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("a", 40)
	store := catalogstore.New(root, staticSource{release: validRelease(t, commit, validManifest("example", "1.2.3"))})
	snapshot, err := store.AcquireLatest(context.Background(), official)
	if err != nil {
		t.Fatal(err)
	}
	selection := filepath.Join(filepath.Dir(filepath.Dir(snapshot.Root)), "selected.json")
	data, err := os.ReadFile(selection)
	if err != nil || !strings.Contains(string(data), `"repository": "yersonargotev/packy-catalog"`) {
		t.Fatalf("selected.json = %s, %v", data, err)
	}
	for _, document := range []string{
		`{"schema_version":1,"snapshot_id":"` + commit + `"}`,
		`{"schema_version":2,"repository":"acme/packs","snapshot_id":"` + commit + `"}`,
	} {
		if err := os.WriteFile(selection, []byte(document), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Selected(official); err == nil || !strings.Contains(err.Error(), "invalid identity") {
			t.Fatalf("Selected() accepted %s: %v", document, err)
		}
	}
}

func TestRegistrationsFailClosedOnAnInvalidRegistration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog", "v2", "acme", "packs", "registration.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, document := range []string{
		`{"schema_version":1,"repository":"acme/packs","trust":"unverified"}`,
		`{"schema_version":1,"repository":"acme/packs","trust":"attested","workflow":"../publish.yml"}`,
		`{"schema_version":1,"repository":"acme/other","trust":"access-controlled"}`,
		`{"schema_version":1,"repository":"acme/packs","trust":"access-controlled","extra":true}`,
	} {
		if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
			t.Fatal(err)
		}
		if registrations, err := catalogstore.New(root, nil).Registrations(); err == nil {
			t.Fatalf("Registrations() accepted %s: %v", document, registrations)
		}
	}
}

func TestParseRepositoryCanonicalizesGitHubNames(t *testing.T) {
	repository, err := catalogstore.ParseRepository("Acme/Packs.v2")
	if err != nil || repository.String() != "acme/packs.v2" {
		t.Fatalf("ParseRepository() = %v, %v", repository, err)
	}
	for _, value := range []string{"", "acme", "acme/", "/packs", "acme/packs/extra", "../packs", "acme/..", "acme/pa cks", "https://github.com/acme/packs"} {
		if _, err := catalogstore.ParseRepository(value); err == nil {
			t.Errorf("ParseRepository(%q) succeeded", value)
		}
	}
}

func mustRepository(t *testing.T, value string) catalogstore.Repository {
	t.Helper()
	repository, err := catalogstore.ParseRepository(value)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

type staticSource struct {
	release catalogstore.Release
	err     error
}

type orderedSource struct {
	mu            sync.Mutex
	releases      []catalogstore.Release
	firstStarted  chan struct{}
	releaseFirst  chan struct{}
	secondStarted chan struct{}
	calls         int
}

func (s *orderedSource) Latest(context.Context, catalogstore.Registration) (catalogstore.Release, error) {
	s.mu.Lock()
	call := s.calls
	s.calls++
	s.mu.Unlock()
	switch call {
	case 0:
		close(s.firstStarted)
		<-s.releaseFirst
	case 1:
		close(s.secondStarted)
	default:
		return catalogstore.Release{}, fmt.Errorf("unexpected Latest call %d", call+1)
	}
	return s.releases[call], nil
}

func (s staticSource) Latest(context.Context, catalogstore.Registration) (catalogstore.Release, error) {
	return s.release, s.err
}

type catalogSource map[string]catalogstore.Release

func (s catalogSource) Latest(_ context.Context, registration catalogstore.Registration) (catalogstore.Release, error) {
	release, ok := s[registration.Repository.String()]
	if !ok {
		return catalogstore.Release{}, fmt.Errorf("no release for %s", registration.Repository)
	}
	return release, nil
}

func validRelease(t *testing.T, commit, manifest string) catalogstore.Release {
	t.Helper()
	return catalogRelease(t, official.String(), official.String(), commit, manifest)
}

func catalogRelease(t *testing.T, repository, indexed, commit, manifest string) catalogstore.Release {
	t.Helper()
	archive := snapshotArchive(t, indexed, commit, manifest)
	checksum := []byte(digest(archive) + "  catalog-snapshot.tar.gz\n")
	return catalogstore.Release{
		Repository: repository, Tag: "catalog-" + commit, Commit: commit,
		Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true,
		Assets: []catalogstore.Asset{{Name: "catalog-snapshot.tar.gz", SHA256: digest(archive), Data: archive}, {Name: "SHA256SUMS", SHA256: digest(checksum), Data: checksum}},
	}
}

func replaceManifest(t *testing.T, release *catalogstore.Release, manifest string) {
	t.Helper()
	*release = catalogRelease(t, release.Repository, release.Repository, release.Commit, manifest)
}

func snapshotArchive(t *testing.T, repository, commit, manifest string) []byte {
	t.Helper()
	manifestBytes := []byte(manifest)
	skillBytes := []byte("# Fixture\n")
	files := []cataloglayout.FileRecord{
		{Path: "pack.json", Mode: "100644", SHA256: digest(manifestBytes)},
		{Path: "skills/" + manifestID(t, manifest) + "/SKILL.md", Mode: "100644", SHA256: digest(skillBytes)},
	}
	closure := digestFileIndex(files)
	packs := []cataloglayout.CatalogSnapshotPack{{ID: manifestID(t, manifest), Version: manifestVersion(t, manifest), ManifestSHA256: digest(manifestBytes), ClosureSHA256: closure, Files: files}}
	encodedPacks, _ := json.Marshal(packs)
	index := cataloglayout.CatalogSnapshotIndex{SchemaVersion: 3, Source: cataloglayout.CatalogSnapshotSource{Repository: repository, Commit: commit}, Builder: "yersonargotev/packy@" + strings.Repeat("b", 40), CatalogSHA256: digest(encodedPacks), Packs: packs}
	indexBytes, _ := json.Marshal(index)

	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, "catalog-index.json", indexBytes)
	writeTar(t, tw, "packs/"+manifestID(t, manifest)+"/"+files[0].Path, manifestBytes)
	writeTar(t, tw, "packs/"+manifestID(t, manifest)+"/"+files[1].Path, skillBytes)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}

func validManifest(id, version string) string {
	return fmt.Sprintf(`{"schema_version":3,"id":%q,"version":%q,"description":"Fixture Pack","selectable":true,"surfaces":["agents"],"readiness_obligations":["runtime-usability","surface-authorization"],"external_requirements":[],"origins":[],"resources":[{"kind":"skill","id":%q,"source":%q,"description":"Fixture skill","requires":[],"conflicts":[],"bindings":[{"surface":"agents","projection":"skill","name":%q,"invocation":%q,"mode":"native","sharing":"exclusive","capabilities":[]}],"surface_exclusions":[]}]}`, id, version, id, "skills/"+id, id, "$"+id)
}

func manifestID(t *testing.T, data string) string {
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	return v.ID
}
func manifestVersion(t *testing.T, data string) string {
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatal(err)
	}
	return v.Version
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func digestFileIndex(files []cataloglayout.FileRecord) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", f.Path, f.Mode, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

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

	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/managedpack"
)

const officialRepository = "yersonargotev/packy-catalog"

func TestStoreAcquiresValidatesAndSelectsAnImmutableOfficialSnapshot(t *testing.T) {
	commit := strings.Repeat("a", 40)
	release := validRelease(t, commit, validManifest("example", "1.2.3"))
	store := catalogstore.New(t.TempDir(), staticSource{release: release})

	snapshot, err := store.AcquireLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != commit || snapshot.Commit != commit || snapshot.Tag != "catalog-"+commit {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if got, want := store.Root(), filepath.Join(filepath.Dir(store.Root()), "catalog"); got != want {
		t.Fatalf("Root() = %q, want %q", got, want)
	}
	selected, err := store.Selected()
	if err != nil || selected.ID != commit {
		t.Fatalf("Selected() = %#v, %v", selected, err)
	}
	bundle, err := store.Resolve(commit)
	if err != nil || bundle != snapshot.BundleRoot {
		t.Fatalf("Resolve() = %q, %v; want %q", bundle, err, snapshot.BundleRoot)
	}
}

func TestFailedAcquisitionPreservesPreviousSelection(t *testing.T) {
	firstCommit := strings.Repeat("a", 40)
	root := t.TempDir()
	store := catalogstore.New(root, staticSource{release: validRelease(t, firstCommit, validManifest("example", "1.2.3"))})
	if _, err := store.AcquireLatest(context.Background()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*catalogstore.Release)
		want   string
	}{
		{"unofficial publisher", func(r *catalogstore.Release) { r.Publisher = "someone" }, "official publisher"},
		{"mutable release", func(r *catalogstore.Release) { r.Immutable = false }, "immutable"},
		{"unverified attestation", func(r *catalogstore.Release) { r.AttestationVerified = false }, "attestation"},
		{"unexpected asset", func(r *catalogstore.Release) { r.Assets = append(r.Assets, catalogstore.Asset{Name: "extra"}) }, "exactly"},
		{"API digest mismatch", func(r *catalogstore.Release) { r.Assets[0].SHA256 = strings.Repeat("0", 64) }, "API SHA-256"},
		{"checksum mismatch", func(r *catalogstore.Release) {
			r.Assets[1].Data = []byte(strings.Repeat("0", 64) + "  catalog-snapshot.tar.gz\n")
			r.Assets[1].SHA256 = digest(r.Assets[1].Data)
		}, "checksum"},
		{"unknown engine vocabulary", func(r *catalogstore.Release) {
			replaceManifest(t, r, strings.Replace(validManifest("newer", "2.0.0"), `"resources":[`, `"future_engine_field":true,"resources":[`, 1))
		}, "newer Packy engine"},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validRelease(t, fmt.Sprintf("%040x", i+2), validManifest("newer", "2.0.0"))
			test.mutate(&candidate)
			failed := catalogstore.New(root, staticSource{release: candidate})
			if _, err := failed.AcquireLatest(context.Background()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("AcquireLatest() error = %v, want %q", err, test.want)
			}
			selected, err := failed.Selected()
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
			_, err := store.AcquireLatest(context.Background())
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
	selected, err := stores[0].Selected()
	if err != nil || selected.ID != commit {
		t.Fatalf("Selected() = %#v, %v", selected, err)
	}
}

func TestRetainedSnapshotReadsFailClosedAfterLocalTampering(t *testing.T) {
	commit := strings.Repeat("a", 40)
	store := catalogstore.New(t.TempDir(), staticSource{release: validRelease(t, commit, validManifest("example", "1.2.3"))})
	snapshot, err := store.AcquireLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot.BundleRoot, "skills", "example", "SKILL.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Selected(); err == nil || !strings.Contains(err.Error(), "indexed identity") {
		t.Fatalf("Selected() error = %v, want integrity failure", err)
	}
	if _, err := store.Resolve(commit); err == nil || !strings.Contains(err.Error(), "indexed identity") {
		t.Fatalf("Resolve() error = %v, want integrity failure", err)
	}
}

type staticSource struct {
	release catalogstore.Release
	err     error
}

func (s staticSource) Latest(context.Context, string) (catalogstore.Release, error) {
	return s.release, s.err
}

func validRelease(t *testing.T, commit, manifest string) catalogstore.Release {
	t.Helper()
	archive := snapshotArchive(t, commit, manifest)
	checksum := []byte(digest(archive) + "  catalog-snapshot.tar.gz\n")
	return catalogstore.Release{
		Repository: officialRepository, Tag: "catalog-" + commit, Commit: commit,
		Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true,
		Assets: []catalogstore.Asset{{Name: "catalog-snapshot.tar.gz", SHA256: digest(archive), Data: archive}, {Name: "SHA256SUMS", SHA256: digest(checksum), Data: checksum}},
	}
}

func replaceManifest(t *testing.T, release *catalogstore.Release, manifest string) {
	t.Helper()
	commit := release.Commit
	*release = validRelease(t, commit, manifest)
}

func snapshotArchive(t *testing.T, commit, manifest string) []byte {
	t.Helper()
	manifestBytes := []byte(manifest)
	skillBytes := []byte("# Fixture\n")
	files := []managedpack.FileRecord{
		{Path: "packs/" + manifestID(t, manifest) + "/pack.json", Mode: "100644", SHA256: digest(manifestBytes)},
		{Path: "skills/" + manifestID(t, manifest) + "/SKILL.md", Mode: "100644", SHA256: digest(skillBytes)},
	}
	closure := digestFileIndex(files)
	packs := []managedpack.CatalogSnapshotPack{{ID: manifestID(t, manifest), Version: manifestVersion(t, manifest), ManifestSHA256: digest(manifestBytes), ClosureSHA256: closure, Files: files}}
	encodedPacks, _ := json.Marshal(packs)
	index := managedpack.CatalogSnapshotIndex{SchemaVersion: 1, Source: managedpack.CatalogSnapshotSource{Repository: officialRepository, Commit: commit}, Builder: "yersonargotev/packy@" + strings.Repeat("b", 40), CatalogSHA256: digest(encodedPacks), Packs: packs}
	indexBytes, _ := json.Marshal(index)

	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, "catalog-index.json", indexBytes)
	writeTar(t, tw, "bundle/"+files[0].Path, manifestBytes)
	writeTar(t, tw, "bundle/"+files[1].Path, skillBytes)
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
	return fmt.Sprintf(`{"schema_version":1,"id":%q,"version":%q,"description":"Fixture Pack","selectable":true,"surfaces":["codex"],"readiness_obligations":["runtime-usability","surface-authorization"],"external_requirements":[],"origins":[],"resources":[{"kind":"skill","id":%q,"source":%q,"description":"Fixture skill","requires":[],"conflicts":[],"bindings":[{"surface":"codex","projection":"skill","name":%q,"invocation":%q,"mode":"native","sharing":"exclusive","capabilities":[]}],"surface_exclusions":[]}]}`, id, version, id, "skills/"+id, id, "$"+id)
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
func digestFileIndex(files []managedpack.FileRecord) string {
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", f.Path, f.Mode, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

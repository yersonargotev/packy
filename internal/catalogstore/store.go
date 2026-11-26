// Package catalogstore owns registration, acquisition, validation, retention,
// and selection of immutable Catalog Snapshots of registered GitHub catalogs.
package catalogstore

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogtransaction"
)

const (
	publisher         = "github-actions[bot]"
	archiveAsset      = "catalog-snapshot.tar.gz"
	checksumAsset     = "SHA256SUMS"
	registrationFile  = "registration.json"
	selectionFile     = "selected.json"
	maxArchiveBytes   = 96 << 20
	maxExtractedBytes = 64 << 20
	maxFileBytes      = 8 << 20
	maxFiles          = 2048
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	idPattern      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	builderPattern = regexp.MustCompile(`^yersonargotev/packy@[0-9a-f]{40}$`)
)

// ErrEarlierLayout reports a Catalog Store that holds only the layout that
// predates catalog registrations.
var ErrEarlierLayout = errors.New("the Catalog Store holds only the layout that predates catalog registrations, which this Packy does not read; follow " + capabilitypack.CatalogAdoptionGuide)

// Source is the network boundary used only by explicit acquisition.
type Source interface {
	Latest(context.Context, Registration) (Release, error)
}

// Release is the complete downloaded evidence for one GitHub release.
type Release struct {
	Repository          string
	Tag                 string
	Commit              string
	Publisher           string
	Published           bool
	Draft               bool
	Prerelease          bool
	Immutable           bool
	AttestationVerified bool
	Assets              []Asset
}

// Asset carries GitHub's API digest together with the downloaded bytes.
type Asset struct {
	Name   string
	SHA256 string
	Data   []byte
}

// Snapshot identifies one retained immutable Catalog Snapshot.
type Snapshot struct {
	ID     string
	Commit string
	Tag    string
	Root   string
	Index  cataloglayout.CatalogSnapshotIndex
}

// Store is a filesystem-backed Catalog Snapshot store keyed by catalog
// registration. Each catalog keeps its selection and retained snapshots below
// catalog/v2/<owner>/<repo>, and every catalog except the built-in official one
// also keeps its registration.json there.
type Store struct {
	root   string
	source Source
}

// DefaultDataRoot returns Packy's user data root.
func DefaultDataRoot(home string) string {
	return filepath.Join(home, ".local", "share", "packy")
}

// New constructs the default catalog store beneath a caller-provided data root.
func New(dataRoot string, source Source) *Store {
	return &Store{root: filepath.Join(dataRoot, "catalog"), source: source}
}

// AcquireLatest downloads, fully validates, retains, and finally selects the
// latest immutable snapshot of a registered catalog.
func (s *Store) AcquireLatest(ctx context.Context, repository Repository) (Snapshot, error) {
	guard, err := s.lock(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer guard.Release()
	return s.selectLatest(ctx, repository)
}

func (s *Store) selectLatest(ctx context.Context, repository Repository) (Snapshot, error) {
	registration, err := s.registration(repository)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, _, err := s.retainLatest(ctx, registration)
	if err != nil {
		return Snapshot{}, err
	}
	if err := writeDocument(s.catalog(repository), selectionFile, newSelection(repository, snapshot.ID)); err != nil {
		return Snapshot{}, fmt.Errorf("select Catalog Snapshot: %w", err)
	}
	return snapshot, nil
}

// Register acquires, validates, and selects the initial snapshot of a new
// catalog, then records its registration. When the official catalog has no
// selection, Register first selects its latest snapshot, so the new catalog's
// Pack IDs are always checked against it. A failed registration leaves no
// registration, selection, or newly retained snapshot of the new catalog
// behind.
func (s *Store) Register(ctx context.Context, registration Registration) (Snapshot, error) {
	if err := registration.validate(); err != nil {
		return Snapshot{}, err
	}
	if registration.Repository == Official().Repository {
		return Snapshot{}, fmt.Errorf("catalog %s is built in and always registered", registration.Repository)
	}
	guard, err := s.lock(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer guard.Release()
	directory := s.catalog(registration.Repository)
	if _, err := os.Lstat(filepath.Join(directory, registrationFile)); err == nil {
		return Snapshot{}, fmt.Errorf("catalog %s is already registered", registration.Repository)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("inspect catalog %s registration: %w", registration.Repository, err)
	}
	if _, err := s.readSelection(Official().Repository); errors.Is(err, fs.ErrNotExist) {
		if _, err := s.selectLatest(ctx, Official().Repository); err != nil {
			return Snapshot{}, fmt.Errorf("register catalog %s: select the official catalog first: %w", registration.Repository, err)
		}
	} else if err != nil {
		return Snapshot{}, fmt.Errorf("register catalog %s: %w", registration.Repository, err)
	}
	_, statErr := os.Lstat(directory)
	fresh := errors.Is(statErr, fs.ErrNotExist)
	snapshot, created, err := s.retainLatest(ctx, registration)
	if err == nil {
		err = writeDocument(directory, selectionFile, newSelection(registration.Repository, snapshot.ID))
	}
	if err == nil {
		err = writeDocument(directory, registrationFile, encodeRegistration(registration))
	}
	if err != nil {
		if fresh {
			_ = os.RemoveAll(directory)
			_ = os.Remove(filepath.Dir(directory))
		} else {
			_ = os.Remove(filepath.Join(directory, selectionFile))
			if created {
				_ = os.RemoveAll(snapshot.Root)
			}
		}
		return Snapshot{}, fmt.Errorf("register catalog %s: %w", registration.Repository, err)
	}
	return snapshot, nil
}

// Unregister removes a catalog registration and its selection. Retained
// snapshots stay in place. inUse runs under the store lock and refuses the
// removal with its error, as when an installed Pack receipt names the catalog.
func (s *Store) Unregister(ctx context.Context, repository Repository, inUse func(context.Context, Repository) error) error {
	if repository == Official().Repository {
		return fmt.Errorf("catalog %s is built in and cannot be removed", repository)
	}
	guard, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer guard.Release()
	if _, err := s.registration(repository); err != nil {
		return err
	}
	if err := inUse(ctx, repository); err != nil {
		return err
	}
	directory := s.catalog(repository)
	if err := os.Remove(filepath.Join(directory, registrationFile)); err != nil {
		return fmt.Errorf("remove catalog %s registration: %w", repository, err)
	}
	if err := os.Remove(filepath.Join(directory, selectionFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove catalog %s selection: %w", repository, err)
	}
	return nil
}

// Registrations returns the official registration followed by every other
// registered catalog in repository order.
func (s *Store) Registrations() ([]Registration, error) {
	paths, err := filepath.Glob(filepath.Join(s.layout(), "*", "*", registrationFile))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	result := []Registration{Official()}
	for _, path := range paths {
		registration, err := readRegistration(path)
		if err != nil {
			return nil, err
		}
		if registration.Repository == Official().Repository || path != filepath.Join(s.catalog(registration.Repository), registrationFile) {
			return nil, fmt.Errorf("catalog registration %s does not match its location", path)
		}
		result = append(result, registration)
	}
	return result, nil
}

// Selected returns the selected retained snapshot of a registered catalog
// without using Source.
func (s *Store) Selected(repository Repository) (Snapshot, error) {
	if _, err := s.registration(repository); err != nil {
		return Snapshot{}, err
	}
	id, err := s.readSelection(repository)
	if err != nil {
		return Snapshot{}, s.earlierLayout(repository, err)
	}
	root := s.snapshot(repository, id)
	index, err := validateSnapshot(root, repository, id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("selected Catalog Snapshot %s of %s: %w", id, repository, err)
	}
	return snapshotFrom(index, "catalog-"+id, root), nil
}

// Resolve returns one retained snapshot root of a registered catalog without
// using Source.
func (s *Store) Resolve(repository Repository, snapshotID string) (string, error) {
	snapshot, err := s.retained(repository, snapshotID)
	return snapshot.Root, err
}

// Providing returns the Catalog Snapshot of a registered catalog whose index
// lists packID, reading each catalog's selected snapshot, or its retained
// snapshot snapshotID when one is given. When no catalog provides the Pack it
// returns the official catalog's snapshot, or the error from reading it, so the
// caller reports the unknown Pack against the official catalog.
func (s *Store) Providing(packID, snapshotID string) (Snapshot, error) {
	registrations, err := s.Registrations()
	if err != nil {
		return Snapshot{}, err
	}
	var official Snapshot
	var officialErr error
	for _, registration := range registrations {
		var snapshot Snapshot
		var err error
		if snapshotID == "" {
			snapshot, err = s.Selected(registration.Repository)
		} else {
			snapshot, err = s.retained(registration.Repository, snapshotID)
		}
		if registration.Repository == Official().Repository {
			official, officialErr = snapshot, err
		} else if err != nil && snapshotID == "" {
			return Snapshot{}, fmt.Errorf("catalog %s: %w", registration.Repository, err)
		}
		if err != nil {
			continue
		}
		for _, pack := range snapshot.Index.Packs {
			if pack.ID == packID {
				return snapshot, nil
			}
		}
	}
	return official, officialErr
}

func (s *Store) retained(repository Repository, snapshotID string) (Snapshot, error) {
	if !commitPattern.MatchString(snapshotID) {
		return Snapshot{}, fmt.Errorf("invalid Catalog Snapshot identity %q", snapshotID)
	}
	if _, err := s.registration(repository); err != nil {
		return Snapshot{}, err
	}
	root := s.snapshot(repository, snapshotID)
	index, err := validateSnapshot(root, repository, snapshotID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve Catalog Snapshot %s of %s: %w", snapshotID, repository, s.earlierLayout(repository, err))
	}
	return snapshotFrom(index, "catalog-"+snapshotID, root), nil
}

func (s *Store) layout() string { return filepath.Join(s.root, "v2") }

func (s *Store) catalog(repository Repository) string {
	return filepath.Join(s.layout(), repository.owner, repository.name)
}

func (s *Store) snapshot(repository Repository, id string) string {
	return filepath.Join(s.catalog(repository), "snapshots", id)
}

func (s *Store) lock(ctx context.Context) (*catalogtransaction.Guard, error) {
	if err := os.MkdirAll(s.layout(), 0o755); err != nil {
		return nil, fmt.Errorf("prepare catalog store: %w", err)
	}
	guard, err := catalogtransaction.Acquire(ctx, s.layout())
	if err != nil {
		return nil, fmt.Errorf("lock catalog store: %w", err)
	}
	return guard, nil
}

func (s *Store) registration(repository Repository) (Registration, error) {
	if repository == Official().Repository {
		return Official(), nil
	}
	if repository == (Repository{}) {
		return Registration{}, errors.New("catalog repository is required")
	}
	registration, err := readRegistration(filepath.Join(s.catalog(repository), registrationFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Registration{}, fmt.Errorf("catalog %s is not registered", repository)
	}
	if err != nil {
		return Registration{}, err
	}
	if registration.Repository != repository {
		return Registration{}, fmt.Errorf("catalog %s registration names %s", repository, registration.Repository)
	}
	return registration, nil
}

// earlierLayout explains a missing retained file when a catalog has no
// current selection and the store still holds the layout that predates
// catalog registrations.
func (s *Store) earlierLayout(repository Repository, err error) error {
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, statErr := os.Lstat(filepath.Join(s.root, selectionFile)); statErr != nil {
		return err
	}
	if _, statErr := os.Lstat(filepath.Join(s.catalog(repository), selectionFile)); !errors.Is(statErr, fs.ErrNotExist) {
		return err
	}
	return fmt.Errorf("%s: %w", s.root, ErrEarlierLayout)
}

// retainLatest validates the latest release of a catalog into its retained
// snapshots without selecting it. created reports a newly retained snapshot.
func (s *Store) retainLatest(ctx context.Context, registration Registration) (Snapshot, bool, error) {
	if s.source == nil {
		return Snapshot{}, false, errors.New("catalog Source is required")
	}
	repository := registration.Repository
	release, err := s.source.Latest(ctx, registration)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("acquire latest release of catalog %s: %w", repository, err)
	}
	assets, err := validateRelease(registration, release)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, false, err
	}
	snapshots := filepath.Join(s.catalog(repository), "snapshots")
	if err := os.MkdirAll(snapshots, 0o755); err != nil {
		return Snapshot{}, false, fmt.Errorf("prepare catalog %s snapshots: %w", repository, err)
	}
	stage, err := os.MkdirTemp(snapshots, ".staging-")
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("stage Catalog Snapshot: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := extractSnapshot(ctx, assets[archiveAsset].Data, stage); err != nil {
		return Snapshot{}, false, err
	}
	index, err := validateSnapshot(stage, repository, release.Commit)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := s.checkPackIDs(repository, index); err != nil {
		return Snapshot{}, false, err
	}
	destination := s.snapshot(repository, release.Commit)
	if _, err := os.Lstat(destination); err == nil {
		// Never replace retained immutable bytes; revalidate them before selection.
		if _, validationErr := validateSnapshot(destination, repository, release.Commit); validationErr != nil {
			return Snapshot{}, false, fmt.Errorf("retained Catalog Snapshot %s of %s conflicts with release: %w", release.Commit, repository, validationErr)
		}
		return snapshotFrom(index, release.Tag, destination), false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Snapshot{}, false, fmt.Errorf("inspect retained Catalog Snapshot: %w", err)
	}
	if err := os.Rename(stage, destination); err != nil {
		return Snapshot{}, false, fmt.Errorf("retain Catalog Snapshot: %w", err)
	}
	return snapshotFrom(index, release.Tag, destination), true, nil
}

// checkPackIDs rejects a snapshot of one catalog that introduces a Pack ID
// already present in the selected snapshot of another registered catalog.
func (s *Store) checkPackIDs(repository Repository, index cataloglayout.CatalogSnapshotIndex) error {
	registrations, err := s.Registrations()
	if err != nil {
		return err
	}
	introduced := make(map[string]bool, len(index.Packs))
	for _, pack := range index.Packs {
		introduced[pack.ID] = true
	}
	for _, other := range registrations {
		if other.Repository == repository {
			continue
		}
		id, err := s.readSelection(other.Repository)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check Pack IDs against catalog %s: %w", other.Repository, err)
		}
		selected, err := readIndex(s.snapshot(other.Repository, id))
		if err != nil {
			return fmt.Errorf("check Pack IDs against catalog %s: %w", other.Repository, err)
		}
		for _, pack := range selected.Packs {
			if !introduced[pack.ID] {
				continue
			}
			err := fmt.Errorf("Catalog Snapshot %s of %s introduces Pack %q, which selected Catalog Snapshot %s of %s already provides", index.Source.Commit, repository, pack.ID, id, other.Repository)
			if repository == Official().Repository {
				err = fmt.Errorf("%w; run `packy catalog remove %s` so the official catalog can be selected", err, other.Repository)
			}
			return err
		}
	}
	return nil
}

type selectionDocument struct {
	SchemaVersion int    `json:"schema_version"`
	Repository    string `json:"repository"`
	SnapshotID    string `json:"snapshot_id"`
}

func newSelection(repository Repository, id string) selectionDocument {
	return selectionDocument{SchemaVersion: 2, Repository: repository.String(), SnapshotID: id}
}

func (s *Store) readSelection(repository Repository) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.catalog(repository), selectionFile))
	if err != nil {
		return "", fmt.Errorf("read selected Catalog Snapshot of %s: %w", repository, err)
	}
	var selection selectionDocument
	if err := strictDecode(data, &selection); err != nil {
		return "", fmt.Errorf("decode selected Catalog Snapshot of %s: %w", repository, err)
	}
	if selection != newSelection(repository, selection.SnapshotID) || !commitPattern.MatchString(selection.SnapshotID) {
		return "", fmt.Errorf("selected Catalog Snapshot of %s has invalid identity", repository)
	}
	return selection.SnapshotID, nil
}

func readRegistration(path string) (Registration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Registration{}, fmt.Errorf("read catalog registration: %w", err)
	}
	var document registrationDocument
	if err := strictDecode(data, &document); err != nil {
		return Registration{}, fmt.Errorf("decode catalog registration %s: %w", path, err)
	}
	registration, err := decodeRegistration(document)
	if err != nil {
		return Registration{}, fmt.Errorf("decode catalog registration %s: %w", path, err)
	}
	return registration, nil
}

func validateRelease(registration Registration, release Release) (map[string]Asset, error) {
	repository := registration.Repository
	if release.Repository != repository.String() || release.Publisher != publisher {
		return nil, fmt.Errorf("catalog %s release does not have the expected repository and publisher metadata", repository)
	}
	if !release.Published || release.Draft || release.Prerelease || !release.Immutable {
		return nil, fmt.Errorf("catalog %s release must be published, stable, and immutable", repository)
	}
	if _, attested := registration.Trust.(Attested); attested && !release.AttestationVerified {
		return nil, fmt.Errorf("catalog %s artifact attestation is not verified", repository)
	}
	if !commitPattern.MatchString(release.Commit) || release.Tag != "catalog-"+release.Commit {
		return nil, errors.New("catalog release tag and source commit do not match")
	}
	if len(release.Assets) != 2 {
		return nil, errors.New("catalog release must contain exactly catalog-snapshot.tar.gz and SHA256SUMS")
	}
	assets := make(map[string]Asset, 2)
	for _, asset := range release.Assets {
		if asset.Name != archiveAsset && asset.Name != checksumAsset || assets[asset.Name].Name != "" {
			return nil, errors.New("catalog release must contain exactly catalog-snapshot.tar.gz and SHA256SUMS")
		}
		if len(asset.Data) > maxArchiveBytes {
			return nil, fmt.Errorf("catalog asset %q exceeds size limit", asset.Name)
		}
		apiDigest := strings.TrimPrefix(asset.SHA256, "sha256:")
		if !digestPattern.MatchString(apiDigest) || digest(asset.Data) != apiDigest {
			return nil, fmt.Errorf("catalog asset %q API SHA-256 does not match downloaded bytes", asset.Name)
		}
		assets[asset.Name] = asset
	}
	archiveDigest, err := parseChecksum(assets[checksumAsset].Data)
	if err != nil || archiveDigest != digest(assets[archiveAsset].Data) {
		return nil, errors.New("catalog archive checksum does not match downloaded bytes")
	}
	return assets, nil
}

func parseChecksum(data []byte) (string, error) {
	line := string(data)
	if !strings.HasSuffix(line, "\n") || strings.Count(line, "\n") != 1 {
		return "", errors.New("invalid checksum")
	}
	parts := strings.Split(strings.TrimSuffix(line, "\n"), "  ")
	if len(parts) != 2 || parts[1] != archiveAsset || !digestPattern.MatchString(parts[0]) {
		return "", errors.New("invalid checksum")
	}
	return parts[0], nil
}

func extractSnapshot(ctx context.Context, archive []byte, root string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open Catalog Snapshot archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(io.LimitReader(gz, maxExtractedBytes+1))
	seen := map[string]bool{}
	var total int64
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read Catalog Snapshot archive: %w", err)
		}
		if count >= maxFiles {
			return errors.New("Catalog Snapshot archive contains too many entries")
		}
		name := header.Name
		if !safeArchivePath(name) || seen[name] {
			return fmt.Errorf("Catalog Snapshot archive contains unsafe or duplicate path %q", name)
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("Catalog Snapshot archive entry %q is not a regular file", name)
		}
		if header.Size < 0 || header.Size > maxFileBytes || total+header.Size > maxExtractedBytes {
			return fmt.Errorf("Catalog Snapshot archive entry %q exceeds size limits", name)
		}
		total += header.Size
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("prepare Catalog Snapshot path: %w", err)
		}
		mode := os.FileMode(0o644)
		if header.Mode&0o111 != 0 {
			mode = 0o755
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return fmt.Errorf("create Catalog Snapshot entry %q: %w", name, err)
		}
		_, copyErr := io.CopyN(file, reader, header.Size)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("extract Catalog Snapshot entry %q: %w", name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close Catalog Snapshot entry %q: %w", name, closeErr)
		}
	}
	return nil
}

func safeArchivePath(name string) bool {
	if name == "" || strings.Contains(name, `\`) || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name {
		return false
	}
	return name == "catalog-index.json" || strings.HasPrefix(name, "packs/")
}

func validateSnapshot(root string, repository Repository, commit string) (cataloglayout.CatalogSnapshotIndex, error) {
	index, err := readIndex(root)
	if err != nil {
		return cataloglayout.CatalogSnapshotIndex{}, err
	}
	if index.SchemaVersion != 3 {
		return cataloglayout.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot index schema_version must be 3")
	}
	if indexed, err := ParseRepository(index.Source.Repository); err != nil || indexed != repository || index.Source.Commit != commit {
		return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot index names %s@%s, not %s@%s", index.Source.Repository, index.Source.Commit, repository, commit)
	}
	if !builderPattern.MatchString(index.Builder) || len(index.Packs) == 0 {
		return cataloglayout.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot index has invalid builder or empty catalog identity")
	}
	encodedPacks, err := json.Marshal(index.Packs)
	if err != nil || digest(encodedPacks) != index.CatalogSHA256 {
		return cataloglayout.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot catalog SHA-256 does not match Pack identities")
	}
	expected := map[string]cataloglayout.FileRecord{"catalog-index.json": {Path: "catalog-index.json"}}
	for i, pack := range index.Packs {
		if !idPattern.MatchString(pack.ID) || pack.Version == "" || i > 0 && index.Packs[i-1].ID >= pack.ID || len(pack.Files) == 0 {
			return cataloglayout.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot Pack identities must be valid, sorted, and unique")
		}
		if digestFileIndex(pack.Files) != pack.ClosureSHA256 {
			return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q closure SHA-256 does not match file identities", pack.ID)
		}
		manifestFound := false
		for j, file := range pack.Files {
			if !safeCatalogPath(file.Path) || !digestPattern.MatchString(file.SHA256) || file.Mode != "100644" && file.Mode != "100755" || j > 0 && pack.Files[j-1].Path >= file.Path {
				return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q has invalid file identities", pack.ID)
			}
			archivePath := "packs/" + pack.ID + "/" + file.Path
			if _, duplicate := expected[archivePath]; duplicate {
				return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot file %q has multiple owners", file.Path)
			}
			expected[archivePath] = file
			if file.Path == "pack.json" && file.SHA256 == pack.ManifestSHA256 {
				manifestFound = true
			}
		}
		if !manifestFound {
			return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q manifest identity does not match", pack.ID)
		}
	}
	actual, err := snapshotFiles(root)
	if err != nil {
		return cataloglayout.CatalogSnapshotIndex{}, err
	}
	if len(actual) != len(expected) {
		return cataloglayout.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot extracted files do not exactly match the index")
	}
	for name, record := range expected {
		if name == "catalog-index.json" {
			continue
		}
		fact, ok := actual[name]
		if !ok || fact.SHA256 != record.SHA256 || fact.Mode != record.Mode {
			return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot file %q does not match its indexed identity", name)
		}
	}
	runtimePacks := []capabilitypack.Pack{}
	for _, pack := range index.Packs {
		loaded, loadErr := capabilitypack.ValidatePackContent(root, pack.ID)
		if loadErr != nil {
			message := loadErr.Error()
			if strings.Contains(message, "unknown field") || strings.Contains(message, "unsupported") || strings.Contains(message, "schema_version") {
				return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q requires a newer Packy engine: %w", pack.ID, loadErr)
			}
			return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("validate Catalog Snapshot Pack %q: %w", pack.ID, loadErr)
		}
		runtimePacks = append(runtimePacks, loaded)
		if loaded.ID != pack.ID || loaded.Version != pack.Version {
			return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q manifest identity does not match index", pack.ID)
		}
	}
	if err := capabilitypack.ValidateOnDemandCatalog(runtimePacks); err != nil {
		return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("validate Catalog Snapshot on-demand contracts: %w", err)
	}
	return index, nil
}

type fileFact struct{ Mode, SHA256 string }

func snapshotFiles(root string) (map[string]fileFact, error) {
	result := map[string]fileFact{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("Catalog Snapshot contains non-regular file %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		mode := "100644"
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		result[filepath.ToSlash(rel)] = fileFact{Mode: mode, SHA256: digest(data)}
		return nil
	})
	return result, err
}

func safeCatalogPath(name string) bool {
	if name == "" || strings.Contains(name, `\`) || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || name == "." || strings.HasPrefix(name, "../") {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if strings.EqualFold(component, ".git") {
			return false
		}
	}
	return true
}

func readIndex(root string) (cataloglayout.CatalogSnapshotIndex, error) {
	data, err := os.ReadFile(filepath.Join(root, "catalog-index.json"))
	if err != nil {
		return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("read Catalog Snapshot index: %w", err)
	}
	var index cataloglayout.CatalogSnapshotIndex
	if err := strictDecode(data, &index); err != nil {
		return cataloglayout.CatalogSnapshotIndex{}, fmt.Errorf("decode Catalog Snapshot index: %w", err)
	}
	return index, nil
}

func writeDocument(directory, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, "."+name+"-")
	if err != nil {
		return err
	}
	staged := temporary.Name()
	defer os.Remove(staged)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(staged, filepath.Join(directory, name))
}

func strictDecode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func snapshotFrom(index cataloglayout.CatalogSnapshotIndex, tag, root string) Snapshot {
	return Snapshot{ID: index.Source.Commit, Commit: index.Source.Commit, Tag: tag, Root: root, Index: index}
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func digestFileIndex(files []cataloglayout.FileRecord) string {
	copyFiles := append([]cataloglayout.FileRecord(nil), files...)
	if !sort.SliceIsSorted(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path }) {
		return ""
	}
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", file.Path, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Package catalogstore owns acquisition, validation, retention, and selection
// of immutable official Catalog Snapshots.
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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/bundletransaction"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

const (
	officialRepository = "yersonargotev/packy-catalog"
	officialPublisher  = "github-actions[bot]"
	archiveAsset       = "catalog-snapshot.tar.gz"
	checksumAsset      = "SHA256SUMS"
	maxArchiveBytes    = 96 << 20
	maxExtractedBytes  = 64 << 20
	maxFileBytes       = 8 << 20
	maxFiles           = 2048
)

var (
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	idPattern      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	builderPattern = regexp.MustCompile(`^yersonargotev/packy@[0-9a-f]{40}$`)
)

// Source is the network boundary used only by explicit acquisition.
type Source interface {
	Latest(context.Context, string) (Release, error)
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
	ID         string
	Commit     string
	Tag        string
	BundleRoot string
	Index      managedpack.CatalogSnapshotIndex
}

// SkillRoot returns the selected snapshot's reviewed skill source.
func (s Snapshot) SkillRoot() string { return filepath.Join(s.BundleRoot, "skills") }

// Store is a filesystem-backed official Catalog Snapshot store.
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

// Root returns the catalog store root.
func (s *Store) Root() string { return s.root }

// AcquireLatest downloads, fully validates, retains, and finally selects the
// latest official immutable snapshot.
func (s *Store) AcquireLatest(ctx context.Context) (Snapshot, error) {
	if s.source == nil {
		return Snapshot{}, errors.New("catalog Source is required")
	}
	if err := os.MkdirAll(filepath.Join(s.root, "snapshots"), 0o755); err != nil {
		return Snapshot{}, fmt.Errorf("prepare catalog store: %w", err)
	}
	guard, err := bundletransaction.Acquire(ctx, s.root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("lock catalog store: %w", err)
	}
	defer guard.Release()
	release, err := s.source.Latest(ctx, officialRepository)
	if err != nil {
		return Snapshot{}, fmt.Errorf("acquire latest official catalog release: %w", err)
	}
	assets, err := validateRelease(release)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	stage, err := os.MkdirTemp(filepath.Join(s.root, "snapshots"), ".staging-")
	if err != nil {
		return Snapshot{}, fmt.Errorf("stage Catalog Snapshot: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := extractSnapshot(ctx, assets[archiveAsset].Data, stage); err != nil {
		return Snapshot{}, err
	}
	index, err := validateSnapshot(stage, release)
	if err != nil {
		return Snapshot{}, err
	}
	destination := filepath.Join(s.root, "snapshots", release.Commit)
	if _, err := os.Stat(destination); err == nil {
		// Never replace retained immutable bytes; revalidate them before selection.
		if _, validationErr := validateSnapshot(destination, release); validationErr != nil {
			return Snapshot{}, fmt.Errorf("retained Catalog Snapshot conflicts with release: %w", validationErr)
		}
	} else if !os.IsNotExist(err) {
		return Snapshot{}, fmt.Errorf("inspect retained Catalog Snapshot: %w", err)
	} else {
		if err := os.Rename(stage, destination); err != nil {
			return Snapshot{}, fmt.Errorf("retain Catalog Snapshot: %w", err)
		}
		keep = true
	}
	if err := writeSelection(s.root, release.Commit); err != nil {
		return Snapshot{}, err
	}
	return snapshotFrom(index, release.Tag, destination), nil
}

// Selected returns the selected retained snapshot without using Source.
func (s *Store) Selected() (Snapshot, error) {
	data, err := os.ReadFile(filepath.Join(s.root, "selected.json"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read selected Catalog Snapshot: %w", err)
	}
	var selection selectionDocument
	if err := strictDecode(data, &selection); err != nil {
		return Snapshot{}, fmt.Errorf("decode selected Catalog Snapshot: %w", err)
	}
	if selection.SchemaVersion != 1 || !commitPattern.MatchString(selection.SnapshotID) {
		return Snapshot{}, errors.New("selected Catalog Snapshot has invalid identity")
	}
	root := filepath.Join(s.root, "snapshots", selection.SnapshotID)
	release := Release{Repository: officialRepository, Tag: "catalog-" + selection.SnapshotID, Commit: selection.SnapshotID}
	index, err := validateSnapshot(root, release)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFrom(index, "catalog-"+selection.SnapshotID, root), nil
}

// Resolve returns one retained snapshot's bundle root without using Source.
func (s *Store) Resolve(snapshotID string) (string, error) {
	if !commitPattern.MatchString(snapshotID) {
		return "", fmt.Errorf("invalid Catalog Snapshot identity %q", snapshotID)
	}
	root := filepath.Join(s.root, "snapshots", snapshotID)
	release := Release{Repository: officialRepository, Tag: "catalog-" + snapshotID, Commit: snapshotID}
	index, err := validateSnapshot(root, release)
	if err != nil {
		return "", fmt.Errorf("resolve Catalog Snapshot %s: %w", snapshotID, err)
	}
	if index.Source.Repository != officialRepository || index.Source.Commit != snapshotID {
		return "", fmt.Errorf("resolve Catalog Snapshot %s: index identity does not match", snapshotID)
	}
	return filepath.Join(root, "bundle"), nil
}

type selectionDocument struct {
	SchemaVersion int    `json:"schema_version"`
	SnapshotID    string `json:"snapshot_id"`
}

func validateRelease(release Release) (map[string]Asset, error) {
	if release.Repository != officialRepository || release.Publisher != officialPublisher {
		return nil, errors.New("catalog release does not have the official publisher metadata")
	}
	if !release.Published || release.Draft || release.Prerelease || !release.Immutable {
		return nil, errors.New("official catalog release must be published, stable, and immutable")
	}
	if !release.AttestationVerified {
		return nil, errors.New("official catalog artifact attestation is not verified")
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
	return name == "catalog-index.json" || strings.HasPrefix(name, "bundle/")
}

func validateSnapshot(root string, release Release) (managedpack.CatalogSnapshotIndex, error) {
	index, err := readIndex(root)
	if err != nil {
		return managedpack.CatalogSnapshotIndex{}, err
	}
	if index.SchemaVersion != 1 {
		return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot index schema_version must be 1; install a newer Packy engine if this snapshot uses a newer schema")
	}
	if index.Source.Repository != officialRepository || index.Source.Commit != release.Commit || release.Tag != "catalog-"+index.Source.Commit {
		return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot index source, release tag, and commit identities do not match")
	}
	if !builderPattern.MatchString(index.Builder) || len(index.Packs) == 0 {
		return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot index has invalid builder or empty catalog identity")
	}
	encodedPacks, err := json.Marshal(index.Packs)
	if err != nil || digest(encodedPacks) != index.CatalogSHA256 {
		return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot catalog SHA-256 does not match Pack identities")
	}
	expected := map[string]managedpack.FileRecord{"catalog-index.json": {Path: "catalog-index.json"}}
	for i, pack := range index.Packs {
		if !idPattern.MatchString(pack.ID) || pack.Version == "" || i > 0 && index.Packs[i-1].ID >= pack.ID || len(pack.Files) == 0 {
			return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot Pack identities must be valid, sorted, and unique")
		}
		if digestFileIndex(pack.Files) != pack.ClosureSHA256 {
			return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q closure SHA-256 does not match file identities", pack.ID)
		}
		manifest := "packs/" + pack.ID + "/pack.json"
		manifestFound := false
		for j, file := range pack.Files {
			if !safeBundlePath(file.Path) || !digestPattern.MatchString(file.SHA256) || file.Mode != "100644" && file.Mode != "100755" || j > 0 && pack.Files[j-1].Path >= file.Path {
				return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q has invalid file identities", pack.ID)
			}
			archivePath := "bundle/" + file.Path
			if _, duplicate := expected[archivePath]; duplicate {
				return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot file %q has multiple owners", file.Path)
			}
			expected[archivePath] = file
			if file.Path == manifest && file.SHA256 == pack.ManifestSHA256 {
				manifestFound = true
			}
		}
		if !manifestFound {
			return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q manifest identity does not match", pack.ID)
		}
	}
	actual, err := snapshotFiles(root)
	if err != nil {
		return managedpack.CatalogSnapshotIndex{}, err
	}
	if len(actual) != len(expected) {
		return managedpack.CatalogSnapshotIndex{}, errors.New("Catalog Snapshot extracted files do not exactly match the index")
	}
	for name, record := range expected {
		if name == "catalog-index.json" {
			continue
		}
		fact, ok := actual[name]
		if !ok || fact.SHA256 != record.SHA256 || fact.Mode != record.Mode {
			return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot file %q does not match its indexed identity", name)
		}
	}
	bundle := filepath.Join(root, "bundle")
	for _, pack := range index.Packs {
		loaded, loadErr := capabilitypack.ValidatePackContent(bundle, pack.ID)
		if loadErr != nil {
			message := loadErr.Error()
			if strings.Contains(message, "unknown field") || strings.Contains(message, "unsupported") || strings.Contains(message, "schema_version") {
				return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q requires a newer Packy engine: %w", pack.ID, loadErr)
			}
			return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("validate Catalog Snapshot Pack %q: %w", pack.ID, loadErr)
		}
		if loaded.ID != pack.ID || loaded.Version != pack.Version {
			return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("Catalog Snapshot Pack %q manifest identity does not match index", pack.ID)
		}
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

func safeBundlePath(name string) bool {
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

func readIndex(root string) (managedpack.CatalogSnapshotIndex, error) {
	data, err := os.ReadFile(filepath.Join(root, "catalog-index.json"))
	if err != nil {
		return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("read Catalog Snapshot index: %w", err)
	}
	var index managedpack.CatalogSnapshotIndex
	if err := strictDecode(data, &index); err != nil {
		return managedpack.CatalogSnapshotIndex{}, fmt.Errorf("decode Catalog Snapshot index: %w", err)
	}
	return index, nil
}

func writeSelection(root, id string) error {
	data, err := json.MarshalIndent(selectionDocument{SchemaVersion: 1, SnapshotID: id}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(root, ".selected-")
	if err != nil {
		return fmt.Errorf("stage Catalog Snapshot selection: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
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
	if err := os.Rename(name, filepath.Join(root, "selected.json")); err != nil {
		return fmt.Errorf("select Catalog Snapshot: %w", err)
	}
	return nil
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

func snapshotFrom(index managedpack.CatalogSnapshotIndex, tag, root string) Snapshot {
	return Snapshot{ID: index.Source.Commit, Commit: index.Source.Commit, Tag: tag, BundleRoot: filepath.Join(root, "bundle"), Index: index}
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func digestFileIndex(files []managedpack.FileRecord) string {
	copyFiles := append([]managedpack.FileRecord(nil), files...)
	if !sort.SliceIsSorted(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path }) {
		return ""
	}
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", file.Path, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

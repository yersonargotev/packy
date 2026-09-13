package managedpack

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	catalogSnapshotArchive  = "catalog-snapshot.tar.gz"
	catalogSnapshotChecksum = "SHA256SUMS"
)

var (
	fullCommitPattern        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	catalogRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	builderIdentityPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)
)

// CatalogSnapshotSource binds a snapshot to its reviewed source and exact
// builder revision.
type CatalogSnapshotSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Builder    string `json:"-"`
}

// CatalogSnapshotPack identifies one exact Pack closure in a Catalog Snapshot.
type CatalogSnapshotPack struct {
	ID             string       `json:"id"`
	Version        string       `json:"version"`
	ManifestSHA256 string       `json:"manifest_sha256"`
	ClosureSHA256  string       `json:"closure_sha256"`
	Files          []FileRecord `json:"files"`
}

// CatalogSnapshotIndex is the deterministic identity and content index stored
// inside every complete Catalog Snapshot.
type CatalogSnapshotIndex struct {
	SchemaVersion int                   `json:"schema_version"`
	Source        CatalogSnapshotSource `json:"source"`
	Builder       string                `json:"builder"`
	CatalogSHA256 string                `json:"catalog_sha256"`
	Packs         []CatalogSnapshotPack `json:"packs"`
}

// CatalogSnapshotResult identifies the immutable files emitted by a snapshot
// build. The archive digest plus the source commit form its publication identity.
type CatalogSnapshotResult struct {
	Index         CatalogSnapshotIndex
	ArchivePath   string
	ChecksumPath  string
	ArchiveSHA256 string
}

// BuildCatalogSnapshot packages only the exact files sealed by a complete
// Catalog Project validation. It refuses source drift and emits deterministic
// archive bytes suitable for immutable publication.
func BuildCatalogSnapshot(ctx context.Context, projectRoot string, validation CatalogValidation, source CatalogSnapshotSource, outputDir string) (result CatalogSnapshotResult, resultErr error) {
	if err := validateCatalogSnapshotInputs(projectRoot, validation, source, outputDir); err != nil {
		return CatalogSnapshotResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return CatalogSnapshotResult{}, err
	}

	index := CatalogSnapshotIndex{
		SchemaVersion: 1,
		Source: CatalogSnapshotSource{
			Repository: source.Repository,
			Commit:     source.Commit,
		},
		Builder: source.Builder,
		Packs:   make([]CatalogSnapshotPack, 0, len(validation.Packs)),
	}
	for _, pack := range validation.Packs {
		index.Packs = append(index.Packs, CatalogSnapshotPack{
			ID:             pack.Manifest.ID,
			Version:        pack.Manifest.Version,
			ManifestSHA256: pack.ManifestSHA256,
			ClosureSHA256:  pack.ClosureSHA256,
			Files:          append([]FileRecord(nil), pack.Files...),
		})
	}
	identity, err := json.Marshal(index.Packs)
	if err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("encode Catalog Snapshot identity: %w", err)
	}
	index.CatalogSHA256 = digestBytes(identity)
	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("encode Catalog Snapshot index: %w", err)
	}
	indexData = append(indexData, '\n')

	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("prepare Catalog Snapshot output parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, ".packy-catalog-snapshot-")
	if err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("create Catalog Snapshot staging directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(temporary)
		}
	}()

	archivePath := filepath.Join(temporary, catalogSnapshotArchive)
	archive, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("create Catalog Snapshot archive: %w", err)
	}
	hasher := sha256.New()
	multi := io.MultiWriter(archive, hasher)
	gzipWriter := gzip.NewWriter(multi)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	closeArchive := func() error {
		if err := tarWriter.Close(); err != nil {
			return err
		}
		if err := gzipWriter.Close(); err != nil {
			return err
		}
		return archive.Close()
	}

	if err := writeSnapshotBytes(tarWriter, "catalog-index.json", "100644", indexData); err != nil {
		_ = closeArchive()
		return CatalogSnapshotResult{}, err
	}
	entries := catalogSnapshotFiles(validation)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			_ = closeArchive()
			return CatalogSnapshotResult{}, err
		}
		path := filepath.Join(projectRoot, "bundle", filepath.FromSlash(entry.Path))
		bundleRoot := filepath.Join(projectRoot, "bundle")
		if err := rejectSymlinkComponents(ctx, bundleRoot, entry.Path); err != nil {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("read validated Catalog Snapshot file %q: %w", entry.Path, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("read validated Catalog Snapshot file %q: %w", entry.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("read validated Catalog Snapshot file %q: source is a symlink or is not regular", entry.Path)
		}
		if canonicalMode(info.Mode()) != entry.Mode {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("Catalog Snapshot source %q drifted from validated mode", entry.Path)
		}
		data, err := readFileBounded(ctx, path, info)
		if err != nil {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("read validated Catalog Snapshot file %q: %w", entry.Path, err)
		}
		if digestBytes(data) != entry.SHA256 {
			_ = closeArchive()
			return CatalogSnapshotResult{}, fmt.Errorf("Catalog Snapshot source %q drifted from validated SHA-256", entry.Path)
		}
		if err := writeSnapshotBytes(tarWriter, "bundle/"+entry.Path, entry.Mode, data); err != nil {
			_ = closeArchive()
			return CatalogSnapshotResult{}, err
		}
	}
	if err := closeArchive(); err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("close Catalog Snapshot archive: %w", err)
	}

	archiveSHA256 := hex.EncodeToString(hasher.Sum(nil))
	checksumPath := filepath.Join(temporary, catalogSnapshotChecksum)
	checksum := []byte(archiveSHA256 + "  " + catalogSnapshotArchive + "\n")
	if err := os.WriteFile(checksumPath, checksum, 0o644); err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("write Catalog Snapshot checksum: %w", err)
	}
	if err := os.Rename(temporary, outputDir); err != nil {
		return CatalogSnapshotResult{}, fmt.Errorf("publish Catalog Snapshot output: %w", err)
	}
	keep = true
	return CatalogSnapshotResult{
		Index:         index,
		ArchivePath:   filepath.Join(outputDir, catalogSnapshotArchive),
		ChecksumPath:  filepath.Join(outputDir, catalogSnapshotChecksum),
		ArchiveSHA256: archiveSHA256,
	}, nil
}

func catalogSnapshotFiles(validation CatalogValidation) []FileRecord {
	entries := make([]FileRecord, 0)
	for _, pack := range validation.Packs {
		for _, file := range pack.Files {
			entries = append(entries, file)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func validateCatalogSnapshotInputs(projectRoot string, validation CatalogValidation, source CatalogSnapshotSource, outputDir string) error {
	if strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("Catalog Snapshot project and output paths are required")
	}
	if !catalogRepositoryPattern.MatchString(source.Repository) || !fullCommitPattern.MatchString(source.Commit) || !builderIdentityPattern.MatchString(source.Builder) {
		return fmt.Errorf("Catalog Snapshot source and builder must name repositories and full commits")
	}
	if len(validation.Packs) == 0 {
		return fmt.Errorf("Catalog Snapshot requires a complete non-empty Catalog Project validation")
	}
	ownedPaths := map[string]bool{}
	for packIndex, pack := range validation.Packs {
		if packIndex > 0 && validation.Packs[packIndex-1].Manifest.ID >= pack.Manifest.ID {
			return fmt.Errorf("Catalog Snapshot validation Packs must be sorted by identity without duplicates")
		}
		if !idPattern.MatchString(pack.Manifest.ID) || len(pack.Files) == 0 || !validSHA256(pack.ManifestSHA256) || digestIndex(pack.Files) != pack.ClosureSHA256 {
			return fmt.Errorf("Catalog Snapshot validation for Pack %q is not sealed", pack.Manifest.ID)
		}
		manifestPath := filepath.ToSlash(filepath.Join("packs", pack.Manifest.ID, "pack.json"))
		manifestFound := false
		for fileIndex, file := range pack.Files {
			if err := validateRelativePath(file.Path, false); err != nil || hasPathComponent(file.Path, ".git") || pathDepth(file.Path) > maxIndexedPathDepth {
				return fmt.Errorf("Catalog Snapshot validation file %q is not allowed", file.Path)
			}
			if fileIndex > 0 && pack.Files[fileIndex-1].Path >= file.Path {
				return fmt.Errorf("Catalog Snapshot validation files for Pack %q must be sorted without duplicates", pack.Manifest.ID)
			}
			if (file.Mode != "100644" && file.Mode != "100755") || !validSHA256(file.SHA256) {
				return fmt.Errorf("Catalog Snapshot validation file %q has invalid mode or digest", file.Path)
			}
			if ownedPaths[file.Path] {
				return fmt.Errorf("Catalog Snapshot validation file %q has multiple owners", file.Path)
			}
			ownedPaths[file.Path] = true
			if file.Path == manifestPath {
				manifestFound = file.SHA256 == pack.ManifestSHA256
			}
		}
		if !manifestFound {
			return fmt.Errorf("Catalog Snapshot validation for Pack %q does not seal its manifest", pack.Manifest.ID)
		}
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		if err != nil {
			return fmt.Errorf("inspect Catalog Snapshot output: %w", err)
		}
		return fmt.Errorf("Catalog Snapshot output already exists: %s", outputDir)
	}
	return nil
}

func writeSnapshotBytes(writer *tar.Writer, name, mode string, data []byte) error {
	fileMode := int64(0o644)
	if mode == "100755" {
		fileMode = 0o755
	}
	header := &tar.Header{
		Name:    name,
		Mode:    fileMode,
		Size:    int64(len(data)),
		ModTime: time.Unix(0, 0),
		Format:  tar.FormatPAX,
	}
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write Catalog Snapshot entry %q: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write Catalog Snapshot entry %q: %w", name, err)
	}
	return nil
}

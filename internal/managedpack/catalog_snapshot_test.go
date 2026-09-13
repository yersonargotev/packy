package managedpack

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestBuildCatalogSnapshotIdentifiesAndPackagesExactValidatedContent(t *testing.T) {
	project := t.TempDir()
	writeCatalogPack(t, project, "alpha", "1.2.0", "alpha guidance\n", "skills/alpha")
	writeCatalogPack(t, project, "beta", "2.0.1", "beta guidance\n", "skills/beta")
	validation, err := ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(t.TempDir(), "dist")
	result, err := BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     strings.Repeat("a", 40),
		Builder:    "yersonargotev/packy@" + strings.Repeat("b", 40),
	}, first)
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "dist")
	secondResult, err := BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     strings.Repeat("a", 40),
		Builder:    "yersonargotev/packy@" + strings.Repeat("b", 40),
	}, second)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(result.Index, secondResult.Index) || result.ArchiveSHA256 != secondResult.ArchiveSHA256 {
		t.Fatalf("snapshot identities differ:\nfirst:  %#v\nsecond: %#v", result, secondResult)
	}
	firstArchive, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	secondArchive, err := os.ReadFile(secondResult.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstArchive, secondArchive) {
		t.Fatal("snapshot archive bytes are not deterministic")
	}
	archiveDigest := sha256.Sum256(firstArchive)
	if got := hex.EncodeToString(archiveDigest[:]); got != result.ArchiveSHA256 {
		t.Fatalf("archive SHA-256 = %s, want %s", result.ArchiveSHA256, got)
	}
	checksum, err := os.ReadFile(result.ChecksumPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(checksum), result.ArchiveSHA256+"  catalog-snapshot.tar.gz\n"; got != want {
		t.Fatalf("checksum = %q, want %q", got, want)
	}

	files := readSnapshotArchive(t, firstArchive)
	wantNames := []string{
		"bundle/packs/alpha/pack.json",
		"bundle/packs/beta/pack.json",
		"bundle/skills/alpha/SKILL.md",
		"bundle/skills/beta/SKILL.md",
		"catalog-index.json",
	}
	gotNames := make([]string, 0, len(files))
	for name := range files {
		gotNames = append(gotNames, name)
	}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("archive entries = %v, want %v", gotNames, wantNames)
	}

	var index CatalogSnapshotIndex
	if err := json.Unmarshal(files["catalog-index.json"], &index); err != nil {
		t.Fatal(err)
	}
	if index.SchemaVersion != 1 || index.Source.Repository != "yersonargotev/packy-catalog" || index.Source.Commit != strings.Repeat("a", 40) {
		t.Fatalf("index identity = %#v", index)
	}
	if index.Builder != "yersonargotev/packy@"+strings.Repeat("b", 40) || len(index.Packs) != 2 {
		t.Fatalf("index builder/Packs = %#v", index)
	}
	if index.Packs[0].ID != "alpha" || index.Packs[0].Version != "1.2.0" || index.Packs[1].ID != "beta" || index.Packs[1].Version != "2.0.1" {
		t.Fatalf("indexed Pack identities = %#v", index.Packs)
	}
	if index.CatalogSHA256 == "" || index.Packs[0].ManifestSHA256 == "" || index.Packs[0].ClosureSHA256 == "" || len(index.Packs[0].Files) != 2 {
		t.Fatalf("index digests/files = %#v", index)
	}
}

func TestBuildCatalogSnapshotRejectsInvalidIdentityAndValidatedSourceDrift(t *testing.T) {
	project := t.TempDir()
	writeCatalogPack(t, project, "alpha", "1.0.0", "original\n", "skills/alpha")
	validation, err := ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     "main",
		Builder:    "yersonargotev/packy@main",
	}, filepath.Join(t.TempDir(), "dist"))
	if err == nil || !strings.Contains(err.Error(), "full commit") {
		t.Fatalf("invalid identity error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(project, "bundle", "skills", "alpha", "SKILL.md"), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     strings.Repeat("a", 40),
		Builder:    "yersonargotev/packy@" + strings.Repeat("b", 40),
	}, filepath.Join(t.TempDir(), "dist"))
	if err == nil || !strings.Contains(err.Error(), "drifted from validated SHA-256") {
		t.Fatalf("source drift error = %v", err)
	}
}

func TestBuildCatalogSnapshotRejectsAnUnsealedValidation(t *testing.T) {
	project := t.TempDir()
	writeCatalogPack(t, project, "alpha", "1.0.0", "original\n", "skills/alpha")
	validation, err := ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	validation.Packs[0].Files[0].Path = "../outside"

	_, err = BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     strings.Repeat("a", 40),
		Builder:    "yersonargotev/packy@" + strings.Repeat("b", 40),
	}, filepath.Join(t.TempDir(), "dist"))
	if err == nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("unsealed validation error = %v", err)
	}
}

func TestBuildCatalogSnapshotRejectsAValidatedFileReplacedBySymlink(t *testing.T) {
	project := t.TempDir()
	writeCatalogPack(t, project, "alpha", "1.0.0", "original\n", "skills/alpha")
	validation, err := ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, "bundle", "skills", "alpha", "SKILL.md")
	copy := filepath.Join(project, "same-bytes")
	if err := os.WriteFile(copy, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(copy, target); err != nil {
		t.Fatal(err)
	}

	_, err = BuildCatalogSnapshot(context.Background(), project, validation, CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     strings.Repeat("a", 40),
		Builder:    "yersonargotev/packy@" + strings.Repeat("b", 40),
	}, filepath.Join(t.TempDir(), "dist"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink replacement error = %v", err)
	}
}

func readSnapshotArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gzipReader, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	result := map[string][]byte{}
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !header.ModTime.Equal(time.Unix(0, 0)) {
			t.Fatalf("entry %s modtime = %s", header.Name, header.ModTime)
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		result[header.Name] = contents
	}
	return result
}

package vercelacceptance

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

const ExactArchiveSHA256 = "6914589e3899ae238c30a0d87c297ef101c87a01d63e160efc3dcfab27676ab7"

//go:embed testdata/vercel-1.0.0.tar.gz
var exactArchive []byte

// SnapshotFile is one inert selected file in the exact portable fixture.
type SnapshotFile struct {
	Path    string
	Mode    int64
	Size    int64
	SHA256  string
	Content []byte
}

func ExactArchive() []byte { return append([]byte(nil), exactArchive...) }

// InspectExactArchive statically reads the selected fixture. It rejects links,
// devices, unsafe paths, duplicate entries, and oversized content.
func InspectExactArchive() ([]SnapshotFile, error) {
	sum := sha256.Sum256(exactArchive)
	if hex.EncodeToString(sum[:]) != ExactArchiveSHA256 {
		return nil, errors.New("exact Vercel fixture archive digest changed")
	}
	zr, err := gzip.NewReader(bytes.NewReader(exactArchive))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	tr := tar.NewReader(io.LimitReader(zr, 16<<20))
	files := []SnapshotFile{}
	seen := map[string]bool{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("unsafe fixture path %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 4<<20 {
			return nil, fmt.Errorf("unsafe fixture entry %q", header.Name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate fixture path %q", name)
		}
		seen[name] = true
		content, err := io.ReadAll(io.LimitReader(tr, header.Size+1))
		if err != nil {
			return nil, err
		}
		if int64(len(content)) != header.Size {
			return nil, fmt.Errorf("fixture size changed for %q", name)
		}
		fileSum := sha256.Sum256(content)
		files = append(files, SnapshotFile{
			Path: name, Mode: header.Mode, Size: header.Size,
			SHA256: hex.EncodeToString(fileSum[:]), Content: content,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func SnapshotFileByPath(name string) (SnapshotFile, error) {
	files, err := InspectExactArchive()
	if err != nil {
		return SnapshotFile{}, err
	}
	for _, file := range files {
		if file.Path == name {
			file.Content = append([]byte(nil), file.Content...)
			return file, nil
		}
	}
	return SnapshotFile{}, fmt.Errorf("fixture file %q is absent", name)
}

package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UnpackCandidate validates local snapshot assets into a new directory. It
// establishes integrity and engine compatibility, never publisher authority.
// It neither acquires nor selects content in the official Catalog Store.
func UnpackCandidate(ctx context.Context, assetsRoot, destination string) (Snapshot, string, error) {
	archive, err := readCandidateAsset(filepath.Join(assetsRoot, archiveAsset), maxArchiveBytes)
	if err != nil {
		return Snapshot{}, "", err
	}
	checksum, err := readCandidateAsset(filepath.Join(assetsRoot, checksumAsset), 256)
	if err != nil {
		return Snapshot{}, "", err
	}
	want, err := parseChecksum(checksum)
	if err != nil || digest(archive) != want {
		return Snapshot{}, "", errors.New("local candidate archive checksum does not match")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return Snapshot{}, "", fmt.Errorf("create local candidate directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(destination)
		}
	}()
	if err := extractSnapshot(ctx, archive, destination); err != nil {
		return Snapshot{}, "", err
	}
	snapshot, err := InspectCandidate(destination)
	if err != nil {
		return Snapshot{}, "", err
	}
	keep = true
	return snapshot, want, nil
}

// InspectCandidate revalidates all retained local bytes without claiming an
// official publication or accepting a different catalog source.
func InspectCandidate(root string) (Snapshot, error) {
	index, err := readIndex(root)
	if err != nil {
		return Snapshot{}, err
	}
	if !commitPattern.MatchString(index.Source.Commit) {
		return Snapshot{}, errors.New("local candidate has an invalid source commit")
	}
	index, err = validateSnapshot(root, Release{Commit: index.Source.Commit, Tag: "catalog-" + index.Source.Commit})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFrom(index, "local-candidate", root), nil
}

func readCandidateAsset(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect local candidate asset: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("local candidate asset %q must be a bounded regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read local candidate asset: %w", err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("local candidate asset %q must be a bounded regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("read bounded local candidate asset %q", path)
	}
	return data, nil
}

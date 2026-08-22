package repositorycandidate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

type bundleOwnership struct {
	owners map[string]map[string]bool
	roots  map[string]map[string]bool
}

func inspectOwnership(ctx context.Context, bundleRoot string) (bundleOwnership, error) {
	entries, err := os.ReadDir(filepath.Join(bundleRoot, "packs"))
	if err != nil {
		return bundleOwnership{}, fmt.Errorf("read Pack catalog ownership: %w", err)
	}
	result := bundleOwnership{owners: map[string]map[string]bool{}, roots: map[string]map[string]bool{}}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return bundleOwnership{}, err
		}
		if !entry.IsDir() {
			return bundleOwnership{}, fmt.Errorf("unexpected Pack catalog entry %q", entry.Name())
		}
		pack, err := capabilitypack.ValidatePackContent(bundleRoot, entry.Name())
		if err != nil {
			return bundleOwnership{}, fmt.Errorf("establish ownership for Pack %q: %w", entry.Name(), err)
		}
		roots := make(map[string]bool)
		for _, resource := range pack.Resources {
			if resource.Source != "" {
				roots[resource.Source] = true
			}
			for _, binding := range resource.Bindings {
				for _, source := range binding.ReferencedSourcePaths() {
					roots[source] = true
				}
			}
		}
		ordered := make([]string, 0, len(roots))
		for root := range roots {
			ordered = append(ordered, root)
		}
		sort.Strings(ordered)
		for _, root := range ordered {
			if result.roots[root] == nil {
				result.roots[root] = map[string]bool{}
			}
			result.roots[root][entry.Name()] = true
			paths, err := regularPaths(ctx, bundleRoot, root)
			if err != nil {
				return bundleOwnership{}, fmt.Errorf("establish ownership for Pack %q root %q: %w", entry.Name(), root, err)
			}
			for _, path := range paths {
				if result.owners[path] == nil {
					result.owners[path] = map[string]bool{}
				}
				result.owners[path][entry.Name()] = true
			}
		}
	}
	return result, nil
}

func regularPaths(ctx context.Context, bundleRoot, relativeRoot string) ([]string, error) {
	root := filepath.Join(bundleRoot, filepath.FromSlash(relativeRoot))
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		relative, err := filepath.Rel(bundleRoot, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	return paths, err
}

func materializeCandidate(ctx context.Context, repositoryRoot, projectRoot string, validation managedpack.Validation, ownership bundleOwnership) (map[string]bool, error) {
	bundleRoot := filepath.Join(repositoryRoot, "bundle")
	target := validation.Manifest.ID
	candidateFiles := map[string]managedpack.FileRecord{}
	for _, record := range validation.Files {
		if record.Path != "pack.json" {
			candidateFiles[record.Path] = record
		}
	}
	if err := validateCandidateRoots(validation, ownership, candidateFiles); err != nil {
		return nil, err
	}

	for path, record := range candidateFiles {
		owners := ownership.owners[path]
		state, err := inspectFile(filepath.Join(bundleRoot, filepath.FromSlash(path)))
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, managedpackpromotion.Reject(managedpackpromotion.GateOwnership, fmt.Sprintf("inspect candidate path %q: %v", path, err))
		}
		otherOwner := false
		for packID := range owners {
			if packID != target {
				otherOwner = true
			}
		}
		if otherOwner && (!exists || state.mode != record.Mode || state.sha256 != record.SHA256) {
			return nil, managedpackpromotion.Reject(managedpackpromotion.GateOwnership, fmt.Sprintf("candidate path %q would drift another Pack's contribution", path))
		}
		if exists && len(owners) == 0 {
			return nil, managedpackpromotion.Reject(managedpackpromotion.GateOwnership, fmt.Sprintf("candidate path %q collides with content not owned by a Pack", path))
		}
	}

	changed := map[string]bool{}
	for path, owners := range ownership.owners {
		if !owners[target] {
			continue
		}
		if _, retained := candidateFiles[path]; retained {
			continue
		}
		shared := false
		for packID := range owners {
			if packID != target {
				shared = true
			}
		}
		if shared {
			continue
		}
		if err := os.Remove(filepath.Join(bundleRoot, filepath.FromSlash(path))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("retire former target path %q: %w", path, err)
		}
		changed[path] = true
		removeEmptyParents(filepath.Dir(filepath.Join(bundleRoot, filepath.FromSlash(path))), bundleRoot)
	}

	stage, err := os.MkdirTemp(filepath.Dir(repositoryRoot), "packy-materialized-*")
	if err != nil {
		return nil, fmt.Errorf("create materialization stage: %w", err)
	}
	defer os.RemoveAll(stage)
	stageBundle := filepath.Join(stage, "bundle")
	if err := managedpack.MaterializeClosure(ctx, projectRoot, stageBundle, validation); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, managedpackpromotion.Reject(managedpackpromotion.GateValidation, fmt.Sprintf("materialize validated closure: %v", err))
	}

	for _, record := range validation.Files {
		destination := record.Path
		if record.Path == "pack.json" {
			destination = filepath.ToSlash(filepath.Join("packs", target, "pack.json"))
		}
		source := filepath.Join(stageBundle, filepath.FromSlash(destination))
		targetPath := filepath.Join(bundleRoot, filepath.FromSlash(destination))
		staged, err := inspectFile(source)
		if err != nil {
			return nil, fmt.Errorf("inspect staged closure path %q: %w", destination, err)
		}
		current, currentErr := inspectFile(targetPath)
		if currentErr == nil && current == staged {
			continue
		}
		if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect materialization target %q: %w", destination, currentErr)
		}
		if err := copyExactFile(source, targetPath, staged.mode); err != nil {
			return nil, fmt.Errorf("publish materialized path %q: %w", destination, err)
		}
		changed[destination] = true
	}
	return changed, nil
}

func validateCandidateRoots(validation managedpack.Validation, ownership bundleOwnership, candidateFiles map[string]managedpack.FileRecord) error {
	target := validation.Manifest.ID
	candidateRoots := map[string]bool{}
	for _, resource := range validation.Manifest.Resources {
		if resource.Source != "" {
			candidateRoots[resource.Source] = true
		}
		for _, binding := range resource.Bindings {
			for _, source := range binding.ReferencedSourcePaths() {
				candidateRoots[source] = true
			}
		}
	}
	for candidateRoot := range candidateRoots {
		for existingRoot, owners := range ownership.roots {
			other := false
			for owner := range owners {
				if owner != target {
					other = true
				}
			}
			if !other || !rootsOverlap(candidateRoot, existingRoot) {
				continue
			}
			if candidateRoot != existingRoot {
				return managedpackpromotion.Reject(managedpackpromotion.GateOwnership, fmt.Sprintf("candidate root %q overlaps another Pack root %q", candidateRoot, existingRoot))
			}
			existingPaths := map[string]bool{}
			for path, pathOwners := range ownership.owners {
				if path == existingRoot || strings.HasPrefix(path, existingRoot+"/") {
					for owner := range pathOwners {
						if owner != target {
							existingPaths[path] = true
						}
					}
				}
			}
			candidatePaths := map[string]bool{}
			for path := range candidateFiles {
				if path == candidateRoot || strings.HasPrefix(path, candidateRoot+"/") {
					candidatePaths[path] = true
				}
			}
			if !reflect.DeepEqual(existingPaths, candidatePaths) {
				return managedpackpromotion.Reject(managedpackpromotion.GateOwnership, fmt.Sprintf("shared root %q must retain the other Pack's complete file set", candidateRoot))
			}
		}
	}
	return nil
}

func rootsOverlap(left, right string) bool {
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func copyExactFile(source, destination, mode string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	permission := os.FileMode(0o644)
	if mode == "100755" {
		permission = 0o755
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".promotion-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}
	if err := temporary.Chmod(permission); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	keep = true
	return nil
}

func removeEmptyParents(path, stop string) {
	stop = filepath.Clean(stop)
	for path = filepath.Clean(path); path != stop && path != filepath.Dir(path); path = filepath.Dir(path) {
		if err := os.Remove(path); err != nil {
			return
		}
	}
}

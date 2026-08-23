package repositorycandidate

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/yersonargotev/packy/internal/managedpack"
)

func currentFileEvidence(repositoryRoot string, current managedpack.Manifest) ([]managedpack.FileRecord, error) {
	record, err := managedpack.LoadAdmissionRecord(filepath.Join(repositoryRoot, filepath.FromSlash(admissionRoot), current.ID, current.Version+".json"))
	if err == nil {
		return append([]managedpack.FileRecord(nil), record.Files...), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	root := filepath.Join(repositoryRoot, "bundle")
	paths := make(map[string]managedpack.FileRecord)
	for _, resource := range current.Resources {
		roots := []string{resource.Source}
		for _, binding := range resource.Bindings {
			roots = append(roots, binding.ReferencedSourcePaths()...)
		}
		for _, relativeRoot := range roots {
			if relativeRoot == "" {
				continue
			}
			err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(relativeRoot)), func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					return nil
				}
				state, err := inspectFile(path)
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				relative = filepath.ToSlash(relative)
				paths[relative] = managedpack.FileRecord{Path: relative, Mode: state.mode, SHA256: state.sha256}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	files := make([]managedpack.FileRecord, 0, len(ordered))
	for _, path := range ordered {
		files = append(files, paths[path])
	}
	return files, nil
}

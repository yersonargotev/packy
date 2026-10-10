// Package catalogcandidate owns disposable authoring workspaces for local
// snapshot lifecycle acceptance. These workspaces cannot select official data.
package catalogcandidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/go-git/go-git/v5"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

const markerName = "local-candidate.json"

// Workspace fixes every writable lifecycle root beneath one explicit directory.
type Workspace struct {
	Home, ConfigurationHome, Project string
	Snapshot                         catalogstore.Snapshot
	ArchiveSHA256                    string
}

type marker struct {
	SchemaVersion int    `json:"schema_version"`
	Provenance    string `json:"provenance"`
	Root          string `json:"root"`
	ArchiveSHA256 string `json:"archive_sha256"`
}

// Open creates a new workspace or reuses one bound to exactly these candidate
// bytes. Rejected input leaves an existing workspace and unrelated files intact.
// Callers must not concurrently edit or operate on the workspace.
func Open(ctx context.Context, assetsRoot, root string) (Workspace, error) {
	if !filepath.IsAbs(assetsRoot) || !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return Workspace{}, errors.New("candidate snapshot and workspace must be explicit absolute paths")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Clean(root)))
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve candidate workspace parent: %w", err)
	}
	root = filepath.Join(parent, filepath.Base(filepath.Clean(root)))
	stage, err := os.MkdirTemp(parent, ".packy-candidate-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(stage)
	snapshot, archiveSHA, err := catalogstore.UnpackCandidate(ctx, assetsRoot, filepath.Join(stage, "snapshot"))
	if err != nil {
		return Workspace{}, err
	}
	expected := marker{SchemaVersion: 1, Provenance: "local-candidate", Root: root, ArchiveSHA256: archiveSHA}
	if _, err := os.Lstat(root); err == nil {
		if err := validateWorkspace(root); err != nil {
			return Workspace{}, err
		}
		data, err := os.ReadFile(filepath.Join(root, markerName))
		if err != nil {
			return Workspace{}, errors.New("candidate workspace already exists without local-candidate ownership")
		}
		var recorded marker
		if err := json.Unmarshal(data, &recorded); err != nil || recorded != expected {
			return Workspace{}, errors.New("candidate workspace ownership or snapshot identity conflicts; use a new disposable workspace")
		}
		retained, err := catalogstore.InspectCandidate(filepath.Join(root, "snapshot"))
		if err != nil {
			return Workspace{}, err
		}
		if !reflect.DeepEqual(retained.Index, snapshot.Index) {
			return Workspace{}, errors.New("retained local candidate differs from supplied snapshot")
		}
		return workspace(root, retained, archiveSHA), nil
	} else if !os.IsNotExist(err) {
		return Workspace{}, err
	}
	for _, name := range []string{"home", "config", "project"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0o700); err != nil {
			return Workspace{}, err
		}
	}
	if _, err := git.PlainInit(filepath.Join(stage, "project"), false); err != nil {
		return Workspace{}, fmt.Errorf("initialize disposable Git project: %w", err)
	}
	data, err := json.Marshal(expected)
	if err != nil {
		return Workspace{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, markerName), append(data, '\n'), 0o600); err != nil {
		return Workspace{}, err
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if err := os.Rename(stage, root); err != nil {
		return Workspace{}, fmt.Errorf("create owned candidate workspace: %w", err)
	}
	snapshot.Root = filepath.Join(root, "snapshot")
	return workspace(root, snapshot, archiveSHA), nil
}

func workspace(root string, snapshot catalogstore.Snapshot, archiveSHA string) Workspace {
	return Workspace{Home: filepath.Join(root, "home"), ConfigurationHome: filepath.Join(root, "config"), Project: filepath.Join(root, "project"), Snapshot: snapshot, ArchiveSHA256: archiveSHA}
}

func validateWorkspace(root string) error {
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("candidate workspace contains an unsafe path %q", path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, name := range []string{"home", "config", "project", "project/.git"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("candidate workspace directory %q is missing", name)
		}
	}
	return nil
}

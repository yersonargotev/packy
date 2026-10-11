// Package catalogcandidate owns disposable authoring workspaces for local
// snapshot lifecycle acceptance. These workspaces cannot select official data.
package catalogcandidate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

const markerName = "local-candidate.json"

var snapshotIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Workspace fixes every writable lifecycle root beneath one explicit directory.
type Workspace struct {
	Home, ConfigurationHome, Project string
	Snapshot                         catalogstore.Snapshot
	ArchiveSHA256                    string
	root                             string
	retained                         map[string]snapshotIdentity
}

type snapshotIdentity struct {
	ArchiveSHA256 string `json:"archive_sha256"`
	IndexSHA256   string `json:"index_sha256"`
}

type marker struct {
	SchemaVersion int                         `json:"schema_version"`
	Provenance    string                      `json:"provenance"`
	Root          string                      `json:"root"`
	SnapshotID    string                      `json:"snapshot_id"`
	Snapshots     map[string]snapshotIdentity `json:"snapshots"`
}

// Open creates a workspace or observes its currently selected snapshot. Changing
// availability requires Transition; ordinary lifecycle calls cannot change it.
// Callers must not concurrently edit or operate on the workspace.
func Open(ctx context.Context, assetsRoot, root string) (Workspace, error) {
	root, err := resolveRoot(assetsRoot, root)
	if err != nil {
		return Workspace{}, err
	}
	_, statErr := os.Lstat(root)
	exists := statErr == nil
	var recorded marker
	if exists {
		recorded, err = inspectWorkspace(root)
		if err != nil {
			return Workspace{}, err
		}
	} else if !os.IsNotExist(statErr) {
		return Workspace{}, statErr
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".packy-candidate-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(stage)
	snapshot, archiveSHA, err := catalogstore.UnpackCandidate(ctx, assetsRoot, filepath.Join(stage, "snapshot"))
	if err != nil {
		return Workspace{}, err
	}
	identity, err := identify(snapshot, archiveSHA)
	if err != nil {
		return Workspace{}, err
	}
	if exists {
		if recorded.SnapshotID != snapshot.ID || recorded.Snapshots[snapshot.ID] != identity {
			return Workspace{}, errors.New("candidate workspace snapshot identity conflicts; use an explicit transition")
		}
		retained, err := inspectRetained(root, snapshot.ID, identity)
		if err != nil {
			return Workspace{}, err
		}
		if !reflect.DeepEqual(retained.Index, snapshot.Index) {
			return Workspace{}, errors.New("retained local candidate differs from supplied snapshot")
		}
		return workspace(root, retained, recorded), nil
	}
	for _, name := range []string{"home", "config", "project", "snapshots"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0o700); err != nil {
			return Workspace{}, err
		}
	}
	if _, err := git.PlainInit(filepath.Join(stage, "project"), false); err != nil {
		return Workspace{}, fmt.Errorf("initialize disposable Git project: %w", err)
	}
	if err := os.Rename(snapshot.Root, filepath.Join(stage, "snapshots", snapshot.ID)); err != nil {
		return Workspace{}, err
	}
	recorded = marker{SchemaVersion: 2, Provenance: "local-candidate", Root: root, SnapshotID: snapshot.ID, Snapshots: map[string]snapshotIdentity{snapshot.ID: identity}}
	if err := writeMarker(filepath.Join(stage, markerName), recorded); err != nil {
		return Workspace{}, err
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	if err := os.Rename(stage, root); err != nil {
		return Workspace{}, fmt.Errorf("create owned candidate workspace: %w", err)
	}
	snapshot.Root = snapshotPath(root, snapshot.ID)
	return workspace(root, snapshot, recorded), nil
}

// Transition changes only snapshot availability, retaining every immutable
// source needed by installed receipts. The expected archive binds the request to
// the observed current selection. A failed acknowledgement restores the previous
// selection and removes newly retained bytes, so failed reporting is unapplied.
func Transition(ctx context.Context, assetsRoot, root, expectedArchive string, acknowledge func(Workspace) error) (Workspace, error) {
	root, err := resolveRoot(assetsRoot, root)
	if err != nil {
		return Workspace{}, err
	}
	recorded, err := inspectWorkspace(root)
	if err != nil {
		return Workspace{}, err
	}
	if !digestPattern.MatchString(expectedArchive) || recorded.Snapshots[recorded.SnapshotID].ArchiveSHA256 != expectedArchive {
		return Workspace{}, errors.New("candidate transition expected archive identity does not match current selection")
	}
	stage, err := os.MkdirTemp(filepath.Dir(root), ".packy-candidate-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(stage)
	snapshot, archiveSHA, err := catalogstore.UnpackCandidate(ctx, assetsRoot, filepath.Join(stage, "snapshot"))
	if err != nil {
		return Workspace{}, err
	}
	identity, err := identify(snapshot, archiveSHA)
	if err != nil {
		return Workspace{}, err
	}
	if snapshot.ID == recorded.SnapshotID {
		return Workspace{}, errors.New("candidate transition must select a different snapshot identity")
	}
	previous, retained := recorded.Snapshots[snapshot.ID]
	if retained && previous != identity {
		return Workspace{}, errors.New("candidate transition conflicts with a retained snapshot identity")
	}
	updated := marker{SchemaVersion: 2, Provenance: recorded.Provenance, Root: root, SnapshotID: snapshot.ID, Snapshots: make(map[string]snapshotIdentity, len(recorded.Snapshots)+1)}
	for id, value := range recorded.Snapshots {
		updated.Snapshots[id] = value
	}
	updated.Snapshots[snapshot.ID] = identity
	previousMarker := filepath.Join(stage, "previous-marker.json")
	if err := writeMarker(previousMarker, recorded); err != nil {
		return Workspace{}, err
	}
	markerStage := filepath.Join(stage, markerName)
	if err := writeMarker(markerStage, updated); err != nil {
		return Workspace{}, err
	}
	fresh, err := inspectWorkspace(root)
	if err != nil {
		return Workspace{}, err
	}
	if !reflect.DeepEqual(fresh, recorded) {
		return Workspace{}, errors.New("candidate transition selection changed during preparation")
	}
	if err := ctx.Err(); err != nil {
		return Workspace{}, err
	}
	destination := snapshotPath(root, snapshot.ID)
	if !retained {
		if _, err := os.Lstat(destination); !os.IsNotExist(err) {
			return Workspace{}, errors.New("candidate transition destination already exists")
		}
		if err := os.Rename(snapshot.Root, destination); err != nil {
			return Workspace{}, err
		}
	}
	if err := os.Rename(markerStage, filepath.Join(root, markerName)); err != nil {
		if !retained {
			_ = os.RemoveAll(destination)
		}
		return Workspace{}, fmt.Errorf("select transitioned candidate: %w", err)
	}
	snapshot.Root = destination
	result := workspace(root, snapshot, updated)
	if acknowledge != nil {
		if err := acknowledge(result); err != nil {
			rollbackErr := os.Rename(previousMarker, filepath.Join(root, markerName))
			if rollbackErr == nil && !retained {
				rollbackErr = os.RemoveAll(destination)
			}
			return Workspace{}, errors.Join(err, rollbackErr)
		}
	}
	return result, nil
}

// Resolve validates receipt-referenced sources in this workspace only.
func (w Workspace) Resolve(ctx context.Context, snapshotID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	identity, ok := w.retained[snapshotID]
	if !ok {
		return "", fmt.Errorf("snapshot %q is outside this candidate workspace", snapshotID)
	}
	if err := validateWorkspace(w.root); err != nil {
		return "", err
	}
	snapshot, err := inspectRetained(w.root, snapshotID, identity)
	if err != nil {
		return "", err
	}
	return snapshot.Root, nil
}

func resolveRoot(assetsRoot, root string) (string, error) {
	if !filepath.IsAbs(assetsRoot) || !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return "", errors.New("candidate snapshot and workspace must be explicit absolute paths")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Clean(root)))
	if err != nil {
		return "", fmt.Errorf("resolve candidate workspace parent: %w", err)
	}
	return filepath.Join(parent, filepath.Base(filepath.Clean(root))), nil
}

func identify(snapshot catalogstore.Snapshot, archiveSHA string) (snapshotIdentity, error) {
	data, err := os.ReadFile(filepath.Join(snapshot.Root, "catalog-index.json"))
	if err != nil {
		return snapshotIdentity{}, err
	}
	return snapshotIdentity{ArchiveSHA256: archiveSHA, IndexSHA256: fmt.Sprintf("%x", sha256.Sum256(data))}, nil
}

func inspectRetained(root, id string, identity snapshotIdentity) (catalogstore.Snapshot, error) {
	snapshot, err := catalogstore.InspectCandidate(snapshotPath(root, id))
	if err != nil {
		return catalogstore.Snapshot{}, err
	}
	observed, err := identify(snapshot, identity.ArchiveSHA256)
	if err != nil {
		return catalogstore.Snapshot{}, err
	}
	if snapshot.ID != id || observed != identity {
		return catalogstore.Snapshot{}, errors.New("retained local candidate identity has drifted")
	}
	return snapshot, nil
}

func inspectWorkspace(root string) (marker, error) {
	if err := validateWorkspace(root); err != nil {
		return marker{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, markerName))
	if err != nil {
		return marker{}, errors.New("candidate workspace already exists without local-candidate ownership")
	}
	var recorded marker
	if err := json.Unmarshal(data, &recorded); err != nil || recorded.SchemaVersion != 2 || recorded.Provenance != "local-candidate" || recorded.Root != root || !snapshotIDPattern.MatchString(recorded.SnapshotID) {
		return marker{}, errors.New("candidate workspace ownership conflicts")
	}
	if _, ok := recorded.Snapshots[recorded.SnapshotID]; !ok {
		return marker{}, errors.New("candidate workspace selection is not retained")
	}
	for id, identity := range recorded.Snapshots {
		if !snapshotIDPattern.MatchString(id) || !digestPattern.MatchString(identity.ArchiveSHA256) || !digestPattern.MatchString(identity.IndexSHA256) {
			return marker{}, errors.New("candidate workspace retained identity is invalid")
		}
		if _, err := inspectRetained(root, id, identity); err != nil {
			return marker{}, err
		}
	}
	return recorded, nil
}

func writeMarker(path string, recorded marker) error {
	data, err := json.Marshal(recorded)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func snapshotPath(root, id string) string { return filepath.Join(root, "snapshots", id) }
func workspace(root string, snapshot catalogstore.Snapshot, recorded marker) Workspace {
	return Workspace{Home: filepath.Join(root, "home"), ConfigurationHome: filepath.Join(root, "config"), Project: filepath.Join(root, "project"), Snapshot: snapshot, ArchiveSHA256: recorded.Snapshots[snapshot.ID].ArchiveSHA256, root: root, retained: recorded.Snapshots}
}

func validateWorkspace(root string) error {
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// Global skill projections link to retained immutable source trees.
			// Isolation roots and project projections must never be redirected.
			relative, _ := filepath.Rel(root, path)
			parts := strings.Split(filepath.ToSlash(relative), "/")
			global := len(parts) == 4 && parts[2] == "skills" && (parts[0] == "home" && (parts[1] == ".agents" || parts[1] == ".claude") || parts[0] == "config" && parts[1] == "opencode")
			target, err := filepath.EvalSymlinks(path)
			if !global || err != nil || !strings.HasPrefix(target, filepath.Join(root, "snapshots")+string(filepath.Separator)) {
				return fmt.Errorf("candidate workspace contains an unsafe path %q", path)
			}
			return nil
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("candidate workspace contains an unsafe path %q", path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, name := range []string{"home", "config", "project", "project/.git", "snapshots"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("candidate workspace directory %q is missing", name)
		}
	}
	project := filepath.Join(root, "project")
	discovered, err := capabilitypack.DiscoverProjectRoot(project)
	if err != nil || discovered != project {
		return errors.New("candidate workspace Git boundary is damaged; refusing an enclosing project")
	}
	return nil
}

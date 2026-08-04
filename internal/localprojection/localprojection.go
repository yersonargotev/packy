package localprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type Executor struct {
	Host         string
	SymlinkKinds map[capabilitypack.ProjectionActionKind]bool
	FileKinds    map[capabilitypack.ProjectionActionKind]bool
	TreeKinds    map[capabilitypack.ProjectionActionKind]bool
}

// TreeFile is one inert regular file in a staged local projection tree.
type TreeFile struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

// ExactTreeEntry is one path fact in an immutable local tree snapshot.
type ExactTreeEntry struct {
	Path          string
	Mode          fs.FileMode
	Directory     bool
	ContentSHA256 string
}

// ExactTreeSnapshot binds every relative directory and regular-file fact.
type ExactTreeSnapshot struct {
	Entries []ExactTreeEntry
	SHA256  string
}

// Apply stages all supported local projections before committing them and
// restores already-committed targets if a later commit fails.
func (e Executor) Apply(actions []capabilitypack.ProjectionAction) error {
	changes := make([]FilesystemChange, 0, len(actions))
	targets := map[string]int{}
	for _, action := range actions {
		change := FilesystemChange{ID: action.ID, Target: action.Target, Delete: action.Mode == capabilitypack.ProjectionDeleteTarget, stagePrefix: ".packy-stage-", stageKey: string(action.Kind) + ":" + action.ID, errorVerb: "commit"}
		change.ExpectedTargetFingerprint = action.Precondition
		change.ExactTreeTarget = e.TreeKinds[action.Kind]
		if !change.Delete {
			switch {
			case e.SymlinkKinds[action.Kind]:
				change.SymlinkSource = action.Source
			case e.TreeKinds[action.Kind]:
				files, err := exactTreeFiles(action.Source)
				if err != nil {
					return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("read copied tree: %w", err)}
				}
				change.TreeFiles = files
				change.ExpectedTreeFingerprint = action.Version
			case e.FileKinds[action.Kind]:
				change.RegularFile = true
				change.FileContent = []byte(action.Content)
				change.FileMode = fs.FileMode(action.FileMode)
				if change.FileMode == 0 {
					change.FileMode = 0o600
				}
			default:
				return capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("unsupported %s projection action %q", e.Host, action.Kind)}
			}
		}
		target := filepath.Clean(action.Target)
		if index, ok := targets[target]; ok {
			changes[index] = change
			continue
		}
		targets[target] = len(changes)
		changes = append(changes, change)
	}
	return ApplyFilesystemChanges(changes)
}

// exactTreeFiles converts an immutable source tree into the inert file payload
// required by ApplyFilesystemChanges. Its sealed fingerprint is carried by the
// action's Version field and verified before any target is published.
func exactTreeFiles(root string) ([]TreeFile, error) {
	files := make([]TreeFile, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source tree contains non-regular path %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, TreeFile{Path: filepath.ToSlash(relative), Content: content, Mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// FingerprintCopiedTree returns the exact fingerprint a regular source tree
// will have after Packy materializes it with project-owned directory modes.
func FingerprintCopiedTree(root string) (string, error) {
	files, err := exactTreeFiles(root)
	if err != nil {
		return "", err
	}
	return FingerprintTreeFiles(files)
}

func ensureDir(dir string) ([]string, error) {
	var missing []string
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}
	return missing, nil
}

func FingerprintBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func FingerprintPath(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "broken", true, nil
		}
		value, err := FingerprintTree(target)
		return value, true, err
	}
	if info.IsDir() {
		value, err := FingerprintTree(path)
		return value, true, err
	}
	data, err := os.ReadFile(path)
	return FingerprintBytes(data), true, err
}

func FingerprintTree(root string) (string, error) {
	var parts []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts = append(parts, filepath.ToSlash(rel)+"="+FingerprintBytes(data))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(parts)
	return FingerprintBytes([]byte(strings.Join(parts, "\n"))), nil
}

// FingerprintTreeFiles binds every relative path, file mode, and byte sequence
// before a composite tree reaches a host-visible path.
func FingerprintTreeFiles(files []TreeFile) (string, error) {
	normalized, err := normalizedTreeFiles(files)
	if err != nil {
		return "", err
	}
	entries := []ExactTreeEntry{{Path: ".", Mode: 0o700, Directory: true}}
	directories := map[string]bool{".": true}
	for _, file := range normalized {
		for dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path))); dir != "."; dir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(dir))) {
			directories[dir] = true
		}
		entries = append(entries, ExactTreeEntry{Path: file.Path, Mode: file.Mode.Perm(), ContentSHA256: FingerprintBytes(file.Content)})
	}
	for path := range directories {
		if path != "." {
			entries = append(entries, ExactTreeEntry{Path: path, Mode: 0o700, Directory: true})
		}
	}
	return fingerprintExactTreeEntries(entries), nil
}

// SnapshotExactTree rejects links and special files and binds every directory
// path/mode plus every regular-file path/mode/content digest.
func SnapshotExactTree(root string) (ExactTreeSnapshot, error) {
	var entries []ExactTreeEntry
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			entries = append(entries, ExactTreeEntry{Path: rel, Mode: info.Mode().Perm(), Directory: true})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("composite tree contains non-regular path %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, ExactTreeEntry{Path: rel, Mode: info.Mode().Perm(), ContentSHA256: FingerprintBytes(data)})
		return nil
	})
	if err != nil {
		return ExactTreeSnapshot{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return ExactTreeSnapshot{Entries: entries, SHA256: fingerprintExactTreeEntries(entries)}, nil
}

// FingerprintExactTree returns the digest of an exact immutable tree snapshot.
func FingerprintExactTree(root string) (string, error) {
	snapshot, err := SnapshotExactTree(root)
	return snapshot.SHA256, err
}

// ChangedExactTreePaths derives the sorted concrete relative paths whose exact
// directory or regular-file facts differ between two snapshots.
func ChangedExactTreePaths(before, after ExactTreeSnapshot) []string {
	beforeFacts := exactTreeFactsByPath(before.Entries)
	afterFacts := exactTreeFactsByPath(after.Entries)
	paths := map[string]bool{}
	for path, fact := range beforeFacts {
		if afterFacts[path] != fact {
			paths[path] = true
		}
	}
	for path, fact := range afterFacts {
		if beforeFacts[path] != fact {
			paths[path] = true
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func fingerprintExactTreeEntries(entries []ExactTreeEntry) string {
	facts := make([]string, len(entries))
	for i, entry := range entries {
		facts[i] = exactTreeEntryFact(entry)
	}
	sort.Strings(facts)
	return FingerprintBytes([]byte(strings.Join(facts, "\n")))
}

func exactTreeFactsByPath(entries []ExactTreeEntry) map[string]string {
	facts := make(map[string]string, len(entries))
	for _, entry := range entries {
		facts[entry.Path] = exactTreeEntryFact(entry)
	}
	return facts
}

func exactTreeEntryFact(entry ExactTreeEntry) string {
	kind := "file"
	if entry.Directory {
		kind = "directory"
	}
	return fmt.Sprintf("%s\x00%s\x00%04o\x00%s", entry.Path, kind, entry.Mode.Perm(), entry.ContentSHA256)
}

// TreeChange is one sealed tree replacement or deletion in a coherent local
// batch. Delete changes intentionally carry no files or fingerprint.
type TreeChange struct {
	ID                  string
	Target              string
	Files               []TreeFile
	ExpectedFingerprint string
	Delete              bool
}

// FilesystemChange is one neutral symlink or exact-tree replacement in a
// coherent host-local transaction.
type FilesystemChange struct {
	ID                        string
	Target                    string
	SymlinkSource             string
	TreeFiles                 []TreeFile
	ExpectedTreeFingerprint   string
	ExpectedTargetFingerprint string
	ExactTreeTarget           bool
	RegularFile               bool
	FileContent               []byte
	FileMode                  fs.FileMode
	Delete                    bool
	stagePrefix               string
	stageKey                  string
	errorVerb                 string
}

type stagedFilesystemChange struct {
	change        FilesystemChange
	stage, backup string
	hadTarget     bool
}

// ApplyFilesystemChanges stages and verifies every symlink and exact tree
// before publishing any target, then restores the complete batch if one commit
// boundary fails.
func ApplyFilesystemChanges(changes []FilesystemChange) error {
	items := make([]stagedFilesystemChange, 0, len(changes))
	createdSet := map[string]bool{}
	var createdDirs []string
	succeeded := false
	defer func() {
		for _, item := range items {
			_ = os.RemoveAll(item.stage)
			if succeeded {
				_ = os.RemoveAll(item.backup)
			}
		}
		if !succeeded {
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = os.Remove(createdDirs[i])
			}
		}
	}()
	targets := map[string]bool{}
	for _, change := range changes {
		target := filepath.Clean(change.Target)
		if change.ID == "" || target == "." || targets[target] {
			return capabilitypack.ProjectionActionError{ID: change.ID, Err: errors.New("filesystem change requires unique identity and target")}
		}
		targets[target] = true
		tree := len(change.TreeFiles) > 0 || change.ExpectedTreeFingerprint != ""
		file := change.RegularFile
		if change.Delete {
			if tree || file || change.SymlinkSource != "" {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: errors.New("filesystem deletion must not contain replacement payload")}
			}
		} else if boolCount(tree, file, change.SymlinkSource != "") != 1 {
			return capabilitypack.ProjectionActionError{ID: change.ID, Err: errors.New("filesystem change must contain exactly one symlink, regular-file, or tree payload")}
		}
		dirs, err := ensureDir(filepath.Dir(target))
		if err != nil {
			return capabilitypack.ProjectionActionError{ID: change.ID, Err: err}
		}
		for _, dir := range dirs {
			if !createdSet[dir] {
				createdSet[dir] = true
				createdDirs = append(createdDirs, dir)
			}
		}
		prefix := change.stagePrefix
		if prefix == "" {
			prefix = ".packy-batch-stage-"
		}
		key := change.stageKey
		if key == "" {
			key = change.ID + "\x00" + target
		}
		suffix := FingerprintBytes([]byte(key))[:12]
		stage := filepath.Join(filepath.Dir(target), prefix+suffix)
		_ = os.RemoveAll(stage)
		item := stagedFilesystemChange{change: change, stage: stage, backup: stage + ".backup"}
		item.change.Target = target
		if _, err := os.Lstat(target); err == nil {
			item.hadTarget = true
		} else if !os.IsNotExist(err) {
			return capabilitypack.ProjectionActionError{ID: change.ID, Err: err}
		}
		items = append(items, item)
		if change.Delete {
			continue
		} else if change.SymlinkSource != "" {
			if err := os.Symlink(change.SymlinkSource, stage); err != nil {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: fmt.Errorf("stage symlink: %w", err)}
			}
			if _, err := filepath.EvalSymlinks(stage); err != nil {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: fmt.Errorf("validate staged symlink: %w", err)}
			}
		} else if file {
			mode := change.FileMode
			if mode == 0 {
				mode = 0o600
			}
			if err := os.WriteFile(stage, change.FileContent, mode); err != nil {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: fmt.Errorf("stage: %w", err)}
			}
			staged, err := os.ReadFile(stage)
			if err != nil || string(staged) != string(change.FileContent) {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: errors.New("validate staged: content mismatch")}
			}
		} else {
			normalized, err := normalizedTreeFiles(change.TreeFiles)
			if err != nil {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: err}
			}
			fingerprint, err := FingerprintTreeFiles(normalized)
			if err != nil || fingerprint != change.ExpectedTreeFingerprint {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: errors.New("tree payload does not match sealed fingerprint")}
			}
			if err := stageTree(stage, normalized, fingerprint); err != nil {
				return capabilitypack.ProjectionActionError{ID: change.ID, Err: err}
			}
		}
	}
	committed := 0
	for i := range items {
		item := &items[i]
		if item.change.ExpectedTargetFingerprint != "" {
			actual, err := filesystemChangeTargetFingerprint(item.change)
			if err != nil {
				return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: err}
			}
			if actual != item.change.ExpectedTargetFingerprint {
				return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("target changed after preflight: expected %s, observed %s", item.change.ExpectedTargetFingerprint, actual)}
			}
		}
		verb := item.change.errorVerb
		if verb == "" {
			verb = "backup"
		}
		if item.hadTarget {
			if err := renameTreePath(item.change.Target, item.backup); err != nil {
				if rollbackErr := rollbackFilesystemChanges(items[:committed]); rollbackErr != nil {
					return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("%s: %v; rollback failed: %w", verb, err, rollbackErr)}
				}
				return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("%s: %w", verb, err)}
			}
		}
		if item.change.Delete {
			committed++
			continue
		}
		if err := renameTreePath(item.stage, item.change.Target); err != nil {
			if item.hadTarget {
				if restoreErr := renameTreePath(item.backup, item.change.Target); restoreErr != nil {
					if rollbackErr := rollbackFilesystemChanges(items[:committed]); rollbackErr != nil {
						return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("publish: %v; restore current target failed: %v; rollback failed: %w", err, restoreErr, rollbackErr)}
					}
					return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("publish: %v; restore current target failed: %w", err, restoreErr)}
				}
			}
			if rollbackErr := rollbackFilesystemChanges(items[:committed]); rollbackErr != nil {
				return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("publish: %v; rollback failed: %w", err, rollbackErr)}
			}
			return capabilitypack.ProjectionActionError{ID: item.change.ID, Err: fmt.Errorf("publish: %w", err)}
		}
		committed++
	}
	succeeded = true
	return nil
}

func filesystemChangeTargetFingerprint(change FilesystemChange) (string, error) {
	info, err := os.Lstat(change.Target)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if change.ExactTreeTarget || len(change.TreeFiles) > 0 || change.ExpectedTreeFingerprint != "" {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			fingerprint, _, err := FingerprintPath(change.Target)
			return fingerprint, err
		}
		return FingerprintExactTree(change.Target)
	}
	fingerprint, _, err := FingerprintPath(change.Target)
	return fingerprint, err
}

func rollbackFilesystemChanges(items []stagedFilesystemChange) error {
	for i := len(items) - 1; i >= 0; i-- {
		if err := os.RemoveAll(items[i].change.Target); err != nil {
			return err
		}
		if items[i].hadTarget {
			if err := renameTreePath(items[i].backup, items[i].change.Target); err != nil {
				return capabilitypack.ProjectionActionError{ID: items[i].change.ID, Err: err}
			}
		}
	}
	return nil
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

var renameTreePath = os.Rename

// ReplaceTrees stages and verifies every replacement before publishing any
// host-visible target, then rolls the complete batch back if a later commit
// fails.
func ReplaceTrees(changes []TreeChange) error {
	filesystemChanges := make([]FilesystemChange, 0, len(changes))
	for _, change := range changes {
		if change.ID == "" || change.Target == "" {
			return errors.New("composite tree change requires identity and target")
		}
		filesystemChanges = append(filesystemChanges, FilesystemChange{
			ID: change.ID, Target: change.Target, TreeFiles: change.Files,
			ExpectedTreeFingerprint: change.ExpectedFingerprint, Delete: change.Delete,
			stagePrefix: ".packy-tree-stage-", stageKey: change.ID + "\x00" + filepath.Clean(change.Target),
		})
	}
	return ApplyFilesystemChanges(filesystemChanges)
}

func ReplaceTree(target string, files []TreeFile, expectedFingerprint string) error {
	return ReplaceTrees([]TreeChange{{ID: target, Target: target, Files: files, ExpectedFingerprint: expectedFingerprint}})
}

func stageTree(stage string, files []TreeFile, expectedFingerprint string) error {
	if err := os.Mkdir(stage, 0o700); err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(stage, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("stage composite tree: %w", err)
		}
		if err := os.WriteFile(path, file.Content, file.Mode.Perm()); err != nil {
			return fmt.Errorf("stage composite tree: %w", err)
		}
		if err := os.Chmod(path, file.Mode.Perm()); err != nil {
			return fmt.Errorf("stage composite tree mode: %w", err)
		}
	}
	fingerprint, err := FingerprintExactTree(stage)
	if err != nil {
		return fmt.Errorf("verify staged composite tree: %w", err)
	}
	if fingerprint != expectedFingerprint {
		return errors.New("staged composite tree fingerprint mismatch")
	}
	return nil
}

func normalizedTreeFiles(files []TreeFile) ([]TreeFile, error) {
	if len(files) == 0 {
		return nil, errors.New("composite tree requires at least one file")
	}
	result := make([]TreeFile, len(files))
	seen := map[string]bool{}
	for i, file := range files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if file.Path == "" || file.Path != clean || clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(filepath.FromSlash(file.Path)) || strings.Contains(file.Path, "\\") {
			return nil, fmt.Errorf("invalid composite tree path %q", file.Path)
		}
		if seen[file.Path] {
			return nil, fmt.Errorf("duplicate composite tree path %q", file.Path)
		}
		seen[file.Path] = true
		mode := file.Mode.Perm()
		if mode != 0o644 && mode != 0o755 {
			return nil, fmt.Errorf("invalid composite tree mode %04o for %q", mode, file.Path)
		}
		result[i] = TreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: mode}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

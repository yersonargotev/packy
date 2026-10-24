// Package skillaccess prepares reviewed skill resources without entering the
// Pack installation lifecycle or executing catalog content.
package skillaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogtransaction"
)

var snapshotPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Error identifies an actionable retrieval failure without a partial success.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func failure(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Skill describes file availability only. It carries no runtime or native
// registration assertion. The consuming agent checks the declared prerequisites.
type Skill struct {
	QualifiedSkill string                 `json:"qualified_skill"`
	PackVersion    string                 `json:"pack_version"`
	Surface        capabilitypack.Surface `json:"surface"`
	SnapshotID     string                 `json:"snapshot_id"`
	EntryDocument  string                 `json:"entry_document"`
	ResourceRoot   string                 `json:"resource_root"`
	Prerequisites  []string               `json:"prerequisites"`
}

// Result includes the complete deterministic transitive skill closure.
type Result struct {
	Skill
	Dependencies []Skill `json:"dependencies"`
}

// ValidateRequest checks identity and explicit host selection before any I/O.
func ValidateRequest(qualified string, surface capabilitypack.Surface) error {
	if !capabilitypack.ValidQualifiedSkill(qualified) {
		return failure("invalid_identity", "skill must be qualified as <pack>:<skill> using lowercase kebab-case; run `packy list` and `packy show <pack>`")
	}
	if surface != capabilitypack.SurfaceClaude && surface != capabilitypack.SurfaceCodex {
		return failure("unsupported_surface", "select an explicit supported surface with --surface codex or --surface claude (received %q)", surface)
	}
	return nil
}

// Prepare consumes a validated retained snapshot. The caller must resolve the
// exact snapshot through catalogstore or catalogcandidate, never a source tree
// override. Index identities are checked again while copying the selected files.
func Prepare(ctx context.Context, snapshotRoot, snapshotID, dataRoot, qualified string, surface capabilitypack.Surface) (Result, error) {
	if err := ValidateRequest(qualified, surface); err != nil {
		return Result{}, err
	}
	if !snapshotPattern.MatchString(snapshotID) {
		return Result{}, failure("snapshot_unavailable", "a retained immutable Catalog Snapshot identity is required")
	}
	dataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return Result{}, failure("preparation_failed", "resolve Packy data root: %v", err)
	}
	parts := strings.Split(qualified, ":")
	pack, err := capabilitypack.ValidatePackContent(snapshotRoot, parts[0])
	if err != nil {
		return Result{}, failure("unknown_skill", "cannot obtain %s from snapshot %s; run `packy show %s`: %v", qualified, snapshotID, parts[0], err)
	}
	if !pack.Selectable {
		return Result{}, failure("unknown_skill", "Pack %s is not selectable in this Catalog Snapshot; run `packy list` for available Packs", pack.ID)
	}

	// Load every Pack from the same retained snapshot. Native file dependencies
	// remain Pack-local; only the explicit access graph can cross this boundary.
	entries, err := os.ReadDir(filepath.Join(snapshotRoot, "packs"))
	if err != nil {
		return Result{}, failure("snapshot_unavailable", "read retained catalog: %v", err)
	}
	packs := []capabilitypack.Pack{}
	for _, entry := range entries {
		loaded, err := capabilitypack.ValidatePackContent(snapshotRoot, entry.Name())
		if err != nil {
			return Result{}, failure("snapshot_unavailable", "validate retained Pack %s: %v", entry.Name(), err)
		}
		packs = append(packs, loaded)
	}
	effectiveRoot := capabilitypack.ResolvePackForSurface(pack, surface)
	var rootSkill *capabilitypack.Resource
	for i := range effectiveRoot.Resources {
		if effectiveRoot.Resources[i].Kind == "skill" && effectiveRoot.Resources[i].ID == parts[1] {
			rootSkill = &effectiveRoot.Resources[i]
			break
		}
	}
	if rootSkill == nil {
		return Result{}, failure("unknown_skill", "unknown skill %s; inspect packy show %s", qualified, pack.ID)
	}
	supported := false
	for _, binding := range rootSkill.Bindings {
		supported = supported || binding.Surface == surface
	}
	if !supported {
		return Result{}, failure("unsupported_surface", "skill %s is excluded or unbound for %s", qualified, surface)
	}
	resources, err := capabilitypack.ResolveOnDemand(packs, qualified, surface)
	if err != nil {
		return Result{}, failure("incompatible_resource", "%v", err)
	}
	skill := resources[0]
	rootsByPack := map[string][]string{}
	targets := map[string]string{}
	for _, owned := range resources {
		resource := owned.Resource
		rootsByPack[owned.Pack.ID] = append(rootsByPack[owned.Pack.ID], resource.Source)
		if resource.Kind == "skill" {
			targets[owned.Pack.ID+":"+resource.ID] = filepath.ToSlash(filepath.Join("packs", owned.Pack.ID, resource.Source, resource.OnDemand.AccessEntry()))
		}
	}
	files := []cataloglayout.FileRecord{}
	for packID, roots := range rootsByPack {
		declared, err := declaredFiles(snapshotRoot, snapshotID, packID, roots)
		if err != nil {
			return Result{}, failure("snapshot_unavailable", "read retained snapshot %s: %v", snapshotID, err)
		}
		for _, file := range declared {
			file.Path = filepath.ToSlash(filepath.Join("packs", packID, file.Path))
			files = append(files, file)
		}
	}
	slices.SortFunc(files, func(a, b cataloglayout.FileRecord) int { return strings.Compare(a.Path, b.Path) })
	destination, err := preparationPath(dataRoot, snapshotRoot, snapshotID, surface, pack.ID, skill.Resource.ID)
	if err != nil {
		return Result{}, failure("preparation_failed", "prepare owned resource directory: %v", err)
	}
	rendered, err := renderedFiles(snapshotRoot, files, resources, targets)
	if err != nil {
		return Result{}, failure("incompatible_resource", "review on-demand references for %s: %v", qualified, err)
	}
	if err := prepareFiles(ctx, snapshotRoot, destination, files, rendered); err != nil {
		return Result{}, failure("preparation_failed", "prepare %s: %v; preserve catalog sources and use separate writable scratch/output paths", qualified, err)
	}
	describe := func(owned capabilitypack.AccessResource) Skill {
		resource := owned.Resource
		root := filepath.Join(destination, "packs", owned.Pack.ID, resource.Source)
		prerequisites := append(slices.Clone(resource.OnDemand.Prerequisites), owned.Pack.Requires.Tools...)
		slices.Sort(prerequisites)
		prerequisites = slices.Compact(prerequisites)
		return Skill{QualifiedSkill: owned.Pack.ID + ":" + resource.ID, PackVersion: owned.Pack.Version, Surface: surface, SnapshotID: snapshotID, EntryDocument: filepath.Join(root, resource.OnDemand.AccessEntry()), ResourceRoot: root, Prerequisites: prerequisites}
	}
	result := Result{Skill: describe(skill), Dependencies: []Skill{}}
	for _, owned := range resources[1:] {
		if owned.Resource.Kind == "skill" {
			result.Dependencies = append(result.Dependencies, describe(owned))
		}
	}
	slices.SortFunc(result.Dependencies, func(a, b Skill) int { return strings.Compare(a.QualifiedSkill, b.QualifiedSkill) })
	return result, nil
}

// Refuse linked descendants so preparing data can never write through a link
// into catalog sources, installed projections, or a consumer's target project.
func preparationPath(dataRoot, snapshotRoot, snapshotID string, surface capabilitypack.Surface, packID, skillID string) (string, error) {
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(dataRoot)
	if err != nil {
		return "", err
	}
	source, err := filepath.EvalSymlinks(snapshotRoot)
	if err != nil {
		return "", err
	}
	if relative, err := filepath.Rel(source, root); err != nil || relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prepared resources must be outside the Catalog Snapshot")
	}
	for _, component := range []string{"skill-access", "v2", snapshotID, string(surface), packID} {
		root = filepath.Join(root, component)
		if err := os.Mkdir(root, 0755); err != nil && !os.IsExist(err) {
			return "", err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("prepared resource parent %s must be a directory, not a link", root)
		}
	}
	return filepath.Join(root, skillID), nil
}

// Compute expected rendered bytes before publishing any portion of the closure.
func renderedFiles(source string, files []cataloglayout.FileRecord, resources []capabilitypack.AccessResource, targets map[string]string) (map[string][]byte, error) {
	indexed := map[string]bool{}
	for _, file := range files {
		indexed[file.Path] = true
	}
	rendered := map[string][]byte{}
	for _, owned := range resources {
		resource := owned.Resource
		if resource.Kind != "skill" {
			continue
		}
		entry := filepath.ToSlash(filepath.Join("packs", owned.Pack.ID, resource.Source, resource.OnDemand.AccessEntry()))
		if !indexed[entry] {
			return nil, fmt.Errorf("skill %s entry_document %s is missing", resource.ID, entry)
		}
		for _, relative := range resource.OnDemand.ReferenceFiles {
			path := filepath.ToSlash(filepath.Join("packs", owned.Pack.ID, resource.Source, relative))
			if !indexed[path] {
				return nil, fmt.Errorf("skill %s reference file %s is missing", resource.ID, path)
			}
			data, err := os.ReadFile(filepath.Join(source, path))
			if err != nil {
				return nil, err
			}
			body, err := capabilitypack.RenderAccessReferences(owned.Pack.ID, resource, relative, data, targets)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			rendered[path] = body
		}
	}
	return rendered, nil
}

func declaredFiles(snapshotRoot, snapshotID, packID string, roots []string) ([]cataloglayout.FileRecord, error) {
	data, err := os.ReadFile(filepath.Join(snapshotRoot, "catalog-index.json"))
	if err != nil {
		return nil, err
	}
	var index cataloglayout.CatalogSnapshotIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.SchemaVersion != 3 || index.Source.Commit != snapshotID {
		return nil, fmt.Errorf("retained index does not match Catalog Snapshot %s", snapshotID)
	}
	for _, pack := range index.Packs {
		if pack.ID != packID {
			continue
		}
		files := []cataloglayout.FileRecord{}
		for _, file := range pack.Files {
			include := file.Path == "pack.json" // Preserve reviewed provenance and attribution declarations.
			for _, root := range roots {
				include = include || file.Path == root || strings.HasPrefix(file.Path, root+"/")
			}
			if include {
				files = append(files, file)
			}
		}
		return files, nil
	}
	return nil, fmt.Errorf("Pack %s has no retained file index", packID)
}

func prepareFiles(ctx context.Context, source, destination string, files []cataloglayout.FileRecord, rendered map[string][]byte) error {
	expected := slices.Clone(files)
	for i, file := range expected {
		if data, ok := rendered[file.Path]; ok {
			sum := sha256.Sum256(data)
			expected[i].SHA256 = hex.EncodeToString(sum[:])
		}
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	return catalogtransaction.WithExclusive(ctx, parent, func() error {
		if _, err := os.Lstat(destination); err == nil {
			return verifyFiles(destination, expected)
		} else if !os.IsNotExist(err) {
			return err
		}
		stage, err := os.MkdirTemp(parent, ".preparing-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			sourcePath := filepath.Join(source, file.Path)
			info, err := os.Lstat(sourcePath)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || (info.Mode().Perm()&0111 != 0) != (file.Mode == "100755") {
				return fmt.Errorf("source %s mode differs from retained index", file.Path)
			}
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				return err
			}
			if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != file.SHA256 {
				return fmt.Errorf("source %s differs from retained index", file.Path)
			}
			if body, ok := rendered[file.Path]; ok {
				data = body
			}
			path := filepath.Join(stage, file.Path)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			mode := os.FileMode(0644)
			if file.Mode == "100755" {
				mode = 0755
			}
			if err := os.WriteFile(path, data, mode); err != nil {
				return err
			}
			if err := os.Chmod(path, mode); err != nil {
				return err
			}
		}
		if err := verifyFiles(stage, expected); err != nil {
			return err
		}
		return os.Rename(stage, destination)
	})
}

func verifyFiles(root string, files []cataloglayout.FileRecord) error {
	expected := make(map[string]cataloglayout.FileRecord, len(files))
	for _, file := range files {
		expected[file.Path] = file
	}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file, ok := expected[filepath.ToSlash(relative)]
		if !ok || !info.Mode().IsRegular() {
			return fmt.Errorf("prepared path %s is unexpected or not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		mode := os.FileMode(0644)
		if file.Mode == "100755" {
			mode = 0755
		}
		if hex.EncodeToString(sum[:]) != file.SHA256 || info.Mode().Perm() != mode {
			return fmt.Errorf("prepared path %s has changed; do not edit retained resources", path)
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(files) {
		return fmt.Errorf("prepared tree is incomplete; required retained resources are missing")
	}
	return nil
}

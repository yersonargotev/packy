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

var qualifiedPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*:[a-z0-9]+(?:-[a-z0-9]+)*$`)
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

// Result describes file availability only. It carries no runtime or native
// registration assertion. The consuming agent checks the declared prerequisites.
type Result struct {
	QualifiedSkill string                 `json:"qualified_skill"`
	PackVersion    string                 `json:"pack_version"`
	Surface        capabilitypack.Surface `json:"surface"`
	SnapshotID     string                 `json:"snapshot_id"`
	EntryDocument  string                 `json:"entry_document"`
	ResourceRoot   string                 `json:"resource_root"`
	Prerequisites  []string               `json:"prerequisites"`
}

// ValidateRequest checks identity and explicit host selection before any I/O.
func ValidateRequest(qualified string, surface capabilitypack.Surface) error {
	if !qualifiedPattern.MatchString(qualified) {
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
	pack = capabilitypack.ResolvePackForSurface(pack, surface)
	var skill capabilitypack.Resource
	for _, resource := range pack.Resources {
		if resource.Kind == "skill" && resource.ID == parts[1] {
			skill = resource
			break
		}
	}
	if skill.ID == "" {
		return Result{}, failure("unknown_skill", "unknown skill %s; run `packy show %s` to inspect its resources", qualified, pack.ID)
	}
	var binding *capabilitypack.Binding
	for i := range skill.Bindings {
		if skill.Bindings[i].Surface == surface {
			binding = &skill.Bindings[i]
			break
		}
	}
	if binding == nil {
		return Result{}, failure("unsupported_surface", "skill %s has no supported %s definition; inspect `packy show %s`", qualified, surface, pack.ID)
	}
	if skill.OnDemand == nil || skill.OnDemand.Protocol != "skill-tree-v1" {
		return Result{}, failure("incompatible_resource", "skill %s has no reviewed on_demand skill-tree-v1 contract for %s; ask the Catalog maintainer to review it", qualified, surface)
	}
	for _, capability := range binding.Capabilities {
		if capability.Type == capabilitypack.SurfaceCapabilitySkillTemplate || capability.Type == capabilitypack.SurfaceCapabilityClaudeCompositeSkill {
			return Result{}, failure("incompatible_resource", "skill %s requires native rendering or composition; provide a reviewed standalone on-demand resource tree", qualified)
		}
	}
	roots, err := supportingRoots(pack, skill)
	if err != nil {
		return Result{}, err
	}
	files, err := declaredFiles(snapshotRoot, snapshotID, pack.ID, roots)
	if err != nil {
		return Result{}, failure("snapshot_unavailable", "read retained snapshot %s: %v", snapshotID, err)
	}
	destination, err := preparationPath(dataRoot, snapshotRoot, snapshotID, surface, pack.ID, skill.ID)
	if err != nil {
		return Result{}, failure("preparation_failed", "prepare owned resource directory: %v", err)
	}
	if err := prepareFiles(ctx, filepath.Join(snapshotRoot, "packs", pack.ID), destination, files); err != nil {
		return Result{}, failure("preparation_failed", "prepare %s: %v; preserve catalog sources and use separate writable scratch/output paths", qualified, err)
	}
	root := filepath.Join(destination, skill.Source)
	prerequisites := append(slices.Clone(skill.OnDemand.Prerequisites), pack.Requires.Tools...)
	slices.Sort(prerequisites)
	prerequisites = slices.Compact(prerequisites)
	return Result{QualifiedSkill: qualified, PackVersion: pack.Version, Surface: surface, SnapshotID: snapshotID,
		EntryDocument: filepath.Join(root, "SKILL.md"), ResourceRoot: root, Prerequisites: prerequisites}, nil
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
	for _, component := range []string{"skill-access", "v1", snapshotID, string(surface), packID} {
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

func supportingRoots(pack capabilitypack.Pack, skill capabilitypack.Resource) ([]string, error) {
	resources := map[string]capabilitypack.Resource{}
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
	}
	roots := []string{}
	seen := map[string]bool{}
	var visit func(capabilitypack.Resource) error
	visit = func(resource capabilitypack.Resource) error {
		identity := resource.Kind + ":" + resource.ID
		if seen[identity] {
			return nil
		}
		seen[identity] = true
		if identity != "skill:"+skill.ID && resource.Kind == "skill" {
			return failure("skill_dependencies_unsupported", "skill %s:%s requires %s; on-demand skill dependencies are not supported in this slice", pack.ID, skill.ID, identity)
		}
		if resource.Kind != "skill" && resource.Kind != "asset" && resource.Kind != "notice" {
			return failure("incompatible_resource", "skill %s:%s requires %s, which is not a supporting file or notice", pack.ID, skill.ID, identity)
		}
		roots = append(roots, resource.Source)
		for _, dependency := range append(slices.Clone(resource.Requires), resource.Notices...) {
			target, ok := resources[dependency]
			if !ok {
				return failure("incompatible_resource", "missing declared supporting resource %s", dependency)
			}
			if err := visit(target); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(skill); err != nil {
		return nil, err
	}
	return roots, nil
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

func prepareFiles(ctx context.Context, source, destination string, files []cataloglayout.FileRecord) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	return catalogtransaction.WithExclusive(ctx, parent, func() error {
		if _, err := os.Lstat(destination); err == nil {
			return verifyFiles(destination, files)
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
		if err := verifyFiles(stage, files); err != nil {
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

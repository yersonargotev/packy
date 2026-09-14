// Package catalogauthor owns atomic, validated Catalog Project authoring.
package catalogauthor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/bundletransaction"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

var (
	authoringIDPattern         = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	authoringRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	authoringCommitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// CreateRequest describes one supported Catalog Project template expansion.
type CreateRequest struct {
	ProjectRoot string
	Template    string
	PackID      string
	Version     string
	Description string
	Surfaces    []string
}

// ImportRequest describes one explicit Pack Import from an immutable origin.
type ImportRequest struct {
	ProjectRoot  string
	PackID       string
	Version      string
	Repository   string
	Commit       string
	OriginID     string
	OriginPath   string
	Destination  string
	Relationship string
	Kind         string
	ResourceID   string
	Description  string
	Hosts        []string
	Notices      []string
	License      string
	Attribution  string
	Requires     []string
	Conflicts    []string
}

// Result identifies the validated reviewable content left by an operation.
type Result struct {
	PackID       string
	Version      string
	Resource     string
	Relationship string
	Repository   string
	Commit       string
	Packs        int
}

// Create prepares and validates a new Pack before atomically installing its
// manifest in the Catalog Project. The only supported template is empty.
func Create(ctx context.Context, request CreateRequest, resolver managedpack.OriginResolver) (Result, error) {
	var result Result
	err := bundletransaction.WithExclusive(ctx, request.ProjectRoot, func() error {
		var err error
		result, err = create(ctx, request, resolver)
		return err
	})
	return result, err
}

func create(ctx context.Context, request CreateRequest, resolver managedpack.OriginResolver) (Result, error) {
	if request.Template != "empty" {
		return Result{}, fmt.Errorf("unsupported Pack template %q; supported templates: empty", request.Template)
	}
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return Result{}, fmt.Errorf("Catalog Project path is required")
	}
	if !authoringIDPattern.MatchString(request.PackID) {
		return Result{}, fmt.Errorf("Pack id must be lowercase kebab-case")
	}
	surfaces, err := parseSurfaces(request.Surfaces)
	if err != nil {
		return Result{}, err
	}
	manifest := managedpack.Manifest{
		SchemaVersion: managedpack.SchemaVersion,
		ID:            request.PackID, Version: request.Version, Description: request.Description,
		Selectable: true, Surfaces: surfaces,
		ReadinessObligations: []capabilitypack.ReadinessObligation{},
		ExternalRequirements: []string{}, Origins: []managedpack.Origin{}, Resources: []managedpack.Resource{},
	}
	stage, cleanup, err := stageBundle(request.ProjectRoot)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	manifestRelative := filepath.Join("bundle", "packs", request.PackID, "pack.json")
	stagedManifest := filepath.Join(stage, manifestRelative)
	if _, err := os.Lstat(stagedManifest); err == nil {
		return Result{}, fmt.Errorf("Pack %q already exists", request.PackID)
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("inspect Pack %q: %w", request.PackID, err)
	}
	if err := writeManifest(stagedManifest, manifest); err != nil {
		return Result{}, err
	}
	validation, err := managedpack.ValidateCatalogProject(ctx, stage, "", resolver)
	if err != nil {
		return Result{}, fmt.Errorf("validate prepared Catalog Project: %w", err)
	}
	destination := filepath.Join(request.ProjectRoot, manifestRelative)
	if _, err := installNewPath(stagedManifest, destination, request.ProjectRoot); err != nil {
		return Result{}, fmt.Errorf("write prepared Pack: %w", err)
	}
	return Result{PackID: manifest.ID, Version: manifest.Version, Packs: len(validation.Packs)}, nil
}

// Import prepares one explicitly selected resource, validates the complete
// Catalog Project, and only then applies the resource and manifest together.
func Import(ctx context.Context, request ImportRequest, resolver managedpack.OriginResolver) (Result, error) {
	var result Result
	err := bundletransaction.WithExclusive(ctx, request.ProjectRoot, func() error {
		var err error
		result, err = importResource(ctx, request, resolver)
		return err
	})
	return result, err
}

func importResource(ctx context.Context, request ImportRequest, resolver managedpack.OriginResolver) (Result, error) {
	if resolver == nil {
		return Result{}, fmt.Errorf("Pack Import requires an exact-origin resolver")
	}
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return Result{}, fmt.Errorf("Catalog Project path is required")
	}
	if err := validateImportRequest(request); err != nil {
		return Result{}, err
	}
	stage, cleanup, err := stageBundle(request.ProjectRoot)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	manifestRelative := filepath.Join("bundle", "packs", request.PackID, "pack.json")
	actualManifest := filepath.Join(request.ProjectRoot, manifestRelative)
	stagedManifest := filepath.Join(stage, manifestRelative)
	beforeManifest, manifest, err := readManifest(stagedManifest)
	if err != nil {
		return Result{}, fmt.Errorf("read Pack %q: %w", request.PackID, err)
	}
	manifest.Version = request.Version

	origin := managedpack.Origin{ID: request.OriginID, Repository: request.Repository, Commit: request.Commit}
	originRoot, err := resolver.Resolve(ctx, origin)
	if err != nil {
		return Result{}, fmt.Errorf("resolve exact upstream commit: %w", err)
	}
	if strings.TrimSpace(originRoot) == "" {
		return Result{}, fmt.Errorf("resolve exact upstream commit: empty local root")
	}
	if request.Kind != "notice" && len(request.Notices) == 0 {
		detected := detectNotice(originRoot)
		if detected != "" {
			return Result{}, fmt.Errorf("resource notice is required; detected upstream notice %q; import it first and provide --notice notice:<id>", detected)
		}
		return Result{}, fmt.Errorf("resource notice is required; import a notice resource first and provide --notice notice:<id>")
	}

	resource, err := buildResource(request, manifest)
	if err != nil {
		return Result{}, err
	}
	if err := addOrigin(&manifest, origin); err != nil {
		return Result{}, err
	}
	if err := addResource(&manifest, resource); err != nil {
		return Result{}, err
	}

	stagedDestination := filepath.Join(stage, "bundle", filepath.FromSlash(request.Destination))
	if _, err := os.Lstat(stagedDestination); err == nil {
		return Result{}, fmt.Errorf("destination %q already exists", request.Destination)
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("inspect destination %q: %w", request.Destination, err)
	}
	source := filepath.Join(originRoot, filepath.FromSlash(request.OriginPath))
	if err := copyPath(source, stagedDestination); err != nil {
		return Result{}, fmt.Errorf("prepare resource from origin path %q: %w", request.OriginPath, err)
	}
	if err := writeManifest(stagedManifest, manifest); err != nil {
		return Result{}, err
	}
	validation, err := managedpack.ValidateCatalogProject(ctx, stage, request.ProjectRoot, resolver)
	if err != nil {
		return Result{}, fmt.Errorf("validate prepared Catalog Project: %w", err)
	}

	actualDestination := filepath.Join(request.ProjectRoot, "bundle", filepath.FromSlash(request.Destination))
	rollback, err := installNewPath(stagedDestination, actualDestination, filepath.Join(request.ProjectRoot, "bundle"))
	if err != nil {
		return Result{}, fmt.Errorf("write prepared resource: %w", err)
	}
	if err := replaceUnchangedFile(actualManifest, beforeManifest, stagedManifest); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			return Result{}, fmt.Errorf("write prepared manifest: %v; rollback failed: %w", err, rollbackErr)
		}
		return Result{}, fmt.Errorf("write prepared manifest: %w", err)
	}
	return Result{
		PackID: manifest.ID, Version: manifest.Version,
		Resource:     request.Kind + ":" + request.ResourceID,
		Relationship: request.Relationship, Repository: request.Repository,
		Commit: request.Commit, Packs: len(validation.Packs),
	}, nil
}

func validateImportRequest(request ImportRequest) error {
	if !authoringIDPattern.MatchString(request.PackID) {
		return fmt.Errorf("Pack id must be lowercase kebab-case")
	}
	if !authoringRepositoryPattern.MatchString(request.Repository) {
		return fmt.Errorf("repository must be an owner/name identity")
	}
	if !authoringCommitPattern.MatchString(request.Commit) {
		return fmt.Errorf("commit must be a full lowercase Git object ID")
	}
	if !authoringIDPattern.MatchString(request.OriginID) {
		return fmt.Errorf("origin id must be lowercase kebab-case")
	}
	if !authoringIDPattern.MatchString(request.ResourceID) {
		return fmt.Errorf("resource id must be lowercase kebab-case")
	}
	if strings.TrimSpace(request.Version) == "" {
		return fmt.Errorf("new Pack version is required")
	}
	if strings.TrimSpace(request.Description) == "" {
		return fmt.Errorf("resource description is required")
	}
	if err := validateRelativePath(request.OriginPath, true); err != nil {
		return fmt.Errorf("origin path: %w", err)
	}
	if err := validateRelativePath(request.Destination, false); err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	return nil
}

func validateRelativePath(value string, allowDot bool) error {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return fmt.Errorf("%q must be a normalized repository-relative path", value)
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean != value || (!allowDot && clean == ".") || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%q must be a normalized repository-relative path", value)
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.EqualFold(part, ".git") {
			return fmt.Errorf("%q must not select Git metadata", value)
		}
	}
	return nil
}

func buildResource(request ImportRequest, manifest managedpack.Manifest) (managedpack.Resource, error) {
	if request.Kind != "skill" && request.Kind != "instruction" && request.Kind != "notice" {
		return managedpack.Resource{}, fmt.Errorf("unsupported import resource kind %q; supported kinds: instruction, notice, skill", request.Kind)
	}
	relationship := managedpack.Relationship(request.Relationship)
	if relationship != managedpack.RelationshipExactCopy && relationship != managedpack.RelationshipAdapted {
		return managedpack.Resource{}, fmt.Errorf("relationship must be exact-copy or adapted")
	}
	resource := managedpack.Resource{
		Kind: request.Kind, ID: request.ResourceID, Source: filepath.ToSlash(request.Destination),
		Description: request.Description,
		Requires:    sortedCopy(request.Requires), Conflicts: sortedCopy(request.Conflicts),
		Notices: sortedCopy(request.Notices), Bindings: []capabilitypack.Binding{},
		SurfaceExclusions: []capabilitypack.SurfaceExclusion{},
		Origin:            &managedpack.ResourceOrigin{ID: request.OriginID, Path: filepath.ToSlash(request.OriginPath), Relationship: relationship},
	}
	if request.Kind == "notice" {
		if len(request.Hosts) != 0 {
			return managedpack.Resource{}, fmt.Errorf("notice imports do not accept --host")
		}
		if strings.TrimSpace(request.License) == "" || strings.TrimSpace(request.Attribution) == "" {
			return managedpack.Resource{}, fmt.Errorf("notice imports require --license and --attribution")
		}
		resource.License = request.License
		resource.Attribution = request.Attribution
		resource.Notices = []string{"notice:" + request.ResourceID}
		return resource, nil
	}
	if strings.TrimSpace(request.License) != "" || strings.TrimSpace(request.Attribution) != "" {
		return managedpack.Resource{}, fmt.Errorf("--license and --attribution apply only to notice imports")
	}
	hosts, err := parseSurfaces(request.Hosts)
	if err != nil {
		return managedpack.Resource{}, fmt.Errorf("hosts: %w", err)
	}
	if !reflect.DeepEqual(hosts, manifest.Surfaces) {
		return managedpack.Resource{}, fmt.Errorf("--host must explicitly name every Pack surface %v", manifest.Surfaces)
	}
	for _, surface := range hosts {
		invocation := request.ResourceID
		sharing := "shared"
		if request.Kind == "skill" {
			sharing = "exclusive"
			switch surface {
			case capabilitypack.SurfaceCodex:
				invocation = "$" + request.ResourceID
			case capabilitypack.SurfaceClaude:
				invocation = "/" + request.ResourceID
			}
		}
		resource.Bindings = append(resource.Bindings, capabilitypack.Binding{
			Surface: surface, Projection: request.Kind, Name: request.ResourceID,
			Invocation: invocation, Mode: "native", Sharing: sharing,
			Capabilities: []capabilitypack.SurfaceCapability{},
		})
	}
	return resource, nil
}

func parseSurfaces(values []string) ([]capabilitypack.Surface, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one surface is required")
	}
	result := make([]capabilitypack.Surface, 0, len(values))
	seen := map[capabilitypack.Surface]bool{}
	for _, value := range values {
		surface := capabilitypack.Surface(value)
		if surface != capabilitypack.SurfaceClaude && surface != capabilitypack.SurfaceCodex && surface != capabilitypack.SurfaceOpenCode {
			return nil, fmt.Errorf("unsupported surface %q", value)
		}
		if seen[surface] {
			return nil, fmt.Errorf("surface %q is duplicated", value)
		}
		seen[surface] = true
		result = append(result, surface)
	}
	slices.Sort(result)
	return result, nil
}

func addOrigin(manifest *managedpack.Manifest, origin managedpack.Origin) error {
	for _, existing := range manifest.Origins {
		if existing.ID == origin.ID {
			if existing != origin {
				return fmt.Errorf("origin %q already identifies %s@%s", existing.ID, existing.Repository, existing.Commit)
			}
			return nil
		}
		if strings.EqualFold(existing.Repository, origin.Repository) {
			return fmt.Errorf("repository %q already uses origin id %q", origin.Repository, existing.ID)
		}
	}
	manifest.Origins = append(manifest.Origins, origin)
	sort.Slice(manifest.Origins, func(i, j int) bool { return manifest.Origins[i].ID < manifest.Origins[j].ID })
	return nil
}

func addResource(manifest *managedpack.Manifest, resource managedpack.Resource) error {
	identity := resource.Kind + ":" + resource.ID
	for _, existing := range manifest.Resources {
		if existing.Kind+":"+existing.ID == identity {
			return fmt.Errorf("resource %q already exists", identity)
		}
	}
	manifest.Resources = append(manifest.Resources, resource)
	sort.Slice(manifest.Resources, func(i, j int) bool {
		return manifest.Resources[i].Kind+":"+manifest.Resources[i].ID < manifest.Resources[j].Kind+":"+manifest.Resources[j].ID
	})
	return nil
}

func stageBundle(projectRoot string) (string, func(), error) {
	info, err := os.Lstat(projectRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", func() {}, fmt.Errorf("Catalog Project root must be an existing directory and not a symlink")
	}
	stage, err := os.MkdirTemp("", "packy-catalog-author-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create authoring stage: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	if err := copyPath(filepath.Join(projectRoot, "bundle"), filepath.Join(stage, "bundle")); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("stage Catalog Project bundle: %w", err)
	}
	return stage, cleanup, nil
}

func readManifest(path string) ([]byte, managedpack.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, managedpack.Manifest{}, err
	}
	var manifest managedpack.Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, managedpack.Manifest{}, err
	}
	return data, manifest, nil
}

func writeManifest(path string, manifest managedpack.Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode prepared Pack manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare Pack manifest directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write staged Pack manifest: %w", err)
	}
	return nil
}

func copyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlinks are not allowed: %s", source)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("only regular files and directories are supported: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		_ = destinationFile.Close()
		_ = os.Remove(destination)
		return err
	}
	if err := destinationFile.Close(); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func installNewPath(source, destination, stopRoot string) (func() error, error) {
	noRollback := func() error { return nil }
	if _, err := os.Lstat(destination); err == nil {
		return noRollback, fmt.Errorf("destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return noRollback, err
	}
	parent := filepath.Dir(destination)
	createdParents, err := absentParents(parent, stopRoot)
	if err != nil {
		return noRollback, err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return noRollback, err
	}
	cleanupParents := func() {
		for _, path := range createdParents {
			_ = os.Remove(path)
		}
	}
	temporary, err := os.MkdirTemp(parent, ".packy-author-")
	if err != nil {
		cleanupParents()
		return noRollback, err
	}
	defer os.RemoveAll(temporary)
	payload := filepath.Join(temporary, "payload")
	if err := copyPath(source, payload); err != nil {
		cleanupParents()
		return noRollback, err
	}
	if err := os.Rename(payload, destination); err != nil {
		cleanupParents()
		return noRollback, err
	}
	rollback := func() error {
		if err := os.RemoveAll(destination); err != nil {
			return err
		}
		for _, path := range createdParents {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	return rollback, nil
}

func absentParents(parent, stopRoot string) ([]string, error) {
	parent = filepath.Clean(parent)
	stopRoot = filepath.Clean(stopRoot)
	relative, err := filepath.Rel(stopRoot, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("destination parent is outside the Catalog Project")
	}
	var result []string
	for current := parent; current != stopRoot; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("destination parent %s must be a directory and not a symlink", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		result = append(result, current)
	}
	return result, nil
}

func replaceUnchangedFile(destination string, before []byte, source string) error {
	current, err := os.ReadFile(destination)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return fmt.Errorf("Pack manifest changed during preparation")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".packy-author-manifest-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func detectNotice(originRoot string) string {
	for _, name := range []string{"LICENSE", "LICENSE.md", "COPYING", "NOTICE"} {
		info, err := os.Lstat(filepath.Join(originRoot, name))
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return name
		}
	}
	return ""
}

func sortedCopy(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

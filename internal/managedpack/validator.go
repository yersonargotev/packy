// Package managedpack owns the public Managed Pack Project contract.
package managedpack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const SchemaVersion = 1

var (
	idPattern         = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Origin identifies one immutable public External Source Project revision.
type Origin struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Revision   string `json:"revision,omitempty"`
}

// Relationship is one reviewed whole-resource provenance relationship.
type Relationship string

const (
	RelationshipExactCopy Relationship = "exact-copy"
	RelationshipAdapted   Relationship = "adapted"
)

// ResourceOrigin describes a whole-resource provenance relationship.
type ResourceOrigin struct {
	ID           string       `json:"id"`
	Path         string       `json:"path"`
	Relationship Relationship `json:"relationship"`
}

// Resource is one Pack resource plus its Managed Pack provenance.
type Resource struct {
	Kind              string                            `json:"kind"`
	ID                string                            `json:"id"`
	Source            string                            `json:"source,omitempty"`
	Command           string                            `json:"command,omitempty"`
	Args              []string                          `json:"args,omitempty"`
	Description       string                            `json:"description,omitempty"`
	Mode              string                            `json:"mode,omitempty"`
	Tools             []string                          `json:"tools,omitempty"`
	Permissions       []string                          `json:"permissions,omitempty"`
	Arguments         capabilitypack.CommandArguments   `json:"arguments,omitempty"`
	License           string                            `json:"license,omitempty"`
	Attribution       string                            `json:"attribution,omitempty"`
	Requires          []string                          `json:"requires"`
	Conflicts         []string                          `json:"conflicts"`
	Notices           []string                          `json:"notices,omitempty"`
	Origin            *ResourceOrigin                   `json:"origin,omitempty"`
	Bindings          []capabilitypack.Binding          `json:"bindings"`
	SurfaceExclusions []capabilitypack.SurfaceExclusion `json:"surface_exclusions"`
}

// Manifest is the strict schema v1 root pack.json contract.
type Manifest struct {
	SchemaVersion        int                                  `json:"schema_version"`
	ID                   string                               `json:"id"`
	Version              string                               `json:"version"`
	Description          string                               `json:"description"`
	Selectable           bool                                 `json:"selectable"`
	Surfaces             []capabilitypack.Surface             `json:"surfaces"`
	ReadinessObligations []capabilitypack.ReadinessObligation `json:"readiness_obligations"`
	ExternalRequirements []string                             `json:"external_requirements"`
	Origins              []Origin                             `json:"origins"`
	Resources            []Resource                           `json:"resources"`
}

type manifestWire struct {
	SchemaVersion        int                                  `json:"schema_version"`
	ID                   string                               `json:"id"`
	Version              string                               `json:"version"`
	Description          string                               `json:"description"`
	Selectable           *bool                                `json:"selectable"`
	Surfaces             []capabilitypack.Surface             `json:"surfaces"`
	ReadinessObligations []capabilitypack.ReadinessObligation `json:"readiness_obligations"`
	ExternalRequirements []string                             `json:"external_requirements"`
	Origins              []Origin                             `json:"origins"`
	Resources            []Resource                           `json:"resources"`
}

// FileRecord is one deterministic member of the Declared Pack Closure.
type FileRecord struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

// Validation is the sealed result of validating one Managed Pack Project.
type Validation struct {
	Manifest       Manifest
	ManifestSHA256 string
	ClosureSHA256  string
	Files          []FileRecord
}

type declaredRoot struct {
	path  string
	owner string
}

// OriginResolver returns a local, exact checkout for a declared Origin.
// ValidateProject only reads the returned tree and never executes its content.
type OriginResolver interface {
	Resolve(context.Context, Origin) (string, error)
}

// ValidateProject validates one root pack.json, its Declared Pack Closure,
// and every declared provenance relationship.
func ValidateProject(ctx context.Context, projectRoot string, resolver OriginResolver) (Validation, error) {
	manifestPath := filepath.Join(projectRoot, "pack.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return Validation{}, fmt.Errorf("read root pack.json: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() || manifestInfo.Mode()&os.ModeSymlink != 0 {
		return Validation{}, fmt.Errorf("root pack.json must be a regular file")
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return Validation{}, fmt.Errorf("read root pack.json: %w", err)
	}
	manifest, err := decodeManifest(manifestData)
	if err != nil {
		return Validation{}, err
	}
	if err := validateManifest(manifest, projectRoot); err != nil {
		return Validation{}, err
	}

	origins := make(map[string]Origin, len(manifest.Origins))
	for _, origin := range manifest.Origins {
		origins[origin.ID] = origin
	}
	resolved := map[string]string{}
	for _, resource := range manifest.Resources {
		if resource.Origin == nil {
			continue
		}
		origin := origins[resource.Origin.ID]
		originRoot, ok := resolved[origin.ID]
		if !ok {
			if resolver == nil {
				return Validation{}, fmt.Errorf("resource %q requires origin %q but no resolver was provided", resourceIdentity(resource), origin.ID)
			}
			originRoot, err = resolver.Resolve(ctx, origin)
			if err != nil {
				return Validation{}, fmt.Errorf("resolve origin %q: %w", origin.ID, err)
			}
			if strings.TrimSpace(originRoot) == "" {
				return Validation{}, fmt.Errorf("resolve origin %q: empty local root", origin.ID)
			}
			resolved[origin.ID] = originRoot
		}
		if err := validateOriginRelationship(projectRoot, resource, originRoot); err != nil {
			return Validation{}, err
		}
	}

	files, err := declaredClosure(projectRoot, manifest)
	if err != nil {
		return Validation{}, err
	}
	manifestDigest := digestBytes(manifestData)
	files = append(files, FileRecord{Path: "pack.json", Mode: canonicalMode(manifestInfo.Mode()), SHA256: manifestDigest})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return Validation{
		Manifest:       manifest,
		ManifestSHA256: manifestDigest,
		ClosureSHA256:  digestIndex(files),
		Files:          files,
	}, nil
}

func decodeManifest(data []byte) (Manifest, error) {
	var wire manifestWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Manifest{}, fmt.Errorf("decode root pack.json: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Manifest{}, fmt.Errorf("decode root pack.json: %w", err)
	}
	if wire.Selectable == nil {
		return Manifest{}, fmt.Errorf("invalid Managed Pack manifest: field selectable is required")
	}
	return Manifest{
		SchemaVersion: wire.SchemaVersion, ID: wire.ID, Version: wire.Version,
		Description: wire.Description, Selectable: *wire.Selectable,
		Surfaces: wire.Surfaces, ReadinessObligations: wire.ReadinessObligations,
		ExternalRequirements: wire.ExternalRequirements, Origins: wire.Origins,
		Resources: wire.Resources,
	}, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateManifest(manifest Manifest, projectRoot string) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid Managed Pack manifest: schema_version must be %d", SchemaVersion)
	}
	if manifest.Origins == nil {
		return fmt.Errorf("invalid Managed Pack manifest: field origins is a required non-null array")
	}
	if manifest.Resources == nil {
		return fmt.Errorf("invalid Managed Pack manifest: field resources is a required non-null array")
	}
	if err := validateOrigins(manifest.Origins); err != nil {
		return fmt.Errorf("invalid Managed Pack manifest: %w", err)
	}
	pack := capabilitypack.Pack{
		ID: manifest.ID, Version: manifest.Version, Description: manifest.Description,
		Selectable: manifest.Selectable, Surfaces: manifest.Surfaces,
		ReadinessObligations: manifest.ReadinessObligations,
		Requires:             capabilitypack.Requirements{Tools: manifest.ExternalRequirements},
	}
	for _, resource := range manifest.Resources {
		pack.Resources = append(pack.Resources, capabilitypack.Resource{
			Kind: resource.Kind, ID: resource.ID, Source: resource.Source,
			Command: resource.Command, Args: resource.Args, Description: resource.Description,
			Mode: resource.Mode, Tools: resource.Tools, Permissions: resource.Permissions,
			Arguments: resource.Arguments, License: resource.License, Attribution: resource.Attribution,
			Requires: resource.Requires, Conflicts: resource.Conflicts, Notices: resource.Notices,
			Bindings: resource.Bindings, SurfaceExclusions: resource.SurfaceExclusions,
			RequiresTools: []string{},
		})
	}
	if err := capabilitypack.ValidateProjectPack(pack, projectRoot); err != nil {
		return fmt.Errorf("invalid Managed Pack manifest: %w", err)
	}
	return validateManagedResources(manifest)
}

func validateOrigins(origins []Origin) error {
	seenRepositories := map[string]bool{}
	for i, origin := range origins {
		if !idPattern.MatchString(origin.ID) {
			return fmt.Errorf("origin %q id must be lowercase kebab-case", origin.ID)
		}
		if i > 0 && origins[i-1].ID >= origin.ID {
			return fmt.Errorf("field origins must be sorted by id without duplicates")
		}
		if !repositoryPattern.MatchString(origin.Repository) {
			return fmt.Errorf("origin %q repository must be an owner/name identity", origin.ID)
		}
		if seenRepositories[strings.ToLower(origin.Repository)] {
			return fmt.Errorf("origin %q duplicates repository %q", origin.ID, origin.Repository)
		}
		seenRepositories[strings.ToLower(origin.Repository)] = true
		if !commitPattern.MatchString(origin.Commit) {
			return fmt.Errorf("origin %q commit must be a full lowercase Git object ID", origin.ID)
		}
		if origin.Revision != "" && strings.TrimSpace(origin.Revision) == "" {
			return fmt.Errorf("origin %q revision must not be blank", origin.ID)
		}
	}
	return nil
}

func validateManagedResources(manifest Manifest) error {
	origins := map[string]bool{}
	for _, origin := range manifest.Origins {
		origins[origin.ID] = true
	}
	notices := map[string]bool{}
	for _, resource := range manifest.Resources {
		if resource.Kind == "notice" {
			notices[resourceIdentity(resource)] = true
		}
	}
	for _, resource := range manifest.Resources {
		identity := resourceIdentity(resource)
		if err := validateCanonicalLayout(resource); err != nil {
			return fmt.Errorf("resource %q: %w", identity, err)
		}
		if !sort.StringsAreSorted(resource.Notices) || hasDuplicate(resource.Notices) {
			return fmt.Errorf("resource %q notices must be a sorted set", identity)
		}
		for _, notice := range resource.Notices {
			if !notices[notice] {
				return fmt.Errorf("resource %q notice %q does not exist", identity, notice)
			}
		}
		if resource.Origin == nil {
			continue
		}
		if resource.Source == "" {
			return fmt.Errorf("derived resource %q must have a source", identity)
		}
		if !origins[resource.Origin.ID] {
			return fmt.Errorf("resource %q references unknown origin %q", identity, resource.Origin.ID)
		}
		if err := validateRelativePath(resource.Origin.Path, true); err != nil {
			return fmt.Errorf("resource %q origin path: %w", identity, err)
		}
		if hasPathComponent(resource.Origin.Path, ".git") {
			return fmt.Errorf("resource %q origin path must not select Git metadata", identity)
		}
		if resource.Origin.Relationship != RelationshipExactCopy && resource.Origin.Relationship != RelationshipAdapted {
			return fmt.Errorf("resource %q origin relationship must be exact-copy or adapted", identity)
		}
		if len(resource.Notices) == 0 {
			return fmt.Errorf("derived resource %q must reference at least one notice", identity)
		}
	}
	return nil
}

func validateCanonicalLayout(resource Resource) error {
	if resource.Source != "" {
		if err := validateRelativePath(resource.Source, false); err != nil {
			return fmt.Errorf("source: %w", err)
		}
		want := map[string]string{
			"skill": "skills", "instruction": "instructions", "agent": "agents",
			"command": "commands", "asset": "assets", "notice": "notices",
		}[resource.Kind]
		if want == "" {
			return fmt.Errorf("resource kind %q does not own a source root", resource.Kind)
		}
		if strings.Split(resource.Source, "/")[0] != want {
			return fmt.Errorf("source %q must use the canonical %s/ layout", resource.Source, want)
		}
	}
	for _, source := range capabilitySources(resource) {
		if err := validateRelativePath(source, false); err != nil {
			return fmt.Errorf("surface capability source: %w", err)
		}
		if !strings.HasPrefix(source, "instructions/") {
			return fmt.Errorf("surface capability source %q must use the canonical instructions/ layout", source)
		}
	}
	return nil
}

func capabilitySources(resource Resource) []string {
	var sources []string
	for _, binding := range resource.Bindings {
		sources = append(sources, binding.ReferencedSourcePaths()...)
	}
	return sources
}

func validateRelativePath(value string, allowDot bool) error {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return fmt.Errorf("%q must be a normalized repository-relative path", value)
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean != value || !allowDot && clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%q must be a normalized repository-relative path", value)
	}
	return nil
}

func hasPathComponent(value, component string) bool {
	for _, part := range strings.Split(value, "/") {
		if part == component {
			return true
		}
	}
	return false
}

func validateOriginRelationship(projectRoot string, resource Resource, originRoot string) error {
	originPath := resource.Origin.Path
	if err := rejectGitlinks(originRoot, []declaredRoot{{path: originPath, owner: resourceIdentity(resource)}}, fmt.Sprintf("origin %q", resource.Origin.ID)); err != nil {
		return err
	}
	originFiles, err := indexTree(originRoot, originPath, false)
	if err != nil {
		return fmt.Errorf("resource %q origin path: %w", resourceIdentity(resource), err)
	}
	if resource.Origin.Relationship != RelationshipExactCopy {
		return nil
	}
	projectFiles, err := indexTree(projectRoot, resource.Source, true)
	if err != nil {
		return fmt.Errorf("resource %q source: %w", resourceIdentity(resource), err)
	}
	if !sameContent(projectFiles, resource.Source, originFiles, resource.Origin.Path) {
		return fmt.Errorf("resource %q exact-copy content differs from origin %q path %q", resourceIdentity(resource), resource.Origin.ID, originPath)
	}
	return nil
}

func declaredClosure(projectRoot string, manifest Manifest) ([]FileRecord, error) {
	var roots []declaredRoot
	for _, resource := range manifest.Resources {
		identity := resourceIdentity(resource)
		if resource.Source != "" {
			roots = append(roots, declaredRoot{resource.Source, identity})
		}
		for _, source := range capabilitySources(resource) {
			roots = append(roots, declaredRoot{source, identity})
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].path == roots[j].path {
			return roots[i].owner < roots[j].owner
		}
		return roots[i].path < roots[j].path
	})
	unique := roots[:0]
	for _, root := range roots {
		if len(unique) > 0 && unique[len(unique)-1].path == root.path {
			if unique[len(unique)-1].owner != root.owner {
				return nil, fmt.Errorf("resource roots %q and %q overlap at %q", unique[len(unique)-1].owner, root.owner, root.path)
			}
			continue
		}
		for _, prior := range unique {
			if strings.HasPrefix(root.path, prior.path+"/") || strings.HasPrefix(prior.path, root.path+"/") {
				return nil, fmt.Errorf("resource roots %q and %q overlap", prior.path, root.path)
			}
		}
		unique = append(unique, root)
	}
	if err := rejectGitlinks(projectRoot, unique, "Managed Pack Project"); err != nil {
		return nil, err
	}
	byPath := map[string]FileRecord{}
	for _, root := range unique {
		indexed, err := indexTree(projectRoot, root.path, true)
		if err != nil {
			return nil, fmt.Errorf("Declared Pack Closure root %q: %w", root.path, err)
		}
		for _, file := range indexed {
			if _, exists := byPath[file.Path]; exists {
				return nil, fmt.Errorf("Declared Pack Closure contains duplicate path %q", file.Path)
			}
			byPath[file.Path] = file
		}
	}
	files := make([]FileRecord, 0, len(byPath))
	for _, file := range byPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func indexTree(base, relative string, rejectGitMetadata bool) ([]FileRecord, error) {
	if err := validateRelativePath(relative, true); err != nil {
		return nil, err
	}
	if err := rejectSymlinkComponents(base, relative); err != nil {
		return nil, err
	}
	root := filepath.Join(base, filepath.FromSlash(relative))
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", relative, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%q is a symlink", relative)
	}
	var files []FileRecord
	err = filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if name != root && entry.Name() == ".git" {
			if rejectGitMetadata {
				return fmt.Errorf("%q contains submodule or Git metadata", rel)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q is a symlink", rel)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", rel)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{Path: rel, Mode: canonicalMode(info.Mode()), SHA256: digestBytes(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func rejectSymlinkComponents(base, relative string) error {
	if relative == "." {
		return nil
	}
	current := base
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q contains a symlink path component", relative)
		}
	}
	return nil
}

func rejectGitlinks(repositoryRoot string, roots []declaredRoot, scope string) error {
	repository, err := git.PlainOpen(repositoryRoot)
	if err == git.ErrRepositoryNotExists {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s Git index: %w", scope, err)
	}
	index, err := repository.Storer.Index()
	if err != nil {
		return fmt.Errorf("inspect %s Git index: %w", scope, err)
	}
	for _, entry := range index.Entries {
		if entry.Mode != filemode.Submodule {
			continue
		}
		path := filepath.ToSlash(entry.Name)
		for _, root := range roots {
			if root.path == "." || path == root.path || strings.HasPrefix(path, root.path+"/") {
				return fmt.Errorf("%s root %q contains submodule %q", scope, root.path, path)
			}
		}
	}
	return nil
}

func sameContent(left []FileRecord, leftRoot string, right []FileRecord, rightRoot string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		leftRel, leftErr := filepath.Rel(filepath.FromSlash(leftRoot), filepath.FromSlash(left[i].Path))
		rightRel, rightErr := filepath.Rel(filepath.FromSlash(rightRoot), filepath.FromSlash(right[i].Path))
		leftRel = filepath.ToSlash(leftRel)
		rightRel = filepath.ToSlash(rightRel)
		if leftErr != nil || rightErr != nil || leftRel != rightRel || left[i].SHA256 != right[i].SHA256 {
			return false
		}
	}
	return true
}

func digestIndex(files []FileRecord) string {
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", file.Path, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalMode(mode os.FileMode) string {
	if mode.Perm()&0o111 != 0 {
		return "100755"
	}
	return "100644"
}

func resourceIdentity(resource Resource) string {
	return resource.Kind + ":" + resource.ID
}

func hasDuplicate(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] == values[i] {
			return true
		}
	}
	return false
}

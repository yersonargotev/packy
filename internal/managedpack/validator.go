// Package managedpack owns the public Managed Pack Project contract.
package managedpack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const (
	SchemaVersion                    = 1
	maxIndexedEntries                = 1024
	maxIndexedPathDepth              = 32
	maxIndexedFileBytes              = int64(8 << 20)
	maxIndexedTotalBytes             = int64(64 << 20)
	maxExactCopyMismatchDetails      = 10
	maxExactCopyDiagnosticValueBytes = 512
)

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

type exactCopyDifference struct {
	class         string
	path          string
	projectSHA256 string
	originSHA256  string
}

type exactCopyMismatchError struct {
	resource    string
	origin      string
	originPath  string
	differences []exactCopyDifference
	total       int
}

func (e *exactCopyMismatchError) Error() string {
	var message strings.Builder
	fmt.Fprintf(
		&message,
		"resource %q exact-copy mismatch with origin %q path %q",
		boundedDiagnosticValue(e.resource),
		boundedDiagnosticValue(e.origin),
		boundedDiagnosticValue(e.originPath),
	)
	for _, difference := range e.differences {
		fmt.Fprintf(
			&message,
			"; mismatch=%s path=%q project_sha256=%q origin_sha256=%q",
			difference.class,
			boundedDiagnosticValue(difference.path),
			difference.projectSHA256,
			difference.originSHA256,
		)
	}
	if omitted := e.total - len(e.differences); omitted > 0 {
		fmt.Fprintf(&message, "; %d additional differences omitted", omitted)
	}
	message.WriteString("; restore exact bytes from the declared origin or explicitly declare the whole resource \"adapted\" and review its notices")
	return message.String()
}

// IsExactCopyMismatch reports whether validation rejected an exact-copy
// resource because its relative file set or bytes differ from the origin.
func IsExactCopyMismatch(err error) bool {
	var mismatch *exactCopyMismatchError
	return errors.As(err, &mismatch)
}

type declaredRoot struct {
	path  string
	owner string
}

type indexBudget struct {
	entries   int
	totalSize int64
}

func (budget *indexBudget) add(path string, info os.FileInfo) error {
	budget.entries++
	if budget.entries > maxIndexedEntries {
		return fmt.Errorf("%q exceeds maximum entry count of %d", path, maxIndexedEntries)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if info.Size() < 0 || info.Size() > maxIndexedFileBytes {
		return fmt.Errorf("%q exceeds maximum file size of %d bytes", path, maxIndexedFileBytes)
	}
	if budget.totalSize > maxIndexedTotalBytes-info.Size() {
		return fmt.Errorf("%q exceeds maximum aggregate size of %d bytes", path, maxIndexedTotalBytes)
	}
	budget.totalSize += info.Size()
	return nil
}

// OriginResolver returns a local, exact checkout for a declared Origin.
// ValidateProject only reads the returned tree and never executes its content.
type OriginResolver interface {
	Resolve(context.Context, Origin) (string, error)
}

// ValidateProject validates one root pack.json, its Declared Pack Closure,
// and every declared provenance relationship.
func ValidateProject(ctx context.Context, projectRoot string, resolver OriginResolver) (Validation, error) {
	if err := ctx.Err(); err != nil {
		return Validation{}, err
	}
	manifestPath := filepath.Join(projectRoot, "pack.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return Validation{}, fmt.Errorf("read root pack.json: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() || manifestInfo.Mode()&os.ModeSymlink != 0 {
		return Validation{}, fmt.Errorf("root pack.json must be a regular file")
	}
	if manifestInfo.Size() > maxIndexedFileBytes {
		return Validation{}, fmt.Errorf("root pack.json exceeds maximum file size of %d bytes", maxIndexedFileBytes)
	}
	manifestData, err := readFileBounded(ctx, manifestPath, manifestInfo)
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
	originBudget := &indexBudget{}
	comparisonBudget := &indexBudget{}
	for _, resource := range manifest.Resources {
		if err := ctx.Err(); err != nil {
			return Validation{}, err
		}
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
			if err := ctx.Err(); err != nil {
				return Validation{}, err
			}
			if strings.TrimSpace(originRoot) == "" {
				return Validation{}, fmt.Errorf("resolve origin %q: empty local root", origin.ID)
			}
			resolved[origin.ID] = originRoot
		}
		if err := validateOriginRelationship(ctx, projectRoot, resource, originRoot, originBudget, comparisonBudget); err != nil {
			return Validation{}, err
		}
	}

	files, err := declaredClosure(ctx, projectRoot, manifest, manifestInfo.Size())
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

// MaterializeClosure copies one exact validated Declared Pack Closure into a
// destination bundle root. The manifest is placed at packs/<id>/pack.json;
// every other closure member retains its bundle-relative path.
func MaterializeClosure(ctx context.Context, projectRoot, destinationRoot string, validation Validation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateMaterialization(validation); err != nil {
		return err
	}
	if err := requireDirectory(projectRoot, "Managed Pack Project root"); err != nil {
		return err
	}

	budget := &indexBudget{}
	for _, record := range validation.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		sourcePath := filepath.Join(projectRoot, filepath.FromSlash(record.Path))
		if err := rejectSymlinkComponents(ctx, projectRoot, record.Path); err != nil {
			return fmt.Errorf("materialize source %q: %w", record.Path, err)
		}
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return fmt.Errorf("materialize source %q: %w", record.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("materialize source %q is not a regular file", record.Path)
		}
		if err := budget.add(record.Path, info); err != nil {
			return fmt.Errorf("materialize source: %w", err)
		}
		if canonicalMode(info.Mode()) != record.Mode {
			return fmt.Errorf("materialize source %q drifted from validated mode", record.Path)
		}
		digest, err := digestFile(ctx, sourcePath, info)
		if err != nil {
			return fmt.Errorf("materialize source %q: %w", record.Path, err)
		}
		if digest != record.SHA256 {
			return fmt.Errorf("materialize source %q drifted from validated SHA-256", record.Path)
		}
		if record.Path == "pack.json" {
			manifestData, err := readFileBounded(ctx, sourcePath, info)
			if err != nil {
				return fmt.Errorf("materialize source %q: %w", record.Path, err)
			}
			manifest, err := decodeManifest(manifestData)
			if err != nil || !reflect.DeepEqual(manifest, validation.Manifest) {
				return fmt.Errorf("materialize source pack.json drifted from validated manifest")
			}
		}
	}

	if err := ensureDirectory(ctx, destinationRoot, "."); err != nil {
		return fmt.Errorf("prepare destination bundle root: %w", err)
	}
	for _, record := range validation.Files {
		destinationPath := record.Path
		if record.Path == "pack.json" {
			destinationPath = filepath.ToSlash(filepath.Join("packs", validation.Manifest.ID, "pack.json"))
		}
		if err := copyClosureFile(ctx, projectRoot, destinationRoot, record, destinationPath); err != nil {
			return err
		}
	}
	return nil
}

func validateMaterialization(validation Validation) error {
	if !idPattern.MatchString(validation.Manifest.ID) {
		return fmt.Errorf("materialize closure: invalid Pack ID %q", validation.Manifest.ID)
	}
	if len(validation.Files) == 0 || len(validation.Files) > maxIndexedEntries {
		return fmt.Errorf("materialize closure: invalid file index size")
	}
	manifestFound := false
	destinations := make(map[string]bool, len(validation.Files))
	for index, record := range validation.Files {
		if err := validateRelativePath(record.Path, false); err != nil {
			return fmt.Errorf("materialize closure file path: %w", err)
		}
		if hasPathComponent(record.Path, ".git") || pathDepth(record.Path) > maxIndexedPathDepth {
			return fmt.Errorf("materialize closure file path %q is not allowed", record.Path)
		}
		if index > 0 && validation.Files[index-1].Path >= record.Path {
			return fmt.Errorf("materialize closure files must be sorted by path without duplicates")
		}
		if record.Mode != "100644" && record.Mode != "100755" {
			return fmt.Errorf("materialize closure file %q has unsupported mode %q", record.Path, record.Mode)
		}
		if !validSHA256(record.SHA256) {
			return fmt.Errorf("materialize closure file %q has invalid SHA-256", record.Path)
		}
		destinationPath := record.Path
		if record.Path == "pack.json" {
			if manifestFound || record.SHA256 != validation.ManifestSHA256 {
				return fmt.Errorf("materialize closure manifest digest does not match pack.json")
			}
			manifestFound = true
			destinationPath = filepath.ToSlash(filepath.Join("packs", validation.Manifest.ID, "pack.json"))
		}
		if destinations[destinationPath] {
			return fmt.Errorf("materialize closure destination %q is duplicated", destinationPath)
		}
		destinations[destinationPath] = true
	}
	if !manifestFound {
		return fmt.Errorf("materialize closure files must contain pack.json")
	}
	if !validSHA256(validation.ManifestSHA256) || digestIndex(validation.Files) != validation.ClosureSHA256 {
		return fmt.Errorf("materialize closure digest does not match file index")
	}
	return nil
}

func copyClosureFile(ctx context.Context, projectRoot, destinationRoot string, record FileRecord, destinationRelative string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureDirectory(ctx, destinationRoot, filepath.ToSlash(filepath.Dir(destinationRelative))); err != nil {
		return fmt.Errorf("prepare materialized file %q: %w", destinationRelative, err)
	}
	destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(destinationRelative))
	mode := os.FileMode(0o644)
	if record.Mode == "100755" {
		mode = 0o755
	}
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create materialized file %q: %w", destinationRelative, err)
	}
	keep := false
	closed := false
	defer func() {
		if !closed {
			if closeErr := destination.Close(); resultErr == nil && closeErr != nil {
				resultErr = fmt.Errorf("close materialized file %q: %w", destinationRelative, closeErr)
			}
		}
		if !keep || resultErr != nil {
			_ = os.Remove(destinationPath)
		}
	}()

	sourcePath := filepath.Join(projectRoot, filepath.FromSlash(record.Path))
	info, err := os.Lstat(sourcePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("materialize source %q drifted after validation", record.Path)
	}
	if canonicalMode(info.Mode()) != record.Mode {
		return fmt.Errorf("materialize source %q drifted from validated mode", record.Path)
	}
	if err := copyRegularFile(ctx, sourcePath, info, destination); err != nil {
		return fmt.Errorf("copy materialized file %q: %w", destinationRelative, err)
	}
	if err := destination.Chmod(mode); err != nil {
		return fmt.Errorf("set materialized file %q mode: %w", destinationRelative, err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close materialized file %q: %w", destinationRelative, err)
	}
	closed = true
	if err := rejectSymlinkComponents(ctx, destinationRoot, destinationRelative); err != nil {
		return fmt.Errorf("verify materialized file %q: %w", destinationRelative, err)
	}
	destinationInfo, err := os.Lstat(destinationPath)
	if err != nil || destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.Mode().IsRegular() {
		return fmt.Errorf("verify materialized file %q: not a regular file", destinationRelative)
	}
	digest, err := digestFile(ctx, destinationPath, destinationInfo)
	if err != nil {
		return fmt.Errorf("verify materialized file %q: %w", destinationRelative, err)
	}
	if digest != record.SHA256 || canonicalMode(destinationInfo.Mode()) != record.Mode {
		return fmt.Errorf("verify materialized file %q: content or mode differs from validation", destinationRelative)
	}
	keep = true
	return nil
}

func requireDirectory(path, description string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a directory and not a symlink", description)
	}
	return nil
}

func ensureDirectory(ctx context.Context, root, relative string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	if err := requireDirectory(root, "destination bundle root"); err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := root
	for _, component := range strings.Split(relative, "/") {
		if err := ctx.Err(); err != nil {
			return err
		}
		current = filepath.Join(current, component)
		if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
		if err := requireDirectory(current, fmt.Sprintf("destination directory %q", relative)); err != nil {
			return err
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
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
		if strings.EqualFold(part, component) {
			return true
		}
	}
	return false
}

func validateOriginRelationship(ctx context.Context, projectRoot string, resource Resource, originRoot string, originBudget, comparisonBudget *indexBudget) error {
	originPath := resource.Origin.Path
	if err := rejectGitlinks(ctx, originRoot, []declaredRoot{{path: originPath, owner: resourceIdentity(resource)}}, fmt.Sprintf("origin %q", resource.Origin.ID)); err != nil {
		return err
	}
	originFiles, err := indexTree(ctx, originRoot, originPath, false, originBudget)
	if err != nil {
		return fmt.Errorf("resource %q origin path: %w", resourceIdentity(resource), err)
	}
	if resource.Origin.Relationship != RelationshipExactCopy {
		return nil
	}
	projectFiles, err := indexTree(ctx, projectRoot, resource.Source, true, comparisonBudget)
	if err != nil {
		return fmt.Errorf("resource %q source: %w", resourceIdentity(resource), err)
	}
	differences := exactCopyDifferences(projectFiles, resource.Source, originFiles, resource.Origin.Path)
	if len(differences) > 0 {
		reported := differences
		if len(reported) > maxExactCopyMismatchDetails {
			reported = reported[:maxExactCopyMismatchDetails]
		}
		return &exactCopyMismatchError{
			resource: resourceIdentity(resource), origin: resource.Origin.ID, originPath: originPath,
			differences: reported, total: len(differences),
		}
	}
	return nil
}

func declaredClosure(ctx context.Context, projectRoot string, manifest Manifest, manifestSize int64) ([]FileRecord, error) {
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
	if err := rejectGitlinks(ctx, projectRoot, unique, "Managed Pack Project"); err != nil {
		return nil, err
	}
	byPath := map[string]FileRecord{}
	budget := &indexBudget{entries: 1, totalSize: manifestSize}
	for _, root := range unique {
		indexed, err := indexTree(ctx, projectRoot, root.path, true, budget)
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

func indexTree(ctx context.Context, base, relative string, rejectGitMetadata bool, budget *indexBudget) ([]FileRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRelativePath(relative, true); err != nil {
		return nil, err
	}
	if err := rejectSymlinkComponents(ctx, base, relative); err != nil {
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
		if pathDepth(rel) > maxIndexedPathDepth {
			return fmt.Errorf("%q exceeds maximum path depth of %d", rel, maxIndexedPathDepth)
		}
		if err := budget.add(rel, info); err != nil {
			return err
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
		digest, err := digestFile(ctx, name, info)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{Path: rel, Mode: canonicalMode(info.Mode()), SHA256: digest})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func rejectSymlinkComponents(ctx context.Context, base, relative string) error {
	if relative == "." {
		return nil
	}
	current := base
	for _, component := range strings.Split(relative, "/") {
		if err := ctx.Err(); err != nil {
			return err
		}
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

func rejectGitlinks(ctx context.Context, repositoryRoot string, roots []declaredRoot, scope string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
		if err := ctx.Err(); err != nil {
			return err
		}
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

func exactCopyDifferences(project []FileRecord, projectRoot string, origin []FileRecord, originRoot string) []exactCopyDifference {
	projectByPath := relativeFileRecords(project, projectRoot)
	originByPath := relativeFileRecords(origin, originRoot)
	paths := make([]string, 0, len(projectByPath)+len(originByPath))
	seen := make(map[string]bool, len(projectByPath)+len(originByPath))
	for path := range projectByPath {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range originByPath {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	differences := make([]exactCopyDifference, 0)
	for _, path := range paths {
		projectFile, inProject := projectByPath[path]
		originFile, inOrigin := originByPath[path]
		switch {
		case !inProject:
			differences = append(differences, exactCopyDifference{class: "missing", path: path, projectSHA256: "-", originSHA256: originFile.SHA256})
		case !inOrigin:
			differences = append(differences, exactCopyDifference{class: "additional", path: path, projectSHA256: projectFile.SHA256, originSHA256: "-"})
		case projectFile.SHA256 != originFile.SHA256:
			differences = append(differences, exactCopyDifference{class: "changed", path: path, projectSHA256: projectFile.SHA256, originSHA256: originFile.SHA256})
		}
	}
	return differences
}

func relativeFileRecords(files []FileRecord, root string) map[string]FileRecord {
	result := make(map[string]FileRecord, len(files))
	for _, file := range files {
		relative, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(file.Path))
		if err != nil {
			continue
		}
		result[filepath.ToSlash(relative)] = file
	}
	return result
}

func boundedDiagnosticValue(value string) string {
	if len(value) <= maxExactCopyDiagnosticValueBytes {
		return value
	}
	return value[:maxExactCopyDiagnosticValueBytes] + "..."
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

func readFileBounded(ctx context.Context, path string, expected os.FileInfo) ([]byte, error) {
	var data bytes.Buffer
	data.Grow(int(expected.Size()))
	if err := copyRegularFile(ctx, path, expected, &data); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func digestFile(ctx context.Context, path string, expected os.FileInfo) (string, error) {
	hash := sha256.New()
	if err := copyRegularFile(ctx, path, expected, hash); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyRegularFile(ctx context.Context, path string, expected os.FileInfo, destination io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) || opened.Size() != expected.Size() || canonicalMode(opened.Mode()) != canonicalMode(expected.Mode()) {
		return fmt.Errorf("%q changed while being read", path)
	}
	written, err := io.Copy(destination, io.LimitReader(contextReader{ctx: ctx, reader: file}, maxIndexedFileBytes+1))
	if err != nil {
		return err
	}
	if written > maxIndexedFileBytes {
		return fmt.Errorf("%q exceeds maximum file size of %d bytes", path, maxIndexedFileBytes)
	}
	if written != opened.Size() {
		return fmt.Errorf("%q changed while being read", path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := reader.reader.Read(data)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return count, contextErr
	}
	return count, err
}

func pathDepth(path string) int {
	if path == "." {
		return 0
	}
	return strings.Count(path, "/") + 1
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

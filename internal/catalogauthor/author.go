// Package catalogauthor owns atomic, validated Catalog Project authoring.
package catalogauthor

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"unicode"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
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

// RefreshRequest describes one Upstream Refresh.
type RefreshRequest struct {
	ProjectRoot string
	PackID      string
	Version     string
	OriginID    string
	Commit      string
	Reconcile   func(AdaptationDiff) (bool, error)
}

// AdaptationDiff presents the upstream changes that a maintainer must
// explicitly reconcile with one maintained adaptation.
type AdaptationDiff struct {
	Resource string
	Changes  string
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

// RefreshResult identifies one validated Upstream Refresh.
type RefreshResult struct {
	PackID      string
	Version     string
	Repository  string
	OldCommit   string
	Commit      string
	ExactCopies int
	Adaptations int
	Packs       int
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
	stage, originalBundle, cleanup, err := stageBundle(request.ProjectRoot)
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
	if err := replaceBundleAtomically(request.ProjectRoot, stage, originalBundle); err != nil {
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
	stage, originalBundle, cleanup, err := stageBundle(request.ProjectRoot)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	manifestRelative := filepath.Join("bundle", "packs", request.PackID, "pack.json")
	stagedManifest := filepath.Join(stage, manifestRelative)
	_, manifest, err := readManifest(stagedManifest)
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

	if err := replaceBundleAtomically(request.ProjectRoot, stage, originalBundle); err != nil {
		return Result{}, fmt.Errorf("write prepared Catalog Project: %w", err)
	}
	return Result{
		PackID: manifest.ID, Version: manifest.Version,
		Resource:     request.Kind + ":" + request.ResourceID,
		Relationship: request.Relationship, Repository: request.Repository,
		Commit: request.Commit, Packs: len(validation.Packs),
	}, nil
}

// RefreshUpstream verifies exact copies and explicitly reconciles maintained
// adaptations before validating and atomically applying the prepared bundle.
func RefreshUpstream(ctx context.Context, request RefreshRequest, resolver managedpack.OriginResolver) (RefreshResult, error) {
	var result RefreshResult
	err := bundletransaction.WithExclusive(ctx, request.ProjectRoot, func() error {
		var err error
		result, err = refreshUpstream(ctx, request, resolver)
		return err
	})
	return result, err
}

func refreshUpstream(ctx context.Context, request RefreshRequest, resolver managedpack.OriginResolver) (RefreshResult, error) {
	if resolver == nil {
		return RefreshResult{}, fmt.Errorf("Upstream Refresh requires an exact-origin resolver")
	}
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return RefreshResult{}, fmt.Errorf("Catalog Project path is required")
	}
	if !authoringIDPattern.MatchString(request.PackID) {
		return RefreshResult{}, fmt.Errorf("Pack id must be lowercase kebab-case")
	}
	if !authoringIDPattern.MatchString(request.OriginID) {
		return RefreshResult{}, fmt.Errorf("origin id must be lowercase kebab-case")
	}
	if !authoringCommitPattern.MatchString(request.Commit) {
		return RefreshResult{}, fmt.Errorf("commit must be a full lowercase Git object ID")
	}
	if strings.TrimSpace(request.Version) == "" {
		return RefreshResult{}, fmt.Errorf("new Pack version is required")
	}

	stage, originalBundle, cleanup, err := stageBundle(request.ProjectRoot)
	if err != nil {
		return RefreshResult{}, err
	}
	defer cleanup()
	manifestPath := filepath.Join("bundle", "packs", request.PackID, "pack.json")
	stagedManifest := filepath.Join(stage, manifestPath)
	_, manifest, err := readManifest(stagedManifest)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("read Pack %q: %w", request.PackID, err)
	}

	originIndex := -1
	for i := range manifest.Origins {
		if manifest.Origins[i].ID == request.OriginID {
			originIndex = i
			break
		}
	}
	if originIndex < 0 {
		return RefreshResult{}, fmt.Errorf("Pack %q has no origin %q", request.PackID, request.OriginID)
	}
	oldOrigin := manifest.Origins[originIndex]
	if oldOrigin.Commit == request.Commit {
		return RefreshResult{}, fmt.Errorf("selected upstream commit is already pinned for origin %q", request.OriginID)
	}

	var exactCopies, adaptations []managedpack.Resource
	for _, resource := range manifest.Resources {
		if resource.Origin == nil || resource.Origin.ID != request.OriginID {
			continue
		}
		if resource.Origin.Relationship == managedpack.RelationshipAdapted {
			adaptations = append(adaptations, resource)
		}
		if resource.Origin.Relationship == managedpack.RelationshipExactCopy {
			exactCopies = append(exactCopies, resource)
		}
	}
	if len(exactCopies) == 0 && len(adaptations) == 0 {
		return RefreshResult{}, fmt.Errorf("origin %q has no resources", request.OriginID)
	}

	if _, err := managedpack.ValidateCatalogProject(ctx, request.ProjectRoot, "", resolver); err != nil {
		return RefreshResult{}, fmt.Errorf("verify current Catalog Project before Upstream Refresh: %w", err)
	}
	newOrigin := oldOrigin
	newOrigin.Commit = request.Commit
	newOrigin.Revision = ""
	newRoot, err := resolver.Resolve(ctx, newOrigin)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("resolve selected upstream commit: %w", err)
	}
	if strings.TrimSpace(newRoot) == "" {
		return RefreshResult{}, fmt.Errorf("resolve selected upstream commit: empty local root")
	}
	oldRoot, err := resolver.Resolve(ctx, oldOrigin)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("resolve previously pinned upstream commit: %w", err)
	}
	if strings.TrimSpace(oldRoot) == "" {
		return RefreshResult{}, fmt.Errorf("resolve previously pinned upstream commit: empty local root")
	}

	for _, resource := range adaptations {
		identity := resource.Kind + ":" + resource.ID
		changes, err := adaptationChanges(
			filepath.Join(oldRoot, filepath.FromSlash(resource.Origin.Path)),
			filepath.Join(newRoot, filepath.FromSlash(resource.Origin.Path)),
		)
		if err != nil {
			return RefreshResult{}, fmt.Errorf("compare adapted resource %q upstream revisions: %w", identity, err)
		}
		if request.Reconcile == nil {
			return RefreshResult{}, fmt.Errorf("origin %q includes adapted resource %q; Upstream Refresh requires explicit reconciliation", request.OriginID, identity)
		}
		accepted, err := request.Reconcile(AdaptationDiff{Resource: identity, Changes: changes})
		if err != nil {
			return RefreshResult{}, fmt.Errorf("reconcile adapted resource %q: %w", identity, err)
		}
		if !accepted {
			return RefreshResult{}, fmt.Errorf("adapted resource %q remains unresolved; Upstream Refresh requires explicit reconciliation and was not applied", identity)
		}
	}

	for _, resource := range exactCopies {
		destination := filepath.Join(stage, "bundle", filepath.FromSlash(resource.Source))
		if err := os.RemoveAll(destination); err != nil {
			return RefreshResult{}, fmt.Errorf("prepare exact-copy resource %q: %w", resource.Kind+":"+resource.ID, err)
		}
		source := filepath.Join(newRoot, filepath.FromSlash(resource.Origin.Path))
		if err := copyPath(source, destination); err != nil {
			return RefreshResult{}, fmt.Errorf("prepare exact-copy resource %q from origin path %q: %w", resource.Kind+":"+resource.ID, resource.Origin.Path, err)
		}
	}
	manifest.Version = request.Version
	manifest.Origins[originIndex] = newOrigin
	if err := writeManifest(stagedManifest, manifest); err != nil {
		return RefreshResult{}, err
	}
	validation, err := managedpack.ValidateCatalogProject(ctx, stage, request.ProjectRoot, resolver)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("validate prepared Catalog Project: %w", err)
	}
	if err := replaceBundleAtomically(request.ProjectRoot, stage, originalBundle); err != nil {
		return RefreshResult{}, fmt.Errorf("write prepared Catalog Project: %w", err)
	}
	return RefreshResult{
		PackID: manifest.ID, Version: manifest.Version, Repository: newOrigin.Repository,
		Commit: newOrigin.Commit, OldCommit: oldOrigin.Commit, ExactCopies: len(exactCopies), Adaptations: len(adaptations), Packs: len(validation.Packs),
	}, nil
}

const maximumAdaptationDiffBytes = 64 * 1024

func adaptationChanges(oldPath, newPath string) (string, error) {
	oldFiles, err := adaptationFiles(oldPath)
	if err != nil {
		return "", fmt.Errorf("read old upstream: %w", err)
	}
	newFiles, err := adaptationFiles(newPath)
	if err != nil {
		return "", fmt.Errorf("read new upstream: %w", err)
	}
	paths := make([]string, 0, len(oldFiles)+len(newFiles))
	seen := map[string]bool{}
	for path := range oldFiles {
		seen[path] = true
		paths = append(paths, path)
	}
	for path := range newFiles {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var report strings.Builder
	for _, path := range paths {
		oldFile, oldExists := oldFiles[path]
		newFile, newExists := newFiles[path]
		if oldExists && newExists && bytes.Equal(oldFile.data, newFile.data) && oldFile.mode == newFile.mode {
			continue
		}
		oldLabel, newLabel := "old/"+path, "new/"+path
		if !oldExists {
			oldLabel = "/dev/null"
		}
		if !newExists {
			newLabel = "/dev/null"
		}
		fmt.Fprintf(&report, "--- %s\n+++ %s\n", oldLabel, newLabel)
		switch {
		case !oldExists:
			fmt.Fprintf(&report, "new file mode %s\n", newFile.mode)
		case !newExists:
			fmt.Fprintf(&report, "deleted file mode %s\n", oldFile.mode)
		case oldFile.mode != newFile.mode:
			fmt.Fprintf(&report, "old mode %s\nnew mode %s\n", oldFile.mode, newFile.mode)
		}
		if textBytes(oldFile.data) && textBytes(newFile.data) {
			writeChangedLines(&report, oldFile.data, newFile.data)
		} else {
			fmt.Fprintf(&report, "-sha256:%x\n+sha256:%x\n", sha256.Sum256(oldFile.data), sha256.Sum256(newFile.data))
		}
		if report.Len() > maximumAdaptationDiffBytes {
			return "", fmt.Errorf("upstream differences exceed the %d-byte display limit; reconciliation cannot continue", maximumAdaptationDiffBytes)
		}
	}
	if report.Len() == 0 {
		return "(no upstream changes)\n", nil
	}
	return report.String(), nil
}

type adaptationFile struct {
	data []byte
	mode string
}

func adaptationFiles(root string) (map[string]adaptationFile, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks are not allowed: %s", root)
	}
	files := map[string]adaptationFile{}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, err
		}
		files[filepath.Base(root)] = adaptationFile{data: data, mode: canonicalGitFileMode(info.Mode())}
		return files, nil
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("only regular files and directories are supported: %s", root)
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("only regular files and directories are supported: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = adaptationFile{data: data, mode: canonicalGitFileMode(info.Mode())}
		return nil
	})
	return files, err
}

func canonicalGitFileMode(mode os.FileMode) string {
	if mode.Perm()&0o111 != 0 {
		return "100755"
	}
	return "100644"
}

func textBytes(data []byte) bool {
	return utf8.Valid(data) && !bytes.ContainsRune(data, 0)
}

func writeChangedLines(report *strings.Builder, oldData, newData []byte) {
	differ := diffmatchpatch.New()
	oldChars, newChars, lines := differ.DiffLinesToChars(string(oldData), string(newData))
	differences := differ.DiffCharsToLines(differ.DiffMain(oldChars, newChars, false), lines)
	for _, difference := range differences {
		prefix := ""
		switch difference.Type {
		case diffmatchpatch.DiffDelete:
			prefix = "-"
		case diffmatchpatch.DiffInsert:
			prefix = "+"
		default:
			continue
		}
		for _, line := range strings.SplitAfter(difference.Text, "\n") {
			if line == "" {
				continue
			}
			report.WriteString(prefix)
			for _, char := range strings.TrimSuffix(line, "\n") {
				if char == '\t' || !unicode.IsControl(char) {
					report.WriteRune(char)
					continue
				}
				fmt.Fprintf(report, "\\u%04x", char)
			}
			report.WriteByte('\n')
		}
		if difference.Text != "" && !strings.HasSuffix(difference.Text, "\n") {
			report.WriteString("\\ No newline at end of file\n")
		}
	}
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

func stageBundle(projectRoot string) (string, string, func(), error) {
	info, err := os.Lstat(projectRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", func() {}, fmt.Errorf("Catalog Project root must be an existing directory and not a symlink")
	}
	metadataRoot, err := gitMetadataRoot(projectRoot)
	if err != nil {
		return "", "", func() {}, err
	}
	stage, err := os.MkdirTemp(metadataRoot, "packy-catalog-author-")
	if err != nil {
		return "", "", func() {}, fmt.Errorf("create authoring stage: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	bundleRoot := filepath.Join(projectRoot, "bundle")
	original, err := treeIdentity(bundleRoot)
	if err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("inspect Catalog Project bundle: %w", err)
	}
	if err := copyPath(bundleRoot, filepath.Join(stage, "bundle")); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("stage Catalog Project bundle: %w", err)
	}
	current, err := treeIdentity(bundleRoot)
	if err != nil || current != original {
		cleanup()
		return "", "", func() {}, fmt.Errorf("Catalog Project changed while its bundle was staged")
	}
	return stage, original, cleanup, nil
}

func gitMetadataRoot(projectRoot string) (string, error) {
	path := filepath.Join(projectRoot, ".git")
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("Catalog Project must be a Git worktree: %w", err)
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return "", fmt.Errorf("Catalog Project .git entry is invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Catalog Project .git entry: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if !strings.HasPrefix(value, "gitdir: ") {
		return "", fmt.Errorf("Catalog Project .git entry is invalid")
	}
	metadata := strings.TrimSpace(strings.TrimPrefix(value, "gitdir: "))
	if !filepath.IsAbs(metadata) {
		metadata = filepath.Join(projectRoot, metadata)
	}
	metadataInfo, err := os.Lstat(metadata)
	if err != nil || !metadataInfo.IsDir() || metadataInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("Catalog Project Git metadata directory is unavailable")
	}
	return metadata, nil
}

func treeIdentity(root string) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(digest, "%s\x00%s\x00", filepath.ToSlash(relative), info.Mode().Type()|info.Mode().Perm())
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(digest, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func replaceBundleAtomically(projectRoot, stage, original string) error {
	bundleRoot := filepath.Join(projectRoot, "bundle")
	current, err := treeIdentity(bundleRoot)
	if err != nil {
		return err
	}
	if current != original {
		return fmt.Errorf("Catalog Project changed during preparation")
	}
	if err := exchangePaths(bundleRoot, filepath.Join(stage, "bundle")); err != nil {
		return fmt.Errorf("atomically exchange prepared bundle: %w", err)
	}
	return nil
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

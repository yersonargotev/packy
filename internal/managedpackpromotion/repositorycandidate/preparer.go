// Package repositorycandidate prepares fully gated local Packy proposal candidates.
package repositorycandidate

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
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	admissionRoot = "managed-packs/admissions"
	botName       = "Packy Promotion Bot"
	botEmail      = "packy-promotion@users.noreply.github.com"
)

type gates interface {
	GenerateDocs(context.Context, string) error
	ValidateResources(context.Context, string) error
	ValidateSuite(context.Context, string) error
}

type preparer struct {
	gates gates
}

// New returns the production CandidatePreparer. Its gates run only in the
// temporary detached clone and before the candidate commit is created.
func New() managedpackpromotion.CandidatePreparer {
	return newWithGates(productionGates{})
}

func newWithGates(value gates) managedpackpromotion.CandidatePreparer {
	return preparer{gates: value}
}

func (p preparer) Prepare(ctx context.Context, repositoryRoot string, acquisition managedpackpromotion.Acquisition, validation managedpack.Validation) (result managedpackpromotion.CandidatePreparation, resultErr error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if p.gates == nil {
		return result, fmt.Errorf("repository candidate gates are required")
	}
	coordinate, err := managedpackpromotion.ParseCoordinate(validation.Manifest.ID + "@" + validation.Manifest.Version)
	if err != nil {
		return result, fmt.Errorf("construct candidate coordinate: %w", err)
	}

	clone, baseSHA, err := cloneDetachedBase(ctx, repositoryRoot)
	if err != nil {
		return result, err
	}
	keep := false
	cleanup := func() error { return os.RemoveAll(filepath.Dir(clone)) }
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, cleanup())
		}
	}()

	record := buildAdmissionRecord(acquisition, validation)
	if _, err := managedpack.MarshalAdmissionRecord(record); err != nil {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateValidation, err.Error())
	}
	ownership, err := inspectOwnership(ctx, filepath.Join(clone, "bundle"))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		return result, managedpackpromotion.Reject(managedpackpromotion.GateOwnership, err.Error())
	}
	exact, err := exactAdmission(clone, record, validation, ownership)
	if err != nil {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateValidation, err.Error())
	}
	if exact {
		keep = true
		return managedpackpromotion.CandidatePreparation{
			NoChangeReason: fmt.Sprintf("%s is already admitted with exact release evidence and bytes", coordinate),
			Cleanup:        cleanup,
		}, nil
	}

	current, err := loadCurrentManifest(clone, coordinate.PackID)
	if err != nil {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateValidation, err.Error())
	}
	if err := enforceVersionFloor(current, validation.Manifest); err != nil {
		return result, err
	}

	changedBundlePaths, err := materializeCandidate(ctx, clone, acquisition.ProjectRoot, validation, ownership)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		return result, err
	}
	if _, err := managedpack.WriteAdmissionRecord(filepath.Join(clone, filepath.FromSlash(admissionRoot)), record); err != nil {
		return result, err
	}

	if err := p.gates.GenerateDocs(ctx, clone); err != nil {
		return result, gateError(ctx, managedpackpromotion.GateGeneratedDocs, err)
	}
	allowed := candidateAllowlist(coordinate, validation, changedBundlePaths)
	if err := enforceAllowlist(ctx, clone, allowed); err != nil {
		return result, err
	}
	if err := p.gates.ValidateResources(ctx, clone); err != nil {
		return result, gateError(ctx, managedpackpromotion.GateResourceSurfaces, err)
	}
	if err := p.gates.ValidateSuite(ctx, clone); err != nil {
		return result, gateError(ctx, managedpackpromotion.GatePackySuite, err)
	}
	if err := enforceAllowlist(ctx, clone, allowed); err != nil {
		return result, err
	}

	summary, err := candidateSummary(ctx, clone, acquisition, validation, current)
	if err != nil {
		return result, err
	}
	headSHA, treeSHA, err := commitDetachedCandidate(ctx, clone, coordinate, baseSHA)
	if err != nil {
		return result, err
	}
	identifier := candidateID(coordinate, acquisition.Release.Project, baseSHA, headSHA, treeSHA, summary)
	keep = true
	return managedpackpromotion.CandidatePreparation{
		Candidate: &managedpackpromotion.Candidate{
			ID: identifier, Summary: summary, Coordinate: coordinate, Project: acquisition.Release.Project,
			RepositoryRoot: clone, BaseSHA: baseSHA, HeadSHA: headSHA, ResultTreeSHA: treeSHA,
			Branch: "promote/" + coordinate.PackID + "-" + coordinate.Version,
		},
		Cleanup: cleanup,
	}, nil
}

func cloneDetachedBase(ctx context.Context, repositoryRoot string) (string, string, error) {
	baseSHA, err := gitText(ctx, repositoryRoot, "rev-parse", "refs/remotes/origin/main^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("resolve exact origin/main base: %w", err)
	}
	originURL, err := gitText(ctx, repositoryRoot, "remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("resolve Packy origin URL: %w", err)
	}
	temporary, err := os.MkdirTemp("", "packy-promotion-candidate-*")
	if err != nil {
		return "", "", fmt.Errorf("create candidate temporary directory: %w", err)
	}
	clone := filepath.Join(temporary, "repository")
	if _, err := run(ctx, "", nil, "git", "clone", "--local", "--no-hardlinks", "--no-checkout", repositoryRoot, clone); err != nil {
		_ = os.RemoveAll(temporary)
		return "", "", fmt.Errorf("clone Packy candidate repository: %w", err)
	}
	if _, err := run(ctx, clone, nil, "git", "checkout", "--detach", baseSHA); err != nil {
		_ = os.RemoveAll(temporary)
		return "", "", fmt.Errorf("check out exact detached Packy base: %w", err)
	}
	if _, err := run(ctx, clone, nil, "git", "remote", "set-url", "origin", originURL); err != nil {
		_ = os.RemoveAll(temporary)
		return "", "", fmt.Errorf("restore candidate origin URL: %w", err)
	}
	return clone, baseSHA, nil
}

func buildAdmissionRecord(acquisition managedpackpromotion.Acquisition, validation managedpack.Validation) managedpack.AdmissionRecord {
	objects := make([]managedpack.TagObject, len(acquisition.Release.TagObjects))
	for i, object := range acquisition.Release.TagObjects {
		objects[i] = managedpack.TagObject{SHA: object.SHA, TargetSHA: object.TargetSHA, TargetType: string(object.TargetType)}
	}
	if objects == nil {
		objects = []managedpack.TagObject{}
	}
	return managedpack.AdmissionRecord{
		SchemaVersion: managedpack.SchemaVersion, PackID: validation.Manifest.ID, PackVersion: validation.Manifest.Version,
		Project: acquisition.Release.Project, RepositoryID: acquisition.Release.RepositoryID, ReleaseID: acquisition.Release.ReleaseID,
		ReleaseImmutable: acquisition.Release.Immutable, Tag: acquisition.Release.Tag,
		TagRefType: string(acquisition.Release.TagRef.Type), TagRefSHA: acquisition.Release.TagRef.SHA, TagObjects: objects,
		Commit: acquisition.Release.CommitSHA, RootTree: acquisition.Release.RootTreeSHA,
		ManifestSHA256: validation.ManifestSHA256, ClosureSHA256: validation.ClosureSHA256,
		Files: append([]managedpack.FileRecord(nil), validation.Files...),
	}
}

func exactAdmission(repositoryRoot string, want managedpack.AdmissionRecord, validation managedpack.Validation, ownership bundleOwnership) (bool, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(admissionRoot), want.PackID, want.PackVersion+".json")
	got, err := managedpack.LoadAdmissionRecord(path)
	if errors.Is(err, os.ErrNotExist) || err != nil && strings.Contains(err.Error(), "no such file or directory") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load existing coordinate admission: %w", err)
	}
	if !reflect.DeepEqual(got, want) {
		return false, nil
	}
	candidatePaths := map[string]bool{}
	for _, file := range validation.Files {
		relative := file.Path
		if relative == "pack.json" {
			relative = filepath.ToSlash(filepath.Join("packs", want.PackID, "pack.json"))
		} else {
			candidatePaths[relative] = true
		}
		state, err := inspectFile(filepath.Join(repositoryRoot, "bundle", filepath.FromSlash(relative)))
		if err != nil || state.mode != file.Mode || state.sha256 != file.SHA256 {
			return false, nil
		}
	}
	for path, owners := range ownership.owners {
		if !owners[want.PackID] || candidatePaths[path] {
			continue
		}
		shared := false
		for owner := range owners {
			if owner != want.PackID {
				shared = true
			}
		}
		if !shared {
			return false, nil
		}
	}
	return true, nil
}

func loadCurrentManifest(repositoryRoot, packID string) (managedpack.Manifest, error) {
	path := filepath.Join(repositoryRoot, "bundle", "packs", packID, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return managedpack.Manifest{}, fmt.Errorf("read current Pack manifest: %w", err)
	}
	var manifest managedpack.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return managedpack.Manifest{}, fmt.Errorf("decode current Pack manifest: %w", err)
	}
	if manifest.ID != packID {
		return managedpack.Manifest{}, fmt.Errorf("current Pack manifest ID %q does not match %q", manifest.ID, packID)
	}
	return manifest, nil
}

func enforceVersionFloor(current, candidate managedpack.Manifest) error {
	from, err := semver.StrictNewVersion(current.Version)
	if err != nil {
		return managedpackpromotion.Reject(managedpackpromotion.GateSemVer, fmt.Sprintf("current Pack version %q is not SemVer", current.Version))
	}
	to, err := semver.StrictNewVersion(candidate.Version)
	if err != nil {
		return managedpackpromotion.Reject(managedpackpromotion.GateSemVer, fmt.Sprintf("candidate Pack version %q is not SemVer", candidate.Version))
	}
	if !to.GreaterThan(from) {
		return managedpackpromotion.Reject(managedpackpromotion.GateSemVer, fmt.Sprintf("candidate version %s must be greater than current version %s", to, from))
	}
	floor, reason := compatibilityFloor(current, candidate)
	actual := changeLevel(from, to)
	if actual < floor {
		return managedpackpromotion.Reject(managedpackpromotion.GateCompatibilityFloor, fmt.Sprintf("%s requires at least a %s version increment; %s to %s is %s", reason, floor, from, to, actual))
	}
	return nil
}

type versionLevel int

const (
	patchLevel versionLevel = iota + 1
	minorLevel
	majorLevel
)

func (level versionLevel) String() string {
	return [...]string{"", "patch", "minor", "major"}[level]
}

func changeLevel(from, to *semver.Version) versionLevel {
	if to.Major() > from.Major() {
		return majorLevel
	}
	if to.Minor() > from.Minor() {
		return minorLevel
	}
	return patchLevel
}

func compatibilityFloor(current, candidate managedpack.Manifest) (versionLevel, string) {
	oldSurfaces := stringSet(current.Surfaces)
	newSurfaces := stringSet(candidate.Surfaces)
	if missingKey(oldSurfaces, newSurfaces) != "" {
		return majorLevel, "removing a supported surface"
	}
	oldReadiness := stringSet(current.ReadinessObligations)
	newReadiness := stringSet(candidate.ReadinessObligations)
	oldRequirements := stringSet(current.ExternalRequirements)
	newRequirements := stringSet(candidate.ExternalRequirements)
	if missingKey(oldReadiness, newReadiness) != "" || current.Selectable != candidate.Selectable {
		return majorLevel, "removing or breaking a Pack contract"
	}
	if missingKey(newRequirements, oldRequirements) != "" {
		return majorLevel, "adding a mandatory external requirement"
	}
	oldResources := resourceMap(current.Resources)
	newResources := resourceMap(candidate.Resources)
	if missingKey(oldResources, newResources) != "" {
		return majorLevel, "removing a resource"
	}
	for identity, oldResource := range oldResources {
		change := compareResourceContract(oldResource, newResources[identity])
		if change == majorLevel {
			return majorLevel, "breaking an existing resource contract"
		}
	}
	resourcesAdded := false
	for identity, resource := range newResources {
		if _, exists := oldResources[identity]; exists {
			continue
		}
		if resourceHasMandatoryContract(resource) {
			return majorLevel, "adding a resource with a mandatory graph, requirement, authority, or surface capability"
		}
		resourcesAdded = true
	}
	if missingKey(newSurfaces, oldSurfaces) != "" || missingKey(newReadiness, oldReadiness) != "" || resourcesAdded {
		return minorLevel, "adding to the Pack or an existing resource contract"
	}
	return patchLevel, "changing existing Pack content or metadata"
}

func resourceHasMandatoryContract(resource managedpack.Resource) bool {
	if len(resource.Requires)+len(resource.Conflicts)+len(resource.Tools)+len(resource.Permissions) > 0 {
		return true
	}
	for _, binding := range resource.Bindings {
		if len(binding.Capabilities) > 0 {
			return true
		}
	}
	return false
}

func stringSet[T ~string](values []T) map[string]T {
	result := make(map[string]T, len(values))
	for _, value := range values {
		result[string(value)] = value
	}
	return result
}

func resourceMap(resources []managedpack.Resource) map[string]managedpack.Resource {
	result := make(map[string]managedpack.Resource, len(resources))
	for _, resource := range resources {
		result[resource.Kind+":"+resource.ID] = resource
	}
	return result
}

func missingKey[A, B any](left map[string]A, right map[string]B) string {
	for key := range left {
		if _, ok := right[key]; !ok {
			return key
		}
	}
	return ""
}

func compareResourceContract(left, right managedpack.Resource) versionLevel {
	if missingKey(stringSet(right.Requires), stringSet(left.Requires)) != "" ||
		missingKey(stringSet(right.Conflicts), stringSet(left.Conflicts)) != "" ||
		resourceProjectionChanged(left, right) {
		return majorLevel
	}
	return patchLevel
}

func resourceProjectionChanged(left, right managedpack.Resource) bool {
	return left.Source != right.Source || left.Command != right.Command ||
		!reflect.DeepEqual(left.Args, right.Args) || left.Mode != right.Mode ||
		!reflect.DeepEqual(left.Tools, right.Tools) || !reflect.DeepEqual(left.Permissions, right.Permissions) ||
		!reflect.DeepEqual(left.Bindings, right.Bindings) || !reflect.DeepEqual(left.Arguments, right.Arguments) ||
		!reflect.DeepEqual(left.SurfaceExclusions, right.SurfaceExclusions)
}

func candidateAllowlist(coordinate managedpackpromotion.Coordinate, validation managedpack.Validation, changedBundlePaths map[string]bool) map[string]bool {
	allowed := map[string]bool{
		filepath.ToSlash(filepath.Join(admissionRoot, coordinate.PackID, coordinate.Version+".json")): true,
		filepath.ToSlash(filepath.Join("docs", "packs", coordinate.PackID+".md")):                     true,
		"docs/packs/index.md": true,
	}
	for path := range changedBundlePaths {
		allowed[filepath.ToSlash(filepath.Join("bundle", path))] = true
	}
	for _, file := range validation.Files {
		path := file.Path
		if path == "pack.json" {
			path = filepath.ToSlash(filepath.Join("packs", coordinate.PackID, "pack.json"))
		}
		allowed[filepath.ToSlash(filepath.Join("bundle", path))] = true
	}
	return allowed
}

func enforceAllowlist(ctx context.Context, repositoryRoot string, allowed map[string]bool) error {
	changed, err := gitPaths(ctx, repositoryRoot, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return fmt.Errorf("inspect candidate changes: %w", err)
	}
	untracked, err := gitPaths(ctx, repositoryRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("inspect candidate untracked files: %w", err)
	}
	changed = append(changed, untracked...)
	sort.Strings(changed)
	for _, path := range changed {
		if !allowed[path] {
			return managedpackpromotion.Reject(managedpackpromotion.GateValidation, fmt.Sprintf("candidate changed non-allowlisted path %q", path))
		}
	}
	return nil
}

func commitDetachedCandidate(ctx context.Context, repositoryRoot string, coordinate managedpackpromotion.Coordinate, baseSHA string) (string, string, error) {
	if _, err := run(ctx, repositoryRoot, nil, "git", "add", "--all"); err != nil {
		return "", "", fmt.Errorf("stage candidate: %w", err)
	}
	if _, err := run(ctx, repositoryRoot, nil, "git", "diff", "--cached", "--quiet"); err == nil {
		return "", "", managedpackpromotion.Reject(managedpackpromotion.GateValidation, "candidate has no repository changes")
	}
	environment := []string{
		"GIT_AUTHOR_NAME=" + botName, "GIT_AUTHOR_EMAIL=" + botEmail,
		"GIT_COMMITTER_NAME=" + botName, "GIT_COMMITTER_EMAIL=" + botEmail,
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	}
	if _, err := run(ctx, repositoryRoot, environment, "git", "commit", "--no-gpg-sign", "-m", "Promote "+coordinate.String()); err != nil {
		return "", "", fmt.Errorf("commit detached candidate: %w", err)
	}
	headSHA, err := gitText(ctx, repositoryRoot, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("seal candidate head: %w", err)
	}
	parent, err := gitText(ctx, repositoryRoot, "rev-parse", "HEAD^1^{commit}")
	if err != nil || parent != baseSHA {
		return "", "", fmt.Errorf("candidate commit parent is not exact base %s", baseSHA)
	}
	treeSHA, err := gitText(ctx, repositoryRoot, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return "", "", fmt.Errorf("seal candidate tree: %w", err)
	}
	return headSHA, treeSHA, nil
}

func candidateID(coordinate managedpackpromotion.Coordinate, project, base, head, tree, summary string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{coordinate.String(), project, base, head, tree, summary}, "\n") + "\n"))
	return hex.EncodeToString(digest[:])
}

func candidateSummary(ctx context.Context, repositoryRoot string, acquisition managedpackpromotion.Acquisition, validation managedpack.Validation, current managedpack.Manifest) (string, error) {
	coordinate := validation.Manifest.ID + "@" + validation.Manifest.Version
	changes, err := changedPathSummary(ctx, repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("summarize candidate changes: %w", err)
	}
	chain := make([]string, len(acquisition.Release.TagObjects))
	for i, object := range acquisition.Release.TagObjects {
		chain[i] = fmt.Sprintf("%s -> %s (%s)", object.SHA, object.TargetSHA, object.TargetType)
	}
	if len(chain) == 0 {
		chain = []string{"lightweight tag directly references commit"}
	}
	origins := make([]string, len(validation.Manifest.Origins))
	for i, origin := range validation.Manifest.Origins {
		origins[i] = fmt.Sprintf("%s=%s@%s", origin.ID, origin.Repository, origin.Commit)
		if origin.Revision != "" {
			origins[i] += " (" + origin.Revision + ")"
		}
	}
	if len(origins) == 0 {
		origins = []string{"none"}
	}
	var adaptations, notices []string
	for _, resource := range validation.Manifest.Resources {
		identity := resource.Kind + ":" + resource.ID
		if resource.Origin != nil && resource.Origin.Relationship == managedpack.RelationshipAdapted {
			adaptations = append(adaptations, identity+" from "+resource.Origin.ID+":"+resource.Origin.Path)
		}
		for _, notice := range resource.Notices {
			notices = append(notices, identity+" -> "+notice)
		}
	}
	if len(adaptations) == 0 {
		adaptations = []string{"none"}
	}
	if len(notices) == 0 {
		notices = []string{"none"}
	}
	sort.Strings(adaptations)
	sort.Strings(notices)
	floor, floorReason := compatibilityFloor(current, validation.Manifest)
	return fmt.Sprintf("Promote `%s` from immutable Managed Pack release `%s`.\n\n"+
		"- Release identity: repository `%d`, release `%d`, tag `%s`, ref `%s` (`%s`)\n"+
		"- Tag chain: %s\n"+
		"- Peeled commit/tree: `%s` / `%s`\n"+
		"- Manifest/closure SHA-256: `%s` / `%s`\n"+
		"- Origins: %s\n"+
		"- Adaptations: %s\n"+
		"- Notice coverage: %s\n"+
		"- Compatibility floor: `%s` (%s); version `%s` → `%s`\n"+
		"- Repository changes: %s\n"+
		"- Admission gates: generated docs, resource/catalog fitness, and complete Packy suite passed before this detached commit.",
		coordinate, acquisition.Release.Project,
		acquisition.Release.RepositoryID, acquisition.Release.ReleaseID, acquisition.Release.Tag,
		acquisition.Release.TagRef.SHA, acquisition.Release.TagRef.Type,
		strings.Join(chain, "; "), acquisition.Release.CommitSHA, acquisition.Release.RootTreeSHA,
		validation.ManifestSHA256, validation.ClosureSHA256,
		strings.Join(origins, "; "), strings.Join(adaptations, "; "), strings.Join(notices, "; "),
		floor, floorReason, current.Version, validation.Manifest.Version, strings.Join(changes, "; "),
	), nil
}

func changedPathSummary(ctx context.Context, repositoryRoot string) ([]string, error) {
	output, err := run(ctx, repositoryRoot, nil, "git", "diff", "--name-status", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	var changes []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			changes = append(changes, strings.Join(strings.Fields(line), " "))
		}
	}
	untracked, err := gitPaths(ctx, repositoryRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, path := range untracked {
		changes = append(changes, "A "+path)
	}
	sort.Strings(changes)
	return changes, nil
}

func gateError(ctx context.Context, gate managedpackpromotion.Gate, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return managedpackpromotion.Reject(gate, err.Error())
}

func gitText(ctx context.Context, root string, arguments ...string) (string, error) {
	output, err := run(ctx, root, nil, "git", arguments...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitPaths(ctx context.Context, root string, arguments ...string) ([]string, error) {
	output, err := run(ctx, root, nil, "git", arguments...)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, path := range bytes.Split(output, []byte{0}) {
		if len(path) > 0 {
			result = append(result, string(path))
		}
	}
	return result, nil
}

func run(ctx context.Context, root string, environment []string, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = root
	command.Env = isolatedGitEnvironment(environment)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, message)
	}
	return output, nil
}

func isolatedGitEnvironment(additions []string) []string {
	values := map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
		"HOME":                filepath.Join(os.TempDir(), "packy-promotion-no-home"),
		"LANG":                "C",
		"LC_ALL":              "C",
		"PATH":                os.Getenv("PATH"),
		"TMPDIR":              os.TempDir(),
		"XDG_CONFIG_HOME":     filepath.Join(os.TempDir(), "packy-promotion-no-config"),
	}
	for _, entry := range additions {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			values[name] = value
		}
	}
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, name := range keys {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

type fileState struct {
	mode   string
	sha256 string
}

func inspectFile(path string) (fileState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fileState{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fileState{}, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fileState{}, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return fileState{}, err
	}
	mode := "100644"
	if info.Mode().Perm()&0o111 != 0 {
		mode = "100755"
	}
	return fileState{mode: mode, sha256: hex.EncodeToString(digest.Sum(nil))}, nil
}

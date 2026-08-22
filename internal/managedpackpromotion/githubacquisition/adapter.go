// Package githubacquisition acquires immutable Managed Pack releases and
// exact External Source Project revisions through Packy's read-only GitHub
// source. Upstream trees are copied as inert data and are never executed.
package githubacquisition

import (
	"bytes"
	"context"
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
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/packsync"
)

var (
	originIDPattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	repositoryPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	gitObjectIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

const maxManifestBytes = int64(8 << 20)

// Source is the smallest read-only boundary needed from githubsource.Client.
type Source interface {
	Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error)
	ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error)
	ResolveCommit(context.Context, packsync.SourceConfig, string) (packsync.Candidate, error)
	WithSnapshot(context.Context, packsync.Candidate, string, func(string) error) error
}

// Adapter implements managedpackpromotion.Acquirer with a read-only source.
type Adapter struct {
	source Source
}

func New(source Source) Adapter {
	return Adapter{source: source}
}

// Acquire returns durable local snapshots and exact release evidence. Every
// failure removes all partially acquired state before returning.
func (adapter Adapter) Acquire(ctx context.Context, project string, coordinate managedpackpromotion.Coordinate) (result managedpackpromotion.Acquisition, err error) {
	if adapter.source == nil {
		return result, errors.New("GitHub acquisition source is required")
	}
	if !repositoryPattern.MatchString(project) {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateRegistration, "registered Managed Pack Project must be an owner/name identity")
	}
	version, versionErr := semver.StrictNewVersion(coordinate.Version)
	if versionErr != nil || version.Prerelease() != "" {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "promotion coordinate must identify a stable SemVer")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	root, err := os.MkdirTemp("", "packy-managed-pack-acquisition-")
	if err != nil {
		return result, fmt.Errorf("create Managed Pack acquisition root: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			removeErr := os.RemoveAll(root)
			if removeErr != nil {
				err = errors.Join(err, fmt.Errorf("clean up failed Managed Pack acquisition: %w", removeErr))
			}
		}
	}()

	config := sourceConfig(project)
	releases, err := adapter.source.Releases(ctx, config)
	if err != nil {
		return result, fmt.Errorf("list Managed Pack releases: %w", err)
	}
	selected, err := selectRelease(releases, coordinate)
	if err != nil {
		return result, err
	}
	first, err := adapter.source.ResolveRelease(ctx, config, selected)
	if err != nil {
		return result, fmt.Errorf("resolve Managed Pack release: %w", err)
	}
	if reason := validateReleaseCandidate(first, project, selected); reason != "" {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateRelease, reason)
	}

	projectRoot := filepath.Join(root, "project")
	if err := adapter.snapshot(ctx, first, filepath.Join(root, "staging-project"), projectRoot); err != nil {
		return result, fmt.Errorf("acquire Managed Pack Project snapshot: %w", err)
	}
	origins, err := readOrigins(projectRoot)
	if err != nil {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateOrigins, err.Error())
	}
	originRoots := make(map[string]string, len(origins))
	for _, origin := range origins {
		candidate, resolveErr := adapter.source.ResolveCommit(ctx, sourceConfig(origin.Repository), origin.Commit)
		if resolveErr != nil {
			return result, fmt.Errorf("resolve origin %q exact commit: %w", origin.ID, resolveErr)
		}
		if reason := validateOriginCandidate(candidate, origin); reason != "" {
			return result, managedpackpromotion.Reject(managedpackpromotion.GateOrigins, reason)
		}
		destination := filepath.Join(root, "origins", origin.ID)
		if err := adapter.snapshot(ctx, candidate, filepath.Join(root, "staging-origin-"+origin.ID), destination); err != nil {
			return result, fmt.Errorf("acquire origin %q snapshot: %w", origin.ID, err)
		}
		finalOrigin, resolveErr := adapter.source.ResolveCommit(ctx, sourceConfig(origin.Repository), origin.Commit)
		if resolveErr != nil {
			return result, fmt.Errorf("re-resolve origin %q exact commit: %w", origin.ID, resolveErr)
		}
		if reason := validateOriginCandidate(finalOrigin, origin); reason != "" {
			return result, managedpackpromotion.Reject(managedpackpromotion.GateOrigins, reason)
		}
		if !sameOriginEvidence(candidate, finalOrigin) {
			return result, managedpackpromotion.Reject(managedpackpromotion.GateOrigins, fmt.Sprintf("origin %q repository or Git evidence moved during acquisition", origin.ID))
		}
		originRoots[origin.ID] = destination
	}

	final, err := adapter.source.ResolveRelease(ctx, config, selected)
	if err != nil {
		return result, fmt.Errorf("re-resolve Managed Pack release: %w", err)
	}
	if reason := validateReleaseCandidate(final, project, selected); reason != "" {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateRelease, reason)
	}
	if !sameReleaseEvidence(first, final) {
		return result, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release or Git tag evidence moved during acquisition")
	}

	var cleanupOnce sync.Once
	var cleanupErr error
	cleanup := func() error {
		cleanupOnce.Do(func() { cleanupErr = os.RemoveAll(root) })
		return cleanupErr
	}
	result = managedpackpromotion.Acquisition{
		Release: releaseEvidence(project, first), ProjectRoot: projectRoot,
		OriginRoots: originRoots, Cleanup: cleanup,
	}
	completed = true
	return result, nil
}

func sourceConfig(repository string) packsync.SourceConfig {
	return packsync.SourceConfig{Provider: "github", Repository: repository}
}

func selectRelease(releases []packsync.Release, coordinate managedpackpromotion.Coordinate) (packsync.Release, error) {
	want := "pack-v" + coordinate.Version
	matches := make([]packsync.Release, 0, 1)
	for _, release := range releases {
		if release.Tag == want {
			matches = append(matches, release)
		}
	}
	if len(matches) != 1 {
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, fmt.Sprintf("expected exactly one release tagged %q, found %d", want, len(matches)))
	}
	release := matches[0]
	switch {
	case release.ID <= 0:
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release ID must be positive")
	case release.PublishedAt.IsZero():
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release is not published")
	case release.Draft:
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release is a draft")
	case release.Prerelease:
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release is a prerelease")
	case !release.Immutable:
		return packsync.Release{}, managedpackpromotion.Reject(managedpackpromotion.GateRelease, "release does not report GitHub immutability")
	}
	return release, nil
}

func validateReleaseCandidate(candidate packsync.Candidate, project string, selected packsync.Release) string {
	if !strings.EqualFold(candidate.Repository, project) {
		return fmt.Sprintf("resolved repository %q does not match registered project %q", candidate.Repository, project)
	}
	if !candidate.Public {
		return "Managed Pack Project is not public"
	}
	if candidate.RepositoryID <= 0 {
		return "repository ID must be positive"
	}
	if candidate.Release == nil || !reflect.DeepEqual(*candidate.Release, selected) {
		return "resolved release identity does not match the selected release"
	}
	if candidate.TagRefName != "refs/tags/"+selected.Tag {
		return "resolved tag ref name does not match the selected release"
	}
	if !gitObjectIDPattern.MatchString(candidate.TagRefSHA) || !gitObjectIDPattern.MatchString(candidate.Commit) || !gitObjectIDPattern.MatchString(candidate.Tree) {
		return "release tag, commit, and tree must be full lowercase Git object IDs"
	}
	switch candidate.TagRefType {
	case "commit":
		if len(candidate.TagObjects) != 0 || candidate.TagRefSHA != candidate.Commit {
			return "lightweight release tag does not resolve directly to the acquired commit"
		}
	case "tag":
		if reason := validateTagChain(candidate); reason != "" {
			return reason
		}
	default:
		return fmt.Sprintf("release tag ref has unsupported object type %q", candidate.TagRefType)
	}
	return ""
}

func validateTagChain(candidate packsync.Candidate) string {
	if len(candidate.TagObjects) == 0 {
		return "annotated release tag has no tag object chain"
	}
	seen := map[string]bool{}
	for index, object := range candidate.TagObjects {
		if !gitObjectIDPattern.MatchString(object.SHA) || !gitObjectIDPattern.MatchString(object.TargetSHA) || seen[object.SHA] {
			return "annotated release tag chain is malformed"
		}
		seen[object.SHA] = true
		if index == 0 && object.SHA != candidate.TagRefSHA {
			return "release tag ref does not identify the first annotated tag object"
		}
		if index+1 < len(candidate.TagObjects) {
			if object.TargetType != "tag" || object.TargetSHA != candidate.TagObjects[index+1].SHA {
				return "annotated release tag chain moved before its commit"
			}
			continue
		}
		if object.TargetType != "commit" || object.TargetSHA != candidate.Commit {
			return "annotated release tag does not peel to the acquired commit"
		}
	}
	return ""
}

func releaseEvidence(project string, candidate packsync.Candidate) managedpackpromotion.Release {
	var tagObjects []managedpackpromotion.TagObject
	if candidate.TagObjects != nil {
		tagObjects = make([]managedpackpromotion.TagObject, len(candidate.TagObjects))
	}
	for index, object := range candidate.TagObjects {
		tagObjects[index] = managedpackpromotion.TagObject{SHA: object.SHA, TargetSHA: object.TargetSHA, TargetType: managedpackpromotion.GitObjectType(object.TargetType)}
	}
	return managedpackpromotion.Release{
		Project: project, RepositoryID: candidate.RepositoryID, ReleaseID: candidate.Release.ID,
		Public: candidate.Public, Published: !candidate.Release.PublishedAt.IsZero(),
		Stable: !candidate.Release.Draft && !candidate.Release.Prerelease,
		Draft:  candidate.Release.Draft, Prerelease: candidate.Release.Prerelease,
		Immutable: candidate.Release.Immutable, Tag: candidate.Release.Tag,
		TagRef:     managedpackpromotion.GitObject{SHA: candidate.TagRefSHA, Type: managedpackpromotion.GitObjectType(candidate.TagRefType)},
		TagObjects: tagObjects, CommitSHA: candidate.Commit, RootTreeSHA: candidate.Tree,
	}
}

func sameReleaseEvidence(first, final packsync.Candidate) bool {
	return strings.EqualFold(first.Repository, final.Repository) &&
		first.RepositoryID == final.RepositoryID && first.Public == final.Public &&
		reflect.DeepEqual(first.Release, final.Release) && first.TagRefName == final.TagRefName &&
		first.TagRefType == final.TagRefType && first.TagRefSHA == final.TagRefSHA &&
		reflect.DeepEqual(first.TagObjects, final.TagObjects) && first.Commit == final.Commit && first.Tree == final.Tree
}

func readOrigins(projectRoot string) ([]managedpack.Origin, error) {
	manifestPath := filepath.Join(projectRoot, "pack.json")
	manifest, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read root pack.json origins: %w", err)
	}
	defer manifest.Close()
	data, err := io.ReadAll(io.LimitReader(manifest, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read root pack.json origins: %w", err)
	}
	if int64(len(data)) > maxManifestBytes {
		return nil, fmt.Errorf("root pack.json exceeds maximum file size of %d bytes", maxManifestBytes)
	}
	var root struct {
		Origins json.RawMessage `json:"origins"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode root pack.json origins: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode root pack.json origins: %w", err)
	}
	if len(root.Origins) == 0 || bytes.Equal(bytes.TrimSpace(root.Origins), []byte("null")) {
		return nil, errors.New("root pack.json origins must be a non-null array")
	}
	var origins []managedpack.Origin
	originDecoder := json.NewDecoder(bytes.NewReader(root.Origins))
	originDecoder.DisallowUnknownFields()
	if err := originDecoder.Decode(&origins); err != nil {
		return nil, fmt.Errorf("decode root pack.json origins: %w", err)
	}
	if err := requireJSONEOF(originDecoder); err != nil {
		return nil, fmt.Errorf("decode root pack.json origins: %w", err)
	}
	if origins == nil {
		return nil, errors.New("root pack.json origins must be a non-null array")
	}
	seenRepositories := map[string]bool{}
	for index, origin := range origins {
		switch {
		case !originIDPattern.MatchString(origin.ID):
			return nil, fmt.Errorf("origin %q id must be lowercase kebab-case", origin.ID)
		case index > 0 && origins[index-1].ID >= origin.ID:
			return nil, errors.New("origins must be sorted by id without duplicates")
		case !repositoryPattern.MatchString(origin.Repository):
			return nil, fmt.Errorf("origin %q repository must be an owner/name identity", origin.ID)
		case seenRepositories[strings.ToLower(origin.Repository)]:
			return nil, fmt.Errorf("origin %q duplicates repository %q", origin.ID, origin.Repository)
		case !gitObjectIDPattern.MatchString(origin.Commit):
			return nil, fmt.Errorf("origin %q commit must be a full lowercase Git object ID", origin.ID)
		case origin.Revision != "" && strings.TrimSpace(origin.Revision) == "":
			return nil, fmt.Errorf("origin %q revision must not be blank", origin.ID)
		}
		seenRepositories[strings.ToLower(origin.Repository)] = true
	}
	return origins, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateOriginCandidate(candidate packsync.Candidate, origin managedpack.Origin) string {
	if !strings.EqualFold(candidate.Repository, origin.Repository) {
		return fmt.Sprintf("origin %q resolved repository %q instead of %q", origin.ID, candidate.Repository, origin.Repository)
	}
	if !candidate.Public {
		return fmt.Sprintf("origin %q External Source Project is not public", origin.ID)
	}
	if candidate.RepositoryID <= 0 {
		return fmt.Sprintf("origin %q repository ID must be positive", origin.ID)
	}
	if candidate.Commit != origin.Commit {
		return fmt.Sprintf("origin %q resolved commit %q instead of exact commit %q", origin.ID, candidate.Commit, origin.Commit)
	}
	if !gitObjectIDPattern.MatchString(candidate.Tree) {
		return fmt.Sprintf("origin %q root tree must be a full lowercase Git object ID", origin.ID)
	}
	return ""
}

func sameOriginEvidence(first, final packsync.Candidate) bool {
	return strings.EqualFold(first.Repository, final.Repository) &&
		first.RepositoryID == final.RepositoryID && first.Public == final.Public &&
		first.Commit == final.Commit && first.Tree == final.Tree
}

func (adapter Adapter) snapshot(ctx context.Context, candidate packsync.Candidate, stagingRoot, destination string) error {
	if err := os.Mkdir(stagingRoot, 0o700); err != nil {
		return err
	}
	err := adapter.source.WithSnapshot(ctx, candidate, stagingRoot, func(snapshot string) error {
		return copyInertTree(ctx, snapshot, destination)
	})
	removeErr := os.Remove(stagingRoot)
	if removeErr != nil {
		removeErr = fmt.Errorf("remove snapshot staging directory: %w", removeErr)
	}
	return errors.Join(err, removeErr)
}

func copyInertTree(ctx context.Context, source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("snapshot root must be a real directory")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	directories := []struct {
		path string
		mode os.FileMode
	}{{destination, info.Mode().Perm()}}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == source {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains forbidden symbolic link %q", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		switch {
		case entryInfo.IsDir():
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
			directories = append(directories, struct {
				path string
				mode os.FileMode
			}{target, entryInfo.Mode().Perm()})
			return nil
		case entryInfo.Mode().IsRegular():
			return copyRegularFile(path, target, entryInfo.Mode().Perm())
		default:
			return fmt.Errorf("snapshot contains forbidden non-regular entry %q", path)
		}
	})
	if err != nil {
		return err
	}
	sort.SliceStable(directories, func(i, j int) bool { return len(directories[i].path) > len(directories[j].path) })
	for _, directory := range directories {
		if err := os.Chmod(directory.path, directory.mode); err != nil {
			return err
		}
	}
	return nil
}

func copyRegularFile(source, destination string, mode os.FileMode) (result error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, input.Close()) }()
	openedInfo, err := input.Stat()
	if err != nil {
		return err
	}
	if !openedInfo.Mode().IsRegular() {
		return fmt.Errorf("snapshot entry changed while copying %q", source)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Chmod(destination, mode)
}

var _ managedpackpromotion.Acquirer = Adapter{}

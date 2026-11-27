// Package githubfake serves fixture GitHub catalogs over TLS so the real Packy
// CLI can consume Catalog Publications without the network.
//
// A fixture directory holds one directory per repository:
//
//	<root>/<owner>/<repo>/repository.json       optional Repository access and faults
//	<root>/<owner>/<repo>/release/release.json  the latest Release
//	<root>/<owner>/<repo>/release/<asset>       release asset bytes
//	<root>/<owner>/<repo>/releases/<tag>/       an earlier Release, laid out as release/
//
// The release asset API answers with a redirect to an unauthenticated asset
// host URL, as GitHub does.
//
// The handler reads the directory on every request, so a test or a person can
// edit a fixture while the server runs. Asset digests, sizes, and attestations
// derive from the bytes on disk.
package githubfake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	releaseDirectory = "release"
	earlierDirectory = "releases"
	releaseFile      = "release.json"
	repositoryFile   = "repository.json"
	publisher        = "github-actions[bot]"
)

// Repository is the optional repository.json of one fixture repository.
type Repository struct {
	// Token makes the repository private. Requests without it get 404, and
	// requests with another token get 401, as GitHub answers them.
	Token string `json:"token,omitempty"`
	// ReleaseStatus replaces the releases/latest response with this status.
	ReleaseStatus int `json:"release_status,omitempty"`
	// AttestationWorkflow is the signer workflow that served attestations name.
	// The default is <owner>/<repo>/.github/workflows/publish.yml.
	AttestationWorkflow string `json:"attestation_workflow,omitempty"`
}

// Release holds the GitHub release fields that the fake serves for one
// release. The fake adds the assets from the release directory.
type Release struct {
	Tag         string `json:"tag_name"`
	Commit      string `json:"target_commitish"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Immutable   bool   `json:"immutable"`
	PublishedAt string `json:"published_at"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

// AssetHost is the host that the release asset API redirects to, as GitHub's
// does.
const AssetHost = "release-assets.githubusercontent.com"

type asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
}

type attestation struct {
	SubjectSHA256  string `json:"subject_sha256"`
	SignerWorkflow string `json:"signer_workflow"`
}

// Publish makes the catalog-snapshot.tar.gz and SHA256SUMS in dist the latest
// stable, immutable release of repository, tagged catalog-<commit> and
// published by github-actions[bot]. The previous latest release stays
// available by its tag below releases/, and repository.json stays.
func Publish(root, repository, commit, dist string) error {
	directory := filepath.Join(root, filepath.FromSlash(repository))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(directory, ".release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, name := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(dist, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, name), data, 0o644); err != nil {
			return err
		}
	}
	release := Release{Tag: "catalog-" + commit, Commit: commit, Immutable: true, PublishedAt: "2026-01-01T00:00:00Z"}
	release.Author.Login = publisher
	if err := writeJSON(filepath.Join(stage, releaseFile), release); err != nil {
		return err
	}
	current := filepath.Join(directory, releaseDirectory)
	var previous Release
	switch err := readJSON(filepath.Join(current, releaseFile), &previous); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case previous.Tag != release.Tag && filepath.IsLocal(previous.Tag):
		earlier := filepath.Join(directory, earlierDirectory, previous.Tag)
		if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
			return err
		}
		if err := os.RemoveAll(earlier); err != nil {
			return err
		}
		if err := os.Rename(current, earlier); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(current); err != nil {
		return err
	}
	return os.Rename(stage, current)
}

// WriteRepository replaces the repository.json of a fixture repository and
// keeps its release.
func WriteRepository(root, repository string, value Repository) error {
	directory := filepath.Join(root, filepath.FromSlash(repository))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(directory, repositoryFile), value)
}

// Handler serves the fixture repositories below root for the api.github.com
// and asset hosts.
func Handler(root string) http.Handler {
	fixtures := fixtureRoot(root)
	mux := http.NewServeMux()
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}", fixtures.repository)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/releases/latest", fixtures.latestRelease)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/releases/tags/{tag}", fixtures.taggedRelease)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/attestations/{subject}", fixtures.attestations)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/releases/assets/{id}", fixtures.asset)
	mux.HandleFunc("GET "+AssetHost+"/{owner}/{repo}/{tag}/{asset}", fixtures.download)
	return mux
}

type fixtureRoot string

type fixture struct {
	name       string
	directory  string
	repository Repository
}

func (root fixtureRoot) open(w http.ResponseWriter, r *http.Request) (fixture, bool) {
	result, ok := root.find(w, r)
	if !ok {
		return fixture{}, false
	}
	if token := result.repository.Token; token != "" {
		switch authorization := r.Header.Get("Authorization"); authorization {
		case "Bearer " + token, "token " + token:
		case "":
			writeStatus(w, http.StatusNotFound)
			return fixture{}, false
		default:
			writeStatus(w, http.StatusUnauthorized)
			return fixture{}, false
		}
	}
	return result, true
}

func (root fixtureRoot) find(w http.ResponseWriter, r *http.Request) (fixture, bool) {
	name := r.PathValue("owner") + "/" + r.PathValue("repo")
	if !filepath.IsLocal(name) {
		writeStatus(w, http.StatusNotFound)
		return fixture{}, false
	}
	directory := filepath.Join(string(root), filepath.FromSlash(name))
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		writeStatus(w, http.StatusNotFound)
		return fixture{}, false
	}
	result := fixture{name: name, directory: directory}
	if err := readJSON(filepath.Join(directory, repositoryFile), &result.repository); err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return fixture{}, false
	}
	return result, true
}

type published struct {
	Release
	directory string
	assets    []asset
}

// releases returns the latest release first, then the earlier ones by tag.
// Asset IDs are unique across them, as GitHub's are.
func (f fixture) releases(w http.ResponseWriter) ([]published, bool) {
	directories := []string{filepath.Join(f.directory, releaseDirectory)}
	earlier, err := os.ReadDir(filepath.Join(f.directory, earlierDirectory))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	for _, entry := range earlier {
		if entry.IsDir() {
			directories = append(directories, filepath.Join(f.directory, earlierDirectory, entry.Name()))
		}
	}
	var result []published
	var id int64
	for _, directory := range directories {
		release := published{directory: directory, assets: []asset{}}
		if err := readJSON(filepath.Join(directory, releaseFile), &release.Release); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeStatus(w, http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return nil, false
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return nil, false
		}
		for _, entry := range entries {
			if entry.Name() == releaseFile || !entry.Type().IsRegular() {
				continue
			}
			id++
			data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil, false
			}
			sum := sha256.Sum256(data)
			release.assets = append(release.assets, asset{
				ID:     id,
				Name:   entry.Name(),
				Digest: "sha256:" + hex.EncodeToString(sum[:]),
				URL:    fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", f.name, id),
				Size:   int64(len(data)),
			})
		}
		result = append(result, release)
	}
	return result, true
}

func (root fixtureRoot) repository(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	visibility := "public"
	if f.repository.Token != "" {
		visibility = "private"
	}
	writeBody(w, map[string]any{"full_name": f.name, "private": visibility == "private", "visibility": visibility})
}

func (root fixtureRoot) latestRelease(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	if status := f.repository.ReleaseStatus; status != 0 {
		writeStatus(w, status)
		return
	}
	releases, ok := f.releases(w)
	if !ok {
		return
	}
	writeRelease(w, releases[0])
}

func (root fixtureRoot) taggedRelease(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	releases, ok := f.releases(w)
	if !ok {
		return
	}
	for _, release := range releases {
		if release.Tag == r.PathValue("tag") {
			writeRelease(w, release)
			return
		}
	}
	writeStatus(w, http.StatusNotFound)
}

func writeRelease(w http.ResponseWriter, release published) {
	writeBody(w, struct {
		Release
		Assets []asset `json:"assets"`
	}{release.Release, release.assets})
}

func (root fixtureRoot) attestations(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	releases, ok := f.releases(w)
	if !ok {
		return
	}
	workflow := f.repository.AttestationWorkflow
	if workflow == "" {
		workflow = f.name + "/.github/workflows/publish.yml"
	}
	for _, release := range releases {
		for _, candidate := range release.assets {
			if candidate.Digest == r.PathValue("subject") {
				subject := strings.TrimPrefix(candidate.Digest, "sha256:")
				writeBody(w, map[string]any{"attestations": []map[string]attestation{{"bundle": {SubjectSHA256: subject, SignerWorkflow: workflow}}}})
				return
			}
		}
	}
	writeStatus(w, http.StatusNotFound)
}

func (root fixtureRoot) asset(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	releases, ok := f.releases(w)
	if !ok {
		return
	}
	for _, release := range releases {
		for _, candidate := range release.assets {
			if strconv.FormatInt(candidate.ID, 10) != r.PathValue("id") {
				continue
			}
			if r.Header.Get("Accept") != "application/octet-stream" {
				writeBody(w, candidate)
				return
			}
			http.Redirect(w, r, "https://"+AssetHost+"/"+f.name+"/"+release.Tag+"/"+candidate.Name, http.StatusFound)
			return
		}
	}
	writeStatus(w, http.StatusNotFound)
}

// download serves the bytes behind a redirect. Like a signed URL, it needs no
// token, and like GitHub's asset host, it refuses a request that carries one.
func (root fixtureRoot) download(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "" {
		writeStatus(w, http.StatusBadRequest)
		return
	}
	f, ok := root.find(w, r)
	if !ok {
		return
	}
	releases, ok := f.releases(w)
	if !ok {
		return
	}
	for _, release := range releases {
		if release.Tag != r.PathValue("tag") {
			continue
		}
		for _, candidate := range release.assets {
			if candidate.Name == r.PathValue("asset") {
				http.ServeFile(w, r, filepath.Join(release.directory, candidate.Name))
				return
			}
		}
	}
	writeStatus(w, http.StatusNotFound)
}

func writeStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": http.StatusText(status)})
}

func writeBody(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Package githubfake serves fixture GitHub catalogs over TLS so the real Packy
// CLI can consume Catalog Publications without the network.
//
// A fixture directory holds one directory per repository:
//
//	<root>/<owner>/<repo>/repository.json       optional Repository access and faults
//	<root>/<owner>/<repo>/release/release.json  the latest Release
//	<root>/<owner>/<repo>/release/<asset>       release asset bytes
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
	"strings"
)

const (
	releaseDirectory = "release"
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

// Release holds the GitHub release fields that the fake serves for the latest
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

type asset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
}

type attestation struct {
	SubjectSHA256  string `json:"subject_sha256"`
	SignerWorkflow string `json:"signer_workflow"`
}

// Publish makes the catalog-snapshot.tar.gz and SHA256SUMS in dist the latest
// stable, immutable release of repository, tagged catalog-<commit> and
// published by github-actions[bot]. It replaces the previous release and keeps
// repository.json.
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
	if err := os.RemoveAll(current); err != nil {
		return err
	}
	return os.Rename(stage, current)
}

// WriteRepository writes the repository.json of a fixture repository.
func WriteRepository(root, repository string, value Repository) error {
	directory := filepath.Join(root, filepath.FromSlash(repository))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(directory, repositoryFile), value)
}

// Handler serves the fixture repositories below root for the api.github.com
// and github.com hosts.
func Handler(root string) http.Handler {
	fixtures := fixtureRoot(root)
	mux := http.NewServeMux()
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}", fixtures.repository)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/releases/latest", fixtures.latestRelease)
	mux.HandleFunc("GET api.github.com/repos/{owner}/{repo}/attestations/{subject}", fixtures.attestations)
	mux.HandleFunc("GET github.com/{owner}/{repo}/releases/download/{tag}/{asset}", fixtures.download)
	return mux
}

type fixtureRoot string

type fixture struct {
	name       string
	directory  string
	repository Repository
}

func (root fixtureRoot) open(w http.ResponseWriter, r *http.Request) (fixture, bool) {
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

func (f fixture) release(w http.ResponseWriter) (Release, []asset, bool) {
	var release Release
	if err := readJSON(filepath.Join(f.directory, releaseDirectory, releaseFile), &release); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeStatus(w, http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return Release{}, nil, false
	}
	entries, err := os.ReadDir(filepath.Join(f.directory, releaseDirectory))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return Release{}, nil, false
	}
	assets := []asset{}
	for _, entry := range entries {
		if entry.Name() == releaseFile || !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.directory, releaseDirectory, entry.Name()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return Release{}, nil, false
		}
		sum := sha256.Sum256(data)
		assets = append(assets, asset{
			Name:   entry.Name(),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
			URL:    "https://github.com/" + f.name + "/releases/download/" + release.Tag + "/" + entry.Name(),
			Size:   int64(len(data)),
		})
	}
	return release, assets, true
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
	release, assets, ok := f.release(w)
	if !ok {
		return
	}
	writeBody(w, struct {
		Release
		Assets []asset `json:"assets"`
	}{release, assets})
}

func (root fixtureRoot) attestations(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	_, assets, ok := f.release(w)
	if !ok {
		return
	}
	workflow := f.repository.AttestationWorkflow
	if workflow == "" {
		workflow = f.name + "/.github/workflows/publish.yml"
	}
	for _, candidate := range assets {
		if candidate.Digest == r.PathValue("subject") {
			subject := strings.TrimPrefix(candidate.Digest, "sha256:")
			writeBody(w, map[string]any{"attestations": []map[string]attestation{{"bundle": {SubjectSHA256: subject, SignerWorkflow: workflow}}}})
			return
		}
	}
	writeStatus(w, http.StatusNotFound)
}

func (root fixtureRoot) download(w http.ResponseWriter, r *http.Request) {
	f, ok := root.open(w, r)
	if !ok {
		return
	}
	release, assets, ok := f.release(w)
	if !ok {
		return
	}
	if r.PathValue("tag") == release.Tag {
		for _, candidate := range assets {
			if candidate.Name == r.PathValue("asset") {
				http.ServeFile(w, r, filepath.Join(f.directory, releaseDirectory, candidate.Name))
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

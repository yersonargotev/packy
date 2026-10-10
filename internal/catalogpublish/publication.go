// Package catalogpublish publishes prepared Catalog Snapshot assets through gh.
// It does not build Catalog content or issue artifact attestations.
package catalogpublish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

const archiveName = "catalog-snapshot.tar.gz"

var assetNames = []string{"SHA256SUMS", archiveName}
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}  catalog-snapshot\.tar\.gz\n$`)

type Request struct {
	Repository string
	Commit     string
	Dist       string
}

type Result struct {
	Outcome       string
	Repository    string
	Commit        string
	Tag           string
	ArchiveSHA256 string
}

// Error identifies the failed publication boundary without exposing credentials.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }
func failure(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

type release struct {
	Tag       string            `json:"tag_name"`
	Target    string            `json:"target_commitish"`
	Draft     *bool             `json:"draft"`
	Immutable *bool             `json:"immutable"`
	Assets    []json.RawMessage `json:"assets"`
}

type publisher struct {
	request  Request
	tag      string
	gh       string
	prepared string
}

// Publish accepts only unchanged prepared bytes and matching immutable releases.
// Official workflow preparation and attestation remain separate prerequisites.
func Publish(ctx context.Context, request Request) (Result, error) {
	if !repositoryPattern.MatchString(request.Repository) || !commitPattern.MatchString(request.Commit) || request.Dist == "" {
		return Result{}, failure("invalid_arguments", "publication requires --repository owner/repo, --commit full lowercase commit, and --dist directory")
	}
	staging, err := os.MkdirTemp("", "packy-catalog-publication-")
	if err != nil {
		return Result{}, failure("assets_invalid", "stage Catalog Snapshot: %v", err)
	}
	defer os.RemoveAll(staging)
	prepared := filepath.Join(staging, "prepared")
	if err := os.Mkdir(prepared, 0o700); err != nil {
		return Result{}, failure("assets_invalid", "stage Catalog Snapshot: %v", err)
	}
	for _, name := range assetNames {
		if err := copyRegularFile(filepath.Join(request.Dist, name), filepath.Join(prepared, name)); err != nil {
			return Result{}, failure("assets_invalid", "prepare %s: %v", name, err)
		}
	}
	checksum, err := os.ReadFile(filepath.Join(prepared, "SHA256SUMS"))
	if err != nil || !checksumPattern.Match(checksum) {
		return Result{}, failure("assets_invalid", "SHA256SUMS must contain exactly one SHA-256 entry for %s", archiveName)
	}
	digest, err := fileDigest(filepath.Join(prepared, archiveName))
	if err != nil {
		return Result{}, failure("assets_invalid", "hash Catalog Snapshot: %v", err)
	}
	if digest != string(checksum[:64]) {
		return Result{}, failure("assets_invalid", "Catalog Snapshot checksum mismatch")
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return Result{}, failure("transport_failed", "Catalog Publication requires authenticated gh on PATH: %v", err)
	}
	p := publisher{request: request, tag: "catalog-" + request.Commit, gh: gh, prepared: prepared}
	current, exists, err := p.fetchWithRetry(ctx)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		if _, err := p.run(ctx, "release", "create", p.tag, "--repo", request.Repository, "--target", request.Commit, "--title", p.tag, "--notes", "Immutable Catalog Snapshot for reviewed commit "+request.Commit+".", "--draft", "--latest=false"); err != nil {
			// A failed response may follow a successful create; resolve before retrying.
			if _, found, lookupErr := p.fetchWithRetry(ctx); lookupErr != nil {
				return Result{}, lookupErr
			} else if !found {
				return Result{}, err
			}
		}
		current, _, err = p.fetchWithRetry(ctx)
		if err != nil {
			return Result{}, err
		}
	}
	if err := p.verifyRelease(current); err != nil {
		return Result{}, err
	}
	published := filepath.Join(staging, "published")
	if err := os.Mkdir(published, 0o700); err != nil {
		return Result{}, failure("transport_failed", "stage published assets: %v", err)
	}
	if len(current.Assets) > 0 {
		if _, err := p.run(ctx, "release", "download", p.tag, "--repo", request.Repository, "--dir", published); err != nil {
			return Result{}, err
		}
	}
	if err := p.compareAssets(published, *current.Draft); err != nil {
		return Result{}, err
	}
	for _, name := range assetNames {
		if _, err := os.Lstat(filepath.Join(published, name)); os.IsNotExist(err) {
			if _, err := p.run(ctx, "release", "upload", p.tag, filepath.Join(prepared, name), "--repo", request.Repository); err != nil {
				return Result{}, err
			}
		}
	}
	if err := p.downloadAndCompare(ctx, published); err != nil {
		return Result{}, err
	}
	if *current.Draft {
		if _, err := p.run(ctx, "release", "edit", p.tag, "--repo", request.Repository, "--draft=false"); err != nil {
			return Result{}, err
		}
		final, found, err := p.fetchWithRetry(ctx)
		if err != nil {
			return Result{}, err
		}
		if !found {
			return Result{}, failure("transport_failed", "could not verify published Catalog Snapshot")
		}
		if err := p.verifyRelease(final); err != nil {
			return Result{}, err
		}
		if *final.Draft || !*final.Immutable {
			return Result{}, failure("publication_conflict", "published Catalog Snapshot is not immutable")
		}
		if err := p.downloadAndCompare(ctx, published); err != nil {
			return Result{}, err
		}
	}
	outcome := "unchanged"
	if !exists {
		outcome = "published"
	} else if *current.Draft {
		outcome = "resumed"
	}
	return Result{Outcome: outcome, Repository: request.Repository, Commit: request.Commit, Tag: p.tag, ArchiveSHA256: digest}, nil
}

func (p publisher) run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, p.gh, args...)
	// This production child intentionally inherits gh's authentication environment.
	output, err := command.Output()
	if err != nil {
		return nil, failure("transport_failed", "gh %s failed: %v; check authentication, repository permissions, and connectivity", args[0], err)
	}
	return output, nil
}

func (p publisher) fetch(ctx context.Context) (release, bool, error) {
	output, err := p.run(ctx, "api", "repos/"+p.request.Repository+"/releases/tags/"+p.tag)
	if err != nil {
		output, err = p.run(ctx, "api", "--paginate", "repos/"+p.request.Repository+"/releases?per_page=100", "--jq", `.[] | select(.tag_name == "`+p.tag+`")`)
		if err != nil {
			return release{}, false, failure("transport_failed", "could not determine Catalog Snapshot release state: %v", err)
		}
		if len(bytes.TrimSpace(output)) == 0 {
			return release{}, false, nil
		}
	}
	var value release
	if err := json.Unmarshal(output, &value); err != nil {
		return release{}, false, failure("publication_conflict", "published Catalog Snapshot metadata is malformed: %v", err)
	}
	return value, true, nil
}

func (p publisher) fetchWithRetry(ctx context.Context) (release, bool, error) {
	var value release
	var found bool
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		value, found, err = p.fetch(ctx)
		if found || (err != nil && err.(*Error).Code == "publication_conflict") {
			return value, found, err
		}
		if attempt < 4 {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return release{}, false, failure("transport_failed", "Catalog Publication canceled: %v", ctx.Err())
			case <-timer.C:
			}
		}
	}
	return value, found, err
}

func (p publisher) verifyRelease(value release) error {
	if value.Tag != p.tag {
		return failure("publication_conflict", "published Catalog Snapshot tag differs")
	}
	if value.Target != p.request.Commit {
		return failure("publication_conflict", "published Catalog Snapshot commit differs")
	}
	if value.Draft == nil || value.Immutable == nil {
		return failure("publication_conflict", "published Catalog Snapshot state is malformed")
	}
	if !*value.Draft && !*value.Immutable {
		return failure("publication_conflict", "published Catalog Snapshot is mutable")
	}
	return nil
}

func (p publisher) downloadAndCompare(ctx context.Context, directory string) error {
	if err := os.RemoveAll(directory); err != nil {
		return failure("transport_failed", "clear downloaded assets: %v", err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return failure("transport_failed", "stage downloaded assets: %v", err)
	}
	if _, err := p.run(ctx, "release", "download", p.tag, "--repo", p.request.Repository, "--dir", directory); err != nil {
		return err
	}
	return p.compareAssets(directory, false)
}

func (p publisher) compareAssets(directory string, allowMissing bool) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return failure("transport_failed", "read downloaded assets: %v", err)
	}
	for _, entry := range entries {
		if !slices.Contains(assetNames, entry.Name()) {
			return failure("publication_conflict", "published Catalog Snapshot contains unexpected asset %s", entry.Name())
		}
	}
	for _, name := range assetNames {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && allowMissing {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return failure("publication_conflict", "published Catalog Snapshot is missing regular asset %s", name)
		}
		// Compare digests of sealed local bytes and downloaded remote bytes.
		expected, err := fileDigest(filepath.Join(p.prepared, name))
		if err != nil {
			return failure("assets_invalid", "hash prepared asset: %v", err)
		}
		actual, err := fileDigest(path)
		if err != nil {
			return failure("transport_failed", "hash downloaded asset: %v", err)
		}
		if actual != expected {
			return failure("publication_conflict", "published Catalog Snapshot bytes differ for %s", name)
		}
	}
	return nil
}

func copyRegularFile(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

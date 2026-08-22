package githubsource

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- Git object identity is defined by SHA-1.
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/packsync"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type recursiveGitTree struct {
	SHA       string         `json:"sha"`
	Truncated bool           `json:"truncated"`
	Entries   []gitTreeEntry `json:"tree"`
}

type gitTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size *int64 `json:"size"`
}

// WithGitTreeSnapshot exposes an inert snapshot proven against Candidate.Tree.
// Unlike the legacy archive transport, it reads the recursive Git tree and
// every blob at the exact commit, verifies Git object identities, and never
// requests a generated tarball.
func (client *Client) WithGitTreeSnapshot(ctx context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) (result error) {
	if visit == nil {
		return errors.New("snapshot visitor is required")
	}
	if err := validateGitTreeCandidate(candidate); err != nil {
		return err
	}
	if err := emptyDirectory(temporaryRoot); err != nil {
		return err
	}
	defer func() {
		if cleanupErr := cleanDirectory(temporaryRoot); result == nil && cleanupErr != nil {
			result = cleanupErr
		}
	}()

	tree, err := client.readRecursiveGitTree(ctx, candidate)
	if err != nil {
		return err
	}
	if err := validateRecursiveGitTree(tree, candidate.Tree, client.archiveLimits); err != nil {
		return err
	}

	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := os.Mkdir(snapshot, 0o755); err != nil {
		return err
	}
	if err := client.materializeVerifiedGitTree(ctx, candidate, tree, snapshot); err != nil {
		return err
	}
	return visit(snapshot)
}

func validateGitTreeCandidate(candidate packsync.Candidate) error {
	parts := strings.Split(candidate.Repository, "/")
	if len(parts) != 2 || !safeRepositoryPart(parts[0]) || !safeRepositoryPart(parts[1]) {
		return errors.New("Git tree acquisition requires an owner/name repository identity")
	}
	if !fullGitObjectID(candidate.Commit) || !fullGitObjectID(candidate.Tree) {
		return errors.New("Git tree acquisition requires exact lowercase commit and tree object IDs")
	}
	return nil
}

func safeRepositoryPart(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func fullGitObjectID(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}

func (client *Client) readRecursiveGitTree(ctx context.Context, candidate packsync.Candidate) (recursiveGitTree, error) {
	endpoint := client.apiBase + "/repos/" + candidate.Repository + "/git/trees/" + candidate.Tree + "?recursive=1"
	request, err := client.request(ctx, endpoint)
	if err != nil {
		return recursiveGitTree{}, err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return recursiveGitTree{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return recursiveGitTree{}, newHTTPError("read exact recursive Git tree", response)
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: response.Body}, maxArchiveBytes+1))
	if err != nil {
		return recursiveGitTree{}, err
	}
	if len(data) > maxArchiveBytes {
		return recursiveGitTree{}, fmt.Errorf("recursive Git tree response exceeds %d bytes", maxArchiveBytes)
	}
	var tree recursiveGitTree
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&tree); err != nil {
		return recursiveGitTree{}, fmt.Errorf("decode recursive Git tree: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return recursiveGitTree{}, errors.New("decode recursive Git tree: multiple JSON values")
		}
		return recursiveGitTree{}, fmt.Errorf("decode recursive Git tree: %w", err)
	}
	return tree, nil
}

func validateRecursiveGitTree(tree recursiveGitTree, expectedRoot string, limits archiveLimits) error {
	if tree.SHA != expectedRoot {
		return fmt.Errorf("GitHub returned a different root tree: got %q, want %q", tree.SHA, expectedRoot)
	}
	if tree.Truncated {
		return errors.New("GitHub returned a truncated recursive Git tree")
	}
	if len(tree.Entries) > limits.maxEntries {
		return fmt.Errorf("Git tree entry count exceeds %d", limits.maxEntries)
	}
	seen := make(map[string]bool, len(tree.Entries))
	portablePaths := make(map[string]string, len(tree.Entries))
	caseFolder := cases.Fold()
	treePaths := map[string]bool{"": true}
	var declaredBytes int64
	for _, entry := range tree.Entries {
		if !safeArchivePath(entry.Path) || seen[entry.Path] {
			return fmt.Errorf("unsafe or duplicate Git tree path %q", entry.Path)
		}
		seen[entry.Path] = true
		portablePath := norm.NFC.String(caseFolder.String(norm.NFC.String(entry.Path)))
		if prior, exists := portablePaths[portablePath]; exists {
			return fmt.Errorf("portable path collision between %q and %q", prior, entry.Path)
		}
		portablePaths[portablePath] = entry.Path
		if depth := len(strings.Split(entry.Path, "/")); depth > limits.maxPathDepth {
			return fmt.Errorf("Git tree path depth exceeds %d for %q", limits.maxPathDepth, entry.Path)
		}
		if !fullGitObjectID(entry.SHA) {
			return fmt.Errorf("Git tree entry %q has malformed object ID", entry.Path)
		}
		switch {
		case entry.Type == "tree" && entry.Mode == "040000":
			treePaths[entry.Path] = true
			if entry.Size != nil {
				return fmt.Errorf("Git tree entry %q unexpectedly declares a size", entry.Path)
			}
		case entry.Type == "blob" && (entry.Mode == "100644" || entry.Mode == "100755" || entry.Mode == "120000"):
			if entry.Size == nil || *entry.Size < 0 {
				return fmt.Errorf("Git blob %q has no valid size", entry.Path)
			}
			if *entry.Size > limits.maxFileBytes {
				return fmt.Errorf("Git blob file size exceeds %d bytes for %s", limits.maxFileBytes, entry.Path)
			}
			if *entry.Size > math.MaxInt64-declaredBytes {
				return errors.New("Git tree expanded size overflows")
			}
			declaredBytes += *entry.Size
			if declaredBytes > limits.maxExpandedBytes {
				return fmt.Errorf("Git tree expanded size exceeds %d bytes", limits.maxExpandedBytes)
			}
		case entry.Type == "commit" && entry.Mode == "160000":
			if entry.Size != nil {
				return fmt.Errorf("Git submodule entry %q unexpectedly declares a size", entry.Path)
			}
		default:
			return fmt.Errorf("Git tree entry %q has unsupported type/mode %s/%s", entry.Path, entry.Type, entry.Mode)
		}
	}
	for _, entry := range tree.Entries {
		parent := path.Dir(entry.Path)
		if parent == "." {
			parent = ""
		}
		if !treePaths[parent] {
			return fmt.Errorf("Git tree entry %q has an absent or non-tree parent", entry.Path)
		}
	}
	return nil
}

func (client *Client) materializeVerifiedGitTree(ctx context.Context, candidate packsync.Candidate, tree recursiveGitTree, snapshot string) error {
	entries := append([]gitTreeEntry(nil), tree.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type != "tree" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(snapshot, filepath.FromSlash(entry.Path)), 0o755); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type != "blob" {
			continue
		}
		blob, err := client.readExactCommitBlob(ctx, candidate, entry)
		if err != nil {
			return err
		}
		if got := gitObjectSHA("blob", blob); got != entry.SHA {
			return fmt.Errorf("Git blob SHA mismatch for %q: got %s, want %s", entry.Path, got, entry.SHA)
		}
		if entry.Mode == "120000" {
			continue
		}
		target := filepath.Join(snapshot, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if entry.Mode == "100755" {
			mode = 0o755
		}
		if err := os.WriteFile(target, blob, mode); err != nil {
			return err
		}
		if err := os.Chmod(target, mode); err != nil {
			return err
		}
	}
	return verifyGitTreeObjects(tree)
}

func (client *Client) readExactCommitBlob(ctx context.Context, candidate packsync.Candidate, entry gitTreeEntry) ([]byte, error) {
	segments := strings.Split(entry.Path, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	// raw.githubusercontent.com serves the bytes addressed by exact commit and
	// path without spending one unauthenticated REST API request per file. The
	// verified recursive tree and blob hash prove those bytes are the declared
	// Git object, including for an inert symbolic link.
	endpoint := client.rawBase + "/" + candidate.Repository + "/" + candidate.Commit + "/" + strings.Join(segments, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "packy-pack-source-check")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, newHTTPError("read exact Git blob", response)
	}
	limit := *entry.Size
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: response.Body}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != limit {
		return nil, fmt.Errorf("Git blob %q size = %d, want %d", entry.Path, len(data), limit)
	}
	return data, nil
}

func verifyGitTreeObjects(tree recursiveGitTree) error {
	children := map[string][]gitTreeEntry{"": {}}
	expected := map[string]string{"": tree.SHA}
	for _, entry := range tree.Entries {
		parent := path.Dir(entry.Path)
		if parent == "." {
			parent = ""
		}
		child := entry
		child.Path = path.Base(entry.Path)
		children[parent] = append(children[parent], child)
		if entry.Type == "tree" {
			expected[entry.Path] = entry.SHA
			if _, ok := children[entry.Path]; !ok {
				children[entry.Path] = nil
			}
		}
	}
	paths := make([]string, 0, len(expected))
	for treePath := range expected {
		paths = append(paths, treePath)
	}
	sort.Slice(paths, func(i, j int) bool {
		leftDepth := strings.Count(paths[i], "/")
		rightDepth := strings.Count(paths[j], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return paths[i] > paths[j]
	})
	for _, treePath := range paths {
		got, err := gitTreeSHA(children[treePath])
		if err != nil {
			return err
		}
		if got != expected[treePath] {
			label := treePath
			if label == "" {
				label = "."
			}
			return fmt.Errorf("Git tree SHA mismatch for %q: got %s, want %s", label, got, expected[treePath])
		}
	}
	return nil
}

func gitTreeSHA(entries []gitTreeEntry) (string, error) {
	entries = append([]gitTreeEntry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i].Path
		if entries[i].Type == "tree" {
			left += "/"
		}
		right := entries[j].Path
		if entries[j].Type == "tree" {
			right += "/"
		}
		return left < right
	})
	var content bytes.Buffer
	for _, entry := range entries {
		mode := entry.Mode
		if mode == "040000" {
			mode = "40000"
		}
		objectID, err := hex.DecodeString(entry.SHA)
		if err != nil || len(objectID) != 20 {
			return "", fmt.Errorf("decode Git object ID for %q", entry.Path)
		}
		content.WriteString(mode)
		content.WriteByte(' ')
		content.WriteString(entry.Path)
		content.WriteByte(0)
		content.Write(objectID)
	}
	return gitObjectSHA("tree", content.Bytes()), nil
}

func gitObjectSHA(kind string, content []byte) string {
	hasher := sha1.New() // #nosec G401 -- Git object identity is defined by SHA-1.
	_, _ = io.WriteString(hasher, kind+" "+strconv.Itoa(len(content))+"\x00")
	_, _ = hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}

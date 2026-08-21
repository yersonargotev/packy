package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type pstackReview struct {
	Repository      string            `json:"repository"`
	Release         string            `json:"release"`
	Commit          string            `json:"commit"`
	InventorySHA256 string            `json:"inventory_sha256"`
	Counts          map[string]int    `json:"counts"`
	Resources       []pstackReviewRow `json:"resources"`
}

type pstackReviewRow struct {
	Path        string `json:"path"`
	Disposition string `json:"disposition"`
	Resource    string `json:"resource"`
	ExclusionID string `json:"exclusion_id"`
}

type pstackSourceLock struct {
	Candidate struct {
		Commit  string `json:"commit"`
		Release struct {
			Tag       string `json:"tag"`
			Immutable bool   `json:"immutable"`
		} `json:"release"`
	} `json:"candidate"`
	Resources []struct {
		Kind         string `json:"kind"`
		ResourceID   string `json:"resource_id"`
		UpstreamPath string `json:"upstream_path"`
		VendoredPath string `json:"vendored_path"`
		Files        []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	} `json:"resources"`
}

func TestCheckedInPstackPackMatchesReviewedRelease(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := Discover(context.Background(), bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show(context.Background(), "pstack")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "1.0.0" || pack.manifestVersion != manifestSchemaV4 || !pack.Selectable {
		t.Fatalf("pstack identity = version %q schema %d selectable %v", pack.Version, pack.manifestVersion, pack.Selectable)
	}
	if got, want := pack.Surfaces, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pstack surfaces = %v want %v", got, want)
	}
	if pack.SourceReference == nil || pack.SourceReference.Repository != "https://github.com/yersonargotev/pstack.git" || pack.SourceReference.Revision != "v0.1.1" {
		t.Fatalf("pstack source reference = %+v", pack.SourceReference)
	}
	if counts := pack.ResourceCounts(); counts.Skills != 26 || counts.Notices != 1 || len(pack.Resources) != 27 {
		t.Fatalf("pstack resource counts = %+v total=%d", counts, len(pack.Resources))
	}

	identities := map[string]bool{}
	for _, resource := range pack.Resources {
		identity := (ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String()
		identities[identity] = true
		if resource.Kind == "notice" {
			if identity != "notice:pstack-mit" || resource.License != "MIT" || resource.Attribution != "Copyright (c) 2026 Lauren Tan" {
				t.Fatalf("pstack notice = %+v", resource)
			}
			continue
		}
		if resource.Kind != "skill" || !reflect.DeepEqual(bindingSurfaces(resource.Bindings), pack.Surfaces) ||
			!reflect.DeepEqual(resource.Notices, []string{"notice:pstack-mit"}) || len(resource.Conflicts) != 0 || len(resource.SurfaceExclusions) != 0 {
			t.Fatalf("pstack skill %q contract = %+v", identity, resource)
		}
	}

	for _, surface := range pack.Surfaces {
		assertPstackSelectionTargets(t, pack, surface, ResourceSelection{Mode: SelectionAll})
		for _, resource := range pack.Resources {
			if resource.Kind != "skill" {
				continue
			}
			assertPstackSelectionTargets(t, pack, surface, ResourceSelection{
				Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: resource.Kind, ID: resource.ID}},
			})
		}
	}

	var review pstackReview
	readPstackJSON(t, filepath.Join("..", "..", "docs", "research", "evidence", "pstack-v0.1.1-resource-review.json"), &review)
	if review.Repository != "yersonargotev/pstack" || review.Release != "v0.1.1" || review.Commit != "ddb726b4bcd16ac21c8082153b62740ab21a23ab" || len(review.Resources) != 156 {
		t.Fatalf("pstack review identity = repository %q release %q commit %q resources %d", review.Repository, review.Release, review.Commit, len(review.Resources))
	}

	var lock pstackSourceLock
	readPstackJSON(t, filepath.Join(bundleRoot, "sources", "pstack-source.lock.json"), &lock)
	if lock.Candidate.Commit != review.Commit || lock.Candidate.Release.Tag != review.Release || !lock.Candidate.Release.Immutable || len(lock.Resources) != 27 {
		t.Fatalf("pstack lock candidate = %+v resources=%d", lock.Candidate, len(lock.Resources))
	}

	admitted := map[string]bool{}
	for _, resource := range lock.Resources {
		for _, file := range resource.Files {
			upstreamPath := resource.UpstreamPath
			if file.Path != "." {
				upstreamPath += "/" + file.Path
			}
			admitted[upstreamPath] = true
			currentPath := filepath.Join("..", "..", filepath.FromSlash(resource.VendoredPath))
			historyPath := filepath.Join(bundleRoot, "history", "pstack", "1.0.0", filepath.FromSlash(strings.TrimPrefix(resource.VendoredPath, "bundle/")))
			if file.Path != "." {
				currentPath = filepath.Join(currentPath, filepath.FromSlash(file.Path))
				historyPath = filepath.Join(historyPath, filepath.FromSlash(file.Path))
			}
			current := readPstackFile(t, currentPath)
			history := readPstackFile(t, historyPath)
			if !reflect.DeepEqual(current, history) {
				t.Fatalf("history differs from current resource %q", upstreamPath)
			}
			sum := sha256.Sum256(current)
			if hex.EncodeToString(sum[:]) != file.SHA256 {
				t.Fatalf("resource %q digest differs from source lock", upstreamPath)
			}
		}
	}

	actualCounts := map[string]int{
		"selected":        0,
		"dependency-only": 0,
		"adapted":         0,
		"excluded":        0,
	}
	paths := make([]string, 0, len(review.Resources))
	seen := map[string]bool{}
	for _, row := range review.Resources {
		if seen[row.Path] {
			t.Fatalf("duplicate reviewed path %q", row.Path)
		}
		seen[row.Path] = true
		paths = append(paths, row.Path)
		actualCounts[row.Disposition]++
		switch row.Disposition {
		case "selected", "dependency-only":
			if !admitted[row.Path] || !identities[row.Resource] {
				t.Fatalf("reviewed %s path %q resource %q is not admitted", row.Disposition, row.Path, row.Resource)
			}
		case "excluded":
			if admitted[row.Path] || !pstackExclusionMatches(pack.Contract.Exclusions, row.ExclusionID, row.Path) {
				t.Fatalf("excluded path %q is not covered by manifest exclusion %q", row.Path, row.ExclusionID)
			}
		case "adapted":
			t.Fatalf("initial pstack admission must not contain adapted upstream bytes: %q", row.Path)
		default:
			t.Fatalf("unknown disposition %q for %q", row.Disposition, row.Path)
		}
	}
	if !reflect.DeepEqual(actualCounts, review.Counts) || review.Counts["selected"] != 26 || review.Counts["dependency-only"] != 2 || review.Counts["excluded"] != 128 || review.Counts["adapted"] != 0 {
		t.Fatalf("pstack review counts = actual %#v recorded %#v", actualCounts, review.Counts)
	}
	for path := range admitted {
		if !seen[path] {
			t.Fatalf("source lock path %q is missing from the review", path)
		}
	}
	sort.Strings(paths)
	sum := sha256.Sum256([]byte(strings.Join(paths, "\n") + "\n"))
	if hex.EncodeToString(sum[:]) != review.InventorySHA256 {
		t.Fatalf("pstack inventory digest = %x want %s", sum, review.InventorySHA256)
	}

	currentManifest := readPstackFile(t, filepath.Join(bundleRoot, "packs", "pstack", "pack.json"))
	historyManifest := readPstackFile(t, filepath.Join(bundleRoot, "history", "pstack", "1.0.0", "pack.json"))
	if !reflect.DeepEqual(currentManifest, historyManifest) {
		t.Fatal("pstack history manifest differs from the admitted manifest")
	}
}

func assertPstackSelectionTargets(t *testing.T, pack Pack, surface Surface, selection ResourceSelection) {
	t.Helper()
	selected, err := selectPackResources(pack, selection)
	if err != nil {
		t.Fatalf("select pstack %s roots %+v: %v", surface, selection.Roots, err)
	}
	targets := map[string]string{}
	for _, resource := range selected.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			if previous := targets[binding.Name]; previous != "" {
				t.Fatalf("pstack %s selection has target collision %q between %s and %s", surface, binding.Name, previous, resource.ID)
			}
			targets[binding.Name] = resource.ID
		}
	}
}

func bindingSurfaces(bindings []Binding) []Surface {
	result := make([]Surface, len(bindings))
	for i, binding := range bindings {
		result[i] = binding.Surface
	}
	return result
}

func pstackExclusionMatches(exclusions []Exclusion, id, path string) bool {
	for _, exclusion := range exclusions {
		if exclusion.ID != id {
			continue
		}
		for _, pattern := range exclusion.SourcePaths {
			if strings.HasSuffix(pattern, "/**") && strings.HasPrefix(path, strings.TrimSuffix(pattern, "**")) || path == pattern {
				return true
			}
		}
	}
	return false
}

func readPstackJSON(t *testing.T, path string, target any) {
	t.Helper()
	if err := json.Unmarshal(readPstackFile(t, path), target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func readPstackFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

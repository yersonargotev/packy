package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackListJSONReportsValidatedCatalogInCanonicalOrder(t *testing.T) {
	opts, repositoryRoot := packListRepositoryOptions(t)

	output, err := executeCommand(t, NewRootCommand(opts), "list", "--json")
	if err != nil {
		t.Fatalf("pack list --json: %v\n%s", err, output)
	}
	var report struct {
		SchemaVersion int    `json:"schema_version"`
		Report        string `json:"report"`
		Packs         []struct {
			ID          string                   `json:"id"`
			Version     string                   `json:"version"`
			Description string                   `json:"description"`
			Surfaces    []capabilitypack.Surface `json:"surfaces"`
		} `json:"packs"`
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	if err := decoder.Decode(&report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, output)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("pack list emitted more than one JSON document: %v\n%s", err, output)
	}
	if report.SchemaVersion != 1 || report.Report != "pack-list" || report.Packs == nil {
		t.Fatalf("report header = %#v", report)
	}

	catalog, err := capabilitypack.Discover(context.Background(), filepath.Join(repositoryRoot, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := catalog.ListCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Packs) != len(want) {
		t.Fatalf("packs = %d, want %d", len(report.Packs), len(want))
	}
	for i, pack := range report.Packs {
		expected := want[i]
		if pack.ID != expected.ID || pack.Version != expected.Version || pack.Description != expected.Description || !reflect.DeepEqual(pack.Surfaces, expected.Surfaces) {
			t.Fatalf("pack %d = %#v, want id=%q version=%q description=%q surfaces=%v", i, pack, expected.ID, expected.Version, expected.Description, expected.Surfaces)
		}
		if i > 0 && report.Packs[i-1].ID >= pack.ID {
			t.Fatalf("packs are not in canonical ID order: %#v", report.Packs)
		}
		if !slices.IsSorted(pack.Surfaces) {
			t.Fatalf("surfaces are not in canonical order for %q: %v", pack.ID, pack.Surfaces)
		}
	}
}

func TestPackListJSONRepresentsAnEmptyCatalogWithAnEmptyArray(t *testing.T) {
	bundleRoot := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(filepath.Join(bundleRoot, "packs"), 0o700); err != nil {
		t.Fatal(err)
	}
	createSkillSourceAt(t, filepath.Join(bundleRoot, "skills"))
	home := t.TempDir()
	opts := Options{Env: MapEnv{
		"HOME":                home,
		"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
		"PATH":                "",
		"PACKY_SKILLS_SOURCE": filepath.Join(bundleRoot, "skills"),
	}}

	output, err := executeCommand(t, NewRootCommand(opts), "list", "--json")
	if err != nil {
		t.Fatalf("pack list --json: %v\n%s", err, output)
	}
	if output != "{\"schema_version\":1,\"report\":\"pack-list\",\"packs\":[]}\n" {
		t.Fatalf("empty report = %q", output)
	}
}

func TestPackListHumanOutputRemainsUnchanged(t *testing.T) {
	opts, _ := packListRepositoryOptions(t)
	mattyVersion := checkedInMattyFacts(t).Version

	output, err := executeCommand(t, NewRootCommand(opts), "list")
	if err != nil {
		t.Fatalf("pack list: %v\n%s", err, output)
	}
	want := "PACK            VERSION  DESCRIPTION                                                            AVAILABLE ON\n" +
		"addy            1.1.0    Addy agent skills                                                      claude, codex, opencode\n" +
		"argote          1.0.2    Yerson Argote's engineering and communication guidance                 claude, codex, opencode\n" +
		"engram          3.1.0    Upstream Engram CLI memory workflows for agent work                    codex\n" +
		"issue-delivery  1.1.1    Deliver issues through policy-driven or Matt-configured workflows      codex\n" +
		fmt.Sprintf("matty           %-7s  Matty workflow                                                         claude, codex, opencode\n", mattyVersion) +
		"orchestrate     1.0.1    Coordinate focused Codex subagents                                     codex\n" +
		"pstack          1.0.0    Apply pstack's reviewed portable engineering workflows and principles  claude, codex, opencode\n"
	if output != want {
		t.Fatalf("human output changed:\n%s", output)
	}
}

func packListRepositoryOptions(t *testing.T) (Options, string) {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return Options{Env: MapEnv{
		"HOME":                home,
		"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
		"PATH":                "",
		"PACKY_SKILLS_SOURCE": filepath.Join(repositoryRoot, "bundle", "skills"),
	}}, repositoryRoot
}

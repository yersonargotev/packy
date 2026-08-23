package ci_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIssue672EngramPackPreservesReviewedRuntimeContract(t *testing.T) {
	root := repositoryRoot(t)
	var manifest struct {
		Version              string   `json:"version"`
		Surfaces             []string `json:"surfaces"`
		ReadinessObligations []string `json:"readiness_obligations"`
		ExternalRequirements []string `json:"external_requirements"`
		Resources            []struct {
			Kind        string   `json:"kind"`
			ID          string   `json:"id"`
			Source      string   `json:"source"`
			Notices     []string `json:"notices"`
			Attribution string   `json:"attribution"`
			License     string   `json:"license"`
			Bindings    []struct {
				Surface      string `json:"surface"`
				Projection   string `json:"projection"`
				Name         string `json:"name"`
				Invocation   string `json:"invocation"`
				Capabilities []struct {
					Type                          string `json:"type"`
					ExternalExecutableAcquisition struct {
						Tool string `json:"tool"`
					} `json:"external_executable_acquisition"`
				} `json:"capabilities"`
			} `json:"bindings"`
		} `json:"resources"`
	}
	manifestData, err := os.ReadFile(filepath.Join(root, "bundle", "packs", "engram", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" {
		t.Fatalf("Engram generation identity = %#v", manifest)
	}
	if !reflect.DeepEqual(manifest.Surfaces, []string{"codex"}) || !reflect.DeepEqual(manifest.ReadinessObligations, []string{"runtime-usability", "surface-authorization"}) || !reflect.DeepEqual(manifest.ExternalRequirements, []string{"engram"}) {
		t.Fatalf("Engram runtime contract = %#v", manifest)
	}
	if len(manifest.Resources) != 2 {
		t.Fatalf("Engram resources = %#v; want one notice and one skill", manifest.Resources)
	}
	notice, skill := manifest.Resources[0], manifest.Resources[1]
	if notice.Kind != "notice" || notice.ID != "mit" || notice.Source != "notices/engram-mit" || notice.License != "MIT" || notice.Attribution != "Copyright (c) 2026 Alan Buscaglia" {
		t.Fatalf("Engram legal notice = %#v", notice)
	}
	if skill.Kind != "skill" || skill.ID != "engram-memory-cli" || skill.Source != "skills/engram-memory-cli" || !reflect.DeepEqual(skill.Notices, []string{"notice:mit"}) || len(skill.Bindings) != 1 {
		t.Fatalf("Engram skill = %#v", skill)
	}
	binding := skill.Bindings[0]
	if binding.Surface != "codex" || binding.Projection != "skill" || binding.Name != "engram-memory-cli" || binding.Invocation != "$engram-memory-cli" || len(binding.Capabilities) != 1 || binding.Capabilities[0].Type != "external-executable-acquisition" || binding.Capabilities[0].ExternalExecutableAcquisition.Tool != "engram" {
		t.Fatalf("Engram skill binding = %#v", binding)
	}

	wantFiles := map[string]bool{
		"SKILL.md":               true,
		"agents/openai.yaml":     true,
		"references/curation.md": true,
	}
	skillRoot := filepath.Join(root, "bundle", "skills", "engram-memory-cli")
	gotFiles := map[string]bool{}
	err = filepath.WalkDir(skillRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(skillRoot, path)
		if err != nil {
			return err
		}
		gotFiles[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Fatalf("vendored Engram skill inventory = %#v; want reviewed tree %#v", gotFiles, wantFiles)
	}
	if _, err := os.Stat(filepath.Join(root, "bundle", "skills", "engram-memory")); !os.IsNotExist(err) {
		t.Fatalf("obsolete Packy-authored skill remains: %v", err)
	}
}

func TestIssue672EngramHistoricalGenerationIsCompleteAndSealed(t *testing.T) {
	root := repositoryRoot(t)
	legacyRoot := filepath.Join(root, "bundle", "history", "engram", "3.0.0")
	legacyFiles := map[string]string{
		"artifact.json":                                   "239ef3e8b6b089c286790bf1d507050af3d1cf801804e3e8751c4c650af9e127",
		"notices/engram-mit":                              "09608597ddda4e5f9033ac407a0d401986d96376c47f6d46789ca38db672dc15",
		"pack.json":                                       "7c915b461ba3ef77aaec3f25e67d98afe5a6c661ef7d03671351275f56cf93c0",
		"skills/engram-memory-cli/SKILL.md":               "20589b1e95c770c72dce5ef645c0c0730919ee5a3f5e5df1c3f48ce43d780f9d",
		"skills/engram-memory-cli/agents/openai.yaml":     "e5d99dae07dd1fa1a8259dbcf9aebae67785f99e8688f5b4feda1b85ce2a1088",
		"skills/engram-memory-cli/references/curation.md": "6f671f02ffeeccfd7a95d9b0fc645806e6e6b1a037ddd43c58695a62ece6c5e6",
	}
	for relative, want := range legacyFiles {
		data, err := os.ReadFile(filepath.Join(legacyRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Fatalf("historical Engram 3.0.0 %s digest = %s, want %s", relative, got, want)
		}
	}

	historyRoot := filepath.Join(root, "bundle", "history", "engram", "3.1.0")
	var artifact struct {
		SchemaVersion   int    `json:"schema_version"`
		PackID          string `json:"pack_id"`
		PackVersion     string `json:"pack_version"`
		AggregateSHA256 string `json:"aggregate_sha256"`
		Manifest        struct {
			Path   string `json:"path"`
			Size   int64  `json:"size"`
			Mode   uint32 `json:"mode"`
			SHA256 string `json:"sha256"`
		} `json:"manifest"`
		Resources []struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Source string `json:"source"`
			SHA256 string `json:"sha256"`
			Files  []struct {
				Path   string `json:"path"`
				Size   int64  `json:"size"`
				Mode   uint32 `json:"mode"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		} `json:"resources"`
	}
	artifactData, err := os.ReadFile(filepath.Join(historyRoot, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(artifactData, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != 1 || artifact.PackID != "engram" || artifact.PackVersion != "3.1.0" || artifact.AggregateSHA256 != "c5915bc62fb2c34e11afba468705c1697cb573ed50ec166e8cd8f4095a1d5ff0" || len(artifact.Resources) != 2 {
		t.Fatalf("historical Engram artifact = %#v", artifact)
	}
	if artifact.Manifest.Path != "pack.json" || artifact.Manifest.Size != 1615 || artifact.Manifest.Mode != 0o644 || artifact.Manifest.SHA256 != "9858083d7c5ba1bb286fd7517bd44484f525276df93afcf6322f4c653d29cf86" {
		t.Fatalf("historical Engram manifest evidence = %#v", artifact.Manifest)
	}
	manifestDigest := sha256.Sum256(mustReadHistoricalFile(t, historyRoot, artifact.Manifest.Path, artifact.Manifest.Size, artifact.Manifest.Mode))
	if got := hex.EncodeToString(manifestDigest[:]); got != artifact.Manifest.SHA256 {
		t.Fatalf("historical Engram manifest digest = %s, want %s", got, artifact.Manifest.SHA256)
	}
	if artifact.Resources[0].Kind != "notice" || artifact.Resources[0].ID != "mit" || artifact.Resources[0].Source != "notices/engram-mit" || len(artifact.Resources[0].Files) != 1 ||
		artifact.Resources[1].Kind != "skill" || artifact.Resources[1].ID != "engram-memory-cli" || artifact.Resources[1].Source != "skills/engram-memory-cli" || len(artifact.Resources[1].Files) != 3 {
		t.Fatalf("historical resource evidence = %#v", artifact.Resources)
	}
	aggregate := sha256.New()
	fmt.Fprintf(aggregate, "%d\x00%s\x00%s\n", artifact.SchemaVersion, artifact.PackID, artifact.PackVersion)
	fmt.Fprintf(aggregate, "manifest\x00%s\x00%d\x00%04o\x00%s\n", artifact.Manifest.Path, artifact.Manifest.Size, artifact.Manifest.Mode, artifact.Manifest.SHA256)
	for _, resource := range artifact.Resources {
		resourceDigest := sha256.New()
		for _, file := range resource.Files {
			data := mustReadHistoricalFile(t, historyRoot, file.Path, file.Size, file.Mode)
			digest := sha256.Sum256(data)
			if got := hex.EncodeToString(digest[:]); got != file.SHA256 {
				t.Fatalf("historical Engram %s digest = %s, want %s", file.Path, got, file.SHA256)
			}
			fmt.Fprintf(resourceDigest, "%s\x00%d\x00%04o\x00%s\n", file.Path, file.Size, file.Mode, file.SHA256)
		}
		if got := hex.EncodeToString(resourceDigest.Sum(nil)); got != resource.SHA256 {
			t.Fatalf("historical Engram %s:%s resource digest = %s, want %s", resource.Kind, resource.ID, got, resource.SHA256)
		}
		fmt.Fprintf(aggregate, "%s\x00%s\x00%s\x00%s\n", resource.Kind, resource.ID, resource.Source, resource.SHA256)
	}
	if got := hex.EncodeToString(aggregate.Sum(nil)); got != artifact.AggregateSHA256 {
		t.Fatalf("historical Engram aggregate digest = %s, want %s", got, artifact.AggregateSHA256)
	}
}

func mustReadHistoricalFile(t *testing.T, root, relative string, size int64, mode uint32) []byte {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) != size || uint32(info.Mode().Perm()) != mode {
		t.Fatalf("historical Engram %s size/mode = %d/%04o, want %d/%04o", relative, len(data), info.Mode().Perm(), size, mode)
	}
	return data
}

func TestIssue672EngramSourceLockSealsTheExactReleaseAndCompleteSelection(t *testing.T) {
	root := repositoryRoot(t)
	var config struct {
		Sources []struct {
			ID         string `json:"id"`
			Repository string `json:"repository"`
			Selector   struct {
				Mode string `json:"mode"`
			} `json:"selector"`
			Resources []struct {
				Kind         string `json:"kind"`
				ResourceID   string `json:"resource_id"`
				UpstreamPath string `json:"upstream_path"`
			} `json:"resources"`
		} `json:"sources"`
	}
	configData, err := os.ReadFile(filepath.Join(root, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Sources) < 1 || config.Sources[0].ID != "engram-source" || config.Sources[0].Repository != "yersonargotev/engram" || config.Sources[0].Selector.Mode != "stable-release" || len(config.Sources[0].Resources) != 2 {
		t.Fatalf("Engram source registration = %#v", config.Sources)
	}
	wantBindings := []struct {
		Kind         string
		ResourceID   string
		UpstreamPath string
	}{
		{Kind: "notice", ResourceID: "mit", UpstreamPath: "LICENSE"},
		{Kind: "skill", ResourceID: "engram-memory-cli", UpstreamPath: "skills/engram-memory-cli"},
	}
	for i, want := range wantBindings {
		got := config.Sources[0].Resources[i]
		if got.Kind != want.Kind || got.ResourceID != want.ResourceID || got.UpstreamPath != want.UpstreamPath {
			t.Fatalf("Engram source binding %d = %#v, want %#v", i, got, want)
		}
	}

	lockData, err := os.ReadFile(filepath.Join(root, "bundle", "sources", "engram-source.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	lockDigest := sha256.Sum256(lockData)
	if hex.EncodeToString(lockDigest[:]) != "f58eb802bb43908630f21016c467613d39405992048ccc1de405fd714b601641" {
		t.Fatalf("Engram source lock digest = %x", lockDigest)
	}
	var lock struct {
		SourceID  string `json:"source_id"`
		Candidate struct {
			Commit  string `json:"commit"`
			Release struct {
				Tag string `json:"tag"`
			} `json:"release"`
		} `json:"candidate"`
		Resources []struct {
			Kind         string `json:"kind"`
			ResourceID   string `json:"resource_id"`
			UpstreamPath string `json:"upstream_path"`
			Files        []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.SourceID != "engram-source" || lock.Candidate.Release.Tag != "v2.2.0" || lock.Candidate.Commit != "8da8d43284f757bf31ab0afa62f063c60b810b78" || len(lock.Resources) != 2 || len(lock.Resources[0].Files) != 1 || len(lock.Resources[1].Files) != 3 {
		t.Fatalf("Engram source lock = %#v", lock)
	}
	for i, want := range wantBindings {
		got := lock.Resources[i]
		if got.Kind != want.Kind || got.ResourceID != want.ResourceID || got.UpstreamPath != want.UpstreamPath {
			t.Fatalf("Engram locked binding %d = %#v, want %#v", i, got, want)
		}
	}
}

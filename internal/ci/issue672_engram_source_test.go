package ci_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIssue672EngramPackUsesExactUpstreamSkill(t *testing.T) {
	root := repositoryRoot(t)
	var manifest struct {
		Version              string   `json:"version"`
		Surfaces             []string `json:"surfaces"`
		ReadinessObligations []string `json:"readiness_obligations"`
		ExternalRequirements []string `json:"external_requirements"`
		SourceReference      struct {
			Repository string `json:"repository"`
			Revision   string `json:"revision"`
		} `json:"source_reference"`
		Resources []struct {
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
	if manifest.Version != "3.0.0" || manifest.SourceReference.Repository != "https://github.com/yersonargotev/engram.git" || manifest.SourceReference.Revision != "v2.0.0" {
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

	wantFiles := map[string]string{
		"SKILL.md":               "20589b1e95c770c72dce5ef645c0c0730919ee5a3f5e5df1c3f48ce43d780f9d",
		"agents/openai.yaml":     "e5d99dae07dd1fa1a8259dbcf9aebae67785f99e8688f5b4feda1b85ce2a1088",
		"references/curation.md": "6f671f02ffeeccfd7a95d9b0fc645806e6e6b1a037ddd43c58695a62ece6c5e6",
	}
	skillRoot := filepath.Join(root, "bundle", "skills", "engram-memory-cli")
	gotFiles := map[string]string{}
	err = filepath.WalkDir(skillRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(skillRoot, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		gotFiles[filepath.ToSlash(relative)] = hex.EncodeToString(digest[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Fatalf("vendored Engram skill inventory = %#v; want exact upstream tree %#v", gotFiles, wantFiles)
	}
	if _, err := os.Stat(filepath.Join(root, "bundle", "skills", "engram-memory")); !os.IsNotExist(err) {
		t.Fatalf("obsolete Packy-authored skill remains: %v", err)
	}
}

func TestIssue672EngramHistoricalGenerationIsCompleteAndSealed(t *testing.T) {
	root := repositoryRoot(t)
	historyRoot := filepath.Join(root, "bundle", "history", "engram", "3.0.0")
	for _, relative := range []string{
		"pack.json",
		"notices/engram-mit",
		"skills/engram-memory-cli/SKILL.md",
		"skills/engram-memory-cli/agents/openai.yaml",
		"skills/engram-memory-cli/references/curation.md",
	} {
		currentRelative := relative
		if relative == "pack.json" {
			currentRelative = "packs/engram/pack.json"
		}
		current, err := os.ReadFile(filepath.Join(root, "bundle", filepath.FromSlash(currentRelative)))
		if err != nil {
			t.Fatal(err)
		}
		historical, err := os.ReadFile(filepath.Join(historyRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(historical, current) {
			t.Fatalf("historical %s does not preserve the current generation exactly", relative)
		}
	}
	var artifact struct {
		SchemaVersion   int    `json:"schema_version"`
		PackID          string `json:"pack_id"`
		PackVersion     string `json:"pack_version"`
		AggregateSHA256 string `json:"aggregate_sha256"`
		Resources       []struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Source string `json:"source"`
			Files  []struct {
				Path string `json:"path"`
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
	if artifact.SchemaVersion != 1 || artifact.PackID != "engram" || artifact.PackVersion != "3.0.0" || artifact.AggregateSHA256 != "6026f8916af6aced84c03fc596b8d939b99ffc3ba5e277c5dbf4c32cdc039ebc" || len(artifact.Resources) != 2 {
		t.Fatalf("historical Engram artifact = %#v", artifact)
	}
	if artifact.Resources[0].Kind != "notice" || artifact.Resources[0].ID != "mit" || artifact.Resources[0].Source != "notices/engram-mit" || len(artifact.Resources[0].Files) != 1 ||
		artifact.Resources[1].Kind != "skill" || artifact.Resources[1].ID != "engram-memory-cli" || artifact.Resources[1].Source != "skills/engram-memory-cli" || len(artifact.Resources[1].Files) != 3 {
		t.Fatalf("historical resource evidence = %#v", artifact.Resources)
	}
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
	if hex.EncodeToString(lockDigest[:]) != "4490d7d6d1ec35d66fc42c817530b564b5b6f7e3cac1d59aab4b5ea0cbd1fc9d" {
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
	if lock.SourceID != "engram-source" || lock.Candidate.Release.Tag != "v2.0.0" || lock.Candidate.Commit != "ca403b6264aeac561f87940c139a97ead2f2d2f4" || len(lock.Resources) != 2 || len(lock.Resources[0].Files) != 1 || len(lock.Resources[1].Files) != 3 {
		t.Fatalf("Engram source lock = %#v", lock)
	}
	for i, want := range wantBindings {
		got := lock.Resources[i]
		if got.Kind != want.Kind || got.ResourceID != want.ResourceID || got.UpstreamPath != want.UpstreamPath {
			t.Fatalf("Engram locked binding %d = %#v, want %#v", i, got, want)
		}
	}
}

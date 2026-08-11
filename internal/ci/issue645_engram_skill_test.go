package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue645EngramSkillContract(t *testing.T) {
	root := repositoryRoot(t)
	manifestPath := filepath.Join(root, "bundle", "packs", "engram", "pack.json")
	var manifest struct {
		Version              string   `json:"version"`
		Surfaces             []string `json:"surfaces"`
		ExternalRequirements []string `json:"external_requirements"`
		Resources            []struct {
			Kind     string `json:"kind"`
			ID       string `json:"id"`
			Source   string `json:"source"`
			Bindings []struct {
				Surface      string `json:"surface"`
				Projection   string `json:"projection"`
				Capabilities []struct {
					Type                          string `json:"type"`
					ExternalExecutableAcquisition struct {
						Tool string `json:"tool"`
					} `json:"external_executable_acquisition"`
				} `json:"capabilities"`
			} `json:"bindings"`
		} `json:"resources"`
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "2.0.0" || strings.Join(manifest.Surfaces, ",") != "codex" || strings.Join(manifest.ExternalRequirements, ",") != "engram" {
		t.Fatalf("Engram manifest identity = %#v", manifest)
	}
	if len(manifest.Resources) != 1 {
		t.Fatalf("Engram resources = %#v; want one selective skill", manifest.Resources)
	}
	resource := manifest.Resources[0]
	if resource.Kind != "skill" || resource.ID != "engram-memory" || resource.Source != "skills/engram-memory" || len(resource.Bindings) != 1 {
		t.Fatalf("Engram resource = %#v", resource)
	}
	binding := resource.Bindings[0]
	if binding.Surface != "codex" || binding.Projection != "skill" || len(binding.Capabilities) != 1 {
		t.Fatalf("Engram binding = %#v", binding)
	}
	capability := binding.Capabilities[0]
	if capability.Type != "external-executable-acquisition" || capability.ExternalExecutableAcquisition.Tool != "engram" {
		t.Fatalf("Engram acquisition capability = %#v", capability)
	}

	skillPath := filepath.Join(root, "bundle", "skills", "engram-memory", "SKILL.md")
	skill := readFile(t, skillPath)
	description := "description: Use when prior project knowledge could materially change the current approach, or when completed work produced a durable decision, root cause, convention, configuration, or reusable discovery."
	if !strings.Contains(skill, description) {
		t.Fatalf("skill must expose exactly the two selective-memory triggers; missing %q", description)
	}
	for _, required := range []string{
		"Search only when prior project knowledge could materially change the current\napproach.",
		"engram search \"<narrow query>\" --project \"<project>\" --limit 5",
		"at most one concise structured\nobservation",
		"`What`, `Why`, and `Where`",
		"Where: <relevant subsystem or path>\" --project \"<project>\"",
		"Add `--topic \"<topic>\"` only when the observation belongs to an evolving topic",
		"routine, transient, already documented, or\nlow-future-value results",
		"CLI is unavailable, fails, or returns an\nerror, continue delivering the primary task",
		"An empty result means no\nrelevant memory was found",
		"If output is truncated, do not\ninfer the missing text or save it as fact. Refine the query once",
		"does not promise full observation retrieval, session\nlifecycle, automatic compaction recovery, or behavior equivalent to\n`engram setup`",
	} {
		if !strings.Contains(skill, required) {
			t.Errorf("skill missing contract %q", required)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "bundle", "instructions", "engram-memory.md")); !os.IsNotExist(err) {
		t.Errorf("obsolete setup instruction remains: %v", err)
	}
}

func TestIssue645EngramSkillScenariosStaySelective(t *testing.T) {
	skill := readFile(t, filepath.Join(repositoryRoot(t), "bundle", "skills", "engram-memory", "SKILL.md"))
	for _, scenario := range []struct {
		name string
		want []string
	}{
		{"material prior decision", []string{"Search only when prior project knowledge could materially change"}},
		{"durable completed root cause", []string{"After the primary work is complete, save at most one concise structured"}},
		{"routine work", []string{"Do\nnot search for routine work", "Complete without writing for routine"}},
		{"empty search", []string{"An empty result means no"}},
		{"truncated search", []string{"If output is truncated, do not", "Refine the query once"}},
		{"topic upsert", []string{"only when the observation belongs to an evolving topic"}},
		{"cli failure", []string{"continue delivering the primary task"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, want := range scenario.want {
				if !strings.Contains(skill, want) {
					t.Errorf("scenario %q missing guidance %q", scenario.name, want)
				}
			}
		})
	}
}

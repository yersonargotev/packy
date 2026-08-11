package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
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
	description := "description: Project memory only. Activate before work when a prior project decision, root cause, convention, configuration, or discovery could materially change the approach, or after work when it produced one of those durable project findings. Never activate for personal memory."
	if !containsNormalizedText(skill, description) {
		t.Fatalf("skill must expose exactly the two selective-memory triggers; missing %q", description)
	}
	for _, required := range []string{
		"Search only when prior project knowledge could materially change the current\napproach.",
		"one lookup intent at a time using one to three distinctive terms",
		"Prefer literal project anchors",
		"Inspect every returned memory for relevance",
		"If a material project memory is expected and the first query is empty",
		"remove generic terms and search the strongest literal anchor",
		"Complete recall when relevant context is found or both targeted searches are\nempty.",
		"engram search \"<narrow query>\" --project \"<project>\"\n--limit 5",
		"at most one concise structured\nobservation",
		"`What`, `Why`, and `Where`",
		"The helper always invokes `engram save` with `--project`",
		"adds `--topic \"<topic>\"`",
		"routine, transient, already documented, or\nlow-future-value results",
		"CLI is unavailable, fails, or returns an\nerror, continue delivering the primary task",
		"If output is truncated, do not\ninfer the missing text or save it as fact",
		"does not promise full observation retrieval, session\nlifecycle, automatic compaction recovery, or behavior equivalent to\n`engram setup`",
	} {
		if !containsNormalizedText(skill, required) {
			t.Errorf("skill missing contract %q", required)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "bundle", "instructions", "engram-memory.md")); !os.IsNotExist(err) {
		t.Errorf("obsolete setup instruction remains: %v", err)
	}
}

func TestIssue645EngramCLIHelper(t *testing.T) {
	root := repositoryRoot(t)
	helper := filepath.Join(root, "bundle", "skills", "engram-memory", "scripts", "engram-memory")
	for _, scenario := range []struct {
		name       string
		args       []string
		output     string
		fail       bool
		wantLog    string
		wantOutput string
	}{
		{name: "empty search", args: []string{"search", "packy", "architecture"}, wantLog: "search\narchitecture\n--project\npacky\n--limit\n5\n"},
		{name: "truncated search", args: []string{"search", "packy", "projection"}, output: strings.Repeat("x", 400), wantLog: "search\nprojection\n--project\npacky\n--limit\n5\n", wantOutput: strings.Repeat("x", 400)},
		{name: "explicit project save", args: []string{"save", "packy", "Decision", "What: result\nWhy: reuse\nWhere: internal"}, wantLog: "save\nDecision\nWhat: result\nWhy: reuse\nWhere: internal\n--project\npacky\n"},
		{name: "topic upsert", args: []string{"save", "packy", "Convention", "What: result\nWhy: reuse\nWhere: docs", "architecture/memory"}, wantLog: "save\nConvention\nWhat: result\nWhy: reuse\nWhere: docs\n--project\npacky\n--topic\narchitecture/memory\n"},
		{name: "CLI failure is best effort", args: []string{"search", "packy", "failure"}, fail: true, wantLog: "search\nfailure\n--project\npacky\n--limit\n5\n", wantOutput: "Engram search failed; continuing without memory."},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "engram.log")
			fake := filepath.Join(dir, "engram")
			fakeScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$ENGRAM_LOG\"\nif [ \"${ENGRAM_FAIL:-}\" = 1 ]; then exit 9; fi\nprintf '%s' \"${ENGRAM_OUTPUT:-}\"\n"
			if err := os.WriteFile(fake, []byte(fakeScript), 0o700); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("bash", append([]string{helper}, scenario.args...)...)
			command.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "ENGRAM_LOG="+logPath, "ENGRAM_OUTPUT="+scenario.output)
			if scenario.fail {
				command.Env = append(command.Env, "ENGRAM_FAIL=1")
			}
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("helper blocked the primary task: %v\n%s", err, output)
			}
			log, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(log) != scenario.wantLog || !strings.Contains(string(output), scenario.wantOutput) {
				t.Fatalf("helper result log=%q output=%q", log, output)
			}
		})
	}
}

func TestIssue645EngramSkillScenariosStaySelective(t *testing.T) {
	skill := readFile(t, filepath.Join(repositoryRoot(t), "bundle", "skills", "engram-memory", "SKILL.md"))
	for _, scenario := range []struct {
		name string
		want []string
	}{
		{"material prior decision", []string{"Project memory only. Activate before work when a prior project decision", "one lookup intent at a time", "Inspect every returned memory"}},
		{"durable completed root cause", []string{"After the primary work is complete, save at most one concise structured"}},
		{"routine work", []string{"Do\nnot search for routine work", "Complete without writing for routine"}},
		{"expected empty search", []string{"If a material project memory is expected and the first query is empty", "remove generic terms and search the strongest literal anchor", "both targeted searches are\nempty"}},
		{"truncated search", []string{"If output is truncated, do not"}},
		{"identity or preference prompt", []string{"Never activate for personal memory", "Project memory excludes user identity, personal preferences"}},
		{"topic upsert", []string{"only when the observation belongs to an evolving", "adds `--topic"}},
		{"cli failure", []string{"continue delivering the primary task"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, want := range scenario.want {
				if !containsNormalizedText(skill, want) {
					t.Errorf("scenario %q missing guidance %q", scenario.name, want)
				}
			}
		})
	}
}

func containsNormalizedText(document, fragment string) bool {
	normalize := func(value string) string {
		return strings.Join(strings.Fields(value), " ")
	}
	return strings.Contains(normalize(document), normalize(fragment))
}

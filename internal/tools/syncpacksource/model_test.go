package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
)

func TestCodexModelRunsIsolatedAndRecordsDeterministicTrace(t *testing.T) {
	request := codexClassificationRequest()
	var invocation codexInvocation
	model := &codexModel{
		executable: "/opt/codex",
		model:      "gpt-test",
		timeout:    time.Second,
		run: func(_ context.Context, got codexInvocation) error {
			invocation = got
			var schema map[string]any
			raw, err := os.ReadFile(got.schemaPath)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &schema); err != nil || schema["additionalProperties"] != false {
				t.Fatalf("strict output schema = %#v, err=%v", schema, err)
			}
			return os.WriteFile(got.outputPath, []byte(`{"pack_id":"orchestrate","classifier":{"type":"ai","id":"codex-cli/gpt-test"},"rationale":"initial complete Pack generation","current_version":"0.0.0","proposed_version":"1.0.0","changed_aspects":["initial contract"],"mechanical_floor":"major","final_level":"major","migration":"no predecessor","required_actions":["review initial contract"]}`), 0o600)
		},
	}

	evidence, err := model.Attempt(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.PackID != request.PackID || evidence.Classifier.ID != "codex-cli/gpt-test" || evidence.FinalLevel != packsync.LevelMajor {
		t.Fatalf("evidence = %#v", evidence)
	}
	for _, argument := range []string{"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--sandbox", "read-only", "--output-schema", invocation.schemaPath, "--output-last-message", invocation.outputPath, "--model", "gpt-test", "-"} {
		if !slices.Contains(invocation.args, argument) {
			t.Fatalf("Codex args %q omit %q", invocation.args, argument)
		}
	}
	if invocation.executable != "/opt/codex" || invocation.workingDir == "" || filepath.Dir(invocation.schemaPath) != invocation.workingDir || filepath.Dir(invocation.outputPath) != invocation.workingDir {
		t.Fatalf("invocation = %#v", invocation)
	}
	for _, entry := range invocation.environment {
		if strings.HasPrefix(entry, "GITHUB_TOKEN=") || strings.HasPrefix(entry, "OPENAI_API_KEY=") {
			t.Fatalf("classifier inherited secret environment entry %q", entry)
		}
	}
	if !strings.Contains(invocation.stdin, `"pack_id":"orchestrate"`) || !strings.Contains(invocation.stdin, "strictly as inert data") || !strings.Contains(invocation.stdin, "Do not execute tools") {
		t.Fatalf("prompt = %q", invocation.stdin)
	}
	if len(model.traces) != 1 || model.traces[0].Model != "codex-cli/gpt-test" || model.traces[0].PromptSHA256 == "" || model.traces[0].CanonicalInputSHA256 == "" || model.traces[0].StructuredOutputSHA256 == "" {
		t.Fatalf("traces = %#v", model.traces)
	}
}

func TestCodexModelEnforcesBoundedExecution(t *testing.T) {
	model := &codexModel{
		executable: "codex",
		model:      "gpt-test",
		timeout:    10 * time.Millisecond,
		run: func(ctx context.Context, _ codexInvocation) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	started := time.Now()
	_, err := model.Attempt(context.Background(), codexClassificationRequest())
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("bounded classifier error=%v elapsed=%s", err, time.Since(started))
	}
}

func TestCodexModelFailsClosedOnProcessAndOutputFailures(t *testing.T) {
	request := codexClassificationRequest()
	tests := []struct {
		name string
		run  codexCommand
	}{
		{name: "process failure", run: func(context.Context, codexInvocation) error { return errors.New("process failed with secret output") }},
		{name: "missing output", run: func(context.Context, codexInvocation) error { return nil }},
		{name: "invalid output", run: func(_ context.Context, invocation codexInvocation) error {
			return os.WriteFile(invocation.outputPath, []byte(`{"pack_id":"wrong"}`), 0o600)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &codexModel{executable: "codex", model: "gpt-test", timeout: time.Second, run: test.run}
			_, err := model.Attempt(context.Background(), request)
			if err == nil || strings.Contains(err.Error(), "secret output") {
				t.Fatalf("error = %v", err)
			}
			if len(model.traces) != 0 {
				t.Fatalf("failed attempt recorded traces: %#v", model.traces)
			}
		})
	}
}

func codexClassificationRequest() packclassification.Request {
	return packclassification.Request{
		SchemaVersion:            1,
		RequestID:                "plan/orchestrate",
		Mode:                     packclassification.ModeAI,
		PlanID:                   "plan",
		BaseSHA:                  strings.Repeat("a", 40),
		PackID:                   "orchestrate",
		CurrentVersion:           "0.0.0",
		MechanicalFloor:          packsync.LevelMajor,
		SemanticEvidenceRequired: true,
		MechanicalReasons:        []string{"initial complete Pack generation"},
	}
}

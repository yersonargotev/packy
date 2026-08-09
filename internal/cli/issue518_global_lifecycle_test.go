package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue518ActivationPublishesMinimalInstalledPackReceipt(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	fixture := newCLITestFixture(t, opts)

	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("activate Matty: %v\n%s", err, out)
	}

	data, err := os.ReadFile(fixture.packState.File())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion int `json:"schema_version"`
		Receipts      []struct {
			Pack struct {
				ID      string `json:"id"`
				Version string `json:"version"`
			} `json:"pack"`
			Surface     string `json:"surface"`
			Resources   []any  `json:"resources"`
			Projections []struct {
				Target string `json:"target"`
				Digest string `json:"digest"`
			} `json:"projections"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode receipt document: %v\n%s", err, data)
	}
	if document.SchemaVersion != 1 || len(document.Receipts) != 1 {
		t.Fatalf("receipt document = %#v\n%s", document, data)
	}
	receipt := document.Receipts[0]
	if receipt.Pack.ID != "matty" || receipt.Pack.Version != "1.0.2" || receipt.Surface != "codex" || len(receipt.Resources) == 0 || len(receipt.Projections) == 0 {
		t.Fatalf("receipt omitted installed Pack facts: %#v\n%s", receipt, data)
	}
	for _, projection := range receipt.Projections {
		if projection.Target == "" || projection.Digest == "" {
			t.Fatalf("receipt projection omitted target or digest: %#v\n%s", projection, data)
		}
	}
	for _, retired := range []string{"intent", "intents", "ownership", "provider_choices", "applying_journal", "last_attempts", "attempt_history", "plan_id", "plan_digest", "outcome", "history", "recovery"} {
		if jsonContainsKey(data, retired) {
			t.Fatalf("minimal receipt persisted retired field %q:\n%s", retired, data)
		}
	}
}

func TestIssue518StatusReportsCurrentReceiptWithoutHistoricalAttempts(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("activate Matty: %v\n%s", err, out)
	}

	human, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("status Matty: %v\n%s", err, human)
	}
	for _, fact := range []string{"matty 1.0.2 on codex", "Resources:", "Readiness:", "Receipt ownership:", "Drift:"} {
		if !strings.Contains(human, fact) {
			t.Fatalf("status omitted %q:\n%s", fact, human)
		}
	}
	if strings.Contains(strings.ToLower(human), "attempt") || strings.Contains(strings.ToLower(human), "history") {
		t.Fatalf("status exposed retired attempt history:\n%s", human)
	}

	encoded, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("JSON status Matty: %v\n%s", err, encoded)
	}
	var report capabilitypack.JSONStatusReport
	if err := json.Unmarshal([]byte(encoded), &report); err != nil || len(report.Entries) != 1 {
		t.Fatalf("decode JSON status: %v\n%s", err, encoded)
	}
	if jsonContainsKey([]byte(encoded), "latest_attempt") {
		t.Fatalf("JSON status exposed retired attempt history:\n%s", encoded)
	}
}

func jsonContainsKey(data []byte, key string) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	return containsJSONKey(value, key)
}

func containsJSONKey(value any, key string) bool {
	switch value := value.(type) {
	case map[string]any:
		for candidate, child := range value {
			if candidate == key || containsJSONKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if containsJSONKey(child, key) {
				return true
			}
		}
	}
	return false
}

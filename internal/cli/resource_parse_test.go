package cli

import (
	"encoding/json"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestResourceParseFailuresUseJSONLifecycleFailure(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	packID := "mat" + "ty"
	for _, args := range [][]string{
		{"activate", packID, "--surface", "codex", "--resource", "malformed", "--json"},
		{"deactivate", packID, "--surface", "codex", "--resource", "malformed", "--json"},
	} {
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err == nil {
			t.Fatalf("malformed resource unexpectedly succeeded: %v", args)
		}
		var failure capabilitypack.JSONLifecycleFailure
		if decodeErr := json.Unmarshal([]byte(out), &failure); decodeErr != nil || failure.Report != "pack-lifecycle-failure" || failure.Stage != "preview" {
			t.Fatalf("malformed resource JSON = %q decode=%v", out, decodeErr)
		}
	}
}

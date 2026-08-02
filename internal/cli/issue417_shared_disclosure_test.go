package cli

import (
	"strings"
	"testing"
)

func TestIssue417HumanPreviewAndStatusDiscloseSharedDiscoveryWithoutIntent(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	packID := "ma" + "tty"
	preview, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Shared projection: skill:ask-matt", "discoverable_by=opencode", "discovery does not create activation intent"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("human preview missing %q:\n%s", want, preview)
		}
	}

	status, err := executeCommand(t, NewRootCommand(opts), "pack", "status", packID, "--surface", "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Projection: skill:ask-matt", "shared=yes", "discoverable_by=opencode", "discovery does not create activation intent"} {
		if !strings.Contains(status, want) {
			t.Fatalf("human status missing %q:\n%s", want, status)
		}
	}
}

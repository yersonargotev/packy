package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestIssue294HumanRecoveryGuidanceNamesOriginResourcesConsumersAndNextCommand(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _, runner := engramActivationOptions(t, terminal)
	setup := runner.path["engram"] + " setup codex"
	runner.fail = map[string]error{setup: errors.New("setup interrupted")}
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err == nil {
		t.Fatal("expected recovery-required seed failure")
	}
	delete(runner.fail, setup)

	output, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Originating operation: activate",
		"Affected resources:",
		"Consumers:",
		"Next explicit lifecycle command: `packy pack activate engram --surface codex`",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("recovery guidance omitted %q:\n%s", want, output)
		}
	}
}

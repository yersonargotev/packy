package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackActivationAcceptsAndRendersExplicitProviderChoiceWithoutMutation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeCompositionBundle(t, false)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	before := snapshotTree(t, home)
	packID := "ma" + "tty"

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", "codex", "--provider", "cap:dep=engram", "--dry-run")
	if err != nil {
		t.Fatalf("provider choice dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{"Provider choice: capability=cap:dep provider=engram/all", "Activation: required engram 1.0.0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("provider choice output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 || snapshotTree(t, home) != before {
		t.Fatal("provider choice dry-run prompted or mutated HOME")
	}
}

func TestParseProviderChoices(t *testing.T) {
	resource := capabilitypack.ResourceIdentity{Kind: "skill", ID: "storage"}
	got, err := parseProviderChoices([]string{"cap:storage=provider/skill:storage", "cap:legacy=legacy"})
	if err != nil {
		t.Fatal(err)
	}
	want := []capabilitypack.ProviderChoice{
		{Capability: "cap:storage", ProviderPack: "provider", ProviderResource: &resource},
		{Capability: "cap:legacy", ProviderPack: "legacy"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider choices = %#v, want %#v", got, want)
	}
}

func TestParseProviderChoicesRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "cap:storage", "=provider", "cap:storage=", "cap:storage=/skill:storage", "cap:storage=provider/malformed"} {
		t.Run(value, func(t *testing.T) {
			_, err := parseProviderChoices([]string{value})
			if err == nil || !strings.Contains(err.Error(), "provider choice") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

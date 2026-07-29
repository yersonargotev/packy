package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// TestIssue295LifecycleHumanAndJSONRenderTheSameDomainFacts protects the
// accepted rule that human and structured output are two renderings of one
// lifecycle result. Each preview runs twice against one unchanged sandbox.
func TestIssue295LifecycleHumanAndJSONRenderTheSameDomainFacts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) (Options, []string)
	}{
		{
			name: "activate",
			setup: func(t *testing.T) (Options, []string) {
				opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
				return opts, []string{"pack", "activate", "matty", "--surface", "codex", "--dry-run"}
			},
		},
		{
			name: "update",
			setup: func(t *testing.T) (Options, []string) {
				opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
				bundle := writeUpdateBundle(t, "1.0.1")
				opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
					t.Fatalf("seed update: %v\n%s", err, out)
				}
				writeUpdateManifest(t, bundle, "2.0.0")
				return opts, []string{"pack", "update", "matty", "--surface", "codex", "--dry-run"}
			},
		},
		{
			name: "reconcile",
			setup: func(t *testing.T) (Options, []string) {
				opts, home, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
					t.Fatalf("seed reconcile: %v\n%s", err, out)
				}
				if err := os.Remove(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
					t.Fatal(err)
				}
				return opts, []string{"pack", "reconcile", "matty", "--surface", "codex", "--dry-run"}
			},
		},
		{
			name: "deactivate",
			setup: func(t *testing.T) (Options, []string) {
				opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
					t.Fatalf("seed deactivation: %v\n%s", err, out)
				}
				return opts, []string{"pack", "deactivate", "matty", "--surface", "codex", "--dry-run"}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, args := tc.setup(t)
			before := snapshotTree(t, opts.Env.(MapEnv)["HOME"])
			human, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("human preview: %v\n%s", err, human)
			}
			jsonArgs := append(append([]string{}, args...), "--json")
			structured, err := executeCommand(t, NewRootCommand(opts), jsonArgs...)
			if err != nil {
				t.Fatalf("JSON preview: %v\n%s", err, structured)
			}
			if got := snapshotTree(t, opts.Env.(MapEnv)["HOME"]); got != before {
				t.Fatalf("paired dry-runs mutated sandbox HOME:\nbefore:\n%s\nafter:\n%s", before, got)
			}

			var report capabilitypack.JSONLifecyclePlan
			if err := json.Unmarshal([]byte(structured), &report); err != nil {
				t.Fatalf("decode lifecycle JSON: %v\n%s", err, structured)
			}
			if report.SchemaVersion != capabilitypack.LifecycleJSONSchemaVersion ||
				report.Report != "pack-lifecycle-preview" || !report.DryRun {
				t.Fatalf("versioned JSON envelope=%+v", report)
			}
			assertIssue295HumanEquivalent(t, human, report)
		})
	}
}

func assertIssue295HumanEquivalent(t *testing.T, human string, report capabilitypack.JSONLifecyclePlan) {
	t.Helper()
	for _, fact := range []string{
		fmt.Sprintf("plan %s", report.PlanID),
		"Digest: " + report.Digest,
		"Selection mode: " + string(report.Selection.Mode),
		"Plan disposition: " + string(report.Disposition),
	} {
		if !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON fact %q:\n%s", fact, human)
		}
	}
	if report.Contract.Compatibility != "" {
		fact := fmt.Sprintf("Compatibility: %s", report.Contract.Compatibility)
		if !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON fact %q:\n%s", fact, human)
		}
	}
	for _, root := range report.Selection.Roots {
		if fact := "Selection root: " + root.String(); !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON root %q:\n%s", fact, human)
		}
	}
	for _, choice := range report.ProviderChoices {
		provider := "all"
		if choice.ProviderResource != nil {
			provider = choice.ProviderResource.String()
		}
		fact := fmt.Sprintf("Provider choice: capability=%s provider=%s/%s", choice.Capability, choice.ProviderPack, provider)
		if !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON provider %q:\n%s", fact, human)
		}
	}
	for _, alias := range report.Aliases {
		if fact := fmt.Sprintf("Alias: %s:%s=%s", alias.Kind, alias.ID, alias.Name); !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON alias %q:\n%s", fact, human)
		}
	}
	for _, migration := range report.Migrations {
		if fact := "Migration: " + migration; !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON migration %q:\n%s", fact, human)
		}
	}
	for _, blocker := range report.Blockers {
		fact := fmt.Sprintf("Blocker: %s %s", blocker.Kind, blocker.Subject)
		if !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON blocker %q:\n%s", fact, human)
		}
	}
	for _, phase := range report.Phases {
		if fact := fmt.Sprintf("Phase: %s (%s)", phase.Kind, phase.Digest); !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON phase %q:\n%s", fact, human)
		}
		for _, action := range phase.Actions {
			if fact := "Action facts: id=" + action.ID; !strings.Contains(human, fact) {
				t.Fatalf("human output omitted JSON action %q:\n%s", fact, human)
			}
		}
	}
}

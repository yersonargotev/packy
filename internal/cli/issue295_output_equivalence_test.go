package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
)

// TestIssue295LifecycleHumanAndJSONRenderTheSameDomainFacts protects the
// accepted rule that human and structured output are two renderings of one
// lifecycle result. Each preview runs twice against one unchanged sandbox.
func TestIssue295LifecycleHumanAndJSONRenderTheSameDomainFacts(t *testing.T) {
	covered := map[string]bool{}
	tests := []struct {
		name    string
		wantErr bool
		setup   func(*testing.T) (Options, []string)
	}{
		{
			name: "activate",
			setup: func(t *testing.T) (Options, []string) {
				opts, _, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				return opts, issue295GranularArgs("activate", "alpha", bundle)
			},
		},
		{
			name: "update",
			setup: func(t *testing.T) (Options, []string) {
				opts, home, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				if out, err := executeCommand(t, NewRootCommand(opts), issue295GranularArgs("activate", "alpha", bundle)[:len(issue295GranularArgs("activate", "alpha", bundle))-1]...); err != nil {
					t.Fatalf("seed update: %v\n%s", err, out)
				}
				rewriteIssue295IntentRoot(t, filepath.Join(home, ".packy", "packs.json"), "alpha", "legacy")
				return opts, []string{"pack", "update", "matty", "--surface", "codex", "--dry-run"}
			},
		},
		{
			name: "reconcile",
			setup: func(t *testing.T) (Options, []string) {
				opts, home, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				seed := issue295GranularArgs("activate", "alpha", bundle)
				if out, err := executeCommand(t, NewRootCommand(opts), seed[:len(seed)-1]...); err != nil {
					t.Fatalf("seed reconcile: %v\n%s", err, out)
				}
				if err := os.Remove(filepath.Join(home, ".agents", "skills", "personal-alpha")); err != nil {
					t.Fatal(err)
				}
				return opts, []string{"pack", "reconcile", "matty", "--surface", "codex", "--dry-run"}
			},
		},
		{
			name: "deactivate",
			setup: func(t *testing.T) (Options, []string) {
				opts, _, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				seed := issue295GranularArgs("activate", "alpha", bundle)
				if out, err := executeCommand(t, NewRootCommand(opts), seed[:len(seed)-1]...); err != nil {
					t.Fatalf("seed deactivation: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--resource", "instruction:beta"); err != nil {
					t.Fatalf("seed shared retention: %v\n%s", err, out)
				}
				return opts, []string{"pack", "deactivate", "matty", "--surface", "codex", "--resource", "skill:alpha", "--dry-run"}
			},
		},
		{
			name: "recovery",
			setup: func(t *testing.T) (Options, []string) {
				opts, home, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				delegate := codex.NewSurfaceAdapter(bundle, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "AGENTS.md"))
				adapter := &issue295FailingCLIAdapter{delegate: delegate, fail: true}
				opts.SurfaceAdapters = map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter}
				seed := issue295GranularArgs("activate", "alpha", bundle)
				if _, err := executeCommand(t, NewRootCommand(opts), seed[:len(seed)-1]...); err == nil {
					t.Fatal("expected injected projection failure")
				}
				adapter.fail = false
				return opts, seed
			},
		},
		{
			name:    "blocked",
			wantErr: true,
			setup: func(t *testing.T) (Options, []string) {
				opts, _, bundle := issue295GranularCLIOptions(t, "2.0.0", "alpha", true)
				args := issue295GranularArgs("activate", "alpha", bundle)
				args = append(args[:7], append([]string{"--resource", "skill:alternate"}, args[7:]...)...)
				return opts, args
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, args := tc.setup(t)
			before := snapshotTree(t, opts.Env.(MapEnv)["HOME"])
			human, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil && !tc.wantErr {
				t.Fatalf("human preview: %v\n%s", err, human)
			}
			if err == nil && tc.wantErr {
				t.Fatalf("human preview unexpectedly succeeded:\n%s", human)
			}
			jsonArgs := append(append([]string{}, args...), "--json")
			structured, err := executeCommand(t, NewRootCommand(opts), jsonArgs...)
			commandErr := err
			if err != nil && !tc.wantErr {
				t.Fatalf("JSON preview: %v\n%s", err, structured)
			}
			if err == nil && tc.wantErr {
				t.Fatalf("JSON preview unexpectedly succeeded:\n%s", structured)
			}
			if got := snapshotTree(t, opts.Env.(MapEnv)["HOME"]); got != before {
				t.Fatalf("paired dry-runs mutated sandbox HOME:\nbefore:\n%s\nafter:\n%s", before, got)
			}

			var report capabilitypack.JSONLifecyclePlan
			if err := json.NewDecoder(strings.NewReader(structured)).Decode(&report); err != nil {
				t.Fatalf("decode lifecycle JSON: %v; command error=%v\n%s", err, commandErr, structured)
			}
			if report.SchemaVersion != capabilitypack.LifecycleJSONSchemaVersion ||
				report.Report != "pack-lifecycle-preview" || !report.DryRun {
				t.Fatalf("versioned JSON envelope=%+v", report)
			}
			assertIssue295HumanEquivalent(t, human, report, covered)
		})
	}
	for _, fact := range []string{"custom roots", "resource graph", "capability requirements", "providers", "aliases", "authority", "readiness", "contributors", "contract diff", "migrations", "actions", "recovery", "blockers", "retention", "evidence"} {
		if !covered[fact] {
			t.Fatalf("canonical v4 lifecycle suite never populated %s", fact)
		}
	}
}

func assertIssue295HumanEquivalent(t *testing.T, human string, report capabilitypack.JSONLifecyclePlan, covered map[string]bool) {
	t.Helper()
	for _, fact := range []string{
		fmt.Sprintf("plan %s", report.PlanID),
		"Digest: " + report.Digest,
		"Pack: " + report.Pack,
		"Surface: " + string(report.Surface),
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
		covered["custom roots"] = true
		if fact := "Selection root: " + root.String(); !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON root %q:\n%s", fact, human)
		}
	}
	for _, resource := range report.ResourceGraph.Resources {
		covered["resource graph"] = true
		fact := fmt.Sprintf("Resource graph: resource=%s role=%s dependency_chain=%s requires=%s notices=%s",
			resource.Resource, resource.Role, renderIdentityChain(resource.DependencyChain),
			renderIdentityChain(resource.Requires), renderIdentityChain(resource.Notices))
		assertIssue295Fact(t, human, fact)
	}
	for _, requirement := range report.CapabilityRequirements {
		covered["capability requirements"] = true
		consumer, provider := "all", "all"
		if requirement.ConsumerResource != nil {
			consumer = requirement.ConsumerResource.String()
		}
		if requirement.ProviderResource != nil {
			provider = requirement.ProviderResource.String()
		}
		fact := fmt.Sprintf("Capability requirement: consumer=%s/%s capability=%s provider=%s/%s tools=%s authority=%s readiness=configured:%t,authorized:%t,usable:%t",
			requirement.ConsumerPack, consumer, requirement.Capability, requirement.ProviderPack, provider,
			joinFacts(requirement.RequiredTools), joinFacts(requirement.RequiredAuthority),
			requirement.ResultingReadiness.Configured, requirement.ResultingReadiness.Authorized, requirement.ResultingReadiness.Usable)
		assertIssue295Fact(t, human, fact)
	}
	for _, choice := range report.ProviderChoices {
		covered["providers"] = true
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
		covered["aliases"] = true
		if fact := fmt.Sprintf("Alias: %s:%s=%s", alias.Kind, alias.ID, alias.Name); !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON alias %q:\n%s", fact, human)
		}
	}
	for _, migration := range report.Migrations {
		covered["migrations"] = true
		if fact := "Migration: " + migration; !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON migration %q:\n%s", fact, human)
		}
	}
	for _, blocker := range report.Blockers {
		covered["blockers"] = true
		fact := fmt.Sprintf("Blocker: %s %s", blocker.Kind, blocker.Subject)
		if !strings.Contains(human, fact) {
			t.Fatalf("human output omitted JSON blocker %q:\n%s", fact, human)
		}
	}
	for _, origin := range report.SensitiveEffects {
		for _, authority := range origin.PromptAuthorities {
			covered["authority"] = true
			assertIssue295Fact(t, human, fmt.Sprintf("Authority origin: %s pack=%s resource=%s root=%s dependency_chain=%s",
				authority, origin.Pack, origin.Resource, origin.Root, renderIdentityChain(origin.DependencyChain)))
		}
		for _, authority := range origin.RuntimeAuthorities {
			covered["authority"] = true
			assertIssue295Fact(t, human, fmt.Sprintf("Runtime authority origin: mode=%s kind=%s scope=%s pack=%s resource=%s root=%s dependency_chain=%s",
				authority.ModeID, authority.Kind, factOrNone(string(authority.Scope)), origin.Pack, origin.Resource, origin.Root,
				renderIdentityChain(origin.DependencyChain)))
		}
		for _, effect := range origin.RuntimeEffects {
			covered["authority"] = true
			assertIssue295Fact(t, human, fmt.Sprintf("Sensitive effect origin: mode=%s kind=%s scope=%s pack=%s resource=%s root=%s dependency_chain=%s",
				effect.ModeID, effect.Kind, factOrNone(string(effect.Scope)), origin.Pack, origin.Resource, origin.Root,
				renderIdentityChain(origin.DependencyChain)))
		}
	}
	covered["readiness"] = true
	assertIssue295Fact(t, human, fmt.Sprintf("Expected readiness: configured=%s, authorized=%s, usable=%s",
		readinessValue(report.ReadinessObserved.Configured, report.ExpectedReadiness.Configured),
		readinessValue(report.ReadinessObserved.Authorization, report.ExpectedReadiness.Authorized),
		readinessValue(report.ReadinessObserved.Usability, report.ExpectedReadiness.Usable)))
	assertIssue295Fact(t, human, "Observed evidence: "+renderPendingAction(report.Evidence))
	assertIssue295Fact(t, human, "Pending evidence: "+renderPendingAction(report.PendingEvidence))
	covered["evidence"] = covered["evidence"] || len(report.Evidence) > 0
	for id, contributors := range report.Contributors {
		covered["contributors"] = true
		assertIssue295Fact(t, human, fmt.Sprintf("Contributors: %s <- %s", id, strings.Join(contributors, ", ")))
	}
	for id, contributor := range report.RemovedContributors {
		assertIssue295Fact(t, human, fmt.Sprintf("Contributor removed: %s <- %s", id, contributor))
	}
	for _, retained := range report.RetainedProjections {
		covered["retention"] = true
		assertIssue295Fact(t, human, fmt.Sprintf("Retained shared projection: %s <- %s (no rewrite)", retained.ID, strings.Join(retained.Contributors, ", ")))
	}
	for _, pending := range report.PendingHumanActions {
		assertIssue295Fact(t, human, "  - "+pending)
	}
	if report.Recovery {
		covered["recovery"] = true
		if report.RecoveryGuidance == nil {
			t.Fatal("recovery report omitted structured guidance")
		}
		guidance := report.RecoveryGuidance
		for _, fact := range []string{
			"Originating operation: " + string(guidance.OriginatingOperation),
			"Affected resources: " + renderRecoveryResources(guidance.AffectedResources),
			"Consumers: " + renderRecoveryConsumers(guidance.Consumers),
			"Completed: " + joinFacts(guidance.Completed),
			fmt.Sprintf("Failed: %s — %s", guidance.FailedAction, guidance.FailureDetail),
			"Not started: " + joinFacts(guidance.NotStarted),
			"Next explicit lifecycle command: `" + guidance.NextCommand + "`",
		} {
			assertIssue295Fact(t, human, fact)
		}
	}
	covered["contract diff"] = true
	assertIssue295Fact(t, human, fmt.Sprintf("Contract diff: added=%s changed=%s removed=%s retained=%s",
		joinFacts(report.ContractDiff.Added), joinFacts(report.ContractDiff.Changed),
		joinFacts(report.ContractDiff.Removed), joinFacts(report.ContractDiff.Retained)))
	for _, phase := range report.Phases {
		covered["actions"] = covered["actions"] || len(phase.Actions) > 0
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

type issue295FailingCLIAdapter struct {
	delegate capabilitypack.SurfaceAdapter
	fail     bool
}

func (a *issue295FailingCLIAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	return a.delegate.InspectSurface(ctx, transition)
}

func (a *issue295FailingCLIAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	if a.fail && len(actions) > 0 {
		return &capabilitypack.ProjectionActionError{ID: actions[0].ID, Err: errors.New("issue 295 injected projection failure")}
	}
	return a.delegate.ApplyProjections(ctx, actions)
}

func assertIssue295Fact(t *testing.T, human, fact string) {
	t.Helper()
	if !strings.Contains(human, fact) {
		t.Fatalf("human output omitted JSON fact %q:\n%s", fact, human)
	}
}

func issue295GranularArgs(operation, root string, _ string) []string {
	return []string{"pack", operation, "matty", "--surface", "codex", "--resource", "skill:" + root,
		"--provider", "cap:storage=engram/skill:storage", "--alias", "skill:" + root + "=personal-" + root, "--dry-run"}
}

func issue295GranularCLIOptions(t *testing.T, version, root string, migration bool) (Options, string, string) {
	t.Helper()
	opts, home, repoRoot := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	bundle := copyPackBundleForUpdate(t, repoRoot)
	writeIssue295GranularManifest(t, bundle, version, root, migration)
	opts.Env.(MapEnv)["PACKY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	return opts, home, bundle
}

func writeIssue295GranularManifest(t *testing.T, bundle, version, root string, migration bool) {
	t.Helper()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		"assets/guide.txt":          "guide\n",
		"notices/terms.txt":         "terms\n",
		"agents/authority.md":       "authority\n",
		"skills/shared/SKILL.md":    "shared\n",
		"skills/alpha/SKILL.md":     "alpha\n",
		"skills/legacy/SKILL.md":    "legacy\n",
		"skills/alternate/SKILL.md": "alternate\n",
		"instructions/beta.md":      "beta\n",
		"skills/storage/SKILL.md":   "storage\n",
	} {
		write(filepath.Join(bundle, path), content)
	}
	binding := func(projection, name string) []map[string]any {
		return []map[string]any{{"surface": "codex", "projection": projection, "name": name, "invocation": "$" + name, "mode": "native", "sharing": "exclusive"}}
	}
	base := func(kind, id string) map[string]any {
		resource := map[string]any{
			"kind": kind, "id": id, "requires": []string{}, "conflicts": []string{}, "notices": []string{},
			"bindings": []any{}, "surface_exclusions": []any{}, "provides_capabilities": []string{},
			"requires_capabilities": []string{}, "requires_tools": []string{}, "capability_conflicts": []string{},
		}
		if kind == "skill" || kind == "agent" || kind == "command" {
			resource["runtime_modes"] = []any{}
		}
		return resource
	}
	asset := base("asset", "guide")
	asset["source"] = "assets/guide.txt"
	notice := base("notice", "terms")
	delete(notice, "conflicts")
	delete(notice, "notices")
	notice["source"], notice["license"], notice["attribution"] = "notices/terms.txt", "MIT", "Packy acceptance fixture"
	authority := base("agent", "authority")
	authority["source"], authority["description"], authority["mode"] = "agents/authority.md", "authority fixture", "primary"
	authority["tools"], authority["permissions"], authority["bindings"] = []string{}, []string{"filesystem"}, binding("agent", "authority")
	shared := base("skill", "shared")
	shared["source"], shared["bindings"] = "skills/shared", binding("skill", "shared")
	beta := base("instruction", "beta")
	beta["source"], beta["bindings"], beta["requires"] = "instructions/beta.md", binding("instruction", "beta"), []string{"skill:shared"}
	rootResource := func(id string) map[string]any {
		resource := base("skill", id)
		resource["source"], resource["bindings"] = "skills/"+id, binding("skill", id)
		resource["requires"] = []string{"agent:authority", "asset:guide", "skill:shared"}
		resource["notices"], resource["conflicts"] = []string{"notice:terms"}, []string{"skill:alternate"}
		resource["requires_capabilities"] = []string{"cap:storage"}
		return resource
	}
	selected := rootResource(root)
	alternate := base("skill", "alternate")
	alternate["source"], alternate["bindings"], alternate["conflicts"] = "skills/alternate", binding("skill", "alternate"), []string{"skill:" + root}
	migrations := []map[string]string{}
	if migration {
		migrations = append(migrations, map[string]string{"from": "skill:legacy", "to": "skill:alpha"})
	}
	resources := []map[string]any{authority, asset, beta, notice, alternate, selected, shared}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i]["kind"].(string)+":"+resources[i]["id"].(string) <
			resources[j]["kind"].(string)+":"+resources[j]["id"].(string)
	})
	app := map[string]any{
		"schema_version": 4, "id": "matty", "version": version, "surfaces": []string{"codex"},
		"provides": []string{}, "requires": map[string]any{"capabilities": []string{}, "tools": []string{}}, "conflicts": []string{},
		"resources": resources, "root_migrations": migrations,
		"contract": map[string]any{"exclusions": []any{}},
	}
	storage := base("skill", "storage")
	storage["source"], storage["bindings"], storage["provides_capabilities"] = "skills/storage", binding("skill", "storage"), []string{"cap:storage"}
	provider := map[string]any{
		"schema_version": 4, "id": "engram", "version": "1.0.0", "surfaces": []string{"codex"},
		"provides": []string{}, "requires": map[string]any{"capabilities": []string{}, "tools": []string{}}, "conflicts": []string{},
		"resources": []any{storage}, "root_migrations": []any{}, "contract": map[string]any{"exclusions": []any{}},
	}
	for id, manifest := range map[string]map[string]any{"matty": app, "engram": provider} {
		raw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(bundle, "packs", id, "pack.json"), string(append(raw, '\n')))
	}
}

func rewriteIssue295IntentRoot(t *testing.T, statePath, from, to string) {
	t.Helper()
	var document any
	if err := json.Unmarshal([]byte(readFileString(t, statePath)), &document); err != nil {
		t.Fatal(err)
	}
	var rewrite func(any)
	rewrite = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if typed["kind"] == "skill" && typed["id"] == from {
				typed["id"] = to
			}
			if typed["name"] == "personal-"+from {
				typed["name"] = "personal-" + to
			}
			for _, child := range typed {
				rewrite(child)
			}
		case []any:
			for _, child := range typed {
				rewrite(child)
			}
		}
	}
	rewrite(document)
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

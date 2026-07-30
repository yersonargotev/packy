package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackStatusDetailRendersOrderedOptionalAuthorityFacts(t *testing.T) {
	var output strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	entry := capabilitypack.StatusEntry{
		Pack:    capabilitypack.Pack{ID: "addy", Version: "1.1.0"},
		Surface: capabilitypack.SurfaceClaude,
		OptionalAuthorities: []capabilitypack.OptionalAuthorityObservation{
			{ModeID: "shipping", Authority: "deploy", State: capabilitypack.OptionalAuthorityUnavailable, Fallback: "none"},
			{ModeID: "browser-network", Authority: "network", State: capabilitypack.OptionalAuthorityAvailable, Fallback: "static evidence-only analysis"},
			{ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnknown, Fallback: "static evidence-only analysis"},
		},
		ProjectionDetails: []capabilitypack.ProjectionStatus{{
			ID: "agent:reviewer", Target: "<host-path>/reviewer.md", Owner: "packy",
			Health: capabilitypack.ProjectionVerified, ObservedFingerprint: "sha256:observed",
			DesiredFingerprint: "sha256:desired", Contributors: []string{"addy"},
		}},
	}
	if err := renderPackStatusDetail(cmd, entry, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	first := strings.Index(got, "Optional authority: mode=browser-network authority=browser state=unknown fallback=static evidence-only analysis")
	second := strings.Index(got, "Optional authority: mode=browser-network authority=network state=available fallback=static evidence-only analysis")
	third := strings.Index(got, "Optional authority: mode=shipping authority=deploy state=unavailable fallback=none")
	if first < 0 || second <= first || third <= second {
		t.Fatalf("optional authority facts are missing or unordered:\n%s", got)
	}
	if !strings.Contains(got, "Readiness: configured=unknown, authorized=unknown, usable=unknown") {
		t.Fatalf("optional authority facts changed readiness rendering:\n%s", got)
	}
	if !strings.Contains(got, "Projection: agent:reviewer target=<host-path>/reviewer.md owner=packy health=verified observed=sha256:observed desired=sha256:desired contributors=addy") {
		t.Fatalf("human status omitted structured projection facts:\n%s", got)
	}
}

func TestPackShowRenderersExposeTheSameWithdrawnHistoryRouteAndIntentFacts(t *testing.T) {
	alias := capabilitypack.SurfaceAlias{Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}
	counts := capabilitypack.ResourceCounts{
		Skills: 24, Agents: 4, Commands: 8, Assets: 7, Notices: 1,
	}
	contract := capabilitypack.LifecycleContract{
		Compatibility: capabilitypack.CompatibilityComplete, CompatibilityObserved: true,
		Counts: counts, DependencyClosure: []string{"skill:using-agent-skills"},
		SelectionValidity: capabilitypack.SelectionValidity{
			All: capabilitypack.SelectionAvailability{Available: true},
		},
		Bindings: []capabilitypack.LifecycleBinding{{
			Kind: "agent", ID: "reviewer", Projection: "agent", Name: alias.Name,
			Invocation: alias.Name, Mode: "native", Sharing: "exclusive",
		}},
		Exclusions: []capabilitypack.LifecycleExclusion{{
			ID: "lifecycle:cache", ResourceKind: "lifecycle", Surface: capabilitypack.SurfaceClaude,
			Mode: "optional", Code: "unsupported", SourcePaths: []string{"hooks/cache.sh"}, Reason: "typed hook unavailable",
		}}, OptionalModes: []capabilitypack.OptionalMode{{
			ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static evidence",
		}},
		PromptAuthorities: []string{"browser", "network"}, Aliases: []capabilitypack.SurfaceAlias{alias},
		AuthorityDisclosure: "Host approval remains required.",
	}
	report := capabilitypack.ShowReport{
		Detail: capabilitypack.CatalogDetail{
			Withdrawn: true,
			Pack: capabilitypack.Pack{
				ID: "addy", Version: "1.1.0", Description: "Agent workflows",
				Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude},
				Provides: []string{"workflow:addy"},
			},
			HistoricalVersions: []string{"1.0.0", "1.1.0"},
			UpdateRoutes: []capabilitypack.UpdateRoute{{
				FromVersion: "1.0.0", ToVersion: "1.1.0",
				ExistingSurfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode},
			}},
		},
		SourceIdentity: capabilitypack.PackSourceIdentity{
			PackID: "addy", Version: "1.1.0", SchemaVersion: 3,
			Limitation: "Upstream provenance is unavailable.",
		},
		ResourceCounts: counts,
		ResourceGraph: capabilitypack.ResourceGraph{Resources: []capabilitypack.ResourceClosureFact{{
			Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "using-agent-skills"},
			Role:     capabilitypack.ResourceRoleRoot,
			DependencyChain: []capabilitypack.ResourceIdentity{
				{Kind: "skill", ID: "using-agent-skills"},
			},
			Requires: []capabilitypack.ResourceIdentity{},
			Notices:  []capabilitypack.ResourceIdentity{},
		}}},
		LifecycleAvailability: capabilitypack.ShowLifecycleAvailability{
			LifecycleVerbsAvailable: true,
		},
		Surfaces: []capabilitypack.ShowSurfaceReport{{
			Surface: capabilitypack.SurfaceClaude, Contract: contract,
			Intent: capabilitypack.ShowIntent{
				Present: true, Active: true, Version: "1.1.0", Revision: 7,
				Aliases: []capabilitypack.SurfaceAlias{alias},
			},
		}},
	}

	var structured bytes.Buffer
	if err := renderPackShowJSON(&structured, report); err != nil {
		t.Fatal(err)
	}
	var document packShowJSON
	if err := json.Unmarshal(structured.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredOutput(t, root, "pack-show.schema.json", structured.Bytes()); err != nil {
		t.Fatalf("withdrawn structured show failed schema validation: %v\n%s", err, structured.String())
	}
	if document.CatalogState != "withdrawn" ||
		len(document.HistoricalVersions) != 2 ||
		len(document.UpdateRoutes) != 1 ||
		len(document.SurfaceContracts) != 1 ||
		document.SurfaceContracts[0].Intent.State != "known" ||
		document.SurfaceContracts[0].Intent.Active == nil ||
		!*document.SurfaceContracts[0].Intent.Active ||
		len(document.SurfaceContracts[0].Intent.Aliases) != 1 ||
		document.LifecycleAvailability != (packShowLifecycleAvailabilityJSON{LifecycleVerbsAvailable: true}) ||
		document.ResourceCounts != counts {
		t.Fatalf("structured show facts = %#v", document)
	}

	var human bytes.Buffer
	if err := renderPackShowHuman(&human, report); err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{
		"Decision summary:",
		"What will change: keep claude at 1.1.0",
		"Important risks / prerequisites: catalog entry is withdrawn; fresh activation is unavailable",
		"Next command: packy pack status addy --surface claude",
		"Catalog state: withdrawn",
		"Source identity: pack=addy version=1.1.0 schema=3",
		"Source limitation: Upstream provenance is unavailable.",
		"Historical versions: 1.0.0, 1.1.0",
		"Update route: 1.0.0 -> 1.1.0 on codex, opencode",
		"Resources: 24 skill, 0 instruction, 0 mcp_server, 0 lifecycle, 4 agent, 8 command, 7 asset, 1 notice",
		"Lifecycle availability: fresh_activation=no catalog_update=no lifecycle_verbs=yes automatic_downgrade=no",
		"Resource: skill:using-agent-skills role=root requires=none notices=none",
		"Dependency closure: skill:using-agent-skills",
		"Binding: agent:reviewer -> addy-reviewer [native]; projection=agent name=addy-reviewer sharing=exclusive",
		"Exclusion: lifecycle:cache — typed hook unavailable; resource_kind=lifecycle surface=claude mode=optional code=unsupported source_paths=hooks/cache.sh",
		"Optional mode: browser-network — browser, network; fallback=static evidence",
		"Contract aliases: agent:reviewer=addy-reviewer",
		"Surface intent: known active=yes version=1.1.0 revision=7",
		"Intent aliases: agent:reviewer=addy-reviewer",
	} {
		if !strings.Contains(human.String(), fact) {
			t.Fatalf("human show missing %q:\n%s", fact, human.String())
		}
	}
	if summary, detail := strings.Index(human.String(), "Decision summary:"), strings.Index(human.String(), "addy 1.1.0"); summary < 0 || detail <= summary {
		t.Fatalf("decision summary does not lead detailed output:\n%s", human.String())
	}
	if !strings.Contains(human.String(), "Surface contract: claude") {
		t.Fatalf("decision summary replaced surface contract details:\n%s", human.String())
	}

	var repeated bytes.Buffer
	if err := renderPackShowJSON(&repeated, report); err != nil {
		t.Fatal(err)
	}
	if repeated.String() != structured.String() {
		t.Fatalf("structured show is nondeterministic:\nfirst=%s\nsecond=%s", structured.String(), repeated.String())
	}
	for _, humanOnly := range []string{"Decision summary:", "What will change:", "Important risks / prerequisites:", "Next command:"} {
		if strings.Contains(structured.String(), humanOnly) {
			t.Fatalf("structured show contains human-only summary label %q:\n%s", humanOnly, structured.String())
		}
	}
}

func TestPackShowDecisionSummaryReportsPrerequisitesAndActivationPreview(t *testing.T) {
	report := capabilitypack.ShowReport{
		Detail: capabilitypack.CatalogDetail{Pack: capabilitypack.Pack{
			ID: "sample", Version: "2.0.0",
			Requires: capabilitypack.Requirements{
				Capabilities: []string{"cap:z", "cap:a"},
				Tools:        []string{"zsh", "git"},
			},
			Conflicts: []string{"cap:legacy"},
		}},
		LifecycleAvailability: capabilitypack.ShowLifecycleAvailability{FreshActivationAvailable: true},
		ResourceCounts:        capabilitypack.ResourceCounts{Skills: 2, Commands: 1},
		Surfaces: []capabilitypack.ShowSurfaceReport{{
			Surface: capabilitypack.SurfaceCodex,
			Contract: capabilitypack.LifecycleContract{SelectionValidity: capabilitypack.SelectionValidity{
				All: capabilitypack.SelectionAvailability{Available: true},
			}},
		}},
	}
	var output strings.Builder
	if err := renderPackShowHuman(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"What will change: activation would manage 3 catalog resources across codex",
		"Important risks / prerequisites: requires capabilities cap:a, cap:z; requires global tools git, zsh; conflicts with capabilities cap:legacy",
		"Next command: packy pack activate sample --surface codex --dry-run",
		"Surface contract: codex",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("decision summary missing %q:\n%s", want, output.String())
		}
	}
}

func TestPackShowDecisionSummaryReportsNoRisksAndPropagatesWriterFailure(t *testing.T) {
	report := capabilitypack.ShowReport{Detail: capabilitypack.CatalogDetail{
		Pack: capabilitypack.Pack{ID: "empty", Version: "1.0.0"},
	}}
	var output strings.Builder
	if err := renderPackShowHuman(&output, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Important risks / prerequisites: none reported") {
		t.Fatalf("risk-free summary missing:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "Next command: packy pack list") {
		t.Fatalf("fallback next command missing:\n%s", output.String())
	}

	wantErr := errors.New("write failed")
	if err := renderPackShowHuman(failingWriter{err: wantErr}, report); !errors.Is(err, wantErr) {
		t.Fatalf("writer error = %v, want %v", err, wantErr)
	}
}

func TestLifecycleAndShowHumanRenderTheSameCompleteContract(t *testing.T) {
	contract := capabilitypack.LifecycleContract{
		Counts:            capabilitypack.ResourceCounts{Skills: 1, Agents: 2},
		DependencyClosure: []string{"capability:a"},
		Bindings: []capabilitypack.LifecycleBinding{{
			Kind: "agent", ID: "reviewer", Projection: "agent", Name: "reviewer",
			Invocation: "@reviewer", Mode: "native", Sharing: "exclusive",
		}},
		Exclusions: []capabilitypack.LifecycleExclusion{{
			ID: "hooks", SourcePaths: []string{"hooks/**"}, Reason: "inert",
		}},
		OptionalModes: []capabilitypack.OptionalMode{{
			ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static analysis",
		}},
		PromptAuthorities:   []string{"browser", "network"},
		Aliases:             []capabilitypack.SurfaceAlias{{Kind: "agent", ID: "reviewer", Name: "safe-reviewer"}},
		AuthorityDisclosure: "Host approval remains required.",
	}
	var showOutput strings.Builder
	if err := renderPackShowContract(&showOutput, contract); err != nil {
		t.Fatal(err)
	}
	var lifecycleOutput strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&lifecycleOutput)
	if err := renderPackContract(cmd, contract); err != nil {
		t.Fatal(err)
	}
	if lifecycleOutput.String() != showOutput.String() {
		t.Fatalf("human contract renderers diverged:\nshow:\n%s\nlifecycle:\n%s", showOutput.String(), lifecycleOutput.String())
	}
	for _, fact := range []string{
		"Logical resources: 1 skill, 0 instruction, 0 mcp_server, 0 lifecycle, 2 agent",
		"Dependency closure: capability:a",
		"projection=agent name=reviewer sharing=exclusive",
		"source_paths=hooks/**",
		"fallback=static analysis",
		"Contract aliases: agent:reviewer=safe-reviewer",
	} {
		if !strings.Contains(lifecycleOutput.String(), fact) {
			t.Fatalf("complete lifecycle contract omitted %q:\n%s", fact, lifecycleOutput.String())
		}
	}
}

func TestPackShowContractRendersPerSurfaceSelectionValidity(t *testing.T) {
	left := capabilitypack.ResourceIdentity{Kind: "skill", ID: "left"}
	right := capabilitypack.ResourceIdentity{Kind: "skill", ID: "right"}
	contract := capabilitypack.LifecycleContract{
		SelectionValidity: capabilitypack.SelectionValidity{
			All: capabilitypack.SelectionAvailability{
				Available: false,
				Reasons: []capabilitypack.SelectionValidityReason{{
					Code:     capabilitypack.SelectionReasonResourceConflict,
					Resource: left, Role: capabilitypack.ResourceRoleRoot,
					DependencyChain:     []capabilitypack.ResourceIdentity{left},
					ConflictingResource: right.String(), ConflictingRole: capabilitypack.ResourceRoleDependency,
					ConflictingDependencyChain: []capabilitypack.ResourceIdentity{
						{Kind: "command", ID: "consumer"}, right,
					},
					Surface: capabilitypack.SurfaceCodex,
					Detail:  "the complete set conflicts", Remediation: "use custom selection",
				}},
				OptionalExclusions: []capabilitypack.SelectionValidityReason{{
					Code:                       capabilitypack.SelectionReasonOptionalExclusion,
					Resource:                   capabilitypack.ResourceIdentity{Kind: "agent", ID: "optional"},
					Role:                       capabilitypack.ResourceRoleRoot,
					DependencyChain:            []capabilitypack.ResourceIdentity{{Kind: "agent", ID: "optional"}},
					ConflictingDependencyChain: []capabilitypack.ResourceIdentity{},
					Surface:                    capabilitypack.SurfaceCodex,
					Detail:                     "optional on codex", Remediation: "use another surface",
				}},
			},
			Roots: []capabilitypack.ResourceSelectability{{
				Resource: left, Available: false,
				Reasons: []capabilitypack.SelectionValidityReason{{
					Code:     capabilitypack.SelectionReasonRootExcluded,
					Resource: left, Role: capabilitypack.ResourceRoleRoot,
					DependencyChain:            []capabilitypack.ResourceIdentity{left},
					ConflictingDependencyChain: []capabilitypack.ResourceIdentity{},
					Surface:                    capabilitypack.SurfaceCodex,
					Detail:                     "excluded on codex", Remediation: "use another root",
				}},
			}},
		},
	}
	var output strings.Builder
	if err := renderPackShowContract(&output, contract); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"All selection: available=no",
		"Optional exclusion: code=optional-exclusion resource=agent:optional role=root chain=agent:optional surface=codex",
		"All unavailable: code=resource-conflict resource=skill:left role=root chain=skill:left surface=codex conflicts_with=skill:right conflicting_role=dependency conflicting_chain=command:consumer -> skill:right",
		"Root selection: resource=skill:left available=no",
		"Root unavailable: code=root-excluded resource=skill:left role=root chain=skill:left surface=codex",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("selection output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPackLifecycleHumanOutputIncludesRedactedStructuredActionAndApplyFacts(t *testing.T) {
	opts, home, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	preview, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "ma"+"tty", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{"Phase approval required: yes", "Action facts: id=", "target=<host-path>/", "adapter_provenance="} {
		if !strings.Contains(preview, fact) {
			t.Fatalf("human preview omitted %q:\n%s", fact, preview)
		}
	}
	if strings.Contains(preview, home) {
		t.Fatalf("human preview leaked sandbox path %q:\n%s", home, preview)
	}

	applied, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "ma"+"tty", "--surface", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(applied, "Apply result facts: verified=yes projections=25") {
		t.Fatalf("human apply omitted structured result facts:\n%s", applied)
	}
}

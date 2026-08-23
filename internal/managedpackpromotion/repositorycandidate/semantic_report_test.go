package repositorycandidate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/managedpack"
)

func TestSemanticReportPackContractUsesMaximumFloorAndAllReasons(t *testing.T) {
	current := semanticManifest("1.2.3")
	candidate := semanticManifest("2.0.0")
	candidate.Surfaces = []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}
	candidate.ReadinessObligations = []capabilitypack.ReadinessObligation{capabilitypack.ReadinessRuntimeUsability}
	candidate.ExternalRequirements = []string{"helper"}
	candidate.Selectable = false

	report := compareSemanticChanges(current, nil, candidate, nil)

	if report.previousVersion != "1.2.3" || report.candidateVersion != "2.0.0" || report.actual != majorLevel || report.floor != majorLevel {
		t.Fatalf("version classification = %#v", report)
	}
	wantReasons := []semanticReason{
		{level: majorLevel, detail: "added external requirement helper"},
		{level: minorLevel, detail: "added supported surface opencode"},
		{level: majorLevel, detail: "changed selectability from true to false"},
		{level: majorLevel, detail: "removed readiness obligation surface-authorization"},
		{level: majorLevel, detail: "removed supported surface codex"},
	}
	if !reflect.DeepEqual(report.floorReasons, wantReasons) {
		t.Fatalf("floor reasons = %#v\nwant %#v", report.floorReasons, wantReasons)
	}
	wantMarkdown := "## Semantic changes\n\n" +
		"### Pack contract\n\n" +
		"- External requirement `helper` added.\n" +
		"- Readiness obligation `surface-authorization` removed.\n" +
		"- Selectability changed from `true` to `false`.\n" +
		"- Supported surface `codex` removed.\n" +
		"- Supported surface `opencode` added.\n\n" +
		"### Compatibility\n\n" +
		"- Version: `1.2.3` → `2.0.0` (`major`).\n" +
		"- Mechanical floor: `major`.\n" +
		"- Reasons: added external requirement helper; added supported surface opencode; changed selectability from true to false; removed readiness obligation surface-authorization; removed supported surface codex.\n\n" +
		"### Human judgment\n\n" +
		"- None.\n"
	if got := report.renderMarkdown(); got != wantMarkdown {
		t.Fatalf("Markdown =\n%s\nwant\n%s", got, wantMarkdown)
	}
}

func TestSemanticReportResourcesGraphBindingsAndCapabilitiesAreIndependent(t *testing.T) {
	current := semanticManifest("1.0.0")
	current.Resources = append(current.Resources, semanticResource("asset", "removed"))
	current.Resources[0].Requires = []string{"asset:old"}
	current.Resources[0].Conflicts = []string{"skill:rival"}
	current.Resources[0].Bindings = []capabilitypack.Binding{{
		Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "guide", Mode: "native", Sharing: "exclusive",
		Capabilities: []capabilitypack.SurfaceCapability{{
			Type:               capabilitypack.SurfaceCapabilityProjectInstruction,
			ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "guide", Source: "instructions/old.md"},
		}},
	}}
	candidate := semanticManifest("2.0.0")
	candidate.Resources = append(candidate.Resources, semanticResource("asset", "added"))
	candidate.Resources[0].Source = "skills/new-guide"
	candidate.Resources[0].Requires = []string{"asset:new"}
	candidate.Resources[0].Conflicts = []string{"skill:new-rival"}
	candidate.Resources[0].Bindings = []capabilitypack.Binding{{
		Surface: capabilitypack.SurfaceCodex, Projection: "command", Name: "guide", Mode: "native", Sharing: "exclusive",
		Capabilities: []capabilitypack.SurfaceCapability{{
			Type:               capabilitypack.SurfaceCapabilityProjectInstruction,
			ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "guide", Source: "instructions/new.md"},
		}},
	}}

	report := compareSemanticChanges(current, nil, candidate, nil)

	wantChanges := []string{
		"Binding `skill:guide/codex` changed from `{" + `"surface":"codex","projection":"skill","name":"guide","invocation":"","mode":"native","sharing":"exclusive","capabilities":null` + "}` to `{" + `"surface":"codex","projection":"command","name":"guide","invocation":"","mode":"native","sharing":"exclusive","capabilities":null` + "}`.",
		"Capability `skill:guide/codex/project-instruction` changed from `{" + `"type":"project-instruction","project_instruction":{"id":"guide","source":"instructions/old.md"}` + "}` to `{" + `"type":"project-instruction","project_instruction":{"id":"guide","source":"instructions/new.md"}` + "}`.",
		"Conflict edge `skill:guide — skill:new-rival` added.",
		"Conflict edge `skill:guide — skill:rival` removed.",
		"Requires edge `skill:guide → asset:new` added.",
		"Requires edge `skill:guide → asset:old` removed.",
		"Resource `asset:added` added.",
		"Resource `asset:removed` removed.",
		"Resource `skill:guide` modified.",
		"Resource `skill:guide` source changed from `skills/guide` to `skills/new-guide`.",
	}
	if got := semanticDetails(report.changes); !reflect.DeepEqual(got, wantChanges) {
		t.Fatalf("changes = %#v\nwant %#v", got, wantChanges)
	}
	wantReasons := []string{
		"added conflict edge skill:guide — skill:new-rival",
		"added isolated resource asset:added",
		"added requires edge skill:guide → asset:new",
		"changed binding skill:guide/codex",
		"changed capability skill:guide/codex/project-instruction",
		"changed projection source of skill:guide",
		"removed conflict edge skill:guide — skill:rival",
		"removed requires edge skill:guide → asset:old",
		"removed resource asset:removed",
	}
	if got := semanticReasonDetails(report.floorReasons); report.floor != majorLevel || !reflect.DeepEqual(got, wantReasons) {
		t.Fatalf("floor=%s reasons=%#v\nwant %#v", report.floor, got, wantReasons)
	}
	wantHuman := []string{
		"Conflict edge `skill:guide — skill:rival` removed; review its meaning.",
		"Requires edge `skill:guide → asset:old` removed; review its meaning.",
	}
	if got := semanticDetails(report.humanJudgment); !reflect.DeepEqual(got, wantHuman) {
		t.Fatalf("human judgment = %#v\nwant %#v", got, wantHuman)
	}
}

func TestSemanticReportProvenanceLegalAndNoticeContentStayHumanReviewed(t *testing.T) {
	current := semanticManifest("1.0.0")
	current.Origins = []managedpack.Origin{
		{ID: "gone", Repository: "owner/gone", Commit: "111", Revision: "old"},
		{ID: "upstream", Repository: "owner/old", Commit: "aaa", Revision: "v1"},
	}
	current.Resources = append(current.Resources, semanticResource("notice", "mit"))
	current.Resources[1].License = "MIT"
	current.Resources[1].Attribution = "Original author"
	current.Resources[1].Origin = &managedpack.ResourceOrigin{ID: "upstream", Path: "LICENSE", Relationship: managedpack.RelationshipExactCopy}

	candidate := semanticManifest("1.0.1")
	candidate.Origins = []managedpack.Origin{
		{ID: "new", Repository: "owner/new", Commit: "222", Revision: "new"},
		{ID: "upstream", Repository: "owner/current", Commit: "bbb", Revision: "v2"},
	}
	candidate.Resources = append(candidate.Resources, semanticResource("notice", "mit"))
	candidate.Resources[0].Notices = []string{"notice:mit"}
	candidate.Resources[0].Origin = &managedpack.ResourceOrigin{ID: "upstream", Path: "guide", Relationship: managedpack.RelationshipExactCopy}
	candidate.Resources[1].License = "Apache-2.0"
	candidate.Resources[1].Attribution = "Current author"
	candidate.Resources[1].Origin = &managedpack.ResourceOrigin{ID: "upstream", Path: "NOTICE", Relationship: managedpack.RelationshipAdapted}

	currentFiles := []managedpack.FileRecord{{Path: "notices/mit/LICENSE", Mode: "100644", SHA256: "old"}}
	candidateFiles := []managedpack.FileRecord{{Path: "notices/mit/LICENSE", Mode: "100644", SHA256: "new"}}
	report := compareSemanticChanges(current, currentFiles, candidate, candidateFiles)

	wantChanges := []string{
		"Resource `notice:mit` attribution changed from `Original author` to `Current author`.",
		"Notice resource `notice:mit` content changed.",
		"Resource `notice:mit` license changed from `MIT` to `Apache-2.0`.",
		"Resource `skill:guide` notice association `notice:mit` added.",
		"Origin `gone` removed.",
		"Origin `new` added.",
		"Origin `upstream` commit changed from `aaa` to `bbb`.",
		"Origin `upstream` repository changed from `owner/old` to `owner/current`.",
		"Origin `upstream` revision changed from `v1` to `v2`.",
		"Resource `notice:mit` origin path changed from `LICENSE` to `NOTICE`.",
		"Resource `notice:mit` relationship changed from `exact-copy` to `adapted`.",
		"Resource `skill:guide` changed from authored to derived (`upstream:guide`, `exact-copy`).",
		"Resource `notice:mit` modified.",
		"Resource `skill:guide` modified.",
	}
	if got := semanticDetails(report.changes); !reflect.DeepEqual(got, wantChanges) {
		t.Fatalf("changes = %#v\nwant %#v", got, wantChanges)
	}
	if report.floor != patchLevel || len(report.humanJudgment) != 12 {
		t.Fatalf("floor=%s human=%#v", report.floor, report.humanJudgment)
	}
	for _, item := range report.humanJudgment {
		if item.detail == "" {
			t.Fatal("human-judgment item lacks review detail")
		}
	}
}

func TestSemanticReportExpandsAddedAndRemovedResourceReviewEvidence(t *testing.T) {
	terms := semanticResource("notice", "terms")
	terms.License = "MIT"
	terms.Attribution = "Example author"
	terms.Notices = []string{"notice:terms"}
	terms.Origin = &managedpack.ResourceOrigin{ID: "upstream", Path: "LICENSE", Relationship: managedpack.RelationshipExactCopy}
	files := []managedpack.FileRecord{{Path: "notices/terms/LICENSE", Mode: "100644", SHA256: "terms"}}
	tests := []struct {
		name           string
		current        []managedpack.Resource
		currentFiles   []managedpack.FileRecord
		candidate      []managedpack.Resource
		candidateFiles []managedpack.FileRecord
		want           []string
	}{
		{
			name:    "added",
			current: semanticManifest("1.0.0").Resources, candidate: append(semanticManifest("1.0.1").Resources, terms), candidateFiles: files,
			want: []string{"Resource `notice:terms` added.", "Notice resource `notice:terms` content changed.", "Resource `notice:terms` changed from authored to derived (`upstream:LICENSE`, `exact-copy`).", "Resource `notice:terms` license changed from `` to `MIT`.", "Resource `notice:terms` notice association `notice:terms` added."},
		},
		{
			name:    "removed",
			current: append(semanticManifest("1.0.0").Resources, terms), currentFiles: files, candidate: semanticManifest("2.0.0").Resources,
			want: []string{"Resource `notice:terms` removed.", "Notice resource `notice:terms` content changed.", "Resource `notice:terms` changed from derived (`upstream:LICENSE`, `exact-copy`) to authored.", "Resource `notice:terms` license changed from `MIT` to ``.", "Resource `notice:terms` notice association `notice:terms` removed."},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := semanticManifest("1.0.0")
			current.Origins = []managedpack.Origin{{ID: "upstream", Repository: "owner/upstream", Commit: "aaa"}}
			current.Resources = test.current
			candidate := semanticManifest("2.0.0")
			candidate.Origins = current.Origins
			candidate.Resources = test.candidate
			report := compareSemanticChanges(current, test.currentFiles, candidate, test.candidateFiles)
			changes := semanticDetails(report.changes)
			for _, want := range test.want {
				if !containsString(changes, want) {
					t.Fatalf("changes lack %q: %#v", want, changes)
				}
			}
			if len(report.humanJudgment) < 4 {
				t.Fatalf("resource review evidence is incomplete: %#v", report.humanJudgment)
			}
		})
	}
}

func TestSemanticReportIgnoresEquivalentCollectionOrderAndRendersDeterministically(t *testing.T) {
	current := semanticManifest("1.0.0")
	current.Surfaces = []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode}
	current.ExternalRequirements = []string{"alpha", "beta"}
	current.Origins = []managedpack.Origin{
		{ID: "alpha", Repository: "owner/alpha", Commit: "aaa"},
		{ID: "beta", Repository: "owner/beta", Commit: "bbb"},
	}
	current.Resources = []managedpack.Resource{
		semanticResource("asset", "shared"),
		semanticResource("notice", "terms"),
		semanticResource("skill", "guide"),
	}
	current.Resources[2].Requires = []string{"asset:alpha", "asset:beta"}
	current.Resources[2].Conflicts = []string{"skill:alpha", "skill:beta"}
	current.Resources[2].Tools = []string{"Read", "Write"}
	current.Resources[2].Permissions = []string{"read", "write"}
	current.Resources[2].Notices = []string{"notice:license", "notice:terms"}
	current.Resources[2].Bindings = []capabilitypack.Binding{
		{Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "guide", Capabilities: []capabilitypack.SurfaceCapability{
			{Type: capabilitypack.SurfaceCapabilityProjectInstruction, ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "guide", Source: "guide.md"}},
			{Type: capabilitypack.SurfaceCapabilityClaudeCompositeSkill, ClaudeCompositeSkill: &capabilitypack.ClaudeCompositeSkillCapability{
				Dependencies: []capabilitypack.ResourceIdentity{{Kind: "asset", ID: "alpha"}, {Kind: "asset", ID: "beta"}},
				References:   []capabilitypack.ResourceIdentity{{Kind: "asset", ID: "one"}, {Kind: "asset", ID: "two"}},
			}},
		}},
		{Surface: capabilitypack.SurfaceOpenCode, Projection: "skill", Name: "guide"},
	}
	current.Resources[2].SurfaceExclusions = []capabilitypack.SurfaceExclusion{
		{Surface: capabilitypack.SurfaceCodex, Mode: "one", Code: "one", Reason: "one"},
		{Surface: capabilitypack.SurfaceOpenCode, Mode: "two", Code: "two", Reason: "two"},
	}
	ordered := cloneSemanticManifest(t, current)
	ordered.Version = "1.0.1"
	permuted := cloneSemanticManifest(t, ordered)
	permuted.Surfaces = reversed(permuted.Surfaces)
	permuted.ReadinessObligations = reversed(permuted.ReadinessObligations)
	permuted.ExternalRequirements = reversed(permuted.ExternalRequirements)
	permuted.Origins = reversed(permuted.Origins)
	permuted.Resources = reversed(permuted.Resources)
	guide := &permuted.Resources[0]
	guide.Requires = reversed(guide.Requires)
	guide.Conflicts = reversed(guide.Conflicts)
	guide.Tools = reversed(guide.Tools)
	guide.Permissions = reversed(guide.Permissions)
	guide.Notices = reversed(guide.Notices)
	guide.Bindings = reversed(guide.Bindings)
	guide.SurfaceExclusions = reversed(guide.SurfaceExclusions)
	guide.Bindings[1].Capabilities = reversed(guide.Bindings[1].Capabilities)
	composite := guide.Bindings[1].Capabilities[0].ClaudeCompositeSkill
	composite.Dependencies = reversed(composite.Dependencies)
	composite.References = reversed(composite.References)
	currentFiles := []managedpack.FileRecord{
		{Path: "notices/terms/A", Mode: "100644", SHA256: "a"},
		{Path: "notices/terms/B", Mode: "100644", SHA256: "b"},
	}

	orderedReport := compareSemanticChanges(current, currentFiles, ordered, currentFiles)
	permutedReport := compareSemanticChanges(current, currentFiles, permuted, reversed(currentFiles))
	if len(permutedReport.changes) != 0 || len(permutedReport.humanJudgment) != 0 || len(permutedReport.floorReasons) != 0 {
		t.Fatalf("permutation produced semantic changes: %#v", permutedReport)
	}
	if orderedReport.renderMarkdown() != permutedReport.renderMarkdown() {
		t.Fatalf("render changed under permutations:\n%s\n---\n%s", orderedReport.renderMarkdown(), permutedReport.renderMarkdown())
	}
}

func TestSemanticReportTreatsConflictEdgesAsCanonicalUndirectedEdges(t *testing.T) {
	current := semanticManifest("1.0.0")
	current.Resources = []managedpack.Resource{semanticResource("skill", "alpha"), semanticResource("skill", "beta")}
	current.Resources[0].Conflicts = []string{"skill:beta"}
	candidate := cloneSemanticManifest(t, current)
	candidate.Version = "1.0.1"
	candidate.Resources[0].Conflicts = []string{}
	candidate.Resources[1].Conflicts = []string{"skill:alpha"}

	report := compareSemanticChanges(current, nil, candidate, nil)
	if len(report.changes) != 0 || len(report.floorReasons) != 0 || len(report.humanJudgment) != 0 {
		t.Fatalf("equivalent undirected edge produced changes: %#v", report)
	}
}

func TestSemanticReportClassifiesAddedResourcesByMandatoryContract(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*managedpack.Resource)
		wantFloor  versionLevel
		wantReason string
	}{
		{name: "isolated", mutate: func(*managedpack.Resource) {}, wantFloor: minorLevel, wantReason: "added isolated resource asset:added"},
		{name: "requires", mutate: func(resource *managedpack.Resource) { resource.Requires = []string{"asset:required"} }, wantFloor: majorLevel, wantReason: "added mandatory resource asset:added"},
		{name: "conflicts", mutate: func(resource *managedpack.Resource) { resource.Conflicts = []string{"asset:rival"} }, wantFloor: majorLevel, wantReason: "added mandatory resource asset:added"},
		{name: "tools", mutate: func(resource *managedpack.Resource) { resource.Tools = []string{"Read"} }, wantFloor: majorLevel, wantReason: "added mandatory resource asset:added"},
		{name: "permissions", mutate: func(resource *managedpack.Resource) { resource.Permissions = []string{"read"} }, wantFloor: majorLevel, wantReason: "added mandatory resource asset:added"},
		{name: "capability", mutate: func(resource *managedpack.Resource) {
			resource.Bindings = []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Capabilities: []capabilitypack.SurfaceCapability{{
				Type:                          capabilitypack.SurfaceCapabilityExternalExecutableAcquisition,
				ExternalExecutableAcquisition: &capabilitypack.ExternalExecutableAcquisitionCapability{Tool: "helper"},
			}}}}
		}, wantFloor: majorLevel, wantReason: "added mandatory resource asset:added"},
		{name: "binding without capability", mutate: func(resource *managedpack.Resource) {
			resource.Bindings = []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "asset", Name: "added"}}
		}, wantFloor: minorLevel, wantReason: "added isolated resource asset:added"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := semanticManifest("1.0.0")
			candidate := semanticManifest("2.0.0")
			added := semanticResource("asset", "added")
			test.mutate(&added)
			candidate.Resources = append(candidate.Resources, added)

			report := compareSemanticChanges(current, nil, candidate, nil)
			if report.floor != test.wantFloor || !containsReason(report.floorReasons, test.wantReason) {
				t.Fatalf("floor=%s reasons=%#v, want %s and %q", report.floor, report.floorReasons, test.wantFloor, test.wantReason)
			}
		})
	}
}

func TestSemanticReportUsesOnlySuppliedResourceFileIndexesAndExactSourcePrefixes(t *testing.T) {
	current := semanticManifest("1.0.0")
	candidate := semanticManifest("1.0.1")
	currentFiles := []managedpack.FileRecord{
		{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "same"},
		{Path: "skills/guide-extra/SKILL.md", Mode: "100644", SHA256: "old-neighbor"},
	}

	t.Run("neighboring prefix is ignored", func(t *testing.T) {
		candidateFiles := []managedpack.FileRecord{
			{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "same"},
			{Path: "skills/guide-extra/SKILL.md", Mode: "100644", SHA256: "new-neighbor"},
		}
		report := compareSemanticChanges(current, currentFiles, candidate, candidateFiles)
		if len(report.changes) != 0 {
			t.Fatalf("neighboring source prefix produced changes: %#v", report.changes)
		}
	})

	t.Run("indexed content change is human reviewed", func(t *testing.T) {
		candidateFiles := []managedpack.FileRecord{{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "changed"}}
		report := compareSemanticChanges(current, currentFiles, candidate, candidateFiles)
		if got, want := semanticDetails(report.changes), []string{
			"Resource `skill:guide` content changed.",
			"Resource `skill:guide` modified.",
		}; !reflect.DeepEqual(got, want) {
			t.Fatalf("content changes = %#v, want %#v", got, want)
		}
		if report.floor != patchLevel || len(report.humanJudgment) != 1 {
			t.Fatalf("content classification = %#v", report)
		}
	})

	t.Run("typed capability content is human reviewed", func(t *testing.T) {
		binding := capabilitypack.Binding{Surface: capabilitypack.SurfaceCodex, Capabilities: []capabilitypack.SurfaceCapability{{
			Type:               capabilitypack.SurfaceCapabilityProjectInstruction,
			ProjectInstruction: &capabilitypack.ProjectInstructionCapability{ID: "guide", Source: "instructions/guide.md"},
		}}}
		current.Resources[0].Bindings = []capabilitypack.Binding{binding}
		candidate.Resources[0].Bindings = []capabilitypack.Binding{binding}
		currentWithInstruction := append(currentFiles, managedpack.FileRecord{Path: "instructions/guide.md", Mode: "100644", SHA256: "old-instruction"})
		candidateWithInstruction := []managedpack.FileRecord{
			{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "same"},
			{Path: "instructions/guide.md", Mode: "100644", SHA256: "new-instruction"},
		}
		report := compareSemanticChanges(current, currentWithInstruction, candidate, candidateWithInstruction)
		if got := semanticDetails(report.changes); !containsString(got, "Resource `skill:guide` content changed.") {
			t.Fatalf("typed capability content change is absent: %#v", got)
		}
		if len(report.humanJudgment) != 1 {
			t.Fatalf("typed capability content classification = %#v", report)
		}
	})
}

func TestSemanticReportShowsStructuralBeforeAndAfterValues(t *testing.T) {
	current := semanticManifest("1.0.0")
	current.Resources[0].Attribution = "Old author"
	current.Resources[0].Bindings = []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "guide"}}
	candidate := semanticManifest("2.0.0")
	candidate.Resources[0].Attribution = "New author"
	candidate.Resources[0].Bindings = []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "command", Name: "guide"}}
	report := compareSemanticChanges(current, nil, candidate, nil)
	markdown := report.renderMarkdown()
	for _, want := range []string{"attribution changed from `Old author` to `New author`", "Binding `skill:guide/codex` changed from `{\"surface\":\"codex\",\"projection\":\"skill\"", "to `{\"surface\":\"codex\",\"projection\":\"command\""} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown lacks %q:\n%s", want, markdown)
		}
	}
}

func containsReason(reasons []semanticReason, want string) bool {
	for _, reason := range reasons {
		if reason.detail == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func reversed[T any](values []T) []T {
	result := append([]T(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func cloneSemanticManifest(t *testing.T, value managedpack.Manifest) managedpack.Manifest {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result managedpack.Manifest
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func semanticManifest(version string) managedpack.Manifest {
	return managedpack.Manifest{
		SchemaVersion: 1,
		ID:            "example",
		Version:       version,
		Description:   "Example",
		Selectable:    true,
		Surfaces:      []capabilitypack.Surface{capabilitypack.SurfaceCodex},
		ReadinessObligations: []capabilitypack.ReadinessObligation{
			capabilitypack.ReadinessRuntimeUsability,
			capabilitypack.ReadinessSurfaceAuthorization,
		},
		ExternalRequirements: []string{},
		Origins:              []managedpack.Origin{},
		Resources: []managedpack.Resource{{
			Kind: "skill", ID: "guide", Source: "skills/guide", Description: "Guide",
			Requires: []string{}, Conflicts: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{},
		}},
	}
}

func semanticResource(kind, id string) managedpack.Resource {
	return managedpack.Resource{
		Kind: kind, ID: id, Source: kind + "s/" + id, Description: id,
		Requires: []string{}, Conflicts: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{},
	}
}

func semanticDetails(changes []semanticChange) []string {
	result := make([]string, len(changes))
	for i := range changes {
		result[i] = changes[i].detail
	}
	return result
}

func semanticReasonDetails(reasons []semanticReason) []string {
	result := make([]string, len(reasons))
	for i := range reasons {
		result[i] = reasons[i].detail
	}
	return result
}

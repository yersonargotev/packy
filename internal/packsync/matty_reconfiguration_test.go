package packsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestApprovedMattyReconfigurationInventoryIsCompleteAndClosed(t *testing.T) {
	desired := []string{
		"skills/engineering/ask-matt", "skills/engineering/code-review", "skills/engineering/codebase-design", "skills/engineering/diagnosing-bugs", "skills/engineering/domain-modeling", "skills/engineering/grill-with-docs", "skills/engineering/implement", "skills/engineering/improve-codebase-architecture", "skills/engineering/prototype", "skills/engineering/research", "skills/engineering/resolving-merge-conflicts", "skills/engineering/setup-matt-pocock-skills", "skills/engineering/tdd", "skills/engineering/to-spec", "skills/engineering/to-tickets", "skills/engineering/triage", "skills/engineering/wayfinder", "skills/engineering/wizard",
		"skills/in-progress/claude-handoff", "skills/in-progress/loop-me", "skills/in-progress/setup-ts-deep-modules", "skills/in-progress/writing-beats", "skills/in-progress/writing-fragments", "skills/in-progress/writing-shape",
		"skills/productivity/grill-me", "skills/productivity/grilling", "skills/productivity/handoff", "skills/productivity/teach", "skills/productivity/to-questionnaire", "skills/productivity/wait-what", "skills/productivity/writing-for-agents",
	}
	current := SourceConfig{ID: "mattpocock-skills", Provider: "github", Repository: "mattpocock/skills", Selector: Selector{Mode: SelectorStableRelease}, Resources: mattyFourBindings(t, repositoryRoot(t))}
	proposal := current
	proposal.Resources = make([]Binding, 0, len(desired))
	for _, path := range desired {
		parts := strings.Split(path, "/")
		proposal.Resources = append(proposal.Resources, Binding{PackID: "matty", Kind: "skill", ResourceID: parts[len(parts)-1], UpstreamPath: path})
	}
	normalized, digest, err := canonicalSourceConfig(proposal, "reconfiguration")
	if err != nil || digest == "" || len(normalized.Resources) != 31 {
		t.Fatalf("canonical approved inventory: resources=%d digest=%q err=%v", len(normalized.Resources), digest, err)
	}
	for _, binding := range normalized.Resources {
		if !strings.HasPrefix(binding.UpstreamPath, "skills/engineering/") && !strings.HasPrefix(binding.UpstreamPath, "skills/in-progress/") && !strings.HasPrefix(binding.UpstreamPath, "skills/productivity/") {
			t.Fatalf("out-of-scope category admitted: %s", binding.UpstreamPath)
		}
	}
	before, after := bindingKeysForPack(current.Resources, "matty"), bindingKeysForPack(normalized.Resources, "matty")
	added, removed := stringSetDelta(before, after)
	wantAdded := []string{"matty/skill/claude-handoff", "matty/skill/setup-ts-deep-modules", "matty/skill/to-questionnaire", "matty/skill/wait-what", "matty/skill/wizard", "matty/skill/writing-beats", "matty/skill/writing-for-agents", "matty/skill/writing-fragments", "matty/skill/writing-shape"}
	sort.Strings(wantAdded)
	if !reflect.DeepEqual(added, wantAdded) || !reflect.DeepEqual(removed, []string{"matty/skill/writing-great-skills"}) {
		t.Fatalf("approved delta: added=%v removed=%v", added, removed)
	}
}

func TestManifestReconfigurationFloorDistinguishesAdditionAndBreakingProjection(t *testing.T) {
	before := []byte(`{"schema_version":3,"id":"pack","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/one","requires":[]}]}`)
	addition := []byte(`{"schema_version":3,"id":"pack","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/one","requires":[]},{"kind":"skill","id":"two","source":"skills/two","requires":[]}]}`)
	floor, _, err := manifestReconfigurationFloor(before, addition)
	if err != nil || floor != LevelMinor {
		t.Fatalf("isolated addition floor=%s err=%v", floor, err)
	}
	projection := []byte(`{"schema_version":3,"id":"pack","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/moved","requires":[]}]}`)
	floor, _, err = manifestReconfigurationFloor(before, projection)
	if err != nil || floor != LevelMajor {
		t.Fatalf("breaking projection floor=%s err=%v", floor, err)
	}
}

func TestReconfigurationRequiresExactCurrentImmutableHistory(t *testing.T) {
	source := repositoryRoot(t)
	repository := t.TempDir()
	copyTree(t, filepath.Join(source, "bundle"), filepath.Join(repository, "bundle"))
	manifests, _, err := loadManifests(repository)
	if err != nil {
		t.Fatal(err)
	}
	current := manifests["matty"]
	currentBytes, err := os.ReadFile(filepath.Join(repository, "bundle", "packs", "matty", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCurrentHistoricalGeneration(repository, "matty", current.Version, current, currentBytes); err != nil {
		t.Fatalf("valid current history rejected: %v", err)
	}
	artifact := filepath.Join(repository, "bundle", "history", "matty", current.Version, "artifact.json")
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var edited compositeHistoricalArtifact
	if err := json.Unmarshal(data, &edited); err != nil {
		t.Fatal(err)
	}
	edited.AggregateSHA256 = strings.Repeat("0", 64)
	writeJSON(t, artifact, edited)
	if err := validateCurrentHistoricalGeneration(repository, "matty", current.Version, current, currentBytes); err == nil {
		t.Fatal("edited immutable history artifact was accepted")
	}
}

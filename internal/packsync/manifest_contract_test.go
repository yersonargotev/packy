package packsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestManifestFloorVersionBoundRules(t *testing.T) {
	baseResource := resourceV4Wire{
		Kind: "skill", ID: "root", Source: "skills/root", Requires: []string{}, Conflicts: []string{},
		Notices: []string{}, RequiresCapabilities: []string{}, RequiresTools: []string{},
		CapabilityConflicts: []string{}, Bindings: []map[string]any{{"surface": "codex", "projection": "skill"}},
		SurfaceExclusions: []map[string]any{}, RuntimeModes: []runtimeModeV4Wire{{
			ID: "local", Requirements: []map[string]any{{"kind": "tool", "id": "git"}},
			Authorities: []map[string]any{{"kind": "filesystem_read", "scope": "consumer_project"}},
		}},
	}
	base := manifestV4Wire{SchemaVersion: 4, ID: "example", Version: "1.0.0", Surfaces: []string{"claude", "codex"},
		Requires: emptyRequirements(), Conflicts: []string{}, Resources: []resourceV4Wire{baseResource}, RootMigrations: []migrationV4Wire{}}
	isolated := resourceV4Wire{Kind: "asset", ID: "extra", Source: "assets/extra", Requires: []string{}, Conflicts: []string{},
		Notices: []string{}, RequiresCapabilities: []string{}, RequiresTools: []string{}, CapabilityConflicts: []string{},
		Bindings: []map[string]any{}, SurfaceExclusions: []map[string]any{}, RuntimeModes: []runtimeModeV4Wire{}}

	tests := []struct {
		name string
		edit func(*manifestV4Wire)
		want ClassificationLevel
	}{
		{"isolated resource add", func(m *manifestV4Wire) { m.Resources = append(m.Resources, isolated) }, LevelMinor},
		{"resource removal", func(m *manifestV4Wire) { m.Resources = nil }, LevelMajor},
		{"resource rename", func(m *manifestV4Wire) { m.Resources[0].ID = "renamed" }, LevelMajor},
		{"mandatory dependency add", func(m *manifestV4Wire) { m.Resources[0].Requires = []string{"asset:extra"} }, LevelMajor},
		{"mandatory conflict add", func(m *manifestV4Wire) { m.Conflicts = []string{"cap:other"} }, LevelMajor},
		{"same length requirement replacement", func(m *manifestV4Wire) {
			m.Resources[0].RuntimeModes[0].Requirements[0]["id"] = "gh"
		}, LevelMajor},
		{"same length authority replacement", func(m *manifestV4Wire) {
			m.Resources[0].RuntimeModes[0].Authorities[0]["kind"] = "network"
		}, LevelMajor},
		{"added mandatory mode", func(m *manifestV4Wire) {
			m.Resources[0].RuntimeModes = append(m.Resources[0].RuntimeModes, runtimeModeV4Wire{
				ID: "remote", Authorities: []map[string]any{{"kind": "network"}}, Requirements: []map[string]any{},
			})
		}, LevelMajor},
		{"lost surface", func(m *manifestV4Wire) { m.Surfaces = []string{"codex"} }, LevelMajor},
		{"binding projection change", func(m *manifestV4Wire) { m.Resources[0].Bindings[0]["projection"] = "command" }, LevelMajor},
		{"source projection change", func(m *manifestV4Wire) { m.Resources[0].Source = "skills/replacement" }, LevelMajor},
		{"MCP command change", func(m *manifestV4Wire) { m.Resources[0].Command = "replacement" }, LevelMajor},
		{"MCP args replacement", func(m *manifestV4Wire) { m.Resources[0].Args = []string{"--changed"} }, LevelMajor},
		{"agent mode change", func(m *manifestV4Wire) { m.Resources[0].Mode = "subagent" }, LevelMajor},
		{"agent tools replacement", func(m *manifestV4Wire) { m.Resources[0].Tools = []string{"Read"} }, LevelMajor},
		{"agent permissions replacement", func(m *manifestV4Wire) { m.Resources[0].Permissions = []string{"filesystem-read"} }, LevelMajor},
		{"command arguments change", func(m *manifestV4Wire) {
			m.Resources[0].Arguments = map[string]any{"mode": "required"}
		}, LevelMajor},
		{"runtime effects change", func(m *manifestV4Wire) {
			m.Resources[0].RuntimeModes[0].Effects = []map[string]any{{"kind": "consumer_project_file_change"}}
		}, LevelMajor},
		{"provided capability change", func(m *manifestV4Wire) {
			m.Resources[0].ProvidesCapabilities = []string{"cap:changed"}
		}, LevelMajor},
		{"top-level provides change", func(m *manifestV4Wire) { m.Provides = []string{"cap:changed"} }, LevelMajor},
		{"surface exclusion added", func(m *manifestV4Wire) {
			m.Resources[0].SurfaceExclusions = []map[string]any{{"surface": "claude", "reason": "unsupported"}}
		}, LevelMajor},
		{"notice only", func(m *manifestV4Wire) { m.Resources[0].Notices = []string{"notice:terms"} }, LevelNone},
		{"migration only", func(m *manifestV4Wire) {
			m.RootMigrations = []migrationV4Wire{{From: json.RawMessage(`{"kind":"skill","id":"old"}`), To: json.RawMessage(`{"kind":"skill","id":"root"}`)}}
		}, LevelNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := cloneManifestWire(t, base)
			test.edit(&after)
			got, _ := manifestFloor(base, after)
			if got != test.want {
				t.Fatalf("floor = %s, want %s", got, test.want)
			}
		})
	}
}

func TestApplyManifestContractsSealsCanonicalEvidenceAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	baseline := canonicalPortableManifestV4(t, root)
	historyPath := filepath.Join(root, "bundle", "history", "example", "1.0.0", "pack.json")
	writeFile(t, historyPath, string(baseline))
	writeHistoricalManifestArtifact(t, filepath.Dir(historyPath), "example", "1.0.0", baseline)

	current := strings.Replace(string(baseline), `"notices": []`, `"notices": ["notice:z","notice:a"]`, 1)
	current = strings.Replace(current, `"root_migrations": []`, `"root_migrations": [`+
		`{"from":{"kind":"skill","id":"z"},"to":{"kind":"skill","id":"example"}},`+
		`{"from":{"kind":"skill","id":"a"},"to":{"kind":"skill","id":"example"}}]`, 1)
	manifests := map[string]packManifest{"example": {
		SchemaVersion: 4, ID: "example", Version: "1.0.0", canonicalV4: []byte(current),
	}}
	var blockers []string
	impacts := applyManifestContracts(root, manifests, map[string]bool{"example": true}, nil, map[string]bool{"example": true}, false, &blockers)
	if len(blockers) != 0 || len(impacts) != 1 {
		t.Fatalf("impacts=%#v blockers=%#v", impacts, blockers)
	}
	contract := impacts[0].Contract
	var currentWire manifestV4Wire
	currentNormalized, err := normalizedManifestV4([]byte(current), &currentWire)
	if err != nil {
		t.Fatal(err)
	}
	var baselineWire manifestV4Wire
	baselineNormalized, err := normalizedManifestV4(baseline, &baselineWire)
	if err != nil {
		t.Fatal(err)
	}
	if contract.CurrentManifestSHA256 != hashBytes(currentNormalized) || contract.BaselineManifestSHA256 != hashBytes(baselineNormalized) {
		t.Fatalf("canonical hashes = %#v", contract)
	}
	if !reflect.DeepEqual(contract.RootMigrations, []MigrationIdentity{
		{From: "skill:a", To: "skill:example"}, {From: "skill:z", To: "skill:example"},
	}) || !reflect.DeepEqual(contract.NoticeAssociations, []NoticeAssociation{
		{Resource: "skill:example", Notice: "notice:a"}, {Resource: "skill:example", Notice: "notice:z"},
	}) {
		t.Fatalf("contract evidence = %#v", contract)
	}

	blockers = nil
	if unrelated := applyManifestContracts(root, manifests, map[string]bool{"other": true}, nil, map[string]bool{"example": true}, false, &blockers); len(unrelated) != 0 || len(blockers) != 0 {
		t.Fatalf("unrelated source inspected foreign v4 pack: impacts=%#v blockers=%#v", unrelated, blockers)
	}

	if err := os.Remove(historyPath); err != nil {
		t.Fatal(err)
	}
	blockers = nil
	_ = applyManifestContracts(root, manifests, map[string]bool{"example": true}, nil, map[string]bool{"example": true}, false, &blockers)
	if len(blockers) != 1 || !strings.Contains(blockers[0], "baseline is invalid") {
		t.Fatalf("missing baseline blockers = %#v", blockers)
	}
}

func TestHistoricalManifestBaselineAdmitsArchivedV4AndRejectsArtifactTamper(t *testing.T) {
	root := repositoryRoot(t)
	wire, canonical, err := readHistoricalManifestBaseline(root, "vercel", "1.0.0")
	if err != nil {
		t.Fatalf("real archived v4 baseline rejected: %v", err)
	}
	if wire.SchemaVersion != 4 || wire.ID != "vercel" || wire.Version != "1.0.0" || len(canonical) == 0 {
		t.Fatalf("archived baseline = %#v, canonical bytes=%d", wire, len(canonical))
	}

	copyRoot := t.TempDir()
	history := filepath.Join(copyRoot, "bundle", "history", "vercel", "1.0.0")
	copyTree(t, filepath.Join(root, "bundle", "history", "vercel", "1.0.0"), history)
	artifactPath := filepath.Join(history, "artifact.json")
	var artifact map[string]any
	data, err := os.ReadFile(artifactPath)
	if err != nil || json.Unmarshal(data, &artifact) != nil {
		t.Fatal(err)
	}
	artifact["manifest"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
	tampered, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, append(tampered, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readHistoricalManifestBaseline(copyRoot, "vercel", "1.0.0"); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("artifact manifest hash tamper error = %v", err)
	}
}

func TestManifestContractEvidenceIsRenderedAndSealed(t *testing.T) {
	plan := Plan{
		SchemaVersion: 1, SourceID: "source", Candidate: Candidate{Commit: strings.Repeat("a", 40), Release: &Release{Tag: "v1.2.3"}},
		Preconditions: Preconditions{SourceLockSHA256: "old-lock"}, SourceLockSHA256: "new-lock",
		AffectedPacks: []PackImpact{{
			PackID: "example", CurrentVersion: "1.0.0", MechanicalFloor: LevelMajor,
			Reasons: []string{"manifest resource removed or renamed"}, Contract: &ManifestContractEvidence{
				SchemaVersion: 4, CurrentVersion: "1.0.0", CurrentManifestSHA256: "current", BaselineManifestSHA256: "baseline",
				RootMigrations:     []MigrationIdentity{{From: "skill:old", To: "skill:new"}},
				NoticeAssociations: []NoticeAssociation{{Resource: "skill:new", Notice: "notice:terms"}},
			},
		}},
	}
	var err error
	plan.PlanID, err = seal(plan)
	if err != nil {
		t.Fatal(err)
	}
	human := plan.Human()
	encoded, err := plan.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{"v1.2.3", "old-lock", "new-lock", "skill:old -> skill:new", "skill:new -> notice:terms"} {
		if !strings.Contains(human, evidence) {
			t.Fatalf("human plan omitted %q:\n%s", evidence, human)
		}
	}
	for _, evidence := range []string{`"current_version":"1.0.0"`, `"from":"skill:old"`, `"notice":"notice:terms"`} {
		if !strings.Contains(string(encoded), evidence) {
			t.Fatalf("JSON plan omitted %q: %s", evidence, encoded)
		}
	}
	plan.AffectedPacks[0].Contract.CurrentManifestSHA256 = "tampered"
	if plan.VerifySeal() {
		t.Fatal("contract evidence tampering preserved the plan seal")
	}
}

func emptyRequirements() capabilitypack.Requirements {
	return capabilitypack.Requirements{Capabilities: []string{}, Tools: []string{}}
}

func cloneManifestWire(t *testing.T, value manifestV4Wire) manifestV4Wire {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone manifestV4Wire
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func writeHistoricalManifestArtifact(t *testing.T, root, packID, version string, manifest []byte) {
	t.Helper()
	artifact := historicalManifestArtifact{
		SchemaVersion: 1, PackID: packID, PackVersion: version,
		Manifest:  FileEvidence{Path: "pack.json", Size: int64(len(manifest)), Mode: 0o644, SHA256: hashBytes(manifest)},
		Resources: json.RawMessage(`[]`), AggregateSHA256: strings.Repeat("a", 64),
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifact.json"), append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

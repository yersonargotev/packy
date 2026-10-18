package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

type issue855ClaudeFixture struct {
	pack     testsupport.Fixture
	source   *catalogSourceFixture
	fixture  syntheticCLIFixture
	terminal *fakeTerminal
	opts     Options
}

func newIssue855ClaudeFixture(t *testing.T, id, snapshot string) *issue855ClaudeFixture {
	t.Helper()
	pack := testsupport.SkillVariants(id, testsupport.SurfaceClaude)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, snapshot, pack)}
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	opts := fixture.options
	opts.catalogRootOverride = ""
	opts.CatalogSource = source
	return &issue855ClaudeFixture{pack: pack, source: source, fixture: fixture, terminal: terminal, opts: opts}
}

func (f *issue855ClaudeFixture) run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := executeCommand(t, NewRootCommand(f.opts), args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

func TestIssue855ClaudeUpdateHandsOffByteIdenticalSkillToSelectedSnapshot(t *testing.T) {
	initialSnapshot := strings.Repeat("a", 40)
	updatedSnapshot := strings.Repeat("b", 40)
	test := newIssue855ClaudeFixture(t, "snapshot-handoff", initialSnapshot)
	pack, source, fixture := test.pack, test.source, test.fixture

	test.run(t, "init")
	test.run(t, "activate", pack.ID(), "--surface", "claude")
	target := filepath.Join(fixture.home, ".claude", "skills", "guide")
	resolvedA, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	personal := filepath.Join(fixture.home, ".claude", "personal-notes.md")
	if err := os.WriteFile(personal, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated := pack.WithVersion("2.0.0")
	source.release = catalogReleaseFixture(t, updatedSnapshot, updated)
	test.run(t, "catalog", "refresh")
	test.run(t, "update", pack.ID(), "--surface", "claude")
	resolvedB, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedB == resolvedA {
		t.Fatalf("byte-identical skill remained linked to the previous Catalog Snapshot: %s", resolvedB)
	}
	if !strings.Contains(filepath.ToSlash(resolvedB), updatedSnapshot) {
		t.Fatalf("updated skill target %s does not belong to snapshot %s", resolvedB, updatedSnapshot)
	}

	receiptPath := filepath.Join(fixture.home, ".packy", "packs.json")
	receiptAfterUpdate, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(receiptAfterUpdate), `"version": "2.0.0"`) || !strings.Contains(string(receiptAfterUpdate), `"catalog_snapshot": "`+updatedSnapshot+`"`) {
		t.Fatalf("updated receipt did not select snapshot B:\n%s", receiptAfterUpdate)
	}
	status := test.run(t, "status", pack.ID(), "--surface", "claude", "--json")
	if !strings.Contains(status, `"health":"verified"`) || !strings.Contains(status, `"configured":"true"`) || !strings.Contains(status, `"verified":1`) {
		t.Fatalf("updated source identity was not verified and configured:\n%s", status)
	}
	if out := test.run(t, "update", pack.ID(), "--surface", "claude"); !strings.Contains(out, "Already converged") {
		t.Fatalf("same-snapshot update was not a no-op:\n%s", out)
	}
	resolvedAgain, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	receiptAfterNoOp, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedAgain != resolvedB || string(receiptAfterNoOp) != string(receiptAfterUpdate) {
		t.Fatal("converged update changed the selected skill target or receipt")
	}

	test.run(t, "deactivate", pack.ID(), "--surface", "claude")
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("retired managed skill remains: %v", err)
	}
	if data, err := os.ReadFile(personal); err != nil || string(data) != "keep\n" {
		t.Fatalf("deactivation changed unrelated Claude content: %q, %v", data, err)
	}
	var receipts struct {
		Receipts []json.RawMessage `json:"receipts"`
	}
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &receipts); err != nil {
		t.Fatal(err)
	}
	if len(receipts.Receipts) != 0 {
		t.Fatalf("deactivation retained installed receipt contributions:\n%s", data)
	}
}

func TestIssue855ClaudeUpdateRejectsSameBytesSourceChangeAfterPreview(t *testing.T) {
	initialSnapshot := strings.Repeat("e", 40)
	updatedSnapshot := strings.Repeat("f", 40)
	test := newIssue855ClaudeFixture(t, "snapshot-stale-preview", initialSnapshot)
	test.run(t, "init")
	test.run(t, "activate", test.pack.ID(), "--surface", "claude")
	target := filepath.Join(test.fixture.home, ".claude", "skills", "guide")
	sourceA, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	sourceC := filepath.Join(test.fixture.home, "operator", "stale-guide")
	if err := os.MkdirAll(sourceC, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(sourceC, os.DirFS(sourceA)); err != nil {
		t.Fatal(err)
	}
	resolvedC, err := filepath.EvalSymlinks(sourceC)
	if err != nil {
		t.Fatal(err)
	}
	sourceCBefore := snapshotTree(t, sourceC)

	updated := test.pack.WithVersion("2.0.0")
	test.source.release = catalogReleaseFixture(t, updatedSnapshot, updated)
	test.run(t, "catalog", "refresh")
	receiptPath := filepath.Join(test.fixture.home, ".packy", "packs.json")
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var mutationErr error
	mutated := false
	test.terminal.onApprove = func() {
		mutated = true
		if mutationErr = os.Remove(target); mutationErr == nil {
			mutationErr = os.Symlink(sourceC, target)
		}
	}

	out, err := executeCommand(t, NewRootCommand(test.opts), "update", test.pack.ID(), "--surface", "claude")
	if !mutated || mutationErr != nil {
		t.Fatalf("retarget after Preview: %v", mutationErr)
	}
	var stalePlan capabilitypack.StalePlanError
	if !errors.Is(err, capabilitypack.ErrStalePlan) || !errors.As(err, &stalePlan) || !strings.Contains(err.Error(), "projections changed after Preview") {
		t.Fatalf("same-bytes source change did not stale the plan: %v\n%s", err, out)
	}
	resolved, resolveErr := filepath.EvalSymlinks(target)
	if resolveErr != nil || resolved != resolvedC {
		t.Fatalf("stale Apply changed the concurrent source C link to %s: %v", resolved, resolveErr)
	}
	receiptAfter, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotTree(t, sourceC) != sourceCBefore || string(receiptAfter) != string(receiptBefore) {
		t.Fatal("stale Apply changed the concurrent source tree or installed receipt")
	}
}

func TestIssue855ClaudeUpdateRejectsForeignSameBytesRetarget(t *testing.T) {
	test := newIssue855ClaudeFixture(t, "snapshot-foreign", strings.Repeat("c", 40))
	pack, source, fixture := test.pack, test.source, test.fixture
	test.run(t, "init")
	test.run(t, "activate", pack.ID(), "--surface", "claude")
	target := filepath.Join(fixture.home, ".claude", "skills", "guide")
	ownedSource, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(fixture.home, "operator", "guide")
	if err := os.MkdirAll(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(foreign, os.DirFS(ownedSource)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, target); err != nil {
		t.Fatal(err)
	}
	resolvedForeign, err := filepath.EvalSymlinks(foreign)
	if err != nil {
		t.Fatal(err)
	}
	foreignBefore := snapshotTree(t, foreign)
	receiptPath := filepath.Join(fixture.home, ".packy", "packs.json")
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}

	updated := pack.WithVersion("2.0.0")
	source.release = catalogReleaseFixture(t, strings.Repeat("d", 40), updated)
	test.run(t, "catalog", "refresh")
	out, err := executeCommand(t, NewRootCommand(test.opts), "update", pack.ID(), "--surface", "claude")
	if err == nil || !strings.Contains(err.Error(), "owned Claude skill identity changed") {
		t.Fatalf("foreign same-bytes retarget was not rejected: %v\n%s", err, out)
	}
	resolved, resolveErr := filepath.EvalSymlinks(target)
	if resolveErr != nil || resolved != resolvedForeign {
		t.Fatalf("foreign retarget changed to %s: %v", resolved, resolveErr)
	}
	receiptAfter, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotTree(t, foreign) != foreignBefore || string(receiptAfter) != string(receiptBefore) {
		t.Fatal("rejected update changed the foreign target or installed receipt")
	}

	out, err = executeCommand(t, NewRootCommand(test.opts), "deactivate", pack.ID(), "--surface", "claude")
	if err == nil || !strings.Contains(err.Error(), "owned Claude skill identity changed") {
		t.Fatalf("foreign same-bytes retarget removal was not rejected: %v\n%s", err, out)
	}
	resolved, resolveErr = filepath.EvalSymlinks(target)
	if resolveErr != nil || resolved != resolvedForeign {
		t.Fatalf("rejected removal changed foreign retarget to %s: %v", resolved, resolveErr)
	}
	receiptAfter, err = os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotTree(t, foreign) != foreignBefore || string(receiptAfter) != string(receiptBefore) {
		t.Fatal("rejected removal changed the foreign target or installed receipt")
	}
}

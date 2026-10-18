package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue855ClaudeUpdateHandsOffByteIdenticalSkillToSelectedSnapshot(t *testing.T) {
	pack := testsupport.SkillVariants("snapshot-handoff", testsupport.SurfaceClaude)
	initialSnapshot := strings.Repeat("a", 40)
	updatedSnapshot := strings.Repeat("b", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, initialSnapshot, pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	opts := fixture.options
	opts.catalogRootOverride = ""
	opts.CatalogSource = source
	run := func(args ...string) string {
		t.Helper()
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}

	run("init")
	run("activate", pack.ID(), "--surface", "claude")
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
	run("catalog", "refresh")
	run("update", pack.ID(), "--surface", "claude")
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
	status := run("status", pack.ID(), "--surface", "claude", "--json")
	if !strings.Contains(status, `"health":"verified"`) || !strings.Contains(status, `"configured":"true"`) || !strings.Contains(status, `"verified":1`) {
		t.Fatalf("updated source identity was not verified and configured:\n%s", status)
	}
	if out := run("update", pack.ID(), "--surface", "claude"); !strings.Contains(out, "Already converged") {
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

	run("deactivate", pack.ID(), "--surface", "claude")
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

func TestIssue855ClaudeUpdateRejectsForeignSameBytesRetarget(t *testing.T) {
	pack := testsupport.SkillVariants("snapshot-foreign", testsupport.SurfaceClaude)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, strings.Repeat("c", 40), pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	opts := fixture.options
	opts.catalogRootOverride = ""
	opts.CatalogSource = source
	run := func(args ...string) string {
		t.Helper()
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("init")
	run("activate", pack.ID(), "--surface", "claude")
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
	run("catalog", "refresh")
	out, err := executeCommand(t, NewRootCommand(opts), "update", pack.ID(), "--surface", "claude")
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

	out, err = executeCommand(t, NewRootCommand(opts), "deactivate", pack.ID(), "--surface", "claude")
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

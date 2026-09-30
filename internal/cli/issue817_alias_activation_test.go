package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue817EmilAliasPreservesMattyThroughLifecycle(t *testing.T) {
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.CapabilityRich("matty"), testsupport.CapabilityRich("emil").WithExactCopyBytes("skill:helper", "SKILL.md", []byte("---\nname: prototype\ndescription: Emil fixture skill.\n---\n\nEmil workflow.\n")))
	// Keep coherent existing skill bytes and provenance; rename only its contract identity and bindings.
	for _, id := range []string{"matty", "emil"} {
		path := filepath.Join(fixture.catalogRoot, "packs", id, "pack.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.ReplaceAll(strings.ReplaceAll(string(data), `"helper"`, `"prototype"`), `"skill:helper"`, `"skill:prototype"`))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	layout := newCLITestFixture(t, fixture.options)
	store := capabilitypack.NewFileActivationStore(layout.packState.File())
	run := func(args ...string) string {
		t.Helper()
		output, err := executeCommand(t, NewRootCommand(fixture.options), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return output
	}
	run("activate", "matty", "--surface", "codex", "--resource", "skill:prototype")
	before, err := store.LoadSnapshot(context.Background(), capabilitypack.SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	prototype := filepath.Join(fixture.home, ".agents", "skills", "prototype")
	beforeTree := snapshotTree(t, prototype)
	beforeLink, err := os.Readlink(prototype)
	if err != nil {
		t.Fatal(err)
	}
	output, err := executeCommand(t, NewRootCommand(fixture.options), "activate", "emil", "--surface", "codex", "--resource", "skill:prototype", "--dry-run")
	if err == nil || !strings.Contains(output, "ownership") {
		t.Fatalf("unaliased collision: err=%v\n%s", err, output)
	}
	output = run("activate", "emil", "--surface", "codex", "--resource", "skill:prototype", "--alias", "skill:prototype=emil-prototype")
	if !strings.Contains(output, "verified=yes") {
		t.Fatalf("aliased activation was not verified:\n%s", output)
	}
	aliased := filepath.Join(fixture.home, ".agents", "skills", "emil-prototype")
	if _, err := os.ReadFile(filepath.Join(aliased, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	run("update", "emil", "--surface", "codex", "--alias", "skill:prototype=emil-updated")
	if _, err := os.Lstat(aliased); !os.IsNotExist(err) {
		t.Fatalf("retired alias remains: %v", err)
	}
	updated := filepath.Join(fixture.home, ".agents", "skills", "emil-updated")
	if _, err := os.ReadFile(filepath.Join(updated, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	run("deactivate", "emil", "--surface", "codex")
	if _, err := os.Lstat(updated); !os.IsNotExist(err) {
		t.Fatalf("deactivated alias remains: %v", err)
	}
	after, err := store.LoadSnapshot(context.Background(), capabilitypack.SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	// Receipt revisions follow the shared state document; compare the retained contract.
	before.Intents[0].Revision = after.Intents[0].Revision
	if !reflect.DeepEqual(before.Intents, after.Intents) || !reflect.DeepEqual(before.Ownership, after.Ownership) {
		a, _ := json.Marshal(after)
		t.Fatalf("Matty receipt changed: %s", a)
	}
	afterLink, err := os.Readlink(prototype)
	if err != nil || afterLink != beforeLink || snapshotTree(t, prototype) != beforeTree {
		t.Fatalf("Matty projection changed: %v", err)
	}
}

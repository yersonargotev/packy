package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

// syntheticCLIFixture is the CLI-only adapter over capabilitypack/testsupport.
// It owns only workstation wiring; manifest vocabulary and bundle bytes remain
// owned by the domain fixture package.
type syntheticCLIFixture struct {
	options    Options
	home       string
	bundleRoot string
	packs      map[string]testsupport.Fixture
}

func newSyntheticCLIFixture(t *testing.T, terminal Terminal, packs ...testsupport.Fixture) syntheticCLIFixture {
	t.Helper()
	if len(packs) == 0 {
		t.Fatal("synthetic CLI fixture requires at least one Pack")
	}
	bundleRoot := t.TempDir()
	for _, group := range []string{"engineering", "productivity"} {
		if err := os.MkdirAll(filepath.Join(bundleRoot, "skills", group), 0o755); err != nil {
			t.Fatalf("create synthetic Skill Source group %q: %v", group, err)
		}
	}
	// skillbundle still validates its selected v0 in-progress sentinel even when
	// the tested Pack contributes no legacy grouped skill. Keep that CLI source
	// adapter concern out of the domain fixture.
	loopMe := filepath.Join(bundleRoot, "skills", "in-progress", "loop-me")
	if err := os.MkdirAll(loopMe, 0o755); err != nil {
		t.Fatalf("create synthetic Skill Source sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loopMe, "SKILL.md"), []byte("# Synthetic source sentinel\n"), 0o644); err != nil {
		t.Fatalf("write synthetic Skill Source sentinel: %v", err)
	}
	byID := make(map[string]testsupport.Fixture, len(packs))
	for _, pack := range packs {
		manifest := pack.Manifest()
		if _, exists := byID[manifest.ID]; exists {
			t.Fatalf("duplicate synthetic Pack ID %q", manifest.ID)
		}
		if err := pack.WriteBundle(bundleRoot); err != nil {
			t.Fatalf("write synthetic Pack %q: %v", manifest.ID, err)
		}
		byID[manifest.ID] = pack
	}
	home := t.TempDir()
	return syntheticCLIFixture{
		options: Options{
			Env: MapEnv{
				"HOME":                home,
				"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
				"PATH":                "",
				"PACKY_SKILLS_SOURCE": filepath.Join(bundleRoot, "skills"),
			},
			Runner:   &fakeRunner{},
			Terminal: terminal,
		},
		home:       home,
		bundleRoot: bundleRoot,
		packs:      byID,
	}
}

func (f syntheticCLIFixture) pack(t *testing.T, id string) testsupport.Fixture {
	t.Helper()
	pack, ok := f.packs[id]
	if !ok {
		t.Fatalf("synthetic CLI fixture has no Pack %q", id)
	}
	return pack
}

func syntheticResource(t *testing.T, pack testsupport.Fixture, kind, id string) testsupport.Resource {
	t.Helper()
	for _, resource := range pack.Manifest().Resources {
		if resource.Kind == kind && resource.ID == id {
			return resource
		}
	}
	t.Fatalf("synthetic Pack %q has no %s:%s resource", pack.Manifest().ID, kind, id)
	return testsupport.Resource{}
}

func containsPackListRow(output, id, version string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == id && fields[1] == version {
			return true
		}
	}
	return false
}

func TestSyntheticCLIFixtureComposesUnrelatedPacksWithoutLiveCatalog(t *testing.T) {
	portable := testsupport.PortableAllSurfaces("cli-portable")
	rich := testsupport.CapabilityRich("cli-rich")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, portable, rich)

	out, err := executeCommand(t, NewRootCommand(fixture.options), "list")
	if err != nil {
		t.Fatalf("list synthetic Packs: %v\n%s", err, out)
	}
	for _, pack := range []testsupport.Fixture{portable, rich} {
		if manifest := pack.Manifest(); !containsPackListRow(out, manifest.ID, manifest.Version) {
			t.Fatalf("list omitted synthetic Pack %s@%s:\n%s", manifest.ID, manifest.Version, out)
		}
	}
}

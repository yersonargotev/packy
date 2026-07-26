package vercelacceptance

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestCanonicalExactClosureAndV4RoundTrip(t *testing.T) {
	f := Canonical()
	if err := Validate(f); err != nil {
		t.Fatalf("canonical fixture rejected: %v", err)
	}
	counts := map[string]int{}
	bindings, modes := 0, 0
	owners := map[string]int{}
	for _, source := range f.Sources.Sources {
		for _, binding := range source.Resources {
			owners[binding.Kind+":"+binding.ResourceID]++
		}
	}
	for _, r := range f.Pack.Resources {
		counts[r.Kind]++
		bindings += len(r.Bindings)
		modes += len(r.RuntimeModes)
		if owners[r.Kind+":"+r.ID] != 1 {
			t.Fatalf("resource %s:%s has %d source owners", r.Kind, r.ID, owners[r.Kind+":"+r.ID])
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"skill": 9, "asset": 2, "notice": 2}) || bindings != 27 || modes != 28 {
		t.Fatalf("closure counts=%v bindings=%d modes=%d", counts, bindings, modes)
	}
	if len(f.Sources.Sources) != 3 || len(f.Pack.Contract.Exclusions) != 1 || len(f.Pack.Contract.Exclusions[0].SourcePaths) != 6 {
		t.Fatalf("source/exclusion closure changed")
	}
	b, err := CanonicalManifestJSON()
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "vercel-pack-v4.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(b), bytes.TrimSpace(golden)) {
		t.Fatal("detached canonical manifest golden changed")
	}
	root := t.TempDir()
	path := filepath.Join(root, "pack.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := capabilitypack.LoadPortableManifest(path, root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := capabilitypack.EncodePortableManifestV4(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, encoded) {
		t.Fatal("v4 round trip changed canonical manifest")
	}
}

func TestCanonicalDeterminismAndDetachedCopies(t *testing.T) {
	a, _ := CanonicalJSON()
	b, _ := CanonicalJSON()
	if !bytes.Equal(a, b) {
		t.Fatal("canonical JSON is unstable")
	}
	d1, _ := Digest()
	d2, _ := Digest()
	if d1 != d2 {
		t.Fatal("digest is unstable")
	}
	x, y := Canonical(), Canonical()
	x.Pack.Resources[0].ID = "mutated"
	x.Sources.Sources[0].Resources[0].ResourceID = "mutated"
	if y.Pack.Resources[0].ID == "mutated" || y.Sources.Sources[0].Resources[0].ResourceID == "mutated" {
		t.Fatal("Canonical shares mutable storage")
	}
}

func TestCompatibilityAndAliasPolicyMatchTheFirstContract(t *testing.T) {
	f := Canonical()
	if len(f.Compatibility.PatchPreserves) != 12 ||
		len(f.Compatibility.MinorAllows) != 6 ||
		len(f.Compatibility.MajorIncludes) != 8 ||
		!contains(f.Compatibility.MajorIncludes, "weakened redaction or fail-before-effects safety") {
		t.Fatalf("compatibility contract is incomplete: %+v", f.Compatibility)
	}
	if len(f.Aliases.InitialAliases) != 0 ||
		f.Aliases.Selection != "explicit and surface-local" ||
		f.Aliases.SuggestedPattern != "vercel-pack-<public-name>" ||
		!f.Aliases.PreservesLogicalIdentity {
		t.Fatalf("alias policy changed: %+v", f.Aliases)
	}
}

func TestGuidelineAdaptationsAndSealedIdentities(t *testing.T) {
	f := Canonical()
	if len(f.Blobs) != 5 || len(f.Loaders) != 2 {
		t.Fatal("sealed guideline evidence incomplete")
	}
	for _, l := range f.Loaders {
		file, err := SnapshotFileByPath("skills/" + l.ResourceID + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		content := string(file.Content)
		if file.SHA256 != l.AdaptedSHA256 || !strings.Contains(content, l.PackageRelativePath) ||
			strings.Contains(content, "raw.githubusercontent") || strings.Contains(content, "/main/") ||
			strings.Contains(content, "WebFetch") || strings.Contains(content, "Fetch ") {
			t.Fatalf("moving or non-relative loader: %#v", l)
		}
	}
	for _, want := range []string{"eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab", "fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f", "6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2", "7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445"} {
		found := false
		for _, b := range f.Blobs {
			found = found || b.SHA256 == want
		}
		if !found {
			t.Fatalf("missing sealed identity %s", want)
		}
	}
}

func TestExactSelectedTreesAreCompleteInertAndSealed(t *testing.T) {
	files, err := InspectExactArchive()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 306 {
		t.Fatalf("selected fixture has %d files, want 306", len(files))
	}
	counts := map[string]int{}
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".zip") {
			t.Fatalf("excluded archive entered fixture: %s", file.Path)
		}
		for _, skill := range skills {
			prefix := "skills/" + skill.id + "/"
			if strings.HasPrefix(file.Path, prefix) {
				counts[skill.id]++
			}
		}
	}
	want := map[string]int{
		"vercel-composition-patterns": 14, "vercel-deploy-to-vercel": 3,
		"vercel-react-best-practices": 76, "vercel-react-native-skills": 42,
		"vercel-react-view-transitions": 8, "vercel-cli-with-tokens": 1,
		"vercel-optimize": 156, "vercel-web-design-guidelines": 1,
		"vercel-writing-guidelines": 1,
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("selected skill tree counts=%v want=%v", counts, want)
	}
	for path, digest := range map[string]string{
		"references/vercel-web-interface-guidelines-command.md": "eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab",
		"references/vercel-writing-guidelines-command.md":       "fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f",
		"notices/vercel-web-interface-guidelines-MIT.txt":       "6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2",
		"notices/vercel-writing-guidelines-MIT.txt":             "7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445",
	} {
		file, err := SnapshotFileByPath(path)
		if err != nil || file.SHA256 != digest {
			t.Fatalf("sealed file %s: digest=%s err=%v", path, file.SHA256, err)
		}
	}
}

func TestNegativeTwinsFailDeterministicallyWithoutMutation(t *testing.T) {
	base := Canonical()
	root := t.TempDir()
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "duplicate", "stale", "unauthorized", "moving", "undeclared"} {
		t.Run(name, func(t *testing.T) {
			tw, err := NegativeTwin(name)
			if err != nil {
				t.Fatal(err)
			}
			diff := 0
			if !reflect.DeepEqual(base.Pack, tw.Pack) {
				diff++
			}
			if !reflect.DeepEqual(base.Sources, tw.Sources) {
				diff++
			}
			if !reflect.DeepEqual(base.Legal, tw.Legal) {
				diff++
			}
			if diff != 1 {
				t.Fatalf("changed %d fact groups, want one", diff)
			}
			if name == "moving" &&
				(tw.Sources.Sources[1].Selector.Mode != base.Sources.Sources[1].Selector.Mode ||
					tw.Sources.Sources[1].Selector.Ref == base.Sources.Sources[1].Selector.Ref) {
				t.Fatalf("moving twin changed more than the exact selector ref: %+v", tw.Sources.Sources[1].Selector)
			}
			first, second := Validate(tw), Validate(tw)
			if first == nil || second == nil || first.Error() != second.Error() ||
				!strings.HasPrefix(first.Error(), "VERCEL-CONTRACT-") {
				t.Fatalf("unstable acceptance block: first=%v second=%v", first, second)
			}
		})
	}
	if _, err := NegativeTwin("unknown"); err == nil {
		t.Fatal("unknown twin accepted")
	}
	after, err := os.ReadDir(root)
	if err != nil || len(after) != len(before) {
		t.Fatalf("negative validation mutated disposable root: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

func TestMissingNoticeAndBindingTwinsFailWithoutMutation(t *testing.T) {
	base := Canonical()
	root := t.TempDir()
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{"missing-notice", "missing-binding"} {
		t.Run(fact, func(t *testing.T) {
			twin, err := NegativeTwin(fact)
			if err != nil {
				t.Fatal(err)
			}
			if reflect.DeepEqual(base.Pack, twin.Pack) || !reflect.DeepEqual(base.Sources, twin.Sources) || !reflect.DeepEqual(base.Legal, twin.Legal) {
				t.Fatalf("%s twin did not change exactly one Pack contract fact", fact)
			}
			first, second := Validate(twin), Validate(twin)
			if first == nil || second == nil || first.Error() != second.Error() ||
				!strings.HasPrefix(first.Error(), "VERCEL-CONTRACT-") {
				t.Fatalf("unstable %s rejection: first=%v second=%v", fact, first, second)
			}
		})
	}
	after, err := os.ReadDir(root)
	if err != nil || len(after) != len(before) {
		t.Fatalf("negative validation mutated disposable root: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

func TestValidateRejectsEverySealedFixtureGroup(t *testing.T) {
	tests := map[string]func(*Fixture){
		"blob identity": func(f *Fixture) { f.Blobs[0].SHA256 = strings.Repeat("0", 64) },
		"loader":        func(f *Fixture) { f.Loaders[0].AdaptedSHA256 = strings.Repeat("0", 64) },
		"compatibility": func(f *Fixture) { f.Compatibility.PatchPreserves = f.Compatibility.PatchPreserves[1:] },
		"alias policy":  func(f *Fixture) { f.Aliases.Selection = "implicit" },
		"snapshot":      func(f *Fixture) { f.SnapshotSHA256 = strings.Repeat("0", 64) },
		"selectability": func(f *Fixture) { f.CatalogSelectable = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := Canonical()
			mutate(&fixture)
			first, second := Validate(fixture), Validate(fixture)
			if first == nil || second == nil || first.Error() != "VERCEL-CONTRACT-EVIDENCE-BLOCKED" || first.Error() != second.Error() {
				t.Fatalf("unstable evidence validation: first=%v second=%v", first, second)
			}
		})
	}
}

func TestFixtureIsNotSelectableCatalogMaterial(t *testing.T) {
	f := Canonical()
	if f.CatalogSelectable {
		t.Fatal("fixture advertises catalog selection")
	}
	root := filepath.Join("..", "..", "bundle")
	catalog, err := capabilitypack.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range catalog.List() {
		if p.ID == "vercel" {
			t.Fatal("detached fixture entered selectable catalog")
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

package skilltemplate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReferencedSkillsReturnsCanonicalUniqueReferences(t *testing.T) {
	got, err := ReferencedSkills([]byte("use {{packy.skill:teach}} then {{packy.skill:tdd}} and {{packy.skill:teach}}"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tdd", "teach"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("references = %#v, want %#v", got, want)
	}
}

func TestReferencedSkillsRejectsMalformedExplicitDirective(t *testing.T) {
	for _, input := range []string{
		"{{packy.skill:}}",
		"{{packy.skill:Not-Canonical}}",
		"{{packy.skill:tdd",
		"{{packy.skill:tdd extra}}",
	} {
		if _, err := ReferencedSkills([]byte(input)); err == nil {
			t.Fatalf("ReferencedSkills(%q) unexpectedly succeeded", input)
		}
	}
}

func TestRenderTreePreservesBytesModesAndRendersOnlyExplicitTokens(t *testing.T) {
	root := t.TempDir()
	writeTemplateFile(t, root, "SKILL.md", "---\n# retained\nname: 'old-name' # retained too\ndescription: Calls /tdd and $teach\n---\nUse {{packy.skill:tdd}} and `teach`.\n", 0o640)
	writeTemplateFile(t, root, "scripts/run.sh", "#!/bin/sh\nexec {{packy.skill:teach}} \"$@\"\n", 0o750)
	writeTemplateFile(t, root, "assets/raw.bin", "\x00\x01unchanged", 0o644)

	files, err := RenderTree(root, "help", map[string]string{"help": "pack-codex-help", "tdd": "pack-codex-tdd", "teach": "pack-codex-teach"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{files[0].Path, files[1].Path, files[2].Path}; !reflect.DeepEqual(got, []string{"SKILL.md", "assets/raw.bin", "scripts/run.sh"}) {
		t.Fatalf("paths = %#v", got)
	}
	skill := string(files[0].Content)
	for _, want := range []string{"name: 'pack-codex-help' # retained too", "Calls /tdd and $teach", "Use pack-codex-tdd and `teach`."} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md does not contain %q:\n%s", want, skill)
		}
	}
	if files[0].Mode != 0o640 || files[2].Mode != 0o750 || string(files[1].Content) != "\x00\x01unchanged" {
		t.Fatalf("rendered metadata/bytes changed unexpectedly: %#v", files)
	}
}

func TestRenderTreeRejectsUnsafeOrAmbiguousTrees(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		writeTemplateFile(t, root, "SKILL.md", "---\nname: self\n---\n", 0o644)
		if err := os.Symlink("SKILL.md", filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderTree(root, "self", map[string]string{"self": "effective"}); err == nil {
			t.Fatal("RenderTree accepted a symlink")
		}
	})
	t.Run("duplicate name", func(t *testing.T) {
		root := t.TempDir()
		writeTemplateFile(t, root, "SKILL.md", "---\nname: one\nname: two\n---\n", 0o644)
		if _, err := RenderTree(root, "self", map[string]string{"self": "effective"}); err == nil {
			t.Fatal("RenderTree accepted duplicate name keys")
		}
	})
	t.Run("missing referenced name", func(t *testing.T) {
		root := t.TempDir()
		writeTemplateFile(t, root, "SKILL.md", "---\nname: self\n---\n{{packy.skill:other}}\n", 0o644)
		if _, err := RenderTree(root, "self", map[string]string{"self": "effective"}); err == nil {
			t.Fatal("RenderTree accepted an unresolved reference")
		}
	})
}

func writeTemplateFile(t *testing.T, root, relative, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

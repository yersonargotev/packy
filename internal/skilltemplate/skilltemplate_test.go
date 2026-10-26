package skilltemplate

import (
	"encoding/json"
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
		"{{packy.skill}}",
		"{{packy.skill :tdd}}",
		"{{ packy.skill:tdd}}",
		"{{packy.skill:}}",
		"{{packy.skill:Not-Canonical}}",
		"{{packy.skill:tdd",
		"{{packy.skill:tdd extra}}",
		"{{packy.skill:tdd}}}",
		"{{{packy.skill:tdd}}",
		"{{{{packy.skill:tdd}}}}",
	} {
		if _, err := ReferencedSkills([]byte(input)); err == nil {
			t.Fatalf("ReferencedSkills(%q) unexpectedly succeeded", input)
		}
	}
}

func TestReferencedSkillsIgnoresUnrelatedBracesCodeAndShell(t *testing.T) {
	input := "{{other.directive:tdd}} `packy.skill` ${packy_skill} $tdd /tdd map[packy.skill]"
	got, err := ReferencedSkills([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("references = %#v, want none", got)
	}
}

func TestRenderTreePreservesBytesModesAndRendersOnlyExplicitTokens(t *testing.T) {
	root := t.TempDir()
	writeTemplateFile(t, root, "SKILL.md", "---\n# retained\nname: 'old-name' # retained too\ndescription: Calls /tdd and $teach\n---\nUse {{packy.skill:tdd}} and `teach`.\n", 0o640)
	writeTemplateFile(t, root, "scripts/run.sh", "#!/bin/sh\nexec {{packy.skill:teach}} \"$@\"\n", 0o750)
	writeTemplateFile(t, root, "assets/raw.bin", "\x00\x01unchanged", 0o644)

	files, err := RenderTree(root, "help", map[string]string{"help": "pack-agents-help", "tdd": "pack-agents-tdd", "teach": "pack-agents-teach"})
	if err != nil {
		t.Fatal(err)
	}
	if got := filePaths(files); !reflect.DeepEqual(got, []string{metadataFileName, "SKILL.md", "assets/raw.bin", "scripts/run.sh"}) {
		t.Fatalf("paths = %#v", got)
	}
	skill := string(fileByPath(t, files, "SKILL.md").Content)
	for _, want := range []string{"name: 'pack-agents-help' # retained too", "Calls /tdd and $teach", "Use pack-agents-tdd and `teach`."} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md does not contain %q:\n%s", want, skill)
		}
	}
	if fileByPath(t, files, metadataFileName).Mode != 0o644 || fileByPath(t, files, "SKILL.md").Mode != 0o640 || fileByPath(t, files, "scripts/run.sh").Mode != 0o750 || string(fileByPath(t, files, "assets/raw.bin").Content) != "\x00\x01unchanged" {
		t.Fatalf("rendered metadata/bytes changed unexpectedly: %#v", files)
	}
}

func TestRenderTreeMetadataIsDeterministicAndScopedToUsedNames(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	for _, root := range []string{firstRoot, secondRoot} {
		writeTemplateFile(t, root, "SKILL.md", "---\nname: old\n---\nUse {{packy.skill:tdd}}.\n", 0o640)
		writeTemplateFile(t, root, "scripts/run.sh", "#!/bin/sh\n", 0o755)
	}
	names := map[string]string{"unrelated": "must-not-leak", "tdd": "effective-tdd", "self": "effective-self"}

	first, err := renderTree(firstRoot, "self", names, RendererVersion)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderTree(secondRoot, "self", map[string]string{"self": "effective-self", "tdd": "effective-tdd", "unrelated": "must-not-leak"}, RendererVersion)
	if err != nil {
		t.Fatal(err)
	}
	firstMetadata := fileByPath(t, first, metadataFileName)
	secondMetadata := fileByPath(t, second, metadataFileName)
	if !reflect.DeepEqual(firstMetadata, secondMetadata) {
		t.Fatalf("metadata is not deterministic:\nfirst=%s\nsecond=%s", firstMetadata.Content, secondMetadata.Content)
	}
	var metadata renderMetadata
	if err := json.Unmarshal(firstMetadata.Content, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RendererVersion != RendererVersion || !strings.HasPrefix(metadata.SourceContractSHA256, "sha256:") {
		t.Fatalf("metadata identity = %+v", metadata)
	}
	if want := map[string]string{"self": "effective-self", "tdd": "effective-tdd"}; !reflect.DeepEqual(metadata.EffectiveNames, want) {
		t.Fatalf("effective names = %#v, want %#v", metadata.EffectiveNames, want)
	}
	if strings.Contains(string(firstMetadata.Content), firstRoot) || strings.Contains(string(firstMetadata.Content), secondRoot) {
		t.Fatalf("metadata contains an absolute source path: %s", firstMetadata.Content)
	}
}

func TestRenderTreeMetadataChangesWhenNormalizedSourceContractChanges(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTemplateFile(t, firstRoot, "SKILL.md", "---\nname: original-one\n---\nBody.\n", 0o644)
	writeTemplateFile(t, secondRoot, "SKILL.md", "---\nname: original-two\n---\nBody.\n", 0o644)

	first, err := RenderTree(firstRoot, "self", map[string]string{"self": "effective"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderTree(secondRoot, "self", map[string]string{"self": "effective"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fileByPath(t, first, "SKILL.md"), fileByPath(t, second, "SKILL.md")) {
		t.Fatal("rendered SKILL.md differs; test requires the source difference to be normalized")
	}
	if reflect.DeepEqual(fileByPath(t, first, metadataFileName).Content, fileByPath(t, second, metadataFileName).Content) {
		t.Fatal("metadata did not bind the original source contract")
	}
}

func TestRenderTreeMetadataChangesWhenRendererIdentityChanges(t *testing.T) {
	root := t.TempDir()
	writeTemplateFile(t, root, "SKILL.md", "---\nname: original\n---\nBody.\n", 0o644)
	names := map[string]string{"self": "effective"}

	first, err := renderTree(root, "self", names, "skill-template/test-v1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderTree(root, "self", names, "skill-template/test-v2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fileByPath(t, first, "SKILL.md"), fileByPath(t, second, "SKILL.md")) {
		t.Fatal("rendered SKILL.md changed with renderer identity")
	}
	if reflect.DeepEqual(fileByPath(t, first, metadataFileName).Content, fileByPath(t, second, metadataFileName).Content) {
		t.Fatal("metadata did not bind the renderer identity")
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
	t.Run("reserved metadata file", func(t *testing.T) {
		root := t.TempDir()
		writeTemplateFile(t, root, "SKILL.md", "---\nname: self\n---\n", 0o644)
		writeTemplateFile(t, root, metadataFileName, "authored\n", 0o644)
		if _, err := RenderTree(root, "self", map[string]string{"self": "effective"}); err == nil {
			t.Fatal("RenderTree accepted the reserved metadata file")
		}
	})
	t.Run("reserved metadata directory", func(t *testing.T) {
		root := t.TempDir()
		writeTemplateFile(t, root, "SKILL.md", "---\nname: self\n---\n", 0o644)
		if err := os.Mkdir(filepath.Join(root, metadataFileName), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderTree(root, "self", map[string]string{"self": "effective"}); err == nil {
			t.Fatal("RenderTree accepted the reserved metadata directory")
		}
	})
}

func filePaths(files []File) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

func fileByPath(t *testing.T, files []File, path string) File {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("rendered tree has no %q: %#v", path, files)
	return File{}
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

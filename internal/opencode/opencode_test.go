package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDetectsPackInstructionReferenceWithoutReadingTarget(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "opencode.json")
	target := filepath.Join(root, "instructions", "pack.md")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"relative": filepath.Join("instructions", "pack.md"),
		"absolute": target,
		"glob":     filepath.Join(root, "instructions", "*.md"),
	}
	for name, reference := range tests {
		t.Run(name, func(t *testing.T) {
			content := fmt.Sprintf("{\"instructions\":[%q]}\n", reference)
			if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			inspection, err := Inspect(configPath, target)
			if err != nil {
				t.Fatal(err)
			}
			if !inspection.HasPackyInstruction {
				t.Fatalf("reference %q was not detected", reference)
			}
		})
	}
}

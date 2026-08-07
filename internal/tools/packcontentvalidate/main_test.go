package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesRepositoryPackContent(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--repository-root", root}); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidatesOneNamedPack(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	_ = root
	temporary := t.TempDir()
	packDir := filepath.Join(temporary, "bundle", "packs", "argote")
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	invalid := `{"id":"not argote","version":"1.0.0","description":"Argote","selectable":true,"surfaces":["codex"],"external_requirements":[],"resources":[],"exclusions":[]}`
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}

	err = run([]string{"--repository-root", temporary, "--pack", "argote"})
	if err == nil || !strings.Contains(err.Error(), `Pack "not argote" field id`) {
		t.Fatalf("run() error = %v", err)
	}
}

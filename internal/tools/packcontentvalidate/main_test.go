package main

import (
	"path/filepath"
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

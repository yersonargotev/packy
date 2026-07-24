package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

func TestReadCanonicalRegularRejectsMalformedNoncanonicalAndNonregularInputs(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"malformed":    "{",
		"noncanonical": `{"allowed":true,"reasons":[]}`,
		"trailing":     "{\n  \"allowed\": true,\n  \"reasons\": []\n}\n{}",
		"empty":        "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var decision governancedrift.GateDecision
			if _, err := readCanonicalRegular(path, &decision); err == nil {
				t.Fatal("untrusted input accepted")
			}
		})
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	var decision governancedrift.GateDecision
	if _, err := readCanonicalRegular(directory, &decision); err == nil {
		t.Fatal("nonregular input accepted")
	}
}

func TestWriteExclusiveRequiresNewOutOfTreePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(filepath.Join(cwd, "candidate.json"), []byte("{}\n")); err == nil {
		t.Fatal("in-tree output accepted")
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := writeExclusive(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(path, []byte("{}\n")); err == nil {
		t.Fatal("existing output replaced")
	}
}

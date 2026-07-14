package engrambin

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTopologyOwnsCandidatePrecedenceAndObservation(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	fallback := filepath.Join(t.TempDir(), "fallback", "bin", "engram")
	topology := NewTopology(prefix)

	wantCandidates := []string{filepath.Join(prefix, "bin", "engram")}
	if got := topology.Candidates(); !reflect.DeepEqual(got, wantCandidates) {
		t.Fatalf("Candidates = %#v, want %#v", got, wantCandidates)
	}
	observation := topology.Observe(func(string) (string, error) { return fallback, nil })
	if observation.Homebrew() != nil || observation.Installed() {
		t.Fatalf("noncanonical PATH fallback was accepted: %#v", observation)
	}

	writeTopologyExecutable(t, wantCandidates[0])
	observation = topology.Observe(func(string) (string, error) { return fallback, nil })
	if observation.Homebrew() == nil || observation.Homebrew().Path != wantCandidates[0] || !observation.Installed() {
		t.Fatalf("Homebrew observation = %#v", observation)
	}
}

func writeTopologyExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

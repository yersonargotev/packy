package reportredaction

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorRedactsSealedEnvironmentValuesAndPreservesCause(t *testing.T) {
	cause := errors.New("command --env TOKEN=secret and --env=OTHER=value failed: server rejected secret; merged-document")
	got := Error(cause, [][]string{{"--env", "TOKEN=secret", "--env=OTHER=value"}}, []string{"merged-document"})
	if !errors.Is(got, cause) {
		t.Fatal("redacted error does not preserve its cause")
	}
	if strings.Contains(got.Error(), "secret") || strings.Contains(got.Error(), "value") || strings.Contains(got.Error(), "merged-document") {
		t.Fatalf("redacted error = %q", got)
	}
	if !strings.Contains(got.Error(), "TOKEN=<redacted>") || !strings.Contains(got.Error(), "OTHER=<redacted>") {
		t.Fatalf("redacted error = %q", got)
	}
}

func TestTextRedactsSealedEnvironmentValuesAndPayloads(t *testing.T) {
	got := Text("observed TOKEN=secret and merged-document", [][]string{{"--env", "TOKEN=secret"}}, []string{"merged-document"})
	if strings.Contains(got, "secret") || strings.Contains(got, "merged-document") {
		t.Fatalf("redacted text = %q", got)
	}
	if !strings.Contains(got, "TOKEN=<redacted>") {
		t.Fatalf("redacted text = %q", got)
	}
}

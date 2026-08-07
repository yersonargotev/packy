package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func firstJSONDocument(t *testing.T, stream string) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stream), "\n")
	if len(lines) == 0 || !json.Valid([]byte(lines[0])) {
		t.Fatalf("missing first JSON document:\n%s", stream)
	}
	return []byte(lines[0])
}

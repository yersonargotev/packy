package offlinevalidation

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunFailsClosedWhenWorkerBoundarySetupFails(t *testing.T) {
	_, requestPath, responsePath := writeWorkerFixture(t)
	var stderr bytes.Buffer

	exitCode := runWithBoundary(
		[]string{requestPath, responsePath},
		io.Discard,
		&stderr,
		func() error { return errors.New("boundary unavailable") },
	)

	if exitCode != 1 || !strings.Contains(stderr.String(), "install offline validation boundary: boundary unavailable") {
		t.Fatalf("runWithBoundary() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(responsePath); !os.IsNotExist(err) {
		t.Fatalf("response exists after boundary setup failure: %v", err)
	}
}

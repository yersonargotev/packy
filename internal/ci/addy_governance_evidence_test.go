package ci_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadAddyGovernanceEvidenceAdmitsOnlyTrustedWorkflowArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	fake := filepath.Join(t.TempDir(), "gh")
	log := filepath.Join(t.TempDir(), "gh.log")
	writeFile(t, fake, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$GH_LOG"
if [[ "$1" == api && "$2" == repos/owner/repo/actions/workflows/addy-governance.yml ]]; then
  printf '%s\n' 77
elif [[ "$1" == api && "$2" == repos/owner/repo/actions/artifacts* ]]; then
  printf '%s\n' '{"artifacts":[{"id":5,"name":"addy-governance-pr-222-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expired":false,"updated_at":"2026-07-24T17:00:00Z","workflow_run":{"id":99}}]}'
elif [[ "$1" == api && "$2" == repos/owner/repo/actions/runs/99 ]]; then
  if [[ "${WRONG_WORKFLOW:-}" == 1 ]]; then workflow=78; else workflow=77; fi
  printf '{"event":"pull_request_target","status":"completed","conclusion":"success","workflow_id":%s,"repository":{"full_name":"owner/repo"}}\n' "$workflow"
elif [[ "$1" == run && "$2" == download && "$3" == 99 ]]; then
  while (($#)); do
    if [[ "$1" == --dir ]]; then destination="$2"; break; fi
    shift
  done
  for name in blocking-issues.json canonical-issues.json evaluation.json gate.json observation.json; do
    printf '{}\n' >"$destination/$name"
  done
  if [[ "${EXTRA_FILE:-}" == 1 ]]; then
    printf 'unexpected\n' >"$destination/extra.txt"
  fi
else
  exit 1
fi
`)
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, extra ...string) ([]byte, []byte, error) {
		t.Helper()
		output := filepath.Join(t.TempDir(), "governance")
		cmd := exec.Command("/bin/bash",
			filepath.Join(root, "scripts", "download-addy-governance-evidence.sh"),
			"--repo", "owner/repo",
			"--pull-request", "222",
			"--merge-sha", strings.Repeat("a", 40),
			"--output-dir", output,
		)
		cmd.Env = append(os.Environ(),
			"GH_BIN="+fake,
			"GH_LOG="+log,
			"ADDY_GOVERNANCE_POLL_ATTEMPTS=1",
			"ADDY_GOVERNANCE_POLL_INTERVAL=0",
		)
		cmd.Env = append(cmd.Env, extra...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.Bytes(), err
	}

	stdout, stderr, err := run(t)
	if err != nil {
		requests, _ := os.ReadFile(log)
		t.Fatalf("trusted artifact was rejected: %v\n%s\nrequests:\n%s", err, stderr, requests)
	}
	if len(stdout) != 0 {
		t.Fatalf("trusted artifact downloader contaminated gate stdout: %q", stdout)
	}
	if _, stderr, err := run(t, "WRONG_WORKFLOW=1"); err == nil ||
		!strings.Contains(string(stderr), "trusted Addy governance evidence was not available") {
		t.Fatalf("wrong workflow artifact was admitted: err=%v\n%s", err, stderr)
	}
	if _, stderr, err := run(t, "EXTRA_FILE=1"); err == nil ||
		!strings.Contains(string(stderr), "trusted Addy governance evidence was not available") {
		t.Fatalf("artifact with an extra file was admitted: err=%v\n%s", err, stderr)
	}
}

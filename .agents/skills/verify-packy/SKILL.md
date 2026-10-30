---
name: verify-packy
description: Verify Packy changes with canonical validation and real CLI or TUI journeys, observable effects, and retained evidence.
---

# Verify Packy

Use this capability for changed Packy behavior. Read the [feature map](features/README.md), select every affected entry point, and extend the map for an uncovered user path. Repository review, validation, and authority requirements remain in [development guidance](../../../docs/agents/development.md). This skill does not coordinate development or authorize integration.

## Launch

Run from the candidate repository root. Use a new private scratch directory and a separate durable evidence directory. Resolve Go caches before changing `HOME`. Keep the variables below in one owned shell session through Cleanup. Do not use the operator's project or configuration for mutation journeys.

```sh
export PACKY_VERIFY_ROOT="$(pwd)"
export GOCACHE="${GOCACHE:-$(go env GOCACHE)}"
export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"
export GOPATH="${GOPATH:-$(go env GOPATH)}"
export PACKY_VERIFY_SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/packy-verify.XXXXXX")"
export HOME="$PACKY_VERIFY_SCRATCH/home"
export XDG_CONFIG_HOME="$PACKY_VERIFY_SCRATCH/xdg"
export PACKY_VERIFY_EVIDENCE="$(mktemp -d "${TMPDIR:-/tmp}/packy-verify-evidence.XXXXXX")"
export PYTHONDONTWRITEBYTECODE=1
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$PACKY_VERIFY_SCRATCH/project"
go build -o "$PACKY_VERIFY_SCRATCH/packy" ./cmd/packy
```

Keep evidence outside scratch and any path cleanup removes. The binary is ready when the build succeeds and Doctor passes. Start each TUI journey in a fresh private PTY. For project journeys, use a disposable Git worktree or repository under scratch, with only the required reviewed fixture. Do not copy credentials.

## Doctor

```sh
"$PACKY_VERIFY_SCRATCH/packy" version
go version -m "$PACKY_VERIFY_SCRATCH/packy"
```

Check the binary's build information against the candidate revision and dirty state. Record `git rev-parse HEAD`, `git status --short`, `git diff HEAD --binary`, and hashes of relevant untracked files. Compare the binary build metadata with the source identity. If the binary is stale or the build identity cannot be established, rebuild and repeat Doctor. Run Doctor before every new drive and after unexpected behavior.

## Drive

Run the applicable canonical validation from [development guidance](../../../docs/agents/development.md), including `./scripts/validate-packy.sh` when required. Record the command, outcome, and logs. Validation complements real journeys; neither substitutes for the other. For local projection staging, fingerprint, publication, or rollback changes also run [verify-projection-batches](../verify-projection-batches/SKILL.md) and report its actual verdict and limits.

Use the feature map's real commands or private PTY steps. The helper waits for each literal terminal string before sending its corresponding keys. It records raw terminal output and terminates only its own process if a step fails. For example:

```sh
(
  cd "$PACKY_VERIFY_SCRATCH/project"
  "$PACKY_VERIFY_ROOT/.agents/skills/verify-packy/scripts/pty-drive.py" "$PACKY_VERIFY_SCRATCH/packy" "$PACKY_VERIFY_EVIDENCE/tui.raw" --step 'Workstation' '?' --step 'hide' q
)
```

The helper requires Python 3 on macOS or Linux. `ESC`, `TAB`, and `ENTER` send those keys; other values send literal text. It waits at most 30 seconds per step and 15 seconds for exit. Never drive a different run's PTY. If a mapped path requires unavailable catalog, client, network, or setup, record the attempted route and missing prerequisite as incomplete. Do not substitute another entry point.

## Evidence

For each journey retain its feature and entry point, candidate identity, commands and inputs, stdout, stderr, exit status, terminal transcript when applicable, and observed final state. Compare the relevant sandbox files before and after both read-only and mutation paths. Record absent effects as well as expected writes. Inspect independent state after a mutation, not just the success screen. Redact secrets. Mocks are acceptable only at an existing production boundary, and any skipped external behavior remains unverified. Observe the actual effects of a dry-run rather than trusting its name.

## Cleanup

Wait for every process started by the run to exit. The PTY helper owns its child and cleans it on failure. Remove only the scratch directory created by Launch after checking its path. Never remove the evidence directory. Confirm the evidence files still exist and can be read after cleanup.

```sh
rm -r -- "$PACKY_VERIFY_SCRATCH"
test -s "$PACKY_VERIFY_EVIDENCE/tui.raw"
```

Use `/maintain-verification-skill` to audit and update this map as Packy changes.

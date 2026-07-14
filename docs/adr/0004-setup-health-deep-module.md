# ADR 0004: Matty setup health is a deep internal module

## Status

Accepted.

## Context

`matty doctor` diagnoses the base installation across lifecycle state, managed
skill links, Engram, Codex, and OpenCode. Today `internal/cli` owns both those
health semantics and the command adapter. That makes CLI helpers the effective
domain seam and prevents another adapter from consuming setup health without
depending on CLI-owned types.

[ADR 0003](0003-core-lifecycle-deep-module.md) already provides separate,
read-only observations of lifecycle state and managed-skill ownership. It
explicitly does not classify overall setup health. This decision consumes that
observation seam without moving health classification into core lifecycle or
changing lifecycle planning, persistence, recovery, or application.

## Decision

Create `internal/setuphealth` as the sole owner of base-installation health
orchestration and diagnosis. It owns diagnostic names, ordering, severities,
details, remediation text, and summary policy for lifecycle state, managed
skill links, Engram, Codex, and OpenCode.

The module presents one deep operation, with its external dependencies supplied
at construction:

```text
Diagnose(config) -> Report
```

`Diagnose` returns only a structured `Report`, not a report-and-error pair.
Individual observation failures are health facts represented by the established
WARN or FAIL check. Diagnosis continues after such a failure and returns the
most complete report possible.

The report is a self-contained, point-in-time snapshot containing:

- structured context for the resolved home, configuration home, state file and
  observed state status, and agent skills directory;
- ordered checks with stable name, severity, and complete actionable detail;
- summary status and PASS, WARN, and FAIL counts.

Renderers do not reobserve the workstation or reinterpret diagnoses.

## Ownership and dependencies

- Setup health reads the core lifecycle observation seam and reuses stable
  observations or diagnoses owned by lifecycle, Engram, prompt, and OpenCode
  packages. It does not absorb their persistence or artifact parsing policy.
- Filesystem and symlink observations remain real implementation details and
  are tested through sandboxed paths rather than broad filesystem interfaces.
- Active observations are allowed only when read-only: PATH lookup, bounded
  executable-version inspection, and process listing.
- Executable lookup uses a least-authority seam exposing only `LookPath`.
  Existing `engrambin.Facts` supplies substituted version and process facts.
  Setup health does not depend on the CLI `Runner` or receive arbitrary command
  execution.
- Configuration contains only the paths and environment values required for
  base setup diagnosis. The existing broader workstation path model is not
  redesigned by this decision.

Diagnosis must not write files, create directories, repair state, install or
remove artifacts, change configuration, run lifecycle actions, or terminate
processes.

## CLI responsibility and compatibility

`internal/cli` adapts resolved paths into setup-health configuration, invokes
`Diagnose`, renders the returned snapshot, and maps report failures to the
existing command error. Human formatting, JSON encoding, `io.Writer` failures,
and exit adaptation remain CLI responsibilities.

This is a behavior-preserving architecture change. It retains exactly:

- human context and check output;
- JSON schema version 1 and report kind, including the existing omission of
  report context from JSON v1;
- check names, order, severities, details, and remediation language;
- summary fields, counts, and status rules;
- warnings as non-fatal and failures as fatal only after the complete report is
  rendered.

The migration replaces the old owner rather than wrapping it. After callers and
semantic tests move, CLI-owned report and summary types, report builders, check
classifiers, and compatibility aliases are removed. The CLI retains only small
tests for rendering, output errors, exit mapping, and command wiring; the full
semantic scenario matrix belongs at the setup-health report seam.

## Consequences

- Setup health policy gains one owner and one report-level test surface.
- Reports remain internally consistent because shared lifecycle state is
  observed once and reused throughout one diagnosis.
- Observation failures cannot hide unrelated diagnoses.
- Another adapter can consume setup health without importing Cobra or CLI
  types.
- Tests substitute only nondeterministic workstation facts while exercising
  real domain observers against sandboxed filesystems.

## Subsequent refinement

[ADR 0006](0006-own-workstation-layout-by-domain.md) refines this ADR's
temporary shared-workstation boundary. Setup health now receives detached
observations from the owners of lifecycle state, skills, host configuration,
and Engram topology instead of receiving a CLI-composed path configuration.
The diagnostic policy, report seam, rendering boundary, and compatibility
commitments decided here remain unchanged.

## Exclusions

- No new checks, fields, commands, flags, repair actions, or automatic
  remediation.
- No change to diagnostic output, schema, paths, summary, or exit behavior.
- No change to core lifecycle behavior or the shared workstation path model.
- No broad filesystem abstraction or per-domain mock interfaces.
- No arbitrary command execution during diagnosis.
- No capability-pack status, convergence, blockers, projections, or readiness;
  those remain owned by `internal/capabilitypack`.
- No forwarding setup-health module or duplicate CLI diagnosis policy in the
  final architecture.

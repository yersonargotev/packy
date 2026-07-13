Status: resolved
Blocked by: 01

# Route doctor through the setup health module

## Answer

Added `internal/setuphealth` as the sole production owner of base setup
diagnosis. Its constructor accepts only executable lookup and bounded Engram
facts, and its single `Diagnose(Config) Report` operation returns structured
context, ordered checks, and a calculated summary after one shared lifecycle
state observation. The module reuses lifecycle, Engram, prompt, and OpenCode
observations and keeps filesystem diagnosis read-only and best-effort.

The productive `doctor` command now maps resolved CLI paths into the module's
minimal configuration, renders the self-contained report through CLI-owned
human and JSON v1 adapters, and preserves the existing unhealthy exit mapping.
Report-level tests cover healthy and degraded state, skills, Engram, Codex,
OpenCode, observation failures, ordering, details, remediation, summaries, and
read-only sandbox behavior. The ticket 01 command contracts and the full Go
test suite pass unchanged.

The old CLI report builders and private diagnosis helpers remain without a
production caller, explicitly reserved for deletion by ticket 03.

## Parent

[Matty setup health deepening specification](../spec.md)

## What to build

Introduce the complete setup health owner and route the production `doctor`
command through its self-contained report. Users must receive the same base
setup diagnosis while state, skills, Engram, Codex, OpenCode, ordering,
remediation, and summary policy become exercisable through one deep interface.

## Acceptance criteria

- [ ] One independent setup health module owns base-installation observation orchestration, diagnostic classification, check ordering, complete remediation text, and summary calculation.
- [ ] The module exposes one Diagnose operation returning only a self-contained Report with structured context, ordered checks, and summary.
- [ ] The module receives only the paths and environment values needed for diagnosis rather than the CLI's broad workstation-path structure.
- [ ] Executable discovery crosses a least-authority lookup seam, while bounded Engram version and process observations use substitutable facts; the module does not depend on arbitrary command execution.
- [ ] Lifecycle state is observed once per diagnosis and the same snapshot is reused for state, skill, delegated setup, and report-context decisions.
- [ ] Existing lifecycle, Engram, prompt, and OpenCode owner observations are reused; filesystem behavior is exercised through sandboxed paths rather than broad per-domain mocks.
- [ ] Diagnose continues after observation failures and preserves the existing WARN-versus-FAIL behavior while returning the most complete report possible.
- [ ] Diagnose performs only active read-only observations and cannot write, repair, install, remove, mutate configuration, execute lifecycle actions, or terminate processes.
- [ ] The CLI adapts resolved configuration into the setup health module, renders its report through the existing human and JSON adapters, and preserves existing exit behavior.
- [ ] Report-level tests cover the full semantic matrix for state, skills, Engram, Codex, OpenCode, partial failures, ordering, details, and summary.
- [ ] The contract established by ticket 01 remains unchanged and the complete repository test suite passes.
- [ ] Any temporarily retained CLI diagnosis implementation has no production caller and is clearly left for contraction in ticket 03; no forwarding compatibility interface is added.

## Out of scope

- Deleting the now-unreferenced CLI diagnosis implementation and obsolete tests; ticket 03 performs the contraction.
- Changing user-visible behavior, capability-pack status, lifecycle semantics, or workstation path design.
- Adding repair actions or new health checks.

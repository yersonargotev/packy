# Deliver-issue operational frictions

Date: 2026-08-23

## Scope and conclusion

This report investigates three frictions observed while delivering issue
[#728](https://github.com/yersonargotev/packy/issues/728) through pull request
[#733](https://github.com/yersonargotev/packy/pull/733). They have independent
causes and owners:

1. clarify Go cache capture order in the issue-delivery workflow and prefer the
   repository's existing validation wrappers;
2. describe an exact-path cleanup form that complies with Codex's built-in
   execution policy;
3. optionally narrow ADR 0031's absolute documentation wording in a separate
   documentation change.

No Packy product code, test helper, generic review prompt, thin
`deliver-issue` skill, or Codex configuration change is warranted.

## 1. Go sandbox friction: preserve provisioned caches before changing `HOME`

### Evidence

The workflow requires the smallest relevant checks to run with user paths
sandboxed and later repeats that `HOME` and `XDG_CONFIG_HOME` must be sandboxed
when a check resolves or writes user paths. It does not specify when Go's
ambient cache locations must be resolved
([local implementation loop](../../../workflows/issue-delivery.md#local-implementation-loop)).

Both repository wrappers already encode the missing ordering. They resolve
`GOCACHE`, `GOMODCACHE`, and `GOPATH` before exporting sandboxed user roots:

- [`validate-packy.sh`](../../../scripts/validate-packy.sh) captures them at
  lines 19-21 and changes `HOME`/`XDG_CONFIG_HOME` at lines 30-35;
- [`validate-changed.sh`](../../../scripts/validate-changed.sh) captures them at
  lines 135-137 and changes the user roots at lines 138-140.

The test helper has the same dependency. `GoOfflineEnv` uses an explicit outer
`GOMODCACHE` when present; otherwise `outerModuleCache` runs the absolute Go
executable with the current outer `HOME` to obtain `go env GOMODCACHE`
([`environment.go`](../../../internal/testprocess/environment.go), lines
130-164). The accepted issue-724 research defines that outer download cache as
a provisioned build input, not ambient mutable configuration, and requires it
to be resolved before the child environment is constructed
([issue-724 evidence](issue-724-subprocess-environments-2026-08-22.md#go-offline-variant),
lines 143-147).

**Local reproduction evidence:** a focused `go test ./internal/ci` with a new
`HOME` but without first preserving the outer Go cache locations produced a
deterministic dependency/cache failure. The same focused check succeeded when
the outer cache values were captured first. This is an invocation-order error,
not a defect in `internal/testprocess`.

### Decision

Clarify the workflow: for a manual focused Go check, capture the effective Go
cache inputs before exporting a sandboxed `HOME`/`XDG_CONFIG_HOME`; prefer
`./scripts/validate-changed.sh` or `./scripts/validate-packy.sh` whenever its
scope fits. Do not change Packy code, tests, or helpers. The thin skill must not
copy this operational detail: the workflow is explicitly the orchestration
source of truth, while [the skill](../../../.agents/skills/deliver-issue/SKILL.md)
only bootstraps and gates its phases.

## 2. Cleanup friction: built-in Codex execpolicy, not Packy or the OS sandbox

### Evidence

OpenAI documents sandbox mode and approval policy as distinct controls:
sandbox mode defines what a command can technically do, while approval policy
defines when Codex must ask before acting
([Agent approvals & security](https://learn.chatgpt.com/docs/agent-approvals-security)).
OpenAI also documents execpolicy rules separately, including `forbidden`, and
states that the most restrictive matching decision wins
([Rules](https://learn.chatgpt.com/docs/agent-configuration/rules)).

**Local reproduction evidence:** the installed first-party `codex-cli 0.149.0`
was configured with `sandbox_mode = "danger-full-access"` and
`approval_policy = "never"`; `codex doctor --json` reported approval policy
`Never` and filesystem sandbox `unrestricted`. No user or project `.rules` file
accounted for the rejection. The installed binary contains `default.rules`,
`core/src/exec_policy.rs:1119`, and the exact rejection text:

> `rm -f style commands are not permitted. Use a safer approach`

An attempted command containing `rm -rf` was returned as
`CreateProcess { message: "Rejected(...)" }`, so no child process started. An
equivalent check using an ownership-validated temporary path and
`find <owned-path> -depth -delete` started and completed successfully.

Taken together, those observations identify a built-in default Codex
execpolicy decision. They exclude Packy, the macOS filesystem sandbox, and the
user's Codex configuration as the rejecting layer. The conclusion about the
built-in rule is an inference from the first-party binary and the before-
CreateProcess behavior; the public rules documentation explains the decision
model but does not enumerate this embedded default rule.

### Decision

Clarify workflow cleanup to use a validated, workflow-owned exact path and a
policy-compatible operation such as `find <owned-path> -depth -delete`. Keep
the ownership and post-merge verification gates already required by
[`Verify and clean up`](../../../workflows/issue-delivery.md#verify-and-clean-up).
Do not weaken Codex configuration and do not add a Packy wrapper script that
hides deletion. Optionally send upstream Codex ergonomics feedback: the current
message labels a broad `rm -rf` rejection as “rm -f style” without naming the
safe exact-path alternative that the same policy accepts.

## 3. ADR ambiguity: safe current state, recurring review cost

### Evidence

ADR 0031 says absolutely that checked-out documentation contains “only this
accepted architecture record and current operating guidance”
([ADR 0031](../../adr/0031-simplify-packy-around-reviewed-packs.md), lines
50-51). Its originating issue was narrower: issue
[#521](https://github.com/yersonargotev/packy/issues/521) required removing
ADRs, guides, evidence, research, workflow instructions, and agent instructions
*dedicated exclusively to retired systems*.

Accepted ADR 0034 expressly allows historical material to retain an obsolete
command spelling as evidence while current guidance teaches only the current
form ([ADR 0034](../../adr/0034-make-packy-root-namespace-pack-oriented.md),
lines 47-49). Issue [#728](https://github.com/yersonargotev/packy/issues/728)
then established a durable research directory without granting it architectural
authority. The resulting [`docs/research` contract](../README.md) says that
evidence is durable, dated, and non-normative and that accepted ADRs and
`CONTEXT.md` remain authoritative.

**Local reproduction evidence:** the final Standards review for PR #733 first
reported a hard conflict based on ADR 0031's absolute sentence. After
adjudication against issue #521 and ADR 0034, the same reviewer withdrew the
finding and returned PASS. The durable PR evidence records the adjudicated PASS
([PR #733 final evidence](https://github.com/yersonargotev/packy/pull/733#issuecomment-5386686035)).
The repository is therefore safe as-is, but the observed false positive proves
a continuing interpretation and review cost.

### Decision

Prefer a small, separate repository-documentation clarification to ADR 0031,
or an explicit narrowing note attached to it, stating that its consequence
excludes durable non-normative evidence governed by the current research
contract. Leaving the text unchanged is safe but retains review cost. Do not
change `code-review` or `deliver-issue` prompts: both correctly surfaced and
adjudicated the apparent conflict, and teaching them this single historical
exception would duplicate repository authority in generic or orchestration
instructions.

## Decision matrix and prioritized follow-ups

| Priority | Friction / follow-up | Classification | Decision | Why this owner |
| --- | --- | --- | --- | --- |
| P1 | Clarify Go cache capture order and prefer existing validation wrappers | **Workflow docs** | Change `workflows/issue-delivery.md`; no helper or test change | The wrappers and helper already implement the intended contract; only manual orchestration is underspecified. |
| P1 | Specify ownership-validated exact-path cleanup compatible with execpolicy | **Workflow docs** | Change `workflows/issue-delivery.md`; do not add a deletion wrapper | Cleanup command selection is operational workflow behavior, and the workflow already owns resource identity and cleanup completion. |
| P2 | Narrow ADR 0031's absolute documentation consequence | **Repository docs** | Make a small separate ADR clarification; leaving it unchanged is safe | The ambiguity is in repository authority wording and caused measurable adjudication cost. |
| P3 | Report the imprecise rejection message / missing safe-alternative hint upstream | **Product development (Codex)** | Optional upstream ergonomics feedback only | The embedded default rule and its message are first-party Codex behavior, not Packy behavior. |
| No action | Modify Packy code, tests, or `internal/testprocess` | **Product development (Packy)** | Do not change | Reproduction and source contracts show correct existing behavior. |
| No action | Weaken sandbox, approvals, or execpolicy | **Codex config** | Do not change | The filesystem was already unrestricted; the default forbidden decision is a separate, more restrictive layer. |
| No action | Expand the thin skill or generic review prompts | **Workflow docs / repository tooling** | Do not change | The workflow owns operational detail, and the review correctly escalated an ambiguity for adjudication. |

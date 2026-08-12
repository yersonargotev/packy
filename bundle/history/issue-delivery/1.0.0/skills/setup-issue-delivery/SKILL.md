---
name: setup-issue-delivery
description: Configure or audit this repository's issue-delivery policy.
disable-model-invocation: true
---

# Setup Issue Delivery

Configure the repository-owned policy consumed by `$deliver-issue`. This is a
policy interview and audit, not an issue delivery. It may create or update only
`docs/agents/issue-delivery.md`; it does not change code, Git state, GitHub,
CI, repository settings, or other repository instructions.

## Bootstrap

1. Resolve the current repository root with `git rev-parse --show-toplevel`.
   Stop if it is not a Git repository. Resolve every consumer path from that
   absolute root and run every consumer command with that root as its working
   directory.
2. Before interpreting a policy, read the canonical contract at
   `../deliver-issue/references/policy-contract.md`, relative to this skill's
   directory. Stop and report the missing or unreadable dependency if that path
   does not resolve. The contract is the only authority for the policy schema,
   required decisions, permitted `None` values, and validation rules; do not
   reproduce it here.
3. Discover the repository facts the contract calls for. Read every applicable
   instruction file on the path from the repository root to the current
   directory, the existing `docs/agents/issue-delivery.md` when present, and
   the repository's visible tracker, CI, and validation configuration. Inspect
   Git remotes, the default-base indication, and available local tools only
   with read-only commands.
4. Record the exact initial Git status before writing. Produce a fact sheet
   that separates observed facts, inherited instructions, existing policy
   values, and unknowns. Cite each fact by file path or command output. Never
   turn an inference into a policy decision.

## Audit

When a policy already exists, compare every contract field to it. Report each
field as one of: satisfied, missing, contradictory, stale, or unverified. A
contradiction includes a value that conflicts with current repository facts or
applicable instructions. Preserve useful evidence from the existing document,
but treat no existing value as approved merely because it was already written.

## Frontier interview

Resolve decisions in dependency order. A round contains only decisions whose
dependencies are already facts or resolved decisions; do not ask later
questions early. For each round, state the available evidence, the exact
contract fields it would settle, and the proposed value when the evidence
supports one. Ask the user to decide any value that the evidence cannot settle.

After each answer, update the fact sheet and expose the next frontier. Repeat
until every required field is resolved. Use `None` only where the canonical
contract permits it and the user explicitly chooses it. Stop rather than
inventing an answer when a required decision remains unresolved.

## Command evidence

For every policy command, record its purpose, working directory, and evidence
that it is available and safe in this repository. Prefer commands already
declared by repository configuration or instructions. Validate a candidate
without changing Git or remote state: inspect its declaration and executable
availability, then run it only when it is local, non-interactive, bounded, and
does not mutate repository, Git, credentials, infrastructure, or GitHub state.
If those conditions cannot be demonstrated, leave the command unverified and
return it to the frontier; do not write it as validated.

## Write and verify

1. Render `docs/agents/issue-delivery.md` exactly in the canonical contract's
   structure, using the resolved values and their evidence. Keep the canonical
   workflow in `deliver-issue`; this file supplies only repository policy.
2. Re-read the result and audit it against the canonical contract. It is ready
   only when all required decisions are resolved, every referenced pointer
   exists or is an explicit permitted `None`, every command is validated, and
   no placeholder or unresolved question remains.
3. Run `python3 scripts/validate_policy.py <repository-root>`, resolving the
   script from this skill directory. This structural check supplements rather
   than replaces command-safety evidence and the semantic audit.
4. Compare the final Git status with the exact initial status. Require every
   new change made during this run to be confined to
   `docs/agents/issue-delivery.md`; preserve and report every pre-existing
   change without altering it. Inspect the policy diff, then report the audit
   result, the created or changed policy path, and every deliberate `None`.

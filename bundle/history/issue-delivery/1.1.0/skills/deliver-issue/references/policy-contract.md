# Repository policy contract

`docs/agents/issue-delivery.md` is the consumer repository's delivery policy.
It supplies local authority that neither GitHub nor repository files can reveal.
GitHub remains authoritative for the repository identity, default branch,
checks, reviews, conversations, protection, merge queue, and enabled merge
methods; inspect those facts on every delivery instead of caching them here.

## Required structure

Render these headings in this order. Replace every angle-bracket placeholder;
the finished policy contains no placeholder text.

```md
# Issue delivery policy

## Authority

- **Approval condition:** <exact observable GitHub condition>
- **Qualification additions:** <additional local requirements, or `None`>
- **Corrective issue closure after verified merge:** <Authorized or Escalate>

## Normative contracts

| Role | Path |
| --- | --- |
| Repository instructions | <existing path> |
| Issue tracker | <existing path or `None`> |
| Architecture and domain | <existing path or `None`> |
| Validation | <existing path> |
| Standards review | <existing path> |
| Spec review | <existing path or `None`> |

## Local proof

- **Canonical validation:** <command or deterministic procedure>
- **Focused proof:** <how to derive the smallest relevant proof>
- **Manual verification:** <how to derive changed-surface verification, including an explicit non-applicability criterion>
- **User-path isolation:** <required sandboxing, or `None`>

## Advisory checks

<How operational failures and substantive findings are distinguished and handled.>

## Sensitive surfaces

| Surface | Owning policy |
| --- | --- |
| <surface or `None`> | <existing path or `None`> |

Use `None` when the repository has no specialist assurance policies.

## External-effect boundary

<Repository-specific effects that require separate authority, in addition to
the core release, deployment, real migration, production-configuration, and
other irreversible external-effect exclusions. Use `None` for no additions.>
```

An existing instruction file may satisfy multiple normative roles; repeat its
path rather than its content. `None` is permitted only where the template says
so. A command is valid only with its working directory, purpose, and safe
availability established from repository evidence; record those details next
to the command when they are not obvious from the named Validation contract.

The policy may add stricter repository controls. It may not weaken these core
invariants: explicit one-issue authority, approval before delivery, isolated
work, exact-SHA proof, two independent final review axes, protected merge
without bypass, post-merge verification, and ownership-safe cleanup.

Treat missing, ambiguous, materially changed, stale, or conflicting policy as
a decision-ready exception. State the exact gap, affected phase, evidence,
available choices, and recommended resolution. Do not invent an approval
condition, validation command, review rule, or integration authority.

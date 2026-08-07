# ADR 0030: Bind Pack Source delivery to one approved issue

## Status

Accepted.

## Context

The Pack Source workflow owns its pull-request body and invalidates readiness
after human edits. Conventional issue delivery requires the integrating pull
request to close exactly one approved issue. Free-form body edits cannot safely
connect these two ownership models.

## Decision

Pack Source schema suite v2.2.0 adds optional `closing_issue` authority to v2
requests and artifacts. The value is one canonical GitHub issue URL in the
publication repository, not free-form Markdown.

Before every publication mutation and decision-readiness observation, the
workflow freshly requires that exact issue to be open and labeled
`status:approved`. The managed pull-request body then renders exactly one
GitHub-recognized `Closes <URL>` line. The request digest, canonical evidence,
publication record, metadata hash, and readiness identity bind that same URL;
issue-state change invalidates readiness.

Governance treats the two records as conjunctive authority only for canonical
Pack Source proposals: the successful bound synchronization run proves who
produced the exact proposal head, while the one approved closing issue provides
change authority. Other automation, urgent-revert, and private-security
exceptions remain mutually exclusive with approved-issue authorization.

Requests that omit `closing_issue` render no closing keyword. Missing,
malformed, cross-repository, closed, unapproved, changed, or ambiguous issue
identity fails closed. Automation retains no authority to create or explicitly
close issues, merge, enable auto-merge, or accept arbitrary body content.

Codex project instructions are separately represented as surface-neutral,
resource-scoped marked contributions in the project `AGENTS.md`. Codex and
OpenCode use identical contribution bytes and shared-projection identity so
project installation contributor accounting preserves the file until the last
surface contribution is removed. Global projections remain unchanged.

## Consequences

- A decision-ready synchronization proposal can satisfy conventional issue
  delivery without surrendering managed metadata ownership.
- Existing requests have no implicit issue authority and cannot claim closure.
- Publish gains read-only issue observation; all repository and pull-request
  mutation boundaries remain unchanged.
- Published schema bytes remain immutable; v2.2.0 is a new complete five-file
  suite with instance `schema_version: 2`.

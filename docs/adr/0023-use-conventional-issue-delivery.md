# ADR 0023: Use conventional issue delivery

## Status

Accepted.

Supersedes ADR 0020 and ADR 0021.

## Context

Packy's protected branch already provides the durable integration boundary for
repository changes. Approved issues establish authority, pull requests expose
the complete candidate, required CI checks correctness and policy, and review
records Standards and Spec findings against the final candidate.

Maintainers need one predictable path for ordinary, sensitive, and governance
changes. A separate local orchestration lifecycle adds another state machine,
evidence format, and recovery surface without strengthening the protected
branch itself.

Release publication remains a distinct operation because it mutates release
and package-manager state after repository integration.

## Decision

All Packy issue work uses the conventional protected pull-request workflow:

1. an approved GitHub issue states the objective and acceptance criteria;
2. a branch and pull request contain the candidate and close that issue;
3. required CI passes for the final pull-request head;
4. traceable Standards and Spec review covers that final candidate;
5. review conversations are resolved; and
6. branch protection governs merge.

The workflow does not vary by a delivery risk profile. Sensitive changes may
require additional evidence or specialist review when their owning security,
migration, governance, or publication policy says so, but they use the same
pull-request lifecycle.

Release publication is not authorized by issue delivery. It continues through
the dedicated release workflow and its existing controls after the release
candidate has integrated.

## Consequences

- Maintainers use one repository-native workflow for every issue.
- GitHub holds the durable authority, candidate, CI, review, and merge record.
- No local delivery-run state or delivery-profile label is part of Packy's
  governance contract.
- Required CI, branch protection, and release controls remain unchanged.

## Enforcement

Repository instructions and issue-tracker documentation describe only the
protected pull-request path. Governance continues to require an approved
closing issue and the required protected-main checks. Repository validation
must remain green before a change is reported complete.

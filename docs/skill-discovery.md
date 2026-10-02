# Skill discovery and host coexistence

Packy selects a reviewed resource definition from the requested CLI surface.
One logical skill can therefore have different bodies for Codex, Claude Code,
and OpenCode. Separate target directories preserve physical ownership, but a
host can also discover another host's directories. Preview checks both concerns.

## Native targets

| Surface | Global skills | Project skills |
| --- | --- | --- |
| Codex | `$HOME/.agents/skills/<name>` | `.agents/skills/<name>` |
| Claude Code | `$HOME/.claude/skills/<name>` | `.claude/skills/<name>` |
| OpenCode | `$XDG_CONFIG_HOME/opencode/skills/<name>` | `.opencode/skills/<name>` |

When `XDG_CONFIG_HOME` is unset, OpenCode's configuration root defaults to
`$HOME/.config`. Names above are projected names; the logical resource ID
remains the Pack selection and dependency identity. Skills include their
entire selected tree, including references, helper files, and metadata.

Codex and Claude use separate native roots. OpenCode also discovers `.agents`
and `.claude` compatibility roots in global and project scope. Stable OpenCode
has no verified deterministic winner for divergent skills with the same
frontmatter `name`; a different directory basename alone does not isolate them.
Packy does not rely on Codex choosing a duplicate winner either.

## What preview can establish

Packy checks divergent same-name skills across OpenCode and Codex/Claude roots
in either installation order, including global/project combinations and
duplicate names within the native roots. The observation covers global roots,
the current project, and ancestor directories up to the Git worktree boundary.
It does not establish discovery for every other worktree, custom configured
root, IDE launch, plugin, or future host version.

Unverified simultaneous intent blocks before mutation. Global previews report
`host-discovery`; project previews report `host_discovery_conflict`. Readiness
retains the existing `runtime-usability` condition with `usable=unknown` and
reason `runtime-unobservable`, with an explicit discovery explanation, rather
than claiming the host loaded the intended skill. Inspect the reported paths and
names, then choose nonconflicting intent or remove an affected Pack installation
through its ordinary previewed lifecycle. User-owned content remains user-owned.
Two Packs claiming one physical target still conflict, and drift protection
continues to cover the complete receipt-owned tree.

`OPENCODE_DISABLE_EXTERNAL_SKILLS=1` is a user-controlled requirement for one
OpenCode launch. The real-host probe demonstrated native-only discovery in
that process. It is not a persistent host configuration contract: Packy does
not edit shell startup or application launch settings, infer other launches
from its own environment, or use that flag to bypass the coexistence block.

## Verified baseline and limits

The 2026-10-02 probes used Codex `0.160.0`, Claude Code `2.1.220`, and OpenCode
`1.18.15`. These are evidence versions, not a promise that every later version
or launch context behaves identically. Claude `2.1.287` and OpenCode `1.18.34`
were available at the time but were not the binaries exercised.

Actual binaries demonstrated native Codex/Claude discovery and OpenCode
compatibility discovery. Separate global/project Codex runs dispatched the
native body and executed a helper read. Separate global/project OpenCode runs
loaded the native body and read its helper under the explicit external-skills
disabling flag. Local deterministic providers requested those real host tool
calls; no model inference occurred. Claude expanded its selected native body
and read its auxiliary file through permitted dynamic context in separate
global/project probes. Those Claude requests deliberately received HTTP 400
from the local recorder and exited one after the verified pre-inference
expansion. Successful model-backed workflows across all three hosts remain
unverified.
The [dated evidence](research/evidence/skill-discovery-2026-10-02.md) records
methods, versions, sources, and the exact limits. A successful Packy projection
or update is not evidence that a complete host workflow succeeded.

For an existing shared-root installation, follow the
[clean adoption procedure](catalog-adoption.md) before upgrading and reinstalling.

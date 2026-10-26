---
name: packy-cli
description: Discover and obtain a requested Catalog skill through Packy, including Pack-qualified requests and ambiguous skill names.
---

# Packy CLI

Use the Packy CLI as a resource provider. You read the returned instructions and
follow the selected procedure; retrieval itself does not execute that procedure.
This definition uses the Agents projection convention for Codex. Pass `--surface agents` explicitly on every
host-specific inspection and retrieval, including nested calls.

## Discover and select

1. Run `packy list --json` to enumerate the selected local catalog. Inspect
   relevant Packs with `packy show <pack> --surface agents --json`.
   Match the request against qualified identities, effective descriptions,
   reviewed compatibility, dependencies, and known prerequisites. Preserve each
   `<pack>:<skill>` identity throughout selection.
2. Select a unique match directly. When multiple skills match, ask the user to
   choose among the qualified candidates and explain their relevant differences.
   If the user qualified the request, use that identity. Installed state, native
   aliases, and modified installed copies do not rank or replace Catalog Resources.
3. Require reviewed on-demand compatibility for this surface and the consuming client. A native binding alone
   does not establish compatibility. If unsupported, explain the reported reason
   and request a reviewed adaptation rather than attempting native-path access.

Complete when one compatible qualified Catalog Resource has been selected, or
report the unresolved ambiguity or compatibility limit.

## Obtain and read

Run:

```sh
packy get skill <pack>:<skill> --surface agents --json
```

Require a successful versioned `skill-access` report. Retain `qualified_skill`,
`pack_version`, `surface`, `snapshot_id`, `entry_document`, `resource_root`,
`dependencies`, and `prerequisites`. Read the returned entry document first, then
only the supporting files needed by the procedure, resolving relative references
from their document locations within the prepared resource tree. Prepared
same-Pack and cross-Pack dependencies retain their qualified owners and versions;
reviewed access references point to their usable entry documents.

For every nested request, keep the initiating surface and snapshot:

```sh
packy get skill <dependency-pack>:<dependency-skill> --surface agents --snapshot <snapshot_id> --json
```

A new independent request uses the selected local snapshot. Explicit refresh does
not change an active procedure's snapshot. Missing retained content must fail;
restore that exact retained revision instead of substituting a newer one.
Snapshots and prepared paths have no automatic expiry or cleanup. Manual removal
can invalidate them; preserve them while a procedure is using them.

Complete when the entry and needed supporting/dependency documents are readable
and the initiating immutable identity is available for nested calls.

## Follow the procedure

Check reported prerequisites before execution: declarations remain unverified,
and an empty list does not prove runtime readiness. Missing content runtimes do
not prevent obtaining instructions. Preserve the selected skill's approvals,
domain assumptions, tool requirements, stopping rules, and retirement behavior.
Resource availability does not authorize external effects or prove workflow success.

Choose the user's intended target directory explicitly. Keep immutable resource
locations separate from that target and from writable scratch/output locations.
Run helpers with their documented target argument or environment, and place
intermediate files and script dependencies in separate writable scratch space.
Preserve Catalog sources and installed copies. Native invocation continues using
its installed definition, including local modifications.

## Acquisition and errors

Discovery and retrieval use local content, work outside Git after acquisition,
and do not implicitly refresh, download, activate/install requested Packs, modify
target projects, execute scripts, or install runtimes/dependencies. If no local
catalog exists, explain explicit acquisition with `packy init` or
`packy catalog refresh`; perform acquisition only when authorized. Surface or
identity errors require correcting the selection, not guessing another host or
revision. Read `get skill` structured errors and discovery's stderr/nonzero exit.
If Packy itself is unavailable, explain that the CLI must be installed explicitly;
do not automatically provision it or any content runtime.

## Project ownership

This project-owned adaptation derives from `packy:packy-cli` 1.0.0 in Catalog
Snapshot `bbf3eecfd7afb85d36586e6f4e18801d2eef47b6`. Its former `codex` project
receipt was retired with the previous binary before adopting the Agents surface.
It is maintained in this repository rather than represented as an installation
from a newer Catalog Snapshot. See [LICENSE](LICENSE) for the retained MIT notice.

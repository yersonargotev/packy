# Packy future: native ecosystems and cross-host portability

## Assessment

Packy's weakest long-term proposition is installing a collection of skills into
several agents. Native plugin managers and a published cross-vendor package
standard already address much of that task. A more plausible recurring job is
helping a small team establish and verify the exact agent capability setup its
repository expects, while explaining what is configured, authorized, and actually
usable. That is a hypothesis to validate, not demonstrated demand.

The important strategic distinction is between portable files and a working
workflow. Packy should benefit from standard packaging rather than treating
format translation as permanent product value. Conversely, building a universal
agent administration layer would expose a small project to an expensive stream
of host-specific changes.

## Scope and temporal boundaries

This is non-normative evidence assessed on September 13, 2026, with a horizon
from September 2027 to September 2029. The objective is useful, sustainable
open-source software for developers and small teams with a small maintainer
budget. The repository baseline is its [README](../../../README.md),
[domain context](../../../CONTEXT.md), and [research policy](../README.md).
External facts describe official specifications, vendor documentation, and the
upstream Vercel Skills repository at that date. They do not prove every feature
works in every released binary, account, region, or client version. No host
runtime compatibility tests were performed.

Several pages are moving documentation without an explicit publication date.
For example, the old OpenAI Codex plugin URL redirects to ChatGPT Learn. The
retrieval date is not the release date. Exact future adoption rates, maintainer
capacity, user retention, and willingness to maintain another configuration
tool remain unknown. Forecasts and recommendations are explicitly identified.

## Verified current evidence

### 1. Standardization extends beyond individual skill files

Agent Skills specifies a directory with `SKILL.md`, required name and description,
and optional scripts, references, and assets. Environment compatibility is a
short descriptive field; `allowed-tools` is experimental and implementation
support can vary. The specification provides syntax validation, not a guarantee
that instructions achieve their intended result on each model or host.
[Agent Skills specification](https://agentskills.io/specification).

Agent Plugins 1.0.0 is a published, vendor-neutral package standard for shared
skills and MCP servers. Its public overview explicitly leaves distribution,
installation, permissions, user experience, and client-specific features to
clients. Its initial steering committee includes maintainers from Amazon,
Cursor, Microsoft, OpenAI, and Vercel.
[Agent Plugins overview](https://agent-plugins.org/).

The normative specification establishes root `plugin.json`, fixed component
locations, package path containment, and client-managed persistent plugin data.
It standardizes only skills and MCP. Conformance permits supporting just one
component type; unsupported components are ignored, and independent components
continue loading after another fails. Authorization and credential storage are
client-managed. The closed portable manifest does not define dependency graphs,
content receipts, or project lockfiles. Path containment explicitly does not
sandbox a subprocess.
[Agent Plugins 1.0.0, sections 4–11](https://agent-plugins.org/specification).

The compatible-client directory lists Cursor, GitHub Copilot, ChatGPT/Codex,
VS Code, and others, with component and transport support. Claude Code and
OpenCode are not listed on the retrieved page; that absence does not establish
non-support. Vendor-specific support should be checked directly before making
compatibility promises.
[Compatible clients](https://agent-plugins.org/compatible-clients).

**Inference:** Basic package layout translation is already being commoditized.
However, a successfully loaded portable package can still lack part of the
workflow the author intended. Treating format compatibility as operational
readiness would overstate what the standard guarantees.

### 2. Native managers already cover most obvious installer features

| Ecosystem | Verified native capability | Implication for Packy |
| --- | --- | --- |
| Claude Code | Plugins combine skills, agents, hooks, MCP, LSP, and monitors. | A multi-resource bundle is not a unique product category. |
| Codex / ChatGPT | Shared public plugin directory; CLI plugin browser; marketplace distribution; portable Agent Plugins packaging. | Discovery, ordinary installation, and standard package authoring have first-party paths. |
| GitHub Copilot | Plugins across CLI, cloud agent, and desktop app; repository declarations and marketplaces. | Small-team setup can already be committed to a repository. |
| Cursor | Standard and Cursor-specific plugin formats, project/user scope, team marketplaces and component management. | A polished resource-selection interface competes with native UI. |

The rows are supported and qualified below; their implications are this report's
analysis, not statements from the vendors.

Claude's reference documents the broad bundle types above and a local plugin
cache. This makes a second manager for the same native resources potentially
redundant, unless it delivers an additional contract that users need.
[Claude plugin reference](https://code.claude.com/docs/en/plugins-reference).

Claude marketplaces provide discovery, version tracking, automatic updates, and
multiple source types. Git plugin sources support exact commit SHA pinning;
archive sources support SHA-256 verification. Marketplace source pinning differs
from individual plugin pinning. Therefore, neither version selection nor
download integrity alone is a defensible claim of unique Packy value.
[Claude marketplace documentation](https://code.claude.com/docs/en/plugin-marketplaces).

Claude exposes user, project, and local installation scopes, plus administrator
managed scope. Project scope adds shared settings to the repository. Its official
marketplace is available by default, while authors can distribute their own.
[Claude plugin installation](https://code.claude.com/docs/en/discover-plugins).

OpenAI documents a shared public plugin directory for ChatGPT and Codex, a Codex
CLI `/plugins` browser, and workspace marketplace import. The retrieved page says
the IDE extension does not support plugins. Installing web-side hooks does not
deploy their scripts into the execution environment.
[Plugins in ChatGPT and Codex](https://learn.chatgpt.com/docs/plugins).

OpenAI recommends a root Agent Plugins manifest for new portable packages, with
OpenAI-specific metadata and hooks in an extension. Legacy Codex manifests remain
accepted. Local and repository marketplaces are distinct from public publication
and vary by surface. Plugin installation does not automatically authorize hooks;
the current hook definition must be trusted.
[OpenAI package authoring](https://developers.openai.com/plugins/build/plugins).

GitHub recommends Agent Plugins 1.0 for new plugins unless configurable component
paths are required. Shared skills and MCP coexist with Copilot-specific agents,
hooks, commands, and LSP definitions. CLI supports installation commands or
`enabledPlugins` in user or repository settings; cloud agent uses repository
settings. Administrators can also define plugin standards.
[About Copilot plugins](https://docs.github.com/en/copilot/concepts/agents/about-plugins).

Copilot CLI provides install, uninstall, update, enable, disable, marketplace
management, and separate skill installation. Its loader also recognizes older
Claude marketplace paths. This is direct evidence that cross-ecosystem
accommodation can be supplied inside the host, not only by a third-party adapter.
[Copilot CLI plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference).

Cursor loads portable Agent Plugins and richer Cursor plugins. Its native format
additionally supports rules, agents, commands, hooks, and variables.
[Cursor plugin reference](https://prod.cursor.com/docs/reference/plugins).
Its installation UI supports project and user scope. Team marketplaces exist on
Teams and Enterprise plans, with default-off, default-on, and required install
modes. Members can publish personal skills, but that publication does not bundle
other skills they reference. Public listings receive manual review.
[Cursor plugin management](https://prod.cursor.com/docs/plugins).

**Inference:** Reusable bundles, reviewed catalogs, scope separation, and
versioned team distribution are already competitive necessities. A small
independent manager cannot assume it owns these categories because it implements
them carefully. A native manager is often sufficient for a single-host team.

### 3. Independent distribution already has a strong baseline

Vercel's open-source Skills CLI installs selected skills into selected agents,
supports project/global scope, and offers canonical copies with symlinks or
independent copies. Its README documents finding, listing, updating, removing,
and using a skill without installation.
[Vercel Skills README](https://github.com/vercel-labs/skills/blob/main/README.md).
The upstream contributor guide additionally documents restoration from
`skills-lock.json`, source tracking, and GitHub skill-folder hashes. Those are
documented internals, not runtime-verified equivalence to Packy's ownership and
drift guarantees.
[Vercel Skills contributor guide](https://github.com/vercel-labs/skills/blob/main/AGENTS.md).

OpenCode already discovers skills from its own directories, `.claude/skills`,
and `.agents/skills`, at project and global scope. Unknown frontmatter fields are
ignored, and skill access permissions are host-configured.
[OpenCode skills](https://opencode.ai/docs/skills/).
Its documented code-plugin system loads local JavaScript/TypeScript or configured
npm packages, automatically installing npm dependencies with Bun. This is a
different extension mechanism from a portable skill package.
[OpenCode plugins](https://opencode.ai/docs/plugins/).

**Inference:** Adding more target directories is a poor use of scarce maintainer
time unless a real user workflow requires it. Shared directories and existing
CLIs already make the simple case inexpensive. A declaration called a lockfile
is also insufficient differentiation: the useful question is which exact
failure each implementation detects or prevents.

### 4. Real portability gaps remain, but they are costly to own

OpenAI's Claude plugin submission guide requires adaptation of commands and
agents into skills, adaptation of hooks, and replacement of Claude installation
variables. Marketplace approvals do not transfer. It distinguishes local MCP
execution from public remote-server submission and explicitly calls for testing
in a clean environment. This is unusually concrete first-party evidence that
package portability does not imply workflow equivalence.
[OpenAI Claude-plugin conversion guide](https://developers.openai.com/plugins/guides/submit-claude-plugin).

**Inference:** Cross-host friction is not imaginary, but much of it involves
permissions, execution environments, or semantic changes rather than file paths.
An adapter that silently substitutes a skill for an agent might install cleanly
and still change behavior. Packy's opportunity is to state supported behavior
and unsupported requirements precisely. A promise to translate everything would
create more obligations than a small team can plausibly maintain.

## What might remain useful for a small open-source project

The following are proposals derived from the evidence, not verified unmet demand.

| Recurring user job | Smallest useful Packy contribution | Strong counterargument |
| --- | --- | --- |
| Join a repository or change machines | Explain the reviewed capability setup and verify its materialized state. | A committed plugin declaration and one README command may already suffice. |
| Upgrade agent or Pack | Show changed owned content and unresolved readiness obligations before use. | Hosts may expose increasingly good diagnostics; Packy must avoid reimplementing them. |
| Share a workflow across two hosts | Verify a bounded, tested capability contract on the supported pair. | Most workflows may need only a shared skill directory; semantic testing is expensive. |
| Recover from edited or conflicting configuration | Identify ownership and drift without overwriting unrelated files. | If native plugins keep all files in their cache, Packy-created projections can be the source of the problem. |
| Review a capability change in CI | Check the committed resource closure, provenance, notices, and projection integrity offline. | A small validation script might deliver enough value without another installer. |

The current repository already names configured, authorized, and usable
readiness separately, and describes content receipts, collision checks, drift
protection, `verify`, and `audit`. Those are better starting assets than a wider
catalog. They are documented capabilities of this repository, not independent
proof of user demand or implementation correctness.
[Current Packy README](../../../README.md), [current domain model](../../../CONTEXT.md).

A possible focused proposition is: **review and verify the agent setup a
repository depends on, across the few hosts the team actually uses**. Success
would mean saved setup/debugging time, safe upgrades, and understandable failure
reports. It need not require cloud accounts, a new registry, enterprise policy,
or running the agent itself.

However, this proposition is narrower than a universal package manager and must
earn its additional manifest. Packy currently bundles the reviewed catalog in its
binary and updates toward its current Pack version. That coupling may keep
operation simple but can slow content delivery. Before redesigning it, measure
actual delays and maintenance effort; native marketplace publication may solve
distribution with less new machinery.

## One-to-three-year scenarios: forecasts, not facts

| Scenario | Expected pressure on Packy | Decision signal |
| --- | --- | --- |
| Portable core grows and native distribution improves | Simple installation and format conversion lose value. | More hosts load the same packages without patches; users stop needing setup help. |
| Core converges while hooks, agents, and environments remain different | A bounded verifier or tested capability catalog retains value. | Recurring failures are about missing authority, dependencies, or changed execution semantics. |
| Developers mostly settle on one host per team | Cross-host maintenance costs exceed benefit. | Active users rarely enable a second surface; one-host native setup wins comparison trials. |
| Teams regularly switch hosts or mix local/cloud execution | Reproducible intent and diagnostic evidence become more useful. | Users return after upgrades and onboarding, rather than only at first installation. |

Continuing package convergence is the strongest directional expectation
because a published standard and multiple implementations already exist. This
assessment assigns no numerical probabilities: there is no adoption dataset
supporting them. Persistence of host-specific behavior is also plausible because
vendors already document separate extension systems. Neither expectation proves
that Packy is the best place to solve the remaining problems.

## Low-cost validation and scope discipline

These experiments should precede a larger roadmap:

1. Use one existing Pack and two current supported hosts. Ask a few external
   developers or small teams to perform onboarding, one update, and recovery from
   an edited owned file. Compare Packy with native installation plus a README.
2. Record setup time, failure causes, repeat usage, and maintainer support time.
   Count repeated successful use, not download totals or catalog size.
3. Test whether `verify` or `audit` alone provides most of the value. If so, keep
   installation narrow and let host-native distribution carry more of the work.
4. Prototype one portable export only when a current Pack and actual users need
   it. Use the existing standard; do not invent another general plugin format.
5. Set a small explicit maintenance budget. Reject a new surface unless it has a
   recurring user, a concrete contract, and affordable validation. Contribute
   generic portability fixes upstream where that removes future maintenance.

If users return only to install a few skills once, native distribution is the
better long-term home for that content. Maintaining Packy as a personal or small
community utility can still be a valid outcome. Evidence of repeated repository
setup and upgrade problems would justify a stronger public commitment; a broad
ecosystem opportunity by itself does not.

## Open questions and disconfirmation

- No comparative runtime test established how native managers handle local
  cache edits, reproducible dependency closure, or partial readiness. Do not
  claim that all competitors lack Packy's exact guarantees.
- No evidence here establishes how many developers use multiple hosts often
  enough to justify Packy. Conduct user observation before adding surfaces.
- No evidence establishes that reviewed Pack content improves task success
  across models. Byte integrity and provenance do not measure workflow quality.
- A future Agent Plugins revision, native project verification command, or
  existing open-source manager could absorb the proposed niche. Recheck these
  sources before an architectural commitment, and review the hypothesis after
  actual recurring use rather than defending the current implementation.

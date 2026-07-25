# Upstream inventory: `vercel-labs/agent-skills`

## Research question

What is the complete reusable capability contract under `skills/` in Vercel's upstream repository, which files are authoritative or generated, what runtime authority and dependencies do they carry, and what licensing evidence governs redistribution to Codex, OpenCode, and Claude Code?

## Immutable source identity

- Repository: [`vercel-labs/agent-skills`](https://github.com/vercel-labs/agent-skills)
- Default branch observed: `main`
- Inspected commit: [`7c180d9044c9ae2b442b567aad4e42a28dd5ed62`](https://github.com/vercel-labs/agent-skills/tree/7c180d9044c9ae2b442b567aad4e42a28dd5ed62)
- Inspected tree: `0557b732b3907e51bed3fd7898095f8097a0834e`
- Commit timestamp: `2026-07-24T12:25:23+02:00`
- Commit subject: `Merge pull request #310 from vercel-labs/fix-install-command`
- Inspection date: 2026-07-24

All repository citations below are pinned to this SHA. `main` was used only to resolve the initial identity and must not be used as a synchronization authority.

## Method and safety boundary

I resolved `refs/heads/main` with `git ls-remote`, cloned without checking out a branch, detached at the exact SHA, and inspected bytes statically with Git, text tools, Python's ZIP reader, and read-only GitHub repository metadata. I did **not** execute any upstream script, package-manager command, CI workflow, Vercel command, deployment, authentication flow, or prompt capability. No user configuration or external project state was changed.

The pinned `skills/` tree has 308 tracked files, nine capability directories, five root ZIP files, and one nested ZIP. It has no submodules or Git LFS pointers. The repository has no tracked root or skill-local `LICENSE`, `COPYING`, or `NOTICE` file. GitHub's repository metadata reports no detected license.

## Executive findings

1. The source contract is **nine skill directories**, not every child of `skills/`. Five root ZIPs are distribution snapshots; three are stale relative to their same-named directory. The nested `deploy-to-vercel/Archive.zip` is another, older distribution snapshot with macOS metadata. ZIPs are not independent capabilities and must not override source directories.
2. The complete compatible set for Codex, OpenCode, and Claude Code is the nine directory packages with each package's dependency closure. Upstream states the repository follows the Agent Skills format and installs with `npx skills add`; it does not ship host-specific Codex/OpenCode/Claude manifests ([README](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/README.md#L1-L7), [installation](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/README.md#L153-L165)). Host projection is therefore discovery of standard `SKILL.md` packages, not ZIP activation or a host-specific upstream contract.
3. Four guidance packages (`composition-patterns`, `react-best-practices`, `react-native-skills`, `react-view-transitions`) declare `license: MIT` in `SKILL.md`. The other five do not. The root README's one-word “MIT” statement is evidence of intent, but there is no MIT permission text, copyright notice, or repository-level license artifact. Redistribution authority is consequently incomplete and should be resolved by upstream clarification or a tracked license before publication.
4. Markdown is operational authority. Several skills instruct an agent to edit consumer code, inspect secrets, run package managers/CLIs, link projects, mutate environment variables, push Git, or deploy. `deploy-to-vercel` contains two executable network/deployment scripts; `vercel-optimize` contains a large Node program that runs Vercel CLI commands and writes audit artifacts. Synchronization must remain byte-only and inert.
5. `skills.sh.json` is catalog metadata, not a complete or internally consistent capability authority: it groups eight identifiers, omits `writing-guidelines`, and uses public identifiers that differ from several directory/frontmatter names ([catalog](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills.sh.json)). Filesystem directories plus `SKILL.md` frontmatter are the authoritative capability inventory at this commit.

## Complete `skills/` entry classification

Every direct child of [`skills/`](https://github.com/vercel-labs/agent-skills/tree/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills) is classified below.

| Entry | Classification | Package contract / justification |
|---|---|---|
| `composition-patterns/` | **Included capability** | `vercel-composition-patterns` v1.0.0. Include `SKILL.md`, 10 rule/support Markdown files, `metadata.json`, `README.md`, and generated `AGENTS.md` (14 files total). Rules are the modular source; `SKILL.md` routes to selected rules. |
| `deploy-to-vercel/` | **Included capability** | `deploy-to-vercel` v3.0.0. Include current `SKILL.md` and `resources/deploy.sh`, `resources/deploy-codex.sh` (direct executable dependencies). Exclude nested `Archive.zip` from the projected capability as a generated duplicate. |
| `deploy-to-vercel.zip` | **Generated duplicate** | Exact byte duplicate of the four non-directory files in `deploy-to-vercel/`, including the nested archive. Retain only as upstream provenance if desired; never install beside/extract over the source directory. |
| `react-best-practices/` | **Included capability** | `vercel-react-best-practices` v1.0.0. Include `SKILL.md`, 72 rule/support files, `metadata.json`, `README.md`, and generated `AGENTS.md` (76 files). Individual `rules/*.md` are progressive-disclosure dependencies and source for the aggregate. |
| `react-best-practices.zip` | **Generated duplicate (stale)** | 75 archived files; 73 equal current directory bytes, but archived `SKILL.md` and `AGENTS.md` differ. It is not the authority for the pinned commit. |
| `react-native-skills/` | **Included capability** | `vercel-react-native-skills` v1.0.0. Include `SKILL.md`, 38 rule/support files, `metadata.json`, `README.md`, and generated `AGENTS.md` (42 files). README calls the public concept “react-native-guidelines,” while catalog ID/frontmatter/directory use three related names; preserve exact upstream names rather than inventing an alias. |
| `react-view-transitions/` | **Included capability** | `vercel-react-view-transitions` v1.0.0. Include `SKILL.md`, four required `references/*.md`, `metadata.json`, `README.md`, and `AGENTS.md` (8 files). |
| `react-view-transitions.zip` | **Generated duplicate (stale)** | Eight archived files; only `metadata.json` and `AGENTS.md` are byte-equal. Six source/reference files differ, so the directory wins. |
| `vercel-cli-with-tokens/` | **Included capability** | `vercel-cli-with-tokens` v1.0.0. Single `SKILL.md`; no auxiliary files. It is prompt authority for authenticated Vercel CLI, Git, environment, domain, and deployment operations. |
| `vercel-cli-with-tokens.zip` | **Generated duplicate** | Its one `SKILL.md` is byte-equal to the directory source. Not an independent capability. |
| `vercel-optimize/` | **Included capability** | `vercel-optimize` v1.2.0. Include the entire 156-file directory: five root files, 75 `lib/` files, 15 `scripts/` files, and 61 reference/playbook/support files. These are one tightly coupled executable workflow, not separable skills. |
| `web-design-guidelines/` | **Included capability** | `web-design-guidelines` v1.0.0. Single `SKILL.md`; its implicit runtime dependency is a **moving** raw `main/command.md` fetched from the separate first-party `vercel-labs/web-interface-guidelines` repository ([source instruction](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/web-design-guidelines/SKILL.md#L13-L26)). The package is structurally complete but not reproducible/offline-complete. |
| `web-design-guidelines.zip` | **Generated duplicate (stale)** | Archived `SKILL.md` differs from the directory. Exclude from projection. |
| `writing-guidelines/` | **Included capability** | `writing-guidelines` v1.0.0. Single `SKILL.md`; its implicit runtime dependency is moving `main/command.md` in the separate first-party `vercel-labs/writing-guidelines` repository ([source instruction](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/writing-guidelines/SKILL.md#L13-L26)). It is omitted from `skills.sh.json`, but its valid `SKILL.md` makes it a source-tree capability. |

### Nested generated artifact

[`skills/deploy-to-vercel/Archive.zip`](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/deploy-to-vercel/Archive.zip) is **excluded generated duplicate**, not a dependency. Its two scripts equal the directory versions, its `SKILL.md` is stale, and it carries `__MACOSX/._*` metadata. The outer `deploy-to-vercel.zip` embeds this archive, creating a duplicate-inside-duplicate hierarchy.

## Dependency and generation model

### Self-contained rule packages

`composition-patterns`, `react-best-practices`, and `react-native-skills` have the same structure: `SKILL.md` selects individual `rules/*.md`; `metadata.json` supplies version/organization/references; `README.md` is human documentation; `AGENTS.md` is an aggregate generated by repository build tooling. The first-party build configuration explicitly maps these three rule trees to generated `AGENTS.md` outputs, and calls test cases build artifacts rather than skill content ([build configuration](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/packages/react-best-practices-build/src/config.ts)). That generator lives outside `skills/`, so it is source-maintenance authority and **excluded** from a consumer pack.

`react-view-transitions` instead depends directly on its four `references/` files; its `SKILL.md` requires the implementation workflow and CSS recipes rather than treating them as optional reading ([dependency links](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/react-view-transitions/SKILL.md#L49-L55)).

### Executable dependency packages

`deploy-to-vercel` has two executable-mode shell resources:

- [`deploy.sh`](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/deploy-to-vercel/resources/deploy.sh) packages a project and uploads it to Vercel's claimable-deployment service.
- [`deploy-codex.sh`](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/deploy-to-vercel/resources/deploy-codex.sh) provides the Codex-oriented variant.

Both are direct dependencies named by [`SKILL.md`](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/deploy-to-vercel/SKILL.md#L163-L218). Copying the skill without them breaks declared fallback flows. Including them does not authorize automatic execution.

`vercel-optimize` is effectively an application packaged as a skill. `SKILL.md` requires Node.js 20+, Vercel CLI v53+, authentication, project linkage, and often Observability Plus, then directs a multi-stage script pipeline ([prerequisites](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/vercel-optimize/SKILL.md#L20-L30), [pipeline](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/vercel-optimize/SKILL.md#L60-L79)). Its `scripts/` import `lib/`, and both consume schemas, allow-listed docs, doctrine, playbooks, and support topics under `references/`. Partial copying is unsafe.

### Implicit network dependencies

`web-design-guidelines` and `writing-guidelines` fetch live content from separate repositories on every use. The pinned skill SHA therefore pins the loader instructions but **not the reviewed rule content**. A future Packy design must either accept and disclose this moving runtime authority or separately pin and package those upstreams; this inventory does neither.

## Metadata and naming authority

There are three distinct naming layers:

- Filesystem discovery names: the nine directory names above.
- Frontmatter capability names: five are prefixed with `vercel-`; four equal their directory name. Frontmatter versions range from 1.0.0 to 3.0.0.
- [`skills.sh.json`](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills.sh.json) catalog IDs/groupings: eight entries, including `vercel-react-native-skills`, and no `writing-guidelines`.

The root README also uses display names `react-native-guidelines` and `vercel-deploy-claimable`, which do not equal their current directory or frontmatter names ([catalog prose](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/README.md#L82-L142)). Pack synchronization should record all upstream names as metadata but use the directory plus frontmatter pair for discovery. It should not infer a repository release version: this commit has no tag, and per-skill versions are independent.

## Runtime authority and trust boundaries

| Capability | Prompt/direct authority |
|---|---|
| Four React/UI guidance packages | Read and rewrite consumer source; recommend packages/APIs; generate code. Rule/reference files are prompt dependencies, while aggregate `AGENTS.md` can become broad repository policy if projected carelessly. |
| `deploy-to-vercel` | Inspect Git remotes and `.vercel`; run Vercel CLI; link projects; commit/push; deploy preview or production; execute included scripts; package and upload project content. |
| `vercel-cli-with-tokens` | Read token/env state, export credentials, link projects, deploy, inspect logs, mutate Vercel environment variables/domains, and use Git. Its own examples include token-bearing command lines despite a warning to prefer environment use. |
| `vercel-optimize` | Authenticate to and query Vercel metrics/usage/contract/API; read project source; create run artifacts; fan out investigations; generate and verify optimization reports; optionally issue linking commands. Included Node code is the deterministic runtime authority behind the prompt. |
| `web-design-guidelines`, `writing-guidelines` | Fetch moving remote prompt/rules, read matched consumer files, and emit reviews. Remote content can change without this repository SHA changing. |

No upstream script or prompt should run during acquisition, synchronization, packaging, validation, or installation. Host activation must preserve normal user approval boundaries for authentication, external state, Git, deployment, config, package installation, and file mutation. In particular, copying `AGENTS.md` aggregates into a consumer repository root would broaden their scope beyond standard skill-on-demand discovery and is not an equivalent projection.

## Provenance and licensing authority

### Provenance

The repository is owned by the first-party GitHub organization `vercel-labs`, is not a GitHub fork, and the README describes several packages as maintained by Vercel or Vercel Engineering. Commit history at the pinned tree attributes different packages to multiple named contributors; it is provenance evidence, not a license grant. The generated rule packages cite React, Next.js, Expo, SWR, and other external documentation/projects in `metadata.json`; `vercel-optimize` embeds a curated first-party documentation library. These are references, not vendored dependency packages established by this inspection.

GitHub's licensing guidance says that absent a license the default copyright rules reserve reproduction, distribution, and derivative-work rights, while public-repository terms permit viewing and forking but do not by themselves establish general redistribution authority ([GitHub licensing guidance](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository#choosing-the-right-license)). GitHub also notes that a README can carry license information, but recommends a license file as the clear repository artifact ([license location guidance](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository#determining-the-location-of-your-license)).

### License evidence by package

| Package(s) | Evidence | Redistribution conclusion |
|---|---|---|
| `composition-patterns`, `react-best-practices`, `react-native-skills`, `react-view-transitions` | `SKILL.md` frontmatter says `license: MIT` ([example](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/react-best-practices/SKILL.md#L1-L8)). | Issue 254 accepts the pinned root README MIT declaration for the complete selected roots. No standalone text or copyright notice was supplied; retain any supplied notice and fabricate none. |
| `deploy-to-vercel`, `vercel-cli-with-tokens`, `vercel-optimize`, `web-design-guidelines`, `writing-guidelines` | No license frontmatter or local license artifact. Root README says only `## License` / `MIT` ([README](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/README.md#L176-L178)). GitHub detects no license. | Issue 254 accepts this declaration for the exact selected candidate and records redistribution, adaptation, and publication rights in digest-bound legal-admission evidence. |
| Root ZIPs and nested archive | No independent license/notice; they duplicate the packages above, sometimes stale. | They add no authority and should be excluded. |

This is a source-evidence conclusion, not legal advice. Packy’s maintainer decision and exact validity boundary are recorded in [`vercel-agent-skills-legal-admission.json`](evidence/vercel-agent-skills-legal-admission.json); it does not itself authorize Pack implementation or publication. Also, licenses of live content later fetched by the two guideline loaders must be assessed at the separately pinned source commits if Packy chooses to vendor them.

## Compatible-set conclusion for Codex, OpenCode, and Claude Code

The complete host-neutral set at this commit is the nine capability directories, with all non-ZIP descendants preserved. Each has a standard `SKILL.md`; auxiliary content remains package-relative. For all three targets:

1. project the directories as on-demand skills using each host's supported skill discovery mechanism;
2. preserve the exact upstream directory and frontmatter identity metadata;
3. exclude all six ZIP artifacts from installation;
4. do not promote package-local `AGENTS.md` aggregates to root/global instructions;
5. do not execute scripts or fetch moving dependencies during sync/validation;
6. gate all runtime external-state actions through the host/user approval model; and
7. attach commit `7c180d9044c9ae2b442b567aad4e42a28dd5ed62` as source identity, never `main`.

This establishes inventory only. It does not claim host behavioral equivalence, authorize installation, define a Packy manifest, or approve publication.

## Risks and unresolved questions

1. **Exact admission boundary:** issue 254 admits only the pinned README bytes and selected scope; any changed binding requires fresh admission.
2. **Missing standalone materials:** no standalone MIT text, copyright notice, or holder text was supplied; Packy must disclose this and must not fabricate them.
3. **Moving transitive prompts:** the web-design and writing skills fetch `main`; exact behavior is not pinned by this SHA.
4. **Stale archives:** three root ZIPs and the nested archive differ from source. Any consumer preferring ZIPs can silently install old behavior.
5. **Catalog drift:** README, directory, frontmatter, and `skills.sh.json` names/coverage disagree; `writing-guidelines` is absent from the catalog.
6. **Host projection unknowns:** upstream declares the Agent Skills format but does not specify exact installation layouts or permission behavior for each of Codex, OpenCode, and Claude Code. Those belong in later host-specific implementation/verification work.
7. **Runtime breadth:** deployment/token/optimization packages can access secrets, network services, Git, local project configuration, and production state. Static inclusion is not safe activation.

# Vercel execution-ready integration specification

## Purpose and authority

This specification is the implementation handoff for the `vercel` capability
Pack. It assembles the decisions made by the Wayfinder map
[Chart the Vercel capability-pack integration](https://github.com/yersonargotev/packy/issues/226)
without reopening or weakening them. When this document is shorter than a
linked decision asset or accepted ADR, that source remains authoritative.

The map plans the integration. This specification does not authorize Pack
implementation, source registration, synchronization, activation, publication,
or release. The exact `vercel` Pack remains inadmissible until the primary
`vercel-labs/agent-skills` source has exact, durable redistribution authority.
The two secondary MIT grants do not cure that blocker.

Acceptance requires all six sequential gates in
[Decide the Vercel validation and acceptance matrix](https://github.com/yersonargotev/packy/issues/233).
A failed, unknown, stale, or blocked row stops its gate and every later gate;
success on Codex, OpenCode, or Claude Code never compensates for another.

## Immutable decision inputs

| Decision | Contract carried into implementation |
| --- | --- |
| [Upstream inventory](vercel-upstream-inventory.md) | The primary basis is commit `7c180d9044c9ae2b442b567aad4e42a28dd5ed62`, tree `0557b732b3907e51bed3fd7898095f8097a0834e`: nine complete non-ZIP skill trees, six excluded duplicate archives, inert acquisition, and incomplete primary redistribution authority. |
| [Capability mapping](vercel-capability-surface-mapping.md) | All nine skills use native complete-tree projections on Codex, OpenCode, and Claude Code; no Vercel-specific projection kind or adapter policy is allowed. |
| [Observable contract](vercel-observable-contract.md) | All nine skills and all 27 native bindings are mandatory; every surface is atomic and independent; invocation requirements never become pack-wide activation prerequisites. |
| [Source, versioning, licensing, and compatibility](vercel-source-versioning-policy.md) | Pack `1.0.0` requires one exact, license-authorized three-source candidate; Pack SemVer follows observable compatibility; all admission remains manual and inert. |
| [Naming and composition](vercel-naming-composition-policy.md) | Namespaced portable identities preserve free upstream public names; collisions fail closed; aliases are explicit and surface-local; first-contract bindings are exclusive. |
| [Activation, authority disclosure, and readiness prototype](https://github.com/yersonargotev/packy/issues/232) | Per-surface lifecycle is inert and atomic, readiness is configured → authorized → usable, update invalidates prior loading evidence, and missing invocation prerequisites fail before effects. |
| [Runtime requirement vocabulary](https://github.com/yersonargotev/packy/issues/238) | Manifest v4 places typed `runtime_modes` on executable resources, uses Packy-owned tri-state observers, separates requirements, authorities, effects, and verified fallbacks, and forbids secret-bearing evidence. |
| [Guideline rule-source evidence](vercel-guideline-rule-source-evidence.md) | The two exact `command.md` rule blobs and distinct MIT notices are independent source-owned assets; the loaders must read sealed package-local assets rather than moving `main`. |
| [Atomic composite registration](../adr/0016-register-composite-pack-sources-atomically.md) | Initial three-source admission is one ephemeral Composite Pack Source Bundle and private `register_bundle` workflow operation in Pack Source suite v3.0.0. |
| [Validation and acceptance matrix](https://github.com/yersonargotev/packy/issues/233) | Six non-compensating gates cover legal admission, contract closure, lifecycle safety, three-surface conformance, independent readiness, and reproducible publication. |

Accepted ADRs retain module authority: ADR 0005 owns the complete surface-adapter
seam; ADRs 0007–0009 retain complete-tree transaction, classification, and
manual-workflow ownership; ADR 0011 requires immutable complete schema suites;
ADR 0012 retains source-scoped provenance and global freshness; and ADR 0016
adds atomic composite admission without moving those responsibilities.

## Exact first contract

### Exact source set and legal disposition

The candidate is the canonical ascending source-ID set below. Every selector is
a full exact commit. A branch, tag-like label, abbreviated SHA, archive name,
frontmatter version, or catalog value is not synchronization authority.

| Source ID | Repository and exact commit | Exclusive contribution | Legal disposition |
| --- | --- | --- | --- |
| `vercel-agent-skills` | `vercel-labs/agent-skills@7c180d9044c9ae2b442b567aad4e42a28dd5ed62` | Nine complete skill trees after the six ZIP exclusions and two explicit loader adaptations | **Blocked.** Requires exact-candidate license/notices or archived first-party written authorization covering redistribution, adaptation, and publication of every selected byte. |
| `vercel-web-interface-guidelines` | `vercel-labs/web-interface-guidelines@4e799d45c17aec1498c269287a83b9dba22b966b` | `command.md` as `asset:web-interface-guidelines-rules`; `LICENSE` as `notice:web-interface-guidelines-mit` | MIT, with the source-specific Vercel Labs notice retained. |
| `vercel-writing-guidelines` | `vercel-labs/writing-guidelines@83e2316b034cf572400513538e4e4da01c4cc742` | `command.md` as `asset:writing-guidelines-rules`; `LICENSE` as `notice:writing-guidelines-mit` | MIT, with the source-specific Vercel Labs notice retained. |

The web rule blob is Git blob `c6d1a9064f8a8615e8a9a8c50590f80a34545d1d`,
6,939 bytes, SHA-256
`eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab`.
The writing rule blob is Git blob
`8452139a442bef9c25abdd19ed9d4b0ef93aab02`, 14,228 bytes, SHA-256
`fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f`.
The complete notice identities and candidate provenance remain in the guideline
evidence asset and are exact acceptance inputs.

A later primary licensing commit is a new candidate; it does not retroactively
authorize `7c180d9`. If written permission is used, its archived bytes, digest,
issuer/rights-holder identity, covered repositories and material, granted
rights, obligations, and validity must be sealed as legal-admission evidence.

### Portable inventory and source ownership

The manifest is schema version 4, Pack ID `vercel`, Pack version `1.0.0`, and
surfaces `claude`, `codex`, `opencode`. It contains exactly 13 logical resources:
nine `skill`, two non-projecting `asset`, and two non-projecting `notice`
resources. Exactly 27 bindings exist: one native exclusive binding for each
skill on each surface. Assets and notices have no bindings.

| Portable resource | Manifest source | Primary upstream path | Public binding |
| --- | --- | --- | --- |
| `skill:vercel-composition-patterns` | `skills/vercel-composition-patterns` | `skills/composition-patterns` | `vercel-composition-patterns` |
| `skill:vercel-deploy-to-vercel` | `skills/vercel-deploy-to-vercel` | `skills/deploy-to-vercel` minus `Archive.zip` | `deploy-to-vercel` |
| `skill:vercel-react-best-practices` | `skills/vercel-react-best-practices` | `skills/react-best-practices` | `vercel-react-best-practices` |
| `skill:vercel-react-native-skills` | `skills/vercel-react-native-skills` | `skills/react-native-skills` | `vercel-react-native-skills` |
| `skill:vercel-react-view-transitions` | `skills/vercel-react-view-transitions` | `skills/react-view-transitions` | `vercel-react-view-transitions` |
| `skill:vercel-cli-with-tokens` | `skills/vercel-cli-with-tokens` | `skills/vercel-cli-with-tokens` | `vercel-cli-with-tokens` |
| `skill:vercel-optimize` | `skills/vercel-optimize` | `skills/vercel-optimize` | `vercel-optimize` |
| `skill:vercel-web-design-guidelines` | `skills/vercel-web-design-guidelines` | `skills/web-design-guidelines` plus the required web rule asset | `web-design-guidelines` |
| `skill:vercel-writing-guidelines` | `skills/vercel-writing-guidelines` | `skills/writing-guidelines` plus the required writing rule asset | `writing-guidelines` |
| `asset:web-interface-guidelines-rules` | `references/vercel-web-interface-guidelines-command.md` | secondary `command.md` | none |
| `asset:writing-guidelines-rules` | `references/vercel-writing-guidelines-command.md` | secondary `command.md` | none |
| `notice:web-interface-guidelines-mit` | `notices/vercel-web-interface-guidelines-MIT.txt` | secondary `LICENSE` | none |
| `notice:writing-guidelines-mit` | `notices/vercel-writing-guidelines-MIT.txt` | secondary `LICENSE` | none |

`skill:vercel-web-design-guidelines` requires
`asset:web-interface-guidelines-rules`; `skill:vercel-writing-guidelines`
requires `asset:writing-guidelines-rules`. Their projected `SKILL.md` files are
explicit Packy adaptations: replace the moving `main/command.md` fetch with a
deterministic package-relative read of the exact sealed asset. No other prompt
behavior changes. Acceptance fingerprints the adapted loader, exact rule bytes,
relative resolution, and absence of runtime fetch.

Every other skill owns its complete selected tree, including required rules,
references, scripts, libraries, schemas, playbooks, metadata, generated
package-local `AGENTS.md`, and safe Git file modes. A package-local `AGENTS.md`
remains inert inside the skill tree and is never promoted to repository or
global instructions. The five root ZIPs and
`skills/deploy-to-vercel/Archive.zip` are excluded. Source-maintainer tooling,
tests, evals, CI, catalog configuration, and other repository material outside
the selected trees are excluded.

Portable identity, source ownership, public binding, alias, observed occupancy,
projection ownership, sharing, and behavior contract are separate facts.
Source ownership never becomes shared merely because projection bytes match.

### Surface bindings

Every skill has exactly these binding rules:

| Surface | Projection | Invocation | Mode | Sharing |
| --- | --- | --- | --- | --- |
| Claude Code | `skill` | `/<public-name>` | `native` | `exclusive` |
| Codex | `skill` | `$<public-name>` | `native` | `exclusive` |
| OpenCode | `skill` | native skill tool with `<public-name>` | `native` | `exclusive` |

There are no optional skills, surface exclusions, or binding degradations.
Filesystem projection is not proof of host discovery, loading, authorization,
or invocation-mode availability.

### Manifest v4 wire contract

Manifest v4 preserves v3 top-level fields and existing v1–v3 meanings. Its
required top-level fields are `schema_version`, `id`, `version`, `surfaces`,
`provides`, `requires`, `conflicts`, `resources`, and `contract`.
`schema_version` is exactly `4`. The Vercel manifest uses
`provides: ["workflow:vercel"]`, empty pack-wide `requires.tools` and
`requires.capabilities`, and no conflicts. Pack-wide requirements remain legal
only for genuine activation/operation prerequisites; none of Vercel's
invocation-only requirements may appear there.

V4 adds required `runtime_modes` to every executable resource kind (`skill`,
and where applicable future `agent` or `command`). V4 forbids
`contract.optional_modes`; producers cannot dual-write the same fact or fall
back to v3. Readers retain strict v1–v3 support for existing and historical
Packs, but never heuristically translate v3 pack-wide optional modes into v4.

A v4 mode has this exact shape:

```json
{
  "id": "resource-local-kebab-case-id",
  "role": "primary | fallback_only",
  "requirements": [
    {
      "kind": "tool | authentication | project_link | entitlement | service_data",
      "id": "portable-kebab-case-id",
      "version": ">=20.0.0"
    }
  ],
  "authorities": [
    {
      "kind": "filesystem_read | filesystem_write | process_execute | network | environment_inspect | secret_use | package_manager_execute | git_inspect | git_commit | git_push | vercel_project_mutate | vercel_environment_mutate | vercel_domain_mutate | preview_deploy | production_deploy | upload | subagent_delegate",
      "scope": "consumer_project | pack_resource | workstation | local_git | remote_git | vercel_account | vercel_project | deployment_payload"
    }
  ],
  "effects": [
    {
      "kind": "authentication_state_change | consumer_project_file_change | consumer_project_dependency_change | local_git_change | remote_git_change | tool_installation | vercel_project_change | vercel_environment_change | vercel_domain_change | upload | preview_deployment | production_deployment",
      "scope": "consumer_project | workstation | local_git | remote_git | vercel_account | vercel_project | deployment_payload"
    }
  ],
  "fallback": {"kind": "none"},
  "on_unavailable": "fail_before_effects"
}
```

`fallback: {"kind":"mode","mode":"same-resource-mode-id"}` is the exact
alternative shape when a verified fallback exists. `version` is present only
for `kind: tool` and is a normalized SemVer predicate;
it is omitted when no exact version floor is part of the contract. `scope` is
required for the scoped authority/effect kinds and forbidden when meaningless.
Arrays are non-null, sorted, duplicate-free closed sets. References are
resource-local, acyclic, and point only from a primary mode to a
`fallback_only` mode. `fallback.kind: none` forbids `mode`; `kind: mode`
requires it. No manifest field carries probe commands or observed availability.
Human descriptions are annotations and carry no policy semantics.

Packy-owned observers produce, for each declared requirement and authority,
`available`, `unavailable`, or `unverified`, a closed reason code, observation
time, and observer revision. A mode is unavailable when any indispensable fact
is confirmed absent, unverified when none is absent and at least one is not
verified, and available only when all are freshly verified. Fallback evaluation
is independent and never rewrites the requested mode's truth.

Sensitive requirement and authority types structurally forbid token values,
environment values, raw stdout/stderr, credential-bearing commands, and
recoverable secret fingerprints in manifests, observations, status, previews,
plans, logs, errors, workflow artifacts, or acceptance evidence. Only presence,
absence, or redacted identity is admissible.

### Exact runtime-mode rows

The first manifest contains exactly the rows below. IDs are stable and local to
their resource. `R`, `A`, and `E` refer to the closed profiles defined after the
table. A dash means the required empty array. `none` means
`{"kind":"none"}`; `mode:<id>` means
`{"kind":"mode","mode":"<id>"}`. Every row uses
`on_unavailable: "fail_before_effects"`.

| Resource | Mode ID | Role | R | A | E | Fallback |
| --- | --- | --- | --- | --- | --- | --- |
| `vercel-composition-patterns` | `guidance-edit` | primary | — | `guidance-edit` | `guidance-edit` | none |
| `vercel-react-best-practices` | `guidance-edit` | primary | — | `guidance-edit` | `guidance-edit` | none |
| `vercel-react-native-skills` | `guidance-edit` | primary | — | `guidance-edit` | `guidance-edit` | none |
| `vercel-react-view-transitions` | `guidance-edit` | primary | — | `guidance-edit` | `guidance-edit` | none |
| `vercel-web-design-guidelines` | `local-review` | primary | — | `local-review` | — | none |
| `vercel-writing-guidelines` | `local-review` | primary | — | `local-review` | — | none |
| `vercel-deploy-to-vercel` | `git-push-preview` | primary | `git-push` | `git-push-preview` | `git-push-preview` | mode:`claimable-preview` |
| `vercel-deploy-to-vercel` | `git-push-production` | primary | `git-push` | `git-push-production` | `git-push-production` | none |
| `vercel-deploy-to-vercel` | `cli-preview` | primary | `vercel-cli-linked` | `cli-preview` | `cli-preview` | mode:`claimable-preview` |
| `vercel-deploy-to-vercel` | `cli-production` | primary | `vercel-cli-linked` | `cli-production` | `cli-production` | none |
| `vercel-deploy-to-vercel` | `link-cli-preview` | primary | `vercel-cli-authenticated` | `link-cli-preview` | `link-cli-preview` | mode:`claimable-preview` |
| `vercel-deploy-to-vercel` | `link-git-preview` | primary | `vercel-cli-git-authenticated` | `link-git-preview` | `link-git-preview` | mode:`claimable-preview` |
| `vercel-deploy-to-vercel` | `setup-link-preview` | primary | `setup-link` | `setup-link-preview` | `setup-link-preview` | mode:`claimable-preview` |
| `vercel-deploy-to-vercel` | `claimable-preview` | fallback_only | `claimable` | `claimable-preview` | `claimable-preview` | none |
| `vercel-cli-with-tokens` | `inspect` | primary | `token-cli` | `token-inspect` | — | none |
| `vercel-cli-with-tokens` | `deploy-preview` | primary | `token-cli-linked` | `token-cli-preview` | `cli-preview` | none |
| `vercel-cli-with-tokens` | `deploy-production` | primary | `token-cli-linked` | `token-cli-production` | `cli-production` | none |
| `vercel-cli-with-tokens` | `link-project` | primary | `token-cli` | `token-link` | `token-link` | none |
| `vercel-cli-with-tokens` | `environment-read` | primary | `token-cli-linked` | `token-inspect` | — | none |
| `vercel-cli-with-tokens` | `environment-write` | primary | `token-cli-linked` | `token-environment-write` | `token-environment-write` | none |
| `vercel-cli-with-tokens` | `domain-read` | primary | `token-cli-linked` | `token-inspect` | — | none |
| `vercel-cli-with-tokens` | `domain-write` | primary | `token-cli-linked` | `token-domain-write` | `token-domain-write` | none |
| `vercel-cli-with-tokens` | `git-push-preview` | primary | `token-git-push` | `token-git-push-preview` | `git-push-preview` | none |
| `vercel-cli-with-tokens` | `git-push-production` | primary | `token-git-push` | `token-git-push-production` | `git-push-production` | none |
| `vercel-optimize` | `subagent-investigation` | primary | `optimize` | `optimize-subagent` | `optimize` | mode:`sequential-investigation` |
| `vercel-optimize` | `sequential-investigation` | fallback_only | `optimize` | `optimize-sequential` | `optimize` | none |
| `vercel-optimize` | `subagent-observability-plus` | primary | `optimize-observability-plus` | `optimize-subagent` | `optimize` | mode:`sequential-observability-plus` |
| `vercel-optimize` | `sequential-observability-plus` | fallback_only | `optimize-observability-plus` | `optimize-sequential` | `optimize` | none |

Requirement profiles expand exactly as follows:

| R profile | Required entries |
| --- | --- |
| `git-push` | `tool:git`; `authentication:git-provider` |
| `vercel-cli-authenticated` | `tool:vercel-cli`; `authentication:vercel` |
| `vercel-cli-linked` | `tool:vercel-cli`; `authentication:vercel`; `project_link:vercel-project` |
| `vercel-cli-git-authenticated` | `tool:git`; `tool:vercel-cli`; `authentication:git-provider`; `authentication:vercel` |
| `setup-link` | `tool:npm`; `authentication:vercel-interactive` |
| `claimable` | `tool:bash` |
| `token-cli` | `tool:vercel-cli`; `authentication:vercel-token` |
| `token-cli-linked` | `tool:vercel-cli`; `authentication:vercel-token`; `project_link:vercel-project` |
| `token-git-push` | `tool:git`; `tool:vercel-cli`; `authentication:git-provider`; `authentication:vercel-token`; `project_link:vercel-project` |
| `optimize` | `tool:node@>=20.0.0`; `tool:vercel-cli@>=53.0.0`; `authentication:vercel`; `project_link:vercel-project`; `service_data:vercel-project-metrics` |
| `optimize-observability-plus` | all `optimize` entries plus `entitlement:observability-plus` |

Requirement notation maps text before `:` to `kind`, text after it to `id`,
and `@` to the tool `version`. These are portable Packy-owned observer
identities; manifests contain no probe command.

Authority profiles expand to these exact sorted `(kind, scope)` sets:

| A profile | Authority entries |
| --- | --- |
| `guidance-edit` | `filesystem_read:consumer_project`; `filesystem_write:consumer_project` |
| `local-review` | `filesystem_read:consumer_project`; `filesystem_read:pack_resource` |
| `git-push-preview` | `filesystem_read:consumer_project`; `git_commit:local_git`; `git_inspect:local_git`; `git_push:remote_git`; `network:remote_git`; `preview_deploy:vercel_project`; `process_execute:local_git` |
| `git-push-production` | same as `git-push-preview`, replacing `preview_deploy` with `production_deploy:vercel_project` |
| `cli-preview` | `filesystem_read:consumer_project`; `network:vercel_project`; `preview_deploy:vercel_project`; `process_execute:consumer_project`; `secret_use:vercel_account`; `upload:deployment_payload` |
| `cli-production` | same as `cli-preview`, replacing `preview_deploy` with `production_deploy:vercel_project` |
| `link-cli-preview` | all `cli-preview` entries plus `filesystem_write:consumer_project`; `vercel_project_mutate:vercel_project` |
| `link-git-preview` | all `git-push-preview` entries plus `filesystem_write:consumer_project`; `network:vercel_project`; `secret_use:vercel_account`; `vercel_project_mutate:vercel_project` |
| `setup-link-preview` | all `link-cli-preview` entries plus `package_manager_execute:workstation` |
| `claimable-preview` | `filesystem_read:consumer_project`; `network:vercel_project`; `preview_deploy:vercel_project`; `process_execute:consumer_project`; `upload:deployment_payload` |
| `token-inspect` | `environment_inspect:consumer_project`; `filesystem_read:consumer_project`; `network:vercel_account`; `process_execute:consumer_project`; `secret_use:vercel_account` |
| `token-cli-preview` | `filesystem_read:consumer_project`; `network:vercel_project`; `preview_deploy:vercel_project`; `process_execute:consumer_project`; `secret_use:vercel_account`; `upload:deployment_payload` |
| `token-cli-production` | same as `token-cli-preview`, replacing `preview_deploy` with `production_deploy:vercel_project` |
| `token-link` | `environment_inspect:consumer_project`; `filesystem_read:consumer_project`; `filesystem_write:consumer_project`; `network:vercel_project`; `process_execute:consumer_project`; `secret_use:vercel_account`; `vercel_project_mutate:vercel_project` |
| `token-environment-write` | all `token-inspect` entries plus `filesystem_write:consumer_project`; `vercel_environment_mutate:vercel_project` |
| `token-domain-write` | all `token-inspect` entries plus `vercel_domain_mutate:vercel_project` |
| `token-git-push-preview` | all `git-push-preview` entries plus `environment_inspect:consumer_project`; `network:vercel_account`; `secret_use:vercel_account` |
| `token-git-push-production` | same as `token-git-push-preview`, replacing `preview_deploy` with `production_deploy:vercel_project` |
| `optimize-sequential` | `environment_inspect:consumer_project`; `filesystem_read:consumer_project`; `filesystem_read:pack_resource`; `filesystem_write:consumer_project`; `network:vercel_project`; `process_execute:consumer_project`; `secret_use:vercel_account` |
| `optimize-subagent` | all `optimize-sequential` entries plus `subagent_delegate:consumer_project` |

Effect profiles expand to these exact sorted `(kind, scope)` sets:

| E profile | Effect entries |
| --- | --- |
| `guidance-edit` | `consumer_project_dependency_change:consumer_project`; `consumer_project_file_change:consumer_project` |
| `git-push-preview` | `local_git_change:local_git`; `preview_deployment:vercel_project`; `remote_git_change:remote_git` |
| `git-push-production` | `local_git_change:local_git`; `production_deployment:vercel_project`; `remote_git_change:remote_git` |
| `cli-preview` | `preview_deployment:vercel_project`; `upload:deployment_payload` |
| `cli-production` | `production_deployment:vercel_project`; `upload:deployment_payload` |
| `link-cli-preview` | `authentication_state_change:vercel_account`; `consumer_project_file_change:consumer_project`; `preview_deployment:vercel_project`; `upload:deployment_payload`; `vercel_project_change:vercel_project` |
| `link-git-preview` | `consumer_project_file_change:consumer_project`; `local_git_change:local_git`; `preview_deployment:vercel_project`; `remote_git_change:remote_git`; `vercel_project_change:vercel_project` |
| `setup-link-preview` | all `link-cli-preview` entries plus `tool_installation:workstation` |
| `claimable-preview` | `preview_deployment:vercel_project`; `upload:deployment_payload` |
| `token-link` | `consumer_project_file_change:consumer_project`; `vercel_project_change:vercel_project` |
| `token-environment-write` | `consumer_project_file_change:consumer_project`; `vercel_environment_change:vercel_project` |
| `token-domain-write` | `vercel_domain_change:vercel_project` |
| `optimize` | `consumer_project_file_change:consumer_project` |

The closed effect enumeration therefore also includes
`authentication_state_change` and `tool_installation`, with the exact scopes
shown above. They are source-observable consequences and cannot be collapsed
truthfully into an authority.

The adapted guideline assets are package dependencies, not runtime requirement
rows. Production modes have no preview fallback because production is an
explicitly different authority and effect. Token modes have no invented
no-token fallback. Claimable-preview and sequential-optimization fallback edges
remain blocked until exact-contract acceptance evidence proves the same logical
outcome.

### Compatibility

Pack `1.0.0` is an initial registration, not a migration. Pack SemVer is
independent of upstream, schema-suite, and Packy application versions.
Provenance-only movement with identical selected bytes and legal obligations is
a Pack no-op.

Patch preserves resources, names, invocations, projections, requirements,
authorities, effects, availability semantics, fallbacks, exclusions, legal
obligations, and mandatory actions. Minor may add compatible independent modes,
relax requirements, add a verified fallback, or reduce authority/effects while
preserving the logical result, with no migration or mandatory action. Major
includes removal/rename, strengthened requirements or versions, broadened
authority/effects, removed fallback, weakened redaction or fail-before-effects
safety, incompatible legal change, migration, or mandatory action. The effective
classification is the maximum across every resource, mode, and surface.

Every later major includes an exact migration, mandatory human actions, effects
and risks on all three surfaces, alias/collision/external-state handling,
verification, and recovery. If any surface lacks a coherent independently
verifiable route, publication blocks.

## Source, provenance, and composite admission

### Source configuration and selected bindings

Each source configuration is canonical and has a full commit selector. The
primary source exclusively binds the nine `skill` resources to their selected
upstream directories. The two secondary source configurations bind the exact
asset/notice pairs recorded in the guideline evidence. Manifest `source`
selects the vendored destination; source configuration supplies `upstream_path`.

After admission, canonical state is:

```text
bundle/sources/vercel-agent-skills.lock.json
bundle/sources/vercel-web-interface-guidelines.lock.json
bundle/sources/vercel-writing-guidelines.lock.json
bundle/packs/vercel/pack.json
bundle/history/vercel/1.0.0/...
```

plus the selected skill, reference, and notice bytes. There is no composite
index or durable composite entity.

Every source lock owns one exact candidate and that source's complete exclusive
contribution. The canonical ordered lock set derives `lock_set_sha256`; no
aggregate lock index is persisted. Every operation seals each source-lock
digest and the complete lock-set digest. A changed source or bundle generation
makes every older proposal stale.

### Pack Source schema suite v3.0.0

Composite admission is the private `register_bundle` operation in a new,
complete, immutable five-schema Pack Source suite at
`schemas/pack-source/v3.0.0/`. V1/v2 bytes and meanings remain immutable.
A v3 dispatch cannot consume or emit v1/v2 artifacts.

The registration request declares one absent `pack_id`, at least two complete
source registrations ordered by `source_id`, and
`registration_bundle_sha256`, the digest of their canonical representation.
Every member has its own exact commit selector, complete bindings, and sealed
legal-admission evidence reference/digest. The canonical plan additionally
seals repository base, all candidates, resulting configuration, source locks,
complete lock set, Pack/version result, manifests, classification, tree, and
workflow preconditions.

Every v3 inspection, failure, classification, validation, and publication
artifact carries the declared Pack ID, ordered source IDs, registration digest,
ordered candidate/source-lock/legal-evidence members, plan/base/resulting
configuration/manifests/lock-set/tree identities appropriate to its phase, and
the existing no-secret/no-upstream-byte attestations. Any mismatch is stale.

`internal/packsync` owns one complete-set Check and Apply seam. Fewer than two
members, duplicate/unsafe source IDs, an existing target Pack or member source,
bindings outside the Pack, ownership conflicts, incomplete legal evidence,
mixed schema versions, or any invalid complete result fails before writes.
There is no initial no-op, partial convergence, reusable source subplan, or
member-wise Apply.

All resources are materialized into one sibling-staged complete bundle before
Pack dependency closure and validation. Apply/Recover reuse ADR 0007's one
lock, durable marker, two renames, complete old/new tree hashes, validation, and
recovery authority. Recovery never completes or rolls back one member.

### Manual workflow

The private manual **Inspect → Classify → Validate → Publish** workflow and
`internal/tools/syncpacksource` adapter remain the only admission surface. No
public `packy` command or bootstrap writer is added.

1. Inspect statically acquires all exact candidates in disposable roots, proves
   identity, inventory, legal admission, complete closure, selected bytes,
   manifests, locks, Pack compatibility floor, base, and all seals. It executes
   no acquired content.
2. Classify emits one exact-plan evidence document for affected Pack `vercel`;
   source-level classifications have no authority.
3. Validate independently reacquires all candidates, reproduces the complete
   result in disposable roots, and runs every acceptance row without executing
   upstream runtime authority.
4. Publish reacquires all candidates again, reproduces the exact result and
   permitted diff, reruns validation, and freshly observes base, provenance,
   branch/PR ownership, and managed metadata before its first write.

Concurrency, branch, and PR ownership are Pack-scoped. Initial success creates
at most one non-draft, auto-merge-disabled `sync/vercel` PR. Human merge remains
required. Changed candidate, legal fact, byte, base, binding, lock,
classification, evidence, or PR state requires a fresh Inspect; evidence is
never patched forward.

## Capability-pack lifecycle and host adapters

`internal/capabilitypack` owns manifest v4, dependency closure, mode semantics,
Pack composition, collision/alias/sharing policy, ownership, blockers, consent,
readiness, preflight results, plans, stale rejection, verification, recovery,
compatibility, migrations, and mandatory actions.

`internal/codex`, `internal/opencode`, and `internal/claudecode` each implement
the complete ADR 0005 `SurfaceAdapter`: pure fresh inspection and the sole
projection-application mutation seam. They own paths, syntax, normalization,
projection translation, host discovery/loading observation, and application of
exactly sealed actions. They do not choose Vercel policy, aliases, sharing,
fallback equivalence, lifecycle disposition, or readiness meaning.

Every adapter observation covers reserved/native, unmanaged, and Packy-owned
occupied targets; desired/observed fingerprints and contributor ownership;
authorization/loading/usability evidence; pending human actions; per-mode safe
observations; and one revision sealing every relied-on fact. Duplicate or
inconsistent occupancy/evidence invalidates the whole observation.

An unresolved mandatory collision blocks the complete affected surface before
effects. Packy never adopts matching unmanaged content, overwrites, chooses
precedence, or silently renames. The user may explicitly select a surface-local
alias for one portable identity; `vercel-pack-<public-name>` is only a
suggestion. The normalized alias is collision-checked and sealed through
preview, approval, Apply, verification, recovery, status, and update. It
survives compatible updates by logical identity and changes neither provenance
nor Pack version.

All first-contract bindings are exclusive. Future sharing requires a versioned
mutual declaration, identical normalized projection, tree and metadata,
identical behavior including dependencies/runtime modes/readiness/failures, and
recorded contributor ownership. Equal bytes or unmanaged occupancy never
suffice.

Activation, update, reconciliation, status, and deactivation inspect or apply
only inert projection trees. They never execute upstream scripts, authenticate,
read secret values, link projects, access runtime service data, mutate consumer
projects, perform skill-directed Git operations, upload, or deploy. Preview is
pure. Apply uses exact typed consent and fails stale before effects. Update
replaces all nine projections atomically on one surface, preserves aliases,
returns readiness to configured, and requires fresh authorization/loading
evidence. Deactivation removes only freshly verified Packy-owned projections;
drifted, ambiguous, unmanaged, and retargeted-symlink targets remain pending
human actions.

A projected invocation explicitly selects a runtime mode and performs fresh
pre-effect preflight. Preview/status disclosure never authorizes that future
invocation. A missing or unverified indispensable input lists every affected
identity and fails before authority or effect. Packy is not a permanent runtime
proxy: the host/user retains execution authorization, but acceptance must prove
the projection preserves mode selection, redaction, verified fallback, and the
pre-effect boundary.

## CLI and readiness contract

Human and JSON output are renderings of one structured domain result. Preview,
status, update, and acceptance evidence disclose:

- the exact 13-resource inventory, nine skill trees, 27 bindings, exclusions,
  source identities, and dependency closure;
- each public/effective binding, invocation, alias, collision, owner,
  contributor set, contract diff, migration, and mandatory action;
- each runtime mode's typed requirements/versions, authentication/linkage/
  entitlement/service-data facts, authorities, effects, fallback, availability,
  reason, and observation freshness;
- pending human actions and exact consent scope; and
- configured, authorized, and usable as independent `yes`, `no`, or `unknown`.

Configured means all nine Packy-owned projections match. Authorized requires
fresh host trust/loading permission evidence. Usable requires current
host-native evidence that all nine skills were discovered and loaded. A pack
can be usable while an invocation mode is unavailable or unverified. Missing
invocation prerequisites do not reduce whole-pack readiness. If one mandatory
skill is not discovered or loaded, the surface cannot be reported usable.

Dry-run mutates nothing and asks for no approval. `--require usable` fails for
both `no` and `unknown`. Secrets remain structurally absent from every output.

## Reproducible acceptance

Every blocking row has a stable ID, gate and surface; exact candidate/contract
binding; canonical fixture; disposable repository, acquisition, state, HOME,
XDG, and host roots; operation/preconditions; structured result and stable
diagnostic; exact allowed-diff or zero-mutation oracle; one-fact negative twin;
deterministic rerun; evidence fingerprint/freshness; and one disposition:
Proven existing, Implementation target, or Blocked by prerequisite.

The six gates are:

1. Admission: exact three-source identity, locks, legal evidence, and complete
   lock-set reproduction. Primary legal authority is currently blocked.
2. Contract closure: exactly nine skill trees, 13 resources, 27 native bindings,
   28 runtime-mode rows, complete auxiliary closure, two adapted local
   guideline loaders, notices, six ZIP exclusions, and no moving or undeclared
   input.
3. Lifecycle safety: per-surface activation, pure preview, stale rejection,
   collisions/aliases, atomic update, no-op, write-boundary failure/recovery,
   deactivation, foreign/drift/symlink preservation, and cross-surface isolation.
4. Conformance/disclosure/preflight: complete typed mode disclosure, redaction,
   tri-state observations, fallback evidence, fresh mode selection, and
   fail-before-effects negative twins on every surface.
5. Compatibility/readiness: independent disposable projection conformance and a
   real host-native smoke test for exact supported Codex, OpenCode, and Claude
   Code versions; no cross-host/version inference and no real deployment needed.
6. Reproducible publication: independent Validate/Publish reacquisition and
   reproduction, least privilege, Pack-scoped non-cancelling concurrency, and
   exactly one human-mergeable `sync/vercel` proposal.

Narrative, screenshots, filesystem presence alone, or a one-off successful
invocation do not make a row green. Publication-only smoke evidence does not
become lifecycle status evidence until a safe stable observer exists.

## Tracer-bullet delivery graph

Each implementation ticket keeps `go test ./...` green and runs
`./scripts/validate-packy.sh`. Generic infrastructure may be implemented with
synthetic fixtures before legal admission, but no selected primary Vercel byte
may enter the bundle or a Vercel-specific selectable contract before slice L.

| Slice | Smallest independently verifiable outcome | Blocked by |
| --- | --- | --- |
| L. Establish primary legal admission | Exact candidate license/notices or archived first-party written authorization yields a durable, digest-bound redistributable disposition for every selected `agent-skills` byte. | None; external prerequisite |
| A. Publish manifest v4 runtime contracts | Strict v4 manifest types/schema, runtime modes, closed vocabularies, canonicalization, secret-safe evidence shapes, v1–v3 compatibility, and offline producer/validator parity are executable with synthetic fixtures. | None |
| B. Implement Composite Pack Source Bundle admission | Pack Source suite v3.0.0 and `register_bundle` Check/Apply/workflow seams prove atomic complete-set admission, seals, legal evidence, old/new recovery, and unchanged v1/v2 behavior using synthetic sources. | None |
| C. Carry the exact Vercel contract through the portable model | The authorized three-source fixture produces the exact 13-resource manifest, 27 bindings, local guideline adaptations, complete source ownership, v4 mode rows, compatibility facts, and all closure negative twins without entering the selectable catalog. | A, B, L |
| D. Prove Codex projection and invocation preflight | Codex plans/applies/verifies all nine native `$name` skills, aliases/collisions, complete trees, v4 disclosure, fresh preflight, readiness, lifecycle failure/recovery, and inertness. | C |
| E. Prove OpenCode projection and invocation preflight | OpenCode proves the same contract through its native skill tool and independent host evidence. | C |
| F. Prove Claude Code projection and invocation preflight | Claude Code proves the same contract through `/name` and independent host evidence. | C |
| G. Deliver shared lifecycle and CLI behavior | Human/JSON preview, consent, aliases, status, update, reconciliation, deactivation, recovery, tri-state readiness/mode availability, redaction, and cross-surface isolation share one domain result. | D, E, F |
| H. Close the complete Vercel acceptance cohort | Packy-owned fixtures and exact host smokes turn every row across all six gates green, including compatibility floors and independent three-host readiness, without real credentials or deployment. | G |
| I. Admit Vercel through the manual workflow | `register_bundle` independently reacquires the exact three-source candidate through Inspect/Classify/Validate/Publish and creates one decision-ready non-draft `sync/vercel` PR with the complete Pack `1.0.0` generation. | H |
| J. Accept the exact bundle generation | The proposal is reviewed and merged only while all evidence is fresh and green; the merged SHA is revalidated with no real-home activation or Packy release side effect. | I |

Native blocking edges:

```text
L -> C
A -> C
B -> C
C -> D
C -> E
C -> F
D -> G
E -> G
F -> G
G -> H
H -> I
I -> J
```

D, E, and F are the only planned parallel host frontier. They own host
translation and evidence, not shared capability-pack policy.

## File and module impact guide

This guide identifies likely ownership; it does not authorize moving policy.

- `schemas/pack-source/v3.0.0/`, schema validators, Pages verification, and
  workflow fixtures: immutable composite workflow suite and offline parity.
- `internal/packsync`, `internal/packsync/githubsource`,
  `internal/packclassification`, `internal/packsyncworkflow`, and
  `internal/tools/syncpacksource`: complete candidate acquisition, composite
  seals, per-Pack classification, legal evidence, and manual admission.
- `internal/bundletransaction`: unchanged single complete-tree mutation and
  recovery authority; only the composite sealed inputs/diagnostics extend.
- `internal/capabilitypack/catalog.go`, composition, activation, status,
  lifecycle output, compatibility/history, and tests: manifest v4, mode
  evidence, dependency closure, aliases, sharing, lifecycle, and readiness.
- `internal/codex`, `internal/opencode`, `internal/claudecode`: complete host
  observation, translation, application, fresh readiness, and preflight
  preservation without Vercel-specific policy.
- `internal/cli/pack.go` and CLI tests: structured v4 disclosure, aliases,
  consent, tri-state results, redaction, failures, update, recovery, and gates.
- `internal/ci`, acceptance packages, host-smoke packages, and
  `scripts/validate-packy.sh`: six-gate orchestration and exact negative twins.
- `bundle/sources.json`, `bundle/sources/vercel-*.lock.json`,
  `bundle/packs/vercel/**`, `bundle/history/vercel/**`, selected skills,
  references, and notices: produced only by the accepted synchronization result,
  never by an installer, ad hoc copy, or infrastructure PR.

## Decision-closure audit

No product, architecture, naming, lifecycle, source, schema, validation,
migration, or delivery-order decision remains before slices A, B, or L can
start. The following are execution observations, not open decisions:

- exact legal authority for the primary selected bytes must be obtained and
  verified; failure leaves L blocked rather than weakening the contract;
- mode rows are mechanically inventoried from the fixed candidate under the
  closed v4 vocabulary; an unrepresentable source route is contradictory new
  evidence and requires a new decision ticket;
- exact supported host versions are recorded by the acceptance run and evidence
  fingerprint, not chosen by this Pack contract;
- runtime availability may truthfully be unavailable or unverified without
  reducing whole-pack readiness when all nine skills are loaded;
- no real credentials, consumer project mutation, deployment, operator-home
  activation, or Packy release is required to implement or accept the bundle;
  and
- merge of the `sync/vercel` proposal remains a human decision after every row
  is green.

If implementation discovers evidence that contradicts a fixed decision, it
must stop and open one new Wayfinder decision ticket. It must not reinterpret
this specification, narrow the nine-skill contract, weaken legal admission,
collapse a runtime mode, or waive an acceptance row locally.

# PROTOTYPE PR brief — blocked dry run

## Identity

- Source: `mattpocock-skills` (`mattpocock/skills`, repository ID `1148788086`, owner ID `28293365`)
- Release: `v1.1.0` → tag object `eabea89380927aadb93abf6e290a19334d249292` → commit `d574778f94cf620fcc8ce741584093bc650a61d3`
- Tree: `fa3f8882cef6fa6d9960283a49db0a58636af3ca`; parents: `cc1e24891df515a43a034cd91d3f64e17d1c9ffb, 47845ac1e15d048c2bbb20413a44de8681209601`
- Plan: `6b23527b3edbce7a191d2db15d90a69bdc198d3c83c386c5a2a232d56fca615e` on base `bc6ed4215b30a0b391eddf97fc8f3da045308c62`
- Review: https://github.com/mattpocock/skills/tree/d574778f94cf620fcc8ce741584093bc650a61d3

## Normalized request

```json
{
  "classification_mode": "ai",
  "request_reason": "Validate the synchronization design with a Matty dry run",
  "selector": "latest-stable",
  "source_id": "mattpocock-skills"
}
```

## Real diff

Selected: 23 resources / 45 files. Changes: 5 modified, 0 added, 0 removed, 0 moved.

- `bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-github.md`: `273c1d57c36426d80df362b20288d8575d3d9ba5aeb7334bd4c966cd73eae863` → `52e9f9f1ec0f6d47c6785ac500708ac0521f34abb4cc5475b5dc747165dab17e`
- `bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-gitlab.md`: `07fc81c40f529e5574f1d9cc20bb6196679efe910c66ffc3217d4ed65031c1e3` → `4470f2f64d015fba01af877233296b401bb0d44b5acb9572ffc3ff6b30e8de88`
- `bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-local.md`: `1ad2ce603d6bdb7b8db99417a8e1b4b8a32837369a87d525fec696d12322fcad` → `e0dd9835e3658909132058e9a5fdd851e972220bbadaf78a57e3c6220d470922`
- `bundle/skills/engineering/to-tickets/SKILL.md`: `87376aa0b0e0f2e5bcfe51d8686b27637bfe0a42ba6430943602dc3f14c20823` → `918bdefab9313100cb1f7ccb412e2a773fe2f2801dd20d44f6b2acf7a42ca456`
- `bundle/skills/engineering/wayfinder/SKILL.md`: `6ea93ad3760821ca8f21e8d5f2c7eeb2a812eb8c4d7ed6349494787fc5e7f522` → `bef437de697fb6984a8a90b7fd82f128609148d6e02f635ce419d03555b351e1`

Unselected upstream resources discovered: `skills/deprecated/design-an-interface`, `skills/deprecated/qa`, `skills/deprecated/request-refactor-plan`, `skills/deprecated/ubiquitous-language`, `skills/in-progress/claude-handoff`, `skills/in-progress/wizard`, `skills/in-progress/writing-beats`, `skills/in-progress/writing-fragments`, `skills/in-progress/writing-shape`, `skills/misc/git-guardrails-claude-code`, `skills/misc/migrate-to-shoehorn`, `skills/misc/scaffold-exercises`, `skills/misc/setup-pre-commit`, `skills/personal/edit-article`, `skills/personal/obsidian-vault`.

## Pack impact

AI proposes **major** `matty` `1.0.0` → `2.0.0` because reverting the five local adaptations changes established issue/spec vocabulary, local ticket paths, and local wayfinder behavior.

> evidencia propuesta por IA en el plan original; la clasificación `major` fue aceptada posteriormente por el mantenedor durante esta revisión HITL

Migration proposal: move the intentional adaptations to a Matty-owned seam or explicitly adopt upstream behavior, then document the resulting user-visible change.

## Validations

- **eligible** — repository and release provenance: unsigned tag plus GitHub-verified peeled commit
- **pass** — source allowlist and manifest-derived destinations: 23 bindings resolve to real matty pack resources
- **pass** — selected resource presence: 45 proposed files across 23 resources
- **blocked** — byte identity: 5 modified, 0 added, 0 removed
- **pass** — archive paths, symlinks, and permissions: 0 symlink/hardlink entries rejected; no absolute or parent paths admitted
- **blocked** — generated provenance lock: production lock absent; proposed hashes are emitted without writing bundle
- **blocked** — historical artifacts: matty 1.0.0 immutable artifact absent
- **blocked** — compatibility and migration: AI proposes major 2.0.0; maintainer acceptance/migration decision pending
- **pass** — allowed repository diff: prototype wrote only beneath its explicitly allowed scratch directory
- **blocked** — Matty-owned safe validation: required targeted production entrypoint does not yet exist
- **pass** — upstream execution: no upstream scripts, hooks, Actions, tests, generators, binaries, lifecycle scripts, submodules, or LFS were executed

## Blockers

- The real bundle has five locally modified selected files and therefore violates byte identity.
- Production bundle/sources.json and bundle/sources.lock.json do not exist; this dry run uses the accepted prototype fixture only.
- The legacy root skills-lock.json covers one resource, pins no immutable release identity, and does not describe the local wayfinder bytes.
- A major AI classification and migration are proposed evidence only; no maintainer has accepted them.
- No immutable historical artifact for matty 1.0.0 exists to preserve pinned-version behavior.
- The hardened Matty-owned validation entrypoint required by the workflow contract is not implemented.

## Result

**Blocked.** No branch, PR, dispatch, production lock, pack version, or vendored byte was written. Auto-merge remains disabled; manual merge would be required only after a future proposal becomes decision-ready.

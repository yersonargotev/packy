# Reviewed skill conflict resolution (2026-10-16)

The original surface-qualified naming and execution below are historical evidence
for engine commit `3c7010c8`. The current policy uses automatic `<pack-id>-<binding-name>`
names without user aliases. See the [short-name follow-up](skill-template-short-names-2026-10-16.md)
for the naming investigation and current validation.

## Pre-change conclusion

Packy cannot safely make the existing `--alias` mechanism resolve Matty/pstack conflicts by merely renaming directories or replacing text. The current alias is a host binding rename only. Global skill projections remain symlinks to immutable catalog trees, so their `SKILL.md` frontmatter and internal references remain unchanged. Packy's discovery observer correctly reads the frontmatter `name`, and the accepted architecture explicitly rejects automatic installation-time prompt rewriting.

A safe implementation is possible only as a new, reviewed contract: declare semantic intra-Pack skill references in the manifest (or reviewed alias-aware source templates), render a deterministic derived skill tree, record the renderer identity and resulting tree digest in lifecycle/receipt evidence, and fail closed for undeclared ambiguous text. This requires an ADR superseding the relevant part of ADR 0041 and a new closed surface-capability vocabulary member. Existing schema-v3 manifests remain unchanged; older engines reject snapshots that request the new capability. It is not a small alias bug fix.

## Documented pre-change facts

1. **A projected directory name is not the discovered skill identity.** Native targets use the projected binding name, but `docs/skill-discovery.md:16-25` says logical identity remains separate and that a different basename does not isolate divergent trees with the same frontmatter `name`. Unverified coexistence blocks before mutation (`docs/skill-discovery.md:27-44`).

2. **The observer parses frontmatter, not just the path.** `internal/skilldiscovery/adapter.go:182-226` reads `SKILL.md`, extracts YAML frontmatter with `go.yaml.in/yaml/v3`, and returns `name`; desired generated trees are also inspected from `TreeFiles` (`adapter.go:32-62`). This is a narrow read-only parser. There is no general Markdown/frontmatter mutation or reference AST.

3. **Aliases only mutate the effective binding name.** `resourceWithSurfaceAlias` clones the binding slice and assigns `binding.Name = alias.Name` (`internal/capabilitypack/composition.go:100-113`). `SurfaceAlias` contains only `{kind,id,name}` (`activation.go:341-345`) and validation only checks supported kinds, identity, canonical text, and uniqueness (`state_store.go:301-328`). It carries no reference mapping or transformation policy.

4. **Global Codex/OpenCode skills are exact source-tree links.** Codex fingerprints the catalog source and creates `ActionSkillLink{Source: source, Target: <skills>/<binding-name>}` (`internal/codex/surface.go:121-145`). OpenCode does the same (`internal/opencode/surface.go:168-186`); Claude has an analogous skill-link path. Therefore a binding alias changes the link path but not `SKILL.md` bytes or sibling paths inside the source tree.

5. **The accepted ADR currently forbids the requested adaptation.** ADR 0041 requires reviewed variants and states: “Automatic prompt rewriting would introduce unreviewed installation-time adaptation” (`docs/adr/0041-keep-logical-resource-identity-across-surface-variants.md:37-40`). It also requires unsupported or unverified same-name combinations to block before mutation (`:30-35`). Implementing general automatic rewriting without a replacement ADR would violate accepted architecture.

6. **The manifest has dependency identity but not lexical intra-skill references.** General resource dependencies express closure. The only field named `references` is inside the typed Claude composite-skill capability (`schemas/pack/v3/pack.schema.json:298-319`; `internal/capabilitypack/catalog.go:283-284`), where it means asset files copied into a generated Claude tree, not occurrences of skill calls in prose. Claude agent rendering is one useful precedent: typed skill identities are resolved to effective Claude binding names and rendered into generated frontmatter (`internal/claudecode/claude_agent_document.go:100-146`).

7. **Generated, digested trees already have a precedent.** Claude composite skills read reviewed files, build a canonical file list, and calculate source, definition, and resulting tree fingerprints (`internal/claudecode/claude_composition.go:62-180`). This is the closest safe implementation pattern for alias-aware skill materialization.

8. **Receipts must describe the projected result.** Installed receipts record projected paths and content digests, authorize safe removal/update, and detect drift (`docs/capability-packs.md:69-81`; `CONTEXT.md:132-156`). A derived tree therefore must be fingerprinted after rendering. The source digest alone is insufficient; renderer/version and alias/reference inputs must participate in the desired/definition identity so Preview/Apply freshness remains exact.

9. **The actual packs contain semantically different reference forms.** Current catalog evidence includes pstack links such as `$tdd` and `/tdd` labels paired with a `../tdd/SKILL.md` destination, prose invocations such as `$tdd` and `/teach`, Matty prose `/tdd`, a statement that a worker “calls the Skill tool with `tdd`”, and plain backticked `tdd`. These occurrences are not equivalent syntax and cannot all be inferred from text alone (`packy-catalog/packs/pstack/skills/{codex/,}poteto-help/**`, `packs/matty/skills/engineering/{implement,implement-spec,ask-matt}/SKILL.md`).

10. **Relevant prior work treats aliases as explicit conflict resolution, not content transformation.** Issue/PR #817 fixed alias contract mutation while preserving aliases as projection naming. Issue #824 established the pre-mutation host-discovery block for divergent same-name skills. Neither defines internal reference rewriting.

## Why generic text rewriting is unsafe

- `/tdd` can be an OpenCode/Claude-style invocation, prose, an example, or part of a filesystem path/URL.
- `$tdd` can be a Codex invocation, shell variable, template syntax, or literal teaching text.
- `` `tdd` `` can name a skill, CLI argument, source symbol, package, branch, or conceptual term.
- `../tdd/SKILL.md` is a relative link whose correct rewrite depends on both the referring tree layout and the target tree's projected location. Renaming the destination to another sibling can make it `../pstack-tdd/SKILL.md`; materializing each skill independently or moving assets elsewhere changes that calculation.
- A global find/replace can rewrite examples, quotations, code fences, URLs, unrelated identifiers, or another pack's names. A Markdown AST reduces syntactic false positives but cannot infer semantic intent for plain prose or code spans.
- YAML round-tripping can alter reviewed bytes and comments. Frontmatter `name` should be patched through a narrowly validated scalar operation or rendered from an explicit reviewed template, then the complete output should be digested.

## Viable safe design

1. Add a typed, reviewed reference contract owned by each skill resource. It should identify the target as portable `{kind:"skill", id:"tdd"}` and enumerate the exact semantic sites to render. Prefer explicit placeholders/directives in adapted catalog content (for example, an invocation token and a relative-skill-link token) over inference. Exact expected text plus file-relative location can support controlled migration, but a stable directive is simpler and safer long term.
2. Resolve all effective binding names for the composed Pack before projection. Reference rendering must use the same alias map that resolves the target binding, including references from non-aliased consumers to an aliased target.
3. Render `SKILL.md` frontmatter `name` from the effective binding. Render only declared invocation/link directives. Preserve every other byte. Reject a declared reference that is missing, duplicated unexpectedly, targets an unselected/unrealizable resource, escapes the tree, or lacks a surface rendering.
4. Materialize canonical derived trees for global and project scope instead of symlinking aliased sources. Use the Claude composite-tree pattern: canonical file ordering/modes, source fingerprint, transformation definition fingerprint (surface + renderer revision + effective alias map + declared references), and final tree fingerprint.
5. Feed `TreeFiles` or an equivalent generated-tree action to discovery observation so Preview sees the rewritten frontmatter before mutation. Keep the existing blocker whenever either tree is unverified or a legacy skill lacks the new contract.
6. Persist aliases as intent as today; receipts/locks should continue storing projected targets and final digests. Expose adapter provenance/renderer version in plans so updates to rendering invalidate stale plans and trigger an ordinary reviewed update.
7. Update catalog validation to prove every directive resolves within the same Pack's selected dependency closure and every declared output is deterministic for each supported surface. Catalog content that needs adaptation must carry adapted provenance/notices under ADR 0041's existing provenance rules.
8. Supersede ADR 0041's no-rewrite decision before implementation. No existing skill tree is reinterpreted: the new capability is an explicit opt-in. Older engines already reject unknown surface capabilities; no second format reader is needed.

## Recommendation for the proposed issue/PR

Do **not** merge a PR that searches/replaces `/name`, `$name`, code spans, or relative paths automatically, or one that only rewrites frontmatter. It would violate ADR 0041, leave known Matty/pstack references stale, and weaken exact reviewed-content provenance.

I would merge a narrower first PR only if its issue explicitly includes: accepted replacement ADR; typed reference/template schema; deterministic generated-tree projection on all three surfaces and both global/project scopes; final-output receipt digests; fail-closed legacy behavior; Matty+pstack `tdd` and `teach` acceptance fixtures covering frontmatter, `$tdd`, `/tdd`, tool-call prose, and relative links; update/deactivate/drift/stale-plan tests; actual-host discovery checks at the documented baseline; and repository-wide validation. The catalog migration/publication should remain a separately reviewed follow-up after the engine contract ships.

## Design decisions and remaining limit

- Naming is deterministic `<pack-id>-<surface>-<binding-name>` when the reviewed binding opts into `pack-surface`; explicit aliases take precedence. Preview shows the effective names and invocations.
- Semantic reference sites use exact source placeholders and direct same-Pack dependencies. Every other user-facing mention remains authored content.
- Actual successful model-backed workflows across all three hosts remain unverified by the existing discovery evidence (`docs/skill-discovery.md:52-72`). Filesystem and deterministic host probes alone cannot prove every natural-language reference dispatches correctly.

## Implemented contract and execution evidence

Issue: https://github.com/yersonargotev/packy/issues/851

[ADR 0043](../../adr/0043-render-reviewed-skill-identities-and-references.md) records
the accepted replacement decision. The implementation uses exact reviewed
`{{packy.skill:<logical-id>}}` tokens rather than inferred lexical references.
A template binding selects native or stable pack/surface naming. Direct
dependencies keep a consumer's target in its selected same-Pack closure.

Every generated tree carries reserved `.packy-skill-template.json` metadata.
Its renderer revision, original source-tree digest, and relevant effective names
participate in the final receipt digest. Sources cannot supply that file. This
makes renderer/source changes observable even if the rendered body stays equal.

The original complete Packs remain blocked, including directory aliases. The
implementation does not reinterpret arbitrary upstream prose. The opt-in probe
authors an isolated, reviewed pstack candidate and leaves Matty unchanged; no
Catalog publication or user installation is modified.

### Repository validation

- `./scripts/validate-packy.sh --ci`: passed formatting, vet, static analysis
  (zero issues), the repository package suite, and capability-domain race tests.
- `go test ./internal/cli -run '^TestIssue851' -count=1`: passed both install
  orders on Codex, Claude, and OpenCode in global and project scope, including
  convergence, aliases, updates, drift, stale previews, source immutability,
  legacy-to-template adoption, and safe retirement.
- Focused renderer tests passed malformed/reserved-token rejection, reserved
  metadata file/directory rejection, unrelated-byte preservation, deterministic
  metadata, source-contract changes with equal rendered bodies, and renderer
  revisions with equal rendered bodies.
- Independent Standards review: zero remaining findings after accepting ADR 0043
  and recording ADR 0041's limited supersession.
- Independent Spec review: both original blockers (durable renderer identity and
  malformed reserved directives) fixed; zero remaining code findings.

### Actual complete-Pack acceptance

The opt-in [Catalog probe](../../../scripts/probes/skill-template-catalog.py)
passed **59 commands** using engine commit
`3c7010c8f44e7d480cac0832aec3924da0b030b9` and Catalog source commit
`15ca0f7033d8d505a2325d20d224ba31fc66fdac`. `--focus-pair` builds the complete
Matty/pstack dependency closures, rather than every unrelated Catalog Pack.

The baseline is Matty 4.0.0 plus pstack 4.3.0. The isolated pstack 5.0.0 candidate
opts `tdd` and `teach` into pack/surface naming on all three surfaces. Its Codex
`poteto-help` retains its native name and gains declared dependencies and reviewed
tokens in `SKILL.md`, `references/prompting.md`, and `references/recipes.md`.
The candidate version is a local test label; no release is published.

| Surface | Project | Global |
| --- | --- | --- |
| Codex | Passed | Passed |
| Claude | Passed | Passed |
| OpenCode | Passed | Passed |

Every cell verifies original full-Pack collision and alias rejection, read-only
availability transition, complete adapted coexistence, materialized namespaced
frontmatter, same-Pack help routes where applicable, and unchanged Matty trees.
Project installations also pass portable `verify`. The supplied source Catalog's
complete Pack tree is identical before and after the run.

Recorded candidate source commit: `cd66c2d7d2e0c541a32c9c4347e3a514a7f5aaa5`.
Adapted archive SHA-256:
`d3f47869cc94e228501c1e69ab9414a9c0dd53feb2e816a756e81ce5b548949b`.
Baseline archive SHA-256:
`53860a583198a3cd802d4b1b50d707e6666a2e82592f446157678ce2b6800f85`.

The probe emits machine-readable `evidence.json` plus its successful owned
workspaces. Reproduction commands and sandbox guarantees are documented in the
[probe README](../../../scripts/probes/README.md).

### Actual native-host checks

The [host probe](../../../scripts/probes/skill-template-hosts.py) then passed
against those same six successful workspaces and the installed executables:

| Host | Version | Project | Global | Observed contract |
| --- | --- | --- | --- | --- |
| Codex | 0.163.0 | Passed | Passed | `skills/list` discovers all four identities without errors |
| Claude Code | 2.1.220 | Passed | Passed | All four explicit commands expand to their complete expected bodies |
| OpenCode | 1.18.15 | Passed | Passed | The actual `skill` tool loads all four complete expected bodies |

The four identities are Matty's `tdd` and `teach`, plus
`pstack-<surface>-tdd` and `pstack-<surface>-teach`. Claude and OpenCode results
include **16 complete-body checks** across the two hosts and scopes. The check
compares the complete expected installed body, avoiding ambiguous shared titles
such as "Teach".

Codex uses its app-server discovery API without creating a thread or model turn.
Claude's localhost recorder returns an intentional HTTP 400 after observing skill
expansion; every case records two requests and expected exit status 1. OpenCode's
deterministic localhost provider requests the native skill tool and then stops;
every case records three requests and exit status 0. Neither performs model
inference or executes the skill workflow. OpenCode's compatibility discovery roots
remain enabled. These results establish native discovery/body loading, not the
semantic success of arbitrary model-driven workflows.

### Merge assessment

**Merge the engine change.** Repository validation, complete real-Pack acceptance,
native discovery/body loading, and both independent review axes pass. The three
review findings are closed: architectural authority, durable renderer identity,
and malformed reserved directives. The residual operational risk is future host
discovery changes; native-host probes are opt-in and should be repeated when
updating supported host baselines.

Release the engine before separately reviewing and publishing Catalog adaptations
that request `skill-template`. This PR does not release the engine, publish the
local candidate, alter active user Packs, or merge itself.

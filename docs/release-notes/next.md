# {{TAG}} — Automatic Pack skill names and verified origin reuse

## Changes since the previous release

The previous stable release is `v0.2.28`.

- Pack authors can opt reviewed skills into the new `skill-template` capability.
  The `pack` naming policy automatically generates `<pack-id>-<binding-name>`,
  such as `pstack-tdd` and `pstack-teach`, on Codex, Claude Code, and OpenCode.
  Users do not need aliases to obtain these names. The `native` policy lets a
  consumer retain its own name while referencing renamed skills.
- Explicit `{{packy.skill:<logical-id>}}` source tokens resolve to the selected
  same-Pack skill's effective native name, including any optional user alias.
  Frontmatter and canonical invocations follow that binding; reviewed invocation
  syntax, tool-call names, and sibling paths keep their surrounding bytes.
  References to another skill require a direct dependency. Invalid or unresolved
  references fail before mutation; arbitrary prose and shell variables are not
  rewritten.
- Template skills use deterministic generated trees in global and project scope.
  Generated trees record renderer/source metadata; installed receipts seal their
  exact final digests, retaining ordinary freshness, drift, ownership, update, and removal
  protections. Catalog source bytes and unrelated installations remain unchanged.
  Lifecycle transitions correctly retire old targets when replacing source links
  with generated trees and stage distinct old/new paths independently.
- Native discovery still blocks divergent same-name trees in overlapping roots,
  including OpenCode's Codex and Claude compatibility roots. Short names do not
  make different host variants interchangeable. Existing non-template conflicts
  remain blocked; no host precedence or persistent launch isolation is assumed.
- Catalog authoring reuses verified exact-origin downloads across validation and
  builds. Reuse never bypasses source, candidate, or baseline validation;
  private temporary copies isolate consumers, while age and entry-count eviction
  bound retained downloads. Corrupt entries are rejected with cleanup guidance.
- Maintainer focused validation checks documentation-only changes explicitly.
  The release handoff now verifies the prior stable baseline, complete release
  notes, and an exact-version/commit publication brief before the immutable tag.

## Catalog adoption

The new template capability requires a reviewed Catalog adaptation. Existing
published Packs are not automatically rewritten. Publish this engine before
adopting and publishing Catalog content that requests the capability; older
engines reject it. This release does not publish or select new Catalog content.

See [skill names and references](https://github.com/yersonargotev/packy/blob/main/docs/capability-packs.md#reviewed-native-skill-names-and-references)
and [native discovery limits](https://github.com/yersonargotev/packy/blob/main/docs/skill-discovery.md)
for the contract, and the [Catalog command documentation](https://github.com/yersonargotev/packy/blob/main/docs/catalog-project.md)
for authoring, verification, candidate isolation, and trust boundaries.

For a new installation, acquire the official catalog and preview activation
before changing a CLI surface:

```sh
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the engine's supported floor.

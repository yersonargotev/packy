# Skill-template short names (2026-10-16)

## Conclusion

The surface segment is not required by Codex, Claude Code, OpenCode, or the Agent Skills layout. Packy can safely define the reviewed automatic naming policy as `native | pack`, where `pack` renders `<pack-id>-<binding-name>` (for example, `pstack-tdd`), while retaining the existing strict same-name discovery blocker and its requested-surface scope.

This policy does **not** make divergent host variants interchangeable. The current pstack `tdd` and `teach` trees differ for Codex, Claude, and OpenCode. Packy's supported guarantee is evaluated for the surface the user asks it to change: Codex and Claude may coexist because neither host discovers the other's root; a requested OpenCode projection inspects its compatibility roots and blocks a divergent same-name definition before mutation. An unrelated future OpenCode launch can still inspect existing Codex and Claude roots, but that hypothetical launch is outside a Codex-only or Claude-only transition's guarantee. Supporting every unrequested future host launch would require a broader product contract, not a surface segment in the generated name.

## Documented facts

### Host discovery does not require a surface segment

- OpenCode stable discovers skills in its native global/project roots and in both Claude-compatible and agent-compatible roots: `~/.config/opencode/skills`, `~/.claude/skills`, `~/.agents/skills`, `.opencode/skills`, `.claude/skills`, and `.agents/skills`. It tells users to keep names unique across all locations. Its portable naming rule is a lowercase kebab-case leaf directory matching frontmatter `name`; it does not prescribe a host/surface component. [OpenCode Agent Skills](https://docs.opencode.ai/docs/skills/)
- OpenCode V2 documents the same native and compatibility roots and walks project roots from the current directory to the project root. It derives the skill ID from the leaf filename/directory, so intermediate grouping directories do not namespace the visible ID. It also documents precedence by source: later sources override earlier sources, with `.agents` after `.claude` and native `.opencode` after compatibility roots. [OpenCode V2 Skills](https://opencode.ai/v2/docs/skills)
- Codex discovers repository and user skills under `.agents/skills`; a skill is a directory containing `SKILL.md` with `name` and `description`. The official format does not require the name to include `codex`. [OpenAI, Build skills](https://learn.chatgpt.com/docs/build-skills)
- Claude Code discovers project skills under `.claude/skills/<skill-name>/SKILL.md`, plus its documented personal, managed, nested, and added-directory scopes. Frontmatter `name` may define the visible command and otherwise the directory name is used; the official format does not require the name to include `claude`. [Claude Code, Extend Claude with skills](https://code.claude.com/docs/en/skills)

OpenCode V2 precedence makes duplicate resolution deterministic, but does not make a Codex or Claude adaptation correct for OpenCode. For example, with only `.agents/skills/pstack-tdd` and `.claude/skills/pstack-tdd`, V2 selects the `.agents` entry even though the catalog's Codex and Claude bodies contain host-specific instructions. When `.opencode/skills/pstack-tdd` also exists, the native OpenCode entry wins, but older/stable documentation only requires unique names and does not promise this precedence. Therefore precedence is not a portable coexistence guarantee.

### Packy's accepted architecture supports pack-only naming with blocking

- ADR 0041 models one logical resource across host variants and explicitly says separate host directories are insufficient when a host discovers compatibility directories. Divergent same-name combinations without verified coexistence must block before mutation ([ADR 0041](../../adr/0041-keep-logical-resource-identity-across-surface-variants.md), lines 7-13 and 30-35).
- ADR 0043 now defines the two reviewed modes as `native` and `pack`, with `pack` producing `<pack-id>-<binding-name>`. It retains the rule that remaining collisions block ([ADR 0043](../../adr/0043-render-reviewed-skill-identities-and-references.md), lines 14-19). This is the simplest contract matching the requested automatic `pstack-tdd` behavior; explicit aliases remain an override rather than a required user step.
- The observer already reads the rendered `SKILL.md` name and fingerprints the whole projected tree, including file modes ([adapter.go](../../../internal/skilldiscovery/adapter.go), lines 35-62). It scans the requested host's discovery domain: OpenCode includes the Codex and Claude compatibility roots, while Codex and Claude do not inspect each other's roots (lines 67-94 and 109-128).
- The blocker permits a same-name duplicate only when the complete fingerprints are equal and verified; differing or unverified trees block ([skill_discovery.go](../../../internal/capabilitypack/skill_discovery.go), lines 15-29). This is the correct safety boundary for `pack` naming.
- A `skill-template` binding must already use the canonical native invocation for its surface; `validateBindingV3` rejects any other value before the naming policy is applied ([catalog.go](../../../internal/capabilitypack/catalog.go), lines 1110-1125). The acceptance probe adaptation also writes canonical `/name`, `$name`, or `name` invocations for Claude, Codex, and OpenCode fixtures ([issue851_skill_templates_test.go](../../../internal/cli/issue851_skill_templates_test.go), lines 519-528). Legacy pstack Claude bindings such as `invocation: "tdd"` therefore require catalog adaptation when opting into `skill-template`; they are not an unresolved renderer ambiguity.

### The catalog variants are materially different

The pstack manifest keeps `tdd` and `teach` as one logical resource each with Claude, Codex, and OpenCode variants ([pstack pack.json](https://github.com/yersonargotev/packy-catalog/blob/15ca0f7033d8d505a2325d20d224ba31fc66fdac/packs/pstack/pack.json)). The variant files have different SHA-256 values and different host instructions:

| Skill | Common | Codex | Claude | OpenCode |
| --- | --- | --- | --- | --- |
| `tdd` | `eeb868e…` | `242e0a5…` | `a709bb3…` | `e0906d7…` |
| `teach` | `bd286e5…` | `9fc1a4f…` | `0475b72…` | `590f020…` |

Codex variants direct sibling lookup to `.agents/skills`; Claude variants direct it to `.claude/skills` and add Claude manual-invocation behavior; OpenCode variants direct it to `.opencode/skills` and use OpenCode metadata and permissions. These differences mean that equal short names across roots are not equal-tree duplicates and must not pass the blocker.

## Requested-surface policy and test coverage

The current root filter intentionally scans all compatibility roots during an OpenCode transition, while a Codex or Claude transition scans that host's own root plus the native OpenCode root ([adapter.go](../../../internal/skilldiscovery/adapter.go), lines 109-128). This matches the existing support boundary: validate what the requested host discovers, without treating an unrequested hypothetical OpenCode launch as part of every Codex or Claude operation.

The existing `TestNativeAndCompatibilityDiscoveryInBothOrders` covers OpenCode↔Codex and OpenCode↔Claude, both orders, equal and divergent fingerprints, auxiliary-file differences, and file-mode differences ([adapter_test.go](../../../internal/skilldiscovery/adapter_test.go), lines 25-69). `TestIssue824CodexClaudeVariantsCoexistAcrossUpdateAndRemoval` separately proves the deliberate Codex/Claude coexistence policy through update and removal in global and project scope ([issue824_coexistence_test.go](../../../internal/cli/issue824_coexistence_test.go), starting at line 102).

Required mixed-surface coverage for `pack` naming:

1. Name resolution: for one logical `tdd` resource, every effective surface renders `pstack-tdd`. An explicit alias still overrides that default, and generated frontmatter, target directory, references, receipt metadata, preview, and the advertised canonical invocation all agree. Catalog/probe adaptation must normalize legacy invocations before enabling `skill-template`, as closed validation already requires.
2. Every surface pair—Codex/Claude, Codex/OpenCode, Claude/OpenCode—tested in both install orders and with the transition requested from each applicable surface. Equal verified trees remain non-blocking. Divergent OpenCode/native-or-compatibility pairs block when OpenCode is the discovery host. Codex/Claude pairs continue to coexist under the documented requested-surface policy.
3. Scope combinations: global/global, project/project, global/project, and project/global for each pair, including project ancestor walking up to the git worktree boundary. Combining three pairs, two orders, four scope combinations, and equal/divergent content gives the 48-case matrix expected for the short-name policy.
4. OpenCode native singular and plural roots (`skill` and `skills`) wherever the supported engine versions still scan both.
5. A three-root case with divergent Codex, Claude, and OpenCode trees proving that a requested OpenCode projection blocks before mutation and does not silently choose by installation order or V2 precedence; do not turn the established Codex/Claude-only coexistence case into a blocker.
6. An end-to-end lifecycle case using reviewed templates and real pstack-shaped variants: install one surface, preview a second, verify the discovery blocker and zero filesystem mutation; repeat in reverse. Also retain a positive identical-tree case so the strict blocker does not prohibit harmless duplicate projections.

These tests establish filesystem discovery and Packy's refusal behavior. They cannot prove which duplicate an arbitrary host version will advertise, auto-select, or execute. That requires versioned native host probes in the actual launch contexts, as ADR 0041 already requires.

## Decision for the parent

Adopt `naming: pack` and `<pack-id>-<binding-name>` for the automatic short name. Keep canonical invocation validation, canonical catalog/probe adaptation, and the strict blocker when the requested discovery host sees a divergent duplicate. Preserve the existing Codex/Claude coexistence policy. A stronger promise that every future, unrelated OpenCode launch will select the intended variant remains unsupported and would require a separate product decision and native-host evidence.

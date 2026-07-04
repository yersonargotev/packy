# Wayfinder map: Matty product shape

## Notes

Domain: Matty — a lightweight, multi-CLI AI workflow toolkit with Matt Pocock-style skills, Engram memory, and explicit subagent delegation.

Standing preferences for this effort:
- User writes in Spanish and wants concise progress notes.
- Keep Matty lighter than Gentle AI: reduce initial system prompt/token cost and avoid SDD-first defaults.
- Initial CLI surfaces: Codex and OpenCode only. Future candidates: Claude Code, Antigravity, GitHub Copilot CLI.
- Prefer Matt Pocock engineering skills as the workflow vocabulary; validate whether `code-review`/`review` covers the desired review flow before inventing a 4R-style replacement.
- Use Engram for memory, but design graceful degradation for ambiguous project detection, missing hooks, and transport differences.
- Use subagent delegation when there is a safe independent slice; keep the main agent responsible for decisions, integration, and verification.

Initial repo facts already observed while charting:
- `gentle-ai/`, `engram/`, and `skills/` are cloned as sibling repos under this workspace.
- There is no `.codegraph/` index at the workspace root; use normal file reads/rg unless a subrepo has its own index.
- `docs/agents/issue-tracker.md` now documents a local-markdown tracker for this effort.
- Matt Pocock's current clone exposes `code-review` as the active review skill; an older/personal install may expose it as `review`.
- Engram already has Codex and OpenCode integration surfaces, but project resolution can be ambiguous in this workspace because multiple git repos are nested.

Skills to consult while resolving tickets:
- `grilling` and `domain-modeling` for product boundary and terminology decisions.
- `research` when comparing Gentle AI, Engram, Matt Pocock skills, or external CLI/subagent capabilities.
- `prototype` when turning a decision into a rough installer/config artifact.
- `review`/`code-review` when evaluating implementation changes against standards/spec.

Delegation preflight for this map:
- Delegation was authorized by the repo instructions and used for independent read-only exploration slices.
- Dots explorer agents on `gpt-5.3-codex-spark` inspected `skills/` and `engram/`; a `gentle-ai/` explorer was launched and may be consulted if it completes later.

## Decisions so far

<!-- Closed ticket pointers go here. -->

## Fog

- Packaging and distribution are intentionally foggy until the CLI integration architecture and initial skill bundle are decided. Possibilities include a standalone Matty CLI, generated config overlays, or a plugin-style installer.
- The exact prompt budget target is not sharp yet. First define what Matty must inject at session start versus what can be discovered lazily.
- Cross-CLI parity is unclear: Codex and OpenCode may not support identical hooks, subagents, or skill-loading semantics. The first pass should optimise for an honest common core plus surface-specific adapters.
- Review workflow naming is unsettled: the user's mental model mentions `review 4r`, while the Matt Pocock clone currently has `code-review` and prior memory says the user liked the Matt `review` skill. Decide after comparison.
- Future CLI support should stay foggy until Codex/OpenCode prove the architecture: Claude Code, Antigravity, and GitHub Copilot CLI belong in an extension point, not in the first implementation plan.

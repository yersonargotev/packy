# Matty roadmap

Matty v0 is implemented as an installer/configurator for Codex and OpenCode.
The next work should prove the tool in sandboxed manual use before broadening
the product.

## Next checkpoint

Run a manual smoke test in a sandboxed HOME/config environment:

```bash
go test ./...
matty install --dry-run
matty install
matty doctor
matty update --dry-run
matty update
matty uninstall --dry-run
matty uninstall
matty doctor
```

Do not validate by writing to the operator's real HOME/config paths.

## Near-term follow-ups

| Topic | Question to answer |
| --- | --- |
| Packaging | How should users install the Matty binary after v0 proves the lifecycle? |
| Smoke testing | Which sandbox script should become the canonical manual acceptance check? |
| Token budget | What measurement proves Matty is materially lighter than Gentle AI at session start? |
| Review workflow | Is Matt Pocock `review`/`code-review` sufficient, or does Matty need a distinct review layer later? |
| Engram ambiguity | What user-facing guidance is needed when Engram project detection is ambiguous? |

## Future adapters

These are intentionally out of v0 until the Codex/OpenCode architecture is proven:

- Claude Code.
- Antigravity.
- GitHub Copilot CLI.
- Gemini, Cursor, or other host CLIs.

When adding adapters, keep the same boundary: Matty should configure host-specific prompts/state through narrow adapters and avoid growing the core prompt.

## Historical planning source

This roadmap consolidates durable points from the previous
`.scratch/matty-product-map` exploration. The detailed exploration tickets were
temporary planning artifacts, not runtime documentation.

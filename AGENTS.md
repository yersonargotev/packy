# Agent guidance

- Read the relevant accepted ADR under `docs/adr/` before changing architecture; keep architectural decisions there rather than duplicating them here.
- Keep Packy domain behavior in its owning package under `internal/`; `internal/cli` should adapt that behavior to commands and state.
- Sandbox `HOME` and `XDG_CONFIG_HOME` for tests or manual checks that resolve or write user paths.
- Run `./scripts/validate-packy.sh` as the repository validation authority
  before committing or reporting success. Keep `go test ./...` green while the
  repository has no vendored upstream Go content.

## Engineering principles

- Do not preserve backward compatibility. Optimize for the current
  requirements and remove obsolete paths instead of adding compatibility
  layers, fallbacks, or migrations.
- Choose the simplest implementation that fully meets the current
  requirements. Avoid speculative abstractions, configuration, and
  indirection.
- Build in small, end-to-end increments. Keep the product working after each
  meaningful change.
- Give each component clear ownership of a cohesive concern. Introduce a new
  boundary or module only when it provides a concrete separation benefit.
- Before implementing common functionality, inspect the project's existing
  dependencies, documentation, and types. Use an existing capability when it
  fits; otherwise prefer an established, well-maintained library when it
  reduces total complexity or materially improves reliability.
- Design for every known requirement without planning a later replacement.
  When requirements are uncertain, prefer simple and reversible decisions.

## Agent skills

### Issue tracker

Issues are tracked in GitHub through the `gh` CLI; external pull requests are not a triage surface. See `docs/agents/issue-tracker.md`.

Explicit requests to deliver one approved issue end to end use the project-local
`.agents/skills/deliver-issue` gate and its canonical
`workflows/issue-delivery.md` contract.

### Triage labels

Canonical triage roles map to the repository's existing status vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

Packy uses a single-context domain layout. See `docs/agents/domain.md`.

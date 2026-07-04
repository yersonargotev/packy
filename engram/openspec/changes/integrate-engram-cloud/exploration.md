## Exploration: integrate-engram-cloud

### Current State
`engram` already contains key local-first primitives needed for cloud replication: mutation journal in `internal/store` (`sync_state`, `sync_mutations`, enrolled projects), sync status exposure in `internal/server`, and chunk transport abstraction in `internal/sync`. However, this repo does **not** include the cloud subsystem from `engram-cloud` (`internal/cloud/*`) nor the CLI/cloud wiring in `cmd/engram/main.go` (cloud auth, cloud config, remote transport setup, autosync manager lifecycle). This means the safest path is additive integration on top of existing local behavior, not replacement.

### Affected Areas
- `cmd/engram/main.go` — entrypoint where cloud commands/config/autosync wiring would be integrated; highest regression risk.
- `internal/sync/sync.go` + `internal/sync/transport.go` — must preserve existing local sync while adding cloud remote transport usage paths.
- `internal/store/store.go` — already has local mutation journal/enrollment; must remain source-of-truth and deterministic for local-first semantics.
- `internal/server/server.go` — already exposes `/sync/status` and write hooks; integration must keep local API behavior unchanged.
- `internal/cloud/**` (to be imported from `engram-cloud`) — cloudstore, cloudserver, auth, remote transport, autosync, dashboard packages.
- `internal/setup/plugins/**` and `plugin/claude-code/scripts/**` — launch/startup script behavior may be impacted indirectly; defer rewrite, keep compatibility in this change.
- `go.mod` / `go.sum` — dependency additions (`lib/pq`, `jwt/v5`, `x/crypto`, `templ`) must be introduced carefully to avoid unrelated drift.
- `docs/*`, `DOCS.md`, `README.md` — docs must be updated in lockstep with behavior changes (especially cloud command surface and constraints).

### Approaches
1. **Big-bang parity merge from `engram-cloud`** — port all cloud-related diffs at once (commands, cloud packages, setup/docs, plugin scripts, dashboard).
   - Pros: Fastest route to feature parity with `engram-cloud`; fewer intermediate states.
   - Cons: Very high blast radius in `cmd`, setup/plugin/docs; hard to isolate regressions; violates “preserve current engram behavior” safety intent.
   - Effort: **High**

2. **Selective additive integration (recommended)** — import cloud subsystem in phases, wire only explicit cloud entrypoints first, keep all existing local flows as defaults.
   - Pros: Matches local-first guardrails; isolates risk by boundary; allows push/pull regression testing per phase; keeps launch script rewrite deferred and measurable.
   - Cons: Longer delivery; temporary dual-path complexity while both local-only and cloud-capable paths coexist.
   - Effort: **Medium/High**

3. **Sidecar/bridge integration** — keep cloud server in separate repo/runtime and only add minimal client commands in this repo.
   - Pros: Minimal immediate code churn in `engram`; low local regression risk.
   - Cons: Architecture split across repos increases long-term maintenance cost; weakens single-binary strategy and testability in this repo.
   - Effort: **Medium**

### Recommendation
Use **Approach 2: Selective additive integration** with strict phase gates and rollback points.

Suggested phased plan (safe sequencing):
1. **Phase A — Import-only boundary**: bring `internal/cloud/{auth,cloudstore,cloudserver,remote,autosync,dashboard}` into this repo with module path normalization, no command wiring yet.
2. **Phase B — Explicit cloud commands**: add `engram cloud ...` entrypoints and cloud config handling in `cmd/engram/main.go`; keep `serve`, `mcp`, `search`, `context`, `sync` default-local unless explicit remote/cloud flags are used.
3. **Phase C — Background sync wiring**: enable autosync in long-lived local processes only when valid cloud config exists; preserve current behavior when unconfigured; ensure blocked sync returns loud errors (no silent drop).
4. **Phase D — Policy + UX hardening**: validate project enrollment/paused controls and deterministic failures (409/explicit status) across push/pull paths; update docs/examples.
5. **Phase E — Launch script follow-up (deferred)**: after integration stabilizes, run dedicated validation/rewrite of launch/session scripts as a separate change with explicit compatibility matrix.

### Risks
- **Command-surface regression** in `cmd/engram/main.go` can break existing local flows if defaults change.
- **Dependency churn risk** from cloud stack imports (`templ`, Postgres/auth libs) may cause unrelated build/test failures.
- **Policy mismatch risk** if paused/unenrolled project behavior diverges between local journal and cloud enforcement.
- **Silent sync failure risk** if autosync errors are not surfaced through `/sync/status` and CLI status commands.
- **Docs drift risk** if new commands/env vars are added without same-PR documentation updates.
- **No `openspec/config.yaml` in this repo**: phase constraints must be carried explicitly in artifacts/tests to avoid ambiguity.

### Ready for Proposal
**Yes** — proceed to `sdd-propose` with a proposal that scopes this as phased additive integration, defines hard non-regression gates for local behavior, and explicitly defers launch script rewrite into a follow-up validation change.

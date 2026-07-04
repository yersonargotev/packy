# Exploration mirror: memory-conflict-surfacing

This file is a brief pointer to the full exploration content stored in Engram.

- Engram topic_key: `sdd/memory-conflict-surfacing/explore`
- Engram observation id: 2623
- Project: engram

## How to read the full exploration

Use the Engram MCP tools:

```
mem_search(query: "sdd/memory-conflict-surfacing/explore", project: "engram")
mem_get_observation(id: 2623)
```

## Summary

The exploration mapped the Engram codebase reality for conflict surfacing:

- **Schema**: `observations` table layout in `internal/store/store.go`,
  idempotent migrations via `addColumnIfNotExists`, `modernc.org/sqlite v1.45.0`.
- **FTS5**: `observations_fts` with BM25 ranking already wired into
  `Search()`, `sanitizeFTS()` for query escaping, negative-rank convention.
- **MCP surface**: `mem_save` and `mem_search` return JSON envelopes via
  `respondWithProject`; `serverInstructions` is the only "always available"
  agent instruction surface.
- **AddObservation logic**: three-path decision tree (topic_key upsert →
  dedupe window → new insert), all wrapped in a single transaction with
  `enqueueSyncMutationTx`.
- **Cloud sync**: `sync_mutations` journal on local, `cloud_mutations` on
  Postgres; relations would need a new entity type if synced (deferred to
  Phase 2).
- **Test patterns**: `newTestStore` and `newMCPTestStore` helpers; no
  migration N→N+1 test pattern exists yet (this change establishes one).

## Architectural decisions made downstream

The exploration surfaced six forks-in-the-road. The user resolved all of them
before the propose phase began. See the proposal document
(`proposal.md` / engram topic `sdd/memory-conflict-surfacing/proposal`) for
the locked decisions and rationale.

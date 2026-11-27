---
status: accepted
---

# Search current catalog skills by text

## Context

Agents discover on-demand skills by running `packy list --json` and then
`packy show <pack> --surface <surface> --json` for each relevant Pack. The
official catalog holds 14 Packs and about 300 skill definitions, and their
descriptions alone take roughly 72 KB. Finding one skill this way loads most
of the catalog into the agent's context, and the cost grows with each Pack.

[Issue 863](https://github.com/yersonargotev/packy/issues/863) put "a CLI
semantic search/ranking engine" out of scope for discovery. Its concern was
relevance ranking, automatic ambiguity resolution, and model-dependent
selection.

## Decision

Add Skill Search as `packy search <query>... --surface <agents|claude> [--json]`,
a root command beside `list`, `show`, and `get` per
[ADR 0034](0034-make-packy-root-namespace-pack-oriented.md).

Skill Search is a deterministic text filter, not the ranking engine that
issue 863 excluded. Query arguments split on whitespace into terms. A skill
matches when every term is a case-insensitive substring of its qualified
skill, its native name on the surface, or its effective description on the
surface. Pack descriptions and instruction bodies are not searched. Results
are sorted by qualified skill and carry no score.

Skill Search covers the same Packs as `packy list`, the selected Catalog
Snapshot of every registered catalog. It reuses the effective-resource and
compatibility facts of `show --surface`. It returns incompatible candidates
with their reason. When two catalogs provide the same qualified skill, both
candidates appear with their catalog, and `get skill` keeps its ambiguity
failure.

The versioned `skill-search` report holds the query, the surface, and a
compact candidate list. Dependencies, definitions, and prerequisites stay in
`show --surface`. No matches is a successful empty result. Errors follow the
`list` and `show` conventions.

## Consequences

Agents can find a candidate in one small call, then inspect it with `show`
and obtain it with `get skill`. Lexical matching misses synonyms. The
`packy-cli` skill covers this by trying a few terms before falling back to
`list` and `show`.

Skill Search is local and offline after explicit acquisition. It never
refreshes, prepares resources, executes content, or lets installed state
order or hide candidates. Synonyms, fuzzy matching, body search, relevance
ranking, non-skill resources, and TUI changes are out of scope.

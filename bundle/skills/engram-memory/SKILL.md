---
name: engram-memory
description: Project memory only. Activate before work when a prior project decision, root cause, convention, configuration, or discovery could materially change the approach, or after work when it produced one of those durable project findings. Never activate for personal memory.
---

# Engram Memory

Use the Engram CLI selectively. It is an optional, best-effort memory aid, not
part of the primary task's delivery path.

## Recall

Search only when prior project knowledge could materially change the current
approach. Search for one lookup intent at a time using one to three distinctive
terms. Prefer literal project anchors such as an identifier, subsystem, error,
decision, or convention name over sentences or collections of related
concepts. One lookup intent names the subject of the search; it does not limit
the number of returned memories. Choose the stable project identifier used by
the current repository. Resolve this skill's directory and run its reviewed
helper:

```bash
bash "<skill-directory>/scripts/engram-memory" search "<project>" "<narrow query>"
```

The helper invokes `engram search "<narrow query>" --project "<project>"
--limit 5` and treats a CLI failure as best-effort.

Inspect every returned memory for relevance, then use useful context as input
to the task. If a material project memory is expected and the first query is
empty, refine once: remove generic terms and search the strongest literal
anchor. If output is truncated, do not infer the missing text or save it as
fact. Do not search for routine work, curiosity, or background that cannot
change the approach.

Complete recall when relevant context is found or both targeted searches are
empty.

## Preserve

After the primary work is complete, save at most one concise structured
observation, and only after a durable result is known. Capture the result and
why it matters for future work; do not save a task transcript, progress note,
or speculative conclusion.

Every write must name its project explicitly and use the structured fields
`What`, `Why`, and `Where`:

```bash
bash "<skill-directory>/scripts/engram-memory" save "<project>" "<concise title>" "What: <durable result>
Why: <future value>
Where: <relevant subsystem or path>"
```

The helper always invokes `engram save` with `--project`. Add a final
`"<topic>"` helper argument only when the observation belongs to an evolving
topic whose later observations should be grouped or updated; the helper then
adds `--topic "<topic>"`. Do not add a topic for a one-off discovery.

Complete without writing for routine, transient, already documented, or
low-future-value results. If the CLI is unavailable, fails, or returns an
error, continue delivering the primary task and do not retry indefinitely.

## Boundaries

This skill provides curated CLI search and a possible single durable
observation. It does not promise full observation retrieval, session
lifecycle, automatic compaction recovery, or behavior equivalent to
`engram setup`. Project memory excludes user identity, personal preferences,
collaborator profiles, and cross-project or global memory.

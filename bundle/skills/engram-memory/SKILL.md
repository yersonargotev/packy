---
name: engram-memory
description: Use when prior project knowledge could materially change the current approach, or when completed work produced a durable decision, root cause, convention, configuration, or reusable discovery.
---

# Engram Memory

Use the Engram CLI selectively. It is an optional, best-effort memory aid, not
part of the primary task's delivery path.

## Recall

Search only when prior project knowledge could materially change the current
approach. Form a narrow query from the decision, subsystem, failure, or
convention that matters now. Choose the stable project identifier used by the
current repository, then run:

```bash
engram search "<narrow query>" --project "<project>" --limit 5
```

Use useful returned context as input to the task. An empty result means no
relevant memory was found; continue normally. If output is truncated, do not
infer the missing text or save it as fact. Refine the query once to the
specific decision or subsystem, then continue with the available context. Do
not search for routine work, curiosity, or background that cannot change the
approach.

## Preserve

After the primary work is complete, save at most one concise structured
observation, and only after a durable result is known. Capture the result and
why it matters for future work; do not save a task transcript, progress note,
or speculative conclusion.

Every write must name its project explicitly and use the structured fields
`What`, `Why`, and `Where`:

```bash
engram save "<concise title>" "What: <durable result>
Why: <future value>
Where: <relevant subsystem or path>" --project "<project>"
```

Add `--topic "<topic>"` only when the observation belongs to an evolving topic
whose later observations should be grouped or updated. Do not add a topic for
a one-off discovery.

Complete without writing for routine, transient, already documented, or
low-future-value results. If the CLI is unavailable, fails, or returns an
error, continue delivering the primary task and do not retry indefinitely.

## Boundaries

This skill provides curated CLI search and a possible single durable
observation. It does not promise full observation retrieval, session
lifecycle, automatic compaction recovery, or behavior equivalent to
`engram setup`.

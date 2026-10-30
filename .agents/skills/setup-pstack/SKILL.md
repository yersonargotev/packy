---
name: setup-pstack
description: Configure the models and reasoning budget used by pstack roles on the active agent host. Use for /setup-pstack, "configure pstack models", or "pstack budget".
---

## Codex execution

This is a Codex-specific workflow carried by the Agents convention. A shared Agents binding does not provide Codex tools, model choice, or delegation; use only capabilities authorized and available in the actual client.

Read sibling pstack skills from this installation, not from another host's roots. Project skills belong under `.agents/skills`; personal skills under `~/.agents/skills`.

Delegate through the available Codex subagent API; use only its advertised agent types and parameters. Delegate only when current instructions permit it. Read `~/.pstack/codex-models.md` for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

# Setup pstack

Write the portable pstack preference file at `~/.pstack/codex-models.md`. Other
pstack workflows read it when they choose a worker model. It is not a host rule
and does not change the host's own model configuration.

## Steps

1. Detect the current host and the models its subagent tool actually accepts.
   A model shown in a picker is not proof that its subagent tool accepts it.
   If no model list is available, use `inherit-parent`; do not guess slugs.
2. Read an existing `~/.pstack/codex-models.md` if present. Preserve role choices
   still supported by this host. Drop retired role names.
3. Ask the user for a reasoning budget: unlimited (max), large (xhigh),
   medium (high), or small (medium). Offer large when no preference exists.
   Apply the requested tier to confirmed models, including every panel entry,
   while preserving supported role choices and panel sizes. Show the resulting
   model and reasoning tier for each role before writing. Let the user change
   individual roles. Use the host's advertised model and reasoning parameters;
   do not construct model IDs by appending a tier. Use the nearest confirmed
   lower tier when the requested tier is unavailable, and report the fallback.
   Keep `auto` and `inherit-parent` entries unchanged: omit the model override
   and inherit the host reasoning setting. If no model catalog is available,
   record the budget as a preference, not an applied reasoning tier.
4. Write the complete file atomically. Use the role names below. A panel value
   can list multiple models; each entry represents one candidate or reviewer.
   Use `inherit-parent` for a role when the host cannot select a model for it. The cross-judge pool is tried in preference order and only supported models from a family other than the parent qualify; `auto` and `inherit-parent` never judge.
5. Read the file back and report the active choices and any host limitation.

```text
# pstack model preferences for the current host. This file is read by pstack
# skills; it is not automatically loaded by the host.
# host: codex
# budget: large
feature, refactoring: inherit-parent
bug-fix: inherit-parent
perf-issue: inherit-parent
hillclimb: inherit-parent
judgment and prose: inherit-parent
hardest tasks: inherit-parent
how explorer: inherit-parent
how explainer: inherit-parent
why investigators: inherit-parent
why synthesizer: inherit-parent
reflect tooling: inherit-parent
reflect judgment, divergent, synthesizer: inherit-parent
arena runners: inherit-parent, inherit-parent
arena cross-judge pool: inherit-parent
swarm workers: inherit-parent
architect runners: inherit-parent, inherit-parent
interrogate reviewers: inherit-parent, inherit-parent
```

Read this file only when `# host` matches the current host. On another host,
ignore its model values and use `inherit-parent` until the user configures that
host. If no subagent facility exists, run a role in the current context and
report that the review was not independent.

## Optional project verification

After setup, check for an app-driving verification skill or harness. When absent, offer once to generate one with the optional `$create-verification-skill` route. Check separately for a formal verification skill. When absent, offer once to generate one with the optional `$create-formal-verification-skill` route. Read either selected route only after the user accepts; preserve its own compatibility and authorization requirements.

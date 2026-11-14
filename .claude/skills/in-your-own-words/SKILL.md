---
name: in-your-own-words
description: "Restate in your own words what you think the user's goals are and what problem they're trying to solve. Use for $in-your-own-words or /in-your-own-words."
disable-model-invocation: true
---

## Host execution

Apply this workflow when explicitly requested.

Delegate through the host's native subagent tool: Codex's advertised subagent API, or Claude Code's Agent tool with an available agent type such as general-purpose. Use only the agent types and parameters it exposes. Delegate only when current instructions permit it. Read `~/.pstack/codex-models.md` on Codex or `~/.pstack/claude-models.md` on Claude Code for confirmed role preferences; otherwise inherit the host model. `inherit-parent` means omit an override, not a model ID. Same-model reviewers are not a multi-model panel. When delegation is unavailable, label sequential work as such; a required independent-review gate remains incomplete.

Use accessible current-workspace history and authorized tools. Missing history, live control, scheduling, or external integrations is a reported limitation. Monitoring lasts only while the session actually runs unless a supported scheduler is verified. Preserve the user's scope and authority for messages, merges, deployments, and destructive actions. Required evidence gates remain blocked until real evidence exists.

On Agents, this workflow assumes Codex capabilities; a shared Agents binding does not provide Codex tools, model choice, or delegation. Use only capabilities authorized and available in the actual client. On Claude Code, read manual-only pstack dependencies as files; do not invoke them through the Skill tool or preload them as model-invoked skills.

Restate in your own words what you think the user's goals are and what problem they're trying to solve.

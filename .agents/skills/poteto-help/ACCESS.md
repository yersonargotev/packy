---
name: poteto-help
description: Help users configure pstack, start a task with pstack:poteto-mode, choose a workflow or principle, and troubleshoot a run in Codex. Use pstack:poteto-help with a question.
---

## On-demand context

Use the initiating retrieval result's explicit `surface` and `snapshot_id` for
this procedure and every nested request. Read the prepared qualified pstack
entries linked below; native Skill-tool registration and installed siblings are
not required. For a fresh nested descriptor, use:

```sh
packy get skill pstack:<skill> --surface <initiating-surface> --snapshot <initiating-snapshot-id> --json
```

Keep that host and snapshot after catalog selection changes. Missing retained
content requires restoring that exact revision. Another Pack's same-name skill
or a modified native copy cannot replace the selected pstack resource. Optional
routes and comparisons name qualified skills but are not required dependencies.
Obtain an optional route only when selected, with the same host/snapshot and its
own reviewed compatibility; unavailable optional advice does not block this flow.

Resolve references relative to this entry and `resource_root`. Resolve project
inspection and outputs against the user's intended target, with writable scratch
separate from immutable resources. Pass host, snapshot, target, scratch/output
and prepared entry/resource locations to every delegate or handoff. Reading a
resource does not execute it, apply setup, install tools or verify prerequisites.
Preserve this procedure's invocation policy, approvals and capability limits.
Read supporting references only when their procedure branch needs them.

## Qualified discovery and selection

Discover applicable pstack resources with `packy show pstack --surface
<initiating-surface> --json`. Use the returned qualified identities, descriptions,
compatibility and unverified prerequisites. For a selected optional recommendation,
obtain `pstack:<skill>` with the initiating surface/snapshot using the command above,
then read its actual returned host entry and required references before recommending
or routing it. A table entry is advice, not a required dependency or native
invocation. Optional resources do not require installing every suggestion. Missing
retained content fails explicitly. Never substitute Matty TDD, another Pack's
same-name skill, another host or an installed customization for pstack content.

Read prepared pstack:poteto-mode and pstack:setup-pstack when their branch applies.
Missing preferences alone are not a blocker. Explaining setup does not authorize
writing it. For installation advice inspect the current CLI help and catalog version.
The host's native invocation examples are replaced here by qualified resource
requests; read the selected entry as a file under its invocation policy.

## Prepared pstack dependencies

- [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}})
- [pstack:setup-pstack]({{packy.access:pstack:setup-pstack}})

# Poteto help

Answer the user's question about pstack, give a prompt they can send, and link the prepared file the answer came from. A help question calls for an explanation. A request for action, such as "use pstack to fix this bug", authorizes the work: read [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) and follow it within the user's scope.

Read the selected qualified Codex entry before recommending it. Those files own the workflow details and take precedence over this map. Use the returned entry/resource locations; project skills are authored under `.agents/skills`, personal skills under `~/.agents/skills`. The upstream Cursor guide describes a different host. Cite the prepared adaptation for Codex behavior.

## Find out what they need

Infer the need from the message and conversation. If it is unclear, ask whether they need setup, help starting a task, a workflow recommendation, troubleshooting, or customization. Answer only the relevant branch.

Check state only when it changes the answer:

- Read `~/.pstack/codex-models.md` when setup, cost, or model choice matters. Without confirmed preferences for this host, roles inherit the host model; there is no guaranteed model diversity.
- Check for a project `verify-*` skill or another app harness before recommending live verification. If neither exists, suggest `pstack:create-verification-skill`.
- Discover qualified pstack entries. Obtain and read a selected optional recommendation; if unavailable, explain its purpose and the access gap without pretending to load it.

When preferences are missing and setup matters, offer `pstack:setup-pstack` once per conversation and answer the question too. Missing preferences alone do not require setup before work can proceed.

A question about a race or invariant can use the optional `pstack:create-formal-verification-skill` route when no formal check exists. When covered code changed after its recorded sync commit, suggest the optional `pstack:maintain-formal-verification-skill` route. Setup offers app and formal verification separately after model configuration. A multi-day program uses host-run orchestration; it does not require the retired orchestration scripts.

## Get set up

For Packy installation questions, inspect the available CLI help and catalog version before giving an install or update command. This help skill does not install the Pack or change global settings merely to explain setup.

Use [pstack:setup-pstack]({{packy.access:pstack:setup-pstack}}) to choose a reasoning budget and confirmed per-role models. It writes portable preferences that pstack reads when choosing workers; it does not configure Codex itself. It then offers once each to generate app and formal verification skills when the project has none. Start a task with `pstack:poteto-mode`, a goal, and an observable completion check.

Extra workers and review panels consume extra tokens. A shorter configured panel uses fewer workers. A smaller reasoning budget requests a supported lower tier. `inherit-parent` and `auto` omit the model override; neither guarantees a cheaper model or lower reasoning effort. Respect each workflow's required evidence and independent-review gates.

## Start a task with `pstack:poteto-mode`

[pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) selects a playbook from the task and records its steps. Read [prompting guidance](references/ACCESS-prompting.md) when helping word the request. State the goal, done check, evidence, and actual constraints; the playbook supplies the procedure.

Use `pstack:poteto-mode` to explicitly select the skill for a new task. This Pack does not install a persistent Codex mode. For a follow-up, preserve the active task and completed work; "new task" selects a fresh playbook. Follow the live Codex tool contract for delegation and model parameters. Do not prescribe Cursor Custom Modes, `poteto-agent`, or `/loop` as Codex features.

## Pick a skill

Recommend `pstack:poteto-mode` for a broad workflow. Name a leaf skill when the user wants a specific operation. Read its instructions before giving an example. The links below are optional recommendations: obtain and read the selected qualified pstack entry before suggesting or routing it.

| The user wants to | Skill |
|---|---|
| Do any non-trivial task with rigor | [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) |
| Check that the agent understood the goal and problem | `pstack:in-your-own-words` |
| Get a prompt rewritten deeper, not bigger, before sending it. It returns the rewrite and does not execute the task | `pstack:sharpen` |
| Catch up on what changed in this chat, with every item waiting on the user listed and decidable from the reply | `pstack:ketchup` |
| Catch up with a short subtitled video and the waiting calls as text | `pstack:ketchup-video` |
| Know how code works now, or where new code should live | `pstack:how` |
| Know why code is shaped this way, or where a number came from | `pstack:why` |
| Understand a change or subsystem, explained plainly | `pstack:teach` |
| Catch up on their own recent work on a topic | `pstack:recall` |
| Know what a small diff could break outside itself | `pstack:blast-radius` |
| Settle types and module shape before code that crosses a function boundary | `pstack:architect` |
| Get several attempts at one brief, merged into the best one | `pstack:arena` |
| Run parallel checks over slices, or race workers, using available workers | `pstack:swarm` |
| Have independent reviewers challenge a diff | `pstack:interrogate` |
| Fix a bug test-first when a cheap local test exists | `pstack:tdd` |
| Apply TypeScript rules to `.ts` or `.tsx` work | `pstack:typescript-best-practices` |
| Strip comments before review, using a reviewer that didn't write them | `pstack:no-comments` |
| Clean AI tells out of prose | `pstack:unslop` |
| Write docs, an RFC, a README, a PR description, or a commit message to a standard | `pstack:technical-writing` |
| Hear the last reply again in plain words | `pstack:bro` |
| Give agents a scripted way to drive the app and prove behavior | `pstack:create-verification-skill` |
| Bring a verification skill and its feature map back in line with the app | `pstack:maintain-verification-skill` |
| Formally verify a race, invariant, or other property that tests rarely reach | `pstack:create-formal-verification-skill` |
| Bring formal verification back in line with changed code and distinguish drift from a bug | `pstack:maintain-formal-verification-skill` |
| Draw a TLA+ statechart with the shortest path to each state that breaks a rule | `pstack:statechart` |
| Vet a performance number before reporting or acting on it | `pstack:benchmark-checklist` |
| Run a large or cross-cutting change, or one to review after stepping away | `pstack:figure-it-out` |
| Keep a decision log during a run, and review it afterward | `pstack:show-me-your-work` |
| Pick a model for each role and a reasoning budget | [pstack:setup-pstack]({{packy.access:pstack:setup-pstack}}) |
| Turn their own working habits into a personal mode skill | `pstack:automate-me` |
| Turn what a finished task taught into skill edits | `pstack:reflect` |
| Stop agents from repeating the same mistakes in this repo | `pstack:correct` |
| Build a page whose buttons wake a Grok Bot over an available webhook integration | `pstack:make-bot-ui` |
| Find their way around pstack | `pstack:poteto-help` |


If a qualified pstack discovery entry is absent from the table, obtain it on the initiating surface/snapshot and read its entry before routing by its description. For an unavailable optional resource, offer the prepared pstack:poteto-mode workflow when it covers the request, or explain the gap.

Close calls:

- `pstack:how` explains mechanics; `pstack:why` explains reasons; `pstack:teach` presents the result plainly.
- `pstack:arena` gives independent workers the same brief and synthesizes candidates; `pstack:swarm` splits work into slices or a race.
- `pstack:architect` proceeds to implementation after design. Add "with checkpoint" when the user wants to approve the design first.
- `pstack:interrogate` reviews a diff; `pstack:blast-radius` checks consequences outside it.
- `pstack:recall` uses accessible, authorized history and reports gaps. `pstack:ketchup` covers this chat since the user's last message. Picking up one branch uses the Session pickup playbook.
- `pstack:figure-it-out` designs one rigorous run; Orchestrate coordinates a program across PRs; Autonomous run drives a task toward a completion check.

External integrations such as bot webhooks, browser control, and schedulers require available tools and the user's task authority. pstack has no standalone `$orchestrate` skill; Orchestrate is a playbook. A separately installed skill with that name has its own contract.

## Playbooks and principles

Read the Playbooks section of [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) to select the matching procedure. Playbooks are files, not separately invocable skills.

- "babysit this PR" prepares it for merge; merging requires the user's instruction to merge, land, or ship.
- "land the stack" selects Shipping; "take over this branch" selects Session pickup.
- "pause safely" selects Pause safely.
- "full autopilot on this queue" selects Autopilot-full; "don't merge" selects Autopilot-stack.
- "run the eval playbook" selects Eval.

For a multi-phase plan, read the [pstack:poteto-mode]({{packy.access:pstack:poteto-mode}}) `playbooks/ACCESS-multi-phase-plan.md`. A planning-only request produces a plan without implementation. For an unresolved design question, consider Prototype or `pstack:architect` within the user's requested scope.

Principles are qualified `pstack:principle-*` resources read by the relevant workflow. When the user selects a principle, obtain `pstack:principle-<name>` with the initiating surface/snapshot and read its returned entry before explaining the decision it changes.

## Fix a run that went wrong

| Symptom | Next step |
|---|---|
| The workflow stopped applying | Explicitly select `pstack:poteto-mode` and restate the active goal and completed steps. |
| A question continued the previous task | State that this is a help question or a new task. |
| A model preference had no effect | Check the host marker, role name, and live subagent API; report rejected or unsupported overrides. |
| A skill did not activate automatically | For native installation questions, inspect its `agents/openai.yaml`; many pstack workflows preserve explicit invocation. For on-demand work, obtain `pstack:<skill>` with the initiating surface/snapshot and read the returned entry as a file under its invocation policy. |
| Parallel workers overwrote each other | Give each worker an isolated worktree or output directory before retrying. |
| An overnight run stopped | Check whether the session remained active or a supported scheduler actually ran; define observable completion checks. |
| A green build was called proof | Ask for evidence from the real command, flow, stored value, or profile. |

Read [prompting guidance](references/ACCESS-prompting.md) for short steering examples and [recipes](references/ACCESS-recipes.md) for a matching starting prompt. Scheduling and background completion are claims only when verified; an active-session loop does not persist after the session ends.

## Make pstack my own

`pstack:automate-me` drafts a personal workflow from available history; `pstack:reflect` proposes lessons from completed work; `pstack:correct` encodes recurring mistakes into structural checks. Check availability before recommending them. The poteto-mode authoring and eval playbooks cover new skills and behavioral checks.

Packy owns installed resource bytes. For a requested customization, identify the maintained source or use a separate personal skill; explain that editing managed files can create drift on update. Changes outside the requested task need their own scope.

## Reply

Lead with the answer, then give at most one relevant example prompt from [recipes](references/ACCESS-recipes.md) and a link to the prepared source you read. Keep the answer short unless the user asks for the full map.

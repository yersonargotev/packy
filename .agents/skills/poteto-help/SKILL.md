---
name: poteto-help
description: Help users configure pstack, start a task with $poteto-mode, choose a workflow or principle, and troubleshoot a run in Codex. Use $poteto-help with a question.
---

# Poteto help

Answer the user's question about pstack, give a prompt they can send, and link the installed file the answer came from. A help question calls for an explanation. A request for action, such as "use pstack to fix this bug", authorizes the work: read [poteto-mode](../poteto-mode/SKILL.md) and follow it within the user's scope.

Read the relevant installed Codex skill before recommending it. Those files own the workflow details and take precedence over this map. Use paths from this installation; project skills live under `.agents/skills`, personal skills under `~/.agents/skills`. The upstream Cursor guide describes a different host. Cite the installed adaptation for Codex behavior.

## Find out what they need

Infer the need from the message and conversation. If it is unclear, ask whether they need setup, help starting a task, a workflow recommendation, troubleshooting, or customization. Answer only the relevant branch.

Check state only when it changes the answer:

- Read `~/.pstack/codex-models.md` when setup, cost, or model choice matters. Without confirmed preferences for this host, roles inherit the host model; there is no guaranteed model diversity.
- Check for a project `verify-*` skill or another app harness before recommending live verification. If neither exists, suggest [create-verification-skill](../create-verification-skill/SKILL.md).
- Read sibling skills from this installation. If an optional recommendation is missing, say it is not installed and explain its purpose without pretending to load it.

When preferences are missing and setup matters, offer `$setup-pstack` once per conversation and answer the question too. Missing preferences alone do not require setup before work can proceed.

A question about a race or invariant can use the optional `$create-formal-verification-skill` route when no formal check exists. When covered code changed after its recorded sync commit, suggest the optional `$maintain-formal-verification-skill` route. Setup offers app and formal verification separately after model configuration. A multi-day program uses host-run orchestration; it does not require the retired orchestration scripts.

## Get set up

For Packy installation questions, inspect the available CLI help and catalog version before giving an install or update command. This help skill does not install the Pack or change global settings merely to explain setup.

Use [setup-pstack](../setup-pstack/SKILL.md) to choose a reasoning budget and confirmed per-role models. It writes portable preferences that pstack reads when choosing workers; it does not configure Codex itself. Start a task with `$poteto-mode`, a goal, and an observable completion check.

Extra workers and review panels consume extra tokens. A shorter configured panel uses fewer workers. A smaller reasoning budget requests a supported lower tier. `inherit-parent` and `auto` omit the model override; neither guarantees a cheaper model or lower reasoning effort. Respect each workflow's required evidence and independent-review gates.

## Start a task with `$poteto-mode`

[poteto-mode](../poteto-mode/SKILL.md) selects a playbook from the task and records its steps. Read [prompting guidance](references/prompting.md) when helping word the request. State the goal, done check, evidence, and actual constraints; the playbook supplies the procedure.

Use `$poteto-mode` to explicitly select the skill for a new task. This Pack does not install a persistent Codex mode. For a follow-up, preserve the active task and completed work; "new task" selects a fresh playbook. Follow the live Codex tool contract for delegation and model parameters. Do not prescribe Cursor Custom Modes, `poteto-agent`, or `/loop` as Codex features.

## Pick a skill

Recommend `$poteto-mode` for a broad workflow. Name a leaf skill when the user wants a specific operation. Read its instructions before giving an example. The links below are optional recommendations: verify that the selected skill is installed before reading or suggesting it.

| The user wants to | Skill |
|---|---|
| Do any non-trivial task with rigor | [`$poteto-mode`](../poteto-mode/SKILL.md) |
| Check that the agent understood the goal and problem | [`$in-your-own-words`](../in-your-own-words/SKILL.md) |
| Sharpen the approach and task specification before starting | [`$sharpen`](../sharpen/SKILL.md) |
| Catch up on what changed in this chat, with every item waiting on the user listed and decidable from the reply | [`$ketchup`](../ketchup/SKILL.md) |
| Know how code works now, or where new code should live | [`$how`](../how/SKILL.md) |
| Know why code is shaped this way, or where a number came from | [`$why`](../why/SKILL.md) |
| Understand a change or subsystem, explained plainly | [`$teach`](../teach/SKILL.md) |
| Catch up on their own recent work on a topic | [`$recall`](../recall/SKILL.md) |
| Know what a small diff could break outside itself | [`$blast-radius`](../blast-radius/SKILL.md) |
| Settle types and module shape before code that crosses a function boundary | [`$architect`](../architect/SKILL.md) |
| Get several attempts at one brief, merged into the best one | [`$arena`](../arena/SKILL.md) |
| Run parallel checks over slices, or race workers, using available workers | [`$swarm`](../swarm/SKILL.md) |
| Have independent reviewers challenge a diff | [`$interrogate`](../interrogate/SKILL.md) |
| Fix a bug test-first when a cheap local test exists | [`$tdd`](../tdd/SKILL.md) |
| Apply TypeScript rules to `.ts` or `.tsx` work | [`$typescript-best-practices`](../typescript-best-practices/SKILL.md) |
| Strip comments before review, using a reviewer that didn't write them | [`$no-comments`](../no-comments/SKILL.md) |
| Clean AI tells out of prose | [`$unslop`](../unslop/SKILL.md) |
| Write docs, an RFC, a README, a PR description, or a commit message to a standard | [`$technical-writing`](../technical-writing/SKILL.md) |
| Hear the last reply again in plain words | [`$bro`](../bro/SKILL.md) |
| Give agents a scripted way to drive the app and prove behavior | [`$create-verification-skill`](../create-verification-skill/SKILL.md) |
| Bring a verification skill and its feature map back in line with the app | [`$maintain-verification-skill`](../maintain-verification-skill/SKILL.md) |
| Formally verify a race, invariant, or other property that tests rarely reach | [`$create-formal-verification-skill`](../create-formal-verification-skill/SKILL.md) |
| Vet a performance number before reporting or acting on it | [`$benchmark-checklist`](../benchmark-checklist/SKILL.md) |
| Run a large or cross-cutting change, or one to review after stepping away | [`$figure-it-out`](../figure-it-out/SKILL.md) |
| Keep a decision log during a run, and review it afterward | [`$show-me-your-work`](../show-me-your-work/SKILL.md) |
| Pick a model for each role and a reasoning budget | [`$setup-pstack`](../setup-pstack/SKILL.md) |
| Turn their own working habits into a personal mode skill | [`$automate-me`](../automate-me/SKILL.md) |
| Turn what a finished task taught into skill edits | [`$reflect`](../reflect/SKILL.md) |
| Stop agents from repeating the same mistakes in this repo | [`$correct`](../correct/SKILL.md) |
| Build a page whose buttons wake a Grok Bot over an available webhook integration | [`$make-bot-ui`](../make-bot-ui/SKILL.md) |
| Find their way around pstack | `$poteto-help` |


If an installed sibling is absent from the table, read its frontmatter and route by its description. For a missing optional skill, offer the installed `$poteto-mode` workflow when it covers the request, or explain which resource is needed.

Close calls:

- `$how` explains mechanics; `$why` explains reasons; `$teach` presents the result plainly.
- `$arena` gives independent workers the same brief and synthesizes candidates; `$swarm` splits work into slices or a race.
- `$architect` proceeds to implementation after design. Add "with checkpoint" when the user wants to approve the design first.
- `$interrogate` reviews a diff; `$blast-radius` checks consequences outside it.
- `$recall` uses accessible, authorized history and reports gaps. `$ketchup` covers this chat since the user's last message. Picking up one branch uses the Session pickup playbook.
- `$figure-it-out` designs one rigorous run; Orchestrate coordinates a program across PRs; Autonomous run drives a task toward a completion check.

External integrations such as bot webhooks, browser control, and schedulers require available tools and the user's task authority. pstack has no standalone `$orchestrate` skill; Orchestrate is a playbook. A separately installed skill with that name has its own contract.

## Playbooks and principles

Read the Playbooks section of [poteto-mode](../poteto-mode/SKILL.md) to select the matching procedure. Playbooks are files, not separately invocable skills.

- "babysit this PR" prepares it for merge; merging requires the user's instruction to merge, land, or ship.
- "land the stack" selects Shipping; "take over this branch" selects Session pickup.
- "pause safely" selects Pause safely.
- "full autopilot on this queue" selects Autopilot-full; "don't merge" selects Autopilot-stack.
- "run the eval playbook" selects Eval.

For a multi-phase plan, read the [Multi-phase plan playbook](../poteto-mode/playbooks/multi-phase-plan.md). A planning-only request produces a plan without implementation. For an unresolved design question, consider Prototype or `$architect` within the user's requested scope.

Principles are sibling `principle-*` skills read by the relevant workflow. The user can name a principle to steer a run or explicitly invoke `$principle-<name>`. Read the relevant principle before explaining the decision it changes.

## Fix a run that went wrong

| Symptom | Next step |
|---|---|
| The workflow stopped applying | Explicitly select `$poteto-mode` and restate the active goal and completed steps. |
| A question continued the previous task | State that this is a help question or a new task. |
| A model preference had no effect | Check the host marker, role name, and live subagent API; report rejected or unsupported overrides. |
| A skill did not activate automatically | Inspect its `agents/openai.yaml`; many pstack workflows preserve explicit invocation. Select it with `$skill-name`, or read it as a dependency of the requested workflow. |
| Parallel workers overwrote each other | Give each worker an isolated worktree or output directory before retrying. |
| An overnight run stopped | Check whether the session remained active or a supported scheduler actually ran; define observable completion checks. |
| A green build was called proof | Ask for evidence from the real command, flow, stored value, or profile. |

Read [prompting guidance](references/prompting.md) for short steering examples and [recipes](references/recipes.md) for a matching starting prompt. Scheduling and background completion are claims only when verified; an active-session loop does not persist after the session ends.

## Make pstack my own

`$automate-me` drafts a personal workflow from available history; `$reflect` proposes lessons from completed work; `$correct` encodes recurring mistakes into structural checks. Check availability before recommending them. The poteto-mode authoring and eval playbooks cover new skills and behavioral checks.

Packy owns installed resource bytes. For a requested customization, identify the maintained source or use a separate personal skill; explain that editing managed files can create drift on update. Changes outside the requested task need their own scope.

## Reply

Lead with the answer, then give at most one relevant example prompt from [recipes](references/recipes.md) and a link to the installed source you read. Keep the answer short unless the user asks for the full map.

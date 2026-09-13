# Packy's future as sustainable open-source software

## Recommendation

Packy should focus on helping developers and small teams **review, apply, and diagnose changes to their agent configuration**. Its durable product question should be: “What changed in this project's agent setup, what can we verify, and what needs attention before the team relies on it?” Installation remains part of that job, but installation breadth should not determine the roadmap.

This is a recommendation to validate, not an accepted product or architectural decision. The assessment uses Packy revision `40a4e93661ed2a94582d35a6d0936177c7b81ded` and primary sources accessed on September 13, 2026. The planning horizons end in September 2027 and September 2029. The objective is useful, sustainable open-source software for individual developers and small teams; revenue, enterprise procurement, and maximum catalog size are secondary.

The strongest competing strategy is to reduce Packy to a small curated distribution and use native host facilities plus Git for everything else. That option should remain a legitimate outcome of the first 90 days. There is substantial evidence that distribution is becoming easier, but no verified evidence yet that external teams need another configuration tool often enough to justify maintaining it.

Four findings support the recommendation:

1. Standard skill and plugin formats, native marketplaces, and existing installers already cover much of the obvious distribution opportunity.[^1][^2][^3][^4]
2. Portability does not imply identical execution. The common plugin specification deliberately leaves important responsibilities to clients; Packy's existing separation of configuration, authorization, and usability fits that gap.[^2][^5]
3. Packy already has receipts, project verification, drift protection, and structured audit reports. The opportunity is to turn these into a demonstrably valuable workflow, rather than build those mechanisms again.[^6][^7]
4. More installed guidance does not reliably mean better agent outcomes. A small, evidence-backed catalog is a more plausible sustainable contribution than collecting large numbers of skills.[^8][^9][^10]

## Current position

### Assets worth preserving

The current catalog contains seven Packs: Addy, Argote, Engram, Issue Delivery, Matty, Orchestrate, and pstack. Four support Claude Code, Codex, and OpenCode; Engram, Issue Delivery, and Orchestrate currently support Codex only. This is a capability matrix, not seven universally interchangeable bundles.[^11]

The useful foundation is already broader than an installer. Project intent and committed projections are separate from personal activation. Receipts establish ownership, and ordinary mutations stop on owned-file drift or path collisions. `packy verify` checks the committed project contract without consulting credentials or personal runtime state. `packy audit --json` already provides a shareable, redacted report. These are implemented capabilities, not proposed features.[^6][^7]

Readiness uses separate `configured`, `authorized`, and `usable` dimensions with `true`, `false`, and `unknown` results. This prevents successful file copying from becoming a claim that an integration works. The strategic value is understandable diagnosis: a teammate can distinguish a broken shared installation from a missing personal authorization or an unobserved host behavior.[^5]

Managed Pack Projects already author and release Packs independently. Packy admits registered immutable releases through a reviewed promotion process. “Separate Pack authoring from Packy” would therefore duplicate existing work. Content availability to end users still depends on the bundled catalog in the Packy release.[^12]

### Constraints that matter for a broader audience

The current distribution contract offers one bundled current version per Pack. It is not a general resolver for arbitrary versions, and it does not offer arbitrary downgrades. A committed project lock preserves installation evidence; it should not be described as a universal mechanism for reconstructing any historical environment.[^6]

The current authoring model requires a public, maintainer-controlled repository registered by Packy. A small team wanting to distribute its own private engineering workflow cannot simply treat this as a self-service private package system. This is a possible adoption blocker, not a reason to immediately build a registry.[^12]

An especially concrete limitation appears in global update review: when the installed historical contract cannot be reconstructed, `contract_diff.baseline_available` is false. The preview can still explain target actions, but resource comparison is unavailable. The existing comparison classifies resource facts; it is not a complete semantic explanation of changed requirements. Recovering a baseline and explaining requirements are related but separate improvements. This finding does not establish the same gap in project update reporting.[^7][^20]

The repository explicitly rejects backward compatibility and documents a manual reset between earlier generations. That policy is current authority. If external teams adopt Packy, the cost they experience during a contract change becomes a product concern. A future promise of stable automation contracts would require an explicit policy decision; this report does not introduce a compatibility layer or migration obligation.[^13]

### Adoption and maintenance evidence

The repository's v0.2 architecture describes a sole-user starting point. That is historical context, not a current user census. There is no validated external retention, onboarding, or time-saved dataset in the evidence reviewed. Stars, downloads, and commit volume cannot substitute for those measurements.[^13]

A read-only history inspection found 172 non-merge commits since August 13, 2026 at the assessed revision. Changes span domain behavior, promotion, CLI, and adapters. This is a change-activity proxy, not a defect count or an estimate of human hours. The practical implication is to measure maintenance time before adding surfaces, packaging modes, or new trust infrastructure.

The open dependency-signals issue already proposes bounded recurring dependency checks. That work should be accounted for as existing backlog, not invented as a new strategic initiative. Its historical vulnerability observations are not a current vulnerability assessment.[^14]

## External landscape

The comparison below records documented capabilities. “Opportunity to test” does not mean competitors lack that capability; absence of a feature in a reviewed page is not proof of absence from a product.

| Alternative | Verified overlap with Packy | Opportunity to test against it |
| --- | --- | --- |
| Agent Skills | Shared `SKILL.md` structure and portable supporting content | Whether project-specific requirements and changes still need management beyond file placement |
| Agent Plugins 1.0 | Shared packaging of skills and MCP, with client extensions | Whether teams need a clearer view of which required capabilities a particular client can actually use |
| Claude Code native plugins | Native packaging, marketplaces, scopes, and source pinning facilities[^18] | Whether Packy's multi-surface ownership and shared-project diagnosis save time over native management |
| Codex native plugins | Plugin browsing, installation, marketplaces, and host-controlled permissions | Whether a project contract adds value when installation and service connections already have native flows |
| Cursor and Copilot plugins | Native plugin support and documented portable-format integration[^19] | Whether supporting either surface would serve retained users enough to fund its maintenance |
| OpenCode | Native skill discovery across several conventional paths; local and npm JavaScript/TypeScript plugins | Whether Packy explains discovery, configuration, and capability mismatches better than host tools |
| Vercel skills / skills.sh | Multi-agent skill installation and shareable packs, including public and private skills | Whether users need the full Packy lifecycle after this simpler installation path |
| mise / Packslip | Versioned tools with skills, signed release verification, lockfile commitments, and skill synchronization | Whether agent-specific review and diagnosis justify an additional tool for users already on mise |
| Git plus native host configuration | Reviewable configuration history without another package manager | Whether manual review, onboarding, and repair are actually painful enough to improve |

Sources: Agent Skills specification; Agent Plugins documentation; host documentation and the linked ecosystem evidence; skills.sh Packs; mise Packslip backend.[^1][^2][^3][^4][^15][^16]

### Distribution is becoming a weak reason to choose Packy

skills.sh now supports shareable packs combining different skill sources. mise's Packslip backend authenticates release identities and artifacts, records commitments in `mise.lock`, and can synchronize tool-provided skills. These overlap with both convenience and integrity claims. Neither “packs,” “hashes,” nor “multiple agents” is a defensible standalone positioning.[^4][^15]

Native plugin systems also reduce the value of a separate installation interface. For a developer using a single host with a few standard skills, native plugins plus Git may be the best answer. Packy should explicitly help that developer choose the simpler route rather than require adoption for access to useful content.

**Inference:** over one to three years, assuming basic installation becomes less differentiating is more robust than assuming it stays difficult. The rate of standard adoption and the eventual dominant hosts remain uncertain. No market-share forecast is required for this conclusion.

### Standardization leaves some operational differences

Agent Plugins defines a portability floor, while distribution, installation, permissions, and client-specific functionality remain outside that floor. Its conformance rules permit partial component support. An integration can therefore be well formed without every required capability being available on a particular host.[^2]

OpenCode's executable plugin system is also materially different from a portable skill document. Its documentation describes JavaScript/TypeScript modules, event hooks, and npm dependencies. Translating an instruction file and preserving an executable integration's behavior are different engineering tasks.[^16]

**Inference:** the durable boundary is a declared outcome with truthful evidence about each host, not a promise that all agents behave identically. Packy should use a native format when it fits and identify unsupported requirements explicitly. A universal hook translator or lowest-common-denominator workflow engine would increase maintenance while concealing these differences.

### Trust has distinct meanings

For this strategy, separate four questions: where the bytes came from; whether the installed bytes match the reviewed contract; whether the host can use the required capability; and whether that capability improves a task. None establishes all the others.

A verified signature authenticates an identity and artifact under a trust policy; it does not establish harmless behavior. Packslip explicitly makes that distinction. Packy's reviewed catalog likewise should not become a blanket claim of safe agent behavior.[^15]

The most useful small-team promise is narrower: show ownership and intended changes, detect what can be verified, preserve the host's permission boundary, and explain evidence gaps. Security scanners, provenance systems, and package managers can supply evidence where appropriate. Building a universal scanner, certificate authority, or policy platform would be a separate product.

### Skill quality is a conditional benefit

SkillsBench evaluates paired runs with and without skills and reports positive aggregate effects with substantial variation across domains. Its studied model and task configurations do not establish the value of Packy's catalog or future models.[^8]

SWE-Skills-Bench reports that 39 of 49 studied skills produced no pass-rate improvement. Its main experiment uses Claude Code with Haiku 4.5, about 565 task instances, and a one-skill setting; the reported high baseline pass rate also limits headroom. This is useful counterevidence to universal skill-benefit claims, not a forecast that skills become useless.[^9]

WebDev-Skills-Bench reports negative average effects for forced skill injection across its tested models and projects, while identifying useful individual combinations. Its authors note seed variation comparable to headline effect sizes and limits in coverage of visual quality, human review time, and real production work. Forced injection is not identical to selective host discovery.[^10]

The findings are not directly contradictory: they use different tasks, skills, models, selection mechanisms, and evaluation designs. Do not combine their percentages into one market-wide score. The actionable conclusion is to evaluate a claimed workflow benefit in its intended setting and retire guidance that adds cost without benefit.

## Strategic options

These comparisons are qualitative judgments, not measured market scores. Recurring user value and maintainer effort have greater weight than audience size.

| Option | User value | Likely maintenance burden | Main risk | Recommendation |
| --- | --- | --- | --- | --- |
| A. Broader universal installer and marketplace | Convenient discovery and installation across many hosts | High: adapters, intake, moderation, compatibility, distribution | Existing hosts and installers already cover much of the job | Do not pursue as the primary direction |
| B. Reviewed agent-configuration lifecycle for small teams | Understand and safely apply changes; diagnose shared versus personal problems | Moderate if limited to existing surfaces and concrete incidents | Teams may find native tools plus Git sufficient | Primary hypothesis for a 90-day validation |
| C. Small collection of evaluated workflow Packs | Better outcomes from specific, maintained workflows | Moderate; grows quickly with model/host/task combinations | Evaluation cost and changing models erase claimed gains | Supporting strategy for two workflows first |
| D. Curated native distribution with a much smaller Packy | Useful content with low installation friction and less custom machinery | Lowest if redundant code is actually retired | Gives up a differentiated Packy application | Preferred fallback if B lacks recurring value |
| E. Enterprise governance, runtime, or hosted control plane | Central policy and operational control | Very high: service operations, auth, enforcement, support | Misaligned with the chosen audience and available evidence | Exclude from this plan |

Option B uses the strongest existing capabilities without assuming a large market. Option C can make the catalog worth installing even when native distribution wins. Option D protects sustainability if the separate application ceases to earn its maintenance. Options B and C should not become excuses to postpone that decision indefinitely.

A concrete option D experiment would distribute one Managed Pack Project's content directly through a native package and compare it with Packy. If the native route preserves the needed outcome, investigate retaining only portable verification while retiring adapters or UI that no longer add value. Measure the ownership and diagnostic capabilities lost in that reduction; it is not assumed to be a cheap or automatic conversion.

The proposed product statement is: **“Packy helps small teams keep their agent setup reviewable and explain why it changed or stopped working.”** This should be tested in onboarding conversations before replacing the current positioning.

## Product proposal

### 1. Make one team workflow excellent

Target a maintainer of a roughly 2–10 person development team whose repositories contain shared agent instructions or capabilities and whose developers have different local setups. Team size is a targeting assumption, not measured demand. Multiple hosts make a strong test case, but should not be an eligibility requirement if same-host teams have recurring problems.

Use a concrete journey: a contributor clones a repository, checks the committed setup, activates the relevant personal capability, then encounters an update or a host change. The maintainer wants to answer whether shared content changed, an owned file drifted, the selected host lacks a required component, or authorization remains local and incomplete.

Start with existing `verify`, `audit`, `status`, and preview commands. Observe whether users can interpret their results and complete the next action. Count a documentation or diagnostic-message improvement as a successful product increment if it removes the problem. A new command is not intrinsically progress.

A candidate first engineering increment is a more useful global update explanation for an older installed Pack. Start with one observed review question, such as identifying changed selected resources. Keep resource classification, projected-byte changes, requirement semantics, and historical-evidence availability distinct; do not promise all four in one increment. The owning seam is `internal/capabilitypack`, with CLI and TUI presenting its result. If answering the chosen question requires retaining additional installed facts, document the minimum necessary representation in an ADR; a minimal baseline need not reconstruct the entire historical contract.[^7][^12][^20]

### 2. Keep native packaging at the edge

Prefer standard skills and plugin structures for portable content. Packy's distinct metadata should describe reviewed selection, ownership, requirements, and evidence, rather than define another universal skill language. Existing formats and libraries should handle general packaging and acquisition when they meet the contract.

For the first interoperability experiment, compare one simple skill Pack and one Pack with a host-specific requirement. Demonstrate exactly where native installation is sufficient and where Packy adds an explanation or protection. A native manager and Packy must never both believe they exclusively own and update the same paths. Ownership transfer would require explicit design; a prototype should use separate temporary locations.

Do not add a fourth supported surface during the initial validation. Maintain the three current surfaces within their actual capability coverage. Candidate future adapters need a retained user, reproducible fixtures, an owner, and a bounded support commitment. Popularity alone is insufficient.

### 3. Address team-owned content only after verifying the blocker

Ask pilot teams to use a workflow they actually maintain. If the current public registered catalog prevents that, record the blocked scenario rather than substituting an unrelated catalog Pack and declaring adoption validated.

The smallest potential extension is a reviewed project-local immutable content input, with explicit identity, notices, and ownership, evaluated offline. This is a design candidate, not currently supported behavior. It changes the admission boundary in ADR 0038 and needs a separate decision. A private registry, arbitrary repository execution, automatic remote updates, and general dependency resolution are not prerequisites for testing that one need.

If pilot teams only need standard private skills, compare native private distribution and skills.sh's existing facilities before extending Packy. If those solve the problem, route users there. Never require private content to be published merely to fit Packy's catalog model.[^4][^12]

### 4. Curate for outcomes

Choose two workflows with specific acceptance criteria, such as a repository-specific review practice or a repeatable issue-delivery handoff. Generic engineering advice is a weak evaluation target because a capable baseline may already perform the task well.

Each experimental quality note should identify Pack content, host, model, task fixture, date, acceptance criteria, cost, and known limits. Keep this evidence outside ordinary personal receipts: readiness is not a performance leaderboard. Begin with a manually maintained report before adding a new schema or runtime feature.

Compare the native baseline with the selected Pack using the same task and environment. Use several repeats and hold back cases from skill authors. Measure accepted outcomes, human corrections, elapsed time, and token cost where observable. A skill that saves review time may be valuable even without higher test pass rates; a token-heavy skill needs a benefit sufficient to justify that cost. Evaluation guidance supports task-grounded regression suites rather than a single universal score.[^17]

## Delivery plan and decision gates

The dates and thresholds below are proposed management rules. They are not adoption forecasts or statistically established population estimates. A calendar gate cannot pass merely because time elapsed.

### First 30 days: establish demand and a baseline

Recruit 8–12 developers from at least three small teams, with a meaningful portion outside the maintainer's close collaborators. These are proposed interviews, not completed outreach. Ask for the last concrete incident, the current workaround, frequency, time spent, and which files or host changes were involved. Avoid asking only whether the product idea sounds useful.

Run three assisted pilots using existing functionality. Include a single-host team and a mixed-host team where possible. Compare Packy with native plugins plus Git; include skills CLI or mise when that team already uses it. Use equivalent starting states and record assistance, so a maintainer-led demo does not masquerade as self-service usability.

Track onboarding time, review time for one update, diagnosis time for one incident, failure recovery, and maintainer support time. Use content-free notes or consented redacted artifacts. No telemetry service is required.

**Gate:** at least three teams can identify a recent concrete problem in the proposed scope, and at least two agree to exercise the workflow again when a real update or onboarding event occurs. If not, refine the target once rather than build speculative infrastructure.

### Days 31–60: deliver one vertical improvement

Choose the most repeated observed friction. Possible outcomes are clearer diagnosis, a useful review delta, or proof that private team content is the binding constraint. Implement only the chosen outcome with focused package validation and an end-to-end fixture matching the incident.

Pilot acceptance should cover a clean setup, a modified owned file, a collision, and an unavailable host capability. Use benign fixtures, sandboxed `HOME` and `XDG_CONFIG_HOME`, and existing test-process conventions. Demonstrate that a diagnostic separates known failure from unknown state. Do not imply that a multi-Pack operation has guaranteed global rollback.

Prioritize adoption observation and the one selected improvement. A workflow-quality comparison is optional in this period and should run only if it answers a pilot team's concrete question and fits the remaining budget. The second comparison belongs after the adoption gate, using existing evaluation infrastructure where it fits.

**Gate:** two independent teams complete the selected workflow with materially less intervention than their baseline. Use a provisional target of at least 30% lower median time on comparable repeated tasks, and publish raw paired counts. With a tiny sample, treat this as directional evidence, not statistical proof. Any destructive regression blocks expansion regardless of time saved.

### Days 61–90: decide whether Packy earns its place

Repeat onboarding, update, or diagnosis events under normal team conditions. Measure recurrence at the event frequency of the product: a useful configuration utility need not be opened every day. A successful CI run alone does not demonstrate human value or voluntary retention.

**Continue B** if at least three independent teams retain the shared contract and use Packy for a second meaningful event, at least two prefer it over the baseline for a documented reason, and the maintenance budget holds. **Narrow B** if only one incident type creates recurring value. **Choose D** if teams consistently prefer native tools, the pain seldom recurs, or Packy costs more attention than it saves.

If too few real events occur, the result is inconclusive rather than success. Permit one bounded extension with a stated missing observation. Do not use an indefinite pilot to justify ongoing scope expansion.

### By September 2027: useful and supportable

If the 90-day gate passes, aim for 5–10 independently maintained repositories with evidence of repeat use, a second contributor able to handle an adapter fix or release task, and a documented capability matrix for the supported configurations. These are modest operating targets, not predictions.

The product should have one excellent onboarding/change/diagnosis path, two workflow Packs with dated evidence, and clear failure explanations. Add project-local content or decoupled catalog delivery only if pilot observations show that the existing model repeatedly blocks users. Exact-version installation, retained historical contracts, and rollback each need separate problem evidence; they should not arrive as an automatic package-manager checklist.

### By September 2029: keep the job, adapt the implementation

Reassess the competitive baseline every quarter. If native hosts standardize discovery and package management, retire redundant adaptation and retain only useful review and diagnosis. If teams consolidate on one host, support that reality rather than require multi-host complexity. If cloud agents dominate a team's workflow, test a portable repository verification use case before considering a hosted product.

The success criterion is a small maintained tool that teams continue to choose because it prevents or shortens a concrete problem. A much smaller Packy, or native Packs plus a focused verifier, can satisfy that criterion. Preserving today's internal architecture is not the objective.

## Sustainability model

Assume one principal maintainer contributing 4–6 hours per week, with no full-time staffing commitment. This is a planning assumption to validate. During the first 90 days, reserve up to two hours weekly for routine maintenance and use the remaining 2–4 hours for discovery and the selected improvement. A rough experiment budget is 4–6 hours of interviews, 6–9 hours of pilot sessions, four hours of analysis, and 12–20 hours for one bounded improvement: 26–39 hours total, before unexpected support. These are planning allowances, not engineering estimates. At the lower capacity limit, reduce the intervention or extend its delivery; do not squeeze in two evaluation programs. After validation, revisit the allocation using measured support demand.

Set an initial maintenance ceiling of two hours per week averaged over a month for routine compatibility and support. Exceeding it for two consecutive months should pause surface and catalog expansion and trigger scope reduction. Treat episodic security or release work separately so an emergency does not conceal the ordinary cost trend.

Keep the runtime local and the distribution conventional. Avoid a required account, hosted database, always-on daemon, centralized policy service, or default telemetry. Optional sponsorship can offset costs, but the core product should remain operable without a business-model dependency. Sponsorship demand has not been established.

Every maintained Pack and surface needs an identifiable owner and an explicit support scope. New contributions that reuse existing vocabulary should remain predominantly data changes. Contributions requiring new host behavior need fixtures and a maintainer able to diagnose failures. A small catalog with a visible retirement policy is preferable to a large abandoned one.

Before promising long-lived team automation, decide the compatibility policy explicitly. Staying with deliberate clean cuts is possible, but must be explained as such. Alternatively, a later ADR could establish a bounded support policy for public contracts. Neither choice should emerge accidentally from accumulating fallbacks.

## Risks and falsifiers

| Risk or scenario | Evidence or signal | Response |
| --- | --- | --- |
| Native hosts absorb the useful lifecycle | Pilot users complete the same job with native tools and Git, with comparable effort | Reduce Packy; distribute content natively |
| Standards cover syntax but team incidents persist | Repeated partial-support, discovery, or authorization incidents | Deepen diagnostics for those incidents, not universal translation |
| Stronger models make generic Packs redundant | Paired evaluations show no practical benefit or higher correction cost | Retire or narrow those resources |
| Teams cannot use their own workflows | Pilots blocked by public registry requirements | Compare native private distribution; consider one bounded input model only if needed |
| Catalog releases bottleneck users | Repeated requests waiting on binary promotion, with measured lead time | Consider signed/immutable independent catalog delivery after evidence and a trust design |
| Private-content handling expands authority too far | Proposed support needs arbitrary probes, code execution, or broad credentials | Re-scope the use case rather than silently weaken admission |
| Maintenance exceeds the budget | Support log and adapter effort repeatedly exceed the ceiling | Freeze breadth, simplify, or adopt option D |
| Users like demonstrations but do not return | No second real lifecycle event or no preference over the baseline | Treat demand as unproven; stop feature expansion |

The strongest falsifier is straightforward: if native plugins plus Git solve the same team's real problem with equal or lower effort, Packy should not build more infrastructure to manufacture a distinction. The strongest confirming evidence is a repeated incident where Packy's ownership and readiness explanation saves time without requiring the maintainer to interpret it.

## Decisions proposed now

Adopt a 90-day discovery and validation focus around configuration changes and diagnosis. Keep quality curation as a small supporting experiment. Hold new surfaces, marketplace expansion, arbitrary-version resolution, and a hosted platform until demand justifies them.

Do not replace current accepted ADRs with this report. The next implementation decision should come from a real pilot incident, be narrow enough to review, and explicitly identify any current contract it changes. No public outreach, issue creation, roadmap acceptance, or implementation is implied by the research artifact.

The recommendation has **moderate confidence**: confidence is high that basic installation faces strong substitution, but low that an external audience has validated demand for Packy's proposed narrower job. The plan is designed to resolve that uncertainty at a cost compatible with the chosen open-source objective.

## Sources

Primary-source documentation is a point-in-time capability record, not an independent usability study. Undated live documentation below was accessed September 13, 2026. Preprints are author-reported studies, not replications performed for Packy. Local evidence refers to the assessed revision unless explicitly described as a proposed experiment.

[^1]: Agent Skills maintainers. [Agent Skills specification](https://agentskills.io/specification). Living specification. Portable skill structure and metadata.
[^2]: Agent Plugins contributors. [Agent Plugins](https://agent-plugins.org/) and [specification](https://agent-plugins.org/specification). Version 1.0.0. Portability scope, client responsibilities, and component support. See the [ecosystem evidence](packy-future-ecosystem-2026-09-13.md) for host-specific details.
[^3]: OpenAI. [Plugins](https://learn.chatgpt.com/docs/plugins). Living official documentation; the former Codex plugins URL redirects here. Native CLI installation, marketplaces, and permission boundaries.
[^4]: Vercel / skills.sh. [Packs](https://www.skills.sh/docs/packs). Living documentation. Shareable packs and supported public/private skill sources.
[^5]: Packy. [ADR 0035: capability-driven readiness](../../adr/0035-make-pack-readiness-capability-driven.md). Accepted architecture at the assessed revision.
[^6]: Packy. [README](../../../README.md) and [project Pack lifecycle](../../project-pack-lifecycle.md). Current lifecycle, ownership, verification, and bundled-version limits.
[^7]: Packy. [Structured CLI output](../../structured-output.md). Current audit and lifecycle contracts, including unavailable historical contract comparison.
[^8]: SkillsBench authors. [Introducing SkillsBench](https://www.skillsbench.ai/blogs/introducing-skillsbench). Benchmark release report, 2026. Paired design and heterogeneous findings; the headline percentages are not extrapolated to Packy.
[^9]: Tingxu Han et al. [SWE-Skills-Bench: Do Agent Skills Actually Help in Real-World Software Engineering?](https://arxiv.org/html/2603.15401v1). March 16, 2026, v1 preprint. Sections 4.1, results, and future work. The abstract's percentage notation is not converted into a general productivity estimate.
[^10]: Ziyue Yang et al. [Signal or Noise? A Benchmark Study of Agent Skills in Web Development](https://arxiv.org/html/2608.23067v1). August 24, 2026, v1 preprint. Controlled injection design and explicit limitations.
[^11]: Packy. [Generated Pack catalog](../../packs/index.md) and linked Pack guides. Seven current Packs and their supported surfaces.
[^12]: Packy. [ADR 0038: Managed Pack Projects](../../adr/0038-promote-releases-from-managed-pack-projects.md) and [Managed Pack Projects guide](../../managed-pack-projects.md). Current admission and promotion authority.
[^13]: Packy. [Agent guidance](../../../AGENTS.md), [ADR 0031](../../adr/0031-simplify-packy-around-reviewed-packs.md), and [roadmap](../../roadmap.md). Current compatibility policy and historical sole-user scope.
[^14]: Packy maintainers. [Issue #755: Continuously surface vulnerable and stale dependencies](https://github.com/yersonargotev/packy/issues/755). Open backlog inspected September 13, 2026; historical scan findings are not treated as current vulnerability status.
[^15]: mise maintainers. [Packslip backend](https://mise.jdx.dev/dev-tools/backends/packslip.html). Living documentation. Signed release verification, locked installation, skills, and the limits of authentication. See the [trust and sustainability evidence](packy-future-trust-and-sustainability-2026-09-13.md) for additional primary sources.
[^16]: OpenCode / Anomaly. [Agent Skills](https://opencode.ai/docs/skills/) and [Plugins](https://opencode.ai/docs/plugins/). Living documentation. Discovery paths and executable plugin semantics; no runtime equivalence is inferred across hosts.
[^17]: Anthropic. [Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents). January 9, 2026. Task-grounded grading, capability versus regression evaluations, and practical evaluation design.
[^18]: Anthropic. [Plugin marketplaces](https://code.claude.com/docs/en/plugin-marketplaces) and [plugin installation](https://code.claude.com/docs/en/discover-plugins). Living Claude Code documentation. Native distribution, source pinning, and scopes.
[^19]: GitHub. [About GitHub Copilot plugins](https://docs.github.com/en/copilot/concepts/agents/about-plugins). Cursor. [Plugins reference](https://prod.cursor.com/docs/reference/plugins). Living documentation. Native packaging and Agent Plugins support; no runtime comparison was performed.
[^20]: Packy. [Lifecycle output implementation](../../../internal/capabilitypack/lifecycle_output.go), [activation implementation](../../../internal/capabilitypack/activation.go), and [historical comparison tests](../../../internal/capabilitypack/issue762_contract_diff_test.go). Assessed revision. Global update comparison availability and resource classification.

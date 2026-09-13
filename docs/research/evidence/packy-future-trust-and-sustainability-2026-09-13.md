# Packy: trust, reproducibility, and sustainable scope over one to three years

Date: 2026-09-13. Status: non-normative research evidence, not an architecture decision or approved roadmap. Audience: maintainers of a useful, sustainable open-source tool for individual developers and small teams. Repository baseline: `40a4e93661ed2a94582d35a6d0936177c7b81ded`.

## Finding

**The strongest hypothesis is a small, dependable lifecycle tool for reviewed agent configuration: understand what a project expects, preview changes, preserve personal work, detect drift, and explain incomplete readiness.** Treat supply-chain evidence as support for that promise. Do not make a general marketplace, security certification service, enterprise control plane, or full development-environment manager the primary bet.

This is an inference from the overlap below, not evidence of product-market fit. Competitors already distribute skills, bundle them, manage tool versions, and increasingly attach provenance. Packy needs to demonstrate that its complete lifecycle saves users recurring effort that a native plugin manager, checked-in files, or an existing package manager does not.

There is no substantiated adoption, retention, market-size, or willingness-to-pay evidence in this investigation. The objective here is sustainable OSS value, not enterprise monetization. Numbers proposed in experiments are decision thresholds, not observations.

## Existing assets and boundaries

The checked-out [README](../../../README.md) and [domain context](../../../CONTEXT.md) describe a reviewed bundled catalog; exact resource ownership; drift and collision checks before mutation; separately committed project intent and receipts; portable `packy verify`; and a redacted `packy audit`. Readiness distinguishes configured, authorized, and usable. Project verification deliberately excludes credentials, personal activation, and executable availability.

[ADR 0031](../../adr/0031-simplify-packy-around-reviewed-packs.md) records a simplification after earlier machinery exceeded the installer/configurator product. [ADR 0038](../../adr/0038-promote-releases-from-managed-pack-projects.md) keeps authorship in public maintainer-controlled Pack repositories, validates immutable closures without executing project content, and separates acquisition from publication authority. These are useful constraints on future work. A proposal for arbitrary/private catalogs or historical version restoration would change current requirements; neither is an existing capability inferred from the word “lock.”

Packy can currently verify committed projections without proving that an agent loaded them, obeyed them, or can execute their workflows. Its current update target is the bundled version, with arbitrary versions and downgrades unsupported. Consequently, describe its existing reproducibility as **verifiable project configuration**, not complete reconstruction of an arbitrary historical AI workstation.

## Competitor and substitute evidence

The sources below were opened and their relevant documentation inspected on the research date. These are live documentation snapshots, not immutable evidence of when each feature shipped. Advertised support is not a compatibility test performed by this investigation.

| Alternative | Verified first-party capability | Implication for Packy; inference |
| --- | --- | --- |
| Vercel `skills` CLI | Installs skills across many agents, including Packy's three surfaces; accepts local and Git sources, including private repositories; supports project/global installation, copying or symlinking, updates, and removal. [Repository README](https://github.com/vercel-labs/skills) | Generic installation, private Git access, and adapter count are weak reasons to maintain a separate tool. Do not claim that other tools only copy one file or lack lifecycle commands. |
| skills.sh packs | Public and private skill inputs can be combined into a shareable collection. New installations receive current contents. Pack links are unlisted and accessible to anyone holding the URL; pack management uses Vercel accounts. [Packs documentation](https://www.skills.sh/docs/packs) | “Share one install command for my favorite skills” is already served. A reviewed immutable bundle and local project evidence are a narrower distinction. The link privacy model is not evidence that all private Git workflows are insecure. |
| skills.sh security | Documents routine security audits and explicitly declines to guarantee every skill's quality or security. Ranking uses installation telemetry. [Documentation](https://www.skills.sh/docs) | Curation is competitive and does not establish safety. Installation counts should not be used as evidence of retained users, effective workflows, or Packy's demand. |
| mise | Its lockfile records resolved versions and may include artifact checksums and provenance. Its own documentation treats the lockfile as trusted input and distinguishes recorded provenance from proof of verification. [Lockfile documentation](https://mise.jdx.dev/dev-tools/mise-lock.html) | A new binary manager or generic “hashes plus lockfile” pitch would duplicate substantial existing functionality. Integrate through documented tool requirements where actual demand warrants it. |
| mise Packslip resources | Tool releases can carry version-matched skills. `mise skills sync` creates local links in a chosen agent directory, preserves user content, and can prune owned links. These links are not portable committed copies; fetching resources does not execute the tool, while optional generated resources can. [Resource documentation](https://mise.jdx.dev/dev-tools/packslip-resources.html) | This is a closer competitor than a simple runtime version manager. Tool-coupled skills may belong there. Packy's stronger candidate is reviewed composite project configuration beyond tool-owned skills; verify the user benefit rather than assuming uniqueness. |
| mise Packslip trust | Signed manifests and release lists verify publishers; lists can withdraw versions. Optional trusted stampers approve content identified by digests, while vendor signatures remain required. [Verification and policy](https://mise.jdx.dev/dev-tools/packslip-verification.html) | Independent approval and release provenance are not empty territory. Avoid creating another signing protocol or approval network. |
| Nix and Devbox | Nix documents version-controlled declarative shell environments and exact Nixpkgs revisions. Devbox builds isolated development shells from project package definitions using Nix and supports reuse in containers. [Nix tutorial](https://nix.dev/tutorials/first-steps/declarative-shell.html), [Devbox README](https://github.com/jetify-com/devbox) | Toolchains, package resolution, OS dependencies, and environment construction are expensive adjacent ownership. Packy can coexist with these tools while owning its configuration contract. |
| Claude Code marketplaces | Native marketplaces distribute plugins with version tracking, updates, and multiple source types. Managed marketplace restrictions can block non-approved sources before network/filesystem work and apply during installation and updates. [Marketplace documentation](https://code.claude.com/docs/en/plugin-marketplaces) | A Claude-only team may prefer native distribution and policy. Cross-host convenience must survive a comparison against that simpler alternative. Packy is not an enforcement boundary merely because its own installer refuses an operation. |
| Official MCP Registry | Hosts metadata rather than packages, authenticates publisher namespaces, and delegates code scanning to package registries and downstream aggregators. It explicitly permits downstream curation. Its documented scope excludes private servers; its codebase is not supported for self-hosting. [Registry overview](https://modelcontextprotocol.io/registry/about) | Use ecosystem metadata if a concrete Pack needs it. A second broad discovery index adds little. A private registry is an operational commitment, not a small incidental feature. Registry inclusion proves neither safe execution nor Pack admission. |

### Supply-chain trust is several different questions

Keep these questions separate in product language and review:

1. **Identity and provenance:** who supplied the bytes, from which revision and build?
2. **Approval:** who reviewed this exact content for the intended use?
3. **Integrity:** are installed files still the approved files?
4. **Authority and behavior:** what can the loaded capability actually do, and what did it do?

SLSA defines provenance as verifiable production history. Its threat model centers on source/build integrity and explicitly does not directly solve malicious producers or unsafe usage. These are meaningful limits, not reasons to discard provenance. [SLSA provenance](https://slsa.dev/spec/v1.2/provenance), [SLSA threats](https://slsa.dev/spec/v1.2/threats-overview).

GitHub artifact attestations use signed build claims and existing Sigstore infrastructure. GitHub says the claims must be verified to provide security benefit and do not guarantee an artifact is secure. A future Packy attestation experiment should therefore verify expected producer/workflow identity and artifact digest, not merely generate an additional release file. [GitHub artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations).

**Inference:** Packy's maintainable promise can cover review provenance and owned-file integrity. Runtime sandboxing, prompt-injection prevention, credential policy, and continuous behavioral monitoring require different authorities. Selling the former as the latter would create unsupported expectations and a costly support burden.

## Opportunities ranked for a small OSS team

| Option | Recurring developer problem to test | Maintenance assessment | Decision hypothesis |
| --- | --- | --- | --- |
| Make existing project lifecycle easy to understand | A teammate cannot tell whether their configuration matches the project; an update collides with local edits; removal leaves uncertain ownership | Closest to current domain. Documentation, clear diagnostics, and a small lifecycle fixture suite can improve value without a new service | First priority, conditional on observed friction |
| Reviewable capability changes | Maintainer needs to understand which selected resources, projections, origins, or requirements change before accepting an update | Bounded if built from existing manifest/receipt/admission data. Expensive if expanded into semantic safety scoring | Test concrete before/after review evidence; avoid invented risk scores |
| Minimal team governance in Git | Team wants an agreed configuration, a reviewable change, and a CI integrity check | Low incremental cost when using existing project files, Git review, and `packy verify`; higher if it adds another policy language | Present existing workflow clearly before adding policy features |
| Compose with a development-environment manager | A missing executable or wrong host version prevents a reviewed capability from working | Low for actionable diagnostics and examples; high for acquiring every dependency or supporting every environment manager | Start with one documented composition used by actual users |
| Signed release verification | Users need stronger producer/build evidence than downloaded checksums alone | Moderate if existing attestation tooling fits; high if Packy invents key rotation, transparency infrastructure, or a universal trust service | Small experiment only after identifying the actual unverified distribution boundary |
| Large curated catalog or marketplace | Users cannot find useful skills | Human review grows with every update, platform, and executable capability; discovery is already competitive | Keep a small catalog whose maintenance pays for itself in use |
| Enterprise governance platform | Central teams need device enforcement, private distribution, audit retention, policy administration | High operational, compatibility, privacy, and support costs; diverges from chosen audience | Exclude from the strategy unless the product objective changes |
| Full reproducible AI environment | Recreate tools, OS libraries, hosts, authorization, remote services, and agent outcomes | Very high; substantial overlap with environment managers and host-owned concerns | Explicitly reject as the primary ownership claim |

“Low” and “high” are architectural judgments, not measured estimates. The existing multi-host feature matrix may itself be expensive. Measure maintenance per capability and surface; do not assume that a small binary implies a small maintenance obligation.

## One-to-three-year direction with exit conditions

**Next 0–3 months:** validate existing value with people maintaining actual repositories. Prefer documentation, diagnostics, and lifecycle rough edges to expanding catalog or architecture. Publish a short, realistic story: clone a project, inspect its agreed Pack configuration, verify it, activate personally, update deliberately, resolve drift, and remove safely. Record where users needed help.

**Within 3–12 months:** invest only in repeated failure patterns. A small set of maintained Packs and tested host behaviors can be successful OSS without broad adoption. Add a new host only when a contributor or recurring user case can justify its ongoing compatibility work. If nearly all useful work is confined to one host, reduce translation scope instead of preserving cross-host breadth as ideology.

**Within 12–36 months:** aim to remain a dependable configuration lifecycle component alongside native hosts and toolchain managers. Native adoption of a capability should allow a thinner adapter or retirement of duplicated behavior. If users no longer encounter lifecycle problems, preserve the useful validators or Pack content independently and enter maintenance mode; a standalone installer is not an end in itself.

These are conditional directions, not feature delivery dates. A durable niche may be small. Sustainability means users obtain repeatable value while maintainers can support the scope; it does not require becoming a platform.

## Falsifiable experiments

Agree on thresholds before each experiment. The following numbers are proposed starting points, not statistically powered population estimates.

| Experiment | Method and success signal | Counterevidence / stop condition |
| --- | --- | --- |
| Recurring lifecycle value | Recruit 5 developers from at least 3 independently maintained repositories. Observe onboarding and then a real update/removal within 4–6 weeks. Continue if at least 3 voluntarily reuse Packy and can identify a concrete avoided error or saved task | One-time installs, reuse only after maintainer reminders, or content interest with no lifecycle need argue for publishing Packs through existing tools |
| Compare the simplest alternative | Use matched onboarding/update/drift/removal tasks with Packy and the participant's current approach, including native plugins or checked-in files. Record time, errors, interventions, and preference after both | No material improvement, extra concepts dominating the work, or users preferring manual Git changes weaken the installer thesis |
| Review usefulness | Give 5 maintainers an ordinary Pack update diff and a proposed summary derived from existing evidence. Ask them to identify origin, file, executable-requirement, and selected-resource changes. Continue if at least 4 identify consequential changes correctly with less assistance | Cosmetic preference without better comprehension does not justify a new reporting subsystem |
| Honest readiness | Reproduce 6 realistic states: valid projection, edited file, missing executable, unavailable credentials, stale runtime evidence, and host that has not reloaded configuration. Ask users to explain what is proven and what action follows | Any output understood as “safe and usable” when only bytes match is a product defect; fix language and evidence boundaries first |
| Environment composition | In one real repository already using mise or Devbox, demonstrate that its tool setup and Packy's configuration verification cooperate without duplicate ownership or hidden activation | If native tool-coupled skills cover the entire need, document that outcome and avoid the integration feature |
| Provenance verification | In a disposable fixture, verify a legitimate release and reject modified bytes, a wrong publisher/workflow, and mismatched provenance. Record setup and ongoing release work | If downstream consumers do not verify, or existing distribution already covers the threat, defer extra machinery rather than count signed files as success |
| Maintenance budget | For 8 weeks log hours on release operations, Pack reviews, host breakage, support, and useful improvements. Choose a maintainer budget beforehand; an initial example is at most 2 routine maintenance hours per week | Repeatedly exceeding the agreed budget requires reducing Packs, bindings, or promises before increasing reach |

Do not add behavioral telemetry merely to run these experiments. Opt-in observation, issue histories, and voluntary follow-ups are sufficient at this scale. Preserve counts of invited participants and dropouts so enthusiastic responders do not silently become the entire sample.

## Main uncertainties and disconfirming evidence

- **Demand remains unproven.** Repository architecture and ecosystem activity establish feasibility and competitive overlap, not that external developers experience this problem often enough.
- **Portability can erase the differentiation.** Shared skill directories and native plugin formats may make translation less useful. The Vercel and mise evidence already shows increasing overlap. Test composite capabilities rather than winning a host-count comparison.
- **Review can become the bottleneck.** “Reviewed” requires a documented scope and accountable maintenance. A catalog that grows faster than review capacity weakens its own promise.
- **Integrity can be sufficient without Packy.** For a few ordinary text files, Git plus code review may solve the problem more simply. That is the primary baseline, not an artificially weak installer.
- **A verified lock is not an authority boundary.** A developer or host can modify files, install other capabilities, or execute commands outside Packy. Local reports cannot establish organizational compliance or safe model behavior.
- **Documentation moves quickly.** New competitor features were read directly, but no competitors were installed or subjected to lifecycle tests here. Recheck the cited behavior before selecting an implementation or public comparison.

The next decision should follow observed update, drift, and support work. The research supports a bounded hypothesis worth testing; it does not justify expanding Packy's architecture in advance of that evidence.

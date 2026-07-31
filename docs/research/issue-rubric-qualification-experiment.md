# Issue-rubric qualification experiment: measured status and preregistration gap

Research date: 2026-07-31

Issue: [Measure whether an issue rubric reduces qualification rework](https://github.com/yersonargotev/packy/issues/387), part of [Map: Reduce Packy-owned delivery rework and validation latency](https://github.com/yersonargotev/packy/issues/386)

## Decision-ready answer

Five completed Packy Delivery qualifications provide correction counts,
durations, and enough correction content to classify the observed work. The
research ticket can be resolved now as a small descriptive experiment: the
evidence supports no Packy template or linter, but it does not support a causal
claim that the rubric reduced rework.

The defensible retrospective cohort is the first five approved issues that
actually entered Packy Delivery qualification after the experiment's final
[Agent Brief](https://github.com/yersonargotev/packy/issues/387#issuecomment-5140210572):

1. [Establish one process-level release acceptance scenario](https://github.com/yersonargotev/packy/issues/388);
2. [Fail closed on release identity drift at every privileged boundary](https://github.com/yersonargotev/packy/issues/391);
3. [Prove retained-candidate recovery through the release scenario seam](https://github.com/yersonargotev/packy/issues/392);
4. [Integrate release scenarios exactly once into Packy validation](https://github.com/yersonargotev/packy/issues/393); and
5. [Diagnose the current validation performance regression](https://github.com/yersonargotev/packy/issues/390).

That rule operationalizes “the next five approved Packy issues before
qualification” in qualification-start order. No first-party issue or comment
enumerates the five identities before the first attempt at
`2026-07-31T07:03:32.789532Z`. The Agent Brief at `06:57:23Z` fixed a five-issue
cohort and the intervention but named none, and the map's
`06:58:12Z` redesign comment named a work frontier rather than an experimental
cohort
([map comment](https://github.com/yersonargotev/packy/issues/386#issuecomment-5140216829),
[#388 completed run](../../.git/packy/issue-delivery/issue-388/revisions/d832ff17811b80054a2688b43fc5ac56585e288c7dda4474263b8261a6c70714/b0a1663b47587b8c2fdfd152cba78e5c5da16e4f2e49231755a95acc38cad8bb.json)).
The identities are therefore recoverable from the pre-attempt sequential rule
and first-party run order, but not from an explicit preregistration list. That
is a material sampling limitation, not a reason to discard the completed
primary measurements.

[Measure whether validator logs need additional phase timing](https://github.com/yersonargotev/packy/issues/389)
is excluded: it closed without a local Packy Delivery qualification run. Its
absence is also why “the five adjacent issue numbers” is not a valid cohort
definition.

## Fixed intervention and taxonomy

The intervention remains exactly the human-visible four-part rubric in the
[research ticket](https://github.com/yersonargotev/packy/issues/387):

1. objective;
2. enumerated, verifiable acceptance criteria;
3. preservation constraints and out of scope; and
4. dependencies and prerequisites.

It adds no marker, schema, criterion-ID grammar, digest, JSON snapshot, or
linter. The ticket's correction taxonomy remains fixed:

- product intent;
- dependency/prerequisite;
- architecture seam;
- evidence locator;
- preservation proof; or
- Packy Delivery orchestration.

The classification below assigns each correction round one primary category
according to the fact that triggered the round. A completed matrix can contain
secondary evidence of several kinds; those do not become extra correction
rounds.

## Initial issue content

The Packy Delivery authority snapshot at each issue's first measured
qualification attempt contained the same four rubric elements. The short hash
is the prefix of the run's `authority_sha256`; it identifies the exact observed
issue authority without introducing a new Packy protocol.

| Issue | Objective | Verifiable criteria | Preservation / out of scope | Dependencies / prerequisites | Authority |
| --- | --- | ---: | --- | --- | --- |
| [Establish one process-level release acceptance scenario](https://github.com/yersonargotev/packy/issues/388) | Shared checked-in normalization adapter and one valid process scenario | 9 | Fake boundaries, disposable roots, no remote mutation, unchanged release behavior and permissions | None | `9dfb7921bc56` |
| [Fail closed on release identity drift at every privileged boundary](https://github.com/yersonargotev/packy/issues/391) | Inject and deny release-identity drift at each privileged boundary | 8 | No later effect after denial, valid later-main advancement preserved, real checked-in adapters retained | Establish one process-level release acceptance scenario | `97f392a283f7` |
| [Prove retained-candidate recovery through the release scenario seam](https://github.com/yersonargotev/packy/issues/392) | Resume only a sealed release from its original retained candidate | 8 | No rebuild, re-signing, recreation, or invented content; sandboxed fake boundaries | Establish one process-level release acceptance scenario | `9e614c23a3c6` |
| [Integrate release scenarios exactly once into Packy validation](https://github.com/yersonargotev/packy/issues/393) | Integrate the complete release scenario cohort into the single validator exactly once | 9 | No external or real-user mutation; sandboxed paths; ordinary and race coverage preserved | Fail closed on release identity drift at every privileged boundary; Prove retained-candidate recovery through the release scenario seam | `1792745f81be` |
| [Diagnose the current validation performance regression](https://github.com/yersonargotev/packy/issues/390) | Attribute the validation regression and identify only evidence-supported fixes | 8 | Single validation authority and all existing coverage preserved; implementation deferred to separate tickets | None | `1e723f70fbd0` |

The issue bodies used plain Markdown headings and checklists. None contained a
version marker, stable criterion-ID grammar, digest, JSON snapshot, linter
directive, or Governance change.

## Measured cohort

“Correction rounds” is the number of recorded `qualification-correction`
phases. “Qualification wall” runs from the completed run's initial
`qualification` start through its final qualification correction or review.
It includes human/agent work between transitions and is not compiler CPU time.

| Issue | Qualification start | Correction rounds | Qualification wall | Primary correction category |
| --- | --- | ---: | ---: | --- |
| [Establish one process-level release acceptance scenario](https://github.com/yersonargotev/packy/issues/388) | 07:03:32.789532Z | 1 | 3m43.029s | architecture seam |
| [Fail closed on release identity drift at every privileged boundary](https://github.com/yersonargotev/packy/issues/391) | 07:59:35.326930Z | 1 | 1m26.321s | architecture seam |
| [Prove retained-candidate recovery through the release scenario seam](https://github.com/yersonargotev/packy/issues/392) | 09:21:50.518591Z | 1 | 7m05.310s | architecture seam |
| [Integrate release scenarios exactly once into Packy validation](https://github.com/yersonargotev/packy/issues/393) | 10:33:32.938515Z | 2 | 13m21.138s | architecture seam; Packy Delivery orchestration |
| [Diagnose the current validation performance regression](https://github.com/yersonargotev/packy/issues/390) | 11:54:21.563576Z | 1 | 3m50.330s | evidence locator |
| **Cohort** |  | **6** | **29m26.129s total; 3m50.330s median** |  |

The completed run records are the measurement authority
([#388](../../.git/packy/issue-delivery/issue-388/revisions/d832ff17811b80054a2688b43fc5ac56585e288c7dda4474263b8261a6c70714/b0a1663b47587b8c2fdfd152cba78e5c5da16e4f2e49231755a95acc38cad8bb.json),
[#391](../../.git/packy/issue-delivery/issue-391/revisions/4c7c1c5214e4613191c2023a513a11543242023cd742a5fd15ce10cd21918a0c/5571057c35dab9baf0a62d225d3b4f3e1b2b8ac071f92ca6938e018ca84d6161.json),
[#392](../../.git/packy/issue-delivery/issue-392/revisions/5f5c9301190a6aeb7f793e55ee8db72578155f97632e317658c2bcfd2124b871/5c6e8881c7b0c94c82fc4732a8ce2bec78e90aff3b8d88719d1a1b828ba1b824.json),
[#393](../../.git/packy/issue-delivery/issue-393/revisions/7d947a4c38707256ee7cf62cb7040d8612173713107ae5a80350586568d2e80f/a9bd8d7f83cac5346fddf51aba5934434cb1a3c43680d5e9e1b4e81151b87f2b.json),
[#390](../../.git/packy/issue-delivery/issue-390/revisions/b5b5fe6f4acf554f8710573948fc4dfdd47427b8fa1366274bd7e300a3080775/7125bfcc1ae3a22ab70041711199698e2988dcb0d92c61079022c64a5569ef1d.json)).

### Classification evidence

- **Establish one process-level release acceptance scenario** replaced nine
  compiler placeholders in one round with the shared adapter, process scenario,
  fake boundary, sandbox, deterministic result, and reuse seams. Authority text
  and criterion identities were preserved: architecture seam, with no product
  intent change.
- **Fail closed on release identity drift at every privileged boundary**
  replaced one placeholder with the process-scenario seam across every named
  privileged boundary and existing verification adapters: architecture seam,
  with no product intent change.
- **Prove retained-candidate recovery through the release scenario seam**
  replaced one placeholder with the retained-run acquisition, identity
  verification, recovery classification, and fake-effect seam: architecture
  seam, with no product intent change.
- The first **Integrate release scenarios exactly once into Packy validation**
  round bound nine criteria to the existing `internal/release`, validator, CI,
  developer-command, and sandbox seams: architecture seam. Its second round
  corrected which evidence belongs to candidate review versus exact exhaustive
  validation, and stopped attributing delivery receipt invalidation to the
  validator command: Packy Delivery orchestration. Neither changed product
  intent.
- **Diagnose the current validation performance regression** bound eight
  research criteria to controlled measurements, exact research artifacts,
  explicit limitations, unchanged validator topology, and separate follow-up
  scope: evidence locator, with no product intent change.

No correction round's primary cause was product intent,
dependency/prerequisite, or preservation proof. Preservation evidence was
filled in during the matrix corrections, but the completed records do not show
a round triggered primarily by a missing preservation constraint in an issue.

## Historical comparison

Four completed pre-intervention runs provide a descriptive comparison, not a
causal control:

| Issue | Correction rounds | Qualification wall |
| --- | ---: | ---: |
| [Consolidate classic lifecycle CLI execution plumbing](https://github.com/yersonargotev/packy/issues/349) | 1 | 6m29.716s |
| [Make required Packy PR workflows warning-clean](https://github.com/yersonargotev/packy/issues/378) | 1 | 8m10.995s |
| [Make classic lifecycle anti-drift checks semantic](https://github.com/yersonargotev/packy/issues/380) | 1 | 4m00.766s |
| [Automate secure GitHub Release and Homebrew publication from version tags](https://github.com/yersonargotev/packy/issues/384) | 4 | 10m48.984s |
| **Comparison cohort** | **7** | **29m30.5s total; 7m20.356s median** |

Sources:
[#349 completed run](../../.git/packy/issue-delivery/issue-349/revisions/3c59f0f959d1786c6eace078725d5e7d3ee2cefdbdc2a350e6a4b38f6b2decd1/582fc6a53a9e501db56a89bf65d480724734a736b62854da42eeb2040889592c.json),
[#378 completed run](../../.git/packy/issue-delivery/issue-378/revisions/833adfa9e7d42f2f374e08eed3a2ea5ecd9430d15f7bb9df4be8f4664b689b79/d8f2e8ca60cb648e5a948804cc78c5c308ae91601e681d33ccb1a3cf9914bb03.json),
[#380 completed run](../../.git/packy/issue-delivery/issue-380/revisions/b282d3b301d5bb68885ac58998a40fdd7a9d65179d9702a6467b9cc9fae3b80d/e1888278fb2ddecc234ed7ce2024ca7e89acbf71d9c281ca6e9281dcf7ef34c9.json), and
[#384 completed run](../../.git/packy/issue-delivery/issue-384/revisions/0282c0865dac956d093cb34904b9c003ede19e4b03092e7ae75e5786c7bc82b7/922ddee34a8d2f4228ba1588914101775401e08deeeb0b5d252a19fd4b7bd10a.json).

The measured cohort has fewer corrections per issue (1.2 versus 1.75) and a
lower median qualification wall time (3m50.330s versus 7m20.356s). These are
descriptive facts only. The cohorts differ in size, risk, work type,
dependencies, issue relatedness, and qualification path; they were not
randomized or prospectively matched. The retrospective cohort selection and
uncontrolled concurrent process changes prevent attributing the difference to
the issue rubric.

## What the evidence supports

### Measured facts

- Five completed qualifications contain six correction rounds and complete
  timing records.
- All six rounds changed evidence/owning-seam compilation; none changed product
  intent.
- Five rounds were primarily architecture-seam or evidence-locator work. The
  remaining round corrected Packy Delivery phase ownership.
- No repeated correction was a mechanically detectable omission of the
  four-part human rubric.

### Hypotheses

- The rubric may have reduced product-intent and dependency corrections.
- Qualification rework may be dominated by evidence-plan and owning-seam
  compilation even when the issue contains the four rubric elements.
- The lower descriptive median may reflect issue mix or process changes rather
  than the rubric.

The measurements support **no Packy template or linter now**. That conclusion
is narrower than “the rubric reduced rework”: there is no repeated,
mechanically detectable issue-body omission to automate.

## Measurement limitations

The five identities were not explicitly enumerated before qualification. This
report applies the sequential “first five approved issues to enter Packy
Delivery after approval” rule consistently and discloses that operationalization
instead of presenting the cohort as randomized or prospectively matched.

There is a further measurement limitation for **Fail closed on release identity
drift at every privileged boundary** and **Prove retained-candidate recovery
through the release scenario seam**: each completed run supersedes an earlier
typed-decision run. The table follows Packy Delivery's completed-run phase
record and does not count the earlier typed caller decision as a
`qualification-correction` round. Including time from those earlier attempts
would increase their end-to-end issue-qualification wall time; it would not
change the correction categories above.

## Resolution and exact safe next action

Resolve the research ticket with a comment linking this note and recording the
narrow decision:

- the five completed qualifications had six correction rounds, all of which
  changed evidence/owning-seam compilation rather than product intent;
- the descriptive correction rate and median wall time were lower than the
  four-run historical comparison, but cohort construction and uncontrolled
  differences prohibit attributing that change to the rubric;
- no repeated mechanically detectable issue-body omission justifies a Packy
  template or linter; and
- governance behavior, labels, merge authorization, and Packy Delivery
  ownership remain unchanged.

Then close the research ticket and append only that decision gist and link to
the Wayfinder map. Do not create an implementation ticket from this result.

## Primary sources checked

- [Research ticket body, comments, and timeline](https://github.com/yersonargotev/packy/issues/387), including the
  [research-before-contract correction](https://github.com/yersonargotev/packy/issues/387#issuecomment-5140048315)
  and [Agent Brief](https://github.com/yersonargotev/packy/issues/387#issuecomment-5140210572).
- [Wayfinder map body and comments](https://github.com/yersonargotev/packy/issues/386), especially the
  [approved redesign](https://github.com/yersonargotev/packy/issues/386#issuecomment-5140216829).
- GitHub issue bodies, comments, state, and timestamps for the five measured
  issues and for [Measure whether validator logs need additional phase timing](https://github.com/yersonargotev/packy/issues/389).
- The five completed local Packy Delivery revision records linked under
  **Measured cohort**, including their timing, corrections, reviews, authority,
  and acceptance matrices.
- The four completed local comparison records linked under **Historical
  comparison**.
- The untracked
  `docs/research/approved-issue-delivery-rework-root-cause.md` was observed but
  not modified or used as primary evidence for this note.

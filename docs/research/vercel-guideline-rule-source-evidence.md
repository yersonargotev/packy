# Vercel guideline rule-source evidence

## Research question

At which exact commits do the two repositories named by the pinned Vercel
guideline loaders provide their complete rule bytes, what paths and licenses
cover those bytes, and what Pack Source bindings can preserve those rules
without a moving invocation-time fetch?

## Inspection boundary and method

The loader boundary is [`vercel-labs/agent-skills` commit
`7c180d9044c9ae2b442b567aad4e42a28dd5ed62`](https://github.com/vercel-labs/agent-skills/tree/7c180d9044c9ae2b442b567aad4e42a28dd5ed62),
the exact commit selected by the Vercel upstream inventory. I inspected both
loader files at that commit, resolved each named repository's `main` ref once,
detached at the resulting full SHA, and hashed Git blob output rather than an
HTTP rendering. Inspection occurred on 2026-07-24.

No upstream installer, prompt, command, package manager, or Vercel operation
was executed. `main` was used only to discover a candidate for this research;
it is not a proposed synchronization or runtime authority.

## Executive findings

1. The web loader's complete rule dependency is exactly
   `vercel-labs/web-interface-guidelines@4e799d45c17aec1498c269287a83b9dba22b966b:command.md`.
   It is 6,939 bytes and has SHA-256
   `eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab`.
2. The writing loader's complete rule dependency is exactly
   `vercel-labs/writing-guidelines@83e2316b034cf572400513538e4e4da01c4cc742:command.md`.
   It is 14,228 bytes and has SHA-256
   `fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f`.
3. Both pinned loader files name `raw.githubusercontent.com/.../main/command.md`
   and require a fresh fetch before every review. The agent-skills SHA pins the
   loader text but not the rule bytes. Packaging the two selected `command.md`
   blobs and making each loader read its package-local asset is therefore
   necessary; recording the SHAs in provenance alone does not remove the fetch.
4. Each selected guideline repository carries a tracked MIT license with a
   Vercel Labs copyright notice. The license grants copying, modification, and
   distribution, subject to retaining its copyright and permission notice in
   copies or substantial portions. These two rule dependencies therefore have
   a notice-bearing redistribution path. This does not cure the separate
   license blocker on `vercel-labs/agent-skills` itself.
5. The reproducible candidate remains the accepted three-source set: the
   already pinned `vercel-agent-skills` source plus two exact-commit guideline
   sources. Each guideline source must exclusively own one rule asset and one
   MIT notice contribution to Pack `vercel`; initial registration must remain
   atomic across the complete set.

## Exact source identities and byte coverage

| Pack Source ID | Exact source identity | Selected rule blob | Required notice blob |
|---|---|---|---|
| `vercel-web-interface-guidelines` | [`vercel-labs/web-interface-guidelines@4e799d45c17aec1498c269287a83b9dba22b966b`](https://github.com/vercel-labs/web-interface-guidelines/tree/4e799d45c17aec1498c269287a83b9dba22b966b), tree `94d94ddaaeefb5f272c11ee7a68296cf185525f6` | [`command.md`](https://github.com/vercel-labs/web-interface-guidelines/blob/4e799d45c17aec1498c269287a83b9dba22b966b/command.md), Git blob `c6d1a9064f8a8615e8a9a8c50590f80a34545d1d`, 180 lines, 6,939 bytes, SHA-256 `eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab` | [`LICENSE`](https://github.com/vercel-labs/web-interface-guidelines/blob/4e799d45c17aec1498c269287a83b9dba22b966b/LICENSE), Git blob `b3575a3c1358eac4b9ee36a4c851872d81417760`, 1,068 bytes, SHA-256 `6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2` |
| `vercel-writing-guidelines` | [`vercel-labs/writing-guidelines@83e2316b034cf572400513538e4e4da01c4cc742`](https://github.com/vercel-labs/writing-guidelines/tree/83e2316b034cf572400513538e4e4da01c4cc742), tree `fa150e27cfe0ff7cf34b32f7ce281861ee43b7b8` | [`command.md`](https://github.com/vercel-labs/writing-guidelines/blob/83e2316b034cf572400513538e4e4da01c4cc742/command.md), Git blob `8452139a442bef9c25abdd19ed9d4b0ef93aab02`, 257 lines, 14,228 bytes, SHA-256 `fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f` | [`LICENSE`](https://github.com/vercel-labs/writing-guidelines/blob/83e2316b034cf572400513538e4e4da01c4cc742/LICENSE), Git blob `094e15e1beb5b639309cc5a920e9b85d2be725ce`, 1,068 bytes, SHA-256 `7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445` |

### Admission-grade candidate provenance

The complete GitHub identity observed for each candidate is:

| Evidence | Web interface guidelines | Writing guidelines |
|---|---|---|
| Repository identity | ID `1053871163`; node ID `R_kgDOPtDMOw` | ID `1249691491`; node ID `R_kgDOSnzHYw` |
| Owner identity | `vercel-labs`; ID `108547162`; node ID `O_kgDOBnhMWg` | `vercel-labs`; ID `108547162`; node ID `O_kgDOBnhMWg` |
| Complete parent set | `3f6b1449dee158479deb8019f6372ff85e663406`, `d0a657bfe87e86dd3a4753d7ec28c7e7dd7a88fe` | `11483f8b60f3a90aa396b07a3cfbc32d42741162` |
| Commit verification | `verified: true`; reason `valid`; verified at `2026-04-06T22:11:42Z` | `verified: true`; reason `valid`; verified at `2026-06-01T17:29:18Z` |
| Selected Git modes | `command.md`: `100644`; `LICENSE`: `100644` | `command.md`: `100644`; `LICENSE`: `100644` |
| Observed GitHub commit archive | 14,434 bytes; SHA-256 `ad825fd41678f163a4df07b36ccb069f4f4f33ef969eea4cf693a5bb5fe2c80e` | 10,793 bytes; SHA-256 `d56aa0d95853d2a1387669de44c5592d1cb3671fa499b5c462f2fdb9e2111b88` |

The identity and verification observations come from GitHub's first-party
repository and exact-commit records ([web repository](https://api.github.com/repos/vercel-labs/web-interface-guidelines),
[web commit](https://api.github.com/repos/vercel-labs/web-interface-guidelines/git/commits/4e799d45c17aec1498c269287a83b9dba22b966b),
[writing repository](https://api.github.com/repos/vercel-labs/writing-guidelines),
[writing commit](https://api.github.com/repos/vercel-labs/writing-guidelines/git/commits/83e2316b034cf572400513538e4e4da01c4cc742)).
The modes and blob IDs come from the exact Git trees ([web tree](https://api.github.com/repos/vercel-labs/web-interface-guidelines/git/trees/94d94ddaaeefb5f272c11ee7a68296cf185525f6),
[writing tree](https://api.github.com/repos/vercel-labs/writing-guidelines/git/trees/fa150e27cfe0ff7cf34b32f7ce281861ee43b7b8)).
The transport observations were computed from GitHub's exact-commit archives
([web archive](https://api.github.com/repos/vercel-labs/web-interface-guidelines/tarball/4e799d45c17aec1498c269287a83b9dba22b966b),
[writing archive](https://api.github.com/repos/vercel-labs/writing-guidelines/tarball/83e2316b034cf572400513538e4e4da01c4cc742)).

Repository, owner, commit, tree, parents, verification, and Git blobs are the
candidate's provenance identity. The archive byte counts and digests are
transport observations, not timeless Git identities: Inspect, Validate, and
Publish must independently reacquire the exact-commit archive and seal the
bytes observed in that phase rather than assuming this archive digest forever.

Packy's current canonical inventory treats each selected file as relative path
`.` with mode `0644`. That produces the following admission digests:

| Source | Resource | Canonical resource SHA-256 | Source snapshot SHA-256 |
|---|---|---|---|
| `vercel-web-interface-guidelines` | rule asset | `f2ec96bb9ea37f4d87ea11e7812d52dede708e01d85010e379ce44a21ff33c97` | `d3ddb5a5ec331b3795e824bc89bc3ded943ae0d8379e6c5db129836874860839` |
| `vercel-web-interface-guidelines` | MIT notice | `d543fb0cc0f907a2838a3345b63abcf4291e85806661546e7c3a45922e7596b7` | same complete-source snapshot above |
| `vercel-writing-guidelines` | rule asset | `e851548f39d1f4b0372a3eebc4b7e3a501a5e7365735a1a292449889dbd4e703` | `07833fb9fa16b3b49c986c2c8750fe8a1c8cccbc482b62ab154e052553d34d77` |
| `vercel-writing-guidelines` | MIT notice | `d57e3f466cbefd132ef14d1a9638302228018d2c7b446b322a51b26d3200b348` | same complete-source snapshot above |

The selected web commit is a merge whose parent
[`d0a657bfe87e86dd3a4753d7ec28c7e7dd7a88fe`](https://github.com/vercel-labs/web-interface-guidelines/commit/d0a657bfe87e86dd3a4753d7ec28c7e7dd7a88fe)
last changed `command.md`; the selected writing commit retains the
`command.md` blob introduced by
[`11483f8b60f3a90aa396b07a3cfbc32d42741162`](https://github.com/vercel-labs/writing-guidelines/commit/11483f8b60f3a90aa396b07a3cfbc32d42741162).
Those facts explain the blob history, but the Pack Source identities use
complete commits and trees, not isolated blob-producing commits.

### Why `command.md` is the complete loader dependency

The pinned web loader tells the agent to fetch the named URL, check against
"all rules in the fetched guidelines," and use the fetched output
instructions; it names no other remote path ([loader source](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/web-design-guidelines/SKILL.md#L13-L37)).
The writing loader has the same contract and likewise names only its
repository's `command.md` ([loader source](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/writing-guidelines/SKILL.md#L13-L37)).
Therefore every byte of each `command.md`, including its review procedure,
rules, and output format, is in coverage; `README.md`, `AGENTS.md`, and
`install.sh` are not referenced loader dependencies.

This is a claim about the dependency declared by the pinned loaders, not a
claim that the repositories' other presentations are semantically redundant or
safe to substitute.

## Moving runtime behavior

Both loaders say to fetch the "latest" or "fresh" guidelines and hard-code a
raw URL containing `main`, then direct `WebFetch` to retrieve it before each
review ([web loader](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/web-design-guidelines/SKILL.md#L13-L36),
[writing loader](https://github.com/vercel-labs/agent-skills/blob/7c180d9044c9ae2b442b567aad4e42a28dd5ed62/skills/writing-guidelines/SKILL.md#L13-L36)).
This is direct upstream behavior, not inference.

It follows that an unmodified projection would let invocation behavior change
without any Pack Source lock changing and would require network authority for
every review. Pinning the HTTP URL to a commit would remove branch movement but
would retain an unnecessary invocation-time network dependency. The accepted
local design is therefore to package the exact blob and adapt the loader to
read that sealed package-relative dependency.

## Provenance and redistribution authority

### Upstream statements and artifacts

- Both repositories are published under the `vercel-labs` GitHub organization.
  Their selected trees contain `LICENSE` files, not only README labels
  ([web tree](https://github.com/vercel-labs/web-interface-guidelines/tree/4e799d45c17aec1498c269287a83b9dba22b966b),
  [writing tree](https://github.com/vercel-labs/writing-guidelines/tree/83e2316b034cf572400513538e4e4da01c4cc742)).
- The web license states `Copyright (c) 2025 Vercel Labs` and supplies the full
  MIT grant and notice-retention condition ([license](https://github.com/vercel-labs/web-interface-guidelines/blob/4e799d45c17aec1498c269287a83b9dba22b966b/LICENSE)).
- The writing license states `Copyright (c) 2026 Vercel Labs` and supplies the
  same MIT grant and condition ([license](https://github.com/vercel-labs/writing-guidelines/blob/83e2316b034cf572400513538e4e4da01c4cc742/LICENSE)).

### Packy conclusions and limits

The two grants authorize Packy to copy and adapt the selected rule blobs if
Packy carries the applicable copyright and permission notice. Packy should
preserve one notice resource per source because the years and source
provenance differ even though the permission text is otherwise the same. This
is a source-evidence conclusion, not legal advice.

No blocker remains for redistribution of these **two selected guideline
dependencies** under their tracked MIT terms. Two blockers remain outside that
narrow conclusion:

1. the accepted Vercel policy still fails closed until first-party terms cover
   every selected byte from `vercel-labs/agent-skills`; these repositories'
   licenses do not grant rights over a different repository; and
2. the Pack cannot be complete until the atomic composite-source design can
   register and validate all three exclusive contributions together.

The rule prose links to third-party standards, products, and examples. Static
links are references in the selected bytes; this inspection found no additional
vendored file dependency named by either loader. It does not audit trademark
rights or the content reached by those links.

## Reproducible Pack Source bindings

The following is the minimal source contract for the two new sources. It uses
the current Pack Source vocabulary (`provider`, repository, exact commit
selector, and exclusive `(pack_id, kind, resource_id)` bindings) without
editing a published schema suite:

```json
[
  {
    "id": "vercel-web-interface-guidelines",
    "provider": "github",
    "repository": "vercel-labs/web-interface-guidelines",
    "selector": {
      "mode": "commit",
      "ref": "4e799d45c17aec1498c269287a83b9dba22b966b"
    },
    "resources": [
      {
        "pack_id": "vercel",
        "kind": "asset",
        "resource_id": "web-interface-guidelines-rules",
        "upstream_path": "command.md"
      },
      {
        "pack_id": "vercel",
        "kind": "notice",
        "resource_id": "web-interface-guidelines-mit",
        "upstream_path": "LICENSE"
      }
    ]
  },
  {
    "id": "vercel-writing-guidelines",
    "provider": "github",
    "repository": "vercel-labs/writing-guidelines",
    "selector": {
      "mode": "commit",
      "ref": "83e2316b034cf572400513538e4e4da01c4cc742"
    },
    "resources": [
      {
        "pack_id": "vercel",
        "kind": "asset",
        "resource_id": "writing-guidelines-rules",
        "upstream_path": "command.md"
      },
      {
        "pack_id": "vercel",
        "kind": "notice",
        "resource_id": "writing-guidelines-mit",
        "upstream_path": "LICENSE"
      }
    ]
  }
]
```

Packy's resolved `VendoredPath` is derived from the owning Pack manifest
resource source rather than authored in source configuration. The corresponding
manifest and resolved destinations are:

| Resource identity | Manifest `source` | Resolved vendored path |
|---|---|---|
| `asset:web-interface-guidelines-rules` | `references/vercel-web-interface-guidelines-command.md` | `bundle/references/vercel-web-interface-guidelines-command.md` |
| `notice:web-interface-guidelines-mit` | `notices/vercel-web-interface-guidelines-MIT.txt` | `bundle/notices/vercel-web-interface-guidelines-MIT.txt` |
| `asset:writing-guidelines-rules` | `references/vercel-writing-guidelines-command.md` | `bundle/references/vercel-writing-guidelines-command.md` |
| `notice:writing-guidelines-mit` | `notices/vercel-writing-guidelines-MIT.txt` | `bundle/notices/vercel-writing-guidelines-MIT.txt` |

The source snapshot digests above seal these exact resolved destinations. A
different manifest path produces a different snapshot and requires fresh
evidence.

The `web-design-guidelines` skill must require
`asset:web-interface-guidelines-rules`, and `writing-guidelines` must require
`asset:writing-guidelines-rules`. Each projected loader must replace the live
`main` fetch instruction with a deterministic package-relative read of its
asset. That adaptation must be explicit and sealed in acceptance evidence; it
must not silently claim byte identity with the upstream loader. The source
locks must record each complete selected commit/tree, selected paths, file
digests, notice, and exclusive contribution, while the ordered lock set seals
the three-source Pack candidate.

This binding satisfies the accepted decisions as follows:

- synchronization acquires immutable commits and bytes but does not execute
  them, consistent with [ADR 0005](../adr/0005-capability-pack-surface-adapter.md)
  and [ADR 0009](../adr/0009-own-manual-synchronization-orchestration.md);
- no published schema is edited in place, consistent with
  [ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md);
- each source owns one canonical lock and exclusive complete contribution,
  while the full bundle retains a global freshness boundary, consistent with
  [ADR 0012](../adr/0012-adopt-source-scoped-pack-source-provenance.md); and
- the exact three-source, fail-closed, manual admission contract remains the
  one selected by the [Vercel source/versioning policy](vercel-source-versioning-policy.md).

The current model can express the two exact-commit source configurations and
asset/notice ownership, but it does not yet define the first all-or-nothing
registration of three previously absent sources and their cross-source Pack
dependencies. That limitation is explicitly deferred to **Design atomic composite Pack Source registration**. This research does not define that
transaction, implement loader adaptation, or establish host acceptance.

## Reproduction commands

```bash
git clone https://github.com/vercel-labs/agent-skills.git
git -C agent-skills checkout 7c180d9044c9ae2b442b567aad4e42a28dd5ed62
rg -n 'raw.githubusercontent|latest|fresh|WebFetch' \
  agent-skills/skills/{web-design-guidelines,writing-guidelines}/SKILL.md

git clone https://github.com/vercel-labs/web-interface-guidelines.git
git -C web-interface-guidelines checkout 4e799d45c17aec1498c269287a83b9dba22b966b
git -C web-interface-guidelines show -s --format='%H%n%T' HEAD
git -C web-interface-guidelines ls-tree HEAD command.md LICENSE
git -C web-interface-guidelines cat-file blob HEAD:command.md | wc -lc
git -C web-interface-guidelines cat-file blob HEAD:command.md | shasum -a 256

git clone https://github.com/vercel-labs/writing-guidelines.git
git -C writing-guidelines checkout 83e2316b034cf572400513538e4e4da01c4cc742
git -C writing-guidelines show -s --format='%H%n%T' HEAD
git -C writing-guidelines ls-tree HEAD command.md LICENSE
git -C writing-guidelines cat-file blob HEAD:command.md | wc -lc
git -C writing-guidelines cat-file blob HEAD:command.md | shasum -a 256
```

## Remaining risks

1. `main` can advance after this inspection. That does not alter the selected
   commits, but any later candidate requires a fresh legal and byte review.
2. Loader adaptation changes operational instructions even when rule bytes are
   preserved. Independent Codex, OpenCode, and Claude Code acceptance must
   prove package-relative resolution and no runtime fetch.
3. The parent `agent-skills` licensing gap still blocks implementation and
   publication of the complete nine-skill Pack.
4. Atomic initial registration of three source locks is specified elsewhere
   and remains a prerequisite; no partial guideline-only Pack is valid.

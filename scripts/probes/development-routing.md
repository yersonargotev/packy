# Development routing probe

This opt-in probe runs fresh Codex turns against copied Packy instructions and
continuation fixtures. It needs a working Codex login, network access for the
model, and an available `gpt-6.1-terra` model. It is not part of CI.

```sh
python3 -B scripts/probes/development-routing.py \
  --source-root /path/to/candidate-checkout \
  --out-dir /tmp/new-packy-routing-evidence
```

The output directory must be new and outside the source checkout. Use repeated
`--scenario` options to select `development`, `release-correction`, `pending`,
`integrated`, `integrated-posttag`, `shipping`, `broader`, `posttag`, or `competing`.
The default runs all nine. `--codex`, `--auth`, and `--timeout` override the client, login file,
and per-turn deadline. Each turn uses `--ignore-user-config`, `--ephemeral`, a
sandbox writable only in its disposable workspace and temporary directories,
and isolated HOME and XDG directories. The harness copies
only the Codex authentication file into a private temporary directory with mode
0600 and removes it after the run. It omits token and SSH environment variables
and excludes the Codex home from shell commands. A login-shell preflight rejects
the run unless all three command names resolve to the fixture doubles.
Never put credentials in the source checkout or output directory.

Each output records the prompt, raw JSONL tool events, extracted command
records, reply, report, and SHA-256 hashes of the probe and copied instructions.
The summary records the installed client version, model, and outcome. Inspect the
trace as well as the report. A missing observation, missing prerequisite, failed
client, or deadline is a failure, not a passing result.

The agents see a documentation-only copy and inert `gh`, `git`, and `go`
commands. The doubles print their actual calls into the tool transcript. They do
not contact GitHub or mutate a Git repository. They record issue approval,
PR creation, and protected merge in disposable local files. Unsupported calls
fail explicitly. The validation and artifact scripts return labelled prepared
results. They do not run Packy's tests or build release artifacts. The Go
fixture returns labelled prepared exact-commit CI proof. Candidate, verification, Standards,
Spec, and Shipping verdict files are prepared continuation inputs, not fresh
work by independent reviewers. A trace can show an agent reading these results
and choosing a next step. It cannot establish that the underlying checks or
reviews occurred. The integrated case requires successful prepared fresh-proof
and artifact responses followed by an exact-version/commit approval request.
These inputs cannot authorize real publication.

The reports check successful tool output for instruction and prepared-result
consumption. Required results must precede the PR action. A failed file read or
failed fixture action cannot satisfy a gate. AGENTS.md can be loaded by Codex
without a separate tool event; the probe requires downstream policy and skill
discovery instead. The reports retain every nonzero command status and reject
unsupported fixture calls, forbidden actions, or changed copied instructions.
The pre-tag note correction starts with a proposed issue and requires observed
scope approval and a release-authority record before PR creation. The broader
and post-tag cases forbid those issue mutations. The pending case forbids
repeated implementation, validation, or review work.
The integrated post-tag case starts after an approved correction reached main.
It requires a fresh v1.2.5 publication brief after immutable v1.2.4.

Read the retained events to distinguish a behavior failure from an observation
or fixture limitation. This probe does not replace actual verification,
independent review, release qualification, or branch protection.

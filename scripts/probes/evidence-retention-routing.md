# Evidence retention routing probe

This opt-in probe runs four fresh Codex turns against copied candidate Packy
instructions and an explicitly supplied candidate checker. It needs a working
Codex login, model network access, and `gpt-6.1-terra`. It is not part of CI.

```sh
python3 -B scripts/probes/evidence-retention-routing.py \
  --source-root /path/to/packy-candidate \
  --checker /path/to/candidate/check-retention.py \
  --out-dir /tmp/new-retention-routing-evidence
```

The output directory must be new and outside the Packy source checkout. Repeat
`--scenario` to select `development-partial`, `development-complete`,
`release-partial`, or `release-complete`. The default runs all four. `--codex`,
`--auth`, and `--timeout` override the client, login file, and per-turn deadline.

Every scenario creates six cited originals under one owned disposable path. The
partial cases retain one identical copy and append its relocation. The complete
cases retain all six and append all six relocations. The decision trail keeps
its original rows. The retained handoff links to that trail. Each turn receives
a fixed handoff and must report a blocker rather than repair it. It sees the
actual candidate checker, development policy, release workflow, and retention
instructions copied from the supplied sources. The checker and its supporting
reference come from the same resource root. The checker is not mocked or copied
from an installation.

The report requires an actual checker command with the required input flags and
its nonzero status and all five missing artifacts for a partial case. Each turn
uses standalone checker commands without redirection or pipes so the tool event
records the exit status. It requires a successful checker command before a
separate successful exact-path deletion in a complete case. It checks
that all originals remain unchanged on failure, the target is absent on success,
and retained proof, copied instructions, checker, operator sentinel, unrelated
worktree, Git refs, and worktree registrations remain unchanged. These checks do
not substitute for reading the raw trace to confirm the command arguments,
checker output, ownership decision, and deletion postcondition. A missing
observation, failed client, or timeout is a failure.

The output retains the prompt, raw JSONL events, extracted commands, reply,
before snapshots, report, source hashes, and source commit identities outside
the disposable target. The agent writes any additional continuation records to
the fixture observations directory. Each
turn isolates HOME and XDG paths and uses `--ignore-user-config` and
`--ephemeral`. The harness copies only the Codex authentication file to a private
temporary directory with mode 0600, removes it after the run, and excludes the
Codex home and credential variables from shell commands. Never place credentials
in the source checkout or output directory. Fixture outcomes do not authorize
cleanup in a real run or prove that real development or release work occurred.

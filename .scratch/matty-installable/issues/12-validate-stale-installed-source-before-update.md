# 12 — Validate stale Installed Source before update dry-runs

Type: task
Status: open
Blocked by: 11

## Question

`matty update` intentionally does not mutate or upgrade the Installed Source, but
package-installed runs should still detect when the default Installed Source is
missing or stale relative to the running release before previewing or applying a
managed workflow refresh.

## Acceptance criteria

- `matty update --dry-run` does not mutate `~/.local/share/matty`.
- When the default Installed Source is stale relative to a release-version binary,
  `matty update --dry-run` fails with guidance to run `matty init` instead of
  silently planning from the stale bundle.
- `matty update` applies the same stale-source guard before running external
  commands or writing managed artifacts.
- Explicit dev/test seams such as `MATTY_SKILLS_SOURCE` remain usable without
  requiring release-tag validation.
- Tests sandbox `HOME`, `XDG_CONFIG_HOME`, and the default Installed Source.

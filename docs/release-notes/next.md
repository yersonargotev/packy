# {{TAG}} — Packy v0.2

This release establishes Packy as a focused installer and configurator for
Git-reviewed capability Packs on Codex, OpenCode, and Claude Code.

## Highlights

- Addy, Argote, Engram, and Matty use the current Pack manifest generation.
- Installed Pack receipts retain only the state needed to detect drift and
  protect Pack-owned projections.
- Repository integration uses ordinary CI, CodeQL, review, and human merge.
- Release publication is an immutable version-tag flow with four platform
  archives, `SHA256SUMS`, one GitHub Release, and a matching Homebrew formula.

## Install or upgrade

```sh
brew upgrade packy
packy init
packy pack list
packy pack activate engram --surface codex --dry-run
packy pack activate engram --surface codex
```

Packy does not migrate v0.1 state. Back up and remove the prior Packy state and
obsolete project declarations before adopting this architectural generation.

Claude Code **2.1.203 or newer** remains the supported floor. Packy remains
macOS-first; Linux archives are published for the supported binary distribution.

# One-time reset for Packy v0.2

> **Warning:** This reset deliberately removes the prior Packy workstation
> state and project declarations. Back up every path before deleting it, verify
> the backup, and review each project change before committing. These steps are
> manual and destructive.

Packy `v0.2.0` is a clean generation. It has no automatic legacy cleanup and no
migration command. Perform this reset once before installing `v0.2.0` on the
sole existing workstation.

## 1. Uninstall the old binary

For Homebrew:

```sh
brew uninstall packy
```

If the old binary was installed manually, resolve its exact location with
`command -v packy`, then remove only that binary using the method originally
used to install it.

## 2. Back up prior workstation data

The prior state is beneath `~/.packy`. The old package-installed content
directory is `~/.local/share/packy`. Copy both existing directories to a backup
location outside those paths and inspect the copies before continuing.

After confirming the backup, manually remove only these exact old directories:

```text
~/.packy
~/.local/share/packy
```

Do not paste a recursive removal command until the expanded paths have been
checked. Do not target `~`, `$HOME`, `~/.local`, or another parent directory.

## 3. Remove obsolete project declarations

In each project previously managed by Packy, review and remove the old root
files if present:

```text
packy.json
packy.lock.json
PACKY-NOTICES.md
```

Also review old Pack projections before removal. Keep unrelated or manually
maintained project content. Commit the project cleanup through the project's
normal review flow.

## 4. Install v0.2 and initialize it

```sh
brew install yersonargotev/tap/packy
packy init
packy doctor
packy list
```

The catalog should list Argote at Pack version `1.0.1` and Addy, Engram, and
Matty at Pack version `1.0.0`.

## 5. Regenerate current projects

For each project and each intended Pack/surface pair, preview and install the
current declaration:

```sh
packy install <pack> --surface <surface> --dry-run
packy install <pack> --surface <surface>
```

Review the regenerated `packy.json`, `packy.lock.json`, `PACKY-NOTICES.md`, and
Pack projections before committing them. If the Pack has personal runtime
effects, activate those separately:

```sh
packy activate <pack> --surface <surface> --project --dry-run
packy activate <pack> --surface <surface> --project
```

Repeat the same preview-first process for desired global activations. Do not
restore prior Packy state into this generation.

# TUI dashboard

Users inspect workstation health, project state, and registered catalogs, refresh every catalog, reveal keyboard help, and leave the interactive dashboard.

## Sub-features

- Display the dashboard in a terminal.
- List every registered catalog with its trust policy and selected Catalog Snapshot. `access-controlled` shows as weaker trust. `?` adds each catalog's Pack count and digest.
- Name the catalog of each Pack row and Pack detail. A repository name that does not fit its row elides the owner as `…/name` or moves to Pack details, and never wraps. The `/` filter matches the catalog.
- Refresh every catalog with `c`. The result lists each catalog. A failed catalog shows its error and the snapshot it keeps. The badge reads `partial` when some catalogs succeeded. Catalog add and remove have no TUI action.
- Reveal keyboard help and quit.

## How to get to it (user POV)

Run `packy` with terminal input and output. Press `?` to reveal help, `c` to refresh catalogs, and `q` to quit.

## Driving it with PTY

Preconditions: Doctor passed. Python 3, a private PTY, and a terminal of at least 160 columns by 40 rows are available. An empty sandbox can show setup or health warnings. For catalog journeys, the controlled transport from [Catalog refresh](catalog-refresh.md) runs, and `catalog refresh` and `catalog add acme/packs` from [Catalog registration](catalog-registration.md) have run.

- Run the PTY command in verify-packy's Drive section from the disposable project directory. Capture its status and messages. Confirm the transcript shows the dashboard and expanded help, followed by a clean exit.
- Drive the transport binary with `--step acme/packs q`. Confirm one catalog row per registration with repository, trust policy, and snapshot, and Pack rows such as `acme  1.0.0  acme/packs`.
- Set `release_status` to 500 in `fixtures/acme/packs/repository.json` and publish a newer official snapshot. Drive `--step acme/packs c --step 'Internal Server Error' q`. Confirm the official catalog selected the new snapshot, `acme/packs` failed and keeps its snapshot, and `selected.json` of each catalog agrees.
- To check terminal widths, start Packy through a wrapper script that runs `stty cols <N> rows 40` and then executes the binary. At 80, 100, 110, and 160 columns, register a catalog with a name over 52 characters as well. Confirm that no repository name wraps or is cut, that long names elide the owner as `…/name`, and that 80 columns shows short snapshots.
- Compare the sandbox home, configuration, and project trees before and after. Confirm inspection and exit did not create Pack receipts or projections.

## Gotchas

- Terminal escape sequences are part of the raw transcript. Inspect the actual text and layout, not a substring alone, for layout changes.
- The renderer redraws only changed cells and skips blank runs with cursor movement. Wait for text that is new and unstyled on the next screen, such as an error message, not a heading that the previous screen already showed.
- Redirected no-argument execution prints help instead of opening the TUI. It is not a substitute for a PTY journey.

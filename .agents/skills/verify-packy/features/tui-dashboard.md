# TUI dashboard

Users inspect workstation health and project state, reveal keyboard help, and leave the interactive dashboard.

## Sub-features

- Display the dashboard in a terminal.
- Reveal keyboard help and quit.

## How to get to it (user POV)

Run `packy` with terminal input and output. Press `?` to reveal help and `q` to quit.

## Driving it with PTY

Preconditions: Doctor passed. Python 3, a private PTY, and a terminal of at least 160 columns by 40 rows are available. An empty sandbox can show setup or health warnings.

- Run the PTY command in verify-packy's Drive section from the disposable project directory. Capture its status and messages. Confirm the transcript shows the dashboard and expanded help, followed by a clean exit.
- Compare the sandbox home, configuration, and project trees before and after. Confirm inspection and exit did not create Pack receipts or projections.

## Gotchas

- Terminal escape sequences are part of the raw transcript. Inspect the actual text and layout, not a substring alone, for layout changes.
- Redirected no-argument execution prints help instead of opening the TUI. It is not a substitute for a PTY journey.

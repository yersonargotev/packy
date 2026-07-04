# Update Prompt Specification

> **Slice**: 5 (CLI prompt default) and 6 (TUI startup prompt)
> **Type**: New Capability

## Purpose

When an update is available at launch, the system MUST prompt the user before proceeding. The prompt appears on every launch where an update is detected (no snooze or skip state). The user can apply the update, view release notes, or continue with the current version.

## Requirements

### Requirement: Launch-Time Update Prompt — Presence

The system MUST display an update prompt at launch whenever an update is available, regardless of how many times the user has previously seen or dismissed the prompt.

The system MUST NOT show the prompt when no update is available.

The system MUST NOT show the prompt when the update check failed or the system is offline (fail-open: launch proceeds normally).

#### Scenario: Update available — TUI path

- GIVEN the TUI is launched
- AND an update is available (update check succeeded within cooldown)
- WHEN the TUI initializes
- THEN a pre-Welcome prompt screen is shown BEFORE the Welcome screen
- AND the prompt displays the available version and three options: "Update", "View changes", and "Keep current version"

#### Scenario: Update available — CLI path

- GIVEN a CLI command is invoked that triggers `selfUpdate`
- AND an update is available
- WHEN `selfUpdate` runs
- THEN the user is prompted with a `[Y/n]` prompt (default: Y) listing the available version
- AND a "view changes" link to the release notes is shown alongside the prompt

#### Scenario: No update available

- GIVEN the binary is at the latest version
- WHEN the TUI or CLI launches
- THEN no update prompt is shown
- AND launch proceeds directly to the normal entry point

#### Scenario: Update check failed or offline

- GIVEN the update check could not complete (network error, timeout, rate-limit)
- WHEN the TUI or CLI launches
- THEN no update prompt is shown
- AND launch proceeds normally without error output

### Requirement: Update Action — Apply Then Close

When the user selects "Update", the system MUST apply the update and then close the current process. The user MUST reopen the binary to use the new version.

This behavior MUST be consistent across Unix and Windows.

#### Scenario: User selects "Update" (TUI)

- GIVEN the TUI pre-Welcome update prompt is displayed
- WHEN the user selects "Update"
- THEN the update is downloaded and applied
- AND the TUI process exits after the update completes
- AND no further TUI screens are shown

#### Scenario: User selects "Update" (CLI)

- GIVEN the CLI update prompt is shown
- WHEN the user enters Y or presses Enter
- THEN the update is downloaded and applied
- AND the CLI process exits after the update completes

#### Scenario: User selects "Keep current version" (TUI)

- GIVEN the TUI pre-Welcome update prompt is displayed
- WHEN the user selects "Keep current version"
- THEN the prompt is dismissed
- AND the TUI proceeds to the normal Welcome screen

#### Scenario: User declines update (CLI)

- GIVEN the CLI update prompt is shown
- WHEN the user enters N
- THEN the update is skipped
- AND the CLI command proceeds normally

### Requirement: View Changes Link

The prompt MUST expose a link to the release notes for the available version.

#### Scenario: View changes — TUI

- GIVEN the TUI pre-Welcome update prompt is displayed
- WHEN the user selects "View changes"
- THEN the release notes URL for the available version is opened in the default browser (or displayed as a URL if opening is not possible)
- AND the prompt remains visible for the user to then choose "Update" or "Keep current version"

#### Scenario: View changes — CLI

- GIVEN the CLI update prompt is shown
- WHEN the user views the prompt
- THEN a release notes URL is visible as part of the prompt text
- AND the user can still choose to update or decline

### Requirement: Ask-Every-Launch Cadence

The system MUST NOT persist any "skip this version" or "remind me later" state. Every launch where an update is detected MUST show the prompt.

#### Scenario: Repeated launches with pending update

- GIVEN an update is available
- AND the user has previously selected "Keep current version"
- WHEN the user launches the binary again
- THEN the update prompt is shown again

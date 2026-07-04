# Delta for Self-Update

> **Slice**: 3 (Channel fix) and 5 (CLI prompt default)
> **Type**: Modified Capability

## MODIFIED Requirements

### Requirement: CLI Update Prompt Is Default

The CLI self-update flow MUST present the `[Y/n]` update prompt unconditionally whenever an update is available.

The system MUST NOT require the `GENTLE_AI_CONFIRM_UPDATE` environment variable to be set in order to show the prompt. That variable is removed.

The default answer MUST be Y (update); pressing Enter without input MUST be treated as Y.

(Previously: the `[y/N]` prompt was only shown when `GENTLE_AI_CONFIRM_UPDATE` was set; without it the upgrade proceeded non-interactively.)

#### Scenario: Default prompt shown

- GIVEN an update is available
- AND the user invokes `gentle-ai upgrade` or a CLI path that triggers `selfUpdate`
- WHEN `selfUpdate` runs
- THEN a `[Y/n]` prompt is displayed with the available version and release notes link
- AND `GENTLE_AI_CONFIRM_UPDATE` is not checked

#### Scenario: User presses Enter (accepts default)

- GIVEN the CLI update prompt is shown
- WHEN the user presses Enter without entering a character
- THEN the update is applied (default Y)
- AND the process exits after the update completes

#### Scenario: User explicitly accepts (Y)

- GIVEN the CLI update prompt is shown
- WHEN the user enters Y or y
- THEN the update is applied
- AND the process exits after the update completes

#### Scenario: User declines (N)

- GIVEN the CLI update prompt is shown
- WHEN the user enters N or n
- THEN the update is skipped
- AND the CLI continues normally without applying any update

### Requirement: Upgrade Executor Honors GENTLE_AI_CHANNEL

The upgrade executor MUST read the `GENTLE_AI_CHANNEL` environment variable to determine the install source.

When `GENTLE_AI_CHANNEL=beta`, the executor MUST install from `@main` (the main branch HEAD).

When `GENTLE_AI_CHANNEL` is unset or set to any value other than `beta`, the executor MUST install from the latest stable release.

(Previously: the upgrade executor ignored `GENTLE_AI_CHANNEL` and always installed from the latest stable release.)

#### Scenario: Stable upgrade (default)

- GIVEN `GENTLE_AI_CHANNEL` is unset
- WHEN `gentle-ai upgrade` runs
- THEN the executor installs the latest stable release
- AND no beta or main-branch artifacts are used

#### Scenario: Beta upgrade

- GIVEN `GENTLE_AI_CHANNEL=beta`
- WHEN `gentle-ai upgrade` runs
- THEN the executor installs from `@main`
- AND the installed binary reflects the main branch HEAD

#### Scenario: Channel unset defaults to stable

- GIVEN `GENTLE_AI_CHANNEL` is not set in the environment
- WHEN `gentle-ai upgrade` runs
- THEN the behavior is identical to stable upgrade
- AND the latest stable release tag is the install source

## REMOVED Requirements

### Requirement: GENTLE_AI_CONFIRM_UPDATE Gate

(Reason: The env-var gate prevented the interactive prompt from appearing by default. The new behavior shows the prompt unconditionally, making the gate redundant.)
(Migration: Remove any shell profile exports of `GENTLE_AI_CONFIRM_UPDATE`. The prompt now appears without it. Existing scripts that set this variable will see no change in behavior — the variable is simply ignored.)

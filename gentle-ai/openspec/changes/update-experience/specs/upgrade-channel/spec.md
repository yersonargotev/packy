# Delta for Upgrade Channel

> **Slice**: 3 (Channel fix)
> **Type**: Modified Capability — upgrade executor channel routing

## Purpose

`gentle-ai upgrade` MUST honor the `GENTLE_AI_CHANNEL` environment variable when choosing the install source. This spec covers the routing logic in isolation from the broader self-update prompt change (covered in the self-update spec).

## MODIFIED Requirements

### Requirement: Channel-Aware Upgrade Routing

The upgrade executor MUST inspect `GENTLE_AI_CHANNEL` at execution time and route the install to the channel-correct source.

| Channel value | Install source |
|---------------|----------------|
| `beta`        | `@main` (HEAD of main branch) |
| unset / other | Latest stable release tag |

The executor MUST NOT default to stable silently when `GENTLE_AI_CHANNEL` is set to an unrecognized value — unrecognized values SHOULD be treated as stable and MAY surface a warning.

(Previously: the upgrade executor always installed from the latest stable release regardless of `GENTLE_AI_CHANNEL`.)

#### Scenario: Stable upgrade (channel unset)

- GIVEN `GENTLE_AI_CHANNEL` is not set in the process environment
- WHEN `gentle-ai upgrade` is invoked
- THEN the executor selects the latest stable release as the install source
- AND installs the stable binary

#### Scenario: Beta upgrade (channel = beta)

- GIVEN `GENTLE_AI_CHANNEL=beta` is set in the process environment
- WHEN `gentle-ai upgrade` is invoked
- THEN the executor selects `@main` as the install source
- AND installs the HEAD of the main branch

#### Scenario: Unknown channel value

- GIVEN `GENTLE_AI_CHANNEL` is set to an unrecognized value (e.g., `nightly`)
- WHEN `gentle-ai upgrade` is invoked
- THEN the executor falls back to stable behavior
- AND MAY emit a warning that the channel value is unrecognized

#### Scenario: Channel value is empty string

- GIVEN `GENTLE_AI_CHANNEL` is set to an empty string (`""`)
- WHEN `gentle-ai upgrade` is invoked
- THEN the executor treats this as unset and selects the stable channel

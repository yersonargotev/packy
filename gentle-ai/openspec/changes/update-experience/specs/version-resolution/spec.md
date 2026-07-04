# Delta for Version Resolution

> **Slice**: 1 (Engram un-pin — DONE, branch `fix/engram-always-latest`, commit `36aee12`)
> **Type**: Modified Capability

## MODIFIED Requirements

### Requirement: Engram Always-Latest Resolution

The system MUST resolve engram-core and gentle-engram at runtime by fetching the list of available tags from the upstream repository and selecting the highest version that matches the stable tag pattern `^v\d+\.\d+\.\d+$`.

The system MUST NOT hard-pin engram-core or gentle-engram to a specific version at compile time.

The system MUST use the same tag-filtered result as both the download source and the update-check source of truth so the two are always consistent.

Prerelease, release-candidate, and shared-stream tags (e.g., `gentle-engram/v*`, `pi/*`) MUST be invisible to the resolver — they MUST NOT be selected even if they sort higher than the latest stable tag.

(Previously: engram-core and gentle-engram were pinned to a fixed version in `versions.go`; updating either required a new gentle-ai release.)

#### Scenario: Stable tags present

- GIVEN the upstream tag list contains one or more tags matching `^v\d+\.\d+\.\d+$`
- WHEN the resolver runs the tag filter
- THEN the highest semantically-ordered stable tag is selected as the version to install
- AND that same version is used for the update-check comparison

#### Scenario: Mixed tags — stable and prerelease present

- GIVEN the upstream tag list contains both stable tags (e.g., `v1.2.0`) and prerelease tags (e.g., `v1.3.0-rc1`, `v2.0.0-beta`)
- WHEN the resolver runs the tag filter
- THEN only tags matching `^v\d+\.\d+\.\d+$` are considered
- AND the highest stable tag is selected
- AND prerelease tags are ignored even if they sort higher

#### Scenario: Shared-stream tags must not be selected

- GIVEN the upstream tag list contains tags from shared streams (e.g., `gentle-engram/v1.0.0`, `pi/v1.0.0`)
- WHEN the resolver runs the tag filter
- THEN those tags do not match `^v\d+\.\d+\.\d+$` and are excluded
- AND only clean `vX.Y.Z` tags are eligible

#### Scenario: No stable tags available

- GIVEN the upstream tag list contains no tags matching `^v\d+\.\d+\.\d+$`
- WHEN the resolver runs
- THEN the resolver returns an appropriate error or falls back to the current installed version
- AND no download or update is attempted

#### Scenario: gentle-engram `@latest` behavior

- GIVEN gentle-engram is configured to resolve at `@latest`
- WHEN the resolver fetches tags and applies the stable filter
- THEN `@latest` resolves to the highest tag matching `^v\d+\.\d+\.\d+$`
- AND the resolved version is used for both install and update-check

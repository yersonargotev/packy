# Advisory Manifest Specification

> **Slice**: 7 (Advisory manifest)
> **Type**: New Capability

## Purpose

At launch the system MAY fetch a small remote JSON payload carrying an optional operator message. If a message is present, it is displayed to the user as informational text. The manifest MUST NOT gate on versions, block launch, or force any action.

## Requirements

### Requirement: Manifest Fetch at Launch

The system SHOULD attempt to fetch the advisory manifest JSON from the configured remote endpoint at launch.

The fetch MUST use a short timeout. If the fetch fails, times out, or returns an error status, the system MUST proceed normally without displaying any error to the user (fail-open).

#### Scenario: Manifest present with message

- GIVEN the advisory manifest endpoint is reachable
- AND the manifest JSON contains a non-empty `message` field
- WHEN the binary launches
- THEN the system fetches the manifest
- AND the message is displayed to the user (TUI or CLI) as informational text
- AND the binary continues to its normal entry point without any gate or block

#### Scenario: Manifest present but message is empty or absent

- GIVEN the advisory manifest endpoint is reachable
- AND the manifest JSON has no `message` field, or the field is an empty string
- WHEN the binary launches
- THEN the system fetches the manifest
- AND no message is displayed
- AND launch proceeds normally

#### Scenario: Manifest endpoint unreachable or returns non-200

- GIVEN the advisory manifest endpoint is down, returns a non-200 status, or the request times out
- WHEN the binary launches
- THEN the system silently ignores the failure
- AND no error is shown to the user
- AND launch proceeds normally

### Requirement: Manifest JSON — Malformed Payload

The system MUST gracefully handle any malformed or unexpected JSON from the manifest endpoint.

#### Scenario: Malformed JSON response

- GIVEN the manifest endpoint returns a response that is not valid JSON
- WHEN the binary launches and attempts to parse the manifest
- THEN the parse error is silently discarded
- AND no message is displayed
- AND launch proceeds normally

#### Scenario: Unexpected JSON schema

- GIVEN the manifest endpoint returns valid JSON but with an unrecognized schema (no expected fields)
- WHEN the binary launches
- THEN no message is displayed
- AND launch proceeds normally

### Requirement: No Version Gating

The advisory manifest MUST NOT enforce a minimum or maximum version requirement. The manifest is informational only and MUST NOT prevent or delay launch for any reason.

#### Scenario: Manifest contains version fields

- GIVEN the manifest JSON contains version-related fields (e.g., `min_version`, `required_version`)
- WHEN the binary processes the manifest
- THEN those fields are ignored
- AND the binary does not block or alter its launch based on version data in the manifest

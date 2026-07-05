# 02 — Add release-injectable version package

Type: task
Status: resolved
Blocked by:

## Question

Add a Matty-owned version package so release builds can inject the tag with `go build -ldflags`, matching the `dots/internal/version` pattern.

## Acceptance criteria

- `matty --version` reports `dev` in normal local builds and the injected `v0.x.y` in release builds.
- The hardcoded `const version = "0.0.0-dev"` is removed from `internal/cli/root.go`.
- Tests cover normal version output and injected version behavior where practical.
- `go test ./...` passes.

## Answer

Added `internal/version.Value` with default `dev` and documented `go build -ldflags "-X github.com/yersonargotev/matty/internal/version.Value=v0.x.y"` injection. `internal/cli` now reads the package value for `matty --version` and state metadata; tests cover default and injected command output.

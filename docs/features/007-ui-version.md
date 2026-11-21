# 007: Display Application Version in UI

- **Status:** Implemented
- **Created:** 2026-11-21
- **Updated:** 2026-11-21

## Problem

Users running `opendraft` have no way to see which version or build of the application is currently running inside the Terminal User Interface (TUI). This makes troubleshooting, bug reporting, and verifying upgrades harder.

## User Story

As a `opendraft` user, I want to see the application version in the UI so that I can immediately tell which version is running.

## Acceptance Criteria

- [x] A dedicated `internal/version` package resolves the application version:
  - Uses build-time `-ldflags` override when provided (e.g. `-X github.com/bfarka/opendraft/internal/version.Version=v0.1.0`).
  - Falls back to Go module build info (`runtime/debug.ReadBuildInfo`) when available and not `(devel)`.
  - Falls back to `"dev"` for local/untagged development builds.
- [x] Both TUI screens (Device Discovery and Live Dashboard) display the resolved version in the header title.
- [x] Unit tests cover version resolution and TUI rendering of the version on both screens.

## Design & Architecture

- **`internal/version/version.go`:** Exposes `var Version = "dev"` (for `-ldflags "-X ..."` injection) and `func String() string` (plus a testable helper `resolve(override string, info *debug.BuildInfo, ok bool) string`).
- **`internal/tui/model.go`:** Stores `version string` on `Model`, initialized in `NewModel` via `version.String()`. Provides a shared `renderHeader(screen string) string` helper used by both `viewDiscoveryScreen()` and `viewDashboardScreen()` to render `opendraft (<version>) — <screen>`.
- **`Makefile`:** Optionally pass `VERSION` via `-ldflags` if `VERSION` is set or detected, while keeping default `go build` working cleanly.

## Out of Scope

- Adding a `--version` CLI flag or web UI (handled separately if needed).
- Automatic update checks or network requests for newer releases.

## Open Questions

- None.

# AGENTS.md

Instructions for AI coding agents and contributors working on `opendraft`.

## Project Overview

`opendraft` is an open-source, poor man's Wahoo Headwind alternative written in Go (`github.com/bfarka/opendraft`).

## AI-First Feature Workflow

All features are documented as Markdown specifications in `docs/features/` **before** or **alongside** implementation:

1. **Create a feature spec first:** Copy `docs/features/_template.md` to `docs/features/NNN-short-name.md` (e.g. `001-initial-cli.md`), fill out the problem, user story, acceptance criteria, design notes, and out-of-scope items, and register it in `docs/features/README.md`.
2. **Keep specs as the source of truth:** When requirements or design decisions change during implementation, update the corresponding `docs/features/*.md` file and its status (`Draft` -> `Approved` -> `In Progress` -> `Implemented`).
3. **Small, verifiable increments:** Implement one feature (or well-defined slice) at a time with unit tests.

## Go Conventions

- **Layout:**
  - `cmd/opendraft/` — CLI / daemon entrypoint and wiring only.
  - `internal/` — Domain logic, packages, and adapters. Keep packages small, cohesive, and decoupled.
- **Code quality:**
  - Follow standard Go idioms (`gofmt`, `go vet ./...`, explicit error handling with context via `fmt.Errorf("...: %w", err)`).
  - Avoid unnecessary external dependencies; prefer the Go standard library unless a third-party package provides clear value.
  - Write table-driven unit tests (`*_test.go`) for new behavior.
- **Verification before finishing:**
  - Run `go fmt ./...`, `go vet ./...`, and `go test ./...` (or `make check`).

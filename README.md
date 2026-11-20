# opendraft

An open-source, poor man's Wahoo Headwind alternative implemented in Go.

> **Status:** Bootstrapped — no features implemented yet.

## AI-First & Spec-Driven Development

`opendraft` is developed **AI-first** using a spec-driven workflow:

1. Every new feature starts as a Markdown specification in [`docs/features/`](docs/features/README.md) created from [`docs/features/_template.md`](docs/features/_template.md).
2. AI agents and contributors follow [`AGENTS.md`](AGENTS.md) for project conventions, architecture boundaries, and testing expectations.
3. Code is implemented only after the feature spec's scope and acceptance criteria are clear.

## Project Layout

```text
cmd/opendraft/     Application entrypoint
docs/features/     Feature specifications (Markdown)
internal/          Private application packages
```

## Getting Started

Prerequisites: Go 1.27+

```sh
make build
make test
```

Or directly with Go:

```sh
go run ./cmd/opendraft
go test ./...
```

## License

[MIT](LICENSE)

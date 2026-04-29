# SurveyController-go

SurveyController-go is the Go rewrite of SurveyController. The target is a lightweight, high-performance CLI for authorized survey automation testing.

> This project is for authorized learning and testing only. Do not use it to pollute third-party survey data, bypass platform protections, or generate fraudulent responses.

## Current Status

`v0.1` is the project initialization release:

- Go module and minimal `surveyctl` CLI.
- Persistent discovery notes from the original Python project.
- Architecture and roadmap documents.
- GitHub issue/PR templates.
- CI workflow for formatting, tests, vet, race tests, and cross-platform builds.

Three-platform support is a `v1.0` goal. `v0.1` only reserves the architecture boundaries.

## Quick Start

```powershell
go test ./...
go vet ./...
go run ./cmd/surveyctl version
```

Expected version output:

```text
surveyctl v0.1.0
```

## Runtime Engine Direction

Future versions will support three selectable engines:

- `hybrid`: browser-compatible path with HTTP fast paths when a provider explicitly supports safe request reuse.
- `browser`: browser-only mode for maximum compatibility.
- `http`: HTTP-only mode; it fails clearly when the selected provider does not support it.

## Development

Read these first:

- [Development Guide](docs/development.md)
- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [Original Project Analysis](docs/discovery/original-project-analysis.md)
- [Repository Access Notes](docs/discovery/repo-access.md)

# Development Guide

## Defaults

- Use GitHub CLI (`gh`) for GitHub work.
- Use issue -> branch -> PR after the bootstrap commit.
- Keep `main` releasable.
- Prefer small, reviewable changes.

## Local Commands

```powershell
go run ./cmd/surveyctl version
go test ./...
go test -race ./...
go vet ./...
gofmt -w (git ls-files '*.go')
```

On Windows, `go test -race` requires CGO and a C compiler such as `gcc`.
If local race tests fail with `C compiler "gcc" not found`, run the normal test
suite locally and rely on the Ubuntu CI race job until a local C toolchain is
installed.

## Issue First

Create or claim an issue before changing behavior:

```powershell
gh issue create
gh issue view 123
```

The issue should define the user goal, scope, acceptance criteria, and any safety constraints.

## Branches

Use short issue-linked branch names:

```text
codex/issue-123-config-schema
codex/issue-124-provider-contract
```

## Commits

Use concise commit messages:

```text
chore: bootstrap go project
feat: add runtime engine mode parser
test: cover provider capability checks
```

## Pull Requests

Open draft PRs with GitHub CLI:

```powershell
gh pr create --draft --fill
```

Every PR must include:

- Linked issue.
- What changed.
- Why it changed.
- Tests run.
- Performance impact when relevant.
- Risk and rollback notes.

## Style Rules

- Follow Google Go Style Guide and Go Code Review Comments.
- Keep functions single-purpose.
- Keep interfaces close to consumers.
- Use clear package names.
- Use `context.Context` for cancellable work.
- Treat platform verification, login, or anti-abuse pages as stop-and-report conditions.

## Bootstrap Exception

`v0.1` is allowed one direct bootstrap commit to `main` because the remote repository is empty. This exception must not be repeated for normal feature work.

# Contributing

Thanks for helping build SurveyController-go.

## Required Workflow

- Use GitHub CLI (`gh`) by default for GitHub operations.
- After the `v0.1` bootstrap commit, every non-trivial change must start from a GitHub Issue.
- Work on a topic branch, then open a Pull Request.
- Do not push directly to `main` after the bootstrap exception.

Suggested flow:

```powershell
gh issue create
git checkout -b codex/issue-123-short-description
go test ./...
gh pr create --draft --fill
```

## Go Style

- Follow Google Go Style Guide and Go Code Review Comments.
- Run `gofmt` before committing.
- Prefer small packages with clear ownership.
- Keep functions single-purpose.
- Define interfaces on the consuming side.
- Return explicit errors; do not hide failures behind logs only.
- Use `context.Context` for work that can be canceled or timed out.
- Avoid catch-all package names such as `util`, `common`, and `misc`.

## Testing

Before opening a PR, run:

```powershell
gofmt -w (git ls-files '*.go')
go test ./...
go test -race ./...
go vet ./...
```

If a check cannot run locally, explain why in the PR.

## Project Scope

`v0.1` is initialization only. Provider parsing and runtime behavior are planned for later versions. The `v1.0` target is official support for WJX, Tencent Questionnaire, and Credamo.

## Safety

This project is only for authorized learning and testing. Contributions that bypass platform verification, automate abuse, or facilitate fraudulent survey submissions will not be accepted.

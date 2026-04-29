# Repository Access Notes

Date: 2026-04-29

## GitHub CLI Preference

Project GitHub operations should prefer GitHub CLI (`gh`). This preference is persisted in `CONTRIBUTING.md` and `docs/development.md`.

## Verified Account and Repository

Current GitHub CLI account:

- `LING71671`

Target repository:

- `https://github.com/hungryM0/SurveyController-go`
- Private repository.
- Default branch configured as `main`.
- Repository was empty before `v0.1` initialization.

Verified permissions from `gh api repos/hungryM0/SurveyController-go`:

- `pull`: true
- `push`: true
- `triage`: true
- `maintain`: false
- `admin`: false

`gh repo view hungryM0/SurveyController-go` returned a GraphQL 401 during discovery, but REST calls through `gh api` succeeded. Prefer `gh api` when repository metadata checks hit that GraphQL issue.

## Local State Before Initialization

Path:

- `B:\SurveyController\SurveyController-go`

Initial local state:

- Directory existed.
- Directory was empty.
- Directory was not a git repository.

The remote repository was also empty, so `v0.1` is allowed to make one bootstrap commit directly to `main`. After that, normal development must use issue -> branch -> PR.

## Tool Versions

Observed local versions:

- Go: `go1.26.0 windows/amd64`
- Git: `2.51.0.windows.1`
- GitHub CLI: `2.89.0`
- `golangci-lint`: not installed locally during discovery.

Verified GitHub Actions tags:

- `actions/checkout@v5`
- `actions/setup-go@v6`
- `actions/upload-artifact@v7`

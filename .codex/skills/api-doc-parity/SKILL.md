---
name: api-doc-parity
description: Keep SurveyCore HTTP handlers, response schemas, SDK docs, README examples, and tests aligned. Use when routes, request bodies, error codes, response fields, pagination behavior, or config-generation output change.
---

# API Doc Parity

This skill prevents `internal/api` and `docs/sdk` from drifting apart.

## Read first

- `internal/api/server.go`
- `internal/api/types.go`
- `internal/api/server_test.go`
- `docs/sdk/api.md`
- `docs/sdk/schemas.md`
- `docs/sdk/errors.md`
- `docs/sdk/examples.md`
- `README.md`

## Workflow

1. Identify contract surface.

- New route.
- Request body change.
- Response field change.
- Error code or message change.
- Pagination or query param change.

2. Update code and docs together.

- Handler and request/response structs.
- Tests.
- SDK docs.
- README examples if user-facing behavior changed.

3. Keep examples honest.

- Example JSON must match current field names.
- Error docs must match actual `writeError` codes.
- If `/api/configs` output changed, sync `schemas.md` and README sample config.

## Rules

- Do not describe fields the API never returns.
- Do not leave stale error codes in docs.
- If the only change is internal and HTTP contract did not move, skip this skill.

---
name: config-contract-sync
description: Keep SurveyCore runtime configuration contracts aligned across models, defaults, validation, execution shaping, example configs, SDK schemas, and tests. Use when config fields, defaults, validation rules, or question-entry semantics change.
---

# Config Contract Sync

This skill keeps config changes from rotting half the codebase.

## Scope

Read these files first when config shape changes:

- `internal/models/config.go`
- `internal/config/config.go`
- `internal/execution/config.go`
- `configs/example.json`
- `docs/sdk/schemas.md`
- `internal/config/config_test.go`
- `internal/models/execution_test.go`

## Workflow

1. Classify the change.

- New field.
- Default value change.
- Validation rule change.
- Question-entry semantic change.
- Derived execution behavior change.

2. Update the contract in order.

- Source struct in `internal/models/`.
- Defaulting and merge logic in `internal/config/`.
- Execution projection in `internal/config/config.go` and `internal/execution/`.
- Example config.
- SDK docs.
- Tests.

3. Check drift explicitly.

- If a field is accepted by JSON but ignored by execution shaping, fix it.
- If docs mention a field that models no longer expose, fix it.
- If defaults changed but `configs/example.json` stayed old, fix it.

## Rules

- A config field is not done until model, defaults, docs, and tests agree.
- If the field reaches API responses from `/api/configs`, update docs and examples in the same change.
- If the field affects reverse fill, psychometric logic, or provider metadata binding, read `references/risky-areas.md`.

---
name: provider-feature-onboarding
description: Scaffold a new SurveyCore provider capability with the right package split and wiring. Use when adding a new provider, a new parser path, a new answer-builder capability, or a provider-specific question behavior without spreading platform branches across shared packages.
---

# Provider Feature Onboarding

This skill adds provider code without turning the repo into a pile of cross-package hacks.

## Workflow

1. Define the unit of work.

- New provider.
- Existing provider new question type.
- Existing provider new parser or submit branch.

2. Keep provider logic inside its package.

- Create or update files under `internal/providers/<provider>/`.
- Register the provider in `internal/providers/registry.go`.
- Keep shared helpers in `internal/providers/providerutil/` only if at least two providers need them.

3. Wire the full slice, not one file.

- `provider.go`
- `parser.go` or `html_parser.go`
- `answer_builder.go`
- `submit.go` or `client.go`
- `types.go` if metadata shape changed
- tests

4. Update upstream-facing structs when needed.

- If parsed metadata changes, update `internal/models/question.go` and docs.
- If config-generation behavior changes, update `internal/config/`.

## Rules

- Do not put provider `if/else` branches into `internal/engine`, `internal/api`, or `internal/questions` unless there is no sane alternative.
- Follow existing provider package naming.
- Add the narrowest tests that prove registration, parsing, and answer generation.
- If the new capability changes API-visible metadata, also invoke `$api-doc-parity`.

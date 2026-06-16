---
name: provider-regression-lab
description: Build and run the smallest credible regression plan for SurveyCore provider changes. Use when parser, answer builder, submitter, proxy, scheduler, or provider detection code changes and you need proof that one platform fix did not break the others.
---

# Provider Regression Lab

This skill turns a code diff or bug fix into a minimal provider regression plan, then runs the local part of that plan.

## Workflow

1. Read the diff first.

- Identify touched providers and whether the change is parser, builder, submit, detection, shared util, proxy, or scheduler.
- Read `references/test-matrix.md` and choose the smallest matching test set.

2. Prefer narrow tests before broad tests.

- Provider package tests first.
- Cross-provider URL detection tests second.
- Full `go test ./...` only when shared code changed.
- `go test -race ./...` only for concurrency, storage, scheduler, proxy pool, or logging changes.

3. Separate local regression from live regression.

- Local regression is default.
- Live regression in `CI/live_tests/` is opt-in and only for changes that cannot be proven locally.

4. Report in this order.

- What changed.
- Which platforms are at risk.
- What was run.
- What still is not proven.

## Rules

- If one provider changed but a shared helper changed too, test all providers.
- If parser output shape changed, include config-generation and API parse tests.
- If request transport changed, include proxy and engine coverage.
- Do not invent live tests when local tests already prove the behavior.

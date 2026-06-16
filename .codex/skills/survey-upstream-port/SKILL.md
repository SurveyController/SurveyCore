---
name: survey-upstream-port
description: Port fixes from the SurveyController desktop repo into SurveyCore without copying GUI noise. Use when a user mentions an upstream commit, release, parser breakage, provider regression, or asks whether a desktop-side fix also needs to land in SurveyCore.
---

# Survey Upstream Port

Read upstream changes, strip GUI noise, and map the useful part into SurveyCore modules.

## Workflow

1. Start from one upstream anchor.

- Accept one of these inputs: desktop repo commit, tag, release note, issue link, or a concrete failure symptom.
- If the input is vague, inspect the desktop repo first and reduce it to a small set of changed files and behavior changes.

2. Classify the upstream change before touching code.

- `provider protocol`: request params, signatures, cookies, parsing selectors, submit payloads.
- `question semantics`: new question types, option metadata, jump/display logic, psychometric handling.
- `runtime/config`: config fields, defaults, validation, scheduler behavior.
- `desktop-only`: UI, tray, window state, local packaging. Usually ignore these for SurveyCore.

3. Map the change into SurveyCore.

- Provider parsing and submit logic usually lands in `internal/providers/<provider>/`.
- Shared provider logic lands in `internal/providers/providerutil/`.
- Answer semantics and runtime shaping usually land in `internal/questions/`, `internal/config/`, `internal/execution/`, or `internal/models/`.
- Request plumbing may land in `internal/network/httpclient/` or `internal/network/proxy/`.
- API-visible changes may also require `internal/api/`, `docs/sdk/`, `README.md`, and tests.

4. Produce exactly these outputs before large edits.

- Target files in SurveyCore.
- Change type.
- Minimum regression set.

5. Reimplement. Do not paste upstream code blindly.

- SurveyController and SurveyCore do not share language or architecture.
- Recreate intent in Go. Keep provider branches inside provider packages.
- If the upstream fix depends on desktop-only state, stop and redesign the equivalent server-side source of truth.

## Required checks

- Read `references/module-map.md` before deciding file targets.
- If the change touches a provider, read `references/provider-checklist.md`.
- Use `scripts/upstream_diff.ps1` to summarize upstream diffs when the user provides a commit-ish.
- Use `scripts/make_regression_plan.ps1` after mapping the affected SurveyCore files.

## Stop conditions

- The upstream change is only GUI behavior.
- The upstream fix relies on bundled binaries, desktop browser state, or local persistence that SurveyCore does not have.
- The licensing boundary is unclear and the only apparent path is direct code copy. Reimplement from behavior instead.

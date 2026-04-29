# Architecture

## Goal

SurveyController-go will be a lightweight CLI-first rewrite of the original Python SurveyController. The `v1.0` target is official support for WJX, Tencent Questionnaire, and Credamo.

`v0.1` only defines the initial boundaries.

## Layers

- `cmd/surveyctl`: command-line entrypoint.
- `internal/app`: future use-case orchestration.
- `internal/config`: versioned runtime configuration.
- `internal/engine`: runtime engine selection and execution-mode primitives.
- `internal/provider`: platform contracts and normalized survey definitions.
- `internal/runner`: worker orchestration and run-plan validation.
- `internal/browser`: future Playwright Go wrapper.
- `internal/answer`: future answer selection and question strategy logic.

## Runtime Engines

Runtime mode is selectable:

- `hybrid`: default. Use browser compatibility first, then provider-declared HTTP fast paths when safe.
- `browser`: browser-only mode for maximum compatibility.
- `http`: HTTP-only mode. It must fail clearly when the provider does not support it.

Providers must declare supported modes. The runner must not silently downgrade `http` to browser mode.

## Provider Contract

A provider is responsible for:

- Matching supported URLs.
- Parsing a survey into a normalized `SurveyDefinition`.
- Declaring runtime capabilities.
- Future answer execution and submission handling.
- Detecting completion, login, verification, and anti-abuse pages.

Adding a provider should not require changing the runner control loop.

## Configuration

Configuration should be explicit and versioned. Dynamic Python dictionaries from the original project should become Go structs with compatibility migrations when needed.

## Concurrency

Future runtime work should use:

- `context.Context` for cancellation and deadlines.
- Worker pools for concurrency.
- Browser semaphores for resource control.
- Provider-owned cleanup for platform resources.

## Safety

The project must stop and report when it encounters platform verification, login requirements, or anti-abuse pages. It must not implement verification bypass behavior.

# Original Project Analysis

Date: 2026-04-29

Source path: `B:\SurveyController\SurveyController-main`

## Overview

The original SurveyController project is a Python 3.11+ desktop application. It uses PySide6 and QFluentWidgets for the UI, Playwright for browser automation, `httpx` for HTTP paths, BeautifulSoup for HTML parsing, and OpenPyXL for spreadsheet-based reverse fill.

The README describes the tool as a one-stop survey automation program for WJX, Tencent Questionnaire, and Credamo, with support for custom answer distributions, random or regional IP settings, answer duration controls, reliability settings, QR parsing, config import/export, and AI-generated free-text answers.

## Repository Shape

Observed file counts:

- 301 Python files.
- 7 JavaScript files.
- 5 Markdown files.
- Existing GitHub workflows and issue templates.

The largest and highest-risk Python files include:

- `tencent/provider/runtime_interactions.py`
- `credamo/provider/runtime.py`
- `software/network/browser/driver.py`
- `wjx/provider/questions/text.py`
- `software/integrations/ai/client.py`
- `wjx/provider/_submission_core.py`
- `credamo/provider/parser.py`
- `wjx/provider/runtime.py`
- `wjx/provider/questions/single.py`
- `tencent/provider/runtime_answerers.py`

These files are migration hotspots because they mix platform DOM details, runtime state, retries, and fallback behavior.

## Architecture

The main entrypoint is `SurveyController.py`, which calls `software.app.main.main`.

Important boundaries:

- `software/providers`: platform detection, provider registry, normalized survey definitions.
- `software/core/config`: runtime config schema and JSON codec.
- `software/core/task`: execution config, execution state, thread progress, proxy leases, reverse-fill state.
- `software/core/engine`: worker execution loop, browser session service, submission service, stop policy, driver factory.
- `wjx/provider`, `tencent/provider`, `credamo/provider`: platform-specific parser, runtime, navigation, answerers, and submission detection.
- `software/network/browser`: Playwright wrapper and browser lifecycle.
- `software/network/proxy`: proxy source, pool, session, quota, and area logic.
- `software/core/psychometrics`: reliability and joint optimizer behavior.
- `software/core/reverse_fill`: spreadsheet reverse-fill parsing and runtime coordination.

The current Python code already has a useful provider registry model. Go should keep that boundary instead of doing a line-by-line rewrite.

## Provider Notes

WJX:

- Parses through HTTP first, then falls back to Playwright.
- Has detailed HTML parsing split across choice, common, matrix, and rule helpers.
- Has a headless HTTP submission path that captures and reuses browser-generated requests.

Tencent Questionnaire:

- Uses API endpoints for session, metadata, and questions where possible.
- Falls back to Playwright when needed.
- Contains many interaction helpers for choice, text, dropdown, matrix, and star-like questions.

Credamo:

- Depends heavily on Playwright and DOM evaluation.
- Handles dynamic question reveal by priming answers during parsing.
- Contains forced-choice and simple arithmetic trap detection.

## Test Assets

Existing tests cover:

- Config codec and runtime paths.
- Engine loop, cleanup, runtime control, submission service.
- Provider common behavior, Credamo parser/runtime, WJX reverse fill, survey cache.
- Psychometric orientation and joint optimizer.
- Question validation.
- Live parser regression.

Go migration should reuse the behavioral intent of these tests and gradually move important cases into fixtures.

## Migration Risks

- Python uses dynamic dictionaries for question metadata; Go needs explicit structs and versioned schema.
- UI and runtime config are coupled in several paths; Go CLI should separate config, app orchestration, and provider runtime.
- Platform DOM behavior is brittle and must be isolated behind provider implementations.
- Browser lifecycle and cross-thread cleanup issues in Python should become explicit context cancellation and resource ownership in Go.
- HTTP fast paths must be provider-declared and tested; they should not silently replace browser mode.

## V0.1 Implications

`v0.1` should not implement real provider behavior. It should persist these findings, initialize the repository, and create architecture seams for provider contracts, engine modes, config, and runner orchestration.

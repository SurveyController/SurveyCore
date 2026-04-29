# Security Policy

## Supported Versions

`v0.1` is an initialization milestone. Security reports are accepted for the `main` branch until the first tagged release policy is defined.

## Reporting

Please report security issues privately before opening a public issue. If no private channel is available, open a minimal GitHub Issue that asks for maintainer contact without publishing exploit details.

## Boundaries

Do not submit reports that require abusing third-party survey platforms, bypassing verification systems, or collecting data without permission.

## Design Defaults

- Provider implementations must stop and report when they hit login, verification, or anti-abuse pages.
- HTTP-only execution must fail clearly when a provider does not explicitly support it.
- Secrets must not be written to logs, test fixtures, or examples.

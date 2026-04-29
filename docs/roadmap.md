# Roadmap

## Version Targets

- `v0.1`: Project initialization, development rules, CI, persistent discovery docs, minimal CLI.
- `v0.2`: CLI command framework, config schema, logging, and error model.
- `v0.3`: Provider contract, URL detection, and fixture-based test system.
- `v0.4`: Browser engine abstraction and Playwright Go wrapper.
- `v0.5`: HTTP engine abstraction and selectable `hybrid`, `browser`, and `http` runtime modes.
- `v0.6`: WJX parser prototype.
- `v0.7`: Tencent Questionnaire parser prototype.
- `v0.8`: Credamo parser prototype.
- `v0.9`: Three-platform runtime preview, benchmarks, and stability regression.
- `v1.0`: Official three-platform support with parsing, config generation, basic run workflow, tests, and documentation.

## V0.1 Waves

### Wave 0: Persist Discovery

- Add original Python project analysis.
- Add GitHub access and local tool notes.
- Add roadmap.

### Wave 1: Initialize Repository

- Initialize git on `main`.
- Set `origin` to `https://github.com/hungryM0/SurveyController-go.git`.
- Create Go module `github.com/hungryM0/SurveyController-go`.
- Add minimal `cmd/surveyctl` CLI with `version`.

### Wave 2: Development Rules

- Add contribution guide.
- Add development guide.
- Add code of conduct and security policy.
- Add editor and ignore rules.

### Wave 3: GitHub Governance

- Add issue templates.
- Add PR template.
- Add CI workflow.
- Add release workflow skeleton.

### Wave 4: Architecture Placeholders

- Add architecture document.
- Add minimal packages for provider, config, runner, and engine mode.
- Define selectable runtime engines without implementing real survey execution.

### Wave 5: Verify and Bootstrap

- Run `gofmt`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Commit bootstrap to `main`.
- Push `main` to the empty remote.

## V1.0 Boundary

Three-platform support is a `v1.0` requirement, not a `v0.1` requirement. No provider should claim support until parser and runtime behavior are covered by tests.

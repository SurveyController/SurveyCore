# SurveyController Go Rewrite - Migration Plan

## Original Project Architecture

The Python SurveyController is a survey automation tool supporting 3 platforms:
- **WJX (问卷星)** - `wjx/provider/`
- **Tencent Survey (腾讯问卷)** - `tencent/provider/`
- **Credamo (见数)** - `credamo/provider/`

### Key Components

| Python Module | Go Equivalent | Status |
|---|---|---|
| `software/core/config/schema.py` → `RuntimeConfig` | `internal/models/config.go` → `RuntimeConfig` | Complete |
| `software/core/questions/schema.py` → `QuestionEntry` | `internal/models/question.go` → `QuestionEntry` | Complete |
| `software/providers/contracts.py` → `SurveyQuestionMeta` | `internal/models/survey.go` → `SurveyQuestionMeta` | Complete |
| `software/core/task/task_context.py` → `ExecutionConfig/State` | `internal/models/execution.go` → `ExecutionConfig/State` | Complete |
| `software/providers/registry.py` | `internal/providers/registry.go` | Complete |
| `software/providers/common.py` | `internal/providers/common.go` | Complete |
| `wjx/provider/parser.py` | `internal/providers/wjx/html_parser.go` | Complete |
| `wjx/provider/http_runtime.py` | `internal/providers/wjx/provider.go` + `answer_builder.go` | Complete |
| `tencent/provider/parser.py` | `internal/providers/tencent/provider.go` | Complete |
| `tencent/provider/http_runtime.py` | `internal/providers/tencent/provider.go` | Complete |
| `credamo/provider/parser.py` | `internal/providers/credamo/provider.go` | Complete |
| `credamo/provider/http_runtime.py` | `internal/providers/credamo/provider.go` | Complete |
| `software/core/engine/async_engine.py` | `internal/engine/engine.go` | Complete |
| `software/core/engine/async_scheduler.py` | `internal/engine/scheduler.go` | Complete |
| `software/network/http/` | `internal/network/httpclient/` | Complete |
| `software/network/proxy/` | `internal/network/proxy/` | Complete |
| `software/core/questions/` (answer builders) | `internal/questions/` | Complete |
| reverse-fill runtime | `internal/reversefill/` + `internal/models/reverse_fill.go` | Complete |
| QR decode / Excel export | `internal/io/` | Complete |

### Architecture Mapping

- **Python asyncio** → **Go goroutines + channels**
- **PySide6 Qt GUI** → **CLI (standard `flag`)** (GUI deferred to future phase)
- **httpx** → **net/http** (standard library)
- **BeautifulSoup** → **goquery**
- **dataclasses** → **Go structs**
- **threading.Event** → **context.Context + channels**
- **Provider pattern** → **Go interfaces**

## Go Project Structure

```
go-rewrite/
├── cmd/surveycontroller/     # CLI entry point
│   └── main.go
├── internal/
│   ├── config/               # Config serialization (JSON)
│   │   └── default_entries.go # Default per-question config generation
│   ├── models/               # Core data structures
│   │   ├── config.go         # RuntimeConfig
│   │   ├── execution.go      # ExecutionConfig, ExecutionState
│   │   ├── question.go       # QuestionEntry
│   │   ├── reverse_fill.go   # Reverse-fill runtime state
│   │   ├── survey.go         # SurveyQuestionMeta, SurveyDefinition
│   │   └── proxy.go          # ProxyLease, RandomIPSession
│   ├── providers/            # Survey platform adapters
│   │   ├── common.go         # URL detection, provider constants
│   │   ├── registry.go       # Provider registry
│   │   ├── adapter.go        # Provider interface
│   │   ├── wjx/              # 问卷星
│   │   ├── tencent/          # 腾讯问卷
│   │   └── credamo/          # 见数
│   ├── engine/               # Async execution engine
│   │   ├── engine.go         # Main engine
│   │   └── scheduler.go      # Bounded concurrency scheduler
│   ├── network/
│   │   ├── httpclient/       # HTTP client pool
│   │   └── proxy/            # Proxy management
│   ├── questions/            # Answer generation
│   ├── reversefill/          # Reverse-fill sample parsing
│   ├── io/                   # QR decode and Excel export
│   └── logging/              # Logging utilities
├── configs/                  # Example configs
└── tests/                    # Integration tests
```

## Key Design Decisions

1. **CLI-first**: No GUI in initial rewrite; use a small standard-library `flag` CLI
2. **Pure Go concurrency**: goroutines instead of asyncio
3. **Standard library HTTP**: net/http + sync.Pool for client reuse
4. **JSON config**: Compatible with Python version's JSON format
5. **Interface-based providers**: Go interface instead of Python hook callables
6. **Atomic provider boundaries**: parsing, answer planning, submit-body building, and HTTP submission are separated per provider
7. **Runtime-first answer generation**: distribution tracking, consistency rules, psychometric bias, text generation, and reverse-fill samples flow through `internal/questions`

## Current Verification Gate

Run these checks before pushing rewrite changes:

```bash
go test ./...
go vet ./...
go build ./...
go test -race ./...
go mod tidy -diff
git diff --check
```

The current rewrite contains 86 Go tests covering models, config generation, scheduler/engine behavior, provider URL detection, WJX/Tencent/Credamo provider paths, proxy handling, QR/Excel IO, reverse-fill, and runtime answer generation.

# SurveyController Go Rewrite - Migration Plan

## Original Project Architecture

The Python SurveyController is a survey automation tool supporting 3 platforms:
- **WJX (问卷星)** - `wjx/provider/`
- **Tencent Survey (腾讯问卷)** - `tencent/provider/`
- **Credamo (见数)** - `credamo/provider/`

### Key Components

| Python Module | Go Equivalent | Status |
|---|---|---|
| `software/core/config/schema.py` → `RuntimeConfig` | `internal/models/config.go` → `RuntimeConfig` | Migrating |
| `software/core/questions/schema.py` → `QuestionEntry` | `internal/models/question.go` → `QuestionEntry` | Migrating |
| `software/providers/contracts.py` → `SurveyQuestionMeta` | `internal/models/survey.go` → `SurveyQuestionMeta` | Migrating |
| `software/core/task/task_context.py` → `ExecutionConfig/State` | `internal/models/execution.go` → `ExecutionConfig/State` | Migrating |
| `software/providers/registry.py` | `internal/providers/registry.go` | Migrating |
| `software/providers/common.py` | `internal/providers/common.go` | Migrating |
| `wjx/provider/parser.py` | `internal/providers/wjx/parser.go` | Migrating |
| `wjx/provider/http_runtime.py` | `internal/providers/wjx/http_runtime.go` | Migrating |
| `tencent/provider/parser.py` | `internal/providers/tencent/parser.go` | Migrating |
| `tencent/provider/http_runtime.py` | `internal/providers/tencent/http_runtime.go` | Migrating |
| `credamo/provider/parser.py` | `internal/providers/credamo/parser.go` | Migrating |
| `credamo/provider/http_runtime.py` | `internal/providers/credamo/http_runtime.go` | Migrating |
| `software/core/engine/async_engine.py` | `internal/engine/engine.go` | Migrating |
| `software/core/engine/async_scheduler.py` | `internal/engine/scheduler.go` | Migrating |
| `software/network/http/` | `internal/network/httpclient/` | Migrating |
| `software/network/proxy/` | `internal/network/proxy/` | Migrating |
| `software/core/questions/` (answer builders) | `internal/questions/` | Migrating |

### Architecture Mapping

- **Python asyncio** → **Go goroutines + channels**
- **PySide6 Qt GUI** → **CLI (cobra)** (GUI deferred to future phase)
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
│   ├── models/               # Core data structures
│   │   ├── config.go         # RuntimeConfig
│   │   ├── execution.go      # ExecutionConfig, ExecutionState
│   │   ├── question.go       # QuestionEntry
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
│   │   ├── scheduler.go      # Bounded concurrency scheduler
│   │   └── status.go         # Status bus
│   ├── network/
│   │   ├── httpclient/       # HTTP client pool
│   │   └── proxy/            # Proxy management
│   ├── questions/            # Answer generation
│   └── logging/              # Logging utilities
├── configs/                  # Example configs
└── tests/                    # Integration tests
```

## Key Design Decisions

1. **CLI-first**: No GUI in initial rewrite; use cobra CLI
2. **Pure Go concurrency**: goroutines instead of asyncio
3. **Standard library HTTP**: net/http + sync.Pool for client reuse
4. **JSON config**: Compatible with Python version's JSON format
5. **Interface-based providers**: Go interface instead of Python hook callables

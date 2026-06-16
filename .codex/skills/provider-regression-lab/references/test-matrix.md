# Provider 回归矩阵

## 改动到文件 -> 最小测试集

- `internal/providers/wjx/*.go`
  - `go test ./internal/providers/wjx ./tests`
- `internal/providers/tencent/*.go`
  - `go test ./internal/providers/tencent ./tests`
- `internal/providers/credamo/*.go`
  - `go test ./internal/providers/credamo ./tests`
- `internal/providers/providerutil/*.go`
  - `go test ./internal/providers/... ./tests`
- `internal/providers/common.go` 或 provider 识别逻辑
  - `go test ./tests`
- `internal/config/*.go`
  - `go test ./internal/config ./tests`
- `internal/api/*.go`
  - `go test ./internal/api ./tests`
- `internal/network/proxy/*.go`
  - `go test ./internal/network/proxy`
  - 必要时 `go test -race ./internal/network/proxy ./...`
- `internal/engine/*.go` 或 `internal/tasks/*.go`
  - `go test ./internal/engine ./internal/tasks ./tests`
  - 并发相关再加 `-race`

## 升级条件

- 共享模型或共享执行配置变了
  - 跑 `go test ./...`
- 调度、日志、任务存储、代理池变了
  - 跑 `go test -race ./...`
- 只有文档变了
  - 不跑 provider 回归，改走 `$api-doc-parity`

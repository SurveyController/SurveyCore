# Provider 迁移检查单

## 1. 先定平台

- `wjx`
- `qq`
- `credamo`

## 2. 再定环节

- 解析炸了：先看 `parser.go`、`html_parser.go`、`types.go`
- 构答炸了：先看 `answer_builder.go`
- 提交炸了：先看 `submit.go`、`client.go`
- 平台识别炸了：先看 `internal/providers/common.go`、`tests/provider_test.go`

## 3. 再定影响面

- 题目元数据变了：补 `SurveyQuestionMeta`
- 题型语义变了：补 `internal/questions/` 或 `internal/config/`
- 新 header、cookie、UA 规则：补 `internal/network/`
- 新错误分支会冒到 API：补 `internal/api/` 和文档

## 4. 最小回归

- 改 parser：跑目标 provider 单测 + `tests/provider_test.go`
- 改 answer builder：再跑目标 provider builder 测试
- 改 submit/client：再跑相关 provider 全量包测试
- 改共享逻辑：再跑 `go test ./...`

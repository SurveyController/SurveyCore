# SurveyController -> SurveyCore 模块映射

先看意图，再看文件名。桌面端和 SurveyCore 不是一套架构。

## 高概率映射

- 上游解析器、提交器、平台协议修复
  - 落到 `internal/providers/wjx/`
  - 落到 `internal/providers/tencent/`
  - 落到 `internal/providers/credamo/`
- 上游公共平台辅助逻辑
  - 落到 `internal/providers/providerutil/`
- 上游题目结构、题型行为、选项元数据
  - 落到 `internal/models/`
  - 落到 `internal/questions/`
  - 落到 `internal/config/`
  - 落到 `internal/execution/`
- 上游代理、请求头、网络重试、客户端改动
  - 落到 `internal/network/httpclient/`
  - 落到 `internal/network/proxy/`
- 上游任务生命周期、停止逻辑、日志、调度
  - 落到 `internal/tasks/`
  - 落到 `internal/engine/`
  - 落到 `internal/logging/`
- 上游配置字段、默认值、校验规则
  - 落到 `internal/models/config.go`
  - 落到 `internal/config/`
  - 落到 `configs/example.json`
  - 落到 `docs/sdk/schemas.md`
- 上游“解析问卷但不提交”相关行为
  - 落到 `internal/api/server.go`
  - 落到 `TaskService` 实现

## 低价值映射

- 桌面窗口、托盘、按钮、设置页、打包脚本
  - 通常不映射

## SurveyCore 关键入口

- HTTP 接口入口：`internal/api/server.go`
- provider 注册：`internal/providers/registry.go`
- provider URL 识别：`tests/provider_test.go` 和 `internal/providers/common.go`
- 运行配置整形：`internal/config/config.go`
- 调度：`internal/engine/engine.go`、`internal/engine/scheduler.go`

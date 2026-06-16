# Provider 包布局

现有 provider 可以当模板：

- `internal/providers/wjx/`
- `internal/providers/tencent/`
- `internal/providers/credamo/`

## 常见文件职责

- `provider.go`
  - 暴露 `NewProvider()`
  - 实现 `ProviderName()`
  - 连接 parse / build / submit
- `parser.go` 或 `html_parser.go`
  - 解析问卷结构
  - 产出 `SurveyDefinition` 和 `SurveyQuestionMeta`
- `answer_builder.go`
  - 把 `ExecutionConfig` 转成平台提交答案
- `submit.go`
  - 提交请求
- `client.go`
  - 处理请求、cookie、header、基础 URL
- `validator.go`
  - provider 侧的输入校验
- `types.go`
  - 平台私有字段模型

## 最少测试

- provider 注册是否可取到
- 解析一个最小样本
- answer builder 对目标题型是否输出对的结构
- 提供者 URL 识别是否正确

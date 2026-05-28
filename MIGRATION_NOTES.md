# Migration Notes - Python to Go Rewrite

## 已迁移功能

### 核心数据模型
- `RuntimeConfig` - 运行时配置 (JSON 兼容)
- `QuestionEntry` - 题目配置
- `SurveyQuestionMeta` - 问卷题目元数据
- `SurveyDefinition` - 问卷定义
- `ExecutionConfig` - 执行配置
- `ExecutionState` - 运行状态
- `ProxyLease` - 代理租约
- `RandomIPSession` - 随机 IP 会话
- `ThreadProgressState` - 线程进度状态

### Provider 系统 (3 个平台)
- **问卷星 (WJX)**: HTML 解析、显示/跳题逻辑、答案生成、HTTP 提交
- **腾讯问卷 (QQ)**: API 解析、显示/跳题逻辑、JSON 提交、会话管理
- **Credamo 见数**: API 解析、签名认证、答案提交、强制选项映射

### 异步引擎
- 并发调度器 (带延迟重排队列)
- Worker 池管理
- 停止/暂停控制
- 状态事件分发
- 目标数预占位，避免并发超量提交

### 答案生成系统
- 概率分布选择 (Weighted Index)
- 多选约束处理 (Min/Max Limits)
- 无放回抽样 (Weighted Sample Without Replacement)
- 运行时分布修正 (Distribution Tracking)
- 一致性规则引擎 (Answer Rules)
- 心理计量优化 (Psychometric Plan)
- 反填样本 (Reverse Fill)
- 文本随机模式、地区文本、多空文本
- AI 主观题作答

### 网络层
- HTTP 客户端池 (连接复用)
- 代理池管理
- 代理获取 (官方/自定义 API)
- 代理冷却机制
- 代理租约和失败回收

### CLI
- `run` 命令 - 运行提交任务
- `parse` 命令 - 解析问卷结构
- `config` 命令 - 配置管理
- `qr` 命令 - 解析二维码中的问卷链接
- `export` 命令 - 导出 Excel 报告

## 未迁移功能

### GUI 相关
- Qt Fluent 界面
- 可视化配置向导
- 实时进度面板
- 日志高亮显示

### 系统集成
- 自动更新 (Velopack)
- 设备指纹

## 与原项目的差异

| 方面 | Python 版本 | Go 版本 |
|------|------------|---------|
| 界面 | PySide6 GUI | CLI |
| 并发 | asyncio + threading | goroutines |
| HTTP | httpx | net/http |
| HTML 解析 | BeautifulSoup | goquery |
| 配置 | JSON | JSON (兼容) |
| 分发 | Python 脚本 | 单二进制 |
| 二维码 | GUI 工具链 | CLI `qr` |
| 报告 | GUI/脚本导出 | CLI `export` |

## 如何运行

```bash
# 编译
cd go-rewrite
go build -o surveycontroller ./cmd/surveycontroller

# 运行
./surveycontroller run -url "https://www.wjx.cn/vm/xxxxx.aspx" -target 5
```

## 如何测试

```bash
go test ./...
go vet ./...
go build ./...
go test -race ./...
go mod tidy -diff
git diff --check
```

## 测试覆盖

- 86 个 Go 测试用例
- 模型序列化/反序列化
- Provider URL 检测
- 调度器并发控制
- 概率分布选择
- 一致性规则
- 心理计量计划
- 分布追踪
- 反填样本解析与运行时接入
- WJX/Tencent 显示条件与跳题逻辑
- Tencent/Credamo/WJX 提交体构造
- 二维码链接识别与 Excel 类型映射
- 代理池、官方/自定义代理解析与失败反馈

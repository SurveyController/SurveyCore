# SurveyConsole

SurveyController 的无图形界面版本。

当前版本运行为本地 HTTP API 服务。它负责解析问卷、生成配置、创建提交任务、查询任务状态、停止任务和读取任务日志。

> 本项目仅供已授权的学习与测试使用。不要用于污染第三方问卷数据。

## 支持平台

- 问卷星 (WJX)
- 腾讯问卷 (QQ)
- Credamo 见数

## 当前能力

- 解析问卷结构
- 生成默认运行配置
- 创建异步提交任务
- 查询任务列表和任务详情
- 停止运行中的任务
- 持久化任务状态和 JSONL 日志
- 从二维码图片中解析问卷链接
- 多题型答案生成
- 概率分布配置
- 运行时分布修正
- 一致性规则
- 心理计量优化
- 反填样本
- 随机 IP 代理池
- 结构化终端日志

## 快速开始

```bash
git clone https://github.com/SurveyController/SurveyConsole.git
cd SurveyConsole

go build -o surveyconsole ./cmd/surveycontroller
./surveyconsole
```

服务默认监听：

```text
127.0.0.1:19178
```

修改监听地址：

```bash
SURVEYCONSOLE_ADDR=127.0.0.1:8080 ./surveyconsole
```

Windows PowerShell：

```powershell
$env:SURVEYCONSOLE_ADDR="127.0.0.1:8080"
.\surveyconsole.exe
```

## API 使用

### 健康检查

```bash
curl http://127.0.0.1:19178/api/health
```

返回：

```json
{"status":"ok"}
```

### 版本号

```bash
curl http://127.0.0.1:19178/api/version
```

### 解析问卷

```bash
curl -X POST http://127.0.0.1:19178/api/surveys/parse \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.wjx.cn/vm/xxxxx.aspx"}'
```

这个接口只解析题目结构，不会提交答案。

### 生成配置

```bash
curl -X POST http://127.0.0.1:19178/api/configs \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.wjx.cn/vm/xxxxx.aspx"}'
```

传入 `url` 时，服务会解析问卷，并生成默认题目配置。

不传 `url` 时，只返回默认配置模板：

```bash
curl -X POST http://127.0.0.1:19178/api/configs \
  -H "Content-Type: application/json" \
  -d '{}'
```

### 创建提交任务

```bash
curl -X POST http://127.0.0.1:19178/api/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.wjx.cn/vm/xxxxx.aspx",
    "target": 10,
    "threads": 3,
    "answer_duration": [60, 120]
  }'
```

返回示例：

```json
{"status":"pending","task_id":"6f1b2c3d4e5f6789"}
```

任务会在后台运行。创建成功不代表提交已经完成。

### 查询任务列表

```bash
curl http://127.0.0.1:19178/api/tasks
```

### 查询任务详情

```bash
curl http://127.0.0.1:19178/api/tasks/6f1b2c3d4e5f6789
```

任务状态：

| 状态 | 含义 |
|------|------|
| `pending` | 已创建，等待运行 |
| `running` | 正在运行 |
| `succeeded` | 已完成 |
| `failed` | 执行失败 |
| `stopped` | 已停止 |
| `interrupted` | 服务重启导致中断 |

### 停止任务

```bash
curl -X POST http://127.0.0.1:19178/api/tasks/6f1b2c3d4e5f6789/stop
```

### 读取任务日志

```bash
curl http://127.0.0.1:19178/api/tasks/6f1b2c3d4e5f6789/logs
```

日志同时写入本地文件：

```text
data/tasks/<task_id>.logs.jsonl
```

### 解析二维码

```bash
curl -X POST http://127.0.0.1:19178/api/qrcode/decode \
  -F "image=@qrcode.png"
```

返回：

```json
{"url":"https://www.wjx.cn/vm/xxxxx.aspx"}
```

## 运行配置

创建任务时，请直接提交 `RuntimeConfig` JSON。

常用字段：

| 字段 | 说明 |
|------|------|
| `url` | 问卷链接。创建任务时必填 |
| `target` | 目标提交份数 |
| `threads` | 并发线程数 |
| `submit_interval` | 提交间隔，单位秒，例如 `[1, 3]` |
| `answer_duration` | 作答耗时，单位秒，例如 `[60, 120]` |
| `answer_datetime_window` | Credamo 见数作答时间窗 |
| `random_ip_enabled` | 是否启用随机 IP |
| `proxy_source` | 代理来源：`default`、`benefit`、`custom` |
| `custom_proxy_api` | 自定义代理 API |
| `proxy_area_code` | 官方随机 IP 地区编码 |
| `random_ip_user_id` | 官方随机 IP 用户 ID |
| `random_ip_device_id` | 官方随机 IP 设备 ID |
| `ip_extract_endpoint` | 官方随机 IP 提取接口 |
| `random_ip_lease_minute` | 官方随机 IP 租期分钟数 |
| `reverse_fill_enabled` | 是否启用反填样本 |
| `reverse_fill_source_path` | 反填样本文件路径 |
| `reverse_fill_format` | 样本格式：`auto`、`wjx_sequence`、`wjx_score`、`wjx_text` |
| `reverse_fill_start_row` | 样本起始数据行 |
| `reverse_fill_threads` | 反填样本并发线程数 |
| `question_entries` | 每道题的答案生成配置 |
| `answer_rules` | 一致性规则 |

示例：

```json
{
  "url": "https://www.wjx.cn/vm/xxxxx.aspx",
  "target": 10,
  "threads": 3,
  "submit_interval": [0, 0],
  "answer_duration": [60, 120],
  "answer_datetime_window": ["", ""],
  "random_ip_enabled": false,
  "proxy_source": "default",
  "reverse_fill_enabled": false,
  "reverse_fill_format": "auto",
  "reverse_fill_start_row": 1,
  "reverse_fill_threads": 1,
  "question_entries": [
    {
      "question_type": "single",
      "probabilities": [0.25, 0.25, 0.25, 0.25],
      "option_count": 4,
      "distribution_mode": "random"
    }
  ],
  "answer_rules": [
    {
      "condition_question_num": 1,
      "condition_mode": "selected",
      "condition_option_indices": [0],
      "target_question_num": 2,
      "action_mode": "must_not_select",
      "target_option_indices": [1]
    }
  ]
}
```

## 数据保存

任务状态和日志默认保存在：

```text
data/tasks/
```

每个任务会生成两个文件：

```text
<task_id>.json
<task_id>.logs.jsonl
```

服务启动时会读取已有任务。

如果上次退出时任务还在 `pending` 或 `running`，重启后会标记为 `interrupted`。

## 日志

终端日志使用结构化格式，包含时间、级别、消息和字段。

默认启用颜色。

关闭颜色：

```bash
NO_COLOR=1 ./surveyconsole
```

Windows PowerShell：

```powershell
$env:NO_COLOR="1"
.\surveyconsole.exe
```

## 开发命令

项目使用 `go.mod` 中声明的 Go 版本。

```bash
go build -o surveyconsole ./cmd/surveycontroller
go test ./...
go test -race ./...
go vet ./...
go mod tidy -diff
git diff --check
```

## 项目结构

```text
SurveyConsole/
├── cmd/surveycontroller/     # HTTP API 服务入口
├── internal/
│   ├── api/                  # API 路由、任务管理、任务存储
│   ├── config/               # 配置读取、默认值、执行配置构建
│   ├── engine/               # 任务执行引擎
│   ├── io/                   # 二维码、Excel 等输入输出能力
│   ├── logging/              # 结构化日志
│   ├── models/               # 数据模型
│   ├── network/              # HTTP 客户端和代理池
│   ├── providers/            # 平台适配器
│   ├── questions/            # 答案生成、分布修正、心理计量
│   └── reversefill/          # 反填样本解析
├── tests/                    # 高层测试
└── configs/                  # 示例配置
```

## License

AGPL-3.0

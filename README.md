# SurveyCore

![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3.43.1-003B57?logo=sqlite&logoColor=white)

[SurveyController](https://github.com/SurveyController/SurveyController) 的核心 HTTP 提交 API 服务。

负责解析问卷、创建提交任务、查询任务、停止任务、读取任务日志和解析二维码。

> [!CAUTION]
>
> 本项目仅可用于已授权问卷的学习与测试。严禁用于污染第三方问卷数据！

## 支持平台

- [x] 问卷星
- [x] 腾讯问卷
- [x] Credamo 见数
- [ ] ...（欢迎贡献！）

## 使用方法

### 环境要求

- Go 1.26.3

如果还没有安装 Go，可以从 [Go 官方网站](https://go.dev/dl/) 下载并安装适合您操作系统的版本。

### 部署与运行

```bash
git clone https://github.com/SurveyController/SurveyCore.git
cd SurveyCore
go mod download
go build -o surveycore ./cmd/surveycore
./surveycore
```

### API 文档

见 https://surveydoc.hungrym0.top/sdk

## 服务地址

默认监听：

```text
127.0.0.1:19178
```

服务会读取 `configs/surveycore.toml`，可配置监听端口、SQLite 路径和服务端私有 AI 默认值：

```toml
[server]
port = 19178

[storage]
db_path = "data/surveycore.db"

[ai]
base_url = "https://api.deepseek.com/v1"
model = "deepseek-chat"
api_key = ""
```

任务请求里的 AI 配置非空时优先使用请求值；请求未提供时，SurveyCore 会把 `[ai]` 中的服务端默认值补到执行配置，便于 Python 桌面端不携带密钥也能调用服务端 AI 填空。

也可以用环境变量覆盖常用配置：

```text
SURVEY_PORT=8080
SURVEYCORE_DB_PATH=data/surveycore.db
AI_BASE_URL=https://api.deepseek.com/v1
AI_MODEL=deepseek-chat
AI_API_KEY=...
```

## 接口列表

| 方法 | 路由 | 作用 |
|---|---|---|
| `GET` | `/api/health` | 健康检查。服务可用时返回正常状态。 |
| `GET` | `/api/version` | 读取当前服务版本号。 |
| `GET` | `/api/tasks` | 查询任务列表。按创建时间倒序返回。 |
| `GET` | `/api/tasks/{id}` | 查询单个任务详情。 |
| `GET` | `/api/tasks/{id}/logs` | 分页读取指定任务日志。支持 `after` 游标和 `limit` 条数参数。 |
| `GET` | `/api/tasks/{id}/config` | 导出指定任务的运行配置 JSON。 |
| `GET` | `/api/tasks/{id}/report` | 导出指定任务报告。默认 JSON，`?format=csv` 导出日志表。 |
| `POST` | `/api/surveys/parse` | 解析问卷链接，返回问卷标题、平台和题目结构。不会提交答案。 |
| `POST` | `/api/configs` | 生成默认运行配置。传入问卷链接时会先解析问卷，再补全题目配置；不传链接时返回空模板。 |
| `POST` | `/api/configs/import` | 导入并标准化 Python/Go 兼容运行配置 JSON。支持直接配置对象或 `{ "config": ... }` 包络，并保留 Python 额外字段。 |
| `POST` | `/api/configs/export` | 导出标准化运行配置 JSON 文件。支持直接配置对象或 `{ "config": ... }` 包络，并回写 Python 额外字段。 |
| `POST` | `/api/tasks` | 创建提交任务。任务异步运行，创建成功只表示已进入任务队列。支持直接配置对象或 `{ "config": ... }` 包络。 |
| `POST` | `/api/tasks/{id}/stop` | 停止指定任务。任务不存在时返回错误。 |
| `POST` | `/api/ai/test` | 测试当前 AI 配置是否可用，成功时返回模型回复预览。 |
| `GET` | `/api/random-ip/session` | 读取本地随机 IP 设备身份、账号和额度快照。 |
| `POST` | `/api/random-ip/trial` | 领取或刷新随机 IP 试用账号，并保存 `device_id`、`user_id` 和额度。 |
| `POST` | `/api/random-ip/quota/sync` | 从服务端同步当前随机 IP 账号额度。 |
| `POST` | `/api/random-ip/redeem` | 兑换随机 IP 额度卡。请求体字段：`card_code`。 |
| `POST` | `/api/random-ip/bonus` | 领取随机 IP 彩蛋额度。 |
| `POST` | `/api/qrcode/decode` | 从二维码图片中解析问卷链接。 |

## 配置兼容

`POST /api/tasks` 使用与 Python 原项目 `SurveyController` 运行配置一致的 JSON 字段。Go 服务会读取已支持字段，并忽略 Python 端内部字段或未来扩展字段，例如 `_ai_config_present`，以保证同一份配置可以从桌面端无损传入核心 API。任务创建既支持直接传运行配置对象，也支持 Python 桌面端常用的 `{ "config": ... }` 包络。

`POST /api/configs/import` 和 `POST /api/configs/export` 同样按兼容模式读取运行配置，适合桌面端在 Python 与 Go 核心之间传递同一份配置 JSON。导入会补齐 Go 默认值；导出会返回 `Content-Disposition` 附件响应。Go 暂不使用但来自 Python 的字段，例如 `_ai_config_present`、`config_schema_version` 或未来扩展字段，会保存在运行配置的额外字段区并在再次导出时回写，避免桌面端配置往返丢字段。

Go 生成或导出的配置默认带 `config_schema_version=6`，并在存在 AI 配置时写入 `_ai_config_present=true`，以便 Python 原项目按当前配置 schema 直接接回。若导入的 Python 配置已经带有这些字段，Go 会优先保留原值。

配置读取会兼容 Python codec 的宽松输入形态：数字字段可接受字符串数字，布尔字段可接受 `true/false`、`1/0`、`yes/no`，`answer_duration` 可接受旧版单值或单元素数组并转换为 Python 一致的上下浮动范围，`answer_datetime_window` 会按 `YYYY-MM-DD HH:MM:SS` 归一化。随机 UA 支持 Python 当前 preset 键 `wechat_android`、`mobile_android`、`pc_web`，同时兼容旧版 Go 键 `wechat`、`mobile`、`pc`。

其他请求包络（例如 `/api/surveys/parse`、`/api/configs`）保持严格 JSON 校验，避免调用方把错误参数静默传入。

## AI 服务

Go 运行配置使用 Python 同名字段：`ai_mode`、`ai_provider`、`ai_api_key`、`ai_base_url`、`ai_api_protocol`、`ai_model`、`ai_system_prompt`。

当前支持：

| 能力 | 状态 |
|---|---|
| 免费 AI 模式 | 支持真实免费 AI 服务链路，使用随机 IP session 的 `user_id` 和 `device_id`；未认证或服务不可用时，任务运行会回退到本地启发式答案。 |
| DeepSeek/OpenAI 兼容 Chat Completions | 支持 `/chat/completions`。 |
| 自定义 AI Base URL | 支持完整 endpoint 或 `/v1` 基础地址。 |
| Responses API | 支持 `/responses`，`ai_api_protocol=responses` 时直接使用。 |
| 自动协议识别 | `ai_api_protocol=auto` 时默认尝试 Chat Completions，遇到 404/405/410 等端点不匹配错误后 fallback 到 Responses。 |
| 连接测试 | `POST /api/ai/test`。 |

免费 AI 端点默认与 Python 原项目一致：`https://api-wjx.hungrym0.top/api/ai/free`。可以用环境变量 `AI_FREE_ENDPOINT` 覆盖，也可以在 `/api/ai/test` 请求体中传入 `ai_free_endpoint` 做连接测试。

## 随机 IP 服务

SurveyCore 会在本地保存随机 IP 会话文件，默认路径为 `data/random_ip_session.json`，也可以用 `SURVEYCORE_RANDOM_IP_SESSION_PATH` 覆盖。该文件包含稳定 `device_id`、`user_id` 和额度快照，供 Python 桌面端或其他本地调用方直接读取 API 状态。

随机 IP 官方端点默认与 Python 原项目一致，并支持环境变量覆盖：

| 环境变量 | 用途 |
|---|---|
| `AUTH_TRIAL_ENDPOINT` | 试用领取和额度同步。 |
| `AUTH_BONUS_CLAIM_ENDPOINT` | 彩蛋额度领取。 |
| `CARD_REDEEM_ENDPOINT` | 额度卡兑换。 |
| `IP_EXTRACT_ENDPOINT` | 代理 IP 提取。 |

创建任务时，如果配置启用了 `random_ip_enabled`，但没有传入 `random_ip_user_id` 或 `random_ip_device_id`，SurveyCore 会优先使用本地随机 IP session 中已认证的账号和设备身份。

## 错误响应

API 错误统一返回稳定错误码、用户消息和调试详情：

```json
{
  "error": "任务配置无效",
  "code": "validation_error",
  "message": "任务配置无效",
  "detail": "url 不能为空"
}
```

常见错误码：

| 错误码 | 含义 |
|---|---|
| `invalid_json` | JSON 请求体格式错误，或包含不被该接口接受的字段。 |
| `invalid_request` | 请求格式不符合接口要求，例如 multipart 表单无效。 |
| `invalid_query` | 查询参数无效，例如日志游标或条数非法。 |
| `validation_error` | 业务参数未通过校验。 |
| `not_found` | 任务或资源不存在。 |
| `upstream_error` | 问卷平台解析、配置生成等上游调用失败。 |
| `ai_config_error` | AI 配置不完整或不支持。 |
| `ai_connection_failed` | AI 连接测试或调用失败。 |
| `random_ip_not_authenticated` | 随机 IP 本地 session 尚未领取试用或认证。 |
| `random_ip_auth_error` | 随机 IP 服务返回账号、额度或请求参数错误。 |
| `random_ip_upstream_error` | 随机 IP 服务网络或上游异常。 |
| `internal_error` | 服务内部错误。 |

## 任务状态

任务详情响应会包含稳定进度和失败字段：

| 字段 | 含义 |
|---|---|
| `progress.current` | 当前已成功提交数。 |
| `progress.target` | 目标提交数。 |
| `progress.success` | 成功提交数。 |
| `progress.fail` | 失败提交数。 |
| `progress.percent` | 完成比例，范围 `0` 到 `1`。 |
| `error_code` | 标准化任务错误码，优先对齐 Python 的失败原因，例如 `proxy_unavailable`、`fill_failed`、`submission_verification_required`、`survey_provider_unavailable`、`device_quota_limit`、`user_stopped`。 |
| `failure_reason` | 失败原因，优先使用运行时终止原因，其次使用错误消息或停止消息。 |
| `terminal_stop_category` | 终止类别，例如 `fail_threshold`、`reverse_fill_exhausted`、`free_ai_unstable`、`submission_verification`、`target_reached`。 |

| 状态 | 含义 |
|---|---|
| `pending` | 已创建，等待运行。 |
| `running` | 正在运行。 |
| `succeeded` | 已完成。 |
| `failed` | 执行失败。 |
| `stopped` | 已停止。 |
| `interrupted` | 服务重启导致中断。 |

## 能力边界

SurveyCore 是本地 HTTP/API 化的核心执行内核。Python 项目继续负责桌面 GUI、安装更新和用户交互；Go 项目负责解析、配置、任务执行、状态、日志和可被桌面端调用的稳定 API。

SurveyCore 不包含 PySide GUI，也不引入 Playwright、Selenium 或浏览器兼容提交层。

## 许可证

Mozilla Public License Version 2.0

本项目依据 `Mozilla Public License Version 2.0`（MPL-2.0）发布。使用、复制、修改或分发本项目时，应遵守 MPL-2.0 条款。

若分发包含本项目源码文件修改内容的版本，需要保留版权和许可证声明，说明必要的变更，并按 MPL-2.0 开源这些修改过的源文件。

与本项目以独立文件组合形成的更大作品，可按自身选择的许可证分发，但不得限制接收者依据 MPL-2.0 取得和使用本项目相关源代码的权利。

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
localhost:19178
```

只能用环境变量修改端口：

```text
SURVEY_PORT=8080
```

## 接口列表

| 方法 | 路由 | 作用 |
|---|---|---|
| `GET` | `/api/health` | 健康检查。服务可用时返回正常状态。 |
| `GET` | `/api/version` | 读取当前服务版本号。 |
| `GET` | `/api/tasks` | 查询任务列表。按创建时间倒序返回。 |
| `GET` | `/api/tasks/{id}` | 查询单个任务详情。 |
| `GET` | `/api/tasks/{id}/logs` | 分页读取指定任务日志。支持 `after` 游标和 `limit` 条数参数。 |
| `POST` | `/api/surveys/parse` | 解析问卷链接，返回问卷标题、平台和题目结构。不会提交答案。 |
| `POST` | `/api/configs` | 生成默认运行配置。传入问卷链接时会先解析问卷，再补全题目配置；不传链接时返回空模板。 |
| `POST` | `/api/tasks` | 创建提交任务。任务异步运行，创建成功只表示已进入任务队列。 |
| `POST` | `/api/tasks/{id}/stop` | 停止指定任务。任务不存在时返回错误。 |
| `POST` | `/api/qrcode/decode` | 从二维码图片中解析问卷链接。 |

## 配置兼容

`POST /api/tasks` 使用与 Python 原项目 `SurveyController` 运行配置一致的 JSON 字段。Go 服务会读取已支持字段，并忽略 Python 端内部字段或未来扩展字段，例如 `_ai_config_present`，以保证同一份配置可以从桌面端无损传入核心 API。

其他请求包络（例如 `/api/surveys/parse`、`/api/configs`）保持严格 JSON 校验，避免调用方把错误参数静默传入。

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
| `internal_error` | 服务内部错误。 |

## 任务状态

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

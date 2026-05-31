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

## 任务状态

| 状态 | 含义 |
|---|---|
| `pending` | 已创建，等待运行。 |
| `running` | 正在运行。 |
| `succeeded` | 已完成。 |
| `failed` | 执行失败。 |
| `stopped` | 已停止。 |
| `interrupted` | 服务重启导致中断。 |

## 许可证

Mozilla Public License Version 2.0

本项目依据 `Mozilla Public License Version 2.0`（MPL-2.0）发布。使用、复制、修改或分发本项目时，应遵守 MPL-2.0 条款。

若分发包含本项目源码文件修改内容的版本，需要保留版权和许可证声明，说明必要的变更，并按 MPL-2.0 开源这些修改过的源文件。

与本项目以独立文件组合形成的更大作品，可按自身选择的许可证分发，但不得限制接收者依据 MPL-2.0 取得和使用本项目相关源代码的权利。

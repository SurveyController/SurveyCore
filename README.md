# SurveyCore
[SurveyController](https://github.com/SurveyController/SurveyController) 的核心 HTTP 提交 API 服务。

负责解析问卷、创建提交任务、查询任务、停止任务、读取任务日志和解析二维码。

本项目依据 GPL-3.0 许可证发布。任何使用、复制、修改、分发或基于本项目构建衍生作品的行为，均应遵守 GPL-3.0 许可证条款，包括但不限于保留版权声明、明确变更内容，并在分发衍生作品时以同等许可证开放相应源代码。

> [!CAUTION]
>
> 本项目仅可用于已授权问卷的学习与测试。严禁用于污染第三方问卷数据！

## 支持平台

- 问卷星
- 腾讯问卷
- Credamo 见数

## 服务地址

默认监听：

```text
127.0.0.1:19178
```

可用环境变量修改：

```text
SURVEYCONSOLE_ADDR=127.0.0.1:8080
```

## 接口列表

| 方法 | 路由 | 作用 |
|---|---|---|
| `GET` | `/api/health` | 健康检查。服务可用时返回正常状态。 |
| `GET` | `/api/version` | 读取当前服务版本号。 |
| `POST` | `/api/surveys/parse` | 解析问卷链接，返回问卷标题、平台和题目结构。不会提交答案。 |
| `POST` | `/api/configs` | 生成默认运行配置。传入问卷链接时会先解析问卷，再补全题目配置；不传链接时返回空模板。 |
| `POST` | `/api/tasks` | 创建提交任务。任务异步运行，创建成功只表示已进入任务队列。 |
| `GET` | `/api/tasks` | 查询任务列表。按创建时间倒序返回。 |
| `GET` | `/api/tasks/{id}` | 查询单个任务详情。 |
| `POST` | `/api/tasks/{id}/stop` | 停止指定任务。任务不存在时返回错误。 |
| `GET` | `/api/tasks/{id}/logs` | 读取指定任务日志。 |
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

## License

GPL-3.0

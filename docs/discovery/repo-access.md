# 仓库访问记录

日期：2026-04-29

## GitHub CLI 偏好

本项目的 GitHub 相关操作应优先使用 GitHub CLI（`gh`）。这一偏好已写入 `CONTRIBUTING.md` 和 `docs/development.md`。

## 已验证账号与仓库

当前 GitHub CLI 账号：

- `LING71671`

目标仓库：

- `https://github.com/hungryM0/SurveyController-go`
- 私有仓库。
- 默认分支配置为 `main`。
- `v0.1` 初始化前仓库为空。

通过 `gh api repos/hungryM0/SurveyController-go` 验证到的权限：

- `pull`: true
- `push`: true
- `triage`: true
- `maintain`: false
- `admin`: false

探索时 `gh repo view hungryM0/SurveyController-go` 返回过 GraphQL 401，但通过 `gh api` 发起的 REST 调用成功。后续如果仓库元数据检查再次遇到该 GraphQL 问题，优先使用 `gh api`。

## 初始化前本地状态

路径：

- `B:\SurveyController\SurveyController-go`

初始本地状态：

- 目录已存在。
- 目录为空。
- 目录不是 git 仓库。

远程仓库同样为空，因此 `v0.1` 允许一次直接提交到 `main` 的引导提交。之后正常开发必须遵循：议题 -> 分支 -> 拉取请求。

## 工具版本

已观察到的本地版本：

- Go: `go1.26.0 windows/amd64`
- Git: `2.51.0.windows.1`
- GitHub CLI: `2.89.0`
- `golangci-lint`：探索时本地未安装。

已验证的 GitHub Actions 标签：

- `actions/checkout@v5`
- `actions/setup-go@v6`
- `actions/upload-artifact@v7`

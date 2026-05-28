# SurveyController-Go

问卷自动化处理工具的 Go 语言重写版本。

## 支持平台

- **问卷星 (WJX)** - 完整支持
- **腾讯问卷 (QQ)** - 完整支持
- **Credamo 见数** - 完整支持

## 功能特性

- 问卷结构自动解析 (3 个平台)
- 多题型答案生成（单选、多选、矩阵、量表、文本、滑块、排序）
- 概率分布配置
- 运行时分布修正 (Distribution Tracking)
- 一致性规则引擎 (Answer Rules)
- 心理计量优化 (Psychometric Plan)
- AI 主观题作答
- 并发提交引擎
- 代理池管理（随机 IP）
- JSON 配置导入/导出
- CLI 命令行界面

## 安装

```bash
cd go-rewrite
go build -o surveycontroller ./cmd/surveycontroller
```

## 使用方法

### 解析问卷

```bash
# 问卷星
./surveycontroller parse -url "https://www.wjx.cn/vm/xxxxx.aspx"

# 腾讯问卷
./surveycontroller parse -url "https://wj.qq.com/s2/123456/abc/"

# Credamo 见数
./surveycontroller parse -url "https://www.credamo.com/s/xxxxx"
```

### 创建配置

```bash
./surveycontroller config -create -url "https://www.wjx.cn/vm/xxxxx.aspx" -output config.json
```

### 运行提交任务

```bash
# 基本用法
./surveycontroller run -url "https://www.wjx.cn/vm/xxxxx.aspx" -target 10 -threads 3

# 使用配置文件
./surveycontroller run -config config.json

# 启用随机 IP
./surveycontroller run -config config.json -random-ip -proxy-source custom -custom-proxy "http://api.example.com/proxy"
```

### 命令行参数

#### `run` 命令

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-config` | 配置文件路径 (JSON) | - |
| `-url` | 问卷链接 | - |
| `-target` | 目标提交份数 | 1 |
| `-threads` | 并发线程数 | 1 |
| `-random-ip` | 启用随机 IP | false |
| `-proxy-source` | 代理源 | default |
| `-custom-proxy` | 自定义代理 API URL | - |
| `-verbose` | 详细日志 | false |

## 配置文件格式

```json
{
  "url": "https://www.wjx.cn/vm/xxxxx.aspx",
  "survey_provider": "wjx",
  "target": 10,
  "threads": 3,
  "submit_interval": [0, 0],
  "answer_duration": [10, 20],
  "random_ip_enabled": false,
  "proxy_source": "default",
  "question_entries": [
    {
      "question_type": "single",
      "probabilities": [0.25, 0.25, 0.25, 0.25],
      "option_count": 4
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

## 项目结构

```
go-rewrite/
├── cmd/surveycontroller/     # CLI 入口
├── internal/
│   ├── config/               # 配置管理
│   ├── models/               # 数据模型
│   ├── providers/            # 问卷平台适配器
│   │   ├── wjx/             # 问卷星
│   │   ├── tencent/         # 腾讯问卷
│   │   └── credamo/         # Credamo 见数
│   ├── engine/               # 执行引擎
│   ├── network/              # 网络层
│   │   ├── httpclient/       # HTTP 客户端
│   │   └── proxy/            # 代理管理
│   ├── questions/            # 答案生成 & 心理计量
│   └── logging/              # 日志
├── tests/                    # 测试 (25 个)
└── configs/                  # 示例配置
```

## 运行测试

```bash
go test ./tests/ -v
```

## 高级功能

### 一致性规则 (Answer Rules)

定义条件规则，当某题选择了特定选项时，自动约束其他题的答案：

```json
{
  "answer_rules": [
    {
      "condition_question_num": 1,
      "condition_mode": "selected",
      "condition_option_indices": [0],
      "target_question_num": 3,
      "action_mode": "must_not_select",
      "target_option_indices": [2, 3]
    }
  ]
}
```

### 心理计量优化 (Psychometric Plan)

使用 IRT 模型生成心理测量学上有效的答案，自动达到目标 Cronbach's Alpha 信度系数。

### 运行时分布修正

根据实际提交的选项分布，动态调整概率以趋近目标分布。

## 与 Python 版本的差异

| 方面 | Python 版本 | Go 版本 |
|------|------------|---------|
| 界面 | PySide6 GUI | CLI |
| 并发 | asyncio + threading | goroutines |
| HTTP | httpx | net/http |
| HTML 解析 | BeautifulSoup | goquery |
| 配置 | JSON | JSON (兼容) |
| 分发 | Python 脚本 | 单二进制 |

## 后续计划

- [ ] Web UI 界面
- [ ] TUI 终端界面
- [ ] Excel 导出
- [ ] 自动更新

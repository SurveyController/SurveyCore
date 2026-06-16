# 配置高风险区

## 1. `QuestionEntry` 相关

- `question_num`
- `provider_question_id`
- `provider_page_id`
- `distribution_mode`
- `custom_weights`
- `option_fill_texts`
- `attached_option_selects`
- `dimension`
- `psycho_bias`

这些字段会一路流到执行配置。只改模型不改 `BuildExecutionConfigWithError`，等于白改。

## 2. 反向填充

- `reverse_fill_enabled`
- `reverse_fill_source_path`
- `reverse_fill_format`
- `reverse_fill_start_row`
- `reverse_fill_threads`

改这里要看 `internal/reversefill/`。

## 3. 信度和量表

- `reliability_mode_enabled`
- `psycho_target_alpha`
- `dimension_groups`
- `dimension`
- `psycho_bias`

改这里要看 `internal/questions/` 和 `internal/config/config.go` 的维度映射。

## 4. API 影响

- `/api/configs` 会直接返回 `RuntimeConfig`
- 文档位于 `docs/sdk/schemas.md`
- README 也有配置样例

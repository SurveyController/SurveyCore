package io

import (
	"fmt"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/xuri/excelize/v2"
)

// ExportRunReport exports a run report to an Excel file.
func ExportRunReport(filePath string, cfg *models.RuntimeConfig, state *models.ExecutionState, questions []models.SurveyQuestionMeta) error {
	f := excelize.NewFile()
	defer f.Close()

	// Sheet 1: Run Summary
	writeSummarySheet(f, cfg, state)

	// Sheet 2: Questions
	writeQuestionsSheet(f, questions)

	// Sheet 3: Thread Progress
	writeThreadProgressSheet(f, state)

	// Sheet 4: Config
	writeConfigSheet(f, cfg)

	return f.SaveAs(filePath)
}

func writeSummarySheet(f *excelize.File, cfg *models.RuntimeConfig, state *models.ExecutionState) {
	sheetName := "运行摘要"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"项目", "值"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	rows := [][]any{
		{"问卷链接", cfg.URL},
		{"问卷标题", cfg.SurveyTitle},
		{"平台", cfg.SurveyProvider},
		{"目标份数", cfg.Target},
		{"线程数", cfg.Threads},
		{"成功数", state.GetCurNum()},
		{"失败数", state.GetCurFail()},
		{"随机IP", boolToYesNo(cfg.RandomIPEnabled)},
		{"代理源", cfg.ProxySource},
		{"导出时间", time.Now().Format("2006-01-02 15:04:05")},
	}

	for i, row := range rows {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(sheetName, cell, val)
		}
	}

	// Set column widths
	f.SetColWidth(sheetName, "A", "A", 15)
	f.SetColWidth(sheetName, "B", "B", 50)
}

func writeQuestionsSheet(f *excelize.File, questions []models.SurveyQuestionMeta) {
	sheetName := "题目列表"
	f.NewSheet(sheetName)

	headers := []string{"题号", "标题", "类型", "选项数", "选项文本", "平台", "跳题逻辑"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for i, q := range questions {
		row := i + 2
		f.SetCellValue(sheetName, cellName(1, row), q.Num)
		f.SetCellValue(sheetName, cellName(2, row), q.Title)
		f.SetCellValue(sheetName, cellName(3, row), typeCodeToName(q.TypeCode))
		f.SetCellValue(sheetName, cellName(4, row), q.Options)
		f.SetCellValue(sheetName, cellName(5, row), joinStrings(q.OptionTexts, ", "))
		f.SetCellValue(sheetName, cellName(6, row), q.Provider)
		f.SetCellValue(sheetName, cellName(7, row), boolToYesNo(q.HasJump))
	}

	f.SetColWidth(sheetName, "A", "A", 8)
	f.SetColWidth(sheetName, "B", "B", 40)
	f.SetColWidth(sheetName, "C", "C", 12)
	f.SetColWidth(sheetName, "D", "D", 8)
	f.SetColWidth(sheetName, "E", "E", 50)
	f.SetColWidth(sheetName, "F", "F", 10)
	f.SetColWidth(sheetName, "G", "G", 10)
}

func writeThreadProgressSheet(f *excelize.File, state *models.ExecutionState) {
	sheetName := "线程进度"
	f.NewSheet(sheetName)

	headers := []string{"线程名", "成功数", "失败数", "当前步骤", "总步骤", "状态", "运行中"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	progress := state.SnapshotThreadProgress()
	for i, p := range progress {
		row := i + 2
		f.SetCellValue(sheetName, cellName(1, row), p["thread_name"])
		f.SetCellValue(sheetName, cellName(2, row), p["success_count"])
		f.SetCellValue(sheetName, cellName(3, row), p["fail_count"])
		f.SetCellValue(sheetName, cellName(4, row), p["step_current"])
		f.SetCellValue(sheetName, cellName(5, row), p["step_total"])
		f.SetCellValue(sheetName, cellName(6, row), p["status_text"])
		f.SetCellValue(sheetName, cellName(7, row), boolToYesNo(p["running"].(bool)))
	}

	f.SetColWidth(sheetName, "A", "A", 15)
	f.SetColWidth(sheetName, "B", "B", 10)
	f.SetColWidth(sheetName, "C", "C", 10)
	f.SetColWidth(sheetName, "D", "D", 10)
	f.SetColWidth(sheetName, "E", "E", 10)
	f.SetColWidth(sheetName, "F", "F", 15)
	f.SetColWidth(sheetName, "G", "G", 10)
}

func writeConfigSheet(f *excelize.File, cfg *models.RuntimeConfig) {
	sheetName := "配置详情"
	f.NewSheet(sheetName)

	headers := []string{"配置项", "值"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	rows := [][]any{
		{"问卷链接", cfg.URL},
		{"平台", cfg.SurveyProvider},
		{"目标份数", cfg.Target},
		{"线程数", cfg.Threads},
		{"提交间隔", fmt.Sprintf("%d-%d秒", cfg.SubmitInterval[0], cfg.SubmitInterval[1])},
		{"答题时长", fmt.Sprintf("%d-%d秒", cfg.AnswerDuration[0], cfg.AnswerDuration[1])},
		{"随机IP", boolToYesNo(cfg.RandomIPEnabled)},
		{"代理源", cfg.ProxySource},
		{"随机UA", boolToYesNo(cfg.RandomUAEnabled)},
		{"失败停止", boolToYesNo(cfg.FailStopEnabled)},
		{"信度模式", boolToYesNo(cfg.ReliabilityModeEnabled)},
		{"目标Alpha", fmt.Sprintf("%.2f", cfg.PsychoTargetAlpha)},
		{"AI模式", cfg.AIMode},
		{"AI模型", cfg.AIProvider},
		{"题目配置数", len(cfg.QuestionEntries)},
	}

	for i, row := range rows {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(sheetName, cell, val)
		}
	}

	f.SetColWidth(sheetName, "A", "A", 15)
	f.SetColWidth(sheetName, "B", "B", 40)
}

// ExportQuestionEntries exports question entries to Excel.
func ExportQuestionEntries(filePath string, entries []models.QuestionEntry, questions []models.SurveyQuestionMeta) error {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "题目配置"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"题号", "标题", "类型", "选项数", "分布模式", "概率", "维度", "AI"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	for i, entry := range entries {
		row := i + 2
		num := 0
		if entry.QuestionNum != nil {
			num = *entry.QuestionNum
		}
		title := ""
		if entry.QuestionTitle != nil {
			title = *entry.QuestionTitle
		}

		f.SetCellValue(sheetName, cellName(1, row), num)
		f.SetCellValue(sheetName, cellName(2, row), title)
		f.SetCellValue(sheetName, cellName(3, row), entry.QuestionType)
		f.SetCellValue(sheetName, cellName(4, row), entry.OptionCount)
		f.SetCellValue(sheetName, cellName(5, row), entry.DistributionMode)
		f.SetCellValue(sheetName, cellName(6, row), formatProbabilities(entry.Probabilities))
		dim := ""
		if entry.Dimension != nil {
			dim = *entry.Dimension
		}
		f.SetCellValue(sheetName, cellName(7, row), dim)
		f.SetCellValue(sheetName, cellName(8, row), boolToYesNo(entry.AIEnabled))
	}

	f.SetColWidth(sheetName, "A", "A", 8)
	f.SetColWidth(sheetName, "B", "B", 40)
	f.SetColWidth(sheetName, "C", "C", 10)
	f.SetColWidth(sheetName, "D", "D", 8)
	f.SetColWidth(sheetName, "E", "E", 10)
	f.SetColWidth(sheetName, "F", "F", 30)
	f.SetColWidth(sheetName, "G", "G", 15)
	f.SetColWidth(sheetName, "H", "H", 8)

	return f.SaveAs(filePath)
}

// Helper functions

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

func boolToYesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func typeCodeToName(code string) string {
	switch code {
	case "0":
		return "说明"
	case "1":
		return "填空"
	case "2":
		return "填空"
	case "3":
		return "单选"
	case "4":
		return "多选"
	case "5":
		return "量表"
	case "6":
		return "矩阵"
	case "7":
		return "下拉"
	case "8":
		return "滑块"
	case "9":
		return "矩阵/填空"
	case "11":
		return "排序"
	case "12":
		return "排序"
	case "33":
		return "单选"
	case "34":
		return "单选"
	case "35":
		return "下拉"
	default:
		return code
	}
}

func formatProbabilities(probs any) string {
	if probs == nil {
		return ""
	}
	switch p := probs.(type) {
	case []float64:
		result := "["
		for i, v := range p {
			if i > 0 {
				result += ", "
			}
			result += fmt.Sprintf("%.2f", v)
		}
		return result + "]"
	case []any:
		result := "["
		for i, v := range p {
			if i > 0 {
				result += ", "
			}
			result += fmt.Sprintf("%v", v)
		}
		return result + "]"
	}
	return fmt.Sprintf("%v", probs)
}

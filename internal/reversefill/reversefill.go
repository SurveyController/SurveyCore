package reversefill

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/xuri/excelize/v2"
)

type column struct {
	index       int
	header      string
	questionNum int
	suffix      string
}

type rawRow struct {
	dataRowNumber      int
	worksheetRowNumber int
	valuesByColumn     map[int]string
}

type exportData struct {
	sourcePath      string
	selectedFormat  string
	detectedFormat  string
	totalDataRows   int
	questionColumns map[int][]column
	rawRows         []rawRow
}

var questionHeaderRe = regexp.MustCompile(`^\s*(\d+)\s*[、,.，．]\s*(.*?)\s*$`)
var sequenceSuffixRe = regexp.MustCompile(`^\(\s*选项\s*\d+\s*\)$`)
var leadingIndexRe = regexp.MustCompile(`^[\(\[（【]?\s*\d+\s*[\)\]）】]?\s*`)
var numberTextRe = regexp.MustCompile(`^\d+(?:\.0+)?$`)

// BuildSpec builds a reverse-fill plan from a WJX Excel/CSV export.
func BuildSpec(cfg *models.RuntimeConfig, questions []models.SurveyQuestionMeta) (*models.ReverseFillSpec, error) {
	if cfg == nil || !cfg.ReverseFillEnabled {
		return nil, nil
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.SurveyProvider))
	if provider == "" {
		provider = models.ProviderWJX
	}
	if provider != models.ProviderWJX {
		return nil, fmt.Errorf("反填目前只支持问卷星")
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("当前还没有解析出问卷题目，无法校验反填")
	}
	export, err := loadSource(cfg.ReverseFillSourcePath, cfg.ReverseFillFormat)
	if err != nil {
		return nil, err
	}

	startRow := cfg.ReverseFillStartRow
	if startRow <= 0 {
		startRow = 1
	}
	totalSamples := export.totalDataRows
	availableSamples := totalSamples - startRow + 1
	if availableSamples < 0 {
		availableSamples = 0
	}
	target := cfg.Target
	if target <= 0 {
		target = availableSamples
	}

	selectedRows := []rawRow{}
	if startRow-1 < len(export.rawRows) {
		selectedRows = export.rawRows[startRow-1:]
	}

	issues := make([]models.ReverseFillIssue, 0)
	plans := make([]models.ReverseFillQuestionPlan, 0)
	answersByRow := make(map[int]map[int]models.ReverseFillAnswer)
	for _, row := range selectedRows {
		answersByRow[row.dataRowNumber] = make(map[int]models.ReverseFillAnswer)
	}

	if availableSamples <= 0 {
		issues = append(issues, models.ReverseFillIssue{
			QuestionNum: 0,
			Title:       "样本数量",
			Severity:    "block",
			Category:    "sample_range",
			Reason:      fmt.Sprintf("起始样本行设为 %d，但数据源只有 %d 行样本", startRow, totalSamples),
			Suggestion:  "请调整反填起始行或更换样本更多的数据源",
		})
	} else if cfg.Target > 0 && cfg.Target > availableSamples {
		issues = append(issues, models.ReverseFillIssue{
			QuestionNum: 0,
			Title:       "样本数量",
			Severity:    "block",
			Category:    "sample_count",
			Reason:      fmt.Sprintf("目标份数为 %d，但从起始样本行开始只剩 %d 行可用样本", cfg.Target, availableSamples),
			Suggestion:  "请降低目标份数，或把起始样本行往前调，或更换样本更多的数据源",
		})
	}

	for _, meta := range sortedQuestions(questions) {
		if meta.IsDescription {
			continue
		}
		questionType := inferQuestionType(meta)
		columns := export.questionColumns[meta.Num]
		fallbackReady := hasQuestionConfig(cfg, meta.Num)
		if !reverseFillSupported(questionType, meta) {
			issues = append(issues, questionIssue(meta, "unsupported_type", "当前题型或题目结构不在反填支持范围内", fallbackReady, nil))
			plans = append(plans, questionPlan(meta, questionType, statusForFallback(fallbackReady), columns, "当前题型或题目结构不在反填支持范围内", fallbackReady))
			continue
		}
		if len(columns) == 0 {
			issues = append(issues, questionIssue(meta, "mapping_missing", "数据源中没有找到这道题对应的列", fallbackReady, nil))
			plans = append(plans, questionPlan(meta, questionType, statusForFallback(fallbackReady), columns, "数据源中没有找到这道题对应的列", fallbackReady))
			continue
		}

		orderedColumns := columns
		if oneColumnType(questionType) && len(columns) != 1 {
			issues = append(issues, questionIssue(meta, "mapping_ambiguous", "这道题在数据源中对应了多列，无法确认唯一答案列", fallbackReady, nil))
			plans = append(plans, questionPlan(meta, questionType, statusForFallback(fallbackReady), columns, "这道题在数据源中对应了多列，无法确认唯一答案列", fallbackReady))
			continue
		}
		if questionType == "matrix" {
			if meta.Rows > 0 && len(columns) != meta.Rows {
				reason := fmt.Sprintf("矩阵题解析出 %d 行，但数据源里有 %d 列", meta.Rows, len(columns))
				issues = append(issues, questionIssue(meta, "mapping_mismatch", reason, fallbackReady, nil))
				plans = append(plans, questionPlan(meta, questionType, statusForFallback(fallbackReady), columns, reason, fallbackReady))
				continue
			}
			orderedColumns = resolveOrderedColumns(columns, meta.RowTexts)
		}
		if questionType == "multi_text" {
			orderedColumns = resolveOrderedColumns(columns, meta.TextInputLabels)
		}

		parseErrorRows := make([]int, 0)
		for _, row := range selectedRows {
			answer, err := parseQuestionAnswer(meta, questionType, orderedColumns, row, export.selectedFormat)
			if err != nil {
				parseErrorRows = append(parseErrorRows, row.dataRowNumber)
				break
			}
			if answer != nil {
				answersByRow[row.dataRowNumber][meta.Num] = *answer
			}
		}
		if len(parseErrorRows) > 0 {
			reason := "这道题在样本中出现了无法稳定回放的值"
			if questionType == "matrix" || questionType == "single" || questionType == "dropdown" || questionType == "scale" || questionType == "score" {
				reason = "这道题在样本中出现了无法匹配选项的值或不支持的复合值"
			}
			issues = append(issues, questionIssue(meta, "unsupported_value", reason, fallbackReady, parseErrorRows))
			plans = append(plans, questionPlan(meta, questionType, statusForFallback(fallbackReady), columns, reason, fallbackReady))
			for _, rowAnswers := range answersByRow {
				delete(rowAnswers, meta.Num)
			}
			continue
		}

		plans = append(plans, questionPlan(meta, questionType, models.ReverseFillStatusReverse, orderedColumns, "来源列："+headersDetail(orderedColumns), false))
	}

	samples := make([]models.ReverseFillSampleRow, 0, len(selectedRows))
	for _, row := range selectedRows {
		samples = append(samples, models.ReverseFillSampleRow{
			DataRowNumber:      row.dataRowNumber,
			WorksheetRowNumber: row.worksheetRowNumber,
			Answers:            answersByRow[row.dataRowNumber],
		})
	}

	spec := &models.ReverseFillSpec{
		SourcePath:       export.sourcePath,
		SelectedFormat:   export.selectedFormat,
		DetectedFormat:   export.detectedFormat,
		StartRow:         startRow,
		TotalSamples:     totalSamples,
		AvailableSamples: availableSamples,
		TargetNum:        target,
		QuestionPlans:    plans,
		Issues:           issues,
		Samples:          samples,
	}
	if blocking := spec.BlockingIssues(); len(blocking) > 0 {
		return spec, fmt.Errorf("%s", formatBlockingMessage(spec))
	}
	return spec, nil
}

func loadSource(sourcePath, preferredFormat string) (*exportData, error) {
	rawPath := strings.TrimSpace(sourcePath)
	if rawPath == "" {
		return nil, fmt.Errorf("未提供反填数据源路径")
	}
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("反填数据源不存在：%s", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return loadXLSX(path, preferredFormat)
	case ".csv":
		return loadCSV(path, preferredFormat)
	default:
		return nil, fmt.Errorf("反填数据源格式不支持：%s", ext)
	}
}

func loadXLSX(path, preferredFormat string) (*exportData, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取反填 Excel 失败: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel 中没有可读取的工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("读取反填工作表失败: %w", err)
	}
	return buildExport(path, rows, preferredFormat)
}

func loadCSV(path, preferredFormat string) (*exportData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取反填 CSV 失败: %w", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析反填 CSV 失败: %w", err)
	}
	return buildExport(path, rows, preferredFormat)
}

func buildExport(path string, rows [][]string, preferredFormat string) (*exportData, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("数据源缺少表头，无法识别问卷列")
	}
	questionColumns := make(map[int][]column)
	for i, header := range rows[0] {
		match := questionHeaderRe.FindStringSubmatch(strings.TrimSpace(header))
		if match == nil {
			continue
		}
		questionNum, _ := strconv.Atoi(match[1])
		questionColumns[questionNum] = append(questionColumns[questionNum], column{
			index:       i + 1,
			header:      strings.TrimSpace(header),
			questionNum: questionNum,
			suffix:      strings.TrimSpace(match[2]),
		})
	}
	if len(questionColumns) == 0 {
		return nil, fmt.Errorf("数据源表头中没有识别到问卷题目列")
	}
	rawRows := make([]rawRow, 0, len(rows)-1)
	for rowIndex, values := range rows[1:] {
		valuesByColumn := make(map[int]string)
		for _, columns := range questionColumns {
			for _, col := range columns {
				value := ""
				if col.index-1 < len(values) {
					value = strings.TrimSpace(values[col.index-1])
				}
				valuesByColumn[col.index] = value
			}
		}
		rawRows = append(rawRows, rawRow{
			dataRowNumber:      rowIndex + 1,
			worksheetRowNumber: rowIndex + 2,
			valuesByColumn:     valuesByColumn,
		})
	}
	detected := detectFormat(questionColumns, rawRows)
	selected := normalizeFormat(preferredFormat)
	if selected == models.ReverseFillFormatAuto {
		selected = detected
	}
	return &exportData{
		sourcePath:      path,
		selectedFormat:  selected,
		detectedFormat:  detected,
		totalDataRows:   len(rawRows),
		questionColumns: questionColumns,
		rawRows:         rawRows,
	}, nil
}

func detectFormat(questionColumns map[int][]column, rawRows []rawRow) string {
	for _, columns := range questionColumns {
		if len(columns) <= 1 {
			continue
		}
		for _, col := range columns {
			if sequenceSuffixRe.MatchString(strings.TrimSpace(col.suffix)) {
				return models.ReverseFillFormatWJXSequence
			}
		}
	}
	hasNumber := false
	hasText := false
	for _, row := range rawRows {
		for _, value := range row.valuesByColumn {
			if value == "" {
				continue
			}
			if numberTextRe.MatchString(value) {
				hasNumber = true
			} else {
				hasText = true
			}
		}
	}
	if hasNumber && !hasText {
		return models.ReverseFillFormatWJXScore
	}
	return models.ReverseFillFormatWJXText
}

func parseQuestionAnswer(meta models.SurveyQuestionMeta, questionType string, columns []column, row rawRow, exportFormat string) (*models.ReverseFillAnswer, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	switch questionType {
	case "single", "dropdown", "scale", "score":
		return parseChoiceAnswer(meta.Num, row.valuesByColumn[columns[0].index], exportFormat, meta.OptionTexts)
	case "text":
		text := normalizeText(row.valuesByColumn[columns[0].index])
		if text == "" {
			return nil, nil
		}
		return &models.ReverseFillAnswer{QuestionNum: meta.Num, Kind: models.ReverseFillKindText, TextValue: text}, nil
	case "multi_text":
		values := make([]string, 0, len(columns))
		hasValue := false
		for _, col := range columns {
			text := normalizeText(row.valuesByColumn[col.index])
			if text != "" {
				hasValue = true
			}
			values = append(values, text)
		}
		if !hasValue {
			return nil, nil
		}
		return &models.ReverseFillAnswer{QuestionNum: meta.Num, Kind: models.ReverseFillKindMultiText, TextValues: values}, nil
	case "matrix":
		values := make([]string, 0, len(columns))
		for _, col := range columns {
			values = append(values, normalizeText(row.valuesByColumn[col.index]))
		}
		if allBlank(values) {
			return nil, nil
		}
		if anyBlank(values) {
			return nil, fmt.Errorf("矩阵题存在部分行为空")
		}
		indices := make([]int, 0, len(values))
		for _, value := range values {
			answer, err := parseChoiceAnswer(meta.Num, value, exportFormat, meta.OptionTexts)
			if err != nil {
				return nil, err
			}
			if answer == nil || answer.ChoiceIndex == nil {
				return nil, fmt.Errorf("矩阵题行值解析失败")
			}
			indices = append(indices, *answer.ChoiceIndex)
		}
		return &models.ReverseFillAnswer{QuestionNum: meta.Num, Kind: models.ReverseFillKindMatrix, MatrixChoiceIndexes: indices}, nil
	default:
		return nil, nil
	}
}

func parseChoiceAnswer(questionNum int, rawValue string, exportFormat string, optionTexts []string) (*models.ReverseFillAnswer, error) {
	text := normalizeText(rawValue)
	if text == "" {
		return nil, nil
	}
	if strings.Contains(text, "┋") || strings.Contains(text, "→") || (strings.Contains(text, "〖") && strings.Contains(text, "〗")) {
		return nil, fmt.Errorf("检测到不支持的复合值")
	}
	if exportFormat == models.ReverseFillFormatWJXSequence {
		index, ok := parseOneBasedIndex(text)
		if !ok {
			return nil, fmt.Errorf("无法把值 %q 解析为序号", text)
		}
		zeroBased := index - 1
		if zeroBased < 0 || zeroBased >= len(optionTexts) {
			return nil, fmt.Errorf("序号 %d 超出选项范围", index)
		}
		return choiceAnswer(questionNum, zeroBased), nil
	}
	optionMap := optionTextIndexMap(optionTexts)
	for _, variant := range labelVariants(text) {
		if idx, ok := optionMap[variant]; ok {
			return choiceAnswer(questionNum, idx), nil
		}
	}
	if exportFormat == models.ReverseFillFormatWJXScore || exportFormat == models.ReverseFillFormatWJXText {
		if index, ok := parseOneBasedIndex(text); ok {
			zeroBased := index - 1
			if zeroBased >= 0 && zeroBased < len(optionTexts) {
				return choiceAnswer(questionNum, zeroBased), nil
			}
		}
	}
	return nil, fmt.Errorf("无法把值 %q 匹配到题目选项", text)
}

func choiceAnswer(questionNum int, idx int) *models.ReverseFillAnswer {
	return &models.ReverseFillAnswer{QuestionNum: questionNum, Kind: models.ReverseFillKindChoice, ChoiceIndex: &idx}
}

func sortedQuestions(questions []models.SurveyQuestionMeta) []models.SurveyQuestionMeta {
	result := append([]models.SurveyQuestionMeta{}, questions...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Page == result[j].Page {
			return result[i].Num < result[j].Num
		}
		return result[i].Page < result[j].Page
	})
	return result
}

func inferQuestionType(meta models.SurveyQuestionMeta) string {
	typeCode := strings.TrimSpace(meta.TypeCode)
	if meta.Provider == models.ProviderWJX {
		if meta.IsMultiText || (meta.IsTextLike && meta.TextInputCount > 1) {
			return "multi_text"
		}
		if meta.IsTextLike || typeCode == "1" || typeCode == "2" {
			return "text"
		}
		switch typeCode {
		case "3", "33", "34":
			return "single"
		case "4":
			return "multiple"
		case "5":
			if meta.IsRating {
				return "score"
			}
			return "scale"
		case "6", "9":
			return "matrix"
		case "7", "35":
			return "dropdown"
		case "8":
			return "slider"
		case "11", "12":
			return "order"
		}
	}
	switch typeCode {
	case "1", "2":
		if meta.IsTextLike {
			return "text"
		}
		return "single"
	case "3":
		return "single"
	case "4":
		return "multiple"
	case "5":
		if meta.IsRating {
			return "score"
		}
		return "scale"
	case "6":
		return "matrix"
	case "7":
		return "dropdown"
	case "8", "9":
		if meta.TextInputCount > 1 || meta.IsMultiText {
			return "multi_text"
		}
		return "text"
	case "35":
		return "dropdown"
	case "11":
		return "slider"
	case "12":
		return "order"
	default:
		if meta.IsTextLike {
			return "text"
		}
		return "single"
	}
}

func reverseFillSupported(questionType string, meta models.SurveyQuestionMeta) bool {
	switch questionType {
	case "single", "dropdown", "scale", "score", "text", "multi_text", "matrix":
		return !meta.IsLocation
	default:
		return false
	}
}

func oneColumnType(questionType string) bool {
	return questionType == "single" || questionType == "dropdown" || questionType == "scale" || questionType == "score" || questionType == "text"
}

func statusForFallback(fallbackReady bool) string {
	if fallbackReady {
		return models.ReverseFillStatusFallback
	}
	return models.ReverseFillStatusBlocked
}

func questionIssue(meta models.SurveyQuestionMeta, category, reason string, fallbackReady bool, rows []int) models.ReverseFillIssue {
	severity := "block"
	suggestion := "请补充这道题的常规答题配置，或调整反填数据源"
	if fallbackReady {
		severity = "warn"
		suggestion = "执行时会回退到常规答题配置"
	}
	return models.ReverseFillIssue{
		QuestionNum: meta.Num,
		Title:       meta.Title,
		Severity:    severity,
		Category:    category,
		Reason:      reason,
		Suggestion:  suggestion,
		SampleRows:  rows,
	}
}

func questionPlan(meta models.SurveyQuestionMeta, questionType, status string, columns []column, detail string, fallbackReady bool) models.ReverseFillQuestionPlan {
	headers := make([]string, 0, len(columns))
	for _, col := range columns {
		headers = append(headers, col.header)
	}
	return models.ReverseFillQuestionPlan{
		QuestionNum:   meta.Num,
		Title:         meta.Title,
		QuestionType:  questionType,
		Status:        status,
		ColumnHeaders: headers,
		Detail:        detail,
		FallbackReady: fallbackReady,
	}
}

func hasQuestionConfig(cfg *models.RuntimeConfig, questionNum int) bool {
	for _, entry := range cfg.QuestionEntries {
		if entry.QuestionNum != nil && *entry.QuestionNum == questionNum {
			return true
		}
	}
	return false
}

func resolveOrderedColumns(columns []column, labels []string) []column {
	ordered := append([]column{}, columns...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	if len(ordered) == 0 || len(labels) == 0 || len(ordered) != len(labels) {
		return ordered
	}
	labelIndex := make(map[string]int)
	for i, label := range labels {
		for _, variant := range labelVariants(label) {
			if _, exists := labelIndex[variant]; !exists {
				labelIndex[variant] = i
			}
		}
	}
	resolved := make([]column, len(labels))
	used := make(map[int]bool)
	for _, col := range ordered {
		matched := false
		for _, variant := range labelVariants(col.suffix) {
			idx, ok := labelIndex[variant]
			if !ok || used[idx] {
				continue
			}
			resolved[idx] = col
			used[idx] = true
			matched = true
			break
		}
		if !matched {
			return ordered
		}
	}
	if len(used) != len(labels) {
		return ordered
	}
	return resolved
}

func headersDetail(columns []column) string {
	parts := make([]string, 0, len(columns))
	for _, col := range columns {
		parts = append(parts, col.header)
	}
	return strings.Join(parts, ", ")
}

func normalizeFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case models.ReverseFillFormatWJXSequence, models.ReverseFillFormatWJXScore, models.ReverseFillFormatWJXText:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return models.ReverseFillFormatAuto
	}
}

func normalizeText(value string) string {
	return strings.TrimSpace(value)
}

func normalizeKey(value string) string {
	text := normalizeText(value)
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer("（", "(", "）", ")", "【", "[", "】", "]", "—", "-", "–", "-", "－", "-", "：", ":")
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), "")
	return strings.ToLower(text)
}

func labelVariants(value string) []string {
	text := normalizeText(value)
	if text == "" {
		return nil
	}
	variants := make([]string, 0, 4)
	appendVariant := func(candidate string) {
		normalized := normalizeKey(candidate)
		if normalized == "" {
			return
		}
		for _, existing := range variants {
			if existing == normalized {
				return
			}
		}
		variants = append(variants, normalized)
	}
	appendVariant(text)
	stripped := strings.Trim(leadingIndexRe.ReplaceAllString(text, ""), " _")
	appendVariant(stripped)
	for _, sep := range []string{"-", ":", "丨", "|", "/", "／"} {
		if strings.Contains(stripped, sep) {
			parts := strings.Split(stripped, sep)
			appendVariant(strings.TrimSpace(parts[len(parts)-1]))
		}
	}
	return variants
}

func optionTextIndexMap(optionTexts []string) map[string]int {
	mapping := make(map[string]int)
	for i, option := range optionTexts {
		for _, variant := range labelVariants(option) {
			if _, exists := mapping[variant]; !exists {
				mapping[variant] = i
			}
		}
	}
	return mapping
}

func parseOneBasedIndex(value string) (int, bool) {
	text := normalizeText(value)
	if !numberTextRe.MatchString(text) {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	index := int(parsed)
	return index, index > 0
}

func allBlank(values []string) bool {
	for _, value := range values {
		if normalizeText(value) != "" {
			return false
		}
	}
	return true
}

func anyBlank(values []string) bool {
	for _, value := range values {
		if normalizeText(value) == "" {
			return true
		}
	}
	return false
}

func formatBlockingMessage(spec *models.ReverseFillSpec) string {
	issues := spec.BlockingIssues()
	if len(issues) == 0 {
		return ""
	}
	lines := []string{"反填配置校验失败："}
	for i, issue := range issues {
		if i >= 12 {
			lines = append(lines, fmt.Sprintf("  - 其余 %d 个阻塞项已省略", len(issues)-12))
			break
		}
		prefix := "样本数量"
		if issue.QuestionNum > 0 {
			prefix = fmt.Sprintf("第 %d 题", issue.QuestionNum)
		}
		lines = append(lines, fmt.Sprintf("  - %s：%s", prefix, issue.Reason))
		if issue.Suggestion != "" {
			lines = append(lines, "    "+issue.Suggestion)
		}
	}
	return strings.Join(lines, "\n")
}

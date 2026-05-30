package wjx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SurveyController/SurveyConsole/internal/models"
)

var (
	pausedSurveyIDRe  = regexp.MustCompile(`此问卷[（(]\d+[）)]已暂停`)
	notOpenTimeRe     = regexp.MustCompile(`此问卷将于\s*(\d{4}[-/]\d{1,2}[-/]\d{1,2}\s+\d{1,2}:\d{2})\s*开放`)
	leadingTitleNumRe = regexp.MustCompile(`^\s*\*?\s*(\d+)\s*[.．、]\s*(.*)$`)
	forcedLetterRe    = regexp.MustCompile(`(?i)(?:请|务必|直接)?(?:选择|选|勾选)\s*([A-Z])\s*(?:项|选项)?`)
	forcedIndexRe     = regexp.MustCompile(`(?:请|务必|直接)?(?:选择|选|勾选)?\s*第\s*(\d+)\s*(?:项|个)?`)
	digitRe           = regexp.MustCompile(`\d+`)
)

// Survey page state error types
type SurveyPausedError struct{ Msg string }

func (e *SurveyPausedError) Error() string { return e.Msg }

type SurveyStoppedError struct{ Msg string }

func (e *SurveyStoppedError) Error() string { return e.Msg }

type SurveyNotOpenError struct{ Msg string }

func (e *SurveyNotOpenError) Error() string { return e.Msg }

// checkPageStateErrors checks if the HTML indicates a paused/stopped/not-open survey.
func checkPageStateErrors(html string) error {
	if isPausedSurveyPage(html) {
		return &SurveyPausedError{"问卷已暂停，需要前往问卷星后台重新发布"}
	}
	if isStoppedSurveyPage(html) {
		return &SurveyStoppedError{"问卷已停止，无法作答"}
	}
	if msg := buildNotOpenMessage(html); msg != "" {
		return &SurveyNotOpenError{msg}
	}
	return nil
}

func isPausedSurveyPage(html string) bool {
	text := normalizeHTMLText(html)
	if text == "" || !strings.Contains(text, "已暂停") {
		return false
	}
	return strings.Contains(text, "不能填写") || strings.Contains(text, "问卷已暂停") || pausedSurveyIDRe.MatchString(text)
}

func isStoppedSurveyPage(html string) bool {
	text := normalizeHTMLText(html)
	if text == "" {
		return false
	}
	normalized := strings.ReplaceAll(text, " ", "")
	return strings.Contains(normalized, "此问卷处于停止状态，无法作答")
}

func buildNotOpenMessage(html string) string {
	text := normalizeHTMLText(html)
	if text == "" {
		return ""
	}
	normalized := strings.ReplaceAll(text, " ", "")
	keywords := []string{"此问卷将于", "请到时再进入此页面进行填写", "距离开始还有", "尚未开始", "未到开始时间", "未开放", "开放时间"}
	found := false
	for _, kw := range keywords {
		if strings.Contains(normalized, kw) {
			found = true
			break
		}
	}
	if !found {
		return ""
	}
	if match := notOpenTimeRe.FindStringSubmatch(text); match != nil {
		openTime := strings.ReplaceAll(match[1], "/", "-")
		return fmt.Sprintf("该问卷暂未开放，无法解析，开放时间：%s", openTime)
	}
	return "该问卷暂未开放，无法解析"
}

// ParseHTML parses a WJX survey HTML page into question metadata and title.
func ParseHTML(html string) ([]models.SurveyQuestionMeta, string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, "", fmt.Errorf("解析 HTML 失败: %w", err)
	}

	title := extractSurveyTitle(doc)
	questions := parseQuestions(doc, html)

	return questions, title, nil
}

func extractSurveyTitle(doc *goquery.Document) string {
	// Try common WJX title selectors
	for _, sel := range []string{".htitle", "#htitle", "h1", ".surveyTitle"} {
		if text := strings.TrimSpace(doc.Find(sel).First().Text()); text != "" {
			return text
		}
	}
	return doc.Find("title").Text()
}

func parseQuestions(doc *goquery.Document, html string) []models.SurveyQuestionMeta {
	var questions []models.SurveyQuestionMeta
	seen := make(map[int]bool)

	doc.Find("div[topic], div[id^='div'][type]").Each(func(i int, s *goquery.Selection) {
		q := extractQuestion(s, i+1)
		if q != nil {
			if seen[q.Num] {
				return
			}
			seen[q.Num] = true
			questions = append(questions, *q)
		}
	})

	// Fallback: look for question divs with specific patterns
	if len(questions) == 0 {
		doc.Find("div[id^='div']").Each(func(i int, s *goquery.Selection) {
			id, _ := s.Attr("id")
			if strings.HasPrefix(id, "div") && len(id) > 3 {
				q := extractQuestionFromDiv(s, i+1)
				if q != nil {
					questions = append(questions, *q)
				}
			}
		})
	}

	return questions
}

func extractQuestion(s *goquery.Selection, defaultNum int) *models.SurveyQuestionMeta {
	num := defaultNum
	if n, ok := s.Attr("topic"); ok {
		if v, err := strconv.Atoi(n); err == nil {
			num = v
		}
	}

	rawTitle := extractQuestionTitle(s)
	title := cleanupQuestionTitle(rawTitle)
	displayNum := extractDisplayQuestionNum(rawTitle)
	typeCode := extractTypeCode(s)
	optionTexts := extractOptionTexts(s)
	if typeCode == "6" || (typeCode == "9" && !looksLikeTextQuestion(s)) {
		optionTexts = extractMatrixOptionTexts(s, num)
	}

	q := &models.SurveyQuestionMeta{
		Num:         num,
		Title:       title,
		DisplayNum:  displayNum,
		TypeCode:    typeCode,
		Options:     len(optionTexts),
		OptionTexts: optionTexts,
		Provider:    ProviderName,
		Page:        1,
	}

	// Detect rating questions
	if isRatingQuestion(s) {
		q.IsRating = true
		q.RatingMax = q.Options
	}

	// Detect text-like questions
	if isTextQuestion(s, typeCode) {
		q.IsTextLike = true
	}
	if q.IsTextLike {
		q.TextInputLabels = extractTextInputLabels(s, num)
		q.TextInputCount = len(q.TextInputLabels)
		if q.TextInputCount <= 0 {
			q.TextInputCount = countTextInputs(s, num)
		}
		if q.TextInputCount > 1 {
			q.IsMultiText = true
		}
	}
	if (typeCode == "3" || typeCode == "4") && q.Options == 0 && !q.IsTextLike {
		q.IsDescription = true
	}

	// Detect matrix questions
	if typeCode == "6" || (typeCode == "9" && !q.IsTextLike) {
		q.RowTexts = extractRowTexts(s)
		q.Rows = len(q.RowTexts)
		if q.Rows == 0 {
			q.Rows = 1
		}
	}

	// Extract forced option
	if forcedIdx, forcedText := extractForcedOption(s, title, optionTexts); forcedIdx >= 0 {
		q.ForcedOptionIndex = &forcedIdx
		q.ForcedOptionText = forcedText
	}

	if hasJump, jumpRules := extractJumpRules(s, optionTexts); hasJump {
		q.HasJump = true
		q.JumpRules = jumpRules
		q.LogicParseStatus = models.LogicParseStatusComplete
	}

	// Detect slider
	if sliderMin, sliderMax, sliderStep := extractSliderRange(s); sliderMin != nil {
		q.SliderMin = sliderMin
		q.SliderMax = sliderMax
		q.SliderStep = sliderStep
		if q.Options <= 0 {
			q.Options = 1
		}
	}

	return q
}

func extractQuestionFromDiv(s *goquery.Selection, defaultNum int) *models.SurveyQuestionMeta {
	title := strings.TrimSpace(s.Find(".topichtml, .field-label").First().Text())
	if title == "" {
		return nil
	}
	return &models.SurveyQuestionMeta{
		Num:      defaultNum,
		Title:    title,
		TypeCode: "1",
		Options:  0,
		Provider: ProviderName,
		Page:     1,
	}
}

func extractQuestionTitle(s *goquery.Selection) string {
	for _, sel := range []string{".topichtml", ".field-label", "legend", "label"} {
		if text := strings.TrimSpace(s.Find(sel).First().Text()); text != "" {
			return text
		}
	}
	return strings.TrimSpace(s.Find("span").First().Text())
}

func cleanupQuestionTitle(title string) string {
	title = strings.TrimSpace(title)
	if match := leadingTitleNumRe.FindStringSubmatch(title); match != nil {
		return strings.TrimSpace(match[2])
	}
	return title
}

func extractDisplayQuestionNum(title string) *int {
	if match := leadingTitleNumRe.FindStringSubmatch(strings.TrimSpace(title)); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			return &value
		}
	}
	return nil
}

func extractTypeCode(s *goquery.Selection) string {
	// Check type attribute
	if t, ok := s.Attr("type"); ok {
		return t
	}
	// Check class for type hints
	class, _ := s.Attr("class")
	if strings.Contains(class, "matrix") {
		return "6"
	}
	if strings.Contains(class, "slider") {
		return "8"
	}
	// Detect by structure
	if s.Find("textarea").Length() > 0 {
		return "1"
	}
	if s.Find("select").Length() > 0 {
		return "7"
	}
	if s.Find("input[type='range'], .slider, .rangeslider").Length() > 0 {
		return "8"
	}
	if s.Find("input[type='radio']").Length() > 0 {
		return "3"
	}
	if s.Find("input[type='checkbox']").Length() > 0 {
		return "4"
	}
	return "1"
}

func extractOptions(s *goquery.Selection) int {
	radioCount := s.Find("input[type='radio']").Length()
	checkboxCount := s.Find("input[type='checkbox']").Length()
	selectCount := s.Find("select option").Length()
	if radioCount > 0 {
		return radioCount
	}
	if checkboxCount > 0 {
		return checkboxCount
	}
	if selectCount > 0 {
		return selectCount
	}
	return 0
}

func extractOptionTexts(s *goquery.Selection) []string {
	var texts []string
	s.Find(".label, label, option").Each(func(i int, opt *goquery.Selection) {
		text := strings.TrimSpace(opt.Text())
		if text != "" && text != "请选择" {
			texts = append(texts, text)
		}
	})
	return texts
}

func extractMatrixOptionTexts(s *goquery.Selection, questionNum int) []string {
	var texts []string
	s.Find("table tr").EachWithBreak(func(i int, row *goquery.Selection) bool {
		if _, hasRowIndex := row.Attr("rowindex"); hasRowIndex {
			return true
		}
		cells := row.Find("th,td")
		if cells.Length() <= 1 {
			return true
		}
		firstText := strings.TrimSpace(cells.First().Text())
		if firstText != "" {
			return true
		}
		cells.Each(func(cellIndex int, cell *goquery.Selection) {
			if cellIndex == 0 {
				return
			}
			text := strings.TrimSpace(cell.Text())
			if text != "" {
				texts = append(texts, text)
			}
		})
		return len(texts) == 0
	})
	if len(texts) > 0 {
		return dedupeNonEmpty(texts)
	}

	maxColumn := 0
	nameRe := regexp.MustCompile(fmt.Sprintf(`^q%d_\d+_(\d+)$`, questionNum))
	s.Find("input[name]").Each(func(i int, input *goquery.Selection) {
		name, _ := input.Attr("name")
		match := nameRe.FindStringSubmatch(strings.TrimSpace(name))
		if match == nil {
			return
		}
		col, err := strconv.Atoi(match[1])
		if err == nil && col > maxColumn {
			maxColumn = col
		}
	})
	for i := 1; i <= maxColumn; i++ {
		texts = append(texts, strconv.Itoa(i))
	}
	return texts
}

func extractRowTexts(s *goquery.Selection) []string {
	var rows []string
	s.Find("tr[rowindex], .matrix-row").Each(func(i int, row *goquery.Selection) {
		firstCell := row.Find("td:first-child, th:first-child, .row-label").First()
		text := strings.TrimSpace(firstCell.Text())
		if text == "" {
			if title, ok := firstCell.Attr("data-title"); ok {
				text = strings.TrimSpace(title)
			}
		}
		if text != "" {
			rows = append(rows, text)
		}
	})
	if len(rows) == 0 {
		s.Find(".itemTitleSpan").Each(func(i int, item *goquery.Selection) {
			if text := strings.TrimSpace(item.Text()); text != "" {
				rows = append(rows, text)
			}
		})
	}
	return rows
}

func isRatingQuestion(s *goquery.Selection) bool {
	class, _ := s.Attr("class")
	return strings.Contains(class, "rating") || s.Find(".rating").Length() > 0
}

func isTextQuestion(s *goquery.Selection, typeCode string) bool {
	if typeCode == "1" || typeCode == "2" {
		return true
	}
	return looksLikeTextQuestion(s) && typeCode != "8"
}

func looksLikeTextQuestion(s *goquery.Selection) bool {
	return s.Find("textarea, input[type='text'], .textEdit, [contenteditable='true']").Length() > 0
}

func countTextInputs(s *goquery.Selection, questionNum int) int {
	if questionNum > 0 {
		nameRe := regexp.MustCompile(fmt.Sprintf(`^q%d_\d+$`, questionNum))
		seen := make(map[string]bool)
		s.Find("input[name]").Each(func(i int, input *goquery.Selection) {
			name, _ := input.Attr("name")
			name = strings.TrimSpace(name)
			if nameRe.MatchString(name) {
				seen[name] = true
			}
		})
		if len(seen) > 0 {
			return len(seen)
		}
	}
	count := s.Find("textarea, input[type='text']").Length()
	if count <= 0 && looksLikeTextQuestion(s) {
		return 1
	}
	return count
}

func extractTextInputLabels(s *goquery.Selection, questionNum int) []string {
	if questionNum <= 0 {
		return nil
	}
	html, err := s.Html()
	if err != nil || strings.TrimSpace(html) == "" {
		return nil
	}
	inputRe := regexp.MustCompile(fmt.Sprintf(`(?is)<input\b[^>]*\bname=['"]q%d_(\d+)['"][^>]*>`, questionNum))
	matches := inputRe.FindAllStringSubmatchIndex(html, -1)
	if len(matches) == 0 {
		return nil
	}

	labels := make([]string, 0, len(matches))
	prevEnd := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		segment := html[prevEnd:match[0]]
		label := cleanupTextInputLabel(segment)
		labels = append(labels, label)
		prevEnd = match[1]
	}
	for len(labels) > 0 && labels[0] == "" {
		labels[0] = cleanupQuestionTitle(s.Find(".topichtml, .field-label").First().Text())
		break
	}
	return labels
}

func cleanupTextInputLabel(html string) string {
	text := normalizeHTMLText(html)
	text = strings.ReplaceAll(text, "*", " ")
	text = regexp.MustCompile(`^\s*\d+[.．、]\s*`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func extractJumpRules(s *goquery.Selection, optionTexts []string) (bool, []map[string]any) {
	hasJumpAttr := strings.TrimSpace(attrOrEmpty(s, "hasjump")) == "1"
	var rules []map[string]any
	optionIndex := 0

	s.Find("input[type='radio'], input[type='checkbox']").Each(func(i int, input *goquery.Selection) {
		jumptoRaw := firstAttr(input, "jumpto", "data-jumpto")
		if jumptoRaw == "" {
			optionIndex++
			return
		}
		target := parseJumpTarget(jumptoRaw)
		if target > 0 {
			optionText := ""
			if optionIndex >= 0 && optionIndex < len(optionTexts) {
				optionText = optionTexts[optionIndex]
			}
			rules = append(rules, map[string]any{
				"option_index":      optionIndex,
				"jumpto":            target,
				"option_text":       optionText,
				"terminates_survey": jumpTargetTerminates(target, optionText),
			})
		}
		optionIndex++
	})

	if hasJumpAttr {
		if target := parseJumpTarget(firstAttr(s, "jumpto", "data-jumpto", "goto", "data-goto", "anyjump", "data-anyjump")); target > 0 {
			duplicate := false
			for _, rule := range rules {
				if intFromRule(rule, "option_index") < 0 && intFromRule(rule, "jumpto") == target {
					duplicate = true
					break
				}
			}
			if !duplicate {
				rules = append(rules, map[string]any{
					"option_index": -1,
					"jumpto":       target,
				})
			}
		}
	}
	return hasJumpAttr || len(rules) > 0, rules
}

func parseJumpTarget(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	match := digitRe.FindString(value)
	if match == "" {
		return 0
	}
	parsed, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return parsed
}

func jumpTargetTerminates(target int, optionText string) bool {
	if target == 1 || target == -1 {
		return true
	}
	for _, keyword := range []string{"结束作答", "结束答题", "结束填写", "终止作答", "停止作答"} {
		if strings.Contains(optionText, keyword) {
			return true
		}
	}
	return false
}

func firstAttr(s *goquery.Selection, names ...string) string {
	for _, name := range names {
		if value, ok := s.Attr(name); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func attrOrEmpty(s *goquery.Selection, name string) string {
	value, _ := s.Attr(name)
	return value
}

func intFromRule(rule map[string]any, key string) int {
	switch v := rule[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(v))
		return parsed
	default:
		return 0
	}
}

func dedupeNonEmpty(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func extractForcedOption(s *goquery.Selection, title string, optionTexts []string) (int, string) {
	// Check for data-forced attribute
	if forced, ok := s.Attr("data-forced"); ok {
		if idx, err := strconv.Atoi(forced); err == nil {
			return clampOptionIndex(idx, optionTexts), optionTextAt(optionTexts, clampOptionIndex(idx, optionTexts))
		}
	}
	normalizedTitle := normalizeForceText(title)
	for idx, option := range optionTexts {
		normalizedOption := normalizeForceText(option)
		if normalizedOption == "" {
			continue
		}
		if strings.Contains(normalizedTitle, "请选择"+normalizedOption) ||
			strings.Contains(normalizedTitle, "选择"+normalizedOption) ||
			strings.Contains(normalizedTitle, "选"+normalizedOption) ||
			strings.Contains(normalizedTitle, "勾选"+normalizedOption) {
			return idx, strings.TrimSpace(option)
		}
	}
	if match := forcedLetterRe.FindStringSubmatch(title); match != nil {
		letter := strings.ToUpper(match[1])
		for idx, option := range optionTexts {
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(option)), letter) ||
				strings.HasPrefix(strings.ToUpper(strings.TrimSpace(option)), "("+letter+")") ||
				strings.HasPrefix(strings.ToUpper(strings.TrimSpace(option)), letter+".") {
				return idx, strings.TrimSpace(option)
			}
		}
	}
	if match := forcedIndexRe.FindStringSubmatch(title); match != nil {
		if oneBased, err := strconv.Atoi(match[1]); err == nil {
			idx := oneBased - 1
			if idx >= 0 && idx < len(optionTexts) {
				return idx, strings.TrimSpace(optionTexts[idx])
			}
		}
	}
	return -1, ""
}

func normalizeForceText(value string) string {
	replacer := strings.NewReplacer("（", "(", "）", ")", "【", "[", "】", "]", "。", "", "，", "", ",", "", ".", "")
	text := replacer.Replace(strings.TrimSpace(value))
	return strings.ToLower(strings.Join(strings.Fields(text), ""))
}

func clampOptionIndex(idx int, optionTexts []string) int {
	if idx < 0 {
		return 0
	}
	if len(optionTexts) > 0 && idx >= len(optionTexts) {
		return len(optionTexts) - 1
	}
	return idx
}

func optionTextAt(optionTexts []string, idx int) string {
	if idx >= 0 && idx < len(optionTexts) {
		return strings.TrimSpace(optionTexts[idx])
	}
	return ""
}

func extractSliderRange(s *goquery.Selection) (min, max, step *float64) {
	slider := s.Find("input[type='range'], .slider")
	if slider.Length() == 0 {
		return nil, nil, nil
	}
	if minStr, ok := slider.Attr("min"); ok {
		if v, err := strconv.ParseFloat(minStr, 64); err == nil {
			min = &v
		}
	}
	if maxStr, ok := slider.Attr("max"); ok {
		if v, err := strconv.ParseFloat(maxStr, 64); err == nil {
			max = &v
		}
	}
	if stepStr, ok := slider.Attr("step"); ok {
		if v, err := strconv.ParseFloat(stepStr, 64); err == nil {
			step = &v
		}
	}
	return
}

// normalizeHTMLText strips HTML tags and normalizes whitespace.
func normalizeHTMLText(html string) string {
	// Simple tag stripping
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, " ")
	// Normalize whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

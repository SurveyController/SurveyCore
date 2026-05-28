package wjx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SurveyController/SurveyController-Go/internal/models"
)

var (
	pausedSurveyIDRe = regexp.MustCompile(`此问卷[（(]\d+[）)]已暂停`)
	notOpenTimeRe    = regexp.MustCompile(`此问卷将于\s*(\d{4}[-/]\d{1,2}[-/]\d{1,2}\s+\d{1,2}:\d{2})\s*开放`)
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

	doc.Find("fieldset, div[topic], div[type]").Each(func(i int, s *goquery.Selection) {
		q := extractQuestion(s, i+1)
		if q != nil {
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

	title := extractQuestionTitle(s)
	typeCode := extractTypeCode(s)
	optionTexts := extractOptionTexts(s)

	q := &models.SurveyQuestionMeta{
		Num:         num,
		Title:       title,
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
	if isTextQuestion(typeCode) {
		q.IsTextLike = true
	}

	// Detect matrix questions
	if typeCode == "6" {
		q.RowTexts = extractRowTexts(s)
		q.Rows = len(q.RowTexts)
		if q.Rows == 0 {
			q.Rows = 1
		}
	}

	// Extract forced option
	if forcedIdx := extractForcedOption(s); forcedIdx >= 0 {
		q.ForcedOptionIndex = &forcedIdx
	}

	// Detect slider
	if sliderMin, sliderMax, sliderStep := extractSliderRange(s); sliderMin != nil {
		q.SliderMin = sliderMin
		q.SliderMax = sliderMax
		q.SliderStep = sliderStep
	}

	return q
}

func extractQuestionFromDiv(s *goquery.Selection, defaultNum int) *models.SurveyQuestionMeta {
	title := strings.TrimSpace(s.Find(".topichtml, .field-label").First().Text())
	if title == "" {
		return nil
	}
	return &models.SurveyQuestionMeta{
		Num:         defaultNum,
		Title:       title,
		TypeCode:    "1",
		Options:     0,
		Provider:    ProviderName,
		Page:        1,
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
		return "11"
	}
	// Detect by structure
	if s.Find("textarea").Length() > 0 {
		return "8"
	}
	if s.Find("select").Length() > 0 {
		return "35"
	}
	if s.Find("input[type='radio']").Length() > 0 {
		return "1"
	}
	if s.Find("input[type='checkbox']").Length() > 0 {
		return "3"
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

func extractRowTexts(s *goquery.Selection) []string {
	var rows []string
	s.Find("tr, .matrix-row").Each(func(i int, row *goquery.Selection) {
		if text := strings.TrimSpace(row.Find("td:first-child, .row-label").First().Text()); text != "" {
			rows = append(rows, text)
		}
	})
	return rows
}

func isRatingQuestion(s *goquery.Selection) bool {
	class, _ := s.Attr("class")
	return strings.Contains(class, "rating") || s.Find(".rating").Length() > 0
}

func isTextQuestion(typeCode string) bool {
	return typeCode == "8" || typeCode == "9" || typeCode == "1" || typeCode == "2"
}

func extractForcedOption(s *goquery.Selection) int {
	// Check for data-forced attribute
	if forced, ok := s.Attr("data-forced"); ok {
		if idx, err := strconv.Atoi(forced); err == nil {
			return idx
		}
	}
	return -1
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

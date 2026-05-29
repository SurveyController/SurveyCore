package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/reversefill"
)

const dimensionUngrouped = "未分组"

var numericOrdinalOptionRE = regexp.MustCompile(`^\s*(\d+)(?:\s*(?:分|点|级|星))?\s*$`)

var chineseOrdinalNumbers = map[string]int{
	"一": 1,
	"二": 2,
	"三": 3,
	"四": 4,
	"五": 5,
	"六": 6,
	"七": 7,
	"八": 8,
	"九": 9,
	"十": 10,
}

var ordinalTextGroups = [][]string{
	{"非常不满意", "不满意", "一般", "满意", "非常满意"},
	{"很不满意", "不满意", "一般", "满意", "很满意"},
	{"非常不同意", "不同意", "一般", "同意", "非常同意"},
	{"很不同意", "不同意", "一般", "同意", "很同意"},
	{"很差", "较差", "一般", "较好", "很好"},
	{"非常差", "差", "一般", "好", "非常好"},
	{"从不", "偶尔", "有时", "经常", "总是"},
	{"完全没有", "较少", "一般", "较多", "非常多"},
}

// LoadFile loads a RuntimeConfig from a JSON file.
func LoadFile(path string) (*models.RuntimeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	cfg, err := models.DeserializeRuntimeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return cfg, nil
}

// SaveFile saves a RuntimeConfig to a JSON file.
func SaveFile(cfg *models.RuntimeConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := models.SerializeRuntimeConfig(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// MergeDefaults fills in missing fields with sensible defaults.
func MergeDefaults(cfg *models.RuntimeConfig) {
	defaults := models.NewDefaultRuntimeConfig()
	if cfg.SurveyProvider == "" {
		cfg.SurveyProvider = defaults.SurveyProvider
	}
	if cfg.Target <= 0 {
		cfg.Target = defaults.Target
	}
	if cfg.Threads <= 0 {
		cfg.Threads = defaults.Threads
	}
	if cfg.ProxySource == "" {
		cfg.ProxySource = defaults.ProxySource
	}
	if cfg.AIMode == "" {
		cfg.AIMode = defaults.AIMode
	}
	if cfg.AIProvider == "" {
		cfg.AIProvider = defaults.AIProvider
	}
	if cfg.AIAPIProtocol == "" {
		cfg.AIAPIProtocol = defaults.AIAPIProtocol
	}
	if cfg.PsychoTargetAlpha == 0 {
		cfg.PsychoTargetAlpha = defaults.PsychoTargetAlpha
	}
	if cfg.RandomUAKeys == nil {
		cfg.RandomUAKeys = defaults.RandomUAKeys
	}
	if cfg.RandomUARatios == nil {
		cfg.RandomUARatios = defaults.RandomUARatios
	}
}

// BuildExecutionConfig creates an ExecutionConfig from a RuntimeConfig.
func BuildExecutionConfig(cfg *models.RuntimeConfig, questions []models.SurveyQuestionMeta) *models.ExecutionConfig {
	ec, _ := BuildExecutionConfigWithError(cfg, questions)
	return ec
}

// BuildExecutionConfigWithError creates an ExecutionConfig and reports feature preparation errors.
func BuildExecutionConfigWithError(cfg *models.RuntimeConfig, questions []models.SurveyQuestionMeta) (*models.ExecutionConfig, error) {
	entryCount := len(cfg.QuestionEntries)
	ec := &models.ExecutionConfig{
		URL:                         cfg.URL,
		SurveyTitle:                 cfg.SurveyTitle,
		SurveyProvider:              cfg.SurveyProvider,
		NumThreads:                  cfg.Threads,
		TargetNum:                   cfg.Target,
		FailThreshold:               5,
		StopOnFailEnabled:           cfg.FailStopEnabled,
		SubmitIntervalRangeSeconds:  cfg.SubmitInterval,
		AnswerDurationRangeSeconds:  cfg.AnswerDuration,
		RandomProxyIPEnabled:        cfg.RandomIPEnabled,
		ProxySource:                 cfg.ProxySource,
		CustomProxyAPI:              cfg.CustomProxyAPI,
		ProxyAreaCode:               safeStr(cfg.ProxyAreaCode),
		RandomIPUserID:              cfg.RandomIPUserID,
		RandomIPDeviceID:            cfg.RandomIPDeviceID,
		IPExtractEndpoint:           cfg.IPExtractEndpoint,
		RandomIPLeaseMinute:         cfg.RandomIPLeaseMinute,
		RandomUserAgentEnabled:      cfg.RandomUAEnabled,
		RandomUserAgentKeys:         append([]string{}, cfg.RandomUAKeys...),
		UserAgentRatios:             cfg.RandomUARatios,
		PauseOnAliyunCaptcha:        cfg.PauseOnAliyunCaptcha,
		PsychoTargetAlpha:           cfg.PsychoTargetAlpha,
		AIMode:                      cfg.AIMode,
		AIProvider:                  cfg.AIProvider,
		AIAPIKey:                    cfg.AIAPIKey,
		AIBaseURL:                   cfg.AIBaseURL,
		AIAPIProtocol:               cfg.AIAPIProtocol,
		AIModel:                     cfg.AIModel,
		AISystemPrompt:              cfg.AISystemPrompt,
		AnswerRules:                 cfg.AnswerRules,
		SingleProb:                  make([]any, entryCount),
		DroplistProb:                make([]any, entryCount),
		MultipleProb:                make([][]float64, entryCount),
		MatrixProb:                  make([]any, entryCount),
		ScaleProb:                   make([]any, entryCount),
		SliderTargets:               make([]float64, entryCount),
		Texts:                       make([][]string, entryCount),
		TextsProb:                   make([][]float64, entryCount),
		TextEntryTypes:              make([]string, entryCount),
		TextRandomModes:             make([]string, entryCount),
		TextRandomIntRanges:         make([][]int, entryCount),
		TextAIFlags:                 make([]bool, entryCount),
		TextTitles:                  make([]string, entryCount),
		DistributionModes:           make([]string, entryCount),
		MultiTextBlankModes:         make([][]string, entryCount),
		MultiTextBlankAIFlags:       make([][]bool, entryCount),
		MultiTextBlankIntRanges:     make([][][]int, entryCount),
		SingleOptionFillTexts:       make([][]*string, entryCount),
		SingleAttachedOptionSelects: make([][]map[string]any, entryCount),
		DroplistOptionFillTexts:     make([][]*string, entryCount),
		MultipleOptionFillTexts:     make([][]*string, entryCount),
		LocationParts:               make(map[int][]string),
		QuestionDimensionMap:        make(map[int]*string),
		QuestionOrdinalScoreMap:     make(map[int][]int),
		QuestionStrictRatioMap:      make(map[int]bool),
		QuestionPsychoBiasMap:       make(map[int]string),
	}

	// Build question metadata map
	ec.QuestionsMetadata = make(map[int]models.SurveyQuestionMeta)
	ec.ProviderQuestionMetadataMap = make(map[string]models.SurveyQuestionMeta)
	for _, q := range questions {
		ec.QuestionsMetadata[q.Num] = q
		key := models.MakeProviderQuestionKey(q.Provider, q.ProviderPageID, q.ProviderQuestionID)
		if key != "" {
			ec.ProviderQuestionMetadataMap[key] = q
		}
		if q.ProviderQuestionID != "" {
			ec.ProviderQuestionMetadataMap[q.ProviderQuestionID] = q
		}
	}

	// Build config index maps
	ec.QuestionConfigIndexMap = make(map[int]string)
	ec.ProviderQuestionConfigIndexMap = make(map[string]string)
	for i, entry := range cfg.QuestionEntries {
		if entry.QuestionNum != nil {
			ec.QuestionConfigIndexMap[*entry.QuestionNum] = fmt.Sprintf("%d", i)
		}
		if entry.ProviderQuestionID != nil {
			key := models.MakeProviderQuestionKey(entry.SurveyProvider, safeStr(entry.ProviderPageID), *entry.ProviderQuestionID)
			if key != "" {
				ec.ProviderQuestionConfigIndexMap[key] = fmt.Sprintf("%d", i)
			}
			if id := safeStr(entry.ProviderQuestionID); id != "" {
				ec.ProviderQuestionConfigIndexMap[id] = fmt.Sprintf("%d", i)
			}
		}
	}

	// Build per-entry arrays. Provider answer builders use the original
	// QuestionEntry index, so every slice must preserve that index exactly.
	reliabilityCandidates := make([]psychometricCandidate, 0)
	hasExplicitRuntimeDimension := false
	for i, entry := range cfg.QuestionEntries {
		questionNum := i + 1
		if entry.QuestionNum != nil {
			questionNum = *entry.QuestionNum
		}
		optionCount := effectiveOptionCount(entry, ec.QuestionsMetadata[questionNum])
		probs := entry.Probabilities
		if entry.CustomWeights != nil {
			probs = entry.CustomWeights
		}
		strictRatio := isStrictCustomRatioMode(entry.DistributionMode, entry.Probabilities, entry.CustomWeights)
		ec.QuestionStrictRatioMap[questionNum] = strictRatio

		switch entry.QuestionType {
		case "single":
			ec.SingleProb[i] = probs
			ec.SingleOptionFillTexts[i] = entry.OptionFillTexts
			ec.SingleAttachedOptionSelects[i] = cloneMapSlice(entry.AttachedOptionSelects)
			if scores := inferOrdinalOptionScores(questionMetaOptionTexts(ec.QuestionsMetadata, questionNum)); len(scores) == maxInt(1, optionCount) {
				ec.QuestionOrdinalScoreMap[questionNum] = scores
			}
			reliabilityCandidates = append(reliabilityCandidates, psychometricCandidate{QuestionNum: questionNum, QuestionType: entry.QuestionType, StrictRatio: strictRatio})
		case "scale", "score":
			ec.ScaleProb[i] = probs
			reliabilityCandidates = append(reliabilityCandidates, psychometricCandidate{QuestionNum: questionNum, QuestionType: entry.QuestionType, StrictRatio: strictRatio})
		case "dropdown", "droplist":
			ec.DroplistProb[i] = probs
			ec.DroplistOptionFillTexts[i] = entry.OptionFillTexts
			reliabilityCandidates = append(reliabilityCandidates, psychometricCandidate{QuestionNum: questionNum, QuestionType: "dropdown", StrictRatio: strictRatio})
		case "multiple":
			if values, ok := toFloat64Slice(probs); ok {
				ec.MultipleProb[i] = values
			}
			ec.MultipleOptionFillTexts[i] = entry.OptionFillTexts
		case "matrix":
			ec.MatrixProb[i] = probs
			reliabilityCandidates = append(reliabilityCandidates, psychometricCandidate{QuestionNum: questionNum, QuestionType: entry.QuestionType, StrictRatio: strictRatio})
		case "slider":
			if target, ok := toFloat64(probs); ok {
				ec.SliderTargets[i] = target
			}
		}

		ec.Texts[i] = append([]string{}, entry.Texts...)
		if values, ok := toFloat64Slice(probs); ok {
			ec.TextsProb[i] = values
		}
		ec.TextEntryTypes[i] = entry.QuestionType
		ec.TextRandomModes[i] = entry.TextRandomMode
		ec.TextRandomIntRanges[i] = append([]int{}, entry.TextRandomIntRange...)
		ec.TextAIFlags[i] = entry.AIEnabled
		ec.DistributionModes[i] = entry.DistributionMode
		if entry.QuestionTitle != nil {
			ec.TextTitles[i] = *entry.QuestionTitle
		}
		ec.MultiTextBlankModes[i] = append([]string{}, entry.MultiTextBlankModes...)
		ec.MultiTextBlankAIFlags[i] = append([]bool{}, entry.MultiTextBlankAIFlags...)
		ec.MultiTextBlankIntRanges[i] = cloneIntRanges(entry.MultiTextBlankIntRanges)

		if supportsPsychometricQuestionType(entry.QuestionType) {
			ec.QuestionPsychoBiasMap[questionNum] = normalizePsychoBias(entry.PsychoBias)
		}

		if len(entry.LocationParts) > 0 {
			ec.LocationParts[questionNum] = append([]string{}, entry.LocationParts...)
		}
		if dim, ok := runtimeDimension(entry.Dimension); ok {
			ec.QuestionDimensionMap[questionNum] = &dim
			hasExplicitRuntimeDimension = true
		}
	}

	if cfg.ReliabilityModeEnabled && len(reliabilityCandidates) > 0 && !hasExplicitRuntimeDimension {
		for _, candidate := range reliabilityCandidates {
			if !supportsPsychometricQuestionType(candidate.QuestionType) {
				continue
			}
			if _, exists := ec.QuestionDimensionMap[candidate.QuestionNum]; exists {
				continue
			}
			dim := models.GlobalReliabilityDimension
			ec.QuestionDimensionMap[candidate.QuestionNum] = &dim
		}
	}

	if cfg.ReverseFillEnabled {
		spec, err := reversefill.BuildSpec(cfg, questions)
		ec.ReverseFillSpec = spec
		if err != nil {
			return ec, err
		}
	}

	return ec, nil
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toFloat64Slice(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := item.(float64); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case []float64:
		if len(n) > 0 {
			return n[0], true
		}
	case []any:
		if len(n) > 0 {
			return toFloat64(n[0])
		}
	default:
		return 0, false
	}
	return 0, false
}

func cloneIntRanges(src [][]int) [][]int {
	if src == nil {
		return nil
	}
	dst := make([][]int, len(src))
	for i := range src {
		dst[i] = append([]int{}, src[i]...)
	}
	return dst
}

type psychometricCandidate struct {
	QuestionNum  int
	QuestionType string
	StrictRatio  bool
}

func isStrictCustomRatioMode(distributionMode string, probabilities, customWeights any) bool {
	if strings.ToLower(strings.TrimSpace(distributionMode)) != "custom" {
		return false
	}
	return hasPositiveWeightValues(customWeights) || hasPositiveWeightValues(probabilities)
}

func hasPositiveWeightValues(raw any) bool {
	switch value := raw.(type) {
	case nil:
		return false
	case int:
		return value > 0
	case int64:
		return value > 0
	case float32:
		return value > 0 && !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case float64:
		return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
	case json.Number:
		f, err := value.Float64()
		return err == nil && f > 0 && !math.IsNaN(f) && !math.IsInf(f, 0)
	case []float64:
		for _, item := range value {
			if hasPositiveWeightValues(item) {
				return true
			}
		}
	case [][]float64:
		for _, row := range value {
			if hasPositiveWeightValues(row) {
				return true
			}
		}
	case []int:
		for _, item := range value {
			if item > 0 {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if hasPositiveWeightValues(item) {
				return true
			}
		}
	}
	return false
}

func supportsPsychometricQuestionType(questionType string) bool {
	switch strings.ToLower(strings.TrimSpace(questionType)) {
	case "single", "scale", "score", "dropdown", "droplist", "matrix":
		return true
	default:
		return false
	}
}

func normalizePsychoBias(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "right", "center":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "center"
	}
}

func runtimeDimension(dimension *string) (string, bool) {
	if dimension == nil {
		return "", false
	}
	dim := strings.TrimSpace(*dimension)
	if dim == "" || dim == dimensionUngrouped {
		return "", false
	}
	return dim, true
}

func questionMetaOptionTexts(metadata map[int]models.SurveyQuestionMeta, questionNum int) []string {
	if metadata == nil {
		return nil
	}
	meta, ok := metadata[questionNum]
	if !ok {
		return nil
	}
	return meta.OptionTexts
}

func effectiveOptionCount(entry models.QuestionEntry, meta models.SurveyQuestionMeta) int {
	if entry.OptionCount > 0 {
		return entry.OptionCount
	}
	if meta.Options > 0 {
		return meta.Options
	}
	if len(meta.OptionTexts) > 0 {
		return len(meta.OptionTexts)
	}
	return models.InferOptionCount(&entry)
}

func inferOrdinalOptionScores(optionTexts []string) []int {
	texts := normalizeOrdinalTexts(optionTexts)
	if len(texts) < 2 {
		return nil
	}
	scores := parseNumericOrdinalOptions(texts)
	if len(scores) == 0 {
		scores = parseChineseNumericOrdinalOptions(texts)
	}
	if len(scores) == 0 {
		scores = matchOrdinalTextGroup(texts)
	}
	if !isOrdinalPermutation(scores, len(texts)) {
		return nil
	}
	return scores
}

func normalizeOrdinalTexts(optionTexts []string) []string {
	texts := make([]string, 0, len(optionTexts))
	for _, item := range optionTexts {
		normalized := strings.Join(strings.Fields(strings.TrimSpace(item)), "")
		if normalized != "" {
			texts = append(texts, normalized)
		}
	}
	return texts
}

func parseNumericOrdinalOptions(texts []string) []int {
	values := make([]int, 0, len(texts))
	for _, text := range texts {
		match := numericOrdinalOptionRE.FindStringSubmatch(text)
		if len(match) != 2 {
			return nil
		}
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return nil
		}
		values = append(values, value)
	}
	return ordinalScoresFromSequentialValues(values)
}

func parseChineseNumericOrdinalOptions(texts []string) []int {
	values := make([]int, 0, len(texts))
	for _, text := range texts {
		text = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(text, "分"), "点"), "级"), "星")
		value, ok := chineseOrdinalNumbers[text]
		if !ok {
			return nil
		}
		values = append(values, value)
	}
	return ordinalScoresFromSequentialValues(values)
}

func ordinalScoresFromSequentialValues(values []int) []int {
	if len(values) < 2 {
		return nil
	}
	ascending := true
	descending := true
	for i := 1; i < len(values); i++ {
		if values[i] != values[0]+i {
			ascending = false
		}
		if values[i] != values[0]-i {
			descending = false
		}
	}
	if ascending {
		minValue := values[0]
		scores := make([]int, len(values))
		for i, value := range values {
			scores[i] = value - minValue
		}
		return scores
	}
	if descending {
		maxValue := values[0]
		scores := make([]int, len(values))
		for i, value := range values {
			scores[i] = maxValue - value
		}
		return scores
	}
	return nil
}

func matchOrdinalTextGroup(texts []string) []int {
	for _, group := range ordinalTextGroups {
		normalizedGroup := normalizeOrdinalTexts(group)
		if len(texts) > len(normalizedGroup) {
			continue
		}
		if stringSlicesEqual(texts, normalizedGroup[:len(texts)]) {
			return intRange(0, len(texts), 1)
		}
		tail := normalizedGroup[len(normalizedGroup)-len(texts):]
		reversedTail := reversedStrings(tail)
		if stringSlicesEqual(texts, reversedTail) {
			return intRange(len(texts)-1, -1, -1)
		}
		if len(texts) == len(normalizedGroup) && stringSlicesEqual(texts, reversedStrings(normalizedGroup)) {
			return intRange(len(texts)-1, -1, -1)
		}
	}
	return nil
}

func isOrdinalPermutation(scores []int, length int) bool {
	if len(scores) != length {
		return false
	}
	sortedScores := append([]int{}, scores...)
	sort.Ints(sortedScores)
	for i, score := range sortedScores {
		if score != i {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func reversedStrings(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[len(values)-1-i] = value
	}
	return result
}

func intRange(start, stop, step int) []int {
	if step == 0 {
		return nil
	}
	result := make([]int, 0)
	for value := start; (step > 0 && value < stop) || (step < 0 && value > stop); value += step {
		result = append(result, value)
	}
	return result
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// PrettyPrint prints a config as formatted JSON.
func PrettyPrint(cfg *models.RuntimeConfig) string {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data)
}

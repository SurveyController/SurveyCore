package config

import (
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func TestBuildExecutionConfigKeepsQuestionEntryIndices(t *testing.T) {
	q1, q2, q3, q4 := 1, 2, 3, 4
	cfg := &models.RuntimeConfig{
		URL:                  "https://www.wjx.cn/vm/test.aspx",
		SurveyProvider:       models.ProviderWJX,
		Target:               10,
		Threads:              3,
		RandomUAEnabled:      true,
		RandomUAKeys:         []string{"pc", "mobile"},
		RandomUARatios:       map[string]int{"pc": 70, "mobile": 30},
		FailStopEnabled:      true,
		PsychoTargetAlpha:    0.9,
		PauseOnAliyunCaptcha: true,
		QuestionEntries: []models.QuestionEntry{
			{QuestionNum: &q1, QuestionType: "single", Probabilities: []any{0.1, 0.9}, AttachedOptionSelects: []map[string]any{{"option_index": 1}}},
			{QuestionNum: &q2, QuestionType: "multiple", Probabilities: []any{0.8, 0.2}},
			{QuestionNum: &q3, QuestionType: "scale", Probabilities: []any{0.2, 0.3, 0.5}},
			{QuestionNum: &q4, QuestionType: "droplist", Probabilities: []any{0.4, 0.6}},
		},
	}

	execCfg := BuildExecutionConfig(cfg, nil)

	if len(execCfg.SingleProb) != len(cfg.QuestionEntries) {
		t.Fatalf("SingleProb length = %d, want %d", len(execCfg.SingleProb), len(cfg.QuestionEntries))
	}
	if execCfg.SingleProb[0] == nil {
		t.Fatal("single probabilities should be stored at original index 0")
	}
	if len(execCfg.SingleAttachedOptionSelects[0]) != 1 {
		t.Fatalf("single attached selects = %#v, want one copied config", execCfg.SingleAttachedOptionSelects[0])
	}
	if execCfg.MultipleProb[1] == nil {
		t.Fatal("multiple probabilities should be stored at original index 1")
	}
	if execCfg.ScaleProb[2] == nil {
		t.Fatal("scale probabilities should be stored at original index 2")
	}
	if execCfg.DroplistProb[3] == nil {
		t.Fatal("droplist probabilities should be stored at original index 3")
	}
	if execCfg.QuestionConfigIndexMap[q4] != "3" {
		t.Fatalf("question config index for q4 = %q, want 3", execCfg.QuestionConfigIndexMap[q4])
	}
	if !execCfg.RandomUserAgentEnabled || len(execCfg.RandomUserAgentKeys) != 2 {
		t.Fatal("random user agent settings were not copied")
	}
}

func TestBuildExecutionConfigUsesCustomWeights(t *testing.T) {
	q1 := 1
	cfg := &models.RuntimeConfig{
		SurveyProvider: models.ProviderWJX,
		QuestionEntries: []models.QuestionEntry{
			{
				QuestionNum:   &q1,
				QuestionType:  "single",
				Probabilities: []any{0.5, 0.5},
				CustomWeights: []any{0.0, 1.0},
			},
		},
	}

	execCfg := BuildExecutionConfig(cfg, nil)
	got, ok := execCfg.SingleProb[0].([]any)
	if !ok {
		t.Fatalf("SingleProb[0] type = %T, want []any", execCfg.SingleProb[0])
	}
	if got[0].(float64) != 0 || got[1].(float64) != 1 {
		t.Fatalf("SingleProb[0] = %v, want custom weights", got)
	}
}

func TestBuildExecutionConfigIndexesProviderQuestionIDs(t *testing.T) {
	providerID := "q-provider-1"
	pageID := "page-1"
	cfg := &models.RuntimeConfig{
		SurveyProvider: models.ProviderQQ,
		QuestionEntries: []models.QuestionEntry{
			{
				QuestionType:       "single",
				Probabilities:      []any{1.0, 0.0},
				SurveyProvider:     models.ProviderQQ,
				ProviderQuestionID: &providerID,
				ProviderPageID:     &pageID,
			},
		},
	}
	questions := []models.SurveyQuestionMeta{
		{
			Num:                7,
			TypeCode:           "3",
			Provider:           models.ProviderQQ,
			ProviderQuestionID: providerID,
			ProviderPageID:     pageID,
		},
	}

	execCfg := BuildExecutionConfig(cfg, questions)
	fullKey := models.MakeProviderQuestionKey(models.ProviderQQ, pageID, providerID)
	if execCfg.ProviderQuestionConfigIndexMap[fullKey] != "0" {
		t.Fatalf("full provider config index = %q, want 0", execCfg.ProviderQuestionConfigIndexMap[fullKey])
	}
	if execCfg.ProviderQuestionConfigIndexMap[providerID] != "0" {
		t.Fatalf("bare provider config index = %q, want 0", execCfg.ProviderQuestionConfigIndexMap[providerID])
	}
	if execCfg.ProviderQuestionMetadataMap[fullKey].Num != 7 {
		t.Fatalf("full provider metadata = %#v, want question 7", execCfg.ProviderQuestionMetadataMap[fullKey])
	}
	if execCfg.ProviderQuestionMetadataMap[providerID].Num != 7 {
		t.Fatalf("bare provider metadata = %#v, want question 7", execCfg.ProviderQuestionMetadataMap[providerID])
	}
}

func TestBuildDefaultQuestionEntriesCreatesDefaultsForCommonTypes(t *testing.T) {
	forced := 2
	sliderMin := 10.0
	sliderMax := 20.0
	questions := []models.SurveyQuestionMeta{
		{Num: 1, Title: "单选", TypeCode: "3", Options: 4, OptionTexts: []string{"A", "B", "C", "D"}, ForcedOptionIndex: &forced, ForcedOptionText: "C", Provider: models.ProviderWJX},
		{Num: 2, Title: "多选", TypeCode: "4", Options: 3, Provider: models.ProviderWJX},
		{Num: 3, Title: "矩阵", TypeCode: "6", Options: 5, Rows: 2, Provider: models.ProviderWJX},
		{Num: 4, Title: "评分", TypeCode: "5", IsRating: true, RatingMax: 7, Provider: models.ProviderWJX},
		{Num: 5, Title: "滑块矩阵", TypeCode: "5", IsSliderMatrix: true, SliderMin: &sliderMin, SliderMax: &sliderMax, Provider: models.ProviderWJX},
		{Num: 6, Title: "填空", TypeCode: "1", IsTextLike: true, TextInputCount: 1, ForcedTexts: []string{"指定文本"}, Provider: models.ProviderWJX},
		{Num: 7, Title: "说明", TypeCode: "0", IsDescription: true, Provider: models.ProviderWJX},
		{Num: 8, Title: "不支持", TypeCode: "99", Unsupported: true, Provider: models.ProviderWJX},
	}

	entries := BuildDefaultQuestionEntries(questions, nil)

	if len(entries) != 6 {
		t.Fatalf("entries length = %d, want 6: %#v", len(entries), entries)
	}
	if *entries[0].QuestionNum != 1 || entries[0].QuestionType != "single" || entries[0].DistributionMode != "custom" {
		t.Fatalf("single entry = %#v", entries[0])
	}
	if weights := entries[0].Probabilities.([]float64); weights[2] != 1 || weights[0] != 0 {
		t.Fatalf("forced single weights = %#v, want only index 2", weights)
	}
	if entries[1].QuestionType != "multiple" || len(entries[1].Probabilities.([]float64)) != 3 {
		t.Fatalf("multiple entry = %#v", entries[1])
	}
	if entries[2].QuestionType != "matrix" || entries[2].Rows != 2 || entries[2].Probabilities != -1 {
		t.Fatalf("matrix entry = %#v", entries[2])
	}
	if entries[3].QuestionType != "score" || entries[3].OptionCount != 7 || entries[3].DistributionMode != "custom" {
		t.Fatalf("score entry = %#v", entries[3])
	}
	if entries[4].QuestionType != "matrix" || entries[4].Probabilities != -1 {
		t.Fatalf("slider matrix entry = %#v", entries[4])
	}
	if entries[5].QuestionType != "text" || len(entries[5].Texts) != 1 || entries[5].Texts[0] != "指定文本" {
		t.Fatalf("text entry = %#v", entries[5])
	}
}

func TestBuildDefaultQuestionEntriesInfersMultiTextBlankModes(t *testing.T) {
	questions := []models.SurveyQuestionMeta{
		{
			Num:             11,
			Title:           "多项填空",
			TypeCode:        "9",
			TextInputCount:  3,
			IsTextLike:      true,
			IsMultiText:     true,
			TextInputLabels: []string{"项目评价", "请输入手机号", "备注"},
			Provider:        models.ProviderWJX,
		},
	}

	entries := BuildDefaultQuestionEntries(questions, nil)

	if entries[0].QuestionType != "multi_text" {
		t.Fatalf("question type = %q, want multi_text", entries[0].QuestionType)
	}
	want := []string{models.TextRandomNone, models.TextRandomMobile, models.TextRandomNone}
	for i, mode := range want {
		if entries[0].MultiTextBlankModes[i] != mode {
			t.Fatalf("blank modes = %#v, want %#v", entries[0].MultiTextBlankModes, want)
		}
	}
}

func TestBuildDefaultQuestionEntriesReusesExistingByProviderNumAndTitle(t *testing.T) {
	providerID := "provider-1"
	oldTitle := "旧标题"
	newTitle := "新标题"
	multiTitle := "多选题"
	textTitle := "标题匹配"
	existing := []models.QuestionEntry{
		{
			QuestionType:          "single",
			Probabilities:         []float64{0, 1},
			CustomWeights:         []float64{0, 1},
			OptionCount:           2,
			QuestionNum:           intPtr(99),
			QuestionTitle:         &oldTitle,
			SurveyProvider:        models.ProviderWJX,
			ProviderQuestionID:    &providerID,
			DistributionMode:      "custom",
			FillableOptionIndices: []int{1},
			AttachedOptionSelects: []map[string]any{{"option_index": 1, "weights": []float64{1, 0}}},
		},
		{
			QuestionType:     "multiple",
			Probabilities:    []float64{10, 90},
			CustomWeights:    []float64{10, 90},
			OptionCount:      2,
			QuestionNum:      intPtr(2),
			QuestionTitle:    &multiTitle,
			DistributionMode: "custom",
		},
		{
			QuestionType:  "text",
			Probabilities: []float64{1},
			Texts:         []string{"旧答案"},
			QuestionNum:   intPtr(88),
			QuestionTitle: &textTitle,
			AIEnabled:     true,
		},
	}
	questions := []models.SurveyQuestionMeta{
		{Num: 1, Title: newTitle, TypeCode: "3", Options: 2, Provider: models.ProviderWJX, ProviderQuestionID: providerID, FillableOptions: []int{1}},
		{Num: 2, Title: multiTitle, TypeCode: "4", Options: 2, Provider: models.ProviderWJX},
		{Num: 3, Title: textTitle, TypeCode: "1", IsTextLike: true, TextInputCount: 1, Provider: models.ProviderWJX},
	}

	entries := BuildDefaultQuestionEntries(questions, existing)

	if got := entries[0].Probabilities.([]float64); got[1] != 1 {
		t.Fatalf("provider reuse probabilities = %#v", got)
	}
	if entries[0].DistributionMode != "custom" || len(entries[0].AttachedOptionSelects) != 1 {
		t.Fatalf("provider reuse entry = %#v", entries[0])
	}
	if got := entries[1].Probabilities.([]float64); got[0] != 10 || entries[1].DistributionMode != "custom" {
		t.Fatalf("num reuse entry = %#v", entries[1])
	}
	if entries[2].Texts[0] != "旧答案" || !entries[2].AIEnabled {
		t.Fatalf("title reuse entry = %#v", entries[2])
	}
}

func TestBuildExecutionConfigAcceptsSliderTargetSlice(t *testing.T) {
	q1 := 1
	cfg := &models.RuntimeConfig{
		SurveyProvider: models.ProviderWJX,
		QuestionEntries: []models.QuestionEntry{
			{QuestionNum: &q1, QuestionType: "slider", Probabilities: []float64{42}, CustomWeights: []float64{43}},
		},
	}

	execCfg := BuildExecutionConfig(cfg, nil)

	if execCfg.SliderTargets[0] != 43 {
		t.Fatalf("SliderTargets[0] = %v, want first custom weight 43", execCfg.SliderTargets[0])
	}
}

func TestBuildExecutionConfigAcceptsDropdownAliasAndTextRandomConfig(t *testing.T) {
	q1, q2, q3 := 1, 2, 3
	fillText := "其他"
	cfg := &models.RuntimeConfig{
		SurveyProvider: models.ProviderWJX,
		QuestionEntries: []models.QuestionEntry{
			{
				QuestionNum:     &q1,
				QuestionType:    "dropdown",
				Probabilities:   []float64{0, 1},
				OptionFillTexts: []*string{nil, &fillText},
			},
			{
				QuestionNum:        &q2,
				QuestionType:       "text",
				Probabilities:      []float64{1},
				Texts:              []string{"原值"},
				TextRandomMode:     models.TextRandomInteger,
				TextRandomIntRange: []int{9, 3},
			},
			{
				QuestionNum:             &q3,
				QuestionType:            "multi_text",
				Probabilities:           []float64{1},
				Texts:                   []string{"姓名|电话"},
				MultiTextBlankModes:     []string{models.TextRandomName, models.TextRandomMobile},
				MultiTextBlankIntRanges: [][]int{{}, {}},
			},
		},
	}

	execCfg := BuildExecutionConfig(cfg, nil)

	if execCfg.DroplistProb[0] == nil || execCfg.DroplistOptionFillTexts[0][1] == nil || *execCfg.DroplistOptionFillTexts[0][1] != fillText {
		t.Fatalf("dropdown config not copied: prob=%#v fill=%#v", execCfg.DroplistProb[0], execCfg.DroplistOptionFillTexts[0])
	}
	if execCfg.TextEntryTypes[1] != "text" || execCfg.TextRandomModes[1] != models.TextRandomInteger {
		t.Fatalf("text random config = type %q mode %q", execCfg.TextEntryTypes[1], execCfg.TextRandomModes[1])
	}
	if got := execCfg.TextRandomIntRanges[1]; len(got) != 2 || got[0] != 9 || got[1] != 3 {
		t.Fatalf("text random range = %#v, want [9 3]", got)
	}
	if execCfg.TextEntryTypes[2] != "multi_text" || execCfg.MultiTextBlankModes[2][1] != models.TextRandomMobile {
		t.Fatalf("multi-text config not copied: %#v", execCfg.MultiTextBlankModes[2])
	}
}

func TestBuildExecutionConfigMapsReliabilityOrdinalAndBiasSignals(t *testing.T) {
	q1, q2, q3 := 1, 2, 3
	cfg := &models.RuntimeConfig{
		ReliabilityModeEnabled: true,
		QuestionEntries: []models.QuestionEntry{
			{
				QuestionNum:      &q1,
				QuestionType:     "single",
				Probabilities:    []float64{1, 1, 1, 1, 1},
				CustomWeights:    []float64{0, 0, 1, 0, 0},
				DistributionMode: "custom",
				PsychoBias:       "right",
			},
			{
				QuestionNum:   &q2,
				QuestionType:  "scale",
				Probabilities: []float64{1, 1, 1, 1, 1},
			},
			{
				QuestionNum:   &q3,
				QuestionType:  "matrix",
				Probabilities: [][]float64{{1, 1, 1}, {1, 1, 1}},
			},
		},
	}
	questions := []models.SurveyQuestionMeta{
		{Num: q1, TypeCode: "3", Options: 5, OptionTexts: []string{"非常满意", "满意", "一般", "不满意", "非常不满意"}},
		{Num: q2, TypeCode: "5", Options: 5},
		{Num: q3, TypeCode: "6", Options: 3, Rows: 2},
	}

	execCfg := BuildExecutionConfig(cfg, questions)

	if !execCfg.QuestionStrictRatioMap[q1] {
		t.Fatal("single custom ratio should be marked strict")
	}
	wantScores := []int{4, 3, 2, 1, 0}
	for i, want := range wantScores {
		if execCfg.QuestionOrdinalScoreMap[q1][i] != want {
			t.Fatalf("ordinal scores = %#v, want %#v", execCfg.QuestionOrdinalScoreMap[q1], wantScores)
		}
	}
	for _, qNum := range []int{q1, q2, q3} {
		dim := execCfg.QuestionDimensionMap[qNum]
		if dim == nil || *dim != models.GlobalReliabilityDimension {
			t.Fatalf("question %d dimension = %#v, want global reliability", qNum, dim)
		}
	}
	if execCfg.QuestionPsychoBiasMap[q1] != "right" {
		t.Fatalf("psycho bias = %q, want right", execCfg.QuestionPsychoBiasMap[q1])
	}
}

func TestBuildExecutionConfigKeepsExplicitDimensionsInsteadOfGlobalReliability(t *testing.T) {
	q1, q2 := 1, 2
	dim := "服务体验"
	cfg := &models.RuntimeConfig{
		ReliabilityModeEnabled: true,
		QuestionEntries: []models.QuestionEntry{
			{QuestionNum: &q1, QuestionType: "single", Probabilities: []float64{1, 1}, Dimension: &dim},
			{QuestionNum: &q2, QuestionType: "scale", Probabilities: []float64{1, 1}},
		},
	}
	questions := []models.SurveyQuestionMeta{
		{Num: q1, TypeCode: "3", Options: 2},
		{Num: q2, TypeCode: "5", Options: 2},
	}

	execCfg := BuildExecutionConfig(cfg, questions)

	if execCfg.QuestionDimensionMap[q1] == nil || *execCfg.QuestionDimensionMap[q1] != dim {
		t.Fatalf("explicit dimension = %#v, want %q", execCfg.QuestionDimensionMap[q1], dim)
	}
	if _, ok := execCfg.QuestionDimensionMap[q2]; ok {
		t.Fatalf("implicit global dimension should not be added when explicit dimensions exist: %#v", execCfg.QuestionDimensionMap)
	}
}

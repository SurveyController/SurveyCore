package wjx

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
	nethtml "golang.org/x/net/html"
)

func (p Parser) Parse(ctx context.Context, surveyURL string) (model.SurveyDefinition, error) {
	htmlText, err := p.getHTML(ctx, surveyURL)
	if err != nil {
		return model.SurveyDefinition{}, err
	}
	definition, err := p.parseDefinition(ctx, htmlText, surveyURL)
	if err != nil {
		return model.SurveyDefinition{}, err
	}
	if len(definition.Questions) == 0 {
		return model.SurveyDefinition{}, ParseError{Message: "问卷星 HTTP 页面未返回可解析题目"}
	}
	return definition, nil
}

func ParseDefinitionFromHTML(htmlText string) (model.SurveyDefinition, error) {
	if err := pageStateError(htmlText); err != nil {
		return model.SurveyDefinition{}, err
	}
	root, err := parseHTML(htmlText)
	if err != nil {
		return model.SurveyDefinition{}, err
	}
	container := findFirst(root, func(node *nethtml.Node) bool {
		return isElement(node, "div") && attr(node, "id") == "divQuestion"
	})
	if container == nil {
		return model.SurveyDefinition{}, nil
	}
	fieldsets := findAll(container, func(node *nethtml.Node) bool {
		return isElement(node, "fieldset")
	})
	if len(fieldsets) == 0 {
		fieldsets = []*nethtml.Node{container}
	}
	questions := make([]model.QuestionMeta, 0)
	for pageIndex, fieldset := range fieldsets {
		visible := 0
		for _, questionDiv := range directQuestionDivs(fieldset) {
			if isEmptyGapfill(questionDiv) {
				continue
			}
			question := normalizeQuestion(questionDiv, pageIndex+1)
			if question.Num <= 0 {
				continue
			}
			if !isHidden(questionDiv) {
				visible++
				if question.Num != visible {
					question.Page = pageIndex + 1
				}
				display := visible
				question.DisplayNum = &display
			}
			questions = append(questions, question)
		}
	}
	byNum := make(map[int]int, len(questions))
	for index, question := range questions {
		byNum[question.Num] = index
	}
	for _, target := range questions {
		for _, condition := range target.DisplayConditions {
			if index, ok := byNum[condition.QuestionNum]; ok {
				source := &questions[index]
				source.HasDependentDisplayLogic = true
				source.LogicStatus = model.LogicParseStatusComplete
				source.ControlsDisplayTargets = append(source.ControlsDisplayTargets, model.DisplayControl{TargetQuestionNum: target.Num, Mode: condition.Mode, OptionIndices: append([]int(nil), condition.OptionIndices...)})
			}
		}
	}
	for index := range questions {
		targets := questions[index].ControlsDisplayTargets
		sort.SliceStable(targets, func(i, j int) bool { return targets[i].TargetQuestionNum < targets[j].TargetQuestionNum })
	}
	sort.SliceStable(questions, func(i int, j int) bool {
		if questions[i].Page == questions[j].Page {
			return questions[i].Num < questions[j].Num
		}
		return questions[i].Page < questions[j].Page
	})
	return model.SurveyDefinition{
		Provider:  model.ProviderWJX,
		Title:     surveyTitle(root),
		Questions: questions,
	}, nil
}

func isEmptyGapfill(div *nethtml.Node) bool {
	return isHidden(div) && attr(div, "type") == "9" && attr(div, "gapfill") == "1" && (attr(div, "relation") == "" || attr(div, "relation") == "0") && countTextInputs(div) == 0
}

func emptyGapfillNumbers(htmlText string) []int {
	root, err := parseHTML(htmlText)
	if err != nil {
		return nil
	}
	var numbers []int
	for _, div := range directQuestionDivs(root) {
		if isEmptyGapfill(div) && questionNumber(div) > 0 {
			numbers = append(numbers, questionNumber(div))
		}
	}
	return numbers
}

func directQuestionDivs(root *nethtml.Node) []*nethtml.Node {
	candidates := findAll(root, func(node *nethtml.Node) bool {
		return isElement(node, "div") && hasAttr(node, "topic")
	})
	result := make([]*nethtml.Node, 0, len(candidates))
	for _, candidate := range candidates {
		hasQuestionAncestor := false
		for parent := candidate.Parent; parent != nil && parent != root; parent = parent.Parent {
			if isElement(parent, "div") && hasAttr(parent, "topic") {
				hasQuestionAncestor = true
				break
			}
		}
		if !hasQuestionAncestor {
			result = append(result, candidate)
		}
	}
	return result
}

func normalizeQuestion(div *nethtml.Node, page int) model.QuestionMeta {
	num := questionNumber(div)
	typeCode := strings.TrimSpace(attr(div, "type"))
	if typeCode == "" {
		typeCode = "0"
	}
	if typeCode != "11" && looksLikeOrder(div) {
		typeCode = "11"
	}
	multilevel := findFirst(div, func(node *nethtml.Node) bool {
		return isElement(node, "input") && attr(node, "verify") == "多级下拉"
	})
	isMultilevel := typeCode == "1" && multilevel != nil
	if isMultilevel {
		typeCode = "7"
	}
	title := questionTitle(div, num)
	optionTexts, rowTexts, fillable := questionOptions(div, num, typeCode)
	optionCount := len(optionTexts)
	if typeCode == "8" {
		optionCount = maxInt(1, optionCount)
	}
	textInputs := 0
	switch typeCode {
	case "1", "2", "9":
		textInputs = countTextInputs(div)
	}
	isLocation := isLocationQuestion(div) && (typeCode == "1" || typeCode == "2")
	if isLocation {
		textInputs = 0
	}
	isDescription := looksLikeDescription(div, typeCode)
	isSliderMatrix := typeCode != "8" && looksLikeSliderMatrix(div)
	rating := typeCode == "5" && looksLikeRating(div)
	if typeCode == "5" && len(optionTexts) == 0 {
		optionTexts = ratingTexts(div)
	}
	if typeCode == "11" && len(optionTexts) == 0 {
		optionTexts, _ = choiceTexts(div)
	}
	multiMin, multiMax := multiLimits(div, typeCode)
	kind := providerType(typeCode, textInputs, len(optionTexts), isSliderMatrix)
	if rating {
		kind = "score"
	}
	forcedIdx, forcedText := (*int)(nil), ""
	if kind == "single" || kind == "score" || kind == "dropdown" {
		forcedIdx, forcedText = forceSelectOption(title, optionTexts)
	}
	textLabels := textInputLabels(div)
	attachedSelects := attachedOptionSelects(div, optionTexts)
	slider := sliderRange(div)
	hasJump, jumpRules, hasDisplay, displayConditions, hasDependentDisplay, controlsDisplayTargets, logicStatus := logicMetadata(div, num, optionTexts)
	if isDescription {
		typeCode = "0"
	}
	var pageRaw any
	providerType := typeCode
	if isMultilevel {
		providerType = multilevelDropdown
		pageRaw = map[string]any{"cid": attr(multilevel, "cid")}
	}
	unsupported := !isDescription && (!wjxSupportedTypeCode(typeCode) || ((typeCode == "1" || typeCode == "2" || typeCode == "9") && textInputs <= 0 && !isSliderMatrix))
	unsupportedReason := ""
	if unsupported {
		unsupportedReason = "问卷星题型缺少合法文本输入或暂不支持：" + firstNonEmpty(typeCode, "unknown")
	}
	return model.QuestionMeta{
		Num:             num,
		Title:           title,
		Description:     "",
		TypeCode:        typeCode,
		Options:         maxInt(optionCount, len(optionTexts)),
		Rows:            maxInt(1, len(rowTexts)),
		RowTexts:        rowTexts,
		Page:            page,
		OptionTexts:     optionTexts,
		Provider:        model.ProviderWJX,
		ProviderID:      strconv.Itoa(num),
		ProviderPageID:  strconv.Itoa(page),
		ProviderType:    providerType,
		ProviderPageRaw: pageRaw,
		Required:        required(div),
		IsDescription:   isDescription,
		IsLocation:      isLocation,
		IsRating:        rating,
		RatingMax:       ratingMax(rating, len(optionTexts)),
		TextInputs:      textInputs,
		TextInputLabels: textLabels,
		IsTextLike:      !isSliderMatrix && isTextLike(typeCode, textInputs, len(optionTexts), isLocation),
		IsMultiText:     textInputs > 1 && !isSliderMatrix,
		IsSliderMatrix:  isSliderMatrix,
		QuestionLogic: model.QuestionLogic{
			LogicStatus:              logicStatus,
			HasJump:                  hasJump,
			JumpRules:                jumpRules,
			HasDisplayCondition:      hasDisplay,
			DisplayConditions:        displayConditions,
			HasDependentDisplayLogic: hasDependentDisplay,
			ControlsDisplayTargets:   controlsDisplayTargets,
		},
		QuestionMedia:           questionMedia(div, rowTexts, optionTexts),
		SliderRange:             slider,
		MultiMinLimit:           multiMin,
		MultiMaxLimit:           multiMax,
		ForcedOptionIdx:         forcedIdx,
		ForcedOption:            forcedText,
		ForcedTexts:             []string{},
		FillableOptions:         fillable,
		AttachedOptionSelects:   attachedSelects,
		HasAttachedOptionSelect: len(attachedSelects) > 0,
		Unsupported:             unsupported,
		UnsupportedReason:       unsupportedReason,
	}
}

func wjxSupportedTypeCode(typeCode string) bool {
	switch strings.TrimSpace(typeCode) {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "11", "33", "34":
		return true
	default:
		return false
	}
}

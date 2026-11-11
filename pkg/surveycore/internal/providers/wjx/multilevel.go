package wjx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
	nethtml "golang.org/x/net/html"
)

const multilevelDropdown = "multilevel_dropdown"

var (
	activityIDRE        = regexp.MustCompile(`\bvar\s+activityId\s*=\s*(?:DecodeId\((\d+)\)|(\d+))`)
	activityDecoderRE   = regexp.MustCompile(`function\s+DecodeId\([^)]*\)\s*\{\s*return\s+\w+\s*\^\s*(\d+)`)
	externalLevelDataRE = regexp.MustCompile(`levelDataExternalUrl\s*=\s*["'][^"']+`)
	levelMarkerRE       = regexp.MustCompile(`╰[0-9]+╯`)
)

func (p Parser) parseDefinition(ctx context.Context, htmlText, surveyURL string) (model.SurveyDefinition, error) {
	definition, err := ParseDefinitionFromHTML(htmlText)
	if err != nil {
		return definition, err
	}
	definition.SurveyURL = surveyURL
	for index := range definition.Questions {
		question := &definition.Questions[index]
		if question.ProviderType != multilevelDropdown {
			continue
		}
		activity := activityIDRE.FindStringSubmatch(htmlText)
		if len(activity) == 0 {
			return definition, ParseError{Message: "问卷星多级下拉缺少 activityId"}
		}
		id, _ := strconv.ParseInt(firstNonEmpty(activity[1], activity[2]), 10, 64)
		if activity[1] != "" {
			decoder := activityDecoderRE.FindStringSubmatch(htmlText)
			if len(decoder) == 0 {
				return definition, ParseError{Message: "问卷星多级下拉无法解析 activityId"}
			}
			mask, _ := strconv.ParseInt(decoder[1], 10, 64)
			id ^= mask
		}
		page, err := url.Parse(surveyURL)
		if err != nil {
			return definition, err
		}
		endpoint := page.ResolveReference(&url.URL{Path: "/joinnew/setmenuselnew.aspx"})
		query := url.Values{"activityid": {strconv.FormatInt(id, 10)}, "ct": {"5"}, "pos": {strconv.Itoa(question.Num)}}
		metadata := question.ProviderPageRaw.(map[string]any)
		cid, _ := metadata["cid"].(string)
		if cid != "" {
			query.Set("cid", cid)
		}
		endpoint.RawQuery = query.Encode()
		menuHTML, err := fetchHTMLWithReferer(ctx, httpClientOrDefault(p.Client), endpoint.String(), surveyURL, p.UserAgent)
		if err != nil {
			return definition, err
		}
		texts, values, labels, err := parseMultilevelOptions(menuHTML, cid != "")
		if err != nil {
			return definition, err
		}
		question.OptionTexts, question.Options = texts, len(texts)
		metadata["submit_values"], metadata["level_labels"] = values, labels
	}
	return definition, nil
}

func parseMultilevelOptions(htmlText string, encoded bool) ([]string, []string, []string, error) {
	fail := func(message string) ([]string, []string, []string, error) {
		return nil, nil, nil, ParseError{Message: "问卷星多级下拉" + message}
	}
	root, err := parseHTML(htmlText)
	if err != nil {
		return nil, nil, nil, err
	}
	input := findFirst(root, func(node *nethtml.Node) bool { return isElement(node, "input") && attr(node, "id") == "hfLevelData" })
	if input == nil {
		return fail("未返回 hfLevelData")
	}
	for _, script := range findAll(root, func(node *nethtml.Node) bool { return isElement(node, "script") }) {
		if externalLevelDataRE.MatchString(textContent(script)) {
			return fail("暂不支持外部数据源")
		}
	}
	data := attr(input, "value")
	if encoded {
		data, err = url.PathUnescape(data)
		if err != nil {
			return fail("层级数据编码无效")
		}
	}
	levels, labelText, _ := strings.Cut(data, "〒")
	columns := strings.Split(levels, "|")
	if levels == "" || len(columns) > 6 {
		return fail("层级数据为空或超过六级")
	}
	var paths, terminal [][]string
	previous := map[string]bool{}
	for level, column := range columns {
		groups := map[string][]string{}
		parent := ""
		for _, raw := range strings.Split(strings.ReplaceAll(column, "\r\n", "\n"), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if level == 0 {
				if strings.HasPrefix(line, "---") {
					return fail("首级选项无效")
				}
				paths = append(paths, []string{line})
				previous[line] = true
			} else if strings.HasPrefix(line, "---") {
				parent = strings.TrimPrefix(line, "---")
				if !previous[parent] {
					return fail("父选项不存在：" + parent)
				}
				groups[parent] = nil
			} else if parent == "" {
				return fail("子选项缺少父选项")
			} else {
				groups[parent] = append(groups[parent], line)
			}
		}
		if level == 0 {
			if len(paths) == 0 {
				return fail("首级选项无效")
			}
			continue
		}
		var next [][]string
		for _, path := range paths {
			children := groups[path[len(path)-1]]
			if len(children) == 0 {
				return fail("缺少子选项：" + path[len(path)-1])
			}
			ends := false
			for _, child := range children {
				ends = ends || strings.Index(child, "々") > 0
			}
			if ends {
				terminal = append(terminal, path)
			} else {
				for _, child := range children {
					next = append(next, append(append([]string(nil), path...), child))
				}
			}
		}
		paths = next
		previous = map[string]bool{}
		for _, children := range groups {
			for _, child := range children {
				previous[child] = true
			}
		}
	}
	var texts, values []string
	for _, path := range append(terminal, paths...) {
		clean := make([]string, len(path))
		labels := make([]string, len(path))
		for index, value := range path {
			clean[index] = levelMarkerRE.ReplaceAllString(value, "")
			runes := []rune(clean[index])
			if len(runes) > 1 && strings.ContainsRune("┌┍┎┏┐┑┒┓└┕┖┗┘┙┚┛├┝┞┟┠┡┢┣┤┥┦┧┨┩┪┫┬┭┮┯┰┱┲┳┴┵┶┷┸┹┺┻┼┽┾┿╀╁╂╃╄╅╆╇", runes[len(runes)-1]) {
				runes = runes[:len(runes)-1]
			}
			labels[index] = string(runes)
		}
		texts = append(texts, strings.Join(labels, "‐"))
		values = append(values, strings.Join(clean, "‐"))
	}
	labels := []string{}
	if labelText != "" {
		labels = strings.Split(labelText, "|")
	}
	return texts, values, labels, nil
}

func multilevelAnswer(question model.QuestionMeta, indices []int) (string, error) {
	metadata, _ := question.ProviderPageRaw.(map[string]any)
	var values []string
	switch raw := metadata["submit_values"].(type) {
	case []string:
		values = raw
	case []any:
		for _, item := range raw {
			text, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("第%d题多级下拉提交值无效", question.Num)
			}
			values = append(values, text)
		}
	}
	if len(indices) != 1 || indices[0] < 0 || indices[0] >= len(values) {
		return "", fmt.Errorf("第%d题多级下拉缺少有效提交值", question.Num)
	}
	return values[indices[0]], nil
}

package wjx

import (
	"html"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestMultilevelOptionsPreservePathsAndSubmissionMarkers(t *testing.T) {
	for _, test := range []struct {
		data                  string
		encoded               bool
		texts, values, labels []string
	}{
		{"总部╰1╯┌|---总部╰1╯┌\n研发部\n支持部〒公司|部门", true, []string{"总部‐研发部", "总部‐支持部"}, []string{"总部┌‐研发部", "总部┌‐支持部"}, []string{"公司", "部门"}},
		{"甲\n乙|---甲\n无下级々\n---乙\n分部|---无下级々\n占位々\n---分部\n研发部", false, []string{"甲", "乙‐分部‐研发部"}, []string{"甲", "乙‐分部‐研发部"}, []string{}},
	} {
		data := test.data
		if test.encoded {
			data = url.PathEscape(data)
		}
		texts, values, labels, err := parseMultilevelOptions(`<input id="hfLevelData" value="`+html.EscapeString(data)+`">`, test.encoded)
		if err != nil || !reflect.DeepEqual(texts, test.texts) || !reflect.DeepEqual(values, test.values) || !reflect.DeepEqual(labels, test.labels) {
			t.Fatalf("texts=%v values=%v labels=%v error=%v", texts, values, labels, err)
		}
	}
}

func TestMultilevelRejectsIncompleteAndExternalData(t *testing.T) {
	for _, test := range []struct{ html, message string }{
		{"<html></html>", "hfLevelData"},
		{`<input id="hfLevelData" value="">`, "层级数据为空"},
		{"<input id=\"hfLevelData\" value=\"甲|---乙\n子项\">", "父选项不存在"},
		{`<input id="hfLevelData" value="甲|---甲">`, "缺少子选项"},
		{`<input id="hfLevelData" value="甲"><script>var levelDataExternalUrl = "https://example.test/data";</script>`, "外部数据源"},
		{`<input id="hfLevelData" value="甲|乙|丙|丁|戊|己|庚">`, "超过六级"},
	} {
		if _, _, _, err := parseMultilevelOptions(test.html, false); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("input=%s error=%v", test.html, err)
		}
	}
}

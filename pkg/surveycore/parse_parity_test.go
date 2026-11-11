package surveycore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestParseMatchesPythonPublicMetadata(t *testing.T) {
	data, err := os.ReadFile("testdata/python-parse.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name      string                     `json:"name"`
			URL       string                     `json:"url"`
			Responses map[string]json.RawMessage `json:"responses"`
			Expected  model.SurveyDefinition     `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			snapshot := model.CloneSurveyDefinition(test.Expected)
			posts := 0
			client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: submissionTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method == http.MethodPost {
					posts++
					if test.Expected.Provider == "wjx" {
						if err := request.ParseForm(); err != nil {
							return nil, err
						}
						if strings.HasPrefix(test.Name, "wjx-multilevel") && request.Form.Get("submitdata") != "1$}2$总部┌‐研发部" {
							return nil, fmt.Errorf("multilevel answer lost raw values or empty placeholder: %q", request.Form.Get("submitdata"))
						}
						return runResultsResponse(request, "10")
					}
					if test.Expected.Provider == "qq" {
						return runResultsResponse(request, `{"code":"OK","data":{"answer_hash":"fixture-answer"}}`)
					}
					return runResultsResponse(request, `{"success":true,"data":{}}`)
				}
				if strings.Contains(request.URL.Path, "/answer/noauth/init/") {
					return runResultsResponse(request, `{"success":true,"data":{"answerToken":"fixture-token"}}`)
				}
				if test.Expected.Provider == "qq" && posts > 0 && strings.HasSuffix(request.URL.Path, "/session") {
					return runResultsResponse(request, `{"code":"OK","data":{"answer_session_id":"fixture-session","answer_session":{"last_answer_id":1}}}`)
				}
				if request.URL.Path == "/joinnew/setmenuselnew.aspx" {
					query := request.URL.Query()
					if query.Get("activityid") != "366296538" || query.Get("pos") != "2" || query.Get("ct") != "5" || request.Referer() != "https://v.wjx.cn/vm/levels.aspx" || (test.Name == "wjx-multilevel-cid" && query.Get("cid") != "cid47") {
						return nil, fmt.Errorf("invalid multilevel request: %s referer=%s", request.URL, request.Referer())
					}
				}
				body, ok := test.Responses[request.URL.Path]
				if !ok {
					return nil, fmt.Errorf("unexpected request: %s", request.URL)
				}
				var html string
				if json.Unmarshal(body, &html) == nil {
					body = []byte(html)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: request}, nil
			})}))
			actual, err := client.Parse(context.Background(), test.URL)
			if err != nil {
				t.Fatal(err)
			}
			// The Go extension retains provider on each question as well.
			for index := range test.Expected.Questions {
				test.Expected.Questions[index].Provider = test.Expected.Provider
			}
			compareParseJSON(t, "parse", test.Expected, actual)
			config, err := client.DefaultConfig(context.Background(), test.URL)
			if err != nil {
				t.Fatal(err)
			}
			compareParseJSON(t, "config definition", test.Expected, config.SurveyDefinition)
			answerable := 0
			for _, question := range actual.Questions {
				if !question.IsDescription && !question.Unsupported {
					answerable++
				}
			}
			if config.SurveySource.URL != test.Expected.SurveyURL || len(config.AnswerPlan.Strategies) != answerable {
				t.Fatalf("default config lost URL or questions: %#v", config)
			}
			for index := range snapshot.Questions {
				if snapshot.Questions[index].ProviderType == "multilevel_dropdown" {
					first := 0
					snapshot.Questions[index].ForcedOptionIdx = &first
				}
			}
			original, _ := json.Marshal(snapshot)
			config.SurveyDefinition = model.SurveyDefinition{}
			config.PsychometricPolicy.Enabled = false
			config.AnswerPlan = model.AnswerPlan{}
			result, err := client.Submit(context.Background(), &surveycore.SubmissionRequest{Config: config, Survey: &snapshot})
			if err != nil || result == nil || !result.Success || posts != 1 {
				t.Fatalf("Python snapshot submission result=%#v error=%v posts=%d", result, err, posts)
			}
			current, _ := json.Marshal(snapshot)
			if string(original) != string(current) {
				t.Fatal("submission changed the caller's provider metadata")
			}
		})
	}
}

func compareParseJSON(t *testing.T, path string, expected, actual any) {
	t.Helper()
	toJSON := func(value any) any {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var result any
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	var compare func(string, any, any)
	compare = func(path string, expected, actual any) {
		switch expected := expected.(type) {
		case map[string]any:
			object, ok := actual.(map[string]any)
			if !ok {
				t.Errorf("%s: expected object, got %#v", path, actual)
				return
			}
			if len(object) != len(expected) {
				t.Errorf("%s: expected %d fields, got %d", path, len(expected), len(object))
			}
			for key, value := range expected {
				compare(path+"."+key, value, object[key])
			}
		case []any:
			items, ok := actual.([]any)
			if !ok || len(items) != len(expected) {
				t.Errorf("%s: expected %#v, got %#v", path, expected, actual)
				return
			}
			for index, item := range expected {
				compare(fmt.Sprintf("%s[%d]", path, index), item, items[index])
			}
		default:
			if !reflect.DeepEqual(expected, actual) {
				t.Errorf("%s: expected %#v, got %#v", path, expected, actual)
			}
		}
	}
	compare(path, toJSON(expected), toJSON(actual))
}

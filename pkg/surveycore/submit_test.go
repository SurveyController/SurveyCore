package surveycore_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

type submissionTransport func(*http.Request) (*http.Response, error)

func (fn submissionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSubmitRejectsInvalidRequestsBeforeNetwork(t *testing.T) {
	client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: submissionTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached the transport")
		return nil, nil
	})}))
	for _, test := range []struct {
		request *surveycore.SubmissionRequest
		kind    surveycore.ErrorKind
	}{
		{nil, surveycore.ErrorKindConfig},
		{&surveycore.SubmissionRequest{}, surveycore.ErrorKindConfig},
		{&surveycore.SubmissionRequest{Config: &model.RunRequest{}}, surveycore.ErrorKindConfig},
		{&surveycore.SubmissionRequest{Config: &model.RunRequest{SurveySource: model.SurveySource{URL: "https://example.invalid/survey"}}}, surveycore.ErrorKindUnsupported},
		{&surveycore.SubmissionRequest{Config: &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx", Provider: "qq"}}}, surveycore.ErrorKindUnsupported},
		{&surveycore.SubmissionRequest{Config: &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}}, Survey: &model.SurveyDefinition{Provider: "qq"}}, surveycore.ErrorKindConfig},
	} {
		result, err := client.Submit(context.Background(), test.request)
		if result != nil || err == nil || surveycore.ClassifySubmissionError(err) != test.kind {
			t.Fatalf("request=%#v result=%#v error=%v", test.request, result, err)
		}
	}
	if result, err := surveycore.Submit(context.Background(), nil); result != nil || !errors.Is(err, surveycore.ErrInvalidConfig) {
		t.Fatalf("package-level Submit result=%#v error=%v", result, err)
	}
}

func TestSubmitUsesSurveySnapshotAndKeepsCallerState(t *testing.T) {
	posts := 0
	transport := submissionTransport(func(request *http.Request) (*http.Response, error) {
		body := `<html><head><title>Live title</title></head><body><div id="divQuestion"><fieldset><div topic="1" id="div1" type="3"><div class="topichtml">Choice</div><div class="ui-controlgroup"><div><span class="label">A</span></div><div><span class="label">B</span></div></div></div></fieldset></div></body></html>`
		if request.Method == http.MethodPost {
			posts++
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("submitdata") != "1$2" {
				t.Fatalf("survey snapshot was not applied: %q", request.Form.Get("submitdata"))
			}
			body = "10"
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	forced := 1
	request := &surveycore.SubmissionRequest{
		Config: &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}, ExecutionPlan: model.ExecutionPlan{Target: 20, Threads: 8}},
		Survey: &model.SurveyDefinition{Provider: "wjx", Title: "Snapshot", Questions: []model.QuestionMeta{{Num: 1, TypeCode: "3", Options: 2, ForcedOptionIdx: &forced}}},
	}
	original, _ := json.Marshal(request)
	client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: transport}))
	result, err := client.Submit(context.Background(), request)
	if err != nil || !result.Success || posts != 1 {
		t.Fatalf("result=%#v error=%v posts=%d", result, err, posts)
	}
	current, _ := json.Marshal(request)
	if string(original) != string(current) {
		t.Fatal("caller config or snapshot was modified")
	}
}

func TestSubmissionResultMatchesPythonFixture(t *testing.T) {
	// Produced with dataclasses.asdict on surveycore-py 411ef6d SubmissionResult.
	data, err := os.ReadFile("testdata/submission-results.json")
	if err != nil {
		t.Fatal(err)
	}
	var results []surveycore.SubmissionResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	var expected, actual any
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("Go result JSON differs from Python: %s", encoded)
	}
}

func TestSubmitPreservesExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, err := surveycore.Submit(ctx, &surveycore.SubmissionRequest{Config: &model.RunRequest{
		SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"},
	}})
	if !errors.Is(err, context.DeadlineExceeded) || surveycore.ClassifySubmissionError(err) != surveycore.ErrorKindCanceled || result == nil || result.Success || result.Error == nil {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

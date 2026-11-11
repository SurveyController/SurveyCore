package restapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/service"
	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

type submissionTestTransport func(*http.Request) (*http.Response, error)

func (fn submissionTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func submissionTestServer(t *testing.T, success bool) (*Server, *atomic.Int32) {
	t.Helper()
	s, err := New(Config{DBPath: t.TempDir() + "/submissions.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	posts := &atomic.Int32{}
	transport := submissionTestTransport(func(request *http.Request) (*http.Response, error) {
		body := `<html><head><title>Submission</title></head><body><div id="divQuestion"><fieldset><div topic="1" id="div1" type="3"><div class="topichtml">Choice</div><div class="ui-controlgroup"><div><span class="label">A</span></div><div><span class="label">B</span></div></div></div></fieldset></div></body></html>`
		if request.Method == http.MethodPost {
			posts.Add(1)
			if success {
				body = "10"
			} else {
				body = "1〒1〒失败"
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	s.manager = service.NewManager(s.store, surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: transport})))
	return s, posts
}

func TestV1SingleSubmissionAcceptsBothConfigFormats(t *testing.T) {
	native, err := json.Marshal(map[string]any{"config": model.RunRequest{
		SurveySource:  model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"},
		ExecutionPlan: model.ExecutionPlan{Target: 9, Threads: 4, SubmitInterval: [2]int{600, 600}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		string(native),
		`{"config":{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx","target":9,"concurrency":4,"submit_interval":[600,600],"questions":[{"question_type":"single","probabilities":[0,1]}]}}`,
	} {
		s, posts := submissionTestServer(t, true)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/submissions", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var result surveycore.SubmissionResult
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Success || result.Attempt != 0 || result.Provider != "wjx" || result.Error != nil || posts.Load() != 1 || len(s.manager.List()) != 0 || !strings.Contains(rec.Body.String(), `"error":null`) {
			t.Fatalf("result=%#v posts=%d tasks=%d", result, posts.Load(), len(s.manager.List()))
		}
	}
}

func TestV1SingleSubmissionReturnsPlatformFailureInResult(t *testing.T) {
	s, posts := submissionTestServer(t, false)
	body := `{"config":{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx"}}`
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/submissions", strings.NewReader(body)))
	var result surveycore.SubmissionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || result.Success || result.Error == nil || posts.Load() != 1 || result.ElapsedSeconds < 0 {
		t.Fatalf("status=%d result=%#v posts=%d", rec.Code, result, posts.Load())
	}
}

func TestV1SingleSubmissionRejectsInvalidRequestsWithoutNetwork(t *testing.T) {
	s, posts := submissionTestServer(t, true)
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{}`, 400}, {`null`, 400}, {`{"config":null}`, 400},
		{`{"config":{"unknown":1}}`, 400},
		{`{"config":{"schema_version":1,"url":"x"},"unknown":1}`, 400},
		{`{"config":{"schema_version":1,"url":"x"},"survey":{"unknown":1}}`, 400},
		{`{"config":{"schema_version":1,"url":"x"}} {}`, 400},
		{`{"config":{}}`, 422},
		{`{"config":{"schema_version":1,"url":"https://example.invalid/survey"}}`, 422},
		{`{"config":{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx","provider":"qq"}}`, 422},
		{`{"config":{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx"},"survey":{"provider":"qq"}}`, 422},
	} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/submissions", strings.NewReader(test.body)))
		if rec.Code != test.status {
			t.Fatalf("request=%s status=%d body=%s", test.body, rec.Code, rec.Body.String())
		}
	}
	if posts.Load() != 0 || len(s.manager.List()) != 0 {
		t.Fatal("invalid requests created submissions or tasks")
	}
}

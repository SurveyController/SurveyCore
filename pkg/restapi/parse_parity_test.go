package restapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/service"
	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestHTTPParseConfigAndPythonSnapshots(t *testing.T) {
	data, err := os.ReadFile("../surveycore/testdata/python-parse.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name      string                     `json:"name"`
			URL       string                     `json:"url"`
			Responses map[string]json.RawMessage `json:"responses"`
			Expected  json.RawMessage            `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			s, err := New(Config{DBPath: t.TempDir() + "/parse.db"})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			transport := submissionTestTransport(func(request *http.Request) (*http.Response, error) {
				body := test.Responses[request.URL.Path]
				if request.Method == http.MethodPost {
					body = []byte("10")
				} else if body == nil {
					return nil, fmt.Errorf("unexpected request: %s", request.URL)
				} else {
					var html string
					if json.Unmarshal(body, &html) == nil {
						body = []byte(html)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: request}, nil
			})
			s.manager = service.NewManager(s.store, surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: transport})))
			requestBody, _ := json.Marshal(map[string]string{"url": test.URL})
			var expected model.SurveyDefinition
			if err := json.Unmarshal(test.Expected, &expected); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/v1/surveys/parse", "/api/v1/configs"} {
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(requestBody))))
				if rec.Code != http.StatusOK {
					t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
				}
				var definition model.SurveyDefinition
				if strings.HasSuffix(path, "/configs") {
					var config model.RunRequest
					if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil {
						t.Fatal(err)
					}
					definition = config.SurveyDefinition
				} else if err := json.Unmarshal(rec.Body.Bytes(), &definition); err != nil {
					t.Fatal(err)
				}
				if definition.Provider != expected.Provider || definition.SurveyURL != expected.SurveyURL || len(definition.Questions) != len(expected.Questions) {
					t.Fatalf("HTTP lost survey metadata: %#v", definition)
				}
			}
			if expected.Provider == "wjx" {
				body, _ := json.Marshal(map[string]any{
					"config": map[string]any{"schema_version": 1, "url": test.URL, "reliability_mode": false, "answer_duration": []int{0, 0}},
					"survey": test.Expected,
				})
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/submissions", strings.NewReader(string(body))))
				var result surveycore.SubmissionResult
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != http.StatusOK || !result.Success {
					t.Fatalf("Python snapshot status=%d body=%s error=%v", rec.Code, rec.Body.String(), err)
				}
			}
		})
	}
}

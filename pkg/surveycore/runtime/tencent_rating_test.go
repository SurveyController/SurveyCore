package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestTencentDefaultConfigSubmitsRatingsAndConfirmsPersistence(t *testing.T) {
	data, err := os.ReadFile("../internal/providers/tencent/testdata/live-rating-questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var questions []map[string]any
	if err := json.Unmarshal(data, &questions); err != nil {
		t.Fatal(err)
	}
	questions = append(questions, map[string]any{"id": "star", "type": "star", "title": "星级", "star_num": 5, "page_id": "p3", "page": 3, "hidden": false})
	var posts atomic.Int32
	answers := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		response := map[string]any{}
		switch {
		case strings.HasSuffix(request.URL.Path, "/session"):
			response = map[string]any{"answer_session_id": "rating-session", "answer_session": map[string]any{"last_answer_id": posts.Load()}}
		case strings.HasSuffix(request.URL.Path, "/meta"):
			response = map[string]any{"title": "评分回归"}
		case strings.HasSuffix(request.URL.Path, "/questions"):
			response = map[string]any{"questions": questions}
		case strings.HasSuffix(request.URL.Path, "/answers"):
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("invalid submit JSON: %v", err)
				http.Error(w, "bad JSON", http.StatusBadRequest)
				return
			}
			answers <- body
			posts.Add(1)
			response = map[string]any{"answer_hash": "recorded-rating"}
		default:
			t.Errorf("unexpected path: %s", request.URL.Path)
			http.NotFound(w, request)
			return
		}
		writeTestJSON(t, w, map[string]any{"code": "OK", "data": response})
	}))
	defer server.Close()
	client := New(WithHTTPClient(rewriteTencentHTTPClient(server.URL)))
	config, err := client.DefaultConfig(context.Background(), "https://wj.qq.com/s2/123/hash/")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AnswerPlan.Strategies) != 4 || config.AnswerPlan.Strategies[0].QuestionType != model.QuestionKindMatrix || config.AnswerPlan.Strategies[1].QuestionType != model.QuestionKindScore {
		t.Fatalf("default rating strategies=%#v", config.AnswerPlan.Strategies)
	}
	config.ExecutionPlan.Target, config.ExecutionPlan.Threads = 1, 1
	config.PsychometricPolicy.Enabled = false
	for index := range config.AnswerPlan.Strategies {
		entry := &config.AnswerPlan.Strategies[index]
		weights := make([]float64, entry.OptionCount)
		weights[0] = 1
		entry.DistributionMode = "random"
		entry.Probabilities = model.OptionWeights(weights...)
		entry.CustomWeights = model.WeightTable{}
	}
	result, err := client.Run(context.Background(), config)
	if err != nil || result == nil || result.Success != 1 || result.Fail != 0 || posts.Load() != 1 {
		t.Fatalf("result=%#v error=%v posts=%d", result, err, posts.Load())
	}
	body := <-answers
	pages := body["answer_survey"].(map[string]any)["pages"].([]any)
	ids := map[string]bool{}
	for _, page := range pages {
		for _, answer := range page.(map[string]any)["questions"].([]any) {
			ids[answer.(map[string]any)["id"].(string)] = true
		}
	}
	for _, id := range []string{"q-15-a899-0", "q-18-b757-0", "star-1", "q-13-7d39-g-101-ae7c-1", "q-13-7d39-g-105-d55c-1"} {
		if !ids[id] {
			t.Errorf("missing rating answer %q in %#v", id, ids)
		}
	}
	if len(ids) != 8 {
		t.Fatalf("expected every matrix row and scalar score, got %#v", ids)
	}
}

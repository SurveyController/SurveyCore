package tests

import (
	"encoding/json"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/models"
)

func TestRuntimeConfigSerialization(t *testing.T) {
	cfg := models.NewDefaultRuntimeConfig()
	cfg.URL = "https://www.wjx.cn/vm/test.aspx"
	cfg.Target = 10
	cfg.Threads = 3

	data, err := models.SerializeRuntimeConfig(&cfg)
	if err != nil {
		t.Fatalf("SerializeRuntimeConfig failed: %v", err)
	}

	parsed, err := models.DeserializeRuntimeConfig(data)
	if err != nil {
		t.Fatalf("DeserializeRuntimeConfig failed: %v", err)
	}

	if parsed.URL != cfg.URL {
		t.Errorf("URL mismatch: got %s, want %s", parsed.URL, cfg.URL)
	}
	if parsed.Target != cfg.Target {
		t.Errorf("Target mismatch: got %d, want %d", parsed.Target, cfg.Target)
	}
	if parsed.Threads != cfg.Threads {
		t.Errorf("Threads mismatch: got %d, want %d", parsed.Threads, cfg.Threads)
	}
}

func TestQuestionEntryInferOptionCount(t *testing.T) {
	tests := []struct {
		name  string
		entry *models.QuestionEntry
		want  int
	}{
		{
			name:  "nil entry",
			entry: nil,
			want:  0,
		},
		{
			name: "with option_count",
			entry: &models.QuestionEntry{
				QuestionType: "single",
				OptionCount:  5,
			},
			want: 5,
		},
		{
			name: "scale type defaults to 5",
			entry: &models.QuestionEntry{
				QuestionType: "scale",
			},
			want: 5,
		},
		{
			name: "from probabilities",
			entry: &models.QuestionEntry{
				QuestionType:  "single",
				Probabilities: []any{0.1, 0.2, 0.3, 0.4},
			},
			want: 4,
		},
		{
			name: "from texts",
			entry: &models.QuestionEntry{
				QuestionType: "text",
				Texts:        []string{"a", "b", "c"},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := models.InferOptionCount(tt.entry)
			if got != tt.want {
				t.Errorf("InferOptionCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProxyLease(t *testing.T) {
	lease := models.ProxyLease{
		Address:  "127.0.0.1:8080",
		Poolable: true,
		Source:   "test",
	}

	if lease.IsExpired() {
		t.Error("Lease with 0 ExpireTS should not be expired")
	}

	if !lease.HasSufficientTTL(60) {
		t.Error("Lease with 0 ExpireTS should have sufficient TTL")
	}
}

func TestExecutionState(t *testing.T) {
	state := runstate.NewExecutionState()
	state.Config = &execution.ExecutionConfig{TargetNum: 5}

	state.EnsureWorkerThreads(3, "Worker")
	if len(state.ThreadProgress) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(state.ThreadProgress))
	}

	running := true
	state.UpdateThreadStatus("Worker-1", "测试", &running)
	tp := state.ThreadProgress["Worker-1"]
	if tp.StatusText != "测试" {
		t.Errorf("StatusText = %s, want 测试", tp.StatusText)
	}

	state.IncrementThreadSuccess("Worker-1")
	if state.ThreadProgress["Worker-1"].SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", state.ThreadProgress["Worker-1"].SuccessCount)
	}

	state.IncrementSuccess()
	if state.GetCurNum() != 1 {
		t.Errorf("CurNum = %d, want 1", state.GetCurNum())
	}

	state.IncrementFail()
	if state.GetCurFail() != 1 {
		t.Errorf("CurFail = %d, want 1", state.GetCurFail())
	}

	if state.IsStopped() {
		t.Error("State should not be stopped initially")
	}

	state.SignalStop()
	if !state.IsStopped() {
		t.Error("State should be stopped after SignalStop")
	}
}

func TestSurveyQuestionMeta(t *testing.T) {
	q := models.SurveyQuestionMeta{
		Num:         1,
		Title:       "测试题目",
		TypeCode:    "1",
		Options:     4,
		OptionTexts: []string{"A", "B", "C", "D"},
		Provider:    "wjx",
	}

	if q.Get("num") != 1 {
		t.Errorf("Get(num) = %v, want 1", q.Get("num"))
	}
	if q.Get("title") != "测试题目" {
		t.Errorf("Get(title) = %v, want 测试题目", q.Get("title"))
	}

	// Test ToDict
	dict := q.ToDict()
	if dict["num"] != float64(1) {
		t.Errorf("ToDict[num] = %v, want 1", dict["num"])
	}
}

func TestMakeProviderQuestionKey(t *testing.T) {
	key := models.MakeProviderQuestionKey("wjx", "page1", "q1")
	expected := "wjx:page1:q1"
	if key != expected {
		t.Errorf("MakeProviderQuestionKey() = %s, want %s", key, expected)
	}
	if got := models.MakeProviderQuestionKey("wjx", "", "q1"); got != "" {
		t.Errorf("MakeProviderQuestionKey() with missing page = %q, want empty", got)
	}
	if got := models.MakeProviderQuestionKey("wjx", "page1", ""); got != "" {
		t.Errorf("MakeProviderQuestionKey() with missing question = %q, want empty", got)
	}
}

func TestJSONCompatibility(t *testing.T) {
	jsonStr := `{
		"url": "https://www.wjx.cn/vm/test.aspx",
		"survey_provider": "wjx",
		"target": 10,
		"threads": 3,
		"question_entries": [
			{
				"question_type": "single",
				"probabilities": [0.25, 0.25, 0.25, 0.25],
				"option_count": 4
			}
		]
	}`

	cfg, err := models.DeserializeRuntimeConfig([]byte(jsonStr))
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if cfg.URL != "https://www.wjx.cn/vm/test.aspx" {
		t.Errorf("URL = %s", cfg.URL)
	}
	if cfg.Target != 10 {
		t.Errorf("Target = %d", cfg.Target)
	}
	if len(cfg.QuestionEntries) != 1 {
		t.Fatalf("QuestionEntries length = %d, want 1", len(cfg.QuestionEntries))
	}
	if cfg.QuestionEntries[0].OptionCount != 4 {
		t.Errorf("OptionCount = %d, want 4", cfg.QuestionEntries[0].OptionCount)
	}

	// Round-trip test
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var cfg2 models.RuntimeConfig
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg2.URL != cfg.URL {
		t.Errorf("Round-trip URL mismatch")
	}
}

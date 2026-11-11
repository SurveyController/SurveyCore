package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func pythonFixture(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/python-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestImportPythonSerializerFixture(t *testing.T) {
	// Generated with surveycore-py serialize_config at commit 411ef6d.
	request, err := DeserializeRunRequest(pythonFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if request.SurveySource.URL != "https://www.wjx.cn/vm/demo.aspx" || request.SurveySource.Provider != model.ProviderWJX {
		t.Fatalf("source = %#v", request.SurveySource)
	}
	if request.Target != 3 || request.ExecutionPlan.Threads != 2 || request.AnswerDuration != [2]int{} || request.SubmitInterval != [2]int{} || request.FailStop || request.PauseOnAliyunCaptcha {
		t.Fatalf("execution = %#v", request.ExecutionPlan)
	}
	if !request.PsychometricPolicy.Enabled || request.TargetAlpha != 0.85 {
		t.Fatalf("psychometrics = %#v", request.PsychometricPolicy)
	}
	if request.UserAgent == nil || !request.UserAgent.Enabled || request.UserAgent.Ratios["pc"] != 100 {
		t.Fatalf("userAgent = %#v", request.UserAgent)
	}
	strategies := request.AnswerPlan.Strategies
	if len(strategies) != 5 {
		t.Fatalf("strategies = %#v", strategies)
	}
	for index, strategy := range strategies {
		if strategy.QuestionNum == nil || *strategy.QuestionNum != index+1 {
			t.Fatalf("strategy %d is not bound to its implicit question number", index)
		}
	}
	if !reflect.DeepEqual(strategies[0].Probabilities.Options, []float64{0, 1}) || !reflect.DeepEqual(strategies[1].Probabilities.Options, []float64{100, 0, 100}) || !reflect.DeepEqual(strategies[2].Probabilities.Rows, [][]float64{{1, 0}, {0, 1}}) || !reflect.DeepEqual(strategies[3].Texts, []string{"first", "second"}) || !reflect.DeepEqual(strategies[4].Probabilities.Options, []float64{37}) {
		t.Fatalf("imported strategies = %#v", strategies)
	}
}

func TestPythonConfigDefaultsAndRandomWeights(t *testing.T) {
	request, err := DeserializeRunRequest(map[string]any{
		"schema_version": 1, "url": " https://wj.qq.com/s2/123/hash/ ", "provider": nil,
		"questions": []any{map[string]any{"question_type": "single", "probabilities": -1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Target != 1 || request.ExecutionPlan.Threads != 1 || request.AnswerDuration != [2]int{60, 120} || request.UserAgent.Enabled || request.UserAgent.Ratios["pc"] != 34 {
		t.Fatalf("defaults = %#v", request)
	}
	if request.SurveySource.Provider != "" || len(request.AnswerPlan.Strategies[0].Probabilities.Options) != 0 {
		t.Fatalf("auto provider/random weights = %#v", request)
	}
}

func TestPythonUserAgentRatiosReplaceDefaults(t *testing.T) {
	request, err := DeserializeRunRequest(map[string]any{
		"schema_version": 1, "url": "x",
		"user_agent_ratios": map[string]int{"pc": 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.UserAgent.Ratios, map[string]int{"pc": 100}) {
		t.Fatalf("custom ratios were merged with defaults: %#v", request.UserAgent.Ratios)
	}
}

func TestPythonConfigRejectsInvalidInputs(t *testing.T) {
	cases := map[string]any{
		"schema_version": 2, "unknown": true, "target": 0, "concurrency": "2",
		"provider": "tencent", "url": " ", "questions": nil,
		"answer_duration": []any{1, nil}, "submit_interval": []int{3, 2},
		"answer_datetime_window": []string{""}, "random_user_agent": 1,
		"user_agent_ratios": map[string]any{"pc": nil}, "reliability_mode": nil,
		"target_alpha": true, "answer_rules": []any{nil},
		"answer_generator": "callable", "schemaVersion": 2,
	}
	for field, value := range cases {
		t.Run(field, func(t *testing.T) {
			payload := map[string]any{"schema_version": 1, "url": "https://www.wjx.cn/vm/demo.aspx"}
			payload[field] = value
			if _, err := DeserializeRunRequest(payload); err == nil {
				t.Fatalf("accepted %s=%#v", field, value)
			}
		})
	}
	for _, question := range []any{
		nil, map[string]any{}, map[string]any{"question_type": ""},
		map[string]any{"question_type": "single", "probabilities": map[string]any{"options": []int{1}}},
		map[string]any{"question_type": "single", "probabilities": []int{0, 0}},
		map[string]any{"question_type": "single", "ai_enabled": true},
		map[string]any{"question_type": "single", "unknown": true},
	} {
		if _, err := DeserializeRunRequest(map[string]any{"schema_version": 1, "url": "x", "questions": []any{question}}); err == nil {
			t.Fatalf("accepted invalid question %#v", question)
		}
	}
}

func TestLoadPythonConfigAndSaveAsLocalV2(t *testing.T) {
	data, err := os.ReadFile("testdata/python-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	request, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Save(request, path); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	expectedAnswers, err := json.Marshal(request.AnswerPlan)
	if err != nil {
		t.Fatal(err)
	}
	restoredAnswers, err := json.Marshal(restored.AnswerPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.UserAgent, restored.UserAgent) || string(expectedAnswers) != string(restoredAnswers) || request.ExecutionPlan != restored.ExecutionPlan {
		t.Fatalf("local v2 round trip lost Python settings: %#v", restored)
	}
	// Native task persistence must retain the imported settings too.
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var persisted model.RunRequest
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.UserAgent, persisted.UserAgent) {
		t.Fatalf("task JSON lost user-agent settings: %#v", persisted.UserAgent)
	}
}

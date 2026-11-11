package consumer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/config"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/proxy"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/runtime"
)

type textResolver struct{}

func (textResolver) ResolveText(_ context.Context, _ model.AIProfile, persona *model.Persona, _ surveycore.AITextRequest) ([]string, error) {
	if persona == nil {
		return []string{"默认答案"}, nil
	}
	return []string{persona.Description()}, nil
}

var _ surveycore.AITextResolver = textResolver{}

func TestPublicPackagesCompileForExternalConsumer(t *testing.T) {
	client := surveycore.New(surveycore.WithAITextResolver(textResolver{}))
	if client == nil || !surveycore.IsSupportedURL("https://wj.qq.com/s2/26070328/fa89/") {
		t.Fatal("high-level SDK is unavailable")
	}

	runtimeClient := runtime.New()
	if runtimeClient == nil {
		t.Fatal("runtime extension is unavailable")
	}

	document := config.ConfigDocumentFromRunRequest(model.RunRequest{})
	if document.SchemaVersion != config.ConfigSchemaVersion {
		t.Fatalf("schema version = %d", document.SchemaVersion)
	}
	imported, err := config.DeserializeRunRequest(map[string]any{
		"schema_version": 1, "url": "https://www.wjx.cn/vm/demo.aspx", "concurrency": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if imported.ExecutionPlan.Threads != 2 {
		t.Fatalf("Python concurrency = %d", imported.ExecutionPlan.Threads)
	}
	var submission *surveycore.SubmissionResult
	snapshot := model.SurveyDefinition{SurveyURL: "https://www.wjx.cn/vm/demo.aspx", Questions: []model.QuestionMeta{{
		SliderRange:     model.SliderRange{SliderMin: 0.5, SliderMax: "10", SliderStep: nil},
		ProviderPageRaw: map[string]any{"submit_values": []string{"parent‐child"}},
	}}}
	cloned := model.CloneSurveyDefinition(snapshot)
	if cloned.SurveyURL != snapshot.SurveyURL || cloned.Questions[0].SliderMin != 0.5 {
		t.Fatal("parse snapshot metadata is unavailable to external consumers")
	}
	// The new snapshot URL must not shadow the existing promoted run source URL.
	request := model.RunRequest{SurveySource: model.SurveySource{URL: snapshot.SurveyURL}}
	if request.URL != snapshot.SurveyURL {
		t.Fatal("run source URL compatibility was lost")
	}
	submission, err = client.Submit(context.Background(), &surveycore.SubmissionRequest{})
	if submission != nil || !errors.Is(err, surveycore.ErrInvalidConfig) || surveycore.ClassifySubmissionError(err) != surveycore.ErrorKindConfig {
		t.Fatalf("single submission contract: result=%#v error=%v", submission, err)
	}
	count := 0
	for _, err := range client.RunResults(context.Background(), nil) {
		count++
		if !errors.Is(err, surveycore.ErrInvalidConfig) {
			t.Fatalf("run iterator error=%v", err)
		}
	}
	if count != 1 {
		t.Fatalf("run iterator errors=%d", count)
	}

	pool := proxy.NewPool(proxy.PoolOptions{})
	if pool == nil {
		t.Fatal("proxy extension is unavailable")
	}
}

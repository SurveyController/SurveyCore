package consumer_test

import (
	"context"
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

	pool := proxy.NewPool(proxy.PoolOptions{})
	if pool == nil {
		t.Fatal("proxy extension is unavailable")
	}
}

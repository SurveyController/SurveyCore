package live_tests

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/providers"
)

const liveURLEnv = "SURVEY_CONTROLLER_LIVE_TEST_URL"

var transientExternalFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`HTTP 页面未返回可解析题目`),
}

func TestLiveRuntimeRegression(t *testing.T) {
	surveyURL := strings.TrimSpace(os.Getenv(liveURLEnv))
	if surveyURL == "" {
		t.Skipf("%s 未设置，跳过真实线上回归", liveURLEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	registry := providers.NewRegistry()
	runner := engine.NewEngine(registry, nil)

	def, err := runner.ParseSurvey(ctx, surveyURL)
	if err != nil {
		skipTransientExternalFailure(t, err)
		t.Fatalf("解析真实问卷失败: %v", err)
	}

	questions := answerableQuestions(def.Questions)
	runtimeCfg := models.RuntimeConfig{
		URL:                    surveyURL,
		SurveyTitle:            def.Title,
		SurveyProvider:         def.Provider,
		Target:                 1,
		Threads:                1,
		SubmitInterval:         [2]int{0, 0},
		AnswerDuration:         [2]int{0, 0},
		RandomUAEnabled:        false,
		ReliabilityModeEnabled: true,
		ReverseFillEnabled:     false,
		QuestionEntries:        config.BuildDefaultQuestionEntries(questions, nil),
		QuestionsInfo:          questions,
	}

	execCfg, err := config.BuildExecutionConfigWithError(&runtimeCfg, questions)
	if err != nil {
		t.Fatalf("构造执行配置失败: %v", err)
	}
	execCfg.TargetNum = 1
	execCfg.NumThreads = 1
	execCfg.SubmitIntervalRangeSeconds = [2]int{0, 0}
	execCfg.AnswerDurationRangeSeconds = [2]int{0, 0}
	execCfg.RandomUserAgentEnabled = false

	state := runstate.NewExecutionState()
	state.Config = execCfg

	err = runner.Run(ctx, execCfg, state)
	if err != nil {
		skipTransientExternalFailure(t, err)
		t.Fatalf("运行真实提交失败: %v", err)
	}

	category, reason, message := state.GetTerminalStopSnapshot()
	output := fmt.Sprintf(
		"provider=%s cur_num=%d cur_fail=%d terminal=%q/%q/%q",
		execCfg.SurveyProvider,
		state.GetCurNum(),
		state.GetCurFail(),
		category,
		reason,
		message,
	)
	t.Log(output)

	if state.GetCurNum() < 1 {
		t.Fatalf("真实链路未提交成功: %s", output)
	}
}

func answerableQuestions(questions []models.SurveyQuestionMeta) []models.SurveyQuestionMeta {
	result := make([]models.SurveyQuestionMeta, 0, len(questions))
	for _, question := range questions {
		if question.IsDescription || question.Unsupported {
			continue
		}
		result = append(result, question)
	}
	return result
}

func skipTransientExternalFailure(t *testing.T, err error) {
	t.Helper()
	output := ""
	if err != nil {
		output = err.Error()
	}
	for _, pattern := range transientExternalFailurePatterns {
		if pattern.MatchString(output) {
			t.Skipf("真实问卷返回临时不可解析页面，跳过: %v", err)
		}
	}
}

package surveycore_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func ExampleClient_DefaultConfig() {
	ctx := context.Background()
	client := surveycore.New()
	cfg, err := client.DefaultConfig(ctx, "https://www.wjx.cn/vm/example.aspx")
	if err != nil {
		return
	}
	cfg.ExecutionPlan.Target = 1
	cfg.ExecutionPlan.Threads = 1
	_, _ = client.RunWithEvents(ctx, cfg, func(event model.Event) {
		fmt.Println(event.Message)
	})
}

func ExampleClassifyRunError() {
	_, err := surveycore.New().Run(context.Background(), nil)
	fmt.Println(errors.Is(err, surveycore.ErrInvalidConfig))
	fmt.Println(surveycore.ClassifyRunError(err))
	// Output:
	// true
	// config
}

package models

import (
	"encoding/json"
	"testing"
)

func TestRuntimeConfigPreservesPythonExtraFields(t *testing.T) {
	cfg, err := DeserializeRuntimeConfig([]byte(`{
		"url":"https://www.wjx.cn/vm/test.aspx",
		"target":3,
		"_ai_config_present":true,
		"config_schema_version":6,
		"python_future":{"nested":[1,"two",true]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "https://www.wjx.cn/vm/test.aspx" || cfg.Target != 3 {
		t.Fatalf("known fields = %#v, want decoded runtime config", cfg)
	}
	if len(cfg.ExtraFields) != 3 {
		t.Fatalf("extra fields = %#v, want python-only fields preserved", cfg.ExtraFields)
	}

	data, err := SerializeRuntimeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip["_ai_config_present"] != true || roundTrip["config_schema_version"] != float64(6) {
		t.Fatalf("round trip = %#v, want preserved python metadata", roundTrip)
	}
	future, ok := roundTrip["python_future"].(map[string]any)
	if !ok || len(future["nested"].([]any)) != 3 {
		t.Fatalf("python_future = %#v, want preserved nested object", roundTrip["python_future"])
	}
}

func TestRuntimeConfigSerializationAddsPythonSchemaMetadata(t *testing.T) {
	cfg := NewDefaultRuntimeConfig()
	cfg.URL = "https://www.wjx.cn/vm/test.aspx"

	data, err := SerializeRuntimeConfig(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["config_schema_version"] != float64(CurrentConfigSchemaVersion) {
		t.Fatalf("payload = %#v, want Python schema version", payload)
	}
	if payload["_ai_config_present"] != false {
		t.Fatalf("payload = %#v, want no request-side AI config marker", payload)
	}
}

func TestDefaultRuntimeConfigUsesPythonRandomUAKeys(t *testing.T) {
	cfg := NewDefaultRuntimeConfig()
	if len(cfg.RandomUAKeys) != 3 ||
		cfg.RandomUAKeys[0] != "wechat_android" ||
		cfg.RandomUAKeys[1] != "mobile_android" ||
		cfg.RandomUAKeys[2] != "pc_web" {
		t.Fatalf("random UA keys = %#v, want Python preset defaults", cfg.RandomUAKeys)
	}
}

func TestRuntimeConfigSerializationPreservesPythonSchemaMetadata(t *testing.T) {
	cfg, err := DeserializeRuntimeConfig([]byte(`{
		"url":"https://www.wjx.cn/vm/test.aspx",
		"_ai_config_present":false,
		"config_schema_version":5
	}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := SerializeRuntimeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["config_schema_version"] != float64(5) || payload["_ai_config_present"] != false {
		t.Fatalf("payload = %#v, want imported metadata preserved", payload)
	}
}

func TestRuntimeConfigCloneKeepsExtraFields(t *testing.T) {
	original, err := DeserializeRuntimeConfig([]byte(`{
		"url":"https://www.wjx.cn/vm/test.aspx",
		"python_only_future_field":"keep-me"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := SerializeRuntimeConfig(original)
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := DeserializeRuntimeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(cloned.ExtraFields["python_only_future_field"]) != `"keep-me"` {
		t.Fatalf("extra fields = %#v, want clone to keep unknown field", cloned.ExtraFields)
	}
}

func TestRuntimeConfigAcceptsPythonLooseScalarFields(t *testing.T) {
	cfg, err := DeserializeRuntimeConfig([]byte(`{
		"url":123,
		"target":"5",
		"threads":"2",
		"psycho_target_alpha":"0.91",
		"submit_interval":["7","9"],
		"answer_duration":100,
		"answer_datetime_window":["2026-02-10 09:00:00","bad"],
		"random_ua_keys":["pc_web","bad","wechat_android"],
		"random_ua_ratios":{"wechat":"40","mobile":30,"pc":"30"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "123" || cfg.Target != 5 || cfg.Threads != 2 {
		t.Fatalf("basic fields = url %q target %d threads %d, want string/int coercion", cfg.URL, cfg.Target, cfg.Threads)
	}
	if cfg.PsychoTargetAlpha != 0.91 {
		t.Fatalf("alpha = %v, want parsed float", cfg.PsychoTargetAlpha)
	}
	if cfg.SubmitInterval != [2]int{7, 9} {
		t.Fatalf("submit interval = %#v, want parsed pair", cfg.SubmitInterval)
	}
	if cfg.AnswerDuration != [2]int{90, 110} {
		t.Fatalf("answer duration = %#v, want legacy scalar range", cfg.AnswerDuration)
	}
	if cfg.AnswerDatetimeWindow != [2]string{"2026-02-10 09:00:00", ""} {
		t.Fatalf("answer datetime window = %#v, want normalized valid side only", cfg.AnswerDatetimeWindow)
	}
	if cfg.RandomUARatios["wechat"] != 40 || cfg.RandomUARatios["mobile"] != 30 || cfg.RandomUARatios["pc"] != 30 {
		t.Fatalf("ua ratios = %#v, want parsed int map", cfg.RandomUARatios)
	}
	if len(cfg.RandomUAKeys) != 2 || cfg.RandomUAKeys[0] != "pc_web" || cfg.RandomUAKeys[1] != "wechat_android" {
		t.Fatalf("ua keys = %#v, want Python preset keys filtered", cfg.RandomUAKeys)
	}
}

func TestRuntimeConfigAcceptsPythonLooseAnswerDurationLists(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want [2]int
	}{
		{name: "single item", raw: `[120]`, want: [2]int{108, 132}},
		{name: "equal pair", raw: `[100,100]`, want: [2]int{90, 110}},
		{name: "zero pair uses default", raw: `[0,0]`, want: [2]int{60, 120}},
		{name: "ordered pair", raw: `[3,5]`, want: [2]int{3, 5}},
		{name: "empty list uses default", raw: `[]`, want: [2]int{60, 120}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := DeserializeRuntimeConfig([]byte(`{"answer_duration":` + tt.raw + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AnswerDuration != tt.want {
				t.Fatalf("answer duration = %#v, want %#v", cfg.AnswerDuration, tt.want)
			}
		})
	}
}

func TestRuntimeConfigNormalizesPythonCodecBoundariesAndDropsPrivateFields(t *testing.T) {
	cfg, err := DeserializeRuntimeConfig([]byte(`{
		"proxy_source":"bad",
		"ai_mode":"unsupported",
		"random_ip_user_id":88,
		"random_ip_device_id":"device-88",
		"reverse_fill_format":"spreadsheet",
		"reverse_fill_start_row":0,
		"reverse_fill_threads":"0",
		"random_ua_ratios":{"wechat":20,"mobile":20,"pc":20}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"proxy_source", "ai_mode", "random_ip_user_id", "random_ip_device_id"} {
		if _, ok := cfg.ExtraFields[key]; ok {
			t.Fatalf("private legacy field %q preserved in extras: %#v", key, cfg.ExtraFields)
		}
	}
	if cfg.ReverseFillFormat != ReverseFillFormatAuto {
		t.Fatalf("reverse fill format = %q, want auto", cfg.ReverseFillFormat)
	}
	if cfg.ReverseFillStartRow != 1 || cfg.ReverseFillThreads != 1 {
		t.Fatalf("reverse fill start/threads = %d/%d, want 1/1", cfg.ReverseFillStartRow, cfg.ReverseFillThreads)
	}
	if cfg.RandomUARatios["wechat"] != 33 || cfg.RandomUARatios["mobile"] != 33 || cfg.RandomUARatios["pc"] != 34 {
		t.Fatalf("ua ratios = %#v, want Python defaults for invalid total", cfg.RandomUARatios)
	}
}

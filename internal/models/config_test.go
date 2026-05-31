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

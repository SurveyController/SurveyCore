package io

import "testing"

func TestTypeCodeToNameUsesNormalizedProviderCodes(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"0", "说明"},
		{"1", "填空"},
		{"2", "填空"},
		{"3", "单选"},
		{"4", "多选"},
		{"5", "量表"},
		{"6", "矩阵"},
		{"7", "下拉"},
		{"8", "滑块"},
		{"9", "矩阵/填空"},
		{"11", "排序"},
		{"12", "排序"},
		{"35", "下拉"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := typeCodeToName(tt.code); got != tt.want {
				t.Fatalf("typeCodeToName(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

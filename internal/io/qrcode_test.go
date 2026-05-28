package io

import "testing"

func TestIsSurveyURLUsesProviderURLRules(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"https://www.wjx.cn/vm/demo.aspx", true},
		{"wj.qq.com/s2/26070328/fa89/", true},
		{"https://www.credamo.cn/answer.html#/s/demo", true},
		{"https://wj.qq.com/profile", false},
		{"https://www.credamo.com/profile", false},
		{"https://example.com/?next=wjx.cn", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := IsSurveyURL(tt.text); got != tt.want {
				t.Fatalf("IsSurveyURL(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

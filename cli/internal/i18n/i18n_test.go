package i18n

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsChineseLocale(t *testing.T) {
	for _, locale := range []string{"zh-CN", "zh_CN.UTF-8", "zh-HK", "zh-MO", "zh-TW@calendar=roc", "zh-Hant-TW", "zh_CN:en"} {
		if !IsChineseLocale(locale) {
			t.Errorf("expected Chinese locale: %q", locale)
		}
	}
	for _, locale := range []string{"en-US", "zh", "zh-SG", "ja-JP", "C", ""} {
		if IsChineseLocale(locale) {
			t.Errorf("expected non-Chinese locale: %q", locale)
		}
	}
}

func TestCodedErrorBehaviorSurvivesWrapping(t *testing.T) {
	const (
		outerCode  ErrorCode = "test.outer"
		innerCode  ErrorCode = "test.invalid_value"
		joinedCode ErrorCode = "test.joined"
	)
	for _, locale := range []string{"en-US", "zh-CN"} {
		t.Run(locale, func(t *testing.T) {
			t.Setenv("LC_ALL", locale)
			sentinel := fmt.Errorf("sentinel")
			inner := ErrorfCode(innerCode, "invalid value: %w", "值无效：%w", sentinel)
			err := ErrorfCode(outerCode, "outer context: %w", "外层上下文：%w", inner)
			joined := errors.Join(ErrorfCode("test.first", "first", "第一项"), err, ErrorfCode(joinedCode, "joined", "合并项"))
			for _, code := range []ErrorCode{outerCode, innerCode, joinedCode} {
				if !HasCode(joined, code) {
					t.Fatalf("joined error = %v, want code %q", joined, code)
				}
			}
			if HasCode(joined, "test.other") {
				t.Fatalf("joined error unexpectedly matched a different code: %v", joined)
			}
			if !errors.Is(joined, sentinel) {
				t.Fatalf("coded wrapping did not preserve sentinel: %v", joined)
			}
		})
	}
}

func TestCodedErrorRendering(t *testing.T) {
	for _, test := range []struct {
		name   string
		locale string
		want   string
	}{
		{name: "english", locale: "en-US", want: `invalid value "demo"`},
		{name: "chinese", locale: "zh-CN", want: `值 "demo" 无效`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LC_ALL", test.locale)
			err := ErrorfCode("test.invalid_value", `invalid value %q`, `值 %q 无效`, "demo")
			if got := err.Error(); got != test.want {
				t.Fatalf("rendered error = %q, want %q", got, test.want)
			}
		})
	}
}

package i18n

import "testing"

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

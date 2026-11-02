package i18n

import "testing"

func TestRemovedChineseLanguageFallsBackToEnglish(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "vi", "zhCN", "zh-CN", "zh-TW", "unknown"} {
		if normalized := normalizeLang(lang); normalized != LangEn {
			t.Errorf("%s normalized to %s, want English backend fallback", lang, normalized)
		}
	}
	if got := SupportedLanguages(); len(got) != 1 || got[0] != LangEn {
		t.Fatalf("unexpected backend translation bundles: %v", got)
	}
}

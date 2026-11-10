package japanese

import "testing"

func TestKanaToRomaji(t *testing.T) {
	cases := map[string]string{
		// Same as pykakasi's hepburn output.
		"とうきょう": "toukyou", "ラーメン": "raamen", "がっこう": "gakkou", "きって": "kitte",
		"しんぶん": "shinbun", "ちゃ": "cha", "じゃ": "ja", "ふ": "fu", "つ": "tsu", "ぢ": "ji", "づ": "zu",
		"を": "wo", "ヴァイオリン": "vaiorin", "さんぽ": "sanpo", "ほんや": "hon'ya", "ディ": "di",
		"ファ": "fa", "ぁ": "a", "ゐ": "i", "ゑ": "e", "ゔ": "vu", "ん": "n", "おねえさん": "oneesan",
		"せんえん": "sen'en", "きゃっきゃ": "kyakkya", "、": ",",
		// Hepburn where pykakasi differs (it gave kinyoubi, tei, ui, atsu).
		"きんようび": "kin'youbi", "ティ": "ti", "ウィ": "wi", "あっ": "a",
		"まっちゃ": "matcha", "コーヒー": "koohii", "ハロー world": "haroo world", "ｶﾀｶﾅ": "katakana",
	}
	for in, want := range cases {
		if got := KanaToRomaji(in); got != want {
			t.Errorf("KanaToRomaji(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToHiragana(t *testing.T) {
	cases := map[string]string{
		"東京":      "とうきょう",
		"愛してる":    "あいしてる",
		"君の名は":    "きみのなは",
		"カタカナ":    "かたかな",
		"Hello世界": "Helloせかい",
		"12時":     "12じ",
	}
	for in, want := range cases {
		if got := ToHiragana(in); got != want {
			t.Errorf("ToHiragana(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRomanizeLine(t *testing.T) {
	words := []Word{
		{"君", "きみ", "pronoun"}, {"の", "の", "particle"}, {"名", "な", "noun"}, {"は", "は", "particle"},
		{"、", "", "other"}, {"東京", "", "noun"}, {"へ", "へ", "particle"}, {"こんにちは", "", "interjection"},
		{"は", "は", "noun"},
	}
	if got, want := RomanizeLine(words), "kimi no na wa , toukyou e konnichiwa ha"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

package japanese

import (
	"strings"
	"sync"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var (
	tokenizerOnce sync.Once
	tok           *tokenizer.Tokenizer
	tokenizerErr  error
)

// The dictionary takes a moment and some memory to load, so only load it when Japanese is used.
func getTokenizer() (*tokenizer.Tokenizer, error) {
	tokenizerOnce.Do(func() {
		tok, tokenizerErr = tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	})
	return tok, tokenizerErr
}

// ToHiragana reads Japanese text as hiragana: kanji by their dictionary reading, katakana
// converted; anything the dictionary doesn't know (Latin letters, digits, symbols) is kept.
func ToHiragana(text string) string {
	t, err := getTokenizer()
	if err != nil {
		return KatakanaToHiragana(text)
	}
	var out strings.Builder
	for _, token := range t.Tokenize(text) {
		if reading, ok := token.Reading(); ok && reading != "*" && reading != "" {
			out.WriteString(KatakanaToHiragana(reading))
		} else {
			out.WriteString(KatakanaToHiragana(token.Surface))
		}
	}
	return out.String()
}

// ToRomaji converts Japanese text (kana, or kanji via their reading) to Hepburn romaji.
func ToRomaji(text string) string {
	if !hasKanji(text) {
		return KanaToRomaji(text)
	}
	return KanaToRomaji(ToHiragana(text))
}

func hasKanji(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || r == '々' {
			return true
		}
	}
	return false
}

// Particles whose kana spelling differs from how they are pronounced.
var particleRomaji = map[string]string{"は": "wa", "へ": "e", "を": "o"}

// Set phrases that end in a fossilized particle は.
var wordRomaji = map[string]string{"こんにちは": "konnichiwa", "こんばんは": "konbanwa"}

// Word is the part of an annotation token that romanization needs.
type Word struct {
	Surface, Reading, Pos string
}

// RomanizeLine joins the romaji of each token with spaces, reading particles as pronounced.
func RomanizeLine(words []Word) string {
	var parts []string
	for _, w := range words {
		kana := w.Reading
		if kana == "" {
			kana = w.Surface
		}
		var romaji string
		if r, ok := wordRomaji[kana]; ok {
			romaji = r
		} else if r, ok := particleRomaji[kana]; ok && w.Pos == "particle" {
			romaji = r
		} else {
			romaji = ToRomaji(kana)
		}
		if strings.TrimSpace(romaji) != "" {
			parts = append(parts, romaji)
		}
	}
	return strings.Join(parts, " ")
}

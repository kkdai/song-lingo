// Package japanese converts Japanese text to hiragana and Hepburn romaji (replacing pykakasi).
package japanese

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// KatakanaToHiragana maps katakana (including ヴ) to hiragana; other characters are kept.
func KatakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, s)
}

// Two-kana combinations (yōon and the small vowels used in loanwords).
var digraphs = map[string]string{
	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo",
	"ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"しゃ": "sha", "しゅ": "shu", "しょ": "sho", "しぇ": "she",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo", "じぇ": "je",
	"ぢゃ": "ja", "ぢゅ": "ju", "ぢょ": "jo",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho", "ちぇ": "che",
	"にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo",
	"びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo",
	"みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo",
	"ふぁ": "fa", "ふぃ": "fi", "ふぇ": "fe", "ふぉ": "fo", "ふゅ": "fyu",
	"てぃ": "ti", "でぃ": "di", "とぅ": "tu", "どぅ": "du", "てゅ": "tyu", "でゅ": "dyu",
	"うぃ": "wi", "うぇ": "we", "うぉ": "wo", "いぇ": "ye",
	"ゔぁ": "va", "ゔぃ": "vi", "ゔぇ": "ve", "ゔぉ": "vo", "ゔゅ": "vyu",
	"つぁ": "tsa", "つぃ": "tsi", "つぇ": "tse", "つぉ": "tso",
	"くぁ": "kwa", "ぐぁ": "gwa",
}

var monographs = map[rune]string{
	'あ': "a", 'い': "i", 'う': "u", 'え': "e", 'お': "o",
	'か': "ka", 'き': "ki", 'く': "ku", 'け': "ke", 'こ': "ko",
	'が': "ga", 'ぎ': "gi", 'ぐ': "gu", 'げ': "ge", 'ご': "go",
	'さ': "sa", 'し': "shi", 'す': "su", 'せ': "se", 'そ': "so",
	'ざ': "za", 'じ': "ji", 'ず': "zu", 'ぜ': "ze", 'ぞ': "zo",
	'た': "ta", 'ち': "chi", 'つ': "tsu", 'て': "te", 'と': "to",
	'だ': "da", 'ぢ': "ji", 'づ': "zu", 'で': "de", 'ど': "do",
	'な': "na", 'に': "ni", 'ぬ': "nu", 'ね': "ne", 'の': "no",
	'は': "ha", 'ひ': "hi", 'ふ': "fu", 'へ': "he", 'ほ': "ho",
	'ば': "ba", 'び': "bi", 'ぶ': "bu", 'べ': "be", 'ぼ': "bo",
	'ぱ': "pa", 'ぴ': "pi", 'ぷ': "pu", 'ぺ': "pe", 'ぽ': "po",
	'ま': "ma", 'み': "mi", 'む': "mu", 'め': "me", 'も': "mo",
	'や': "ya", 'ゆ': "yu", 'よ': "yo",
	'ら': "ra", 'り': "ri", 'る': "ru", 'れ': "re", 'ろ': "ro",
	'わ': "wa", 'ゐ': "i", 'ゑ': "e", 'を': "wo", 'ん': "n",
	'ゔ': "vu",
	'ぁ': "a", 'ぃ': "i", 'ぅ': "u", 'ぇ': "e", 'ぉ': "o",
	'ゃ': "ya", 'ゅ': "yu", 'ょ': "yo", 'ゎ': "wa", 'ゕ': "ka", 'ゖ': "ke",
}

var punctuation = map[rune]string{
	'、': ",", '。': ".", '・': " ", '「': "\"", '」': "\"", '『': "\"", '』': "\"",
	'〜': "~", '～': "~", '…': "...", '　': " ",
}

func isVowel(b byte) bool { return strings.IndexByte("aeiou", b) >= 0 }

// KanaToRomaji converts kana to Hepburn romaji: long vowels as written (とうきょう → toukyou,
// ー repeats the vowel), っ doubles the next consonant, ん before a vowel or y is "n'".
// Characters that aren't kana are kept, normalized to half width.
func KanaToRomaji(kana string) string {
	s := []rune(KatakanaToHiragana(norm.NFKC.String(kana)))
	var out strings.Builder
	geminate := false
	for i := 0; i < len(s); i++ {
		afterN := i > 0 && s[i-1] == 'ん'
		var syllable string
		if i+1 < len(s) {
			if d, ok := digraphs[string(s[i:i+2])]; ok {
				syllable = d
				i++
			}
		}
		if syllable == "" {
			r := s[i]
			switch {
			case r == 'っ':
				geminate = true
				continue
			case r == 'ー':
				if prev := out.String(); prev != "" && isVowel(prev[len(prev)-1]) {
					out.WriteByte(prev[len(prev)-1])
				}
				continue
			case monographs[r] != "":
				syllable = monographs[r]
			case punctuation[r] != "":
				syllable = punctuation[r]
			default:
				syllable = norm.NFKC.String(string(r))
			}
		}
		if syllable == "" {
			continue
		}
		first := syllable[0]
		if geminate {
			geminate = false
			if strings.HasPrefix(syllable, "ch") {
				out.WriteByte('t')
			} else if first >= 'a' && first <= 'z' && !isVowel(first) && first != 'n' {
				out.WriteByte(first)
			}
		}
		if afterN && (isVowel(first) || first == 'y') {
			out.WriteByte('\'')
		}
		out.WriteString(syllable)
	}
	return out.String()
}

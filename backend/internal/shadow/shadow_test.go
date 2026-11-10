package shadow

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// testdata/difflib.json holds Python's SequenceMatcher(None, a, b, autojunk=False).get_opcodes()
// for random strings and word lists.
func TestOpcodesMatchPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/difflib.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		A, B    string
		Aw, Bw  []string
		Opcodes [][]any
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		var got []Opcode
		if c.Aw != nil || c.Bw != nil {
			got = Opcodes(c.Aw, c.Bw)
		} else {
			got = Opcodes([]rune(c.A), []rune(c.B))
		}
		want := make([]Opcode, len(c.Opcodes))
		for i, op := range c.Opcodes {
			want[i] = Opcode{op[0].(string), int(op[1].(float64)), int(op[2].(float64)), int(op[3].(float64)), int(op[4].(float64))}
		}
		if len(want) == 0 {
			want = nil
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q/%q %v/%v:\n got %v\nwant %v", c.A, c.B, c.Aw, c.Bw, got, want)
		}
	}
}

// testdata/scores.json holds shadow.py's score() for lines without kanji to read, where the
// Python (pykakasi) and Go (kagome) readings agree.
func TestScoreMatchesPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/scores.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Expected   Expected
		Transcript string
		Result     Result
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Score(c.Expected, c.Transcript); !reflect.DeepEqual(got, c.Result) {
			t.Errorf("%+v / %q:\n got %+v\nwant %+v", c.Expected, c.Transcript, got, c.Result)
		}
	}
}

// Transcripts often write words in kanji; they are compared by reading.
func TestScoreReadsKanji(t *testing.T) {
	expected := Expected{Language: "ja", Text: "東京へ行こう", Tokens: []Token{
		{"東京", "とうきょう"}, {"へ", "へ"}, {"行こう", "いこう"}}}
	for transcript, want := range map[string]int{"東京へ行こう": 100, "とうきょうへいこう": 100, "東京へ": 67, "京都へ行こう": 44} {
		if got := Score(expected, transcript).Score; got != want {
			t.Errorf("%s: %d, want %d", transcript, got, want)
		}
	}
}

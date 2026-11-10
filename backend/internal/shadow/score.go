// Package shadow scores a shadowing attempt: the learner's recording of one line is transcribed
// and compared with that line, word by word.
package shadow

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/kkdai/song-lingo/backend/internal/japanese"
)

type Token struct {
	Surface string `json:"surface"`
	Reading string `json:"reading"`
}

// Expected is the line the learner tried to say.
type Expected struct {
	Language string  `json:"language"`
	Text     string  `json:"text"`
	Reading  string  `json:"reading"`
	Tokens   []Token `json:"tokens"`
}

type Word struct {
	Surface string `json:"surface"`
	// "ok" = every character heard; "missing" = none of it was said; "wrong" = said, but differently.
	Status string `json:"status"`
}

type Result struct {
	Transcript string `json:"transcript"`
	Score      int    `json:"score"`
	Words      []Word `json:"words"`
}

// Fold drops punctuation, symbols and spacing and folds width and case, so only the spoken
// content is compared.
func Fold(text string) string {
	text = strings.ToLower(norm.NFKC.String(text))
	return strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.P, unicode.S, unicode.Z) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
}

// charStates aligns the transcript to expected; per expected character: "equal", "replace" or
// "delete".
func charStates(expected, actual []rune) []string {
	states := make([]string, len(expected))
	for i := range states {
		states[i] = "delete"
	}
	for _, op := range Opcodes(expected, actual) {
		if op.Tag == "equal" || op.Tag == "replace" {
			for i := op.I1; i < op.I2; i++ {
				states[i] = op.Tag
			}
		}
	}
	return states
}

func tokenStatuses(parts [][]rune, states []string) []string {
	statuses := make([]string, len(parts))
	pos := 0
	for n, part := range parts {
		span := states[pos : pos+len(part)]
		pos += len(part)
		all := func(s string) bool {
			for _, st := range span {
				if st != s {
					return false
				}
			}
			return true
		}
		switch {
		case all("equal"):
			statuses[n] = "ok"
		case all("delete"):
			statuses[n] = "missing"
		default:
			statuses[n] = "wrong"
		}
	}
	return statuses
}

func concat(parts [][]rune) []rune {
	var all []rune
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

// pyRound rounds half to even, like Python's round().
func pyRound(x float64) int { return int(math.RoundToEven(x)) }

// Score compares a transcript of the learner's recording with the expected line.
func Score(expected Expected, transcript string) Result {
	var surfaces, statuses []string
	var weights []int
	if expected.Language == "ja" {
		tokens := expected.Tokens
		if len(tokens) == 0 {
			tokens = []Token{{Surface: expected.Text, Reading: expected.Reading}}
		}
		readings := make([][]rune, len(tokens))
		written := make([][]rune, len(tokens))
		for n, t := range tokens {
			surfaces = append(surfaces, t.Surface)
			reading := t.Reading
			if reading == "" {
				reading = t.Surface
			}
			readings[n] = []rune(japanese.ToHiragana(Fold(reading)))
			written[n] = []rune(Fold(t.Surface))
			weights = append(weights, max(len(readings[n]), 1))
		}
		// Compare both by reading (transcripts may write a word in kana or different kanji) and by
		// written form (guards against misreading a kanji); either match counts.
		byReading := tokenStatuses(readings, charStates(concat(readings), []rune(japanese.ToHiragana(Fold(transcript)))))
		bySurface := tokenStatuses(written, charStates(concat(written), []rune(Fold(transcript))))
		for n := range tokens {
			if byReading[n] == "ok" || bySurface[n] == "ok" {
				statuses = append(statuses, "ok")
			} else {
				statuses = append(statuses, byReading[n])
			}
		}
	} else {
		var target, spoken []string
		for _, w := range strings.Fields(expected.Text) {
			if f := Fold(w); f != "" {
				surfaces = append(surfaces, w)
				target = append(target, f)
				weights = append(weights, 1)
			}
		}
		for _, w := range strings.Fields(transcript) {
			if f := Fold(w); f != "" {
				spoken = append(spoken, f)
			}
		}
		statuses = make([]string, len(target))
		for i := range statuses {
			statuses[i] = "wrong"
		}
		for _, op := range Opcodes(target, spoken) {
			if op.Tag == "equal" || op.Tag == "delete" {
				status := "ok"
				if op.Tag == "delete" {
					status = "missing"
				}
				for i := op.I1; i < op.I2; i++ {
					statuses[i] = status
				}
			}
		}
	}

	total, correct := 0, 0
	words := make([]Word, len(surfaces))
	for n := range surfaces {
		total += weights[n]
		if statuses[n] == "ok" {
			correct += weights[n]
		}
		words[n] = Word{Surface: surfaces[n], Status: statuses[n]}
	}
	return Result{Transcript: transcript, Score: pyRound(100 * float64(correct) / float64(max(total, 1))), Words: words}
}

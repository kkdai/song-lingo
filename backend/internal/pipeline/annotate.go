package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/japanese"
	"github.com/kkdai/song-lingo/backend/internal/songs"
)

const annotatePrompt = `You are a friendly language teacher helping a Taiwanese learner study a {language_name} song.
Below are the unique lines of the song, each with an index. For EVERY line, return one entry with the
same index.

For each line:
- translation_zh: natural Traditional Chinese (Taiwan) translation that keeps the line's feeling.
- tokens: {token_rule}
- romanization: {romanization_rule}
- grammar_note: one short note in Traditional Chinese about the most useful grammar point or
  expression in this line (empty string if nothing notable).
- pronunciation_tip: one short tip in Traditional Chinese about how to pronounce this line
  (e.g. long vowels, sound changes, linking); empty string if nothing notable.

Lines:
{lines}
`

type langRules struct{ languageName, tokenRule, romanizationRule string }

var langRulesByCode = map[string]langRules{
	"ja": {
		languageName: "Japanese",
		tokenRule: "split the line into words in order, covering the whole line. For each token give " +
			"surface (exactly as written), reading (hiragana, as sung), pos (one of: noun, verb, " +
			"adjective, adverb, particle, auxiliary, pronoun, conjunction, interjection, other) and " +
			"meaning_zh (short Traditional Chinese gloss).",
		romanizationRule: "empty string (computed separately).",
	},
	"ko": {
		languageName: "Korean",
		tokenRule: "split the line into words (eojeol) in order, covering the whole line. For each token " +
			"give surface, reading (empty string), pos, and meaning_zh (short Traditional Chinese gloss).",
		romanizationRule: "Revised Romanization of Korean for the whole line, applying actual pronunciation " +
			"rules (liaison, nasalization, etc.), e.g. 감사합니다 -> gamsahamnida.",
	},
}

func rulesFor(language string) (langRules, bool) {
	if r, ok := langRulesByCode[language]; ok {
		return r, true
	}
	return langRules{
		languageName: language,
		tokenRule: "only the notable vocabulary, idioms or phrasal verbs in the line (can be empty). Give " +
			"surface, reading (empty string), pos, and meaning_zh.",
		romanizationRule: "empty string.",
	}, false
}

var annotationSchema = object(map[string]any{
	"lines": array(object(map[string]any{
		"index":          map[string]any{"type": "integer"},
		"translation_zh": str(""),
		"tokens": array(object(map[string]any{
			"surface":    str(""),
			"reading":    str(""),
			"pos":        str(""),
			"meaning_zh": str(""),
		}, "surface"), ""),
		"romanization":      str(""),
		"grammar_note":      str(""),
		"pronunciation_tip": str(""),
	}, "index", "translation_zh", "tokens"), "One entry per input line, same index"),
}, "lines")

type lineNote struct {
	Index            int           `json:"index"`
	TranslationZh    string        `json:"translation_zh"`
	Tokens           []songs.Token `json:"tokens"`
	Romanization     string        `json:"romanization"`
	GrammarNote      string        `json:"grammar_note"`
	PronunciationTip string        `json:"pronunciation_tip"`
}

func buildPrompt(texts []string, language string) string {
	rules, _ := rulesFor(language)
	var lines strings.Builder
	for i, text := range texts {
		if i > 0 {
			lines.WriteByte('\n')
		}
		fmt.Fprintf(&lines, "%d: %s", i, text)
	}
	return strings.NewReplacer(
		"{language_name}", rules.languageName,
		"{token_rule}", rules.tokenRule,
		"{romanization_rule}", rules.romanizationRule,
		"{lines}", lines.String(),
	).Replace(annotatePrompt)
}

// AnnotateResult summarizes an annotation run.
type AnnotateResult struct {
	FinishReason string
	Unique       int
	Missing      []int
	// Lines whose tokens don't spell out the whole line (ja/ko only; -1 otherwise).
	TokenMismatch int
	// Lines flagged for review (ja only; -1 otherwise).
	NeedsReview int
}

func (r AnnotateResult) String() string {
	lines := []string{
		"finish_reason: " + r.FinishReason,
		fmt.Sprintf("annotated: %d/%d  missing indexes: %v", r.Unique-len(r.Missing), r.Unique, r.Missing),
	}
	if r.TokenMismatch >= 0 {
		lines = append(lines, fmt.Sprintf("lines where tokens don't cover the text: %d", r.TokenMismatch))
	}
	if r.NeedsReview >= 0 {
		lines = append(lines, fmt.Sprintf("lines needing review: %d", r.NeedsReview))
	}
	return strings.Join(lines, "\n")
}

// Annotate asks Gemini about each unique line once (choruses repeat) and returns the annotated
// song: the transcript plus translation, tokens, romanization and notes on every line.
func Annotate(ctx context.Context, client *genai.Client, model string, transcript *songs.File) (*songs.File, AnnotateResult, error) {
	texts := songs.UniqueTexts(transcript.Lines)
	var out struct {
		Lines []lineNote `json:"lines"`
	}
	finish, err := generateJSON(ctx, client, model, []*genai.Part{{Text: buildPrompt(texts, transcript.Language)}}, annotationSchema, &out)
	if errors.Is(err, errRecitation) {
		return nil, AnnotateResult{}, &BlockedError{What: "歌詞的分析"}
	}
	if err != nil {
		return nil, AnnotateResult{}, err
	}
	annotated, result := applyNotes(transcript, texts, out.Lines)
	result.FinishReason = finish
	return annotated, result, nil
}

func stripSpaces(s string) string { return strings.Join(strings.Fields(s), "") }

// applyNotes merges the model's notes into a copy of the transcript, keeping what the user fixed
// in the review UI (manual translations, reviewed lines).
func applyNotes(transcript *songs.File, texts []string, notes []lineNote) (*songs.File, AnnotateResult) {
	language := transcript.Language
	_, knownLanguage := rulesFor(language)
	result := AnnotateResult{Unique: len(texts), Missing: []int{}, TokenMismatch: -1, NeedsReview: -1}
	if knownLanguage {
		result.TokenMismatch = 0
	}

	byIndex := map[int]lineNote{}
	for _, n := range notes {
		byIndex[n.Index] = n
	}
	// The transcript's reading of the first line with each text.
	lineReading := map[string]string{}
	for i := len(transcript.Lines) - 1; i >= 0; i-- {
		lineReading[transcript.Lines[i].Text] = transcript.Lines[i].ReadingOrEmpty()
	}

	type entry struct {
		note        lineNote
		needsReview *bool
	}
	byText := map[string]entry{}
	for i, text := range texts {
		note, ok := byIndex[i]
		if !ok {
			result.Missing = append(result.Missing, i)
			continue
		}
		if note.Tokens == nil {
			note.Tokens = []songs.Token{}
		}
		e := entry{note: note}
		if language == "ja" {
			words := make([]japanese.Word, len(note.Tokens))
			var readings strings.Builder
			for j, t := range note.Tokens {
				words[j] = japanese.Word{Surface: t.Surface, Reading: t.Reading, Pos: t.Pos}
				readings.WriteString(t.Reading)
			}
			e.note.Romanization = japanese.RomanizeLine(words)
			// Transcription and annotation produce readings independently; disagreement flags a
			// likely misread kanji.
			mismatch := stripSpaces(readings.String()) != stripSpaces(lineReading[text])
			e.needsReview = &mismatch
		}
		if knownLanguage {
			var surfaces strings.Builder
			for _, t := range note.Tokens {
				surfaces.WriteString(t.Surface)
			}
			if stripSpaces(surfaces.String()) != stripSpaces(text) {
				result.TokenMismatch++
			}
		}
		byText[text] = e
	}

	annotated := *transcript
	annotated.Lines = make([]songs.Line, len(transcript.Lines))
	for i, line := range transcript.Lines {
		if e, ok := byText[line.Text]; ok {
			n := e.note
			line.TranslationZh = &n.TranslationZh
			line.Tokens = n.Tokens
			line.Romanization = &n.Romanization
			line.GrammarNote = &n.GrammarNote
			line.PronunciationTip = &n.PronunciationTip
			if e.needsReview != nil {
				line.NeedsReview = e.needsReview
			}
		}
		line.Stale = false
		if line.TranslationZhManual != "" {
			manual := line.TranslationZhManual
			line.TranslationZh = &manual
		}
		if line.Reviewed != nil && *line.Reviewed {
			no := false
			line.NeedsReview = &no
			line.Uncertain = false
		}
		annotated.Lines[i] = line
	}
	if language == "ja" {
		result.NeedsReview = 0
		for _, l := range annotated.Lines {
			if l.NeedsReview != nil && *l.NeedsReview {
				result.NeedsReview++
			}
		}
	}
	return &annotated, result
}

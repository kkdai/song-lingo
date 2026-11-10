// Package progress stores what the learner has done in each song.
//
// Lines are keyed by a hash of their text (songs.TextHash), like the teacher-audio cache: repeated
// choruses share one entry, an edit to a line's text carries its entry over, and no lyrics are
// copied into the progress store.
package progress

import (
	"regexp"
	"sort"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// MaxAttempts is how many shadowing attempts are kept per line; older ones are dropped.
const MaxAttempts = 20

type ShadowAttempt struct {
	At    string `json:"at" firestore:"at"`
	Score int    `json:"score" firestore:"score"`
	// Words that weren't heard as written (for "words I keep missing").
	Missed []string `json:"missed" firestore:"missed"`
}

type LineProgress struct {
	Learned bool            `json:"learned,omitempty" firestore:"learned,omitempty"`
	Shadow  []ShadowAttempt `json:"shadow,omitempty" firestore:"shadow,omitempty"`
}

type Progress struct {
	Version       int                      `json:"version" firestore:"version"`
	LastLine      int                      `json:"lastLine" firestore:"lastLine"`
	LastStudiedAt *string                  `json:"lastStudiedAt" firestore:"lastStudiedAt"`
	Lines         map[string]*LineProgress `json:"lines" firestore:"lines"`
}

type Summary struct {
	Learned       int     `json:"learned"`
	Total         int     `json:"total"`
	LastLine      int     `json:"lastLine"`
	LastStudiedAt *string `json:"lastStudiedAt"`
}

func Empty() *Progress {
	return &Progress{Version: 1, Lines: map[string]*LineProgress{}}
}

var LineKey = songs.TextHash

var lineKeyPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// withDefaults fills in missing fields and re-keys entries from early versions, which used the
// line's raw text as the key.
func withDefaults(p *Progress) *Progress {
	if p == nil {
		return Empty()
	}
	if p.Version == 0 {
		p.Version = 1
	}
	if p.Lines == nil {
		p.Lines = map[string]*LineProgress{}
	}
	for key, value := range p.Lines {
		if value == nil {
			delete(p.Lines, key)
			continue
		}
		if lineKeyPattern.MatchString(key) {
			continue
		}
		delete(p.Lines, key)
		merged := &LineProgress{Learned: value.Learned}
		if existing := p.Lines[LineKey(key)]; existing != nil {
			merged.Learned = merged.Learned || existing.Learned
			merged.Shadow = append(merged.Shadow, existing.Shadow...)
		}
		merged.Shadow = append(merged.Shadow, value.Shadow...)
		sort.SliceStable(merged.Shadow, func(i, j int) bool { return merged.Shadow[i].At < merged.Shadow[j].At })
		p.Lines[LineKey(key)] = merged
	}
	return p
}

func (p *Progress) entry(text string) *LineProgress {
	key := LineKey(text)
	if p.Lines[key] == nil {
		p.Lines[key] = &LineProgress{}
	}
	return p.Lines[key]
}

func (p *Progress) SetLearned(text string, learned bool) { p.entry(text).Learned = learned }

// AddAttempt records a shadowing attempt, keeping only the latest MaxAttempts.
func (p *Progress) AddAttempt(text string, attempt ShadowAttempt) {
	if attempt.Missed == nil {
		attempt.Missed = []string{}
	}
	e := p.entry(text)
	e.Shadow = append(e.Shadow, attempt)
	if len(e.Shadow) > MaxAttempts {
		e.Shadow = e.Shadow[len(e.Shadow)-MaxAttempts:]
	}
}

func Summarize(p *Progress, lines []songs.Line) Summary {
	learned := 0
	for _, l := range lines {
		if e := p.Lines[LineKey(l.Text)]; e != nil && e.Learned {
			learned++
		}
	}
	return Summary{Learned: learned, Total: len(lines), LastLine: p.LastLine, LastStudiedAt: p.LastStudiedAt}
}

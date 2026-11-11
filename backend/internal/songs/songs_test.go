package songs

import (
	"os"
	"path/filepath"
	"testing"
)

// The hashes name existing audio clips and progress entries, so they must match what the
// TypeScript (createHash("sha1")...slice(0, 16)) and Python versions produced.
func TestTextHashMatchesNode(t *testing.T) {
	for text, want := range map[string]string{
		"君の名は":        "4411c7fef82a82f8",
		"사랑해요":        "d9853514f68817ca",
		"hello world": "2aae6c35c94fcfb4",
		"":            "da39a3ee5e6b4b0d",
	} {
		if got := TextHash(text); got != want {
			t.Errorf("TextHash(%q) = %s, want %s", text, got, want)
		}
	}
	if got := ClipName("君の名は", "slow"); got != "4411c7fef82a82f8_slow.wav" {
		t.Errorf("ClipName = %s", got)
	}
}

func TestRoundTripKeepsFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.annotated.json")
	reading, empty := "きみ", ""
	no := false
	in := &File{Language: "ja", Source: "audio", Lines: []Line{{
		Start: "00:01", End: "00:02", Text: "君", Reading: &reading, GrammarNote: &empty, NeedsReview: &no,
	}}}
	if err := WriteFile(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l := out.Lines[0]
	if l.GrammarNote == nil || *l.GrammarNote != "" || l.NeedsReview == nil || *l.NeedsReview || l.Section != nil {
		t.Errorf("fields not kept: %+v", l)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.json")); err != ErrNotFound {
		t.Errorf("missing file: %v", err)
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "sub")
	path := filepath.Join(dir, "a.json")
	if err := WriteAtomic(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("y")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
	if got, _ := os.ReadFile(path); string(got) != "y" {
		t.Errorf("content = %q", got)
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmps) != 0 {
		t.Errorf("leftover temp files: %v", tmps)
	}
}

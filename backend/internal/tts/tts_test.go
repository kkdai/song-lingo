package tts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestExtractAudio(t *testing.T) {
	cases := []struct {
		resp map[string]any
		want string
	}{
		{map[string]any{"steps": []any{
			map[string]any{"type": "user_input", "content": []any{map[string]any{"type": "audio", "data": "IN"}}},
			map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "audio", "data": "A"}, map[string]any{"type": "audio", "data": "B"}}},
			map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "text", "text": "x"}}},
		}}, "B"},
		// Stops at the latest user input instead of returning audio from an earlier turn.
		{map[string]any{"steps": []any{
			map[string]any{"type": "model_output", "content": []any{map[string]any{"type": "audio", "data": "OLD"}}},
			map[string]any{"type": "user_input"},
		}}, ""},
		{map[string]any{"outputs": []any{map[string]any{"type": "audio", "data": "O"}}}, "O"},
		{map[string]any{"output_audio": map[string]any{"data": "X"}}, "X"},
	}
	for i, c := range cases {
		if got := ExtractAudio(c.resp); got != c.want {
			t.Errorf("case %d: %q, want %q", i, got, c.want)
		}
	}
}

func TestToWAV(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	wav := ToWAV(pcm, 24000)
	if len(wav) != 48 || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || !bytes.Equal(wav[44:], pcm) {
		t.Fatalf("header = %v", wav[:44])
	}
	if binary.LittleEndian.Uint32(wav[24:]) != 24000 || binary.LittleEndian.Uint32(wav[40:]) != 4 {
		t.Error("bad rate or size")
	}
	if !bytes.Equal(ToWAV(wav, 24000), wav) {
		t.Error("re-wrapped a WAV")
	}
}

func TestClipPattern(t *testing.T) {
	p := ClipPattern([]string{"normal", "slow"})
	for name, want := range map[string]bool{
		"0123456789abcdef_slow.wav": true, "0123456789abcdef_fast.wav": false,
		"0123456789ABCDEF_slow.wav": false, "../x_slow.wav": false,
	} {
		if p.MatchString(name) != want {
			t.Errorf("%s", name)
		}
	}
}

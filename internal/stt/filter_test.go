package stt

import "testing"

func TestFilter(t *testing.T) {
	loud := speech(1)
	quiet := silence(1)
	tests := []struct {
		name    string
		text    string
		samples []float32
		want    string
	}{
		{"blank audio", "[BLANK_AUDIO]", quiet, ""},
		{"music annotation", "(music)", loud, ""},
		{"empty result", "", loud, ""},
		{"whitespace only", "   \n ", loud, ""},
		{"punctuation only", " ... ", loud, ""},
		{"annotation around speech", "[MUSIC] hello there", loud, "hello there"},
		{"normal speech kept", "Thank you for coming today", loud, "Thank you for coming today"},
		{"filler over silence dropped", "Thank you.", quiet, ""},
		{"filler dutch subtitles over silence dropped", "Ondertiteld door de Amara.org gemeenschap", quiet, ""},
		{"filler over real speech kept", "Thank you.", loud, "Thank you."},
		{"real sentence over quiet audio kept", "see you tomorrow at nine", quiet, "see you tomorrow at nine"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Filter(tc.text, tc.samples); got != tc.want {
				t.Fatalf("Filter(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

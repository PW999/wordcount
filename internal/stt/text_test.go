package stt

import "testing"

func TestCleanTextAndCountWords(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantWords int
	}{
		{"plain", "hello world", "hello world", 2},
		{"extra whitespace", "  hello \n  world  ", "hello world", 2},
		{"blank audio", "[BLANK_AUDIO]", "", 0},
		{"parenthesised", "(music)", "", 0},
		{"asterisks", "*noise*", "", 0},
		{"mixed", "hello [BLANK_AUDIO] there (laughs) friend *cough*", "hello there friend", 3},
		{"annotation glued to word", "so[pause]what", "so what", 2},
		{"dutch accented", "Ik heb één café bezocht, naïef genoeg", "Ik heb één café bezocht, naïef genoeg", 7},
		{"punctuation only tokens", "hello - ... ?! world", "hello - ... ?! world", 2},
		{"digits", "ik ben 42 jaar", "ik ben 42 jaar", 4},
		{"empty", "", "", 0},
		{"only annotations", "[MUSIC] (applause) *beep*", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanText(tc.in); got != tc.wantClean {
				t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.wantClean)
			}
			if got := CountWords(tc.in); got != tc.wantWords {
				t.Errorf("CountWords(%q) = %d, want %d", tc.in, got, tc.wantWords)
			}
		})
	}
}

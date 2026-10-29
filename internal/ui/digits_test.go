package ui

import (
	"strings"
	"testing"
)

// Golden renderings of each digit, written out in full.
var goldenDigits = map[int][]string{
	0: {"██████", "██  ██", "██  ██", "██  ██", "██████"},
	1: {"  ██  ", "████  ", "  ██  ", "  ██  ", "██████"},
	2: {"██████", "    ██", "██████", "██    ", "██████"},
	3: {"██████", "    ██", "██████", "    ██", "██████"},
	4: {"██  ██", "██  ██", "██████", "    ██", "    ██"},
	5: {"██████", "██    ", "██████", "    ██", "██████"},
	6: {"██████", "██    ", "██████", "██  ██", "██████"},
	7: {"██████", "    ██", "    ██", "    ██", "    ██"},
	8: {"██████", "██  ██", "██████", "██  ██", "██████"},
	9: {"██████", "██  ██", "██████", "    ██", "██████"},
}

func TestBigDigitsGolden(t *testing.T) {
	for d := 0; d <= 9; d++ {
		got := BigDigits(d)
		want := goldenDigits[d]
		if len(got) != DigitRows {
			t.Fatalf("digit %d has %d rows, want %d", d, len(got), DigitRows)
		}
		for r := range want {
			if got[r] != want[r] {
				t.Errorf("digit %d row %d = %q, want %q", d, r, got[r], want[r])
			}
		}
	}
}

func TestBigDigitsMultiDigit(t *testing.T) {
	got := BigDigits(150)
	for r, row := range got {
		want := goldenDigits[1][r] + "  " + goldenDigits[5][r] + "  " + goldenDigits[0][r]
		if row != want {
			t.Errorf("row %d = %q, want %q", r, row, want)
		}
	}
}

func TestBigDigitsNegativeClampsToZero(t *testing.T) {
	if got := BigDigits(-5); got[0] != goldenDigits[0][0] {
		t.Fatalf("negative should render 0, got %q", got[0])
	}
}

func TestRenderWPMBigAt80x24(t *testing.T) {
	out := RenderWPM(42, 80, 24)
	if rows := strings.Split(out, "\n"); len(rows) != DigitRows {
		t.Fatalf("got %d rows, want %d", len(rows), DigitRows)
	}
	if !strings.Contains(out, "█") {
		t.Fatal("big output should use block characters")
	}
}

func TestRenderWPMFallsBackBelow80x24(t *testing.T) {
	for _, size := range [][2]int{{79, 24}, {80, 23}, {40, 10}} {
		out := RenderWPM(42, size[0], size[1])
		if strings.Contains(out, "\n") || strings.Contains(out, "█") {
			t.Fatalf("%dx%d should fall back to one line, got %q", size[0], size[1], out)
		}
		if !strings.Contains(out, "42 WPM") {
			t.Fatalf("%dx%d fallback %q should contain the value", size[0], size[1], out)
		}
	}
}

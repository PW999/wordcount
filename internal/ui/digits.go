// Package ui is the Bubble Tea terminal interface: device picker, big WPM
// readout and live transcript.
package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DigitRows is the height of the big digits.
const DigitRows = 5

// Terminal size at or above which the big digits are drawn.
const (
	bigMinWidth  = 80
	bigMinHeight = 24
)

// Each glyph is 3 cells wide and 5 tall; '#' is a filled cell.
var glyphs = [10][DigitRows]string{
	{"###", "# #", "# #", "# #", "###"},
	{" # ", "## ", " # ", " # ", "###"},
	{"###", "  #", "###", "#  ", "###"},
	{"###", "  #", "###", "  #", "###"},
	{"# #", "# #", "###", "  #", "  #"},
	{"###", "#  ", "###", "  #", "###"},
	{"###", "#  ", "###", "# #", "###"},
	{"###", "  #", "  #", "  #", "  #"},
	{"###", "# #", "###", "# #", "###"},
	{"###", "# #", "###", "  #", "###"},
}

// BigDigits draws n (clamped at 0) as DigitRows lines. Every glyph cell is two
// characters wide so the number dominates the screen.
func BigDigits(n int) []string {
	if n < 0 {
		n = 0
	}
	digits := strconv.Itoa(n)
	rows := make([]string, DigitRows)
	for r := range rows {
		parts := make([]string, len(digits))
		for i, d := range digits {
			parts[i] = expand(glyphs[d-'0'][r])
		}
		rows[r] = strings.Join(parts, "  ")
	}
	return rows
}

func expand(row string) string {
	var sb strings.Builder
	for _, c := range row {
		if c == '#' {
			sb.WriteString("██")
		} else {
			sb.WriteString("  ")
		}
	}
	return sb.String()
}

// RenderWPM shows the WPM value as big digits when the terminal is at least
// 80x24, and as a single bold line otherwise.
func RenderWPM(n, width, height int) string {
	if width >= bigMinWidth && height >= bigMinHeight {
		return strings.Join(BigDigits(n), "\n")
	}
	return lipgloss.NewStyle().Bold(true).Render(strconv.Itoa(n) + " WPM")
}

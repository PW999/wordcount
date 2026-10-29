package stt

import (
	"regexp"
	"strings"
	"unicode"
)

// annotation matches model markers such as [BLANK_AUDIO], (music) and *noise*.
var annotation = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\*[^*]*\*`)

// CleanText removes annotations and normalises whitespace.
func CleanText(s string) string {
	return strings.Join(strings.Fields(annotation.ReplaceAllString(s, " ")), " ")
}

// CountWords counts whitespace-separated tokens containing a letter or digit,
// after annotations are removed.
func CountWords(s string) int {
	n := 0
	for _, tok := range strings.Fields(CleanText(s)) {
		if strings.IndexFunc(tok, func(r rune) bool {
			return unicode.IsLetter(r) || unicode.IsDigit(r)
		}) >= 0 {
			n++
		}
	}
	return n
}

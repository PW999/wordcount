package stt

import "strings"

// quietRMS is the utterance loudness below which known filler phrases are
// treated as whisper hallucinating on near-silence.
const quietRMS = 0.02

// hallucinations are phrases whisper tends to invent for silence or noise.
var hallucinations = map[string]bool{
	"you":                                  true,
	"thank you":                            true,
	"thanks":                               true,
	"thanks for watching":                  true,
	"thank you for watching":               true,
	"bye":                                  true,
	"bye bye":                              true,
	"subtitles by the amara.org community": true,
	"ondertitels door de amara.org gemeenschap": true,
	"ondertiteld door de amara.org gemeenschap": true,
}

// Filter cleans a transcriber result: annotations are removed, and text that
// is empty, punctuation-only, or a known filler phrase over quiet audio becomes "".
func Filter(text string, samples []float32) string {
	clean := CleanText(text)
	if CountWords(clean) == 0 {
		return ""
	}
	if len(samples) > 0 && rms(samples) < quietRMS && hallucinations[normalise(clean)] {
		return ""
	}
	return clean
}

func normalise(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimRight(s, ".!?,;: ")
}

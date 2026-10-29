package stt

import (
	"math"
	"testing"
)

// tone returns seconds of a 200 Hz sine at the given amplitude (RMS = amp/sqrt2).
func tone(seconds, amp float64) []float32 {
	n := int(seconds * sampleRate)
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*200*float64(i)/sampleRate))
	}
	return out
}

func speech(seconds float64) []float32  { return tone(seconds, 0.3) }
func silence(seconds float64) []float32 { return tone(seconds, 0.001) }

// feed pushes audio in 10 ms chunks like the capture callback does.
func feed(s *Segmenter, audio ...[]float32) []Request {
	var out []Request
	for _, a := range audio {
		for len(a) > 0 {
			n := sampleRate / 100
			if n > len(a) {
				n = len(a)
			}
			out = append(out, s.Push(a[:n])...)
			a = a[n:]
		}
	}
	return out
}

func finals(reqs []Request) []Request {
	var out []Request
	for _, r := range reqs {
		if r.Final {
			out = append(out, r)
		}
	}
	return out
}

// countingTranscriber stands in for whisper and records how often it is called.
type countingTranscriber struct{ calls int }

func (c *countingTranscriber) Transcribe([]float32) (string, error) {
	c.calls++
	return "word", nil
}

func TestUtteranceEndsAfterPause(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	// Speech then silence shorter than the endpoint: no final yet.
	reqs := feed(s, silence(1), speech(2), silence(0.5))
	if len(finals(reqs)) != 0 {
		t.Fatal("final emitted before the 0.7 s endpoint")
	}
	// More silence crosses the endpoint (about 0.7 s, plus the ~0.1 s the
	// energy smoothing needs to notice the speech has stopped).
	reqs = feed(s, silence(0.5))
	fin := finals(reqs)
	if len(fin) != 1 {
		t.Fatalf("got %d finals after the pause, want 1", len(fin))
	}
	// Speech (2 s) plus pre-roll and trailing keep, but not the whole silence.
	got := float64(len(fin[0].Samples)) / sampleRate
	if got < 2.0 || got > 2.7 {
		t.Fatalf("final is %.2f s of audio, want about 2.0-2.7 s", got)
	}
}

func TestPartialsRefreshWhileSpeaking(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	reqs := feed(s, silence(1), speech(3.5))
	if len(reqs) != 3 {
		t.Fatalf("got %d requests in 3.5 s of speech, want 3 partials", len(reqs))
	}
	for i, r := range reqs {
		if r.Final {
			t.Fatalf("request %d is final", i)
		}
		if i > 0 && len(r.Samples) <= len(reqs[i-1].Samples) {
			t.Fatalf("partial %d does not grow the utterance", i)
		}
	}
}

func TestLongMonologueSplitsWithoutDroppingAudio(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	reqs := feed(s, silence(1), speech(20), silence(1))
	fin := finals(reqs)
	if len(fin) != 2 {
		t.Fatalf("got %d finals for 20 s of speech, want 2", len(fin))
	}
	if got := len(fin[0].Samples); got < 15*sampleRate || got > 15*sampleRate+frameSize {
		t.Fatalf("first final has %d samples, want about 15 s", got)
	}
	total := 0
	for _, f := range fin {
		total += len(f.Samples)
	}
	if total < 20*sampleRate {
		t.Fatalf("finals hold %.2f s of audio, want at least 20 s", float64(total)/sampleRate)
	}
}

func TestSilenceOnlyNeverReachesTranscriber(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	tr := &countingTranscriber{}
	for _, r := range feed(s, silence(10), tone(5, 0.002)) {
		_, _ = tr.Transcribe(r.Samples)
	}
	if tr.calls != 0 {
		t.Fatalf("transcriber called %d times for silence, want 0", tr.calls)
	}
}

func TestShortNoiseBlipIsIgnored(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	reqs := feed(s, silence(1), speech(0.04), silence(2))
	if len(reqs) != 0 {
		t.Fatalf("got %d requests for a 40 ms blip, want 0", len(reqs))
	}
}

func TestStartOfSpeechIsNotClipped(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	fin := finals(feed(s, silence(1), speech(1), silence(1)))
	if len(fin) != 1 {
		t.Fatalf("got %d finals, want 1", len(fin))
	}
	// 1 s of speech plus ~0.3 s pre-roll must be present.
	if got := float64(len(fin[0].Samples)) / sampleRate; got < 1.25 {
		t.Fatalf("final holds %.2f s, pre-roll missing", got)
	}
}

// noisyRoom is steady background noise at about 0.05 RMS, like the idle level
// measured on a real laptop microphone.
func noisyRoom(seconds float64) []float32 { return tone(seconds, 0.07) }

// loudSpeech is speech over that noise floor (about 0.28 RMS).
func loudSpeech(seconds float64) []float32 { return tone(seconds, 0.4) }

func TestSteadyBackgroundNoiseIsNotSpeech(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	if reqs := feed(s, noisyRoom(30)); len(reqs) != 0 {
		t.Fatalf("got %d requests for 30 s of steady noise, want 0", len(reqs))
	}
}

func TestSpeechIsSegmentedInANoisyRoom(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	reqs := feed(s, noisyRoom(3), loudSpeech(1.5), noisyRoom(2), loudSpeech(1.5), noisyRoom(2))
	fin := finals(reqs)
	if len(fin) != 2 {
		t.Fatalf("got %d finals, want 2 (the pause must end the utterance despite the noise)", len(fin))
	}
	for i, f := range fin {
		if got := float64(len(f.Samples)) / sampleRate; got < 1.5 || got > 2.3 {
			t.Fatalf("final %d is %.2f s, want about 1.5-2.3 s", i, got)
		}
	}
}

func TestNoUtteranceStartsDuringWarmup(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	// The very first audio is noise: it must be learned as the floor, not treated as speech.
	if reqs := feed(s, noisyRoom(0.25)); len(reqs) != 0 {
		t.Fatalf("got %d requests during warm-up, want 0", len(reqs))
	}
}

func TestTwoUtterancesAreSeparate(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	fin := finals(feed(s, silence(1), speech(1), silence(1), speech(1), silence(1)))
	if len(fin) != 2 {
		t.Fatalf("got %d finals, want 2", len(fin))
	}
}

func TestFlushFinalisesUtteranceInProgress(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	// The stream ends mid-sentence: no pause ever reached the endpoint.
	if reqs := feed(s, silence(1), speech(2)); len(finals(reqs)) != 0 {
		t.Fatal("final emitted before the flush")
	}
	fin := s.Flush()
	if len(fin) != 1 || !fin[0].Final {
		t.Fatalf("Flush returned %d requests, want one Final", len(fin))
	}
	if got := float64(len(fin[0].Samples)) / sampleRate; got < 1.9 || got > 2.7 {
		t.Errorf("flushed utterance is %.2f s, want about 2 s", got)
	}
	if again := s.Flush(); len(again) != 0 {
		t.Errorf("second Flush returned %d requests, want none", len(again))
	}
}

func TestFlushDropsTrailingSilence(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	feed(s, silence(1), speech(2), silence(0.4)) // paused, but shorter than the endpoint
	fin := s.Flush()
	if len(fin) != 1 {
		t.Fatalf("Flush returned %d requests, want 1", len(fin))
	}
	if got := float64(len(fin[0].Samples)) / sampleRate; got > 2.7 {
		t.Errorf("flushed utterance is %.2f s, want the 0.4 s of silence mostly trimmed", got)
	}
}

func TestFlushOfSilenceEmitsNothing(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	feed(s, silence(3))
	if reqs := s.Flush(); len(reqs) != 0 {
		t.Fatalf("Flush of silence returned %d requests, want none", len(reqs))
	}
}

func TestFlushIgnoresShortBlip(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	feed(s, silence(1), speech(0.04))
	if reqs := s.Flush(); len(reqs) != 0 {
		t.Fatalf("Flush of a blip returned %d requests, want none", len(reqs))
	}
}

func TestSegmenterWorksAfterFlush(t *testing.T) {
	s := NewSegmenter(DefaultSegmenterConfig())
	feed(s, silence(1), speech(1))
	s.Flush()
	reqs := feed(s, silence(1), speech(2), silence(1.5))
	if len(finals(reqs)) != 1 {
		t.Fatalf("got %d finals after reuse, want 1", len(finals(reqs)))
	}
}

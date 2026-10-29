package audio

import (
	"math"
	"testing"
)

// sine returns frames of a sine at freq Hz, identical in every channel, interleaved.
func sine(freq float64, rate, channels, frames int, amp float64) []float32 {
	out := make([]float32, frames*channels)
	for i := 0; i < frames; i++ {
		v := float32(amp * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
		for c := 0; c < channels; c++ {
			out[i*channels+c] = v
		}
	}
	return out
}

func rmsOf(s []float32) float64 {
	var sum float64
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(s)))
}

func TestMono16k(t *testing.T) {
	const amp = 0.5
	wantRMS := amp / math.Sqrt2
	tests := []struct {
		name     string
		rate, ch int
	}{
		{"44.1 kHz stereo", 44100, 2},
		{"48 kHz mono", 48000, 1},
		{"16 kHz passthrough", 16000, 1},
		{"16 kHz stereo downmix only", 16000, 2},
		{"8 kHz upsample", 8000, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frames := tc.rate * 2 // two seconds
			d := &Decoded{Samples: sine(440, tc.rate, tc.ch, frames, amp), SampleRate: tc.rate, Channels: tc.ch}
			got := Mono16k(d)
			if want := 2 * SampleRate; math.Abs(float64(len(got)-want)) > 1 {
				t.Errorf("length = %d, want %d", len(got), want)
			}
			// Ignore the edges, where the filter window is truncated.
			mid := got[SampleRate/4 : len(got)-SampleRate/4]
			if r := rmsOf(mid); math.Abs(r-wantRMS) > 0.02 {
				t.Errorf("RMS = %.4f, want %.4f +-0.02", r, wantRMS)
			}
		})
	}
}

func TestMono16kPassthroughIsExact(t *testing.T) {
	in := []float32{0.1, -0.2, 0.3}
	got := Mono16k(&Decoded{Samples: in, SampleRate: SampleRate, Channels: 1})
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("got %v, want %v", got, in)
		}
	}
	got[0] = 9
	if in[0] == 9 {
		t.Error("output must not alias the input")
	}
}

func TestMono16kDownmixAverages(t *testing.T) {
	got := Mono16k(&Decoded{Samples: []float32{1, -1, 0.5, 0.25}, SampleRate: SampleRate, Channels: 2})
	if len(got) != 2 || got[0] != 0 || got[1] != 0.375 {
		t.Errorf("got %v, want [0 0.375]", got)
	}
}

func TestMono16kRejectsAliasing(t *testing.T) {
	// 20 kHz is above the 16 kHz output's Nyquist (8 kHz); it must come out much quieter than the input.
	d := &Decoded{Samples: sine(20000, 48000, 1, 48000, 0.5), SampleRate: 48000, Channels: 1}
	got := Mono16k(d)
	if r := rmsOf(got[SampleRate/4 : len(got)-SampleRate/4]); r > 0.1 {
		t.Errorf("RMS of out-of-band tone = %.3f, want it attenuated below 0.1 (input %.3f)", r, 0.5/math.Sqrt2)
	}
}

func TestMono16kEmpty(t *testing.T) {
	if got := Mono16k(&Decoded{SampleRate: 44100, Channels: 2}); len(got) != 0 {
		t.Errorf("got %d samples for empty input", len(got))
	}
}

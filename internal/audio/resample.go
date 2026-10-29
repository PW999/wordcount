package audio

import "math"

// Mono16k downmixes d to one channel by averaging and resamples it to
// SampleRate (16 kHz). Audio already at 16 kHz is only downmixed.
func Mono16k(d *Decoded) []float32 {
	mono := downmix(d.Samples, d.Channels)
	if d.SampleRate == SampleRate {
		return mono
	}
	return resample(mono, d.SampleRate, SampleRate)
}

func downmix(samples []float32, channels int) []float32 {
	if channels <= 1 {
		return append([]float32(nil), samples...)
	}
	out := make([]float32, len(samples)/channels)
	inv := 1 / float32(channels)
	for i := range out {
		var sum float32
		for _, s := range samples[i*channels : (i+1)*channels] {
			sum += s
		}
		out[i] = sum * inv
	}
	return out
}

// resample converts mono audio between rates with linear interpolation. When
// reducing the rate it first averages over a window as wide as the rate ratio,
// a cheap low-pass that keeps content above the new Nyquist from aliasing into
// the speech band. That is plenty for speech recognition.
func resample(in []float32, from, to int) []float32 {
	if len(in) == 0 {
		return nil
	}
	ratio := float64(from) / float64(to)
	n := int(math.Round(float64(len(in)) / ratio))
	out := make([]float32, n)

	src := in
	if ratio > 1 {
		src = boxFilter(in, int(math.Round(ratio)))
	}
	last := len(src) - 1
	for i := range out {
		pos := float64(i) * ratio
		j := int(pos)
		if j >= last {
			out[i] = src[last]
			continue
		}
		frac := float32(pos - float64(j))
		out[i] = src[j]*(1-frac) + src[j+1]*frac
	}
	return out
}

// boxFilter returns the moving average over width samples centred on each
// sample, shrinking the window at the edges. It uses prefix sums, so its cost
// does not depend on width.
func boxFilter(in []float32, width int) []float32 {
	if width <= 1 {
		return in
	}
	prefix := make([]float64, len(in)+1)
	for i, v := range in {
		prefix[i+1] = prefix[i] + float64(v)
	}
	out := make([]float32, len(in))
	half := width / 2
	for i := range out {
		lo := max(0, i-half)
		hi := min(len(in), i-half+width)
		out[i] = float32((prefix[hi] - prefix[lo]) / float64(hi-lo))
	}
	return out
}

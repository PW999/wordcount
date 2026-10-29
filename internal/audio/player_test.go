package audio

import (
	"errors"
	"testing"
	"unsafe"
)

// drive calls the audio callback by hand for the given number of frames and
// returns the samples the "device" received.
func drive(p *FilePlayer, frames int) []float32 {
	out := make([]float32, frames*p.file.Channels)
	var b []byte
	if len(out) > 0 {
		b = unsafe.Slice((*byte)(unsafe.Pointer(&out[0])), len(out)*4)
	}
	p.onData(b, nil, uint32(frames))
	return out
}

// received drains whatever chunks are queued right now.
func received(p *FilePlayer) (total int, closed bool) {
	for {
		select {
		case c, ok := <-p.chunks:
			if !ok {
				return total, true
			}
			total += len(c)
		default:
			return total, false
		}
	}
}

func TestFilePlayerStreamsInStepWithPlayback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rate, ch int
		callback int // frames per device callback
	}{
		{"16 kHz mono", 16000, 1, 480},
		{"44.1 kHz stereo", 44100, 2, 441},
		{"48 kHz mono odd callback size", 48000, 1, 1001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := tc.rate * 3
			d := &Decoded{Samples: sine(300, tc.rate, tc.ch, frames, 0.4), SampleRate: tc.rate, Channels: tc.ch}
			p := newFilePlayer(d)

			var delivered int
			closed := false
			for played := 0; played < frames+3*tc.callback; played += tc.callback {
				out := drive(p, tc.callback)
				if played < frames-tc.callback { // fully inside the file: output is the file
					for i := range out {
						if out[i] != d.Samples[played*tc.ch+i] {
							t.Fatalf("output sample %d differs from file", played*tc.ch+i)
						}
					}
				}
				n, c := received(p)
				delivered += n
				closed = closed || c

				// Analysis must never run ahead of playback.
				limit := min(len(p.mono), (min(played+tc.callback, frames))*SampleRate/tc.rate)
				if delivered > limit {
					t.Fatalf("delivered %d samples after %d played frames, limit %d", delivered, played+tc.callback, limit)
				}
			}
			if delivered != len(p.mono) {
				t.Errorf("delivered %d samples in total, want %d", delivered, len(p.mono))
			}
			if want := 3 * SampleRate; delivered < want-1 || delivered > want+1 {
				t.Errorf("delivered %d samples, want about %d (3 s at 16 kHz)", delivered, want)
			}
			if !closed {
				t.Error("Chunks was not closed at the end of the file")
			}
			if p.Level() == 0 {
				t.Error("level was never reported")
			}
		})
	}
}

func TestFilePlayerPadsTailWithSilenceAndStaysQuiet(t *testing.T) {
	d := &Decoded{Samples: []float32{0.5, 0.5, 0.5}, SampleRate: SampleRate, Channels: 1}
	p := newFilePlayer(d)
	out := drive(p, 5)
	want := []float32{0.5, 0.5, 0.5, 0, 0}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("output = %v, want %v", out, want)
		}
	}
	if total, closed := received(p); total != 3 || !closed {
		t.Errorf("total = %d closed = %v, want 3 true", total, closed)
	}
	// The device keeps calling after the end: silence, and no second close (which would panic).
	for _, v := range drive(p, 4) {
		if v != 0 {
			t.Fatal("expected silence after the end of the file")
		}
	}
}

func TestFilePlayerCloseWithoutDevice(t *testing.T) {
	newFilePlayer(&Decoded{Samples: []float32{0}, SampleRate: SampleRate, Channels: 1}).Close()
}

type fakeOutputs struct {
	devs []Device
	err  error
}

func (f fakeOutputs) PlaybackDevices() ([]Device, error) { return f.devs, f.err }

func TestRequireOutput(t *testing.T) {
	if err := RequireOutput(fakeOutputs{}); !errors.Is(err, ErrNoOutputDevice) {
		t.Errorf("no devices: error = %v, want ErrNoOutputDevice", err)
	}
	boom := errors.New("boom")
	if err := RequireOutput(fakeOutputs{err: boom}); !errors.Is(err, boom) {
		t.Errorf("backend failure: error = %v, want boom", err)
	}
	if err := RequireOutput(fakeOutputs{devs: []Device{{Name: "speakers"}}}); err != nil {
		t.Errorf("one device: error = %v, want nil", err)
	}
}

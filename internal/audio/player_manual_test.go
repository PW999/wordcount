package audio

import (
	"os"
	"testing"
	"time"
)

// TestManualPlayback plays testdata/jfk.wav through the default output device
// and reports when the stream ends. Listen for the speech while it runs:
// WORDCOUNT_PLAY=1 go test -run ManualPlayback -v ./internal/audio
func TestManualPlayback(t *testing.T) {
	if os.Getenv("WORDCOUNT_PLAY") == "" {
		t.Skip("set WORDCOUNT_PLAY=1 to play audio through the default output device")
	}
	d, err := DecodeFile("../../testdata/jfk.wav")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMalgoBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	p, err := NewFilePlayer(b, d)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	start := time.Now()
	var samples int
	timeout := time.After(d.Duration() + 5*time.Second)
	for {
		select {
		case chunk, ok := <-p.Chunks():
			if !ok {
				took := time.Since(start)
				t.Logf("finished: %d samples (%.1fs at 16 kHz) in %.1fs of playback, %d chunks dropped",
					samples, float64(samples)/SampleRate, took.Seconds(), p.Dropped())
				if diff := took - d.Duration(); diff < -500*time.Millisecond || diff > time.Second {
					t.Errorf("playback took %v, file lasts %v", took, d.Duration())
				}
				return
			}
			samples += len(chunk)
		case err := <-p.Err():
			t.Fatal(err)
		case <-timeout:
			t.Fatalf("playback did not finish; %d samples received", samples)
		}
	}
}

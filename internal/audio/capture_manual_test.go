package audio

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestManualCapture records for a few seconds and prints the input level.
// Speak while it runs: WORDCOUNT_MIC=<device index> go test -run Manual -v ./internal/audio
func TestManualCapture(t *testing.T) {
	spec := os.Getenv("WORDCOUNT_MIC")
	if spec == "" {
		t.Skip("set WORDCOUNT_MIC=<device index> to run against a real microphone")
	}
	b, err := NewMalgoBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	devs, err := ListDevices(b)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := Resolve(devs, spec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Start(dev)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var samples int
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case chunk := <-c.Chunks():
			samples += len(chunk)
		case <-tick.C:
			t.Logf("level %.4f %s", c.Level(), strings.Repeat("#", int(c.Level()*200)))
		case err := <-c.Err():
			t.Fatal(err)
		case <-deadline:
			t.Logf("%d samples (%.1fs at 16 kHz), %d chunks dropped", samples, float64(samples)/SampleRate, c.Dropped())
			if samples < SampleRate*4 {
				t.Fatalf("expected about %d samples, got %s", SampleRate*5, strconv.Itoa(samples))
			}
			return
		}
	}
}

func TestStartFailureIsReturnedAsError(t *testing.T) {
	if os.Getenv("WORDCOUNT_MIC") == "" {
		t.Skip("needs a real audio backend; set WORDCOUNT_MIC=<device index>")
	}
	b, err := NewMalgoBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var bogus Device
	// A well-formed (NUL-terminated) PulseAudio-style id for a device that does not exist.
	copy(bogus.ID[:], "alsa_input.no_such_device\x00")
	bogus.Name = "bogus"
	c, err := b.Start(bogus)
	if err == nil {
		c.Close()
		t.Fatal("expected an error opening a nonexistent device")
	}
	t.Logf("got expected error: %v", err)
}

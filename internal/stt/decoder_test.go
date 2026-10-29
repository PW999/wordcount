package stt

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// gatedTranscriber blocks each call until released, and reports the call's
// sample length as its text so tests can tell requests apart.
type gatedTranscriber struct {
	started chan int
	gate    chan struct{}

	mu    sync.Mutex
	calls []int
	err   error
}

func newGated() *gatedTranscriber {
	return &gatedTranscriber{started: make(chan int, 100), gate: make(chan struct{})}
}

func (g *gatedTranscriber) Transcribe(samples []float32) (string, error) {
	g.mu.Lock()
	g.calls = append(g.calls, len(samples))
	g.mu.Unlock()
	g.started <- len(samples)
	<-g.gate
	if g.err != nil {
		return "", g.err
	}
	return fmt.Sprintf("utterance%d", len(samples)), nil
}

func req(n int, final bool) Request { return Request{Samples: make([]float32, n), Final: final} }

func recv(t *testing.T, d *Decoder) Update {
	t.Helper()
	select {
	case u, ok := <-d.Updates():
		if !ok {
			t.Fatal("updates channel closed early")
		}
		return u
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an update")
	}
	panic("unreachable")
}

func TestFinalsNeverDroppedAndStalePartialsAre(t *testing.T) {
	g := newGated()
	d := NewDecoder(g)
	defer d.Close()

	d.Submit(req(1, false)) // A: being transcribed
	<-g.started
	// While A is stuck: partials pile up, then two finals arrive, then the next utterance's partial.
	d.Submit(req(2, false)) // B  dropped (replaced)
	d.Submit(req(3, false)) // C  dropped (replaced)
	d.Submit(req(4, false)) // D  dropped (final of same utterance supersedes)
	d.Submit(req(5, true))  // F1
	d.Submit(req(6, true))  // F2
	d.Submit(req(7, false)) // E  next utterance's partial
	close(g.gate)

	want := []struct {
		text  string
		final bool
	}{{"utterance1", false}, {"utterance5", true}, {"utterance6", true}, {"utterance7", false}}
	for i, w := range want {
		u := recv(t, d)
		if u.Text != w.text || u.Final != w.final {
			t.Fatalf("update %d = %+v, want text %q final %v", i, u, w.text, w.final)
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.calls) != 4 {
		t.Fatalf("transcriber calls = %v, want exactly 4 (stale partials skipped)", g.calls)
	}
}

func TestSubmitNeverBlocksWhileDecoderIsSlow(t *testing.T) {
	g := newGated()
	d := NewDecoder(g)
	defer func() { close(g.gate); d.Close() }()

	d.Submit(req(1, false))
	<-g.started // decoder is now stuck inside the transcriber

	done := make(chan struct{})
	go func() {
		for i := 0; i < 5000; i++ {
			d.Submit(req(100, false))
		}
		d.Submit(req(100, true))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Submit blocked while the transcriber was busy")
	}
}

func TestTranscriberErrorIsReported(t *testing.T) {
	g := newGated()
	g.err = errors.New("model exploded")
	close(g.gate)
	d := NewDecoder(g)
	defer d.Close()

	d.Submit(req(1, true))
	if u := recv(t, d); u.Err == nil || u.Err.Error() != "model exploded" {
		t.Fatalf("update = %+v, want the transcriber error", u)
	}
}

func TestCloseStopsDecoderAndClosesUpdates(t *testing.T) {
	g := newGated()
	close(g.gate)
	d := NewDecoder(g)
	d.Close()
	if _, ok := <-d.Updates(); ok {
		t.Fatal("updates channel still open after Close")
	}
	d.Submit(req(1, true)) // must not panic or block after Close
}

type fixedTranscriber string

func (f fixedTranscriber) Transcribe([]float32) (string, error) { return string(f), nil }

func TestDecoderFiltersAnnotations(t *testing.T) {
	d := NewDecoder(fixedTranscriber("hello [BLANK_AUDIO] (music) world"))
	defer d.Close()
	d.Submit(Request{Samples: speech(0.5), Final: true})
	if u := recv(t, d); u.Text != "hello world" {
		t.Fatalf("Text = %q, want %q", u.Text, "hello world")
	}
}

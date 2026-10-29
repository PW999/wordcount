package stt

import "sync"

// Update is a transcription result ready for display and counting.
type Update struct {
	Text  string // filtered text; may be empty
	Final bool
	Err   error // set when the transcriber failed; no further useful updates follow
}

// Decoder runs transcription on its own goroutine. Submit never blocks, so it
// is safe to call from the audio pipeline. Under load, only the newest pending
// partial is kept (older ones are dropped); finals are always processed, in order.
type Decoder struct {
	t Transcriber

	mu      sync.Mutex
	cond    *sync.Cond
	finals  []Request
	partial *Request
	closed  bool

	updates chan Update
	stop    chan struct{}
	done    chan struct{}
}

// NewDecoder starts a decoder goroutine that uses t.
func NewDecoder(t Transcriber) *Decoder {
	d := &Decoder{
		t:       t,
		updates: make(chan Update, 64),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	d.cond = sync.NewCond(&d.mu)
	go d.run()
	return d
}

// Updates delivers results. It is closed after Close.
func (d *Decoder) Updates() <-chan Update { return d.updates }

// Submit queues a request without blocking. A new partial replaces any older
// pending partial; a final also discards the pending partial of the same utterance.
func (d *Decoder) Submit(r Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	if r.Final {
		d.finals = append(d.finals, r)
		d.partial = nil
	} else {
		d.partial = &r
	}
	d.cond.Signal()
}

// Close stops the decoder, discarding queued work, and waits for the goroutine to exit.
func (d *Decoder) Close() {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		close(d.stop)
		d.cond.Broadcast()
	}
	d.mu.Unlock()
	<-d.done
}

func (d *Decoder) run() {
	defer close(d.done)
	defer close(d.updates)
	for {
		d.mu.Lock()
		for !d.closed && len(d.finals) == 0 && d.partial == nil {
			d.cond.Wait()
		}
		if d.closed {
			d.mu.Unlock()
			return
		}
		var req Request
		if len(d.finals) > 0 {
			req = d.finals[0]
			d.finals = d.finals[1:]
		} else {
			req = *d.partial
			d.partial = nil
		}
		d.mu.Unlock()

		text, err := d.t.Transcribe(req.Samples)
		u := Update{Final: req.Final}
		if err != nil {
			u.Err = err
		} else {
			u.Text = Filter(text, req.Samples)
		}
		select {
		case d.updates <- u:
		case <-d.stop:
			return
		}
	}
}

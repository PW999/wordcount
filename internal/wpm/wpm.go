// Package wpm computes a rolling words-per-minute value from detected words.
package wpm

import "time"

const (
	// Window is the trailing period words are counted over.
	Window = 60 * time.Second
	// MinElapsed is the floor for the divisor so the first seconds do not spike.
	MinElapsed = 5 * time.Second
)

type event struct {
	at    time.Time
	words int
}

// Calculator tracks word timestamps. It is not safe for concurrent use.
type Calculator struct {
	now       func() time.Time
	start     time.Time
	committed []event // words of finished utterances
	current   []event // words of the in-progress utterance
	total     int
}

// New returns a Calculator whose listening time starts now. A nil clock uses time.Now.
func New(clock func() time.Time) *Calculator {
	if clock == nil {
		clock = time.Now
	}
	return &Calculator{now: clock, start: clock()}
}

// Update sets the word count of the in-progress utterance. Growth is stamped
// with the current time; a shrinking (revised) partial removes its most recent
// words instead of adding, so revisions never inflate the count.
func (c *Calculator) Update(words int) {
	have := sum(c.current)
	switch {
	case words > have:
		c.current = append(c.current, event{c.now(), words - have})
		c.total += words - have
	case words < have:
		drop := have - words
		c.total -= drop
		for drop > 0 {
			last := &c.current[len(c.current)-1]
			if last.words > drop {
				last.words -= drop
				break
			}
			drop -= last.words
			c.current = c.current[:len(c.current)-1]
		}
	}
}

// Finalize commits the in-progress utterance (set its final count with Update first).
func (c *Calculator) Finalize() {
	c.committed = append(c.committed, c.current...)
	c.current = nil
}

// Total is the number of words detected in the session.
func (c *Calculator) Total() int { return c.total }

// Elapsed is the time since listening began.
func (c *Calculator) Elapsed() time.Duration { return c.now().Sub(c.start) }

// WPM returns the words per minute over the trailing window. Safe to call
// every tick: it decays as words leave the window.
func (c *Calculator) WPM() float64 {
	now := c.now()
	cutoff := now.Add(-Window)
	i := 0
	for i < len(c.committed) && !c.committed[i].at.After(cutoff) {
		i++
	}
	c.committed = c.committed[i:]

	words := 0
	for _, list := range [][]event{c.committed, c.current} {
		for _, e := range list {
			if e.at.After(cutoff) {
				words += e.words
			}
		}
	}

	span := now.Sub(c.start)
	if span > Window {
		span = Window
	}
	if span < MinElapsed {
		span = MinElapsed
	}
	return float64(words) / span.Seconds() * 60
}

func sum(events []event) int {
	n := 0
	for _, e := range events {
		n += e.words
	}
	return n
}

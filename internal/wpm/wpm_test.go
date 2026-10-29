package wpm

import (
	"math"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newCalc() (*Calculator, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	return New(clk.now), clk
}

func TestSteady150WPM(t *testing.T) {
	c, clk := newCalc()
	// 5 words every 2 s = 150 wpm, for 90 s.
	for i := 0; i < 45; i++ {
		clk.advance(2 * time.Second)
		c.Update(5)
		c.Finalize()
	}
	got := c.WPM()
	if math.Abs(got-150) > 150*0.15 {
		t.Fatalf("WPM = %.1f, want about 150", got)
	}
	if c.Total() != 225 {
		t.Fatalf("Total = %d, want 225", c.Total())
	}
}

func TestDecaysToZeroAfterSilence(t *testing.T) {
	c, clk := newCalc()
	for i := 0; i < 30; i++ {
		clk.advance(2 * time.Second)
		c.Update(5)
		c.Finalize()
	}
	prev := c.WPM()
	for i := 0; i < 6; i++ {
		clk.advance(10 * time.Second)
		cur := c.WPM()
		if cur > prev {
			t.Fatalf("WPM rose during silence: %.1f -> %.1f", prev, cur)
		}
		prev = cur
	}
	if prev != 0 {
		t.Fatalf("WPM after 60 s silence = %.1f, want 0", prev)
	}
	if c.Total() != 150 {
		t.Fatalf("Total must not decay, got %d", c.Total())
	}
}

func TestStartupFloor(t *testing.T) {
	c, clk := newCalc()
	clk.advance(1 * time.Second)
	c.Update(10)
	// 10 words computed as if 5 s elapsed: 120 wpm, not 600.
	if got := c.WPM(); got != 120 {
		t.Fatalf("WPM = %.1f, want 120", got)
	}
}

func TestPartialRevisionReplacesNotAdds(t *testing.T) {
	c, clk := newCalc()
	clk.advance(5 * time.Second)
	c.Update(5)
	c.Update(4) // model revised its guess downwards
	c.Update(6)
	c.Update(5)
	c.Finalize()
	if c.Total() != 5 {
		t.Fatalf("Total = %d, want 5", c.Total())
	}
	if got := c.WPM(); got != 5.0/5*60 {
		t.Fatalf("WPM = %.1f, want 60", got)
	}
}

func TestRevisionDropsOnlyCurrentUtterance(t *testing.T) {
	c, clk := newCalc()
	clk.advance(5 * time.Second)
	c.Update(3)
	c.Finalize()
	c.Update(2)
	c.Update(0)
	if c.Total() != 3 {
		t.Fatalf("Total = %d, want 3", c.Total())
	}
}

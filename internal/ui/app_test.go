package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wordcount/internal/audio"
)

type testClock struct{ t time.Time }

func (c *testClock) now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// countWords is a simple stand-in for stt.CountWords.
func countWords(s string) int { return len(strings.Fields(s)) }

type harness struct {
	app   *App
	clock *testClock
	level float32
}

// listening returns an App that has picked a device and is running at w x h.
func listening(t *testing.T, w, h int) *harness {
	t.Helper()
	hr := &harness{clock: &testClock{t: time.Unix(1_000_000, 0)}}
	hr.app = New(Config{
		Devices: devices("mic"),
		Start: func(audio.Device) (Source, error) {
			return Source{Level: func() float32 { return hr.level }}, nil
		},
		Count: countWords,
		Clock: hr.clock.now,
	})
	hr.app.Update(tea.WindowSizeMsg{Width: w, Height: h})
	hr.app.Update(DeviceChosenMsg{Device: audio.Device{Name: "mic"}})
	if !hr.app.listening {
		t.Fatal("app did not start listening")
	}
	return hr
}

func (h *harness) text(s string, final bool) {
	h.app.Update(updateMsg(TextUpdate{Text: s, Final: final}))
}
func (h *harness) tick() { h.app.Update(tickMsg(h.clock.now())) }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestPartialThenFinalReplacement(t *testing.T) {
	h := listening(t, 80, 24)

	h.text("hello wor", false)
	h.text("hello world how", false)
	view := h.app.View()
	if strings.Count(view, "hello") != 1 || !strings.Contains(view, "hello world how") {
		t.Fatalf("partial should be updated in place, view:\n%s", view)
	}
	if len(h.app.lines) != 0 {
		t.Fatalf("partial leaked into finals: %v", h.app.lines)
	}

	h.text("hello world how are you", true)
	if len(h.app.lines) != 1 || h.app.lines[0] != "hello world how are you" {
		t.Fatalf("lines = %v, want the final text", h.app.lines)
	}
	if h.app.partial != "" {
		t.Fatalf("partial = %q after final, want empty", h.app.partial)
	}
	if got := h.app.calc.Total(); got != 5 {
		t.Fatalf("Total = %d, want 5 (revisions must not double count)", got)
	}

	h.text("next sentence", false)
	view = h.app.View()
	if strings.Index(view, "hello world how are you") > strings.Index(view, "next sentence") {
		t.Fatalf("new partial must appear below the final line, view:\n%s", view)
	}
	if strings.Count(view, "hello world how are you") != 1 {
		t.Fatalf("final text duplicated, view:\n%s", view)
	}
}

func TestEmptyFinalClearsPartialAndUncountsWords(t *testing.T) {
	h := listening(t, 80, 24)
	h.text("uh huh", false)
	h.text("", true) // model decided it was noise after all
	if h.app.partial != "" || len(h.app.lines) != 0 || h.app.calc.Total() != 0 {
		t.Fatalf("partial=%q lines=%v total=%d, want all empty", h.app.partial, h.app.lines, h.app.calc.Total())
	}
}

func TestFeedOverflowKeepsNewestLines(t *testing.T) {
	h := listening(t, 80, 12)
	for i := 0; i < 40; i++ {
		h.text(fmt.Sprintf("sentence number %d", i), true)
	}
	view := h.app.View()
	if !strings.Contains(view, "sentence number 39") {
		t.Fatalf("newest line missing, view:\n%s", view)
	}
	if strings.Contains(view, "sentence number 0\n") || strings.Contains(view, "sentence number 10") {
		t.Fatalf("oldest lines should have scrolled out, view:\n%s", view)
	}
	if rows := strings.Count(view, "\n") + 1; rows > 12 {
		t.Fatalf("view is %d rows, terminal has 12", rows)
	}
}

func TestPartialStaysVisibleWhenFeedOverflows(t *testing.T) {
	h := listening(t, 80, 12)
	for i := 0; i < 40; i++ {
		h.text(fmt.Sprintf("old %d", i), true)
	}
	h.text("still talking", false)
	if view := h.app.View(); !strings.Contains(view, "still talking") {
		t.Fatalf("in-progress text must stay visible, view:\n%s", view)
	}
}

func TestLongLinesWrapToTerminalWidth(t *testing.T) {
	h := listening(t, 40, 24)
	h.text(strings.Repeat("word ", 30), true)
	for _, line := range strings.Split(h.app.View(), "\n") {
		if n := len([]rune(line)); n > 40 {
			t.Fatalf("line of %d chars exceeds width 40: %q", n, line)
		}
	}
}

func TestTickRefreshesWPMAndLevel(t *testing.T) {
	h := listening(t, 80, 24)
	h.clock.advance(10 * time.Second)
	h.text(strings.Repeat("w ", 25), true) // 25 words over 10 s
	h.level = 0.1
	h.tick()
	if got := h.app.wpm; got < 149 || got > 151 {
		t.Fatalf("wpm = %.1f, want about 150", got)
	}
	if !strings.Contains(h.app.View(), strings.Repeat("█", 12)) {
		t.Fatalf("level bar did not respond to input level:\n%s", h.app.View())
	}

	h.clock.advance(61 * time.Second)
	h.tick()
	if h.app.wpm != 0 {
		t.Fatalf("wpm = %.1f after a minute of silence, want 0", h.app.wpm)
	}
	if h.app.calc.Total() != 25 {
		t.Fatalf("total words must not decay, got %d", h.app.calc.Total())
	}
}

func TestHeaderSizeDependsOnTerminal(t *testing.T) {
	big := listening(t, 80, 24).app.View()
	if !strings.Contains(strings.Split(big, "\n")[0], "██") {
		t.Fatalf("80x24 should draw big digits:\n%s", big)
	}
	small := listening(t, 60, 20).app.View()
	if strings.Contains(small, "██") || !strings.Contains(small, "0 WPM") {
		t.Fatalf("small terminal should use the one-line WPM:\n%s", small)
	}
}

func TestQuitKeysEndSessionWithSummary(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{"q": keyQ, "esc": keyEsc, "ctrl+c": keyCtrlC} {
		t.Run(name, func(t *testing.T) {
			h := listening(t, 80, 24)
			h.clock.advance(10 * time.Second)
			h.text(strings.Repeat("w ", 25), true)

			_, cmd := h.app.Update(key)
			if !isQuit(cmd) {
				t.Fatal("key did not produce tea.Quit")
			}
			s, ok := h.app.Summary()
			if !ok {
				t.Fatal("no summary after quitting while listening")
			}
			if s.Words != 25 || s.Duration != 10*time.Second || s.AvgWPM < 149 || s.AvgWPM > 151 {
				t.Fatalf("summary = %+v, want 25 words, 10 s, about 150 WPM", s)
			}
			if got := s.String(); got != "25 words in 0:10, average 150 WPM" {
				t.Fatalf("summary line = %q", got)
			}
		})
	}
}

func TestQuitFromPickerHasNoSummary(t *testing.T) {
	a := New(Config{Devices: devices("a", "b"), Count: countWords})
	_, cmd := a.Update(keyQ)
	if !isQuit(cmd) {
		t.Fatal("q in the picker should quit")
	}
	if _, ok := a.Summary(); ok {
		t.Fatal("no summary expected when listening never started")
	}
}

func TestPickerThenEnterStartsListening(t *testing.T) {
	var started audio.Device
	a := New(Config{
		Devices: devices("a", "b"),
		Start: func(d audio.Device) (Source, error) {
			started = d
			return Source{}, nil
		},
		Count: countWords,
	})
	if !strings.Contains(a.View(), "Select a microphone") {
		t.Fatalf("picker not shown first:\n%s", a.View())
	}
	a.Update(keyDown)
	_, cmd := a.Update(keyEnter)
	a.Update(cmd())
	if started.Name != "b" || !a.listening {
		t.Fatalf("started %q listening=%v, want b and true", started.Name, a.listening)
	}
}

func TestPreselectedDeviceSkipsPicker(t *testing.T) {
	dev := audio.Device{Index: 3, Name: "usb"}
	a := New(Config{Preselected: &dev, Start: func(audio.Device) (Source, error) { return Source{}, nil }, Count: countWords})
	msg, ok := a.Init()().(DeviceChosenMsg)
	if !ok || msg.Device.Name != "usb" {
		t.Fatalf("Init produced %#v, want DeviceChosenMsg for usb", msg)
	}
}

func TestDeviceOpenFailureQuitsWithError(t *testing.T) {
	boom := errors.New("cannot open device")
	a := New(Config{Devices: devices("a"), Start: func(audio.Device) (Source, error) { return Source{}, boom }, Count: countWords})
	_, cmd := a.Update(DeviceChosenMsg{Device: audio.Device{Name: "a"}})
	if !isQuit(cmd) || !errors.Is(a.Err(), boom) {
		t.Fatalf("quit=%v err=%v, want quit with the open error", isQuit(cmd), a.Err())
	}
}

func TestDeviceFailureDuringCaptureQuitsWithError(t *testing.T) {
	h := listening(t, 80, 24)
	_, cmd := h.app.Update(errMsg{errors.New("device unplugged")})
	if !isQuit(cmd) || h.app.Err() == nil {
		t.Fatal("capture error should end the UI and be reported")
	}
}

func TestTranscriberFailureQuitsWithError(t *testing.T) {
	h := listening(t, 80, 24)
	_, cmd := h.app.Update(updateMsg(TextUpdate{Err: errors.New("model failed")}))
	if !isQuit(cmd) || h.app.Err() == nil {
		t.Fatal("transcriber error should end the UI and be reported")
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  []string
	}{
		{"", 10, nil},
		{"short", 10, []string{"short"}},
		{"one two three four", 9, []string{"one two", "three", "four"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"ik heb één café", 8, []string{"ik heb", "één café"}},
	}
	for _, tc := range tests {
		got := wrap(tc.in, tc.width)
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("wrap(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

// playing returns an App in file mode (AutoStart, label "audio"), with channels the test controls.
func playing(t *testing.T) (*harness, chan TextUpdate, chan struct{}) {
	t.Helper()
	updates := make(chan TextUpdate, 16)
	done := make(chan struct{})
	hr := &harness{clock: &testClock{t: time.Unix(1_000_000, 0)}}
	hr.app = New(Config{
		AutoStart: true,
		Start: func(audio.Device) (Source, error) {
			return Source{Level: func() float32 { return hr.level }, Updates: updates, Done: done, Label: "audio", Length: 30 * time.Second}, nil
		},
		Count: countWords,
		Clock: hr.clock.now,
	})
	hr.app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	msg := hr.app.Init()()
	if _, ok := msg.(DeviceChosenMsg); !ok {
		t.Fatalf("Init produced %#v, want DeviceChosenMsg", msg)
	}
	hr.app.Update(msg)
	if !hr.app.listening {
		t.Fatal("AutoStart did not start the source")
	}
	return hr, updates, done
}

func TestAutoStartSkipsPickerAndLabelsAudio(t *testing.T) {
	h, _, _ := playing(t)
	view := h.app.View()
	if strings.Contains(view, "Select a microphone") {
		t.Fatalf("picker shown in file mode:\n%s", view)
	}
	if !strings.Contains(view, "audio ") || strings.Contains(view, "mic ") {
		t.Fatalf("level label should read audio, view:\n%s", view)
	}
	if strings.Contains(view, "finished") {
		t.Fatalf("finished shown while playing:\n%s", view)
	}
}

func TestMicLabelUnchanged(t *testing.T) {
	h := listening(t, 80, 24)
	if view := h.app.View(); !strings.Contains(view, "mic ") || strings.Contains(view, "audio") {
		t.Fatalf("mic mode should keep the mic label, view:\n%s", view)
	}
}

func TestDoneFreezesClockAndShowsFinished(t *testing.T) {
	h, _, _ := playing(t)
	h.clock.advance(32 * time.Second) // playback took 30 s; the decoder needed 2 s more to deliver the last words
	h.text(strings.Repeat("w ", 60), true)
	h.app.Update(doneMsg{})

	if !strings.Contains(h.app.View(), "finished") {
		t.Fatalf("no finished indication:\n%s", h.app.View())
	}
	elapsed, wpmBefore, view := h.app.calc.Elapsed(), h.app.wpm, h.app.View()

	h.clock.advance(5 * time.Minute) // time passes while the user reads the result
	_, cmd := h.app.Update(tickMsg(h.clock.now()))
	if cmd != nil {
		t.Error("ticking should stop once finished")
	}
	if got := h.app.calc.Elapsed(); got != elapsed || got != 30*time.Second {
		t.Errorf("elapsed = %v after done, want it frozen at 30s", got)
	}
	if h.app.wpm != wpmBefore {
		t.Errorf("WPM moved from %v to %v after done", wpmBefore, h.app.wpm)
	}
	if h.app.View() != view {
		t.Errorf("view changed after done:\n%s\nvs\n%s", view, h.app.View())
	}

	_, cmd = h.app.Update(keyQ)
	if !isQuit(cmd) {
		t.Fatal("q should still quit after finishing")
	}
	s, ok := h.app.Summary()
	if !ok || s.Words != 60 || s.Duration != 30*time.Second || s.AvgWPM < 119 || s.AvgWPM > 121 {
		t.Fatalf("summary = %+v, want 60 words over the frozen 30 s (120 WPM)", s)
	}
}

func TestDoneAppliesUpdatesQueuedBeforeIt(t *testing.T) {
	h, _, _ := playing(t)
	h.clock.advance(10 * time.Second)
	h.app.Update(doneMsg{rest: []TextUpdate{{Text: "last words here", Final: true}}})
	if h.app.calc.Total() != 3 || len(h.app.lines) != 1 {
		t.Fatalf("total=%d lines=%v, want the queued final counted", h.app.calc.Total(), h.app.lines)
	}
	if !h.app.finished {
		t.Fatal("not finished")
	}
}

func TestDoneWithQueuedErrorQuits(t *testing.T) {
	h, _, _ := playing(t)
	_, cmd := h.app.Update(doneMsg{rest: []TextUpdate{{Err: errors.New("model failed")}}})
	if !isQuit(cmd) || h.app.Err() == nil {
		t.Fatal("a transcriber error queued before done should end the UI with the error")
	}
}

func TestWaitEventDeliversQueuedUpdatesBeforeDone(t *testing.T) {
	updates := make(chan TextUpdate, 4)
	done := make(chan struct{})
	updates <- TextUpdate{Text: "a b", Final: true}
	updates <- TextUpdate{Text: "c", Final: true}
	close(done) // done is already closed while two updates are still queued

	var got []string
	for i := 0; i < 3; i++ {
		switch m := waitEvent(updates, done)().(type) {
		case updateMsg:
			got = append(got, m.Text)
		case doneMsg:
			for _, u := range m.rest {
				got = append(got, u.Text)
			}
			if fmt.Sprint(got) != "[a b c]" {
				t.Fatalf("got %v, want both queued updates exactly once, in order", got)
			}
			return
		default:
			t.Fatalf("unexpected message %#v", m)
		}
	}
	t.Fatal("never saw done")
}

func TestQuitDuringPlaybackSummarisesPlayedPortion(t *testing.T) {
	h, _, _ := playing(t)
	h.clock.advance(12 * time.Second)
	h.text(strings.Repeat("w ", 24), true)

	_, cmd := h.app.Update(keyQ)
	if !isQuit(cmd) {
		t.Fatal("q should quit during playback")
	}
	s, ok := h.app.Summary()
	if !ok || s.Words != 24 || s.Duration != 12*time.Second {
		t.Fatalf("summary = %+v, want 24 words in 12 s", s)
	}
	if h.app.finished {
		t.Error("quitting early must not count as finished")
	}
}

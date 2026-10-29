package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wordcount/internal/audio"
	"wordcount/internal/wpm"
)

const (
	tickEvery  = 250 * time.Millisecond // WPM and level refresh; well inside the 1 s requirement
	levelWidth = 30
	defaultW   = 80
	defaultH   = 24
)

// TextUpdate is a transcription result from the decoder.
type TextUpdate struct {
	Text  string
	Final bool
	Err   error
}

// Source is what a running capture provides to the UI.
type Source struct {
	Level   func() float32
	Updates <-chan TextUpdate
	Errs    <-chan error
	Done    <-chan struct{} // closed when a finite source (a file) has ended and its last update was sent; nil for live capture
	Label   string          // names the source next to the level bar; "mic" when empty
	Length  time.Duration   // playing time of a finite source; the clock freezes there, not when the decoder catches up
}

// Starter opens the chosen device and begins transcription.
type Starter func(audio.Device) (Source, error)

// Config wires the UI to the rest of the program.
type Config struct {
	Devices     []audio.Device // shown in the picker
	Preselected *audio.Device  // when set, the picker is skipped
	AutoStart   bool           // start immediately with a zero device, for sources that need none (a file)
	Start       Starter
	Count       func(string) int // words in a piece of text
	Clock       func() time.Time // defaults to time.Now
}

// Summary is printed after the UI exits.
type Summary struct {
	Words    int
	Duration time.Duration
	AvgWPM   float64
}

// String renders the one-line exit summary.
func (s Summary) String() string {
	return fmt.Sprintf("%d words in %s, average %.0f WPM", s.Words, formatElapsed(s.Duration), s.AvgWPM)
}

type (
	tickMsg   time.Time
	updateMsg TextUpdate
	errMsg    struct{ error }

	// doneMsg reports that a finite source has ended. rest holds the updates
	// still queued at that moment, in order, so none is lost to the freeze.
	doneMsg struct{ rest []TextUpdate }
)

// App is the top-level Bubble Tea model: picker, then the live view.
type App struct {
	cfg    Config
	picker Picker

	listening bool
	calc      *wpm.Calculator
	src       Source

	started    time.Time // when the source began, for freezing a finite source at its length
	finished   bool      // the source ended; the clock is frozen at finishedAt
	finishedAt time.Time

	lines   []string // finalised utterances
	partial string
	wpm     float64
	level   float32

	width, height int
	err           error
	summary       *Summary
}

// New returns the UI model.
func New(cfg Config) *App {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &App{cfg: cfg, picker: NewPicker(cfg.Devices), width: defaultW, height: defaultH}
}

// Err is the failure that ended the UI, if any (device or transcriber error).
func (a *App) Err() error { return a.err }

// Summary returns the session summary once the user quit while listening.
func (a *App) Summary() (Summary, bool) {
	if a.summary == nil {
		return Summary{}, false
	}
	return *a.summary, true
}

// Init skips the picker when a device was given on the command line or the source needs none.
func (a *App) Init() tea.Cmd {
	if a.cfg.AutoStart {
		return func() tea.Msg { return DeviceChosenMsg{} }
	}
	if a.cfg.Preselected != nil {
		dev := *a.cfg.Preselected
		return func() tea.Msg { return DeviceChosenMsg{Device: dev} }
	}
	return nil
}

// Update handles keys, size changes, ticks and transcription results.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return a, a.quit()
		}
		if !a.listening {
			var cmd tea.Cmd
			a.picker, cmd = a.picker.Update(msg)
			return a, cmd
		}

	case DeviceChosenMsg:
		if a.listening {
			return a, nil
		}
		src, err := a.cfg.Start(msg.Device)
		if err != nil {
			a.err = err
			return a, tea.Quit
		}
		a.listening = true
		a.src = src
		a.calc = wpm.New(a.now)
		return a, tea.Batch(tick(), waitEvent(src.Updates, src.Done), waitErr(src.Errs))

	case tickMsg:
		a.refresh()
		if a.finished {
			return a, nil // nothing moves any more
		}
		return a, tick()

	case updateMsg:
		if msg.Err != nil {
			a.err = msg.Err
			return a, tea.Quit
		}
		a.apply(TextUpdate(msg))
		return a, waitEvent(a.src.Updates, a.src.Done)

	case doneMsg:
		for _, u := range msg.rest {
			if u.Err != nil {
				a.err = u.Err
				return a, tea.Quit
			}
			a.apply(u)
		}
		a.finish()

	case errMsg:
		a.err = msg.error
		return a, tea.Quit
	}
	return a, nil
}

// now is the calculator's clock: wall time, until the source finishes. Its
// first reading is the calculator's start, which finish measures the source's length from.
func (a *App) now() time.Time {
	if a.finished {
		return a.finishedAt
	}
	t := a.cfg.Clock()
	if a.started.IsZero() {
		a.started = t
	}
	return t
}

// finish freezes the elapsed time, WPM and level for the finished state. A
// source with a known length stops the clock at that length: the final words
// may arrive a moment after playback ended, which must not count as listening time.
func (a *App) finish() {
	a.finishedAt = a.cfg.Clock()
	if end := a.started.Add(a.src.Length); a.src.Length > 0 && end.Before(a.finishedAt) {
		a.finishedAt = end
	}
	a.finished = true
	a.level = 0
	a.wpm = a.calc.WPM()
}

// quit records the summary (if listening) and ends the program.
func (a *App) quit() tea.Cmd {
	if a.listening && a.summary == nil {
		s := Summary{Words: a.calc.Total(), Duration: a.calc.Elapsed()}
		if s.Duration > 0 {
			s.AvgWPM = float64(s.Words) / s.Duration.Minutes()
		}
		a.summary = &s
	}
	return tea.Quit
}

func (a *App) refresh() {
	a.wpm = a.calc.WPM()
	if a.src.Level != nil {
		a.level = a.src.Level()
	}
}

// apply folds a transcription result into the transcript and the word counter.
func (a *App) apply(u TextUpdate) {
	a.calc.Update(a.cfg.Count(u.Text))
	if u.Final {
		a.calc.Finalize()
		if u.Text != "" {
			a.lines = append(a.lines, u.Text)
		}
		a.partial = ""
	} else {
		a.partial = u.Text
	}
	a.wpm = a.calc.WPM()
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// waitEvent delivers the next update, or doneMsg when done closes. One
// goroutine watches both so that updates sent before done are never applied
// after it.
func waitEvent(ch <-chan TextUpdate, done <-chan struct{}) tea.Cmd {
	if ch == nil && done == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case u, ok := <-ch:
			if !ok {
				return nil
			}
			return updateMsg(u)
		case <-done:
			return doneMsg{drain(ch)}
		}
	}
}

// drain takes whatever is queued on ch right now without blocking.
func drain(ch <-chan TextUpdate) []TextUpdate {
	var out []TextUpdate
	for ch != nil {
		select {
		case u, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, u)
		default:
			return out
		}
	}
	return out
}

func waitErr(ch <-chan error) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return nil
		}
		return errMsg{err}
	}
}

// View draws the picker, or the WPM header, stats, level bar and transcript.
func (a *App) View() string {
	if !a.listening {
		return a.picker.View() + "\n"
	}

	label := a.src.Label
	if label == "" {
		label = "mic"
	}
	stats := fmt.Sprintf("words %d   time %s   %s ", a.calc.Total(), formatElapsed(a.calc.Elapsed()), label)
	head := []string{
		RenderWPM(int(a.wpm+0.5), a.width, a.height),
		stats + levelBar(a.level, a.width-utf8.RuneCountInString(stats)),
		strings.Repeat("─", min(a.width, 80)),
	}
	top := strings.Join(head, "\n")
	footer := lipgloss.NewStyle().Faint(true).Render("q to quit")
	if a.finished {
		footer = lipgloss.NewStyle().Bold(true).Render("finished") + lipgloss.NewStyle().Faint(true).Render("  q to quit")
	}

	room := a.height - lipgloss.Height(top) - 2
	if room < 1 {
		room = 1
	}
	return top + "\n" + strings.Join(a.transcriptTail(room), "\n") + "\n" + footer
}

// transcriptTail wraps the feed to the terminal width and keeps the newest
// rows that fit; the in-progress utterance is the last, faint, row(s).
func (a *App) transcriptTail(rows int) []string {
	var out []string
	for _, l := range a.lines {
		out = append(out, wrap(l, a.width)...)
	}
	if a.partial != "" {
		faint := lipgloss.NewStyle().Faint(true).Italic(true)
		for _, l := range wrap(a.partial, a.width) {
			out = append(out, faint.Render(l))
		}
	}
	if len(out) > rows {
		out = out[len(out)-rows:]
	}
	return out
}

// levelBar draws the input level in at most levelWidth cells, fewer if the
// terminal leaves less room. Speech RMS is roughly 0.05-0.3, hence the x4 gain.
func levelBar(level float32, room int) string {
	width := min(levelWidth, room)
	if width < 1 {
		return ""
	}
	fill := int(level * 4 * float32(width))
	fill = max(0, min(width, fill))
	return strings.Repeat("█", fill) + strings.Repeat("░", width-fill)
}

func formatElapsed(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// wrap breaks text at spaces into lines of at most width characters,
// splitting words that are longer than a line.
func wrap(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	var lines []string
	var cur string
	for _, w := range strings.Fields(text) {
		for utf8.RuneCountInString(w) > width {
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			r := []rune(w)
			lines = append(lines, string(r[:width]))
			w = string(r[width:])
		}
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

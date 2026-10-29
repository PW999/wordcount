package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"wordcount/internal/audio"
)

var (
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
	keyQ     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
)

func devices(names ...string) []audio.Device {
	devs := make([]audio.Device, len(names))
	for i, n := range names {
		devs[i] = audio.Device{Index: i, Name: n}
	}
	return devs
}

func TestPickerArrowKeysMoveAndClamp(t *testing.T) {
	p := NewPicker(devices("a", "b", "c"))
	if p.Cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", p.Cursor)
	}
	p, _ = p.Update(keyUp) // already at top
	if p.Cursor != 0 {
		t.Fatalf("cursor moved above the first device: %d", p.Cursor)
	}
	p, _ = p.Update(keyDown)
	p, _ = p.Update(keyDown)
	p, _ = p.Update(keyDown) // already at bottom
	if p.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", p.Cursor)
	}
	p, _ = p.Update(keyUp)
	if p.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", p.Cursor)
	}
}

func TestPickerEnterChoosesHighlightedDevice(t *testing.T) {
	p := NewPicker(devices("a", "b", "c"))
	p, _ = p.Update(keyDown)
	_, cmd := p.Update(keyEnter)
	if cmd == nil {
		t.Fatal("Enter returned no command")
	}
	msg, ok := cmd().(DeviceChosenMsg)
	if !ok || msg.Device.Name != "b" {
		t.Fatalf("cmd produced %#v, want DeviceChosenMsg for b", cmd())
	}
}

func TestPickerSingleDeviceIsPreHighlighted(t *testing.T) {
	p := NewPicker(devices("only mic"))
	if !strings.Contains(p.View(), "> 0: only mic") {
		t.Fatalf("single device should be highlighted, view:\n%s", p.View())
	}
	_, cmd := p.Update(keyEnter)
	if cmd == nil {
		t.Fatal("Enter should confirm the single device")
	}
	if msg := cmd().(DeviceChosenMsg); msg.Device.Name != "only mic" {
		t.Fatalf("chose %q", msg.Device.Name)
	}
}

func TestPickerViewMarksOnlyHighlightedRow(t *testing.T) {
	p := NewPicker(devices("a", "b"))
	p, _ = p.Update(keyDown)
	view := p.View()
	if !strings.Contains(view, "> 1: b") || strings.Contains(view, "> 0: a") {
		t.Fatalf("wrong highlight in view:\n%s", view)
	}
}

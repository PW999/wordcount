package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wordcount/internal/audio"
)

// DeviceChosenMsg is sent when the user confirms a device in the picker.
type DeviceChosenMsg struct{ Device audio.Device }

// Picker is the capture-device list. The first device is highlighted initially,
// so with a single device the user only has to press Enter.
type Picker struct {
	Devices []audio.Device
	Cursor  int
}

// NewPicker returns a picker over devs with the first entry highlighted.
func NewPicker(devs []audio.Device) Picker { return Picker{Devices: devs} }

// Update moves the highlight with the arrow keys (or k/j) and confirms with Enter.
func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || len(p.Devices) == 0 {
		return p, nil
	}
	switch key.String() {
	case "up", "k":
		if p.Cursor > 0 {
			p.Cursor--
		}
	case "down", "j":
		if p.Cursor < len(p.Devices)-1 {
			p.Cursor++
		}
	case "enter":
		dev := p.Devices[p.Cursor]
		return p, func() tea.Msg { return DeviceChosenMsg{Device: dev} }
	}
	return p, nil
}

// View renders the list with the highlighted row marked.
func (p Picker) View() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Select a microphone"))
	sb.WriteString("\n\n")
	for i, d := range p.Devices {
		line := fmt.Sprintf("%d: %s", d.Index, d.Name)
		if i == p.Cursor {
			sb.WriteString(lipgloss.NewStyle().Bold(true).Render("> " + line))
		} else {
			sb.WriteString("  " + line)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n↑/↓ move · Enter select · q quit")
	return sb.String()
}

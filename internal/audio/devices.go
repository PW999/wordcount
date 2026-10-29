// Package audio enumerates capture devices and streams microphone audio.
package audio

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gen2brain/malgo"
)

// ErrNoDevice is returned when the machine has no capture device.
var ErrNoDevice = errors.New("no microphone found")

// Device is a capture device with a stable index for this enumeration.
type Device struct {
	Index int
	Name  string
	ID    malgo.DeviceID
}

// Backend enumerates capture devices. It exists so callers can be tested with a fake.
type Backend interface {
	CaptureDevices() ([]Device, error)
}

// ListDevices returns the capture devices, or ErrNoDevice if there are none.
func ListDevices(b Backend) ([]Device, error) {
	devs, err := b.CaptureDevices()
	if err != nil {
		return nil, err
	}
	if len(devs) == 0 {
		return nil, ErrNoDevice
	}
	return devs, nil
}

// FormatDevices renders one "<index>: <name>" line per device.
func FormatDevices(devs []Device) string {
	var sb strings.Builder
	for _, d := range devs {
		fmt.Fprintf(&sb, "%d: %s\n", d.Index, d.Name)
	}
	return sb.String()
}

// Resolve picks a device from a --device value: an index, or a name substring
// (case-insensitive) that matches exactly one device. Errors include the device list.
func Resolve(devs []Device, spec string) (Device, error) {
	if idx, err := strconv.Atoi(spec); err == nil {
		for _, d := range devs {
			if d.Index == idx {
				return d, nil
			}
		}
		return Device{}, fmt.Errorf("no device with index %d\n%s", idx, FormatDevices(devs))
	}

	var matches []Device
	needle := strings.ToLower(spec)
	for _, d := range devs {
		if strings.Contains(strings.ToLower(d.Name), needle) {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Device{}, fmt.Errorf("no device matches %q\n%s", spec, FormatDevices(devs))
	default:
		return Device{}, fmt.Errorf("%q matches %d devices, be more specific\n%s", spec, len(matches), FormatDevices(matches))
	}
}

// MalgoBackend enumerates and opens devices through miniaudio.
type MalgoBackend struct {
	ctx *malgo.AllocatedContext
}

// NewMalgoBackend initialises miniaudio, loading PulseAudio/PipeWire/ALSA at runtime.
func NewMalgoBackend() (*MalgoBackend, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialise audio: %w", err)
	}
	return &MalgoBackend{ctx: ctx}, nil
}

// Close releases the miniaudio context.
func (b *MalgoBackend) Close() {
	_ = b.ctx.Uninit()
	b.ctx.Free()
}

// CaptureDevices implements Backend.
func (b *MalgoBackend) CaptureDevices() ([]Device, error) {
	infos, err := b.ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("list capture devices: %w", err)
	}
	devs := make([]Device, len(infos))
	for i := range infos {
		devs[i] = Device{Index: i, Name: infos[i].Name(), ID: infos[i].ID}
	}
	return devs, nil
}

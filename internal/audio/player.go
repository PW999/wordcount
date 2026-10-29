package audio

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// ErrNoOutputDevice is returned when the machine has no playback device.
var ErrNoOutputDevice = errors.New("no audio output device found")

// OutputBackend enumerates playback devices. It exists so callers can be tested with a fake.
type OutputBackend interface {
	PlaybackDevices() ([]Device, error)
}

// PlaybackDevices implements OutputBackend.
func (b *MalgoBackend) PlaybackDevices() ([]Device, error) {
	infos, err := b.ctx.Devices(malgo.Playback)
	if err != nil {
		return nil, fmt.Errorf("list output devices: %w", err)
	}
	devs := make([]Device, len(infos))
	for i := range infos {
		devs[i] = Device{Index: i, Name: infos[i].Name(), ID: infos[i].ID}
	}
	return devs, nil
}

// RequireOutput returns ErrNoOutputDevice unless at least one playback device exists.
func RequireOutput(b OutputBackend) error {
	devs, err := b.PlaybackDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return ErrNoOutputDevice
	}
	return nil
}

// FilePlayer plays a decoded file through the default output device and, in
// step with what it hands to the device, streams the matching mono 16 kHz
// audio for transcription. The stream ends (Chunks is closed) once the last
// samples have been played, so analysis never runs ahead of playback.
type FilePlayer struct {
	file *Decoded
	mono []float32 // file as mono 16 kHz, indexed in step with file

	dev    *malgo.Device
	chunks chan []float32
	errs   chan error

	// pos and sent are only touched by the audio callback.
	pos  int // frames of file played so far
	sent int // samples of mono delivered so far

	level   atomic.Uint32 // float32 bits of the latest RMS
	dropped atomic.Uint64
	closing atomic.Bool
}

// NewFilePlayer opens the default output device and starts playing d.
func NewFilePlayer(b *MalgoBackend, d *Decoded) (*FilePlayer, error) {
	if err := RequireOutput(b); err != nil {
		return nil, err
	}
	p := newFilePlayer(d)

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = uint32(d.Channels)
	cfg.SampleRate = uint32(d.SampleRate)

	dev, err := malgo.InitDevice(b.ctx.Context, cfg, malgo.DeviceCallbacks{Data: p.onData, Stop: p.onStop})
	if err != nil {
		return nil, fmt.Errorf("open output device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return nil, fmt.Errorf("start output device: %w", err)
	}
	p.dev = dev
	return p, nil
}

// newFilePlayer prepares the playback state without touching any device.
func newFilePlayer(d *Decoded) *FilePlayer {
	return &FilePlayer{
		file:   d,
		mono:   Mono16k(d),
		chunks: make(chan []float32, chunkBuffer),
		errs:   make(chan error, 1),
	}
}

// monoIndex maps a position in native frames to an index into the 16 kHz mono audio.
func (p *FilePlayer) monoIndex(frame int) int {
	return min(len(p.mono), int(int64(frame)*SampleRate/int64(p.file.SampleRate)))
}

// onData runs on the audio thread: it copies, never blocks.
func (p *FilePlayer) onData(out, _ []byte, frames uint32) {
	ch := p.file.Channels
	n := int(frames) * ch
	if n == 0 || len(out) < n*4 {
		return
	}
	dst := unsafe.Slice((*float32)(unsafe.Pointer(&out[0])), n)

	if p.pos >= p.file.Frames() { // already finished: the device keeps asking, give it silence
		clear(dst)
		return
	}
	copied := copy(dst, p.file.Samples[p.pos*ch:])
	clear(dst[copied:])

	p.pos = min(p.pos+int(frames), p.file.Frames())
	end := p.monoIndex(p.pos)
	if p.pos == p.file.Frames() {
		end = len(p.mono) // rounding must not strand the last samples
	}
	if end > p.sent {
		p.emit(p.mono[p.sent:end])
		p.sent = end
	}
	if p.pos == p.file.Frames() {
		close(p.chunks)
	}
}

func (p *FilePlayer) emit(samples []float32) {
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	p.level.Store(math.Float32bits(float32(math.Sqrt(sum / float64(len(samples))))))

	select {
	case p.chunks <- append([]float32(nil), samples...):
	default:
		p.dropped.Add(1)
	}
}

func (p *FilePlayer) onStop() {
	if p.closing.Load() {
		return
	}
	select {
	case p.errs <- errors.New("audio output device stopped unexpectedly"):
	default:
	}
}

// Chunks delivers the 16 kHz mono audio matching what has been played, then closes.
func (p *FilePlayer) Chunks() <-chan []float32 { return p.chunks }

// Level is the RMS (0..1) of the audio most recently played.
func (p *FilePlayer) Level() float32 { return math.Float32frombits(p.level.Load()) }

// Err receives a value if the output device fails during playback.
func (p *FilePlayer) Err() <-chan error { return p.errs }

// Dropped counts chunks discarded because the consumer fell behind.
func (p *FilePlayer) Dropped() uint64 { return p.dropped.Load() }

// Close stops playback and releases the device.
func (p *FilePlayer) Close() {
	p.closing.Store(true)
	if p.dev == nil {
		return
	}
	_ = p.dev.Stop()
	p.dev.Uninit()
}

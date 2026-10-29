package audio

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// SampleRate is the rate every chunk is delivered at, whatever the device's native format.
const SampleRate = 16000

// chunkBuffer is how many callback-sized chunks may queue before new ones are dropped.
const chunkBuffer = 256

// Capture streams mono 16 kHz float32 audio from one device.
type Capture struct {
	dev     *malgo.Device
	chunks  chan []float32
	errs    chan error
	level   atomic.Uint32 // float32 bits of the latest RMS
	dropped atomic.Uint64
	closing atomic.Bool
}

// Start opens dev and begins capturing. The device is converted to mono,
// 16 kHz, float32 by miniaudio.
func (b *MalgoBackend) Start(dev Device) (*Capture, error) {
	// miniaudio can crash when handed an id that no longer exists, so check first.
	current, err := b.CaptureDevices()
	if err != nil {
		return nil, err
	}
	present := false
	for _, d := range current {
		if d.ID == dev.ID {
			present = true
			break
		}
	}
	if !present {
		return nil, fmt.Errorf("open %q: device is no longer available", dev.Name)
	}

	c := &Capture{
		chunks: make(chan []float32, chunkBuffer),
		errs:   make(chan error, 1),
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate
	cfg.Capture.DeviceID = dev.ID.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: c.onData,
		Stop: c.onStop,
	}
	d, err := malgo.InitDevice(b.ctx.Context, cfg, callbacks)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dev.Name, err)
	}
	if err := d.Start(); err != nil {
		d.Uninit()
		return nil, fmt.Errorf("start %q: %w", dev.Name, err)
	}
	c.dev = d
	return c, nil
}

// onData runs on the audio thread: it copies, never blocks.
func (c *Capture) onData(_, in []byte, frames uint32) {
	n := int(frames)
	if n == 0 || len(in) < n*4 {
		return
	}
	samples := make([]float32, n)
	copy(samples, unsafe.Slice((*float32)(unsafe.Pointer(&in[0])), n))

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	c.level.Store(math.Float32bits(float32(math.Sqrt(sum / float64(n)))))

	select {
	case c.chunks <- samples:
	default:
		c.dropped.Add(1)
	}
}

func (c *Capture) onStop() {
	if c.closing.Load() {
		return
	}
	select {
	case c.errs <- errors.New("audio device stopped unexpectedly"):
	default:
	}
}

// Chunks delivers captured samples in callback-sized pieces.
func (c *Capture) Chunks() <-chan []float32 { return c.chunks }

// Level is the RMS (0..1) of the most recent chunk.
func (c *Capture) Level() float32 { return math.Float32frombits(c.level.Load()) }

// Err receives a value if the device fails or disappears during capture.
func (c *Capture) Err() <-chan error { return c.errs }

// Dropped counts chunks discarded because the consumer fell behind.
func (c *Capture) Dropped() uint64 { return c.dropped.Load() }

// Close stops capture and releases the device.
func (c *Capture) Close() {
	c.closing.Store(true)
	_ = c.dev.Stop()
	c.dev.Uninit()
}

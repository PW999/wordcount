// Command wordcount shows live words per minute and a running transcript from
// a microphone or an audio file, using whisper.cpp locally. It is the
// entrypoint package that wires internal/audio, internal/stt and internal/ui
// together; the Makefile builds it as ./wordcount.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/malgo"

	"wordcount/internal/audio"
	"wordcount/internal/stt"
	"wordcount/internal/ui"
)

func main() {
	model := flag.String("model", "", "path to a ggml-*.bin whisper model")
	device := flag.String("device", "", "capture device: index, or a unique name substring")
	file := flag.String("file", "", "play and analyse a WAV or MP3 file instead of the microphone")
	lang := flag.String("lang", "auto", "spoken language code, or auto")
	listDevices := flag.Bool("list-devices", false, "print capture devices as \"<index>: <name>\" and exit")
	noGPU := flag.Bool("no-gpu", false, "force the CPU (GPU builds only)")
	gpuDevice := flag.Int("gpu-device", -1, "index of the GPU to run the model on")
	showVersion := flag.Bool("version", false, "print backends, GPUs and CPU features and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: wordcount --model <path> [options]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		printVersion()
		return
	}

	if err := run(*model, *device, *file, *lang, *listDevices, *noGPU, *gpuDevice); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(model, device, file, lang string, listDevices, noGPU bool, gpuDevice int) error {
	if listDevices {
		be, err := audio.NewMalgoBackend()
		if err != nil {
			return err
		}
		defer be.Close()
		devs, err := audio.ListDevices(be)
		if err != nil {
			return err
		}
		fmt.Print(audio.FormatDevices(devs))
		return nil
	}

	if device != "" && file != "" {
		return errors.New("--file cannot be combined with --device")
	}
	if gpuDevice >= 0 {
		if !stt.GPUBuild() {
			return errors.New("this binary was built without GPU support; rebuild with GPU=vulkan to use --gpu-device")
		}
		if noGPU {
			return errors.New("--gpu-device cannot be combined with --no-gpu")
		}
		gpus := stt.Devices()
		if gpuDevice >= len(gpus) {
			return fmt.Errorf("--gpu-device %d: no such GPU (indices 0-%d, see --version)", gpuDevice, len(gpus)-1)
		}
	}
	if model == "" {
		return errors.New("no model given: pass --model <path to a ggml-*.bin whisper model>")
	}

	w, err := stt.NewWhisper(stt.WhisperConfig{
		ModelPath: model,
		Lang:      lang,
		NoGPU:     noGPU,
		GPUDevice: max(gpuDevice, 0),
	})
	if err != nil {
		return err
	}
	defer w.Close()

	// A CPU fallback in a GPU build, and any setup notice, are printed before
	// the screen opens so they can never be missed.
	if warn := w.Warning(); warn != "" {
		fmt.Fprintln(os.Stderr, "warning:", warn)
	}
	fmt.Printf("compute backend: %s\n", w.Backend())

	var cfg ui.Config
	var cleanup func()

	switch {
	case file != "":
		decoded, err := audio.DecodeFile(file)
		if err != nil {
			return err
		}
		src, stop, err := fileSource(w, decoded, file)
		if err != nil {
			return err
		}
		cfg = ui.Config{AutoStart: true, Start: src, Count: stt.CountWords}
		cleanup = stop

	default:
		be, err := audio.NewMalgoBackend()
		if err != nil {
			return err
		}
		defer be.Close()
		devs, err := audio.ListDevices(be)
		if err != nil {
			return err
		}
		cfg = ui.Config{
			Devices: devs,
			Start:   micSource(w, be),
			Count:   stt.CountWords,
		}
		if device != "" {
			dev, err := audio.Resolve(devs, device)
			if err != nil {
				return err
			}
			cfg.Preselected = &dev
		}
	}

	app := ui.New(cfg)
	program := tea.NewProgram(app, tea.WithAltScreen())
	final, err := program.Run()
	if cleanup != nil {
		cleanup()
	}
	if err != nil {
		return fmt.Errorf("ui: %w", err)
	}
	app = final.(*ui.App)
	if err := app.Err(); err != nil {
		return err
	}
	if s, ok := app.Summary(); ok {
		fmt.Println(s.String())
	}
	return nil
}

// printVersion lists the compute backends, the GPUs with the indices
// --gpu-device uses, and the CPU features whisper.cpp was built with.
func printVersion() {
	fmt.Println("wordcount")
	backends := stt.BackendNames()
	if len(backends) == 0 {
		fmt.Println("backends: none")
	} else {
		fmt.Println("backends:", strings.Join(backends, ", "))
	}
	gpus := stt.Devices()
	if len(gpus) == 0 {
		fmt.Println("GPUs: none (CPU-only build or no GPU available)")
	} else {
		fmt.Println("GPUs:")
		for _, g := range gpus {
			fmt.Printf("  %s\n", g.String())
		}
	}
	fmt.Println("CPU:", stt.SystemInfo())
}

// micSource returns a Starter that opens the chosen capture device and feeds
// a segmenter/decoder pipeline; it runs for as long as the UI does.
func micSource(w *stt.Whisper, be *audio.MalgoBackend) ui.Starter {
	return func(dev audio.Device) (ui.Source, error) {
		cap, err := be.Start(dev)
		if err != nil {
			return ui.Source{}, err
		}
		dec := stt.NewDecoder(w)
		seg := stt.NewSegmenter(stt.DefaultSegmenterConfig())

		updates := make(chan ui.TextUpdate, 64)
		errs := make(chan error, 4)

		// Transcription results: the decoder's Err rides along in the update.
		go func() {
			defer close(updates)
			for u := range dec.Updates() {
				updates <- ui.TextUpdate(u)
			}
		}()
		// Device failures.
		go func() {
			if err, ok := <-cap.Err(); ok && err != nil {
				errs <- err
			}
		}()
		// Audio: capture chunks become utterance requests. Segmenter and
		// decoder never block, so this goroutine can always keep up.
		go func() {
			for chunk := range cap.Chunks() {
				for _, req := range seg.Push(chunk) {
					dec.Submit(req)
				}
			}
		}()

		return ui.Source{Level: cap.Level, Updates: updates, Errs: errs}, nil
	}
}

// fileSource builds a Starter that plays the decoded file through the default
// output device and feeds the segmenter exactly the audio that has been
// heard, so the display moves in step with the playback. It returns the
// Starter plus a cleanup function that stops playback.
func fileSource(w *stt.Whisper, d *audio.Decoded, path string) (ui.Starter, func(), error) {
	pl, err := newPlayer(d)
	if err != nil {
		return nil, nil, err
	}
	mono := toMono16k(d)

	start := func(audio.Device) (ui.Source, error) {
		dec := stt.NewDecoder(w)
		seg := stt.NewSegmenter(stt.DefaultSegmenterConfig())

		updates := make(chan ui.TextUpdate, 64)
		errs := make(chan error, 4)
		done := make(chan struct{})

		var level atomic.Uint32 // RMS bits of the audio heard so far
		var expected, forwarded atomic.Int64
		feedDone := make(chan struct{})
		var finishOnce sync.Once
		finish := func() { finishOnce.Do(func() { close(done) }) }

		// Forward transcription results; when every final of the file has
		// been delivered, report the source as done so the UI can freeze.
		go func() {
			defer close(updates)
			defer close(errs)
			for u := range dec.Updates() {
				updates <- ui.TextUpdate(u)
				if !u.Final {
					continue
				}
				forwarded.Add(1)
				select {
				case <-feedDone:
					if forwarded.Load() >= expected.Load() {
						finish()
					}
				default:
				}
			}
		}()

		// Feed the segmenter at the pace of the playback, then flush the
		// trailing utterance once the file has been heard to the end.
		go func() {
			ratio := 1.0
			if n := d.Frames(); n > 0 {
				ratio = float64(len(mono)) / float64(n)
			}
			last := 0
			total := int64(0)
			tick := time.NewTicker(50 * time.Millisecond)
			defer tick.Stop()
			for range tick.C {
				target := int(float64(pl.Position()) * ratio)
				if target > len(mono) {
					target = len(mono)
				}
				if target > last {
					chunk := mono[last:target]
					level.Store(rms(chunk))
					for _, req := range seg.Push(chunk) {
						if req.Final {
							total++
						}
						dec.Submit(req)
					}
					last = target
				}
				if pl.Ended() && last >= len(mono) {
					for _, req := range seg.Flush() {
						if req.Final {
							total++
						}
						dec.Submit(req)
					}
					expected.Store(total)
					close(feedDone)
					// All finals may already have been delivered; check once
					// more after the channel close orders the store above.
					go func() {
						<-feedDone
						if forwarded.Load() >= expected.Load() {
							finish()
						}
					}()
					return
				}
			}
		}()

		return ui.Source{
			Level:   func() float32 { return math.Float32frombits(level.Load()) },
			Updates: updates,
			Errs:    errs,
			Done:    done,
			Label:   filepath.Base(path),
			Length:  d.Duration(),
		}, nil
	}
	return start, func() { pl.Close() }, nil
}

// player plays interleaved float32 samples through the default output device
// and reports how far playback has got.
type player struct {
	dev      *malgo.Device
	ctx      *malgo.AllocatedContext
	samples  []float32
	channels int
	pos      atomic.Int64 // frames handed to the device so far
	ended    atomic.Bool
}

// newPlayer opens the default output device at the file's own format; it
// fails when the machine has no usable output device.
func newPlayer(d *audio.Decoded) (*player, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialise audio output: %w", err)
	}
	p := &player{samples: d.Samples, channels: d.Channels}

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = uint32(d.Channels)
	cfg.SampleRate = uint32(d.SampleRate)
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: p.onData})
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, errors.New("no output device found")
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("start playback: %w", err)
	}
	p.dev, p.ctx = dev, ctx
	return p, nil
}

// onData runs on the audio thread: it copies the next samples, zero-fills
// beyond the end, and never blocks.
func (p *player) onData(_, out []byte, frames uint32) {
	n := int(frames) * p.channels
	start := int(p.pos.Load())
	end := start + n
	if start >= len(p.samples) {
		for i := range out {
			out[i] = 0
		}
		p.ended.Store(true)
		return
	}
	if end > len(p.samples) {
		end = len(p.samples)
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(&p.samples[start])), (end-start)*4)
	copy(out, src)
	for i := len(src); i < len(out); i++ {
		out[i] = 0
	}
	p.pos.Store(int64(end / p.channels))
	if end >= len(p.samples) {
		p.ended.Store(true)
	}
}

// Position is the number of frames handed to the device so far.
func (p *player) Position() int { return int(p.pos.Load()) }

// Ended reports whether the last frame has been played.
func (p *player) Ended() bool { return p.ended.Load() }

// Close stops playback and releases the device.
func (p *player) Close() {
	_ = p.dev.Stop()
	p.dev.Uninit()
	p.ctx.Uninit()
	p.ctx.Free()
}

// toMono16k mixes the channels to mono and linearly resamples to 16 kHz,
// the rate the segmenter and whisper expect.
func toMono16k(d *audio.Decoded) []float32 {
	frames := d.Frames()
	mono := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var sum float64
		for c := 0; c < d.Channels; c++ {
			sum += float64(d.Samples[i*d.Channels+c])
		}
		mono[i] = float32(sum / float64(d.Channels))
	}
	if d.SampleRate == audio.SampleRate {
		return mono
	}
	n := int(float64(frames) * float64(audio.SampleRate) / float64(d.SampleRate))
	out := make([]float32, n)
	for i := range out {
		pos := float64(i) * float64(d.SampleRate) / float64(audio.SampleRate)
		j := int(pos)
		if j >= frames-1 {
			out[i] = mono[frames-1]
			continue
		}
		frac := float32(pos - float64(j))
		out[i] = mono[j]*(1-frac) + mono[j+1]*frac
	}
	return out
}

// rms is the root mean square level of a chunk, stored as float32 bits.
func rms(chunk []float32) uint32 {
	var sum float64
	for _, s := range chunk {
		sum += float64(s) * float64(s)
	}
	return math.Float32bits(float32(math.Sqrt(sum / float64(len(chunk)))))
}

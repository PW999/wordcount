package stt

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// minSamples is the shortest input whisper accepts reliably (1 s); shorter
// utterances are zero-padded up to this.
const minSamples = sampleRate + sampleRate/10

// WhisperConfig selects the model and decoding options.
type WhisperConfig struct {
	ModelPath string
	Lang      string // language code, or "auto" (default)
	Threads   int    // 0 means min(NumCPU, 8)

	// NoGPU forces CPU execution. It is negative so that the zero value keeps
	// whisper.cpp's default of using the GPU when the build has one.
	NoGPU     bool
	GPUDevice int // index among the GPU devices; 0 is whisper.cpp's default
}

// gpuParams returns the use_gpu and gpu_device values passed to whisper.cpp.
func (c WhisperConfig) gpuParams() (useGPU bool, device int) {
	return !c.NoGPU, c.GPUDevice
}

// Whisper is a Transcriber backed by whisper.cpp. It is not safe for
// concurrent use; the Decoder calls it from a single goroutine.
type Whisper struct {
	model   whisper.Model
	ctx     whisper.Context
	warning string
	backend Backend
}

// NewWhisper loads the model and prepares a reusable decoding context. It
// fails with a descriptive error if the model is missing or not a valid model file.
func NewWhisper(cfg WhisperConfig) (*Whisper, error) {
	if cfg.ModelPath == "" {
		return nil, errors.New("no model given: pass --model <path to a ggml-*.bin whisper model>")
	}
	useGPU, device := cfg.gpuParams()
	devices := Devices()
	gpuBuild := GPUBuild()
	resetLoadLog()
	model, err := whisper.NewWithGPU(cfg.ModelPath, useGPU, device)
	backend := chooseBackend(readLoadLog(), devices, cfg.NoGPU, gpuBuild)
	if err != nil && useGPU && len(devices) > 0 && errors.Is(err, whisper.ErrUnableToLoadModel) {
		// The model may not fit in GPU memory; whisper.cpp then fails the load
		// instead of falling back, so try again on the CPU.
		resetLoadLog()
		var cpuErr error
		if model, cpuErr = whisper.NewWithGPU(cfg.ModelPath, false, 0); cpuErr == nil {
			err = nil
			backend = Backend{Note: "model could not be loaded on the GPU", Warn: true}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("load model %q: %w", cfg.ModelPath, err)
	}
	ctx, err := model.NewContext()
	if err != nil {
		_ = model.Close()
		return nil, fmt.Errorf("create whisper context: %w", err)
	}

	w := &Whisper{model: model, ctx: ctx, backend: backend}

	lang := cfg.Lang
	if lang == "" {
		lang = "auto"
	}
	if ctx.IsMultilingual() {
		if err := ctx.SetLanguage(lang); err != nil {
			_ = model.Close()
			return nil, fmt.Errorf("language %q: %w", lang, err)
		}
	} else if lang != "auto" && lang != "en" {
		// English-only models decode as English and reject SetLanguage.
		w.warning = fmt.Sprintf("model %q is English-only; ignoring --lang %s and using en", cfg.ModelPath, lang)
	}

	threads := cfg.Threads
	if threads <= 0 {
		threads = min(runtime.NumCPU(), 8)
	}
	ctx.SetThreads(uint(threads))
	return w, nil
}

// Backend reports whether the model runs on a GPU or the CPU, and why.
func (w *Whisper) Backend() Backend { return w.backend }

// Warning is a non-fatal notice from setup (for example an ignored --lang), or "".
func (w *Whisper) Warning() string { return w.warning }

// Transcribe implements Transcriber.
func (w *Whisper) Transcribe(samples []float32) (string, error) {
	if len(samples) < minSamples {
		padded := make([]float32, minSamples)
		copy(padded, samples)
		samples = padded
	}
	if err := w.ctx.Process(samples, nil, nil, nil); err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}
	var parts []string
	for {
		seg, err := w.ctx.NextSegment()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read segment: %w", err)
		}
		parts = append(parts, strings.TrimSpace(seg.Text))
	}
	return strings.Join(parts, " "), nil
}

// Close releases the model.
func (w *Whisper) Close() error { return w.model.Close() }

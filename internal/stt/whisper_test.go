package stt

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	defaultModel = "../../models/ggml-base.bin"
	sampleWAV    = "../../testdata/jfk.wav"
)

// readWAV decodes a 16 kHz mono 16-bit PCM WAV into float32 samples.
func readWAV(t *testing.T, path string) []float32 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("%s is not a WAV file", path)
	}
	for off := 12; off+8 <= len(b); {
		id, size := string(b[off:off+4]), int(binary.LittleEndian.Uint32(b[off+4:off+8]))
		body := off + 8
		if id == "data" {
			end := min(body+size, len(b))
			out := make([]float32, (end-body)/2)
			for i := range out {
				out[i] = float32(int16(binary.LittleEndian.Uint16(b[body+2*i:]))) / 32768
			}
			return out
		}
		off = body + size + size%2
	}
	t.Fatalf("%s has no data chunk", path)
	return nil
}

func modelPath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("WORDCOUNT_MODEL")
	if p == "" {
		p = defaultModel
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no whisper model at %s (set WORDCOUNT_MODEL): %v", p, err)
	}
	return p
}

func TestWhisperTranscribesSampleWAV(t *testing.T) {
	w, err := NewWhisper(WhisperConfig{ModelPath: modelPath(t), Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	text, err := w.Transcribe(readWAV(t, sampleWAV))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("transcript: %q", text)
	if !strings.Contains(strings.ToLower(text), "ask not what your country can do for you") {
		t.Fatalf("unexpected transcript %q", text)
	}
	if n := CountWords(Filter(text, readWAV(t, sampleWAV))); n < 15 {
		t.Fatalf("counted %d words, want at least 15", n)
	}
}

func TestWhisperShortUtteranceIsPadded(t *testing.T) {
	w, err := NewWhisper(WhisperConfig{ModelPath: modelPath(t), Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Transcribe(readWAV(t, sampleWAV)[:sampleRate/4]); err != nil {
		t.Fatalf("short input should not error: %v", err)
	}
}

func TestWhisperEnglishModelIgnoresOtherLanguage(t *testing.T) {
	path := os.Getenv("WORDCOUNT_EN_MODEL")
	if path == "" {
		t.Skip("set WORDCOUNT_EN_MODEL to an English-only model (ggml-*.en.bin) to run")
	}
	w, err := NewWhisper(WhisperConfig{ModelPath: path, Lang: "nl"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if !strings.Contains(w.Warning(), "English-only") {
		t.Fatalf("warning = %q, want an English-only notice", w.Warning())
	}
}

func TestWhisperMultilingualModelNoWarning(t *testing.T) {
	w, err := NewWhisper(WhisperConfig{ModelPath: modelPath(t), Lang: "nl"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Warning() != "" {
		t.Fatalf("unexpected warning %q", w.Warning())
	}
}

func TestWhisperRejectsBadModels(t *testing.T) {
	notAModel := filepath.Join(t.TempDir(), "junk.bin")
	if err := os.WriteFile(notAModel, []byte("this is not a whisper model"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing flag", "", "--model"},
		{"nonexistent file", filepath.Join(t.TempDir(), "nope.bin"), "no such file"},
		{"invalid model file", notAModel, "unable to load model"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, err := NewWhisper(WhisperConfig{ModelPath: tc.path})
			if err == nil {
				w.Close()
				t.Fatal("expected an error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

func TestGPUParamsDefaults(t *testing.T) {
	tests := []struct {
		name       string
		cfg        WhisperConfig
		wantUseGPU bool
		wantDevice int
	}{
		{"zero value keeps the whisper.cpp defaults", WhisperConfig{}, true, 0},
		{"no-gpu", WhisperConfig{NoGPU: true}, false, 0},
		{"device", WhisperConfig{GPUDevice: 1}, true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useGPU, device := tc.cfg.gpuParams()
			if useGPU != tc.wantUseGPU || device != tc.wantDevice {
				t.Fatalf("gpuParams() = (%v, %d), want (%v, %d)", useGPU, device, tc.wantUseGPU, tc.wantDevice)
			}
		})
	}
}

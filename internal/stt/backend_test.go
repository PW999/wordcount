package stt

import (
	"io"
	"os"
	"syscall"
	"testing"
)

// captureStderr returns what is written to fd 2 while fn runs.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := syscall.Dup(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Dup3(int(w.Fd()), 2, 0); err != nil {
		t.Fatal(err)
	}
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	syscall.Dup3(saved, 2, 0)
	syscall.Close(saved)
	w.Close()
	return <-done
}

func TestModelLoadRecordsBackendAndStaysQuiet(t *testing.T) {
	path := modelPath(t)
	resetLoadLog()
	var w *Whisper
	out := captureStderr(t, func() {
		var err error
		w, err = NewWhisper(WhisperConfig{ModelPath: path, NoGPU: true})
		if err != nil {
			t.Error(err)
		}
	})
	if w == nil {
		return
	}
	defer w.Close()
	if out != "" {
		t.Errorf("model load wrote to stderr: %q", out)
	}
	if !readLoadLog().NoGPU {
		t.Errorf(`load log did not record "no GPU found": %+v`, readLoadLog())
	}
	if got := w.Backend(); got.Device != nil || got.Warn {
		t.Errorf("Backend() with NoGPU = %+v, want CPU without warning", got)
	}
}

func TestDevicesMatchBuild(t *testing.T) {
	devs := Devices()
	for i, d := range devs {
		if d.Index != i {
			t.Errorf("device %d has Index %d", i, d.Index)
		}
		if d.Name == "" || d.Description == "" {
			t.Errorf("device %d has no name: %+v", i, d)
		}
	}
	if !GPUBuild() && len(devs) != 0 {
		t.Errorf("CPU-only build lists GPU devices: %v", devs)
	}
	hasCPU := false
	for _, n := range BackendNames() {
		hasCPU = hasCPU || n == "CPU"
	}
	if !hasCPU {
		t.Errorf("BackendNames() = %v, want it to include CPU", BackendNames())
	}
}

func TestChooseBackend(t *testing.T) {
	devs := []Device{{Index: 0, Name: "Vulkan0", Description: "Arc"}, {Index: 1, Name: "Vulkan1", Description: "RTX"}}
	tests := []struct {
		name     string
		log      loadLog
		noGPU    bool
		gpuBuild bool
		wantDev  int // -1 for CPU
		wantWarn bool
		wantNote string
	}{
		{"cpu-only build", loadLog{NoGPU: true}, false, false, -1, false, ""},
		{"forced cpu", loadLog{NoGPU: true}, true, true, -1, false, "--no-gpu"},
		{"gpu", loadLog{Using: "Vulkan1"}, false, true, 1, false, ""},
		{"no gpu found", loadLog{NoGPU: true}, false, true, -1, true, "no GPU found"},
		{"init failed", loadLog{Using: "Vulkan0", Failed: "Vulkan0"}, false, true, -1, true, "failed to initialize Vulkan0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseBackend(tc.log, devs, tc.noGPU, tc.gpuBuild)
			if (got.Device == nil) != (tc.wantDev < 0) || (got.Device != nil && got.Device.Index != tc.wantDev) {
				t.Errorf("device = %+v, want index %d", got.Device, tc.wantDev)
			}
			if got.Warn != tc.wantWarn || got.Note != tc.wantNote {
				t.Errorf("got %+v, want warn=%v note=%q", got, tc.wantWarn, tc.wantNote)
			}
		})
	}
}

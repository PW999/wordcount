package stt

/*
#include <pthread.h>
#include <string.h>

#include "whisper.h"
#include "ggml-backend.h"

// What whisper.cpp reported about backend selection during the last model load.
struct wc_log_rec {
	char using_dev[128]; // device name from "using <name> backend"
	char failed[128];    // device name from "failed to initialize <name> backend"
	int  no_gpu;         // saw "no GPU found"
};

static pthread_mutex_t wc_mu = PTHREAD_MUTEX_INITIALIZER;
static struct wc_log_rec wc_rec;

// copy_between copies the text between the first occurrence of prefix and the
// following suffix into dst, or leaves dst untouched when either is missing.
static void wc_copy_between(char *dst, size_t n, const char *text, const char *prefix, const char *suffix) {
	const char *start = strstr(text, prefix);
	if (!start) return;
	start += strlen(prefix);
	const char *end = strstr(start, suffix);
	if (!end) return;
	size_t len = (size_t)(end - start);
	if (len >= n) len = n - 1;
	memcpy(dst, start, len);
	dst[len] = 0;
}

// wc_log keeps only the backend-selection lines and drops all other output, so
// nothing from whisper.cpp or a GPU driver reaches the terminal.
static void wc_log(enum ggml_log_level level, const char *text, void *user_data) {
	(void)level;
	(void)user_data;
	pthread_mutex_lock(&wc_mu);
	if (strstr(text, "no GPU found")) {
		wc_rec.no_gpu = 1;
	} else if (strstr(text, "failed to initialize") && strstr(text, " backend")) {
		wc_copy_between(wc_rec.failed, sizeof wc_rec.failed, text, "failed to initialize ", " backend");
	} else if (strstr(text, "whisper_backend_init") && strstr(text, ": using ")) {
		wc_copy_between(wc_rec.using_dev, sizeof wc_rec.using_dev, text, ": using ", " backend");
	}
	pthread_mutex_unlock(&wc_mu);
}

static void wc_log_install(void) { whisper_log_set(wc_log, NULL); }

static void wc_log_reset(void) {
	pthread_mutex_lock(&wc_mu);
	memset(&wc_rec, 0, sizeof wc_rec);
	pthread_mutex_unlock(&wc_mu);
}

static void wc_log_snapshot(struct wc_log_rec *out) {
	pthread_mutex_lock(&wc_mu);
	*out = wc_rec;
	pthread_mutex_unlock(&wc_mu);
}

static int wc_is_gpu_type(ggml_backend_dev_t dev) {
	enum ggml_backend_dev_type t = ggml_backend_dev_type(dev);
	return t == GGML_BACKEND_DEVICE_TYPE_GPU || t == GGML_BACKEND_DEVICE_TYPE_IGPU;
}
*/
import "C"

import (
	"fmt"
	"slices"
	"sync"
)

var logOnce sync.Once

// installLog routes whisper.cpp and ggml logging into the backend recorder. It
// must run before the ggml backend registry is first touched, because device
// discovery logs while it initialises.
func installLog() {
	logOnce.Do(func() { C.wc_log_install() })
}

// Device is a GPU that whisper.cpp can run a model on.
type Device struct {
	Index       int    // the value for --gpu-device
	Name        string // backend name, e.g. "Vulkan1"
	Description string // human-readable, e.g. "NVIDIA RTX 500 Ada Generation Laptop GPU"
	Integrated  bool
}

func (d Device) String() string {
	kind := ""
	if d.Integrated {
		kind = ", integrated"
	}
	return fmt.Sprintf("%d: %s (%s%s)", d.Index, d.Description, d.Name, kind)
}

// Devices lists the GPUs in the order and with the index whisper.cpp uses for
// gpu_device: GPU and integrated-GPU devices in backend registry order. A
// CPU-only build returns none.
func Devices() []Device {
	installLog()
	var out []Device
	for i := C.size_t(0); i < C.ggml_backend_dev_count(); i++ {
		dev := C.ggml_backend_dev_get(i)
		if C.wc_is_gpu_type(dev) == 0 {
			continue
		}
		out = append(out, Device{
			Index:       len(out),
			Name:        C.GoString(C.ggml_backend_dev_name(dev)),
			Description: C.GoString(C.ggml_backend_dev_description(dev)),
			Integrated:  C.ggml_backend_dev_type(dev) == C.GGML_BACKEND_DEVICE_TYPE_IGPU,
		})
	}
	return out
}

// BackendNames lists the compute backends this binary was built with, for
// example ["CPU"] or ["Vulkan", "CPU"]. A GPU backend that is compiled in but
// could not start (no driver) is still listed.
func BackendNames() []string {
	installLog()
	var out []string
	for i := C.size_t(0); i < C.ggml_backend_reg_count(); i++ {
		out = append(out, C.GoString(C.ggml_backend_reg_name(C.ggml_backend_reg_get(i))))
	}
	if gpuBackend != "" && !slices.Contains(out, gpuBackend) {
		out = append([]string{gpuBackend}, out...)
	}
	return out
}

// GPUBuild reports whether the binary was built with a GPU backend.
func GPUBuild() bool { return gpuBackend != "" }

// Backend says where the model runs.
type Backend struct {
	Device *Device // nil when running on the CPU
	Note   string  // why the CPU is used in a GPU build, or "" when not a fallback
	Warn   bool    // the CPU is used unintentionally: tell the user
}

func (b Backend) String() string {
	switch {
	case b.Device != nil:
		return fmt.Sprintf("GPU %d: %s (%s)", b.Device.Index, b.Device.Description, b.Device.Name)
	case b.Note != "":
		return "CPU (" + b.Note + ")"
	}
	return "CPU"
}

// loadLog is what whisper.cpp logged about backend selection since resetLoadLog.
type loadLog struct {
	Using  string
	Failed string
	NoGPU  bool
}

func resetLoadLog() { installLog(); C.wc_log_reset() }

func readLoadLog() loadLog {
	var rec C.struct_wc_log_rec
	C.wc_log_snapshot(&rec)
	return loadLog{
		Using:  C.GoString(&rec.using_dev[0]),
		Failed: C.GoString(&rec.failed[0]),
		NoGPU:  rec.no_gpu != 0,
	}
}

// chooseBackend maps the recorded log to the backend report. noGPU is whether
// the GPU was disabled by the caller.
func chooseBackend(l loadLog, devices []Device, noGPU, gpuBuild bool) Backend {
	switch {
	case !gpuBuild:
		return Backend{}
	case noGPU:
		return Backend{Note: "--no-gpu"}
	case l.Failed != "":
		return Backend{Note: "failed to initialize " + l.Failed, Warn: true}
	}
	for i := range devices {
		if l.Using != "" && devices[i].Name == l.Using {
			return Backend{Device: &devices[i]}
		}
	}
	return Backend{Note: "no GPU found", Warn: true}
}

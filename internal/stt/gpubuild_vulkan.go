//go:build vulkan

package stt

// gpuBackend names the GPU backend compiled into whisper.cpp. It is a build
// tag rather than something read from ggml at run time because ggml leaves the
// backend unregistered when no driver is found, which would make a GPU build
// look like a CPU-only one.
const gpuBackend = "Vulkan"

//go:build !vulkan

package stt

// gpuBackend is empty in a CPU-only build; see gpubuild_vulkan.go.
const gpuBackend = ""

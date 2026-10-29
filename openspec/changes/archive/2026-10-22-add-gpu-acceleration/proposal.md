# Proposal

## Why

Whisper decoding runs on the CPU only. With `small` it takes about 2.1 s per 30 s encode at 8 threads, so on a laptop the CPU pegs and the fan ramps up, and `medium` or larger cannot keep up with live use at all. Benchmarks on this machine (RTX 500 Ada, Vulkan) show encode 40x faster than CPU for `base`/`small`, `medium` encoding in about 140 ms using about 1.9 GB of 4 GB VRAM, and clearly lower fan noise under a sustained load. whisper.cpp already supports GPUs; the project just builds it without a GPU backend.

## What Changes

- Add an opt-in GPU build of whisper.cpp (Vulkan) selected at build time; the default build stays CPU-only with no new prerequisites.
- When built with GPU support, run the model on the GPU by default and fall back to the CPU if no usable GPU is found.
- Add `--no-gpu` to force CPU, and `--gpu-device <index>` to choose among several GPUs (this machine has an integrated Intel GPU that enumerates first and is much slower than the NVIDIA card).
- Report which compute backend and device is in use at startup and in `--version`, so a silent CPU fallback is visible.
- Document GPU build prerequisites, usage, measured numbers, and guidance for larger models (`medium`, quantized `large-v3`) in the README.

## Capabilities

### New Capabilities
- `gpu-acceleration`: building with GPU support, selecting and disabling the GPU, falling back to CPU, and reporting the active compute backend.

### Modified Capabilities

None. Transcription, WPM and UI requirements are unchanged; GPU use only affects speed and load.

## Impact

- `Makefile`: GPU build option, separate build directory per backend, extra cgo link flags.
- `cmd/wordcount/main.go`: new flags, backend line in startup output and `--version`.
- `internal/stt`: GPU context creation and device selection; the Go whisper binding hard-codes default context params, so a small patch or wrapper is needed (see design).
- Build prerequisites for GPU builds only: Vulkan headers and loader dev package, `glslc`, `spirv-headers`.
- README: build, usage, and model guidance.
- No change to the offline guarantee: audio still never leaves the machine.

# Tasks

## 1. GPU build option

- [x] 1.1 Add a `GPU` variable to the Makefile (empty or `vulkan`) that configures whisper.cpp with `-DGGML_VULKAN=1` into `build/whisper-vulkan`, leaving the default `build/whisper` untouched; verify `make build` output is unchanged and `make build GPU=vulkan` configures the second directory
- [x] 1.2 Add a prerequisite check for the GPU build (`/usr/include/vulkan/vulkan.h`, `glslc`, SPIRV-Headers) that names what is missing and fails before compiling; verify by running it with a prerequisite hidden from PATH or the include path and checking an existing `wordcount` binary is not overwritten
- [x] 1.3 Extend `CGO_LDFLAGS` for the GPU build with the Vulkan library path and `-lggml-vulkan -lvulkan` (wrapped in `-Wl,--start-group`/`--end-group` if static link order requires it); verify `make build GPU=vulkan` links and `./wordcount --version` runs
- [x] 1.4 Verify switching is cheap: build GPU, then CPU, then GPU again and check that neither rebuild recompiles whisper.cpp
- [x] 1.5 Document the GPU prerequisites (Fedora package names) and `make build GPU=vulkan` in the README Build section, and verify the commands run as written on a clean checkout

## 2. GPU parameters in the Go binding

- [x] 2.1 Write `patches/` patch for `bindings/go` adding a constructor that takes `use_gpu` and `gpu_device`, with the existing `New` keeping current behaviour; verify the patch applies to the pinned `WHISPER_TAG` and that `go build ./...` passes
- [x] 2.2 Make the Makefile apply the patch once after cloning (skip when already applied, fail with a clear message when it does not apply); verify with a fresh `third_party/` and with a second `make build`
- [x] 2.3 Add `UseGPU` and `GPUDevice` to `stt.WhisperConfig` and pass them through `NewWhisper`; verify with a unit test that the defaults keep the current behaviour and the whisper tests still pass (`make test`)

## 3. Backend report and device list

- [x] 3.1 Add the cgo shim in `internal/stt` that installs a `whisper_log_set` callback recording the backend-selection lines and dropping other output; verify with a test that a CPU-only build records "no GPU found" and no log text reaches stderr during model load
- [x] 3.2 Add device enumeration (GPU and iGPU devices in registry order, with the same index rule as whisper.cpp) and an accessor for the active backend; verify with a test that a CPU build returns an empty device list, and manually that the GPU build lists Arc as 0 and the NVIDIA card as 1 on the development machine
- [x] 3.3 Print the active backend/device (or "CPU" with the fallback reason) after the model loads and before the UI opens, and include the backend list and devices in `--version`; verify the output on CPU-only, GPU, and forced-CPU runs
- [x] 3.4 Remove or confirm no longer needed any stderr noise from GPU init in the plain-terminal phase; verify `./wordcount --model models/ggml-base.bin --file testdata/jfk.wav` shows only the loading line and the backend line before the UI opens

## 4. Flags

- [x] 4.1 Add `--no-gpu` and `--gpu-device <index>` to `main.go`, rejecting the combination and out-of-range indices (naming the valid indices) before the model loads; verify with table tests in `main_test.go` for the valid, invalid and combined cases
- [x] 4.2 Handle CPU-only builds: `--no-gpu` is a no-op, `--gpu-device` exits non-zero saying no GPU is available; verify with tests and a manual run of the default build
- [x] 4.3 Add the flags to the README Usage section and verify the examples run

## 5. Behaviour verification and docs

- [x] 5.1 Run the same file (`testdata/jfk.wav` and a longer file) on CPU and on GPU with `base`, and verify total words differ by at most 2% (the "same file on both backends" scenario)
- [x] 5.2 Verify CPU fallback: run the GPU build with `--no-gpu`, and with the GPU hidden (for example with an invalid Vulkan ICD environment), and check the warning and that transcription still works
- [x] 5.3 Verify the 4 GB VRAM guidance by running `medium` and `large-v3-q5_0` with `--file` on the NVIDIA card, noting memory use from `nvidia-smi` and whether live partials keep up
- [x] 5.4 Add a README section with measured encode/decode times (base, small, medium on CPU, Arc, NVIDIA from the benchmark), the device-index note, and model guidance for 4 GB cards; verify the numbers match the benchmark runs
- [x] 5.5 Run `make test` for both the CPU and the GPU build and verify they pass

## Workflow follow-up

- Archive the change after review.
- Verify the archived result and that `openspec/specs/gpu-acceleration/spec.md` exists.

# Design

## Context

See proposal.md for motivation. Current state:

- whisper.cpp v1.9.5 is built by the Makefile with no GPU backend (`GGML_OPENMP=OFF`, static libs in `build/whisper`).
- The Go binding (`third_party/whisper.cpp/bindings/go`, wired by a `replace` in `go.mod`) creates models through `whisper_init_from_file_with_params(path, whisper_context_default_params())`. `use_gpu` therefore defaults to true but `gpu_device` is fixed at 0, and nothing can be changed from Go. `third_party/` is a gitignored clone of a pinned tag.
- The binding's `#cgo LDFLAGS` list only `-lwhisper -lggml -lggml-base -lggml-cpu`; the Makefile already extends linking through `CGO_CFLAGS`/`CGO_LDFLAGS`.
- whisper.cpp picks the `gpu_device`-th device of type GPU or iGPU in ggml's registry order and falls back to CPU silently, only logging to stderr ("no GPU found", "failed to initialize ... backend").
- On the target machine Vulkan device 0 is the integrated Intel Arc (2-3x faster than CPU) and device 1 is the NVIDIA RTX 500 (about 40x faster on encode). A default of "device 0" would pick the wrong GPU.
- `main.go` silences stderr only while the TUI runs, so backend logs printed during model load reach the plain terminal.

## Goals / Non-Goals

**Goals:**
- GPU execution with Vulkan on Linux, selectable at build time, CPU build untouched.
- Make the active backend and device visible, and make the choice of device controllable.
- Keep the change small and local to the build, `internal/stt` and flag handling.

**Non-Goals:**
- CUDA, ROCm, OpenVINO, SYCL or Metal builds. They can be added later behind the same build option.
- Running different parts of the pipeline (VAD, resampling) on the GPU.
- Automatic benchmarking or automatic choice of the fastest device.
- Changing decoding parameters, partial refresh rate or threading strategy.

## Decisions

### Vulkan, not CUDA
Vulkan needs only dev packages (`vulkan-headers`, `vulkan-loader-devel`, `glslc`, `spirv-headers-devel` on Fedora) and a loader that is already present; CUDA needs the full toolkit and adds dynamic CUDA runtime dependencies to a project that today needs only cmake, g++, make, git and Go. Measured on the RTX 500, Vulkan already gives a 40x encode speedup and clearly quieter operation, which meets the goal. Alternative: CUDA would likely be faster still, but the extra weight is unjustified until Vulkan is shown to be too slow.

### Opt-in build with a separate build directory
`make build GPU=vulkan` configures whisper.cpp with `-DGGML_VULKAN=1` into `build/whisper-vulkan`; the default keeps `build/whisper`. The Makefile adds the matching library path and `-lggml-vulkan -lvulkan` to `CGO_LDFLAGS` only for the GPU build, and checks for the prerequisites first (header, `glslc`, spirv headers) with a message naming what is missing. Separate directories avoid a clean rebuild when switching. Alternative: GPU on by default, rejected because it makes every build depend on GPU SDK packages and breaks machines without them.

### Pass GPU parameters through a patch to the Go binding
`use_gpu` and `gpu_device` exist only in `whisper_context_params`, which the binding hard-codes. The Makefile applies a small patch file kept in the repo (`patches/`) to the cloned binding right after `git clone`, adding a constructor that takes those two values; `internal/stt` uses it. Alternatives:
- Environment variable (`GGML_VK_VISIBLE_DEVICES`) for device choice: works without a patch, but is Vulkan-specific and cannot express `--no-gpu` cleanly.
- Vendor the whole binding into the repo: a large copy to maintain for two fields.
- Replace the binding with our own cgo layer: far too much surface (segments, language, params).

The patch is the smallest change that keeps the binding upstream. It is checked by the Makefile (apply only when not already applied) and pinned together with `WHISPER_TAG`.

### A small cgo shim in `internal/stt` for visibility
A shim calls `whisper_log_set` with a C callback that records the backend-selection lines (`using ... backend`, `no GPU found`, `failed to initialize`) and drops everything else, and enumerates devices with `ggml_backend_dev_*` applying the same counting rule as whisper (GPU and iGPU types, in registry order), so the index we print is the index `--gpu-device` expects. This yields:
- A reliable "GPU: NVIDIA RTX 500 Ada" or "CPU (fallback: ...)" line, instead of guessing.
- No driver log noise on the terminal during model load, which also covers the "kept off the UI" requirement.
- The device list for `--version` and for validating `--gpu-device` before loading the model.

Alternative: parse stderr by redirecting fd 2, as `silenceStderr` does; more fragile than a log callback.

### Flags and behaviour
- `--no-gpu` forces `use_gpu=false`; `--gpu-device N` sets `gpu_device`; both together is an error, checked before model load, as is an out-of-range index.
- Default device: index 0 as whisper does, so the report is the safety net. Alternative: prefer discrete over integrated GPUs by default, which would pick the NVIDIA card here automatically. It is a better default on this machine but encodes a policy about device types and registry order that we have not verified on other machines, so it is left for a later change.
- A CPU-only binary accepts `--no-gpu` as a no-op and rejects nothing; `--gpu-device` on a CPU-only binary reports that no GPU is available and exits non-zero.

### Documentation
README gains GPU build prerequisites, the flags, the measured numbers (base, small, medium on CPU, Arc and NVIDIA), and a note that 4 GB of VRAM fits `medium` and a quantized `large-v3` but not fp16 `large-v3`.

## Risks / Trade-offs

- [Static link order or missing symbols when adding `-lggml-vulkan -lvulkan` to the binding's flags] → Spike it first (build `wordcount` against a Vulkan whisper); if order is a problem, wrap the libraries in `-Wl,--start-group ... --end-group` in `CGO_LDFLAGS`.
- [The binding patch breaks when `WHISPER_TAG` is bumped] → Patch is small and applied with a clear failure message; a Makefile check fails the build rather than silently building without GPU parameters.
- [`whisper_log_set` is process-global and replaces logging for the whole program] → Acceptable, since the app has no other whisper logging need; the shim keeps the last relevant lines for the banner and error messages.
- [Device indices differ between ggml's enumeration and Vulkan's own tooling] → The app prints its own index list and uses it consistently; documented in the README.
- [Fallback to CPU hides a broken GPU setup] → Fallback always prints a warning with the recorded reason.
- [GPU speed and noise gains depend on the machine] → The README states measured numbers for this machine as an example only; no performance requirement is added to the spec.
- [Integrated GPU shares system RAM, so "does not fit" may not be detected cleanly there] → Left to whisper.cpp's own failure path, which is reported as a fallback when it fails to initialise.

## Migration Plan

Nothing changes for existing users: `make build` still produces the CPU binary. GPU users run `make build GPU=vulkan` once the prerequisites are installed; going back is `make build`. Rollback of the change is removing the option, the patch and the flags.

## Open Questions

- Whether a later change should default to the discrete GPU when one is present.
- Whether `--version` should also list CPU features, as it does today, next to the device list (the current output is the whisper.cpp system-info string).

# Design

## Context

Greenfield repository (no Go module yet). See proposal.md for motivation. Observed on the dev machine: Go 1.26, gcc/g++ and make present; **cmake is missing** (needed to build whisper.cpp), cargo is missing, ALSA dev headers are missing, PipeWire/PulseAudio tooling (`pactl`) is present and one ALSA capture card exists. These facts drove the stack choice. Target is Linux x86_64 (arm64 best-effort).

## Goals / Non-Goals

**Goals:**
- One executable the user can copy and run; only a model file is external.
- Live feel: partial text refreshing roughly every second on a normal laptop CPU with a `base`/`small` model.
- Pure-Go-side simplicity: small internal packages that can be unit-tested without a microphone or model.

**Non-Goals:**
- GPU acceleration, non-Linux support, persistence of transcripts, true token-level streaming.
- Exact word timing: WPM only needs "when was this word detected", not when it was spoken.

## Decisions

**D1. Go + whisper.cpp via cgo.** Whisper gives good multilingual accuracy offline. whisper.cpp's official Go bindings are built against `libwhisper.a`/`libggml*.a`, so whisper and ggml are linked into the binary statically. Fully static linking (`-extldflags -static`) is not possible together with the dynamic audio backends, and the bindings hard-code `-lstdc++` and `-fopenmp` in their cgo flags, so the binary also depends dynamically on the common system runtime libraries libstdc++, libgcc_s and libgomp besides glibc. This was accepted as a trade-off: they are present on practically every Linux system that has a C++ runtime, and it avoids patching the bindings. *Alternatives:* Vosk (true streaming, but needs `libvosk.so` beside the binary, violating "single deliverable"); Rust (`whisper-rs`/`cpal`: cargo and ALSA headers absent, longer setup); a fat JAR with a JNI whisper (heavier, awkward natives packaging).

**D2. Audio via `malgo` (miniaudio).** Miniaudio is vendored as a single C header inside the module and `dlopen`s PulseAudio/PipeWire-pulse/ALSA at runtime, so no `libasound2-dev` is required to build and the binary does not hard-link an audio lib. It provides device enumeration (names) and resampling/channel conversion, so we request mono, 16 kHz, float32 and let miniaudio convert. *Alternative:* `gordonklaus/portaudio` (needs libportaudio at runtime and headers at build time), raw ALSA via cgo (needs headers).

**D3. Segmentation: energy VAD + growing-buffer re-decode.** An utterance buffer accumulates samples. A simple RMS/hysteresis voice-activity detector marks speech start/end. While speech is active, every ~1 s the whole current utterance buffer is re-transcribed and shown as the *partial*. When silence ≥ 700 ms follows speech (or the buffer hits 15 s) the utterance is transcribed once more and *committed* as final. Silence-only buffers are never sent to the model (also avoids whisper hallucinations on silence). Re-decoding the same buffer is wasteful but trivially correct and bounded to 15 s of audio. *Alternatives:* fixed sliding windows with overlap stitching (duplicate/dropped words at joins); Silero VAD (extra model/dependency, not needed for a first version). A small hold-over of ~300 ms of audio before detected speech start prevents clipped first words.

**D4. Inference runs on its own goroutine with drop-oldest backpressure.** The audio callback only copies samples into a channel (never blocks); a decoder goroutine consumes them. If decoding lags, partial re-decodes are skipped (never final ones), so the UI stays responsive. Threads for whisper default to `min(NumCPU, 8)` and are configurable via `--threads`.

**D5. WPM = words committed-or-partial in a trailing 60 s window.** Each time an utterance's text changes, the delta in word count is stamped with `time.Now()` into a timestamp ring (words, not sentences). For a partial that is later revised the stamps for the previous partial are replaced, so revisions do not inflate the count. `wpm = words_in_window / max(5s, min(60s, elapsed)) * 60`, recomputed on a 1 s tick so it decays during pauses. Words are whitespace-separated tokens containing at least one letter or digit; annotations like `[BLANK_AUDIO]`, `(music)`, `*noise*` are stripped first. This logic is a pure package with an injectable clock for tests.

**D6. UI with Bubble Tea + Lip Gloss.** An Elm-style model fits "async events -> render": messages for audio level, partial text, final text and tick. Big digits are a small built-in 5-row glyph table (no figlet dependency). The device picker is a Bubble Tea list shown first in the same program, so there is one terminal lifecycle. Non-interactive stdout (not a TTY) is rejected with a clear message except for `--list-devices`.

**D7. Model is user supplied (`--model`).** No embedding and no download logic; keeps the binary small and avoids network code. Language via `--lang` (default `auto`); English-only models (`.en` in file name, or `IsMultilingual()` false) force `en` with a warning.

**D8. Layout.**
```
go.mod
cmd/wordcount/main.go          flag parsing, wiring
internal/audio/                device list, capture (malgo), level meter
internal/stt/                  whisper wrapper, VAD/segmenter, text cleaning
internal/wpm/                  rolling window calculator
internal/ui/                   Bubble Tea models, big digits, picker
Makefile                       builds whisper.cpp static libs + the binary
```
Interfaces (`audio.Source`, `stt.Transcriber`) let the segmenter/UI be tested with fakes.

## Risks / Trade-offs

- [cmake not installed on the dev machine] → Makefile target documents and checks for `cmake`; task 1 installs it (`dnf install cmake`) or fails with a clear message.
- [cgo build is slow and whisper.cpp API/bindings change] → pin a whisper.cpp release tag as a git submodule/fetched at a fixed commit by the Makefile; build libs once into `build/`.
- [CPU too slow for live partials with larger models] → partials are skippable (D4); document recommended models (`base`, `small`) and `--threads`; the final text still appears.
- [Whisper hallucinates on noise/silence] → VAD gate plus annotation filtering plus dropping known filler phrases when the buffer's RMS is very low.
- [Partial revisions make the live count jitter] → replace-not-add semantics (D5); the 60 s window smooths the displayed value.
- [Binary depends on libstdc++/libgcc_s/libgomp at runtime] → accepted; whisper/ggml are static, audio libs are dlopen'd, and `ldd` is checked in the final integration task to confirm nothing else (no whisper/ggml/audio libs) is linked.
- [Mic permissions / PipeWire absent] → miniaudio falls back to ALSA; failures surface as a clear exit error (see audio-input spec).

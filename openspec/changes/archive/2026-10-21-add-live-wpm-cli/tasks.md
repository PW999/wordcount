# Tasks

## 1. Project scaffolding and build

- [x] 1.1 Install/verify build prerequisites (`cmake`, `g++`, `make`, `git`) and add a `make check-deps` target; verify it passes on this machine and prints a clear message when cmake is missing
- [x] 1.2 Create `go.mod` and the `cmd/wordcount` + `internal/{audio,stt,wpm,ui}` skeleton with a `main` that prints `--version`; verify `go build ./...` succeeds
- [x] 1.3 Add a Makefile target that fetches whisper.cpp at a pinned tag and builds its static libs into `build/`, and a `make build` target producing `./wordcount` linked against them; verify `./wordcount --version` runs
- [x] 1.4 Add a README with build instructions, model download hint (e.g. `ggml-base.bin`) and usage; verify the documented commands work as written on a clean checkout

## 2. WPM calculation (wpm-display)

- [x] 2.1 Implement `internal/wpm` rolling-window calculator with injectable clock and replace-not-add handling of revised partials; verify unit tests cover steady 150 wpm, decay to 0 after 60 s silence, the 5 s startup floor and partial revision
- [x] 2.2 Implement text cleaning/word counting (strip `[..]`, `(..)`, `*..*` annotations, count tokens with a letter or digit); verify table-driven tests including Dutch/accented words and annotation-only input

## 3. Audio input (audio-input)

- [x] 3.1 Implement device enumeration with index+name in `internal/audio` and the `--list-devices` flag; verify output format `<index>: <name>` on this machine and the "no microphone" error path using a fake backend in a unit test
- [x] 3.2 Implement device resolution for `--device` (index or unique name substring) with ambiguity/no-match errors; verify unit tests for index, substring, none and multiple matches
- [x] 3.3 Implement capture via malgo at mono/16 kHz/float32 delivering chunks through a non-blocking channel plus an RMS level; verify with a short manual run that levels respond to speech and that a device error surfaces as a returned error

## 4. Speech transcription (speech-transcription)

- [x] 4.1 Implement the whisper wrapper (`stt.Transcriber`) with `--model`, `--lang`, `--threads`, English-only-model detection and the missing/invalid model error; verify against a real `ggml-base.bin` that a sample WAV in `testdata/` transcribes and that a bad path exits non-zero before audio opens
- [x] 4.2 Implement the energy-VAD segmenter (pre-roll, 0.7 s endpoint, 15 s cap, silence never sent to the model) behind a fake `Transcriber`; verify unit tests for utterance end, long monologue split and silence-only input producing no calls
- [x] 4.3 Implement the decoder goroutine with ~1 s partial refresh, drop-oldest backpressure for partials and always-processed finals; verify with a slow fake transcriber that finals are never dropped and the audio callback never blocks
- [x] 4.4 Filter annotation tokens and hallucination-on-silence output from transcriber results; verify unit tests with `[BLANK_AUDIO]`, `(music)` and empty results

## 5. Terminal UI (wpm-display, audio-input)

- [x] 5.1 Implement big-digit rendering (5 rows) and single-line fallback for small terminals; verify golden-string tests for digits 0-9 and the fallback below 80x24
- [x] 5.2 Implement the device picker (arrow keys, Enter, pre-highlight for a single device) in Bubble Tea; verify with Bubble Tea model unit tests driving key messages and a manual check
- [x] 5.3 Implement the main view: large WPM, total words, elapsed time, input level bar, scrolling transcript with distinct partial line, 1 s tick; verify model tests for partial-then-final replacement and overflow scrolling
- [x] 5.4 Implement quit handling (`q`/Esc/Ctrl+C), terminal restore and the one-line summary (total words, duration, average WPM); verify via test of the quit message path and a manual run

## 6. Wiring and integration

- [x] 6.1 Wire flags, picker, capture, segmenter/decoder, WPM and UI in `cmd/wordcount/main.go`, with the non-TTY rejection; verify `./wordcount --model <m>` end to end by speaking and reading text and a plausible WPM
- [x] 6.2 Verify the deliverable: `ldd ./wordcount` shows only system runtime libraries (glibc, libstdc++, libgcc_s, libgomp; no whisper/ggml/audio libs), and the binary runs when copied to another directory with only a model file alongside
- [x] 6.3 Verify accuracy by playing a 150 wpm reference recording (or reading a fixed text with a timer) and confirming the displayed WPM is within 15%; record the result in the README

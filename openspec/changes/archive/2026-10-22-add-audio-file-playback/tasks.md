# Tasks

## 1. Decoding and resampling

- [x] 1.1 Add `github.com/go-audio/wav` and `github.com/hajimehoshi/go-mp3` to `go.mod`; verify `go build ./...` (via `make build`) still succeeds
- [x] 1.2 Implement `internal/audio/decode.go` (content sniffing, WAV and MP3 to float32 PCM with native rate/channels, errors for missing, unsupported, empty and over-length files) with table tests using generated WAV bytes and `testdata/jfk.wav`; verify `go test ./internal/audio` passes
- [x] 1.3 Add a short MP3 fixture to `testdata/` (re-encoded from `jfk.wav`, e.g. with `ffmpeg` or `lame`, noted in a testdata README line) and a test that decodes it with the expected duration (+-50 ms) even when named `.wav`; verify the test passes
- [x] 1.4 Implement `internal/audio/resample.go` (downmix to mono, resample to 16 kHz) with tests for 44.1 kHz stereo, 48 kHz mono and 16 kHz passthrough checking output length and a sine tone's RMS; verify tests pass

## 2. Playback source

- [x] 2.1 Define `audio.Stream` (`Chunks`, `Level`, `Err`, `Close`) and verify `*audio.Capture` still satisfies it via a compile-time assertion
- [x] 2.2 Implement `FilePlayer` in `internal/audio/player.go`: playback device at native rate/channels, callback copies output, advances position, emits the matching 16 kHz chunks and RMS level, signals completion; verify with a fake-output unit test that driving the callback by hand yields chunks totalling the file length and no chunk ahead of the played position
- [x] 2.3 Return a clear error from the player constructor when no playback device exists; verify via a test using a backend fake or, if the backend cannot be faked, a documented manual check
- [x] 2.4 Add a manual test (like `capture_manual_test.go`, behind its existing guard) that plays `testdata/jfk.wav` and reports completion; verify it runs on this machine and audio is heard

## 3. End-of-stream handling

- [x] 3.1 Add `Segmenter.Flush()` that finalises an in-progress utterance with enough speech frames and emits nothing for silence; verify with tests for mid-utterance end, trailing silence and silence-only input
- [x] 3.2 Wire completion in the session so that after the last chunk the segmenter is flushed, the decoder drains, and a `Done` signal is raised only after the last final update was delivered; verify with a test using a fake `Transcriber` that the last utterance's words reach the UI before `Done`

## 4. UI

- [x] 4.1 Extend `ui.Source` with `Done` and `Label`, add `Config.AutoStart`, show "finished" and freeze elapsed time/WPM on `Done`, and use the label instead of the hard-coded "mic"; verify with `app_test.go` cases (clock stops advancing after Done, summary uses the frozen duration, label is "audio" in file mode, mic mode unchanged)
- [x] 4.2 Verify quitting during playback still prints the summary and stops the pipeline (extend an existing quit test); run `go test -race ./internal/ui`

## 5. CLI wiring and docs

- [x] 5.1 Add `--file` to `cmd/wordcount/main.go`: reject `--file` with `--device` before the model loads, skip `ListDevices` and the picker in file mode, decode the file and open the output device before the alt screen, and start the file session; verify `./wordcount --file x --device 1` and a missing/invalid file each print the specified error and exit non-zero, and `--list-devices` is unchanged
- [x] 5.2 Update the README usage section with `--file`, supported formats and the end-of-file behaviour; verify the documented command runs as written
- [x] 5.3 Integration check: run `make test`, then `./wordcount --model models/ggml-base.bin --file testdata/jfk.wav` and the MP3 fixture; verify audio is audible, the word count grows during playback, the last words are included at the end, "finished" is shown, and the summary line is printed on `q`
- [x] 5.4 Re-run the README accuracy check using `--file` on a 60 s+ file with a known word count and record the result next to the existing table; verify the displayed total is within 15% of the true count

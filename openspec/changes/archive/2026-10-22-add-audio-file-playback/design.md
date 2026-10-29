# Design

## Context

The pipeline today is `audio.Capture` (malgo capture device, mono 16 kHz float32 chunks, RMS level, error channel) -> `stt.Segmenter` (energy VAD, emits partial/final `Request`s) -> `stt.Decoder` -> `ui.App` (`wpm.Calculator` driven by a wall clock). `main.go`'s `session.start` wires this up for a chosen `audio.Device`, and `ui.Source` (`Level`, `Updates`, `Errs`) is the only thing the UI knows about the audio side. See proposal.md for motivation.

Constraints observed in the repo:
- malgo is built with miniaudio's decoders compiled out (`-DMA_NO_DECODING -DMA_NO_WAV -DMA_NO_MP3`) and its Go API exposes no decoder, so WAV/MP3 must be decoded in Go. malgo does provide playback devices.
- `audio.NewMalgoBackend` and `audio.ListDevices` are called unconditionally in `main.go`; `ListDevices` errors when no microphone exists, which must not block file mode.
- The calculator measures time with an injectable clock; the UI already takes a `Clock`.

## Goals / Non-Goals

**Goals:**
- Reuse the existing segmenter, decoder, calculator and UI unchanged in behaviour; file mode only supplies a different audio source.
- Counting follows what has been *played*, so the numbers the user sees match what they hear.
- Pure decode/resample logic is unit-testable without audio hardware or a model.

**Non-Goals:**
- Pause, seek, volume, output device selection, other formats, headless/non-TTY output.
- Faster-than-realtime transcription of a file (a possible later `--no-play` mode).

## Decisions

**D1. Decode in Go with pure-Go libraries.** WAV via `github.com/go-audio/wav`, MP3 via `github.com/hajimehoshi/go-mp3`. The whole file is decoded up front into float32 PCM (native sample rate, interleaved channels), which gives the exact duration for validation and a simple seekless player. For MP3 the encoder's start delay and end padding (recorded in the Xing/LAME header, plus the silent Info frame itself) are trimmed so the duration matches the audio to within the +-50 ms the tests require; MP3s without such a header are kept as decoded. Format is chosen by sniffing the first bytes (`RIFF....WAVE` vs `ID3`/MPEG frame sync), not by extension. *Alternatives:* building malgo with the `ma_full` tag (decoders are compiled in but unreachable from the Go API without writing our own cgo); shelling out to `ffmpeg` (external runtime dependency, breaks the "one executable plus a model" goal). Memory cost: 10 minutes of 44.1 kHz stereo float32 is about 211 MB, which is accepted for now (see Risks).

**D2. Two derived streams from one decode.** From the decoded PCM we keep (a) the native-rate interleaved samples for playback and (b) a mono 16 kHz copy for the segmenter, produced once by averaging channels and linear-resampling (the speech band is far below 8 kHz; for the 44.1/48 kHz inputs expected, a short box/low-pass before decimation avoids aliasing). Doing it once up front keeps the audio callback allocation-free.

**D3. Playback drives analysis.** A `FilePlayer` opens a malgo playback device at the file's native rate and channel count (miniaudio converts to the device). In the data callback it copies the next frames to the output and advances a played-frame counter. The 16 kHz mono slice corresponding to the frames just handed to the device is sent on a `Chunks()` channel with the same non-blocking, drop-counting send as `Capture`. Because analysis is clocked by the playback callback, the word count cannot run ahead of what has been heard, and the session's audio -> segmenter -> decoder goroutine works for both sources through a small `audio.Stream` interface (`Chunks()`, `Level()`, `Err()`, `Close()`) that `Capture` already satisfies. A short MP3 padding trim (see D1) keeps the decoded length equal to the audio's own length.

**D4. End of stream.** When the callback runs out of samples it outputs silence, enqueues the remaining chunk, then closes the `Chunks()` channel (a live `Capture` never closes it). The session's audio goroutine treats a closed channel as end of stream: it calls `Segmenter.Flush()`, which emits a final `Request` for any in-progress utterance that has enough speech frames, and passes it to the decoder. The session counts final requests submitted against final updates delivered and closes the new `Source.Done` channel only when the stream has ended and no final is outstanding, so the last words are already in the update queue before `Done`. The UI waits on updates and `Done` in a single command, which hands over any updates still queued when `Done` fired, so none is applied after the freeze. On completion the UI freezes its clock (D5), shows "finished" and waits for the quit key.

**D5. Frozen clock for the finished state.** `wpm.Calculator` already reads time through an injected function. On completion the UI records the instant and uses it as the clock for `Elapsed()`, `WPM()` and the summary, so numbers stop moving without changing the calculator. In file mode the elapsed time is wall time since the player started, which equals playback time because the player starts at the same moment as the calculator; no seek or pause exists to break that. The clock freezes at the file's length (`Source.Length`), not at the instant `Done` arrives: on a slow CPU the decoder can finish seconds after playback ended, and that lag must not count as listening time or lower the summary's WPM.

**D6. Flags and start-up order.** `prepareFile` in `main.go` runs first, before the terminal check and before the model loads: `--file` and `--device` conflict -> error, then the file is decoded, so a missing, unsupported or empty file fails immediately instead of after the model load. File mode skips `ListDevices`/picker entirely (the capture device list is not touched, so a machine without a microphone works). Playback-device availability (`RequireOutput`) is checked after the model loads but before the alt screen opens, so that error also prints normally on a plain terminal. The playback device itself is created in the starter, which receives the already-decoded audio.

**D7. UI changes are minimal.** `Source` gains `Done <-chan struct{}`, a `Label string` ("mic" default, "audio" in file mode) and a `Length time.Duration` (the file's playing time, zero for live capture; see D5). `Config.Preselected`/picker logic is bypassed with a new `Config.AutoStart` flag that fires `DeviceChosenMsg` with a zero device, mirroring how preselection works today. *Alternative:* a separate UI model for files; rejected because 90% of the view is shared.

**D8. Layout.**
```
internal/audio/decode.go       sniff + WAV/MP3 decode -> PCM
internal/audio/resample.go     downmix + resample to 16 kHz mono
internal/audio/player.go       FilePlayer on malgo playback device; implements Stream
internal/stt/segmenter.go      + Flush()
internal/ui/app.go             + Done handling, frozen clock, label
internal/audio/stream.go       Stream interface
cmd/wordcount/main.go          --file flag, prepareFile
cmd/wordcount/session.go       session (pipeline + completion tracking), file starter
```

## Risks / Trade-offs

- [Whole file decoded into memory] -> acceptable for speech-length files; the decode step reports an error above a sane cap (about 1 hour of audio) rather than exhausting memory. Streaming decode is a later improvement.
- [WAV libraries do not read 32-bit float or compressed WAV] -> report "unsupported WAV encoding" with the encoding named; 8/16/24/32-bit PCM is supported.
- [Transcription slower than real time on weak CPUs] -> partials are already droppable, but finals queue; the count would lag behind playback and catch up after the end. Mitigated by the existing model advice in the README; the end-of-file state waits for the decoder queue to drain before "finished".
- [Playback device latency makes the heard audio slightly later than analysis] -> the offset is the device buffer (tens of ms), far below the 0.7 s segmentation granularity; ignored.
- [Linear resampling is lower quality than a polyphase filter] -> only used for speech recognition at 16 kHz; checked by transcribing `testdata/jfk.wav` and a re-encoded MP3 of it.
- [Accuracy of MP3 decoding differs from WAV] -> tasks include a test comparing the word count of the same speech in both formats.

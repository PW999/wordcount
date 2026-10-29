# Proposal

## Why

wordcount only listens to a live microphone, so the only way to check it or to measure an existing recording is to play the recording through speakers and let a monitor device capture it (the README's accuracy check does exactly that). Letting the user hand the application a WAV or MP3 file removes that detour: the file is played and the same live word count is shown while it plays.

## What Changes

- New `--file <path>` option that accepts a WAV or MP3 file instead of a capture device.
- The file is decoded, played through the default output device, and the same audio is transcribed in step with playback, so the word count, WPM and transcript advance as the listener hears the words.
- The device picker is skipped in file mode; `--file` and `--device` are mutually exclusive.
- When the file ends, the final utterance is flushed and counted, the display freezes with a "finished" indication, and the app waits for `q`/Esc/Ctrl+C. The exit summary reports the whole file.
- Unreadable, unsupported or empty files, and a missing output device, are reported as errors before the UI starts.
- The UI level label reads "audio" instead of "mic" in file mode.

## Capabilities

### New Capabilities
- `audio-file-playback`: accepting a WAV/MP3 file, playing it back, feeding it to transcription in step with playback, and the end-of-file behaviour.

### Modified Capabilities
- `audio-input`: "Interactive device selection" gains the case where a file is given and no capture device is chosen or opened.

## Impact

- `cmd/wordcount/main.go`: new flag, validation, and a second session path that starts a file player instead of a capture.
- New `internal/audio` code for decoding (WAV, MP3), downmix/resample to 16 kHz mono, and a playback device via the existing malgo backend.
- `internal/stt/segmenter.go`: an end-of-stream flush so the last utterance is finalised.
- `internal/ui`: end-of-playback state, frozen clock, label change.
- New Go dependencies for decoding (WAV and MP3 are not available through malgo's build, which compiles miniaudio's decoders out).
- README: usage section for `--file`.
- Out of scope: pause/seek, other formats (FLAC, OGG, M4A), volume control, selecting the output device, non-interactive (headless) output.

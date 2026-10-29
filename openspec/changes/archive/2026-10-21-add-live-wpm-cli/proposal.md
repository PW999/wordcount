# Proposal

## Why

There is no simple way to see how fast you are speaking while you speak. Presenters, trainers and podcasters want a live words-per-minute figure and a running transcript without uploading audio to a cloud service. A local, offline Linux CLI that listens to a microphone covers this with no accounts or network.

## What Changes

- New Go command-line application, `wordcount`, shipped as a single executable (whisper.cpp compiled in; it relies only on common system runtime libraries: glibc, libstdc++, libgcc_s, libgomp, and the system audio stack).
- On start the user selects a capture device from a list of available microphones (interactive picker, or `--device` to skip it; `--list-devices` prints and exits).
- Captured audio is transcribed locally, in near real time, by a whisper model supplied by the user via `--model <path>`. A multilingual model is supported and the spoken language is selectable via `--lang` (default `auto`).
- A full-screen terminal UI shows a large, prominent words-per-minute count and a live feed of the detected text (in-progress text updates in place, finished utterances scroll up).
- The speech model is NOT embedded or downloaded; the binary stays small and the user manages model files.

Out of scope: non-Linux platforms, GUI, saving transcripts/audio, speaker diarisation, cloud STT.

## Capabilities

### New Capabilities
- `audio-input`: Enumerating capture devices, selecting one (interactively or by flag) and streaming mono 16 kHz audio from it.
- `speech-transcription`: Turning the audio stream into live partial and final text with a user-supplied local model, including language selection and filtering of non-speech output.
- `wpm-display`: Computing a rolling words-per-minute value from detected words and presenting it, together with the live transcript, in the terminal.

### Modified Capabilities

(none, greenfield project)

## Impact

- New Go module in the repository root (`cmd/`/`internal/` layout), no existing code affected.
- Build dependencies: Go toolchain, gcc/g++, make/cmake (to build whisper.cpp's static libs once), git.
- Go dependencies: whisper.cpp Go bindings, `malgo` (miniaudio, loads PulseAudio/PipeWire/ALSA at runtime, so no ALSA dev headers are required), a terminal UI library (Bubble Tea + Lip Gloss).
- Runtime requirement: a user-provided `ggml-*.bin` whisper model (multilingual e.g. `ggml-base.bin` or `ggml-small.bin`).

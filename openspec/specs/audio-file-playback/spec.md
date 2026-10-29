# audio-file-playback Specification

## Purpose

Lets the user analyse a pre-recorded WAV or MP3 file instead of a live microphone: the file is played back and the live word count, WPM and transcript follow the audio as it is heard.

## Requirements

### Requirement: Audio file input
The system SHALL accept a WAV or MP3 file with `--file <path>` and use it as the audio source instead of a capture device. The format SHALL be recognised from the file content, not its extension.

#### Scenario: WAV file
- **WHEN** the user runs the application with `--model <path> --file speech.wav` and the file is a valid PCM WAV file
- **THEN** the system skips the device picker, opens no capture device and starts playing and analysing the file

#### Scenario: MP3 file
- **WHEN** the user passes a valid MP3 file with `--file`
- **THEN** the system plays and analyses it the same way as a WAV file

#### Scenario: Extension does not match content
- **WHEN** the user passes a valid MP3 file named `speech.wav`
- **THEN** the system plays it as an MP3

#### Scenario: File and device both given
- **WHEN** the user passes both `--file` and `--device`
- **THEN** the system prints an error saying the options are mutually exclusive and exits with a non-zero status before loading the model

### Requirement: File validation before start
The system SHALL verify that the file can be opened and decoded, and that an output device is available, before the terminal UI starts, and SHALL exit with a non-zero status and a message naming the problem otherwise.

#### Scenario: Missing or unreadable file
- **WHEN** the path given to `--file` does not exist or cannot be read
- **THEN** the system prints an error naming the path and exits with a non-zero status

#### Scenario: Unsupported or corrupt file
- **WHEN** the file is neither a supported WAV nor a supported MP3, or contains no audio
- **THEN** the system prints an error saying the format is unsupported or the file is empty and exits with a non-zero status

#### Scenario: No output device
- **WHEN** no audio output device is available
- **THEN** the system prints an error saying no output device was found and exits with a non-zero status

### Requirement: Audible playback
The system SHALL play the file through the default audio output device at its original speed and pitch, whatever the file's sample rate or channel count.

#### Scenario: Stereo 44.1 kHz file
- **WHEN** the user plays a stereo 44.1 kHz MP3
- **THEN** the audio is heard at normal speed and the playback lasts as long as the file

### Requirement: Live count in step with playback
The system SHALL analyse the file as it plays, so that the transcript, total word count and WPM reflect the audio heard so far and update with the same latency as for microphone input. The elapsed time shown SHALL follow playback time.

#### Scenario: Count grows while playing
- **WHEN** a file with continuous speech is playing
- **THEN** the partial text, finalised lines, total word count and WPM update during playback rather than only after the file ends

#### Scenario: Count not ahead of audio
- **WHEN** the file is 60 seconds long and 10 seconds have been played
- **THEN** the total word count includes no words spoken after roughly the 10 second mark of the file

#### Scenario: Level indicator
- **WHEN** the file is playing
- **THEN** the level indicator shows the level of the audio being played and is labelled "audio"

### Requirement: End of file
The system SHALL, when playback reaches the end of the file, finalise and count the last utterance, stop the elapsed time and WPM from advancing, show that the file has finished, and keep the final count on screen until the user quits.

#### Scenario: Last utterance counted
- **WHEN** the file ends while the speaker is still within an utterance, with no trailing pause
- **THEN** that utterance is finalised and its words are included in the total

#### Scenario: Finished state
- **WHEN** playback has ended and the last utterance has been counted
- **THEN** the UI shows a "finished" indication, the totals and elapsed time no longer change, and the system waits for the user to quit

#### Scenario: Quit during playback
- **WHEN** the user presses `q`, Esc or Ctrl+C while the file is playing
- **THEN** playback and analysis stop, the terminal is restored and the summary line is printed for the portion played

#### Scenario: Summary for a finished file
- **WHEN** the user quits after the file has finished
- **THEN** the summary line reports the total words, the playback duration of the file and the average WPM over that duration

# audio-input Specification

## Purpose

Defines how the application discovers microphones, lets the user choose one, and obtains a live audio stream from it.

## Requirements

### Requirement: List capture devices
The system SHALL enumerate the audio capture devices available on the machine and present them with a stable index and a human-readable name.

#### Scenario: Devices listed on request
- **WHEN** the user runs the application with `--list-devices`
- **THEN** the system prints one line per capture device, in the form `<index>: <name>`, and exits with status 0 without opening any device

#### Scenario: No capture device present
- **WHEN** no capture device is available
- **THEN** the system prints an error saying no microphone was found and exits with a non-zero status

### Requirement: Interactive device selection
The system SHALL let the user choose the capture device from the list of available devices before listening starts, unless a device was given on the command line or an audio file was given with `--file`, in which case no capture device is chosen or opened.

#### Scenario: Pick from list
- **WHEN** the application starts without `--device` or `--file` and at least one device exists
- **THEN** the system shows the device list, lets the user move a highlight with the arrow keys and confirm with Enter, and starts listening on the confirmed device

#### Scenario: Single device
- **WHEN** exactly one capture device exists and neither `--device` nor `--file` is given
- **THEN** the system still shows the picker with that device pre-highlighted so the user confirms it

#### Scenario: Device given by flag
- **WHEN** the user passes `--device <index>` or `--device <name-substring>` matching exactly one device
- **THEN** the system skips the picker and listens on that device

#### Scenario: Invalid device flag
- **WHEN** `--device` matches no device or more than one device
- **THEN** the system prints an error with the device list and exits with a non-zero status

#### Scenario: Audio file given
- **WHEN** the user passes `--file`
- **THEN** the system shows no device picker and does not require a capture device to exist

### Requirement: Continuous audio capture
The system SHALL capture audio continuously from the selected device until the user quits, converting it to mono 16 kHz samples for transcription regardless of the device's native format.

#### Scenario: Device opens
- **WHEN** the user has selected a device
- **THEN** the system starts capturing and the UI shows a live input level indicator

#### Scenario: Device fails
- **WHEN** the device cannot be opened or disappears during capture
- **THEN** the system restores the terminal, prints the error and exits with a non-zero status

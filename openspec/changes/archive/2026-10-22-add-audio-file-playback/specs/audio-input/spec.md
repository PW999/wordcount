# Spec Delta

## MODIFIED Requirements

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

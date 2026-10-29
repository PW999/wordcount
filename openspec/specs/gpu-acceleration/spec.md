# gpu-acceleration Specification

## Purpose

Lets the speech model run on a GPU instead of the CPU to cut CPU load, heat and fan noise and to make larger models usable live, while keeping CPU-only builds and runs fully supported.

## Requirements

### Requirement: Opt-in GPU build
The system SHALL offer a build option that includes GPU support for whisper.cpp, and SHALL keep the default build CPU-only with the same prerequisites as before.

#### Scenario: Default build
- **WHEN** the user builds without selecting a GPU option
- **THEN** the result is a CPU-only binary and no GPU packages are required

#### Scenario: GPU build
- **WHEN** the user builds with the GPU option and the GPU prerequisites are installed
- **THEN** the build succeeds and produces a binary that can run the model on a GPU

#### Scenario: GPU prerequisites missing
- **WHEN** the user builds with the GPU option and a GPU prerequisite is missing
- **THEN** the build stops with a message naming the missing prerequisite and does not overwrite an existing working binary

#### Scenario: Switching between builds
- **WHEN** the user alternates between the CPU build and the GPU build
- **THEN** each build is produced without a clean rebuild of the other

### Requirement: GPU used by default when available
The system SHALL, in a GPU-enabled build, run speech recognition on the GPU by default, and SHALL fall back to the CPU with a warning when no usable GPU is found or the GPU cannot load the model.

#### Scenario: GPU present
- **WHEN** a GPU-enabled binary starts on a machine with a usable GPU
- **THEN** the model runs on the GPU

#### Scenario: No usable GPU
- **WHEN** a GPU-enabled binary starts on a machine without a usable GPU
- **THEN** the system warns that it is using the CPU and transcription continues on the CPU

#### Scenario: Model does not fit
- **WHEN** the model is too large for the GPU's memory
- **THEN** the system warns that it is using the CPU and transcription continues on the CPU

#### Scenario: CPU-only build
- **WHEN** a CPU-only binary is run
- **THEN** the model runs on the CPU and no GPU warning is shown

### Requirement: Disable the GPU
The system SHALL accept `--no-gpu` to force CPU execution in a GPU-enabled build.

#### Scenario: Forced CPU
- **WHEN** the user passes `--no-gpu` to a GPU-enabled binary on a machine with a usable GPU
- **THEN** the model runs on the CPU and the backend report says so

#### Scenario: Flag on a CPU-only build
- **WHEN** the user passes `--no-gpu` to a CPU-only binary
- **THEN** the system runs normally on the CPU

### Requirement: GPU device selection
The system SHALL accept `--gpu-device <index>` to choose which GPU runs the model when several are present, using the same indices that the backend report lists.

#### Scenario: Choose second GPU
- **WHEN** the machine has two GPUs and the user passes `--gpu-device 1`
- **THEN** the model runs on the GPU with index 1

#### Scenario: Invalid index
- **WHEN** `--gpu-device` is not a listed index
- **THEN** the system prints an error naming the valid indices and exits with a non-zero status before loading the model

#### Scenario: Combined with no-gpu
- **WHEN** the user passes both `--no-gpu` and `--gpu-device`
- **THEN** the system prints an error saying the options are mutually exclusive and exits with a non-zero status

### Requirement: Report the compute backend
The system SHALL show which compute backend and device runs the model, at startup and in `--version` output, so that a CPU fallback is never silent.

#### Scenario: Startup on GPU
- **WHEN** the model loads on a GPU
- **THEN** the system prints the backend and device name before the full-screen UI opens

#### Scenario: Startup on CPU
- **WHEN** the model loads on the CPU
- **THEN** the system prints that it is running on the CPU

#### Scenario: Version output
- **WHEN** the user runs `--version`
- **THEN** the output lists the compute backends the binary was built with and the GPUs it can see, with their indices

#### Scenario: Backend logs kept off the UI
- **WHEN** the full-screen UI is running
- **THEN** no GPU driver or backend log output is drawn on the screen

### Requirement: No change to recognition behaviour
The system SHALL produce the same transcript text and word counting contract on the GPU as on the CPU, within the normal numerical differences of the two backends, and SHALL keep audio on the local machine.

#### Scenario: Same file on both backends
- **WHEN** the same audio file is analysed once on the CPU and once on the GPU with the same model
- **THEN** the total word counts differ by no more than 2%

#### Scenario: Offline
- **WHEN** the model runs on the GPU
- **THEN** no audio or text is sent over the network

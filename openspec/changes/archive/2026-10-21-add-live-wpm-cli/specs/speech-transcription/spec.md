# Spec Delta

## Purpose

Defines how captured speech is converted to text locally and near real time, including model and language configuration.

## ADDED Requirements

### Requirement: Local model supplied by user
The system SHALL perform speech recognition entirely offline using a whisper-format model file given with `--model <path>`, and SHALL NOT send audio over the network.

#### Scenario: Model loads
- **WHEN** the user passes a valid `--model` path
- **THEN** the system loads it before listening starts and shows a "loading model" status until ready

#### Scenario: Missing or invalid model
- **WHEN** `--model` is absent, unreadable or not a valid model file
- **THEN** the system prints an error naming the problem and exits with a non-zero status before opening the microphone

### Requirement: Language selection
The system SHALL support multilingual models with a `--lang <code>` option, defaulting to automatic language detection.

#### Scenario: Explicit language
- **WHEN** the user passes `--lang nl`
- **THEN** speech is transcribed as Dutch

#### Scenario: English-only model with other language
- **WHEN** the model is English-only and `--lang` is set to a different language than `en`
- **THEN** the system prints a warning and uses English

### Requirement: Live partial and final text
The system SHALL emit in-progress (partial) text while the user is speaking, refreshed at least every 2 seconds on supported hardware, and SHALL replace it with final text when the speaker pauses or an utterance reaches 15 seconds.

#### Scenario: Speaking then pausing
- **WHEN** the user speaks a sentence and then is silent for about 0.7 seconds
- **THEN** the partial text is replaced by the final text of that utterance

#### Scenario: Long monologue
- **WHEN** the user speaks continuously for more than 15 seconds
- **THEN** the system finalises the current utterance and starts a new one without dropping audio

### Requirement: Ignore non-speech output
The system SHALL NOT produce text or count words for silence or non-speech audio, including model annotations such as `[BLANK_AUDIO]`, `(music)` or `[noise]`.

#### Scenario: Silence
- **WHEN** the microphone captures only silence or background noise
- **THEN** no text is added to the transcript and the word count does not change

#### Scenario: Annotation tokens
- **WHEN** the model returns text consisting of bracketed or parenthesised annotations
- **THEN** those annotations are removed before display and counting

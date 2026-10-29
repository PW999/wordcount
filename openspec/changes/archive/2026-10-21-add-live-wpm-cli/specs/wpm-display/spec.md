# Spec Delta

## Purpose

Defines how words per minute is computed from detected speech and how it and the live transcript are shown in the terminal.

## ADDED Requirements

### Requirement: Rolling words-per-minute
The system SHALL compute words per minute from the words detected within a trailing 60-second window, dividing by the elapsed listening time when that is shorter than 60 seconds (but not less than 5 seconds), and SHALL update it at least once per second.

#### Scenario: Steady speech
- **WHEN** the user speaks at a constant 150 words per minute for over a minute
- **THEN** the displayed value converges to approximately 150 (within 15%)

#### Scenario: Pause decays value
- **WHEN** the user stops speaking
- **THEN** the displayed value decreases over the following seconds as words leave the window, reaching 0 after 60 seconds of silence

#### Scenario: Startup
- **WHEN** fewer than 5 seconds have elapsed since listening began
- **THEN** the displayed value is computed as if 5 seconds had elapsed

### Requirement: Prominent count display
The system SHALL display the current words-per-minute value in large block characters at the top of a full-screen terminal UI, together with the total word count of the session and the elapsed time.

#### Scenario: Large digits visible
- **WHEN** the UI is running in a terminal of at least 80x24
- **THEN** the WPM value is drawn in digits at least 5 rows tall, clearly dominating the screen

#### Scenario: Small terminal
- **WHEN** the terminal is smaller than the large digits require
- **THEN** the system falls back to a single-line bold WPM display

### Requirement: Live transcript feed
The system SHALL show the detected text below the count as a scrolling feed, with the in-progress utterance visually distinct from finalised utterances and the newest text always visible.

#### Scenario: Partial then final
- **WHEN** a partial text updates repeatedly and is then finalised
- **THEN** the partial line is updated in place and, once final, becomes a normal line in the feed with a new partial line below it

#### Scenario: Feed overflow
- **WHEN** the transcript is taller than the available area
- **THEN** the oldest lines scroll out of view and the newest remain visible

### Requirement: Clean exit
The system SHALL quit on `q`, Esc or Ctrl+C, restoring the terminal to its previous state and printing a one-line summary (total words, duration, average WPM).

#### Scenario: Quit
- **WHEN** the user presses `q` while listening
- **THEN** capture stops, the terminal is restored and the summary line is printed

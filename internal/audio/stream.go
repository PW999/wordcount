package audio

// Stream is a source of mono 16 kHz float32 audio for transcription: a live
// capture device or a file being played back.
type Stream interface {
	// Chunks delivers samples in callback-sized pieces. A stream that ends
	// (a finished file) closes the channel after the last chunk.
	Chunks() <-chan []float32
	// Level is the RMS (0..1) of the most recent chunk.
	Level() float32
	// Err receives a value if the device fails during streaming.
	Err() <-chan error
	// Close stops the stream and releases the device.
	Close()
}

var (
	_ Stream = (*Capture)(nil)
	_ Stream = (*FilePlayer)(nil)
)

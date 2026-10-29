package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wavBytes builds a WAV file with the given fmt tag, bit depth and raw sample data.
func wavBytes(tag uint16, rate, channels, bits int, data []byte) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	blockAlign := channels * bits / 8
	b.WriteString("RIFF")
	binary.Write(&b, le, uint32(36+len(data)))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, le, uint32(16))
	binary.Write(&b, le, tag)
	binary.Write(&b, le, uint16(channels))
	binary.Write(&b, le, uint32(rate))
	binary.Write(&b, le, uint32(rate*blockAlign))
	binary.Write(&b, le, uint16(blockAlign))
	binary.Write(&b, le, uint16(bits))
	b.WriteString("data")
	binary.Write(&b, le, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

func pcm16(samples ...int16) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

func TestDecodeWAV(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		rate, ch     int
		wantSamples  []float32
		wantDuration time.Duration
	}{
		{
			name: "16-bit mono", data: wavBytes(wavPCM, 8000, 1, 16, pcm16(0, 16384, -32768, 32767)),
			rate: 8000, ch: 1, wantSamples: []float32{0, 0.5, -1, 32767.0 / 32768}, wantDuration: 500 * time.Microsecond,
		},
		{
			name: "16-bit stereo keeps interleaving", data: wavBytes(wavPCM, 44100, 2, 16, pcm16(16384, -16384, 0, 8192)),
			rate: 44100, ch: 2, wantSamples: []float32{0.5, -0.5, 0, 0.25}, wantDuration: 2 * time.Second / 44100,
		},
		{
			name: "8-bit is unsigned", data: wavBytes(wavPCM, 8000, 1, 8, []byte{128, 255, 0}),
			rate: 8000, ch: 1, wantSamples: []float32{0, 127.0 / 128, -1}, wantDuration: 3 * time.Second / 8000,
		},
		{
			name: "24-bit", data: wavBytes(wavPCM, 8000, 1, 24, []byte{0, 0, 0x40, 0, 0, 0xC0}),
			rate: 8000, ch: 1, wantSamples: []float32{0.5, -0.5}, wantDuration: 2 * time.Second / 8000,
		},
		{
			name: "32-bit", data: wavBytes(wavPCM, 8000, 1, 32, []byte{0, 0, 0, 0x40}),
			rate: 8000, ch: 1, wantSamples: []float32{0.5}, wantDuration: time.Second / 8000,
		},
		{
			name: "partial trailing frame is dropped", data: wavBytes(wavPCM, 8000, 2, 16, pcm16(1000, 2000, 3000)),
			rate: 8000, ch: 2, wantSamples: []float32{1000.0 / 32768, 2000.0 / 32768}, wantDuration: time.Second / 8000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Decode(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if d.SampleRate != tc.rate || d.Channels != tc.ch {
				t.Fatalf("format = %d Hz %d ch, want %d Hz %d ch", d.SampleRate, d.Channels, tc.rate, tc.ch)
			}
			if len(d.Samples) != len(tc.wantSamples) {
				t.Fatalf("got %d samples, want %d", len(d.Samples), len(tc.wantSamples))
			}
			for i, w := range tc.wantSamples {
				if math.Abs(float64(d.Samples[i]-w)) > 1e-6 {
					t.Errorf("sample %d = %v, want %v", i, d.Samples[i], w)
				}
			}
			if d.Duration() != tc.wantDuration {
				t.Errorf("duration = %v, want %v", d.Duration(), tc.wantDuration)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error  // matched with errors.Is when set
		wantMsg string // substring otherwise
	}{
		{name: "empty input", data: nil, wantErr: ErrUnsupportedFormat},
		{name: "text file", data: []byte("hello, this is not audio"), wantErr: ErrUnsupportedFormat},
		{name: "WAV without samples", data: wavBytes(wavPCM, 16000, 1, 16, nil), wantErr: ErrEmptyAudio},
		{name: "float WAV", data: wavBytes(wavFloat, 16000, 1, 32, make([]byte, 8)), wantMsg: "32-bit float"},
		{name: "compressed WAV", data: wavBytes(2, 16000, 1, 4, make([]byte, 8)), wantMsg: "compressed format tag 2"},
		{name: "12-bit PCM", data: wavBytes(wavPCM, 16000, 1, 12, make([]byte, 8)), wantMsg: "12-bit"},
		{name: "truncated RIFF header", data: []byte("RIFF\x00\x00\x00\x00WAVE"), wantMsg: "invalid WAV"},
		{
			name:    "WAV over the length cap",
			data:    wavBytes(wavPCM, 8000, 1, 8, make([]byte, 8000*3601)),
			wantErr: ErrTooLong,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.data)
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

func TestDecodeFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.wav")
	_, err := DecodeFile(path)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, want not-exist error naming %s", err, path)
	}
}

func TestDecodeFileErrorNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.wav")
	if err := os.WriteFile(path, []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := DecodeFile(path)
	if !errors.Is(err, ErrUnsupportedFormat) || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, want unsupported-format error naming %s", err, path)
	}
}

func TestDecodeJFKWAV(t *testing.T) {
	d, err := DecodeFile("../../testdata/jfk.wav")
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate != 16000 || d.Channels != 1 {
		t.Fatalf("format = %d Hz %d ch, want 16000 Hz 1 ch", d.SampleRate, d.Channels)
	}
	if got := d.Duration(); got < 10900*time.Millisecond || got > 11100*time.Millisecond {
		t.Errorf("duration = %v, want about 11s", got)
	}
}

// The MP3 fixture is decoded by content, so a misleading extension must not matter.
func TestDecodeMP3FixtureEvenWhenNamedWAV(t *testing.T) {
	data, err := os.ReadFile("../../testdata/jfk.mp3")
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(renamed, data, 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := DecodeFile(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate != 16000 {
		t.Errorf("sample rate = %d, want 16000", d.SampleRate)
	}
	const want = 11 * time.Second
	if got := d.Duration(); got < want-50*time.Millisecond || got > want+50*time.Millisecond {
		t.Errorf("duration = %v, want %v +-50ms", got, want)
	}
	var peak float32
	for _, s := range d.Samples {
		peak = max(peak, float32(math.Abs(float64(s))))
	}
	if peak < 0.05 {
		t.Errorf("peak = %v, decoded audio looks silent", peak)
	}
}

func TestSniff(t *testing.T) {
	if !isMP3([]byte{0xFF, 0xFB, 0x90}) || !isMP3([]byte("ID3\x04")) {
		t.Error("MP3 frame sync and ID3 tag should be recognised")
	}
	if isMP3([]byte{0xFF, 0xF9}) { // layer bits 00 are reserved
		t.Error("reserved layer must not be treated as MP3")
	}
	if isWAV([]byte("RIFF....AVI ")) {
		t.Error("RIFF AVI is not WAV")
	}
}

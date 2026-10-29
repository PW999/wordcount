package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-audio/wav"
	"github.com/hajimehoshi/go-mp3"
)

// MaxDuration is the longest file that is decoded; the whole file is held in
// memory, so this keeps a stray multi-hour recording from exhausting it.
const MaxDuration = time.Hour

// Errors returned by Decode so callers can tell the cases apart.
var (
	ErrUnsupportedFormat = errors.New("unsupported audio format (only WAV and MP3 are supported)")
	ErrEmptyAudio        = errors.New("file contains no audio")
	ErrTooLong           = errors.New("audio is too long")
)

// WAV format tags (from the fmt chunk).
const (
	wavPCM        = 1
	wavFloat      = 3
	wavExtensible = 0xFFFE
)

// Decoded is a whole file as interleaved float32 PCM at its native format.
type Decoded struct {
	Samples    []float32
	SampleRate int
	Channels   int
}

// Frames is the number of sample frames (one sample per channel).
func (d *Decoded) Frames() int {
	if d.Channels == 0 {
		return 0
	}
	return len(d.Samples) / d.Channels
}

// Duration is the playing time of the file.
func (d *Decoded) Duration() time.Duration {
	if d.SampleRate == 0 {
		return 0
	}
	return time.Duration(d.Frames()) * time.Second / time.Duration(d.SampleRate)
}

// DecodeFile reads and decodes the WAV or MP3 file at path.
func DecodeFile(path string) (*Decoded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err // already names the path
	}
	d, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// Decode recognises the format from the content, not a file extension.
func Decode(data []byte) (*Decoded, error) {
	switch {
	case isWAV(data):
		return decodeWAV(data)
	case isMP3(data):
		return decodeMP3(data)
	default:
		return nil, ErrUnsupportedFormat
	}
}

func isWAV(b []byte) bool {
	return len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE"
}

// isMP3 accepts an ID3v2 tag or an MPEG audio frame sync (11 set bits) with a
// valid layer, as every MP3 starts with one of the two.
func isMP3(b []byte) bool {
	if len(b) >= 3 && string(b[0:3]) == "ID3" {
		return true
	}
	if len(b) < 2 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return false
	}
	return b[1]&0x06 != 0 // layer bits 00 are reserved
}

func decodeWAV(data []byte) (*Decoded, error) {
	dec := wav.NewDecoder(bytes.NewReader(data))
	// FwdToPCM reads the headers and positions at the sample data, which also
	// fills in PCMLen for the length check below.
	if err := dec.FwdToPCM(); err != nil {
		return nil, fmt.Errorf("invalid WAV: %w", err)
	}
	if dec.NumChans < 1 || dec.SampleRate == 0 {
		return nil, errors.New("invalid WAV: missing channels or sample rate")
	}

	switch dec.WavAudioFormat {
	case wavPCM, wavExtensible:
	case wavFloat:
		return nil, fmt.Errorf("unsupported WAV encoding: %d-bit float (use 8, 16, 24 or 32-bit PCM)", dec.BitDepth)
	default:
		return nil, fmt.Errorf("unsupported WAV encoding: compressed format tag %d (use 8, 16, 24 or 32-bit PCM)", dec.WavAudioFormat)
	}
	switch dec.BitDepth {
	case 8, 16, 24, 32:
	default:
		return nil, fmt.Errorf("unsupported WAV encoding: %d-bit samples (use 8, 16, 24 or 32-bit PCM)", dec.BitDepth)
	}

	// Check the length from the header before reading anything into memory.
	bytesPerFrame := int64(dec.BitDepth/8) * int64(dec.NumChans)
	if dur := time.Duration(dec.PCMLen() / bytesPerFrame * int64(time.Second) / int64(dec.SampleRate)); dur > MaxDuration {
		return nil, fmt.Errorf("%w: %s exceeds the %s limit", ErrTooLong, dur.Round(time.Second), MaxDuration)
	}

	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("invalid WAV: %w", err)
	}
	if len(buf.Data) == 0 {
		return nil, ErrEmptyAudio
	}

	scale, offset := wavScale(int(dec.BitDepth))
	ch := int(dec.NumChans)
	out := make([]float32, len(buf.Data)/ch*ch) // drop a trailing partial frame
	for i := range out {
		out[i] = (float32(buf.Data[i]) - offset) * scale
	}
	return &Decoded{Samples: out, SampleRate: int(dec.SampleRate), Channels: ch}, nil
}

// wavScale maps the integer sample range of bits-per-sample to -1..1. 8-bit
// WAV is unsigned with the midpoint at 128; wider samples are signed.
func wavScale(bits int) (scale, offset float32) {
	if bits == 8 {
		return 1.0 / 128, 128
	}
	return 1.0 / float32(uint64(1)<<(bits-1)), 0
}

func decodeMP3(data []byte) (*Decoded, error) {
	dec, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid MP3: %w", err)
	}
	// go-mp3 always yields 16-bit stereo; mono sources are duplicated.
	const channels, bytesPerFrame = 2, 4
	frames := dec.Length() / bytesPerFrame
	if frames <= 0 {
		return nil, ErrEmptyAudio
	}
	if dur := time.Duration(frames * int64(time.Second) / int64(dec.SampleRate())); dur > MaxDuration {
		return nil, fmt.Errorf("%w: %s exceeds the %s limit", ErrTooLong, dur.Round(time.Second), MaxDuration)
	}

	pcm := make([]byte, frames*bytesPerFrame)
	n, err := io.ReadFull(dec, pcm)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid MP3: %w", err)
	}
	n -= n % bytesPerFrame
	if n == 0 {
		return nil, ErrEmptyAudio
	}

	out := make([]float32, n/2)
	for i := range out {
		out[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
	}

	// Drop the encoder's priming and padding so the length is the audio's own.
	skip, end := mp3Trim(data, len(out)/channels)
	out = out[skip*channels : end*channels]
	if len(out) == 0 {
		return nil, ErrEmptyAudio
	}
	return &Decoded{Samples: out, SampleRate: dec.SampleRate(), Channels: channels}, nil
}

// mp3Trim returns the range of decoded frames (of total) that holds audio. An
// encoder usually writes a silent Xing/Info frame first, followed by the
// encoder delay, and pads the last frame; the Xing/LAME header records both.
// Without such a header the whole decode is kept.
func mp3Trim(data []byte, total int) (skip, end int) {
	skip, end = 0, total
	p := 0
	if len(data) >= 10 && string(data[0:3]) == "ID3" {
		p = 10 + (int(data[6]&0x7F)<<21 | int(data[7]&0x7F)<<14 | int(data[8]&0x7F)<<7 | int(data[9]&0x7F))
		if data[5]&0x10 != 0 {
			p += 10 // footer
		}
	}
	if len(data) < p+4 || data[p] != 0xFF || data[p+1]&0xE0 != 0xE0 {
		return
	}
	version := data[p+1] >> 3 & 3 // 3 = MPEG-1, 2 = MPEG-2, 0 = MPEG-2.5
	mono := data[p+3]>>6 == 3
	frameSamples, sideInfo := 1152, 32
	switch {
	case version == 3 && mono:
		sideInfo = 17
	case version != 3 && mono:
		frameSamples, sideInfo = 576, 9
	case version != 3:
		frameSamples, sideInfo = 576, 17
	}
	tag := p + 4 + sideInfo
	if data[p+1]&1 == 0 {
		tag += 2 // CRC
	}
	if len(data) < tag+4 {
		return
	}
	if id := string(data[tag : tag+4]); id != "Xing" && id != "Info" {
		return
	}
	skip = frameSamples // the Info frame itself decodes to silence

	// Encoder string at tag+120 (after flags, frame count, size, TOC and
	// quality), then delay and padding as two 12-bit values 21 bytes in.
	const lameOffset, decoderDelay = 120, 529
	if len(data) >= tag+lameOffset+24 {
		b := data[tag+lameOffset+21 : tag+lameOffset+24]
		delay, padding := int(b[0])<<4|int(b[1])>>4, int(b[1]&0x0F)<<8|int(b[2])
		if delay+padding < frameSamples*4 { // sanity: both fields are small
			skip += delay + decoderDelay
			end -= max(0, padding-decoderDelay)
		}
	}
	if skip > end {
		return 0, total
	}
	return skip, end
}

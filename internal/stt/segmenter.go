package stt

import "math"

const (
	sampleRate = 16000
	frameSize  = sampleRate / 50 // 20 ms

	blockFrames  = 5     // frames per noise-floor block (100 ms)
	warmupBlocks = 3     // no utterance can start until the floor has been measured
	floorRise    = 1.005 // per-block growth limit of the noise floor
	energyAlpha  = 0.4   // smoothing of frame RMS
)

// Transcriber turns 16 kHz mono float32 samples into text.
type Transcriber interface {
	Transcribe(samples []float32) (string, error)
}

// Request asks for a transcription of the current utterance. Partial requests
// carry the utterance so far; the Final request carries the finished utterance.
type Request struct {
	Samples []float32
	Final   bool
}

// SegmenterConfig tunes the voice-activity detector and utterance limits.
type SegmenterConfig struct {
	PreRoll         int     // samples kept from before speech starts
	Endpoint        int     // trailing silence (samples) that ends an utterance
	MaxUtterance    int     // utterance length (samples) that forces a split
	PartialEvery    int     // audio (samples) between partial requests
	MinSpeechFrames int     // speech frames needed before an utterance counts
	TrailingKeep    int     // silence (samples) kept after the last speech in a final
	MinStartRMS     float64 // lowest speech-start threshold
	MinStopRMS      float64 // lowest speech-stop threshold
}

// DefaultSegmenterConfig matches the design: 300 ms pre-roll, 0.7 s endpoint,
// 15 s cap and a partial about every second.
func DefaultSegmenterConfig() SegmenterConfig {
	return SegmenterConfig{
		PreRoll:         sampleRate * 3 / 10,
		Endpoint:        sampleRate * 7 / 10,
		MaxUtterance:    sampleRate * 15,
		PartialEvery:    sampleRate,
		MinSpeechFrames: 5,
		TrailingKeep:    sampleRate / 5,
		MinStartRMS:     0.015,
		MinStopRMS:      0.010,
	}
}

// Segmenter splits a sample stream into utterances with an energy VAD. It does
// no transcription and never blocks; callers hand its Requests to a decoder.
// Silence is never turned into a Request.
type Segmenter struct {
	cfg SegmenterConfig

	pending []float32 // samples not yet forming a full frame
	preroll []float32 // most recent audio while idle

	// Noise floor: the quietest recent 100 ms block, rising slowly so that
	// steady background noise is learned but a pause in speech pulls it back down.
	floor      float64
	blocks     int // 100 ms blocks seen; thresholds are trusted after warmupBlocks
	blockSumSq float64
	blockN     int
	energy     float64 // smoothed frame RMS used for start/stop decisions

	speaking     bool
	utt          []float32
	speechFrames int
	lastSpeech   int // end offset in utt of the last speech frame
	silence      int // trailing silent samples
	sincePartial int
}

// NewSegmenter returns a Segmenter using cfg.
func NewSegmenter(cfg SegmenterConfig) *Segmenter {
	return &Segmenter{cfg: cfg}
}

// Push feeds captured samples and returns any transcription requests they trigger.
func (s *Segmenter) Push(chunk []float32) []Request {
	var out []Request
	s.pending = append(s.pending, chunk...)
	for len(s.pending) >= frameSize {
		frame := s.pending[:frameSize]
		out = s.frame(frame, out)
		s.pending = s.pending[frameSize:]
	}
	// Keep the backing array from growing without bound.
	s.pending = append([]float32(nil), s.pending...)
	return out
}

// Flush ends the stream: an utterance in progress with enough speech is
// finalised, anything else (silence, a blip) produces nothing. The segmenter
// is idle afterwards. Use it when no more audio will arrive, such as the end of a file.
func (s *Segmenter) Flush() []Request {
	var out []Request
	if s.speaking {
		out = s.finish(out)
		s.speaking = false
	}
	s.pending = nil
	return out
}

func (s *Segmenter) startThreshold() float64 { return math.Max(s.cfg.MinStartRMS, s.floor*3) }
func (s *Segmenter) stopThreshold() float64  { return math.Max(s.cfg.MinStopRMS, s.floor*2) }

// trackNoise folds a frame into the 100 ms block statistics behind the noise floor.
func (s *Segmenter) trackNoise(f []float32) {
	for _, v := range f {
		s.blockSumSq += float64(v) * float64(v)
	}
	s.blockN++
	if s.blockN < blockFrames {
		return
	}
	block := math.Sqrt(s.blockSumSq / float64(s.blockN*frameSize))
	s.blockSumSq, s.blockN = 0, 0
	switch {
	case s.blocks == 0 || block < s.floor:
		s.floor = block
	default:
		s.floor = math.Min(block, s.floor*floorRise)
	}
	s.blocks++
}

func (s *Segmenter) frame(f []float32, out []Request) []Request {
	r := rms(f)
	s.energy += (r - s.energy) * energyAlpha
	s.trackNoise(f)

	if !s.speaking {
		if s.blocks < warmupBlocks || s.energy < s.startThreshold() {
			s.addPreroll(f)
			return out
		}
		s.speaking = true
		s.utt = append(s.utt[:0], s.preroll...)
		s.preroll = s.preroll[:0]
		s.speechFrames, s.silence, s.sincePartial = 0, 0, 0
	}

	s.utt = append(s.utt, f...)
	s.sincePartial += len(f)
	// The raw level counts frames of speech (so a smoothed blip is not
	// mistaken for speech); the smoothed level decides when it has gone quiet.
	if r >= s.stopThreshold() {
		s.speechFrames++
		s.lastSpeech = len(s.utt)
	}
	if s.energy >= s.stopThreshold() {
		s.silence = 0
	} else {
		s.silence += len(f)
	}

	switch {
	case s.silence >= s.cfg.Endpoint:
		out = s.finish(out)
		s.speaking = false
	case len(s.utt) >= s.cfg.MaxUtterance:
		// Split without dropping audio: the next utterance starts empty.
		out = s.finish(out)
	case s.sincePartial >= s.cfg.PartialEvery && s.speechFrames >= s.cfg.MinSpeechFrames:
		s.sincePartial = 0
		out = append(out, Request{Samples: append([]float32(nil), s.utt...)})
	}
	return out
}

// finish emits the final request when the utterance contained real speech and
// resets utterance state either way.
func (s *Segmenter) finish(out []Request) []Request {
	if s.speechFrames >= s.cfg.MinSpeechFrames {
		end := s.lastSpeech + s.cfg.TrailingKeep
		if end > len(s.utt) {
			end = len(s.utt)
		}
		out = append(out, Request{Samples: append([]float32(nil), s.utt[:end]...), Final: true})
	}
	s.utt = s.utt[:0]
	s.speechFrames, s.lastSpeech, s.sincePartial = 0, 0, 0
	return out
}

func (s *Segmenter) addPreroll(f []float32) {
	s.preroll = append(s.preroll, f...)
	if extra := len(s.preroll) - s.cfg.PreRoll; extra > 0 {
		s.preroll = append(s.preroll[:0], s.preroll[extra:]...)
	}
}

func rms(f []float32) float64 {
	var sum float64
	for _, v := range f {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(f)))
}

// Package speaker emulates the Apple II's single-bit speaker at $C030.
// Toggles are recorded on the bus, then once per video frame the
// accumulated toggles are rendered to PCM samples and queued to SDL.
//
// Threading model: SINGLE GOROUTINE. Toggle, EndFrame, and Close are
// all called from the main goroutine. SDL's QueueAudio is a push API,
// so SDL never calls into our code from its audio thread. No mutexes
// are required.
package speaker

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// peakAmplitude is the maximum int16 amplitude used before volume scaling.
// ~24000 leaves ~2 dB of headroom below int16 max (32767) so the one-pole
// lowpass overshoot + volume=1.0 cannot clip.
const peakAmplitude = 24000

// dcBlockR is the pole of the one-pole DC blocker applied after the lowpass.
// y[n] = x[n] - x[n-1] + dcBlockR*y[n-1]
// At 44.1 kHz: fc ~= (1 - R) * Fs / (2*pi) ~= 35 Hz, below 60 Hz motorboat
// and below all Apple II speaker tones. Removes the +amp idle DC bias that
// would otherwise feed the AC-coupled DAC and cause audible frame-rate hum.
const dcBlockR float32 = 0.995

// filterState bundles per-frame audio filter memory so RenderSamples takes
// one parameter/return instead of several positional floats. Zero value is
// the correct initial state.
type filterState struct {
	lpState float32 // one-pole lowpass memory
	xPrev   float32 // HPF input at n-1 (== previous lpState output)
	yPrev   float32 // HPF output at n-1
}

// Config holds tunable speaker parameters.
type Config struct {
	SampleRate int     // e.g. 44100
	CPUClock   int     // e.g. 1023000
	Volume     float32 // [0.0, 1.0]; caller must clamp
	Alpha      float32 // one-pole lowpass coefficient; default 0.15
}

// Speaker buffers $C030 toggle events and emits PCM audio frames.
type Speaker struct {
	sampleRate      int
	cpuClock        int
	samplesPerFrame int
	bytesPerFrame   uint32 // derived once
	amplitude       int16
	alpha           float32

	cyclePtr *uint64 // owned by main; read inside Toggle

	toggles []uint64 // frame-relative CPU cycles, monotone non-decreasing
	state   int8     // current speaker level: +1 or -1
	fs      filterState // lowpass + DC-blocker memory, carried across frames

	totalCycles    uint64 // absolute, drift-free accumulator
	samplesEmitted uint64 // absolute, drift-free accumulator

	underrunOnce sync.Once // only used to fire the first warning
	underruns    uint64
	queueErrOnce sync.Once // only used to fire the first QueueAudio error

	dev     sdl.AudioDeviceID
	enabled bool // false if SDL audio init failed; methods become safe no-ops
}

// New constructs a Speaker and opens an SDL audio device. If audio init
// fails, returns a non-nil *Speaker in disabled mode together with the
// error - this lets the caller defer spk.Close() unconditionally and
// keeps the emulator running silently.
//
// CONTRACT: New always returns a non-nil *Speaker so the caller pattern
// `defer spk.Close()` is safe even on error. Do not "fix" this to return
// (nil, err).
func New(cfg Config, cyclePtr *uint64) (*Speaker, error) {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}
	if cfg.CPUClock == 0 {
		cfg.CPUClock = 1023000
	}
	if cfg.Alpha == 0 {
		cfg.Alpha = 0.15
	}
	// Caller is responsible for clamping Volume to [0,1]; we do not
	// remap volume==0 to a default here (that would surprise the user).
	if cfg.Volume < 0 {
		cfg.Volume = 0
	}
	if cfg.Volume > 1 {
		cfg.Volume = 1
	}

	samplesPerFrame := cfg.SampleRate / 60
	s := &Speaker{
		sampleRate:      cfg.SampleRate,
		cpuClock:        cfg.CPUClock,
		samplesPerFrame: samplesPerFrame,
		bytesPerFrame:   uint32(samplesPerFrame * 2),
		amplitude:       int16(float32(peakAmplitude) * cfg.Volume),
		alpha:           cfg.Alpha,
		state:           +1,
		cyclePtr:        cyclePtr,
	}

	desired := &sdl.AudioSpec{
		Freq:     int32(cfg.SampleRate),
		Format:   sdl.AUDIO_S16LSB,
		Channels: 1,
		Samples:  1024,
	}
	obtained := &sdl.AudioSpec{}
	dev, err := sdl.OpenAudioDevice("", false, desired, obtained, 0)
	if err != nil {
		return s, fmt.Errorf("speaker: SDL audio open failed (running silent): %w", err)
	}
	s.dev = dev
	s.enabled = true

	// Pre-buffer one frame of silence so the device never starts empty.
	silence := make([]int16, s.samplesPerFrame)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&silence[0])), len(silence)*2)
	if err := sdl.QueueAudio(s.dev, raw); err != nil {
		// Non-fatal; emulator continues without pre-buffer.
		fmt.Fprintf(os.Stderr, "speaker: pre-buffer queue failed: %v\n", err)
	}

	sdl.PauseAudioDevice(dev, false)
	return s, nil
}

// Toggle records a $C030 access. Reads *cyclePtr to stamp the event.
// No mutex - main goroutine only.
func (s *Speaker) Toggle() {
	if !s.enabled {
		return
	}
	var c uint64
	if s.cyclePtr != nil {
		c = *s.cyclePtr
	}
	s.toggles = append(s.toggles, c)
}

// EndFrame is called once per video frame after the CPU has run for
// `framePrefixCycles` cycles (the value of *cyclePtr at end-of-frame).
// It advances internal counters, synthesizes (target-emitted) samples,
// and queues them to SDL.
func (s *Speaker) EndFrame(framePrefixCycles uint64) {
	if !s.enabled {
		return
	}

	toggles := s.toggles
	s.toggles = s.toggles[:0]

	// Drift-free sample count (see 2.3a).
	s.totalCycles += framePrefixCycles
	target := s.totalCycles * uint64(s.sampleRate) / uint64(s.cpuClock)
	nSamples := int(target - s.samplesEmitted)

	samples, newState, newFs := RenderSamples(
		toggles, nSamples,
		s.cpuClock, s.sampleRate,
		s.state, s.amplitude, s.alpha,
		s.fs,
		s.totalCycles-framePrefixCycles, // frameStartCycle (absolute)
		s.samplesEmitted,                // first absolute sample index in this frame
	)
	s.state = newState
	s.fs = newFs
	s.samplesEmitted = target

	// Underrun detection BEFORE we push.
	queuedBytes := sdl.GetQueuedAudioSize(s.dev)
	if queuedBytes < s.bytesPerFrame {
		s.underruns++
		s.underrunOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "speaker: audio underrun detected (will not warn again)")
		})
	}

	// Backpressure: if more than ~4 frames queued, drop this frame.
	if queuedBytes > 4*s.bytesPerFrame {
		return
	}

	if len(samples) == 0 {
		return
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), len(samples)*2)
	if err := sdl.QueueAudio(s.dev, raw); err != nil {
		s.queueErrOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "speaker: QueueAudio failed: %v (will not warn again)\n", err)
		})
	}
}

// Close releases the SDL audio device.
func (s *Speaker) Close() {
	if !s.enabled {
		return
	}
	sdl.PauseAudioDevice(s.dev, true)
	sdl.CloseAudioDevice(s.dev)
	s.enabled = false
}

// PauseFromHost pauses or resumes the SDL audio device. Wired to SDL
// window-focus / minimize / restore events from main.go.
// Safe no-op when the speaker is in disabled mode.
func (s *Speaker) PauseFromHost(pause bool) {
	if !s.enabled {
		return
	}
	sdl.PauseAudioDevice(s.dev, pause)
}

// Enabled reports whether the SDL audio device opened successfully.
// Disabled mode is signaled by Enabled() returning false; in that mode
// Toggle/EndFrame/Close/PauseFromHost are all safe no-ops.
func (s *Speaker) Enabled() bool { return s.enabled }

// Underruns returns the count of frames where the SDL queue was below
// one frame worth of audio at EndFrame time. Test/debug only.
func (s *Speaker) Underruns() uint64 { return s.underruns }

// RenderSamples is a pure function. Given a sorted list of frame-
// relative toggle cycles and the absolute global cycle/sample anchors,
// produce the next nSamples PCM samples.
//
// Input contract: toggles must be monotone non-decreasing. Multiple
// toggles at the same cycle are supported (equal cycles cancel/keep
// parity correctly because each applies state = -state in order).
//
// Parameters:
//
//	toggles          - frame-relative cycles, monotone non-decreasing.
//	nSamples         - exact number of samples to emit (drift-free, see 2.3a).
//	cpuClock,sampleRate
//	startState       - speaker level at start of frame, +1 or -1.
//	amplitude        - peak int16 magnitude.
//	alpha            - one-pole lowpass coefficient.
//	fs               - lowpass + DC-blocker memory carried across frames.
//	frameStartCycle  - absolute total cycles at start of this frame.
//	firstSampleIndex - absolute global sample index of out[0].
//
// Returns: (samples, newState, newFs). newFs contains updated
// lpState, xPrev, yPrev after the last emitted sample.
func RenderSamples(
	toggles []uint64,
	nSamples int,
	cpuClock, sampleRate int,
	startState int8,
	amplitude int16,
	alpha float32,
	fs filterState,
	frameStartCycle uint64,
	firstSampleIndex uint64,
) ([]int16, int8, filterState) {
	if nSamples <= 0 || sampleRate == 0 || cpuClock == 0 {
		// Even with zero output samples, apply toggles to update state.
		state := startState
		for range toggles {
			state = -state
		}
		return nil, state, fs
	}
	out := make([]int16, nSamples)
	state := startState
	ti := 0
	amp := float32(amplitude)
	for i := 0; i < nSamples; i++ {
		// Absolute global cycle of this sample.
		absCycle := (firstSampleIndex + uint64(i)) * uint64(cpuClock) / uint64(sampleRate)
		// Translate to frame-relative for toggle comparison.
		var sampleCycle uint64
		if absCycle >= frameStartCycle {
			sampleCycle = absCycle - frameStartCycle
		}
		for ti < len(toggles) && toggles[ti] <= sampleCycle {
			state = -state
			ti++
		}
		x := amp * float32(state)
		fs.lpState += alpha * (x - fs.lpState)

		// DC blocker: one-pole HPF in direct-form I.
		// Input is the post-lowpass signal; output carries only AC content.
		y := fs.lpState - fs.xPrev + dcBlockR*fs.yPrev
		fs.xPrev = fs.lpState
		fs.yPrev = y

		// Explicit int16 clamp BEFORE cast (operates on DC-blocked output).
		clamped := y
		if clamped > 32767 {
			clamped = 32767
		}
		if clamped < -32768 {
			clamped = -32768
		}
		out[i] = int16(clamped)
	}
	// Apply remaining toggles past the last sample so cone state is
	// correct at the start of the next frame.
	for ; ti < len(toggles); ti++ {
		state = -state
	}
	return out, state, fs
}

package speaker

import (
	"testing"
)

// helper: count zero-crossings in int16 slice (sign changes)
func zeroCrossings(samples []int16) int {
	count := 0
	for i := 1; i < len(samples); i++ {
		if (samples[i-1] > 0 && samples[i] < 0) || (samples[i-1] < 0 && samples[i] > 0) {
			count++
		}
	}
	return count
}

// helper: return max absolute value across the slice.
func maxAbs(xs []int16) int32 {
	var m int32
	for _, v := range xs {
		a := int32(v)
		if a < 0 {
			a = -a
		}
		if a > m {
			m = a
		}
	}
	return m
}

// 1. silence_no_toggles: idle state=+1 for one frame; DC blocker must drain the
// bias so the tail is near zero, not locked at +amp.
func TestSilenceNoToggles(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	samples, newState, _ := RenderSamples(
		nil, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)
	if len(samples) != nSamples {
		t.Fatalf("expected %d samples, got %d", nSamples, len(samples))
	}
	if newState != +1 {
		t.Errorf("expected state +1 unchanged, got %d", newState)
	}
	// Analytic bound: y_n <= amp * R^n. After 735 samples at R=0.995,
	// |y| <= 6000 * 0.995^735 ~= 6000 * 0.0247 ~= 148, i.e. ~amp/40.
	// Use amp/30 as the threshold: tighter than the strict bound leaves
	// head-room for floating-point noise while still catching any DC-bias
	// regression an order of magnitude before amp/4.
	lastAbs := int(samples[nSamples-1])
	if lastAbs < 0 {
		lastAbs = -lastAbs
	}
	if lastAbs > int(amplitude)/30 {
		t.Errorf("expected decayed idle tail |s| <= amp/30, got %d", lastAbs)
	}
}

// 2. single_toggle_midframe: toggle at midpoint; tone transitions still produce +/- samples (AC passes).
func TestSingleToggleMidframe(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	// Toggle at cycle ~8525 (half of 17050)
	toggleCycle := uint64(8525)
	samples, newState, _ := RenderSamples(
		[]uint64{toggleCycle}, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)
	if len(samples) != nSamples {
		t.Fatalf("expected %d samples, got %d", nSamples, len(samples))
	}
	if newState != -1 {
		t.Errorf("expected final state -1 after single toggle, got %d", newState)
	}
	// First sample should be positive (state=+1 before toggle).
	if samples[0] <= 0 {
		t.Errorf("first sample should be positive, got %d", samples[0])
	}
	// Last sample should be heading negative.
	if samples[nSamples-1] >= 0 {
		t.Errorf("last sample should be negative after toggle, got %d", samples[nSamples-1])
	}
}

// 3. high_freq_tone: ~1 kHz tone (~34 toggles in 17050 cyc); zero-crossings >= 30.
// 1 kHz is far above the 35 Hz HPF cutoff so it passes essentially unattenuated.
func TestHighFreqTone(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	// 1 kHz: toggle every ~511 cycles -> ~33 toggles in 17050 cycles
	var toggles []uint64
	for c := uint64(255); c < 17050; c += 511 {
		toggles = append(toggles, c)
	}

	samples, _, _ := RenderSamples(
		toggles, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)

	crossings := zeroCrossings(samples)
	if crossings < 30 {
		t.Errorf("expected >= 30 zero crossings for 1kHz tone, got %d", crossings)
	}
	// int16 range is enforced by the return type; see Test #10 for clamp behavior.
}

// 4. toggles_past_end_update_state: toggles with cycle > nSamples boundary still flip final state.
func TestTogglesPastEndUpdateState(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	// Toggle well past the last sample's cycle
	// Last sample's cycle in frame: (735-1)*1023000/44100 = ~17033
	// Put a toggle at 17040 (past last sample but within frame)
	toggles := []uint64{17040}
	samples, newState, _ := RenderSamples(
		toggles, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)
	if len(samples) != nSamples {
		t.Fatalf("expected %d samples, got %d", nSamples, len(samples))
	}
	// Toggle was past last sample: state should have been flipped
	if newState != -1 {
		t.Errorf("expected state -1 after toggle past last sample, got %d", newState)
	}
}

// 5. zero_samples_returns_nil: nSamples==0 returns nil; toggles still update state;
// filter memory passes through byte-identical when no samples are emitted.
func TestZeroSamplesReturnsNil(t *testing.T) {
	seed := filterState{lpState: 3000, xPrev: 3000, yPrev: 2500}
	samples, newState, newFs := RenderSamples(
		nil, 0,
		1023000, 44100,
		+1, 6000, 0.15, seed,
		0, 0,
	)
	if samples != nil {
		t.Fatalf("expected nil samples, got %v", samples)
	}
	if newState != +1 {
		t.Fatalf("state changed on zero-sample path")
	}
	if newFs != seed {
		t.Fatalf("filter memory mutated on zero-sample path: got %+v want %+v",
			newFs, seed)
	}
}

// 6. toggle_at_cycle_zero: toggle at cycle 0 affects the very first sample.
func TestToggleAtCycleZero(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	// cycle 0: first sample's cycle = 0*cpuClock/sampleRate = 0
	// Toggle at 0 should be applied BEFORE sample 0.
	toggles := []uint64{0}
	samples, _, _ := RenderSamples(
		toggles, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)
	// After toggle at cycle 0, state becomes -1, first sample reflects that.
	if samples[0] >= 0 {
		t.Errorf("first sample should be negative after toggle at cycle 0, got %d", samples[0])
	}
}

// 7. lowpass_seam_continuity: carry fs across two calls; seam is smooth.
// HPF at R=0.995 adds negligible seam discontinuity at audio-band frequencies
// because its cutoff (~35 Hz) lies two decades below the 1 kHz test tone;
// the 5% frame-to-frame tolerance remains valid.
func TestLowpassSeamContinuity(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	// Frame 1
	samples1, state1, fs1 := RenderSamples(
		nil, nSamples,
		cpuClock, sampleRate,
		+1, amplitude, alpha, filterState{},
		0, 0,
	)
	// Frame 2 continues from frame 1's state
	samples2, _, _ := RenderSamples(
		nil, nSamples,
		cpuClock, sampleRate,
		state1, amplitude, alpha, fs1,
		17050, uint64(nSamples),
	)
	// Last sample of frame 1 and first sample of frame 2 should be close
	last1 := float32(samples1[nSamples-1])
	first2 := float32(samples2[0])
	diff := last1 - first2
	if diff < 0 {
		diff = -diff
	}
	// Tolerance: within 5% of amplitude
	tol := float32(amplitude) * 0.05
	if diff > tol {
		t.Errorf("seam discontinuity too large: last1=%f first2=%f diff=%f tol=%f", last1, first2, diff, tol)
	}
}

// 8. cycles_to_samples_math: verify accumulator math produces 735 samples for first frame.
func TestCyclesToSamplesMath(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	frameCycles := uint64(17052) // typical first frame

	target := frameCycles * uint64(sampleRate) / uint64(cpuClock)
	nSamples := int(target - 0) // samplesEmitted starts at 0

	if nSamples != 735 {
		t.Errorf("expected 735 samples for first frame with 17052 cycles, got %d", nSamples)
	}
}

// 9. volume_zero_is_silent: amplitude=0 -> all samples == 0.
func TestVolumeZeroIsSilent(t *testing.T) {
	toggles := []uint64{100, 500, 1000, 5000}
	samples, _, _ := RenderSamples(
		toggles, 735,
		1023000, 44100,
		+1, 0, 0.15, filterState{},
		0, 0,
	)
	for i, s := range samples {
		if s != 0 {
			t.Errorf("expected silence with amplitude=0, got samples[%d]=%d", i, s)
		}
	}
}

// 10. int16_clipping_at_extremes: HPF output exceeding int16 range must be clamped.
// alpha=0 -> lpState is rewritten to x each sample (lowpass passes x unchanged to HPF).
func TestInt16ClippingAtExtremes(t *testing.T) {
	// Positive branch:
	// state=+1, amp=6000: x[0]=+6000; seed xPrev=0, yPrev=+40000.
	// y[0] = 6000 - 0 + 0.995*40000 = 45800 -> clamps to +32767.
	fs := filterState{lpState: 0, xPrev: 0, yPrev: 40000}
	samples, _, _ := RenderSamples(
		nil, 1,
		1023000, 44100,
		+1, int16(6000), 0.0, fs,
		0, 0,
	)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0] != 32767 {
		t.Errorf("expected clamped sample 32767, got %d (positive clamp branch not triggered?)", samples[0])
	}

	// Negative branch:
	// state=-1, amp=6000: x[0] = -6000; seed xPrev=0, yPrev=-40000.
	// With alpha=0 the lowpass passes x unchanged to HPF, so:
	// y[0] = x[0] - xPrev + R*yPrev = -6000 - 0 + 0.995*(-40000) = -45800
	// which must clamp to -32768.
	fsNeg := filterState{lpState: 0, xPrev: 0, yPrev: -40000}
	samplesNeg, _, _ := RenderSamples(
		nil, 1,
		1023000, 44100,
		-1, int16(6000), 0.0, fsNeg,
		0, 0,
	)
	if samplesNeg[0] != -32768 {
		t.Errorf("expected clamped sample -32768, got %d (negative clamp branch not triggered?)", samplesNeg[0])
	}
}

// 11. multi_frame_deterministic_count: 60 frames with jitter sum to exactly 44100 samples.
func TestMultiFrameDeterministicCount(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000

	// Pattern: [17050, 17051, 17049, 17050] x 15 = 60 frames
	// Sum: (17050+17051+17049+17050)*15 = 68200*15 = 1023000 cycles exactly
	pattern := []uint64{17050, 17051, 17049, 17050}

	var totalCycles uint64
	var samplesEmitted uint64
	var totalSamples int

	for rep := 0; rep < 15; rep++ {
		for _, frameCycles := range pattern {
			totalCycles += frameCycles
			target := totalCycles * uint64(sampleRate) / uint64(cpuClock)
			n := int(target - samplesEmitted)
			totalSamples += n
			samplesEmitted = target
		}
	}

	if totalSamples != 44100 {
		t.Errorf("expected exactly 44100 samples across 60 frames, got %d", totalSamples)
	}

	// Also verify per-frame counts are within {734, 735, 736}
	totalCycles = 0
	samplesEmitted = 0
	for rep := 0; rep < 15; rep++ {
		for _, frameCycles := range pattern {
			totalCycles += frameCycles
			target := totalCycles * uint64(sampleRate) / uint64(cpuClock)
			n := int(target - samplesEmitted)
			if n < 734 || n > 736 {
				t.Errorf("per-frame sample count %d outside [734,736]", n)
			}
			samplesEmitted = target
		}
	}
}

// 12. alpha_plumbing: two different alpha values produce visibly different rolloffs.
// Higher alpha means faster rise toward amplitude -> larger peak magnitude.
func TestAlphaPlumbing(t *testing.T) {
	nSamples := 100
	amplitude := int16(6000)

	samplesLow, _, _ := RenderSamples(
		nil, nSamples,
		1023000, 44100,
		+1, amplitude, 0.05, filterState{},
		0, 0,
	)
	samplesHigh, _, _ := RenderSamples(
		nil, nSamples,
		1023000, 44100,
		+1, amplitude, 0.5, filterState{},
		0, 0,
	)

	peakLow := maxAbs(samplesLow)
	peakHigh := maxAbs(samplesHigh)
	if peakHigh <= peakLow {
		t.Errorf("expected higher alpha to produce larger peak: high=%d low=%d",
			peakHigh, peakLow)
	}
}

// 13. monotone_toggles_documented: equal cycle toggles cancel parity correctly.
func TestMonotoneTogglesEqualCycles(t *testing.T) {
	// Two toggles at the same cycle = net state unchanged (even count).
	toggles := []uint64{500, 500}
	_, newState, _ := RenderSamples(
		toggles, 735,
		1023000, 44100,
		+1, 6000, 0.15, filterState{},
		0, 0,
	)
	if newState != +1 {
		t.Errorf("two equal-cycle toggles should cancel; expected +1, got %d", newState)
	}

	// Three equal-cycle toggles = odd count -> flipped.
	toggles3 := []uint64{500, 500, 500}
	_, newState3, _ := RenderSamples(
		toggles3, 735,
		1023000, 44100,
		+1, 6000, 0.15, filterState{},
		0, 0,
	)
	if newState3 != -1 {
		t.Errorf("three equal-cycle toggles should flip; expected -1, got %d", newState3)
	}
}

// 14. cycle_stamp_ordering: Toggle reads *cyclePtr at call time, not after Step.
// This locks in the invariant that $C030 accesses are timestamped at the
// instruction's start cycle, as established by main.go's loop ordering.
func TestCycleStampOrdering(t *testing.T) {
	// Wire a local uint64 as the cycle pointer — same pattern as main.go's
	// &frameCycle that is passed to speaker.New.
	var cycle uint64
	s := &Speaker{
		cyclePtr:      &cycle,
		enabled:       true,
		sampleRate:    44100,
		cpuClock:      1023000,
		bytesPerFrame: uint32(735 * 2),
		amplitude:     6000,
		alpha:         0.15,
		state:         +1,
		fs:            filterState{},
	}

	// First toggle: set cycle to 1000, call Toggle via Device.Read
	cycle = 1000
	dev := NewDevice(s)
	dev.Read(0xC030)

	if len(s.toggles) != 1 {
		t.Fatalf("expected 1 toggle recorded, got %d", len(s.toggles))
	}
	if s.toggles[0] != 1000 {
		t.Errorf("expected toggle stamped at cycle 1000, got %d", s.toggles[0])
	}

	// Second toggle: set cycle to 2500, call Toggle via Device.Write
	cycle = 2500
	dev.Write(0xC030, 0)

	if len(s.toggles) != 2 {
		t.Fatalf("expected 2 toggles recorded, got %d", len(s.toggles))
	}
	if s.toggles[1] != 2500 {
		t.Errorf("expected second toggle stamped at cycle 2500, got %d", s.toggles[1])
	}

	// Verify toggles are consumed in order by EndFrame.
	// EndFrame with 5000 cycles should see both toggles at 1000 and 2500.
	// We can't call real EndFrame (needs SDL), so verify the toggle slice ordering
	// directly: must be monotone non-decreasing as required by RenderSamples.
	if s.toggles[0] > s.toggles[1] {
		t.Errorf("toggles not in non-decreasing order: [%d, %d]", s.toggles[0], s.toggles[1])
	}

	// Simulate what EndFrame does with toggles: pass them to RenderSamples
	// and verify both are applied. With 2 toggles from state=+1, final state = +1.
	frameCycles := uint64(5000)
	nSamples := int(frameCycles * 44100 / 1023000)
	if nSamples < 1 {
		nSamples = 1
	}
	_, newState, _ := RenderSamples(
		s.toggles, nSamples,
		1023000, 44100,
		+1, 6000, 0.15, filterState{},
		0, 0,
	)
	// Two toggles: net effect = back to +1 (even count).
	if newState != +1 {
		t.Errorf("expected state +1 after two ordered toggles, got %d", newState)
	}
}

// 15. TestDCBlockerRemovesBias: after the DC blocker, an idle-state stream
// (no toggles, constant state=+1) must decay toward zero rather than sit at +amp.
// Simulates 60 frames (~1 s at 44.1 kHz) and verifies the mean magnitude of the
// final frame is near zero.
//
// Analytic expectation: after 60*735 = 44100 samples, |y| <= amp * R^44100.
// At R=0.995 that is 6000 * 0.995^44100 ~= 6000 * e^-221, which is 0 in
// float32. Allow 10 LSB of headroom for arithmetic noise.
func TestDCBlockerRemovesBias(t *testing.T) {
	sampleRate := 44100
	cpuClock := 1023000
	nSamples := 735
	amplitude := int16(6000)
	alpha := float32(0.15)

	state := int8(+1)
	fs := filterState{}
	var lastFrame []int16

	for frame := 0; frame < 60; frame++ {
		var samples []int16
		samples, state, fs = RenderSamples(
			nil, nSamples,
			cpuClock, sampleRate,
			state, amplitude, alpha, fs,
			uint64(frame)*17050, uint64(frame*nSamples),
		)
		lastFrame = samples
	}

	var sum int64
	for _, s := range lastFrame {
		v := int64(s)
		if v < 0 {
			v = -v
		}
		sum += v
	}
	meanAbs := sum / int64(len(lastFrame))
	// Expected ~= 0; 10 LSB catches regressions an order of magnitude
	// earlier than a looser bound would.
	if meanAbs > 10 {
		t.Errorf("expected DC-blocked idle mean|s| <= 10, got %d", meanAbs)
	}
}

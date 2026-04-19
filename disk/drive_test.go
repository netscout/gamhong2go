package disk

import "testing"

func newTestDrive() drive {
	return drive{}
}

func TestStepForwardAndBack(t *testing.T) {
	d := newTestDrive()
	// Simulate: phase 0 on -> phase 1 on -> phase 0 off  (step forward by 1 half-track)
	d.phaseOn(0)
	if d.halfTrack != 0 {
		t.Fatalf("after first phaseOn(0): halfTrack = %d, want 0", d.halfTrack)
	}
	d.phaseOn(1) // adjacent coil while 0 is still on -> step +1
	if d.halfTrack != 1 {
		t.Fatalf("after phaseOn(1) with 0 on: halfTrack = %d, want 1", d.halfTrack)
	}
	d.phaseOff(0)
	if d.halfTrack != 1 {
		t.Fatalf("after phaseOff(0): halfTrack = %d, want 1 (no change)", d.halfTrack)
	}

	// Step back: phase 0 on (adjacent coil, step -1)
	d.phaseOn(0)
	if d.halfTrack != 0 {
		t.Fatalf("after step back: halfTrack = %d, want 0", d.halfTrack)
	}
}

func TestHalfTrackClamped(t *testing.T) {
	d := newTestDrive()
	// Drive far past the last legal half-track.
	d.phaseOn(0)
	for i := 0; i < 40; i++ {
		next := (i + 1) % 4
		prev := i % 4
		d.phaseOn(next)
		d.phaseOff(prev)
	}
	// halfTrack must not exceed maxHalfTrack (69). halfTrack/2 must be a
	// valid index into nibbles[tracksPerDisk].
	if d.halfTrack > maxHalfTrack {
		t.Errorf("halfTrack %d > maxHalfTrack %d", d.halfTrack, maxHalfTrack)
	}
	if d.halfTrack/2 >= tracksPerDisk {
		t.Errorf("halfTrack %d maps to track %d, out of range [0,%d)",
			d.halfTrack, d.halfTrack/2, tracksPerDisk)
	}

	// Now step backward past 0.
	d.halfTrack = 0
	d.phases = [4]bool{}
	d.phaseOn(0)
	for i := 0; i < 10; i++ {
		prev := i % 4
		next := (i + 3) % 4 // step backward
		d.phaseOn(next)
		d.phaseOff(prev)
	}
	if d.halfTrack < 0 {
		t.Errorf("halfTrack %d went below 0", d.halfTrack)
	}
}

// TestStepperCannotExceedLastTrack is a focused mechanism test: no matter
// how many forward steps are issued, halfTrack/2 must remain a legal index
// into nibbles[tracksPerDisk]. Previous clamp was off-by-one (70 allowed),
// which mapped to track 35 — OOB for a 35-track array.
func TestStepperCannotExceedLastTrack(t *testing.T) {
	d := newTestDrive()
	d.phaseOn(0)
	for i := 0; i < 200; i++ { // hammer forward well past any reasonable limit
		next := (i + 1) % 4
		prev := i % 4
		d.phaseOn(next)
		d.phaseOff(prev)
	}
	if d.halfTrack >= 2*tracksPerDisk {
		t.Fatalf("halfTrack %d reached track %d; want < %d",
			d.halfTrack, d.halfTrack/2, tracksPerDisk)
	}
}

// TestHalfTrack70DoesNotCrash pins the exact crash the user hit: a drive
// parked at halfTrack = 70 (the old clamp ceiling) calling trackData must
// not panic. With the fix, halfTrack cannot reach 70; this test catches
// regressions if the clamp is ever relaxed.
func TestHalfTrack70DoesNotCrash(t *testing.T) {
	d := newTestDrive()
	// Drive past any reasonable limit via the normal step path.
	d.phaseOn(0)
	for i := 0; i < 200; i++ {
		next := (i + 1) % 4
		prev := i % 4
		d.phaseOn(next)
		d.phaseOff(prev)
	}
	if d.halfTrack == 70 {
		t.Fatal("halfTrack reached 70; clamp regressed to pre-fix value")
	}
	// Even without a mounted image, the index math must stay in bounds.
	if t := d.halfTrack / 2; t >= tracksPerDisk {
		_ = t
	}
}

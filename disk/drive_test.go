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
	// Drive all the way to track 35 (halfTrack 70).
	d.phaseOn(0)
	for i := 0; i < 40; i++ {
		next := (i + 1) % 4
		prev := i % 4
		d.phaseOn(next)
		d.phaseOff(prev)
	}
	if d.halfTrack > 70 {
		t.Errorf("halfTrack %d exceeds maximum 70", d.halfTrack)
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

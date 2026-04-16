package disk

import (
	"fmt"
	"io"
)

// Tracer is an opt-in interface for disk activity tracing. Wire a non-nil
// Tracer via Controller.SetTracer to enable logging; nil means silent.
// Unit tests can capture events without touching stderr by providing a
// test-local Tracer implementation.
type Tracer interface {
	// TraceNibbleRead is called on every $C0EC read that actually advances
	// the latch (steps > 0). cycle is the current diskCycle value, drive is
	// 0 or 1, halfTrack is the current half-track, nibblePos is the position
	// in the nibble buffer, and value is the byte returned to the CPU.
	TraceNibbleRead(cycle uint64, drive, halfTrack, nibblePos int, value uint8)

	// TracePhaseStrobe is called for every $C0E0-$C0E7 access (phase on/off).
	// phase is 0..3, on is true for PHASEnON.  oldHT and newHT are the
	// half-track before and after the strobe.
	TracePhaseStrobe(cycle uint64, drive, phase int, on bool, oldHT, newHT int, phases [4]bool)

	// TraceModeChange is called when Q6 or Q7 changes, and on motor on/off.
	TraceModeChange(cycle uint64, drive int, event string)

	// TraceAddressField is called when a full D5 AA 96 address field is
	// successfully decoded from the nibble stream.
	TraceAddressField(cycle uint64, drive, halfTrack, nibblePos int, vol, track, sector uint8)
}

// stderrTracer is a simple Tracer that writes to an io.Writer (stderr by default).
// It rate-limits nibble-read events to at most maxNibbles entries.
type stderrTracer struct {
	w          io.Writer
	maxNibbles int
	nibCount   int
}

// NewStderrTracer returns a Tracer that writes human-readable lines to w.
// maxNibbles caps the number of TraceNibbleRead lines emitted (0 = unlimited).
func NewStderrTracer(w io.Writer, maxNibbles int) Tracer {
	return &stderrTracer{w: w, maxNibbles: maxNibbles}
}

func (t *stderrTracer) TraceNibbleRead(cycle uint64, drive, halfTrack, nibblePos int, value uint8) {
	if t.maxNibbles > 0 && t.nibCount >= t.maxNibbles {
		return
	}
	t.nibCount++
	fmt.Fprintf(t.w, "DISK NIB  cycle=%-10d drv=%d ht=%2d pos=%4d val=%02X\n",
		cycle, drive+1, halfTrack, nibblePos, value)
}

func (t *stderrTracer) TracePhaseStrobe(cycle uint64, drive, phase int, on bool, oldHT, newHT int, phases [4]bool) {
	dir := "OFF"
	if on {
		dir = "ON "
	}
	// Encode phases as a 4-bit integer for %04b formatting.
	var phaseBits uint8
	for i, v := range phases {
		if v {
			phaseBits |= 1 << uint(i)
		}
	}
	fmt.Fprintf(t.w, "DISK PHS  cycle=%-10d drv=%d phase=%d %s  ht %2d->%2d  phases=%04b\n",
		cycle, drive+1, phase, dir, oldHT, newHT, phaseBits)
}

func (t *stderrTracer) TraceModeChange(cycle uint64, drive int, event string) {
	fmt.Fprintf(t.w, "DISK MODE cycle=%-10d drv=%d %s\n", cycle, drive+1, event)
}

func (t *stderrTracer) TraceAddressField(cycle uint64, drive, halfTrack, nibblePos int, vol, track, sector uint8) {
	fmt.Fprintf(t.w, "DISK ADDR cycle=%-10d drv=%d ht=%2d pos=%4d vol=%d trk=%d sec=%d\n",
		cycle, drive+1, halfTrack, nibblePos, vol, track, sector)
}

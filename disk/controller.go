// Package disk emulates the Apple Disk II controller card in slot 6.
//
// Threading model: SINGLE GOROUTINE. Same assumption as speaker.go;
// see that file's header for the full argument. All Read/Write calls
// arrive on the main goroutine via the bus. flush-on-Close also runs on
// the main goroutine. No mutexes are required.
package disk

import (
	"fmt"
	"os"
	"time"
)

const cyclesPerNibble uint64 = 32

// numDrives is the number of drive slots a Disk II controller card has.
// Used for bounds-checking public drive-index APIs.
const numDrives = 2

// nowFn is an injection seam so tests can advance "wall clock" without
// real time.Sleep calls. Production code leaves this pointing at time.Now.
var nowFn = time.Now

// Controller emulates the Disk II controller card.
type Controller struct {
	drives   [2]drive
	selected int // 0 or 1

	q6, q7    bool
	latch     uint8  // data register value exposed at $C0EC
	writeReg  uint8  // latched by $C0ED in write mode
	lastCycle uint64 // *cyclePtr at last latch advance

	cyclePtr *uint64 // injected from main; must be monotonically increasing
	tracer   Tracer  // nil = no tracing
}

// NewController creates a Controller. cyclePtr must point to a monotonically
// increasing CPU cycle counter (NOT a frame-relative counter that resets each
// frame). Use a separate diskCycle variable in main.go; do NOT share the
// frame-relative frameCycle variable used by the speaker, because that resets
// to 0 each frame and would cause uint64 underflow in the delta calculation.
func NewController(cyclePtr *uint64) *Controller {
	return &Controller{cyclePtr: cyclePtr}
}

// SetTracer wires an optional Tracer for disk activity logging. Pass nil to
// disable tracing (the default). The tracer is called from the same goroutine
// as Read/Write, so no locking is needed.
func (c *Controller) SetTracer(t Tracer) {
	c.tracer = t
}

// Mount loads a disk image into the specified drive slot (0 or 1).
// orderOverride is "" to infer from extension, or "dos"/"prodos".
func (c *Controller) Mount(driveIdx int, path, orderOverride string) error {
	if driveIdx < 0 || driveIdx > 1 {
		return fmt.Errorf("disk: invalid drive index %d", driveIdx)
	}
	img, err := LoadImage(path, orderOverride)
	if err != nil {
		return err
	}
	c.drives[driveIdx].image = img
	c.drives[driveIdx].writeProt = img.writeProt
	fmt.Fprintf(os.Stderr, "disk: drive %d: %s (order=%v, wp=%v)\n", driveIdx+1, path, img.order, img.writeProt)
	return nil
}

// Close flushes all dirty tracks and releases resources.
func (c *Controller) Close() {
	c.flushAll()
}

// flushAll attempts to flush dirty tracks for both drives.
func (c *Controller) flushAll() {
	flushed := map[*diskImage]bool{}
	for di := range c.drives {
		img := c.drives[di].image
		if img == nil {
			continue
		}
		if flushed[img] {
			continue
		}
		flushed[img] = true
		if err := img.flush(&c.drives); err != nil {
			fmt.Fprintf(os.Stderr, "disk: close: flush drive %d: %v\n", di+1, err)
		}
	}
}

// recentActivityWindow is the wall-clock lookback window for activity
// indicators. 500 ms is well above the perceptual threshold for a single
// blink (~100 ms) and short enough that a cleanly-finished read fades
// quickly. Wall-clock so the indicator stays correct under turbo (where
// emulated cycles pass much faster than wall-clock cycles).
const recentActivityWindow = 500 * time.Millisecond

// HadRecentRead returns true if drive driveIdx (0 or 1) has read a nibble
// within the last 500 ms of wall-clock time. Safe to call from the main
// goroutine; no locking required (single-goroutine emulator).
func (c *Controller) HadRecentRead(driveIdx int) bool {
	if driveIdx < 0 || driveIdx >= numDrives {
		return false
	}
	last := c.drives[driveIdx].lastReadAt
	if last.IsZero() {
		return false
	}
	return nowFn().Sub(last) < recentActivityWindow
}

// HadRecentWrite returns true if drive driveIdx (0 or 1) has written a
// nibble within the last 500 ms of wall-clock time.
func (c *Controller) HadRecentWrite(driveIdx int) bool {
	if driveIdx < 0 || driveIdx >= numDrives {
		return false
	}
	last := c.drives[driveIdx].lastWriteAt
	if last.IsZero() {
		return false
	}
	return nowFn().Sub(last) < recentActivityWindow
}

// HasDisk returns true if the given drive slot has a mounted disk. Used
// by the window-title indicator to decide whether to show a slot at all.
func (c *Controller) HasDisk(driveIdx int) bool {
	if driveIdx < 0 || driveIdx >= numDrives {
		return false
	}
	return c.drives[driveIdx].image != nil
}

// Strobe handles a softswitch access at addr in [$C0E0..$C0EF].
// Returns the data-register value for read accesses.
func (c *Controller) Strobe(addr uint16) uint8 {
	offset := addr & 0x0F // 0..15

	switch offset {
	case 0x00: // PHASE0OFF
		c.tracePhase(0, false)
		c.drives[c.selected].phaseOff(0)
	case 0x01: // PHASE0ON
		c.tracePhase(0, true)
		c.drives[c.selected].phaseOn(0)
	case 0x02: // PHASE1OFF
		c.tracePhase(1, false)
		c.drives[c.selected].phaseOff(1)
	case 0x03: // PHASE1ON
		c.tracePhase(1, true)
		c.drives[c.selected].phaseOn(1)
	case 0x04: // PHASE2OFF
		c.tracePhase(2, false)
		c.drives[c.selected].phaseOff(2)
	case 0x05: // PHASE2ON
		c.tracePhase(2, true)
		c.drives[c.selected].phaseOn(2)
	case 0x06: // PHASE3OFF
		c.tracePhase(3, false)
		c.drives[c.selected].phaseOff(3)
	case 0x07: // PHASE3ON
		c.tracePhase(3, true)
		c.drives[c.selected].phaseOn(3)

	case 0x08: // MOTOROFF
		c.drives[c.selected].motorOn = false
		if c.tracer != nil {
			c.tracer.TraceModeChange(*c.cyclePtr, c.selected, "MOTOROFF")
		}
		// Flush dirty tracks on motor-off to bound data loss.
		c.flushAll()

	case 0x09: // MOTORON
		c.drives[c.selected].motorOn = true
		if c.tracer != nil {
			c.tracer.TraceModeChange(*c.cyclePtr, c.selected, "MOTORON")
		}

	case 0x0A: // DRIVE1 -- re-route subsequent strobes; do NOT change motor state
		c.selected = 0

	case 0x0B: // DRIVE2
		c.selected = 1

	case 0x0C: // Q6L -- data/status register access
		return c.q6LAccess()

	case 0x0D: // Q6H -- latch write register / enter read-status mode
		oldQ6 := c.q6
		c.q6 = true
		if c.tracer != nil && !oldQ6 {
			c.tracer.TraceModeChange(*c.cyclePtr, c.selected, "Q6=1 (status/write-load)")
		}
		// In write-load mode (Q7=1, Q6=1), latch the write register.
		if c.q7 {
			// write register latch: write handled by Q6H write path
		}
		// Return 0 below (Go switch cases do not fall through by default).

	case 0x0E: // Q7L -- enter read mode
		oldQ7 := c.q7
		c.q7 = false
		if c.tracer != nil && oldQ7 {
			c.tracer.TraceModeChange(*c.cyclePtr, c.selected, "Q7=0 (read side)")
		}

	case 0x0F: // Q7H -- enter write mode
		oldQ7 := c.q7
		c.q7 = true
		if c.tracer != nil && !oldQ7 {
			c.tracer.TraceModeChange(*c.cyclePtr, c.selected, "Q7=1 (write side)")
		}
	}
	return 0
}

// q6LAccess implements the $C0EC data-register access.
// Behaviour depends on the Q7/Q6 mode:
//
//   Q7=0, Q6=0  READ data   advance nibble stream with cycle pacing
//   Q7=0, Q6=1  READ status return WP in bit 7 (motor-on gated)
//   Q7=1, Q6=0  WRITE load  (load mode active; Q6L returns current latch)
//   Q7=1, Q6=1  WRITE run   clock writeReg into nibble stream
func (c *Controller) q6LAccess() uint8 {
	d := &c.drives[c.selected]

	if !c.q7 && c.q6 {
		// READ status: return WP in bit 7, motor-on gated.
		// Bit-7 pacing rule for BPL *-3 does not apply here.
		if d.motorOn && d.writeProt {
			return 0x80
		}
		return 0x00
	}

	if c.q7 && c.q6 {
		// WRITE run: clock writeReg into current nibble position.
		if d.motorOn && !d.writeProt && d.image != nil {
			track := d.halfTrack / 2
			nibs := d.trackData(d.image.order)
			if len(nibs) > 0 {
				d.nibbles[track][d.nibblePos] = c.writeReg
				d.dirty[track] = true
				d.nibblePos = (d.nibblePos + 1) % len(nibs)
			}
			now := *c.cyclePtr
			c.lastCycle = now
			d.lastWriteAt = nowFn()
		}
		return c.latch
	}

	// READ data (Q7=0, Q6=0) or WRITE load (Q7=1, Q6=0):
	// Both return the current latch, but only READ data advances the nibble stream.
	if !c.q7 && !c.q6 {
		return c.readDataLatch(d)
	}
	// Q7=1, Q6=0 (WRITE load): just return current latch without advancing.
	return c.latch
}

// readDataLatch implements the cycle-paced nibble advance for READ data mode.
// Bit-7 pacing rule: if not enough cycles have elapsed, return latch & 0x7F so
// RWTS's BPL *-3 wait-for-byte loop sees bit 7 = 0 ("not ready").
func (c *Controller) readDataLatch(d *drive) uint8 {
	if !d.motorOn || d.image == nil {
		// No disk / motor off: return $FF-like noise (harmless, not $00 which could be valid data)
		return 0xFF
	}

	nibs := d.trackData(d.image.order)
	if len(nibs) == 0 {
		return 0xFF
	}

	now := *c.cyclePtr
	delta := now - c.lastCycle
	steps := delta / cyclesPerNibble
	rem := delta - steps*cyclesPerNibble

	if steps == 0 {
		// Not enough time elapsed for a new nibble.
		// RWTS's BPL *-3 wait-for-byte loop relies on seeing bit 7 = 0 here.
		return c.latch & 0x7F
	}

	// Clamp steps to avoid an enormous advance when the cycle pointer wraps
	// or jumps (e.g. emulator startup). One full revolution is enough.
	nLen := len(nibs)
	if int(steps) > nLen {
		// Jump is larger than one revolution; just keep relative position.
		steps = steps % uint64(nLen)
	}

	d.nibblePos = (d.nibblePos + int(steps)) % nLen
	c.latch = nibs[d.nibblePos]
	c.lastCycle = now - rem // preserve sub-nibble phase
	d.lastReadAt = nowFn()

	// Trace: emit nibble read event (rate-limited by the Tracer implementation).
	if c.tracer != nil {
		c.tracer.TraceNibbleRead(now, c.selected, d.halfTrack, d.nibblePos, c.latch)
	}

	// Detect address field prologues (D5 AA 96) in the nibble stream for tracing.
	if c.tracer != nil {
		c.checkAddressFieldTrace(d, nibs)
	}

	return c.latch
}

// checkAddressFieldTrace scans for a completed D5 AA 96 address prologue ending
// at the current nibblePos and emits a TraceAddressField event if found.
func (c *Controller) checkAddressFieldTrace(d *drive, nibs []uint8) {
	n := len(nibs)
	pos := d.nibblePos
	// Check if the current pos is the '96' of a D5 AA 96 sequence.
	at := func(offset int) uint8 { return nibs[(pos+offset+n)%n] }
	if at(-2) == 0xD5 && at(-1) == 0xAA && at(0) == 0x96 {
		// Read the 4 odd/even pairs of the address field (8 bytes).
		vol := decodeOddEven(at(1), at(2))
		track := decodeOddEven(at(3), at(4))
		sector := decodeOddEven(at(5), at(6))
		chk := decodeOddEven(at(7), at(8))
		if chk == vol^track^sector {
			c.tracer.TraceAddressField(*c.cyclePtr, c.selected, d.halfTrack, pos, vol, track, sector)
		}
	}
}

// Q6WriteReg latches v into the write register (called on $C0ED write in Q7=1 mode).
func (c *Controller) Q6WriteReg(v uint8) {
	if c.q7 {
		c.writeReg = v
	}
}

// tracePhase emits a phase strobe trace event (called before the phase change).
func (c *Controller) tracePhase(phase int, on bool) {
	if c.tracer == nil {
		return
	}
	d := &c.drives[c.selected]
	oldHT := d.halfTrack
	// The actual phase change happens after this call, so newHT is estimated
	// by computing what phaseOn/phaseOff would do.
	newHT := oldHT
	if on {
		prev := d.activePhase()
		if prev >= 0 && prev != phase {
			diff := (phase - prev + 4) % 4
			switch diff {
			case 1:
				newHT = oldHT + 1
				if newHT > 70 {
					newHT = 70
				}
			case 3:
				newHT = oldHT - 1
				if newHT < 0 {
					newHT = 0
				}
			}
		}
	}
	var phaseBits [4]bool
	copy(phaseBits[:], d.phases[:])
	if on {
		phaseBits[phase] = true
	} else {
		phaseBits[phase] = false
	}
	c.tracer.TracePhaseStrobe(*c.cyclePtr, c.selected, phase, on, oldHT, newHT, phaseBits)
}

// traceAddressField scans a recently-loaded latch sequence for D5 AA 96
// and emits a TraceAddressField event if the address field is complete.
// Called internally by readDataLatch after a successful sequence detection.
func (c *Controller) traceAddressField(d *drive, nibs []uint8, pos int, vol, track, sector uint8) {
	if c.tracer == nil {
		return
	}
	c.tracer.TraceAddressField(*c.cyclePtr, c.selected, d.halfTrack, pos,
		vol, track, sector)
}

package disk

// drive holds the per-drive mechanical and magnetic state.
type drive struct {
	image     *diskImage
	phases    [4]bool
	halfTrack int // 0..70; track = halfTrack/2
	motorOn   bool
	writeProt bool
	nibbles   [tracksPerDisk][]uint8 // lazily GCR-encoded, one per track
	nibblePos int
	dirty     [tracksPerDisk]bool
}

// stepPhase updates the half-track position when a stepper phase changes.
// Called whenever a PHASEnON strobe arrives for this drive.
// prev is the phase that was on before; next is the phase being turned on.
func (d *drive) stepPhase(prev, next int) {
	// Determine direction from prev -> next (mod 4).
	diff := (next - prev + 4) % 4
	switch diff {
	case 1:
		d.halfTrack++
	case 3:
		d.halfTrack--
	// diff == 2 means opposite coil; no clean step, ignore.
	}
	if d.halfTrack < 0 {
		d.halfTrack = 0
	}
	if d.halfTrack > 70 {
		d.halfTrack = 70
	}
}

// activePhase returns the index (0..3) of the single active phase, or -1 if
// none or more than one are on.
func (d *drive) activePhase() int {
	found := -1
	for i, on := range d.phases {
		if on {
			if found >= 0 {
				return -1 // multiple phases
			}
			found = i
		}
	}
	return found
}

// phaseOn handles a PHASEnON strobe. It energizes the coil and steps the
// head if an adjacent coil was already energised.
func (d *drive) phaseOn(phase int) {
	prev := d.activePhase()
	d.phases[phase] = true
	if prev >= 0 && prev != phase {
		d.stepPhase(prev, phase)
	}
}

// phaseOff de-energises a coil.
func (d *drive) phaseOff(phase int) {
	d.phases[phase] = false
}

// trackData returns the nibble buffer for the current track, encoding it
// lazily if needed. Returns nil if no image is mounted.
func (d *drive) trackData(order SectorOrder) []uint8 {
	if d.image == nil {
		return nil
	}
	t := d.halfTrack / 2
	if d.nibbles[t] == nil {
		sectors := d.image.trackSectors(t, order)
		d.nibbles[t] = EncodeTrack(sectors, 254, t, order)
	}
	return d.nibbles[t]
}

package disk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const imageSize = tracksPerDisk * sectorsPerTrack * bytesPerSector // 143360

// decodedTrack holds the result of decoding a single dirty track's nibbles.
type decodedTrack struct {
	track   int
	sectors [sectorsPerTrack][bytesPerSector]uint8
	ok      bool
}

// diskImage holds the raw sector data for a 35-track, 16-sector, 256-byte/sector disk.
type diskImage struct {
	path      string
	order     SectorOrder
	writeProt bool
	// data[track][physicalSector][byte]  in the on-disk physical sector order
	// (before interleave reversal — we store them in logical order for simplicity)
	data [tracksPerDisk][sectorsPerTrack][bytesPerSector]uint8
}

// LoadImage opens and validates a disk image file. order overrides the
// inferred sector order when non-empty ("dos" or "prodos"); pass "" to infer.
func LoadImage(path, orderOverride string) (*diskImage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("disk image %s: %w", path, err)
	}
	if len(raw) != imageSize {
		return nil, fmt.Errorf("disk image %s: size %d != %d", path, len(raw), imageSize)
	}

	order := inferOrder(path)
	switch strings.ToLower(orderOverride) {
	case "dos":
		order = OrderDOS
	case "prodos":
		order = OrderProDOS
	case "":
		// keep inferred
	default:
		fmt.Fprintf(os.Stderr, "disk: unknown -order %q; using inferred order\n", orderOverride)
	}

	// Determine write-protect from file permissions.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("disk image %s: %w", path, err)
	}
	wp := info.Mode()&0200 == 0 // write bit clear -> write-protected

	img := &diskImage{path: path, order: order, writeProt: wp}

	// Load raw bytes into [track][sector][byte] in file order.
	// The file stores sectors in the order chosen by `order`:
	//   DOS order (.dsk/.do): file position = track*16*256 + physSec*256
	//   ProDOS order (.po): same physical layout but different interleave meaning.
	// We keep the file in "logical" order (the order the OS sees), so we just
	// split linearly.
	offset := 0
	for t := 0; t < tracksPerDisk; t++ {
		for s := 0; s < sectorsPerTrack; s++ {
			copy(img.data[t][s][:], raw[offset:offset+bytesPerSector])
			offset += bytesPerSector
		}
	}

	return img, nil
}

// inferOrder guesses the sector order from the file extension.
func inferOrder(path string) SectorOrder {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".po":
		return OrderProDOS
	case ".dsk", ".do":
		return OrderDOS
	default:
		fmt.Fprintf(os.Stderr, "disk: unknown extension %q, defaulting to DOS order (use -order to override)\n",
			filepath.Ext(path))
		return OrderDOS
	}
}

// trackSectors returns the 16 sectors of a track in logical order, ready for
// EncodeTrack. The caller owns the returned array.
func (img *diskImage) trackSectors(track int, order SectorOrder) [sectorsPerTrack][bytesPerSector]uint8 {
	return img.data[track]
}

// flush writes all dirty tracks back to the image file atomically.
// It decodes the nibble buffers from the drive state and reassembles the image.
// If any track fails to decode, the original file is left intact and a .corrupt
// sidecar is written instead.
func (img *diskImage) flush(drives *[2]drive) error {
	// Collect dirty tracks from both drives.
	type dirtyEntry struct {
		driveIdx int
		track    int
	}
	var dirty []dirtyEntry
	for di := range drives {
		for t := 0; t < tracksPerDisk; t++ {
			if drives[di].dirty[t] && drives[di].image == img {
				dirty = append(dirty, dirtyEntry{di, t})
			}
		}
	}
	if len(dirty) == 0 {
		return nil
	}

	results := make([]decodedTrack, len(dirty))
	allOK := true
	for i, de := range dirty {
		d := &drives[de.driveIdx]
		nibs := d.nibbles[de.track]
		if nibs == nil {
			results[i] = decodedTrack{track: de.track, ok: true, sectors: img.data[de.track]}
			continue
		}
		sectors, err := DecodeTrack(nibs, img.order)
		if err != nil {
			fmt.Fprintf(os.Stderr, "disk: flush: track %d decode error: %v\n", de.track, err)
			results[i] = decodedTrack{track: de.track, ok: false}
			allOK = false
		} else {
			results[i] = decodedTrack{track: de.track, sectors: sectors, ok: true}
		}
	}

	if allOK {
		return img.writeImage(img.path, drives, results)
	}

	// Failure path: write .corrupt sidecar, leave original untouched.
	corruptPath := img.path + ".corrupt"
	if err := img.writeCorruptImage(corruptPath, drives, results); err != nil {
		fmt.Fprintf(os.Stderr, "disk: flush: could not write sidecar %s: %v\n", corruptPath, err)
	} else {
		fmt.Fprintf(os.Stderr, "disk: flush: decode failures on dirty tracks; original %s preserved; partial image at %s\n",
			img.path, corruptPath)
	}
	return fmt.Errorf("flush: decode failures on dirty tracks; see %s", corruptPath)
}

// writeImage assembles a full image using decoded results for dirty tracks and
// existing data for clean tracks, then atomically replaces dest.
func (img *diskImage) writeImage(dest string, drives *[2]drive, results []decodedTrack) error {
	// Build a combined set: start from current img.data, overlay decoded results.
	newData := img.data
	for _, r := range results {
		if r.ok {
			newData[r.track] = r.sectors
			// Update in-memory image data and clear dirty flags.
			img.data[r.track] = r.sectors
			for di := range drives {
				if drives[di].image == img {
					drives[di].dirty[r.track] = false
				}
			}
		}
	}
	return writeImageToFile(dest, newData)
}

// writeCorruptImage writes a sidecar using decoded data where available.
func (img *diskImage) writeCorruptImage(dest string, drives *[2]drive, results []decodedTrack) error {
	newData := img.data
	for _, r := range results {
		if r.ok {
			newData[r.track] = r.sectors
		}
	}
	return writeImageToFile(dest, newData)
}

// writeImageToFile serialises the sector array and does an atomic rename.
func writeImageToFile(dest string, data [tracksPerDisk][sectorsPerTrack][bytesPerSector]uint8) error {
	raw := make([]byte, imageSize)
	offset := 0
	for t := 0; t < tracksPerDisk; t++ {
		for s := 0; s < sectorsPerTrack; s++ {
			copy(raw[offset:], data[t][s][:])
			offset += bytesPerSector
		}
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, raw, 0666); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

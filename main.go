package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/apple2emu/bus"
	"github.com/apple2emu/cpu"
	"github.com/apple2emu/disk"
	appleio "github.com/apple2emu/io"
	"github.com/apple2emu/languageCard"
	"github.com/apple2emu/memory"
	"github.com/apple2emu/speaker"
	"github.com/apple2emu/video"
)

const (
	// Apple II runs at 1.023 MHz; display refreshes at ~60 Hz.
	//
	// cyclesPerFrame = cpuClock / targetFPS = 17050 is deliberately 20
	// cycles larger than the real NTSC video frame (262×65 = 17030, see
	// io/softswitches.go). Speaker math keys samplesPerFrame to
	// cpuClock/sampleRate = 735 exactly at 44.1 kHz; shrinking the CPU
	// slice to 17030 would drain SDL's queue ~58 samples/sec. Games
	// care about CPU clock accuracy (pitch, disk nibble timing) far
	// more than host-frame wall-clock length; the resulting ~4.2 s/hour
	// CPU-vs-video drift is imperceptible.
	cpuClock       = 1023000
	targetFPS      = 60
	cyclesPerFrame = cpuClock / targetFPS // 17050 — CPU slice per host frame; also drives speaker math.

	// Window size: 3× native resolution for visibility.
	windowScale = 3
	windowW     = video.ScreenW * windowScale // 840
	windowH     = video.ScreenH * windowScale // 576
)

func main() {
	romPath := flag.String("rom", "roms/Apple2_Plus.rom", "Path to Apple II ROM image (12 KB or 16 KB)")
	volume := flag.Float64("volume", 0.25, "Speaker volume in [0.0, 1.0] (0 = mute)")
	sampleRate := flag.Int("samplerate", 44100, "Audio sample rate in Hz")
	disk1 := flag.String("disk1", "", "Path to .dsk/.do/.po image for drive 1")
	disk2 := flag.String("disk2", "", "Path to .dsk/.do/.po image for drive 2 (optional)")
	order := flag.String("order", "", "Sector order override: dos | prodos (default: infer from extension)")
	diskTrace := flag.Bool("disktrace", false, "Log disk activity to stderr (rate-limited to 200 nibble reads)")
	diskTraceN := flag.Int("disktracen", 200, "Max nibble-read lines to emit when -disktrace is set (0 = unlimited)")
	flag.Parse()

	if *volume < 0 || *volume > 1 {
		fmt.Fprintf(os.Stderr, "warning: -volume %.3f out of [0,1], clamping\n", *volume)
		if *volume < 0 {
			*volume = 0
		}
		if *volume > 1 {
			*volume = 1
		}
	}

	// --- Load ROM -----------------------------------------------------------
	rom, err := loadROM(*romPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		fmt.Fprintln(os.Stderr, "Place an Apple II ROM in the roms/ directory:")
		fmt.Fprintln(os.Stderr, "  mkdir -p roms")
		fmt.Fprintln(os.Stderr, "  cp /path/to/Apple2_Plus.rom roms/")
		os.Exit(1)
	}

	// --- Wire the bus -------------------------------------------------------
	// diskCycle must be declared before sw so its address can be passed to
	// NewSoftSwitches (paddle timers share this monotonic counter).
	var diskCycle uint64

	ram := memory.NewRAM()
	sw := appleio.NewSoftSwitches(&diskCycle)
	b := bus.NewBus()

	b.Map(0x0000, 0xBFFF, ram)
	b.Map(0xC000, 0xC0FF, sw)
	b.Map(rom.Base, rom.End(), rom)

	// Language Card (slot 0) — 16 KB RAM overlaying $D000-$FFFF.
	// Unconditional install: the three shipped ROMs (Apple2.rom,
	// Apple2_Plus.rom at $D000 covering 12 KB; Apple2e.rom at $C000
	// covering 16 KB) all fully cover $D000-$FFFF. If a future change
	// ships a partial/minimal ROM that does not cover $D000-$FFFF,
	// NewCard will hold a rom pointer whose Read returns 0xFF for
	// uncovered offsets (see memory/rom.go) — that is a configuration
	// bug in the loader, not a runtime concern here.
	// Mapped AFTER io.SoftSwitches so its $C080-$C08F handler wins the
	// address-decode race (bus last-mapping-wins).
	lc := languageCard.NewCard(rom)
	b.Map(0xD000, 0xFFFF, lc)
	b.Map(0xC080, 0xC08F, languageCard.NewSwitches(lc))
	// ROM remains mapped underneath but is never reached via bus
	// fallthrough; the LC delegates to rom.Read when ramRead=false.

	fmt.Printf("Apple II Emulator — Iteration 7\n")
	fmt.Printf("ROM: %s (%d bytes at $%04X–$%04X)\n", *romPath, rom.Size(), rom.Base, rom.End())

	// --- Init CPU -----------------------------------------------------------
	c := cpu.NewCPU(b)
	c.Reset()
	fmt.Printf("Reset vector: $%04X\n", c.PC)

	// --- Init video ---------------------------------------------------------
	vid := video.NewVideo(ram.Data[:])

	// --- Init SDL2 ----------------------------------------------------------
	if err := sdl.Init(uint32(sdl.INIT_VIDEO) | uint32(sdl.INIT_EVENTS) | uint32(sdl.INIT_AUDIO)); err != nil {
		fmt.Fprintf(os.Stderr, "SDL init failed: %v\n", err)
		os.Exit(1)
	}
	// Explicitly enable drag-and-drop events. macOS defaults-on, but Linux
	// X11/Wayland does not guarantee this. Cite go-sdl2@v0.4.40/sdl/events.go:1357.
	sdl.EventState(sdl.DROPFILE, sdl.ENABLE)
	defer sdl.Quit()

	window, err := sdl.CreateWindow(
		"Apple II Emulator",
		sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		int32(windowW), int32(windowH),
		sdl.WINDOW_SHOWN,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Create window failed: %v\n", err)
		os.Exit(1)
	}
	defer window.Destroy()

	// No RENDERER_PRESENTVSYNC: we use time.Sleep-based pacing in the main loop
	// as the single frame cap. VSync + manual cap cause speed drift on
	// non-60 Hz displays (e.g. 120 Hz ProMotion, 59.94 Hz NTSC).
	renderer, err := sdl.CreateRenderer(window, -1,
		sdl.RENDERER_ACCELERATED)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Create renderer failed: %v\n", err)
		os.Exit(1)
	}
	defer renderer.Destroy()

	// Nearest-neighbor scaling for crisp pixels.
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "0")

	texture, err := renderer.CreateTexture(
		uint32(sdl.PIXELFORMAT_RGBA32),
		sdl.TEXTUREACCESS_STREAMING,
		int32(video.ScreenW), int32(video.ScreenH),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Create texture failed: %v\n", err)
		os.Exit(1)
	}
	defer texture.Destroy()

	var frameCycle uint64
	spk, spkErr := speaker.New(speaker.Config{
		SampleRate: *sampleRate,
		CPUClock:   cpuClock,
		Volume:     float32(*volume),
	}, &frameCycle)
	if spkErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", spkErr)
	}
	defer spk.Close() // LAST defer -> FIRST to run

	// Map speaker AFTER SoftSwitches so it wins $C030-$C03F.
	b.Map(0xC030, 0xC03F, speaker.NewDevice(spk))

	// --- Disk II (slot 6) ---------------------------------------------------
	// diskCycle is a monotonically-increasing CPU cycle counter for the disk
	// controller.  Unlike frameCycle (which resets to 0 at the start of each
	// video frame), diskCycle never wraps, so the controller's lastCycle
	// subtraction never produces a spurious uint64 underflow that would cause
	// nibblePos to jump wildly across the track.
	// Note: diskCycle is declared above (before sw) so both the disk controller
	// and paddle timers share the same monotonic pointer.
	dc := disk.NewController(&diskCycle)
	if *diskTrace {
		dc.SetTracer(disk.NewStderrTracer(os.Stderr, *diskTraceN))
	}
	defer dc.Close() // flushes dirty tracks

	// installDiskCard wires the Disk II slot-6 PROM and softswitches. It is
	// idempotent: once installed, subsequent calls are no-ops. Called at
	// startup (if any disk flag was given) and lazily on drag-and-drop so a
	// user who launches with no -disk1/-disk2 flag can drop a disk mid-session.
	//
	// Ordering: PROM load first. If it fails, bail BEFORE mapping softswitches
	// (avoids a half-installed card whose PROM region is blank). promFailed
	// rate-limits stderr: first failure is verbose, subsequent failures are terse
	// so three consecutive drops with a missing DISK2.rom produce one detailed
	// line + two terse lines, not three identical long lines.
	var (
		diskCardInstalled bool
		promFailed        bool
	)
	installDiskCard := func() {
		if diskCardInstalled {
			return
		}

		// PROM first — if this fails, bail without wiring the softswitches.
		prom, err := memory.LoadROM("roms/DISK2.rom", 0xC600)
		if err != nil {
			if !promFailed {
				fmt.Fprintf(os.Stderr, "disk PROM: %v (card not installed)\n", err)
				promFailed = true
			} else {
				fmt.Fprintln(os.Stderr, "disk PROM: still missing — install failed on prior attempt")
			}
			return
		}
		b.Map(prom.Base, prom.End(), prom)

		// PROM succeeded: now safe to map the softswitches and flip the flag.
		b.Map(0xC0E0, 0xC0EF, disk.NewSwitches(dc))

		fmt.Printf("Disk II PROM: roms/DISK2.rom (%d bytes at $%04X–$%04X)\n",
			prom.Size(), prom.Base, prom.End())
		diskCardInstalled = true
		promFailed = false // reset on success so future attempts are not silenced
	}

	// Startup: install the card only if a flag was set. Preserves the no-disk
	// Applesoft fall-through behaviour when neither -disk1 nor -disk2 is given.
	if *disk1 != "" || *disk2 != "" {
		installDiskCard()
	}
	if *disk1 != "" {
		if err := dc.Mount(0, *disk1, *order); err != nil {
			fmt.Fprintf(os.Stderr, "disk1: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Disk 1: %s\n", *disk1)
	}
	if *disk2 != "" {
		if err := dc.Mount(1, *disk2, *order); err != nil {
			fmt.Fprintf(os.Stderr, "disk2: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Disk 2: %s\n", *disk2)
	}

	fmt.Println("Running... (Esc to quit, Ctrl+R to reset)")

	// --- Main loop ----------------------------------------------------------
	running := true
	frameCount := 0
	fpsTimer := time.Now()

	for running {
		frameStart := time.Now()
		// droppedThisFrame gates multi-file drops: only the first DROPFILE per
		// host frame is processed. Declared here (not module-scope) so it resets
		// per frame; the inner PollEvent loop drains all queued events.
		droppedThisFrame := false

		// 1. Poll SDL events (keyboard, quit)
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch e := event.(type) {
			case *sdl.QuitEvent:
				running = false

			case *sdl.DropEvent:
				if e.Type == sdl.DROPFILE && !droppedThisFrame {
					droppedThisFrame = true
					handleDropFile(e.File, dc, installDiskCard)
				}

			case *sdl.KeyboardEvent:
				switch e.Type {
				case sdl.KEYDOWN:
					// Escape to quit
					if e.Keysym.Sym == sdl.K_ESCAPE {
						running = false
						break
					}
					// Ctrl+R to reset (no audio interference: buffers drain naturally)
					if e.Keysym.Sym == sdl.K_r && e.Keysym.Mod&sdl.KMOD_CTRL != 0 {
						c.Reset()
						lc.Reset()
						break
					}
					// Ctrl+1 / Ctrl+2 eject the respective drive.
					if handleDiskEjectKey(e.Keysym.Sym, e.Keysym.Mod, dc) {
						break
					}
					// F12 turbo mode (hold to activate)
					if handleTurboKeyDown(e.Keysym.Sym) {
						break
					}
					// Arrow keys drive virtual paddle (no character passthrough)
					if handleArrowKeyDown(e.Keysym.Sym, sw) {
						break
					}
					// Joystick buttons (left-alt / right-alt)
					if handleButtonKeyDown(e.Keysym.Sym, sw) {
						break
					}
					// Character keystrokes
					if key := sdlKeyToApple(e); key != 0 {
						sw.PressKey(key)
					}

				case sdl.KEYUP:
					handleTurboKeyUp(e.Keysym.Sym)
					handleArrowKeyUp(e.Keysym.Sym, sw)
					handleButtonKeyUp(e.Keysym.Sym, sw)
				}

			case *sdl.WindowEvent:
				switch e.Event {
				case sdl.WINDOWEVENT_FOCUS_LOST, sdl.WINDOWEVENT_HIDDEN, sdl.WINDOWEVENT_MINIMIZED:
					spk.PauseFromHost(true)
					// Clear any held buttons/paddles to avoid stuck input after
					// alt-tab or window minimize.
					sw.ReleaseButton(0)
					sw.ReleaseButton(1)
					sw.ReleaseButton(2)
					sw.SetPaddle(0, 128)
					sw.SetPaddle(1, 128)
					leftHeld, rightHeld, upHeld, downHeld = false, false, false, false
					turboMode = false // never leave turbo armed after window loses focus
				case sdl.WINDOWEVENT_FOCUS_GAINED, sdl.WINDOWEVENT_SHOWN, sdl.WINDOWEVENT_RESTORED:
					spk.PauseFromHost(false)
				}
			}
		}

		// 1b. Turbo edge detect: if turbo just released, drop the queued
		// fast-speed audio samples BEFORE this frame's EndFrame push, so
		// the new real-speed audio frame is queued cleanly.
		if prevTurbo && !turboMode {
			spk.ClearAudioQueue()
		}
		prevTurbo = turboMode

		// 2. Run CPU for one frame's worth of cycles
		// Reset frame-relative cycle counter at the start of each frame.
		frameCycle = 0
		for frameCycle < uint64(cyclesPerFrame) {
			// frameCycle holds the CPU's cycle offset at the START of the next
			// instruction. Speaker.Toggle reads *frameCycle when $C030 is hit
			// during c.Step(), so toggles are stamped at the instruction's start
			// cycle. We increment AFTER Step() completes.
			consumed := uint64(c.Step())
			frameCycle += consumed
			diskCycle += consumed
		}
		spk.EndFrame(frameCycle)

		// 3. Render the screen — every frame.
		vid.Render(sw.TextMode, sw.MixedMode, sw.HiRes, sw.Page2)

		// 4. Upload pixel buffer to SDL texture and present
		texture.Update(nil, unsafe.Pointer(&vid.Pixels[0]), int(video.ScreenW)*4)
		renderer.Clear()
		renderer.Copy(texture, nil, nil)
		renderer.Present()

		// 5. Title-bar update (FPS / TURBO + disk activity), once per second.
		frameCount++
		if time.Since(fpsTimer) >= time.Second {
			window.SetTitle(buildTitle(frameCount, dc, turboMode))
			frameCount = 0
			fpsTimer = time.Now()
		}

		// 6. Frame cap — bypassed in turbo.
		// Frame cap — sleep until target frame time. Uses time.Sleep for sub-ms
		// precision so we don't truncate ~900µs/frame.
		//
		// Why this matters for audio: under-sleeping makes our loop iterate faster
		// than 60 Hz, which over-fills the SDL audio queue. Speaker back-pressures
		// (drops frames at >4*bytesPerFrame queued, see speaker.go:191), eventually
		// underrunning and emitting the warning at speaker.go:184. Sub-ms sleep
		// keeps the queue near 1 frame deep.
		//
		// Floor at 100µs avoids hot-spin when the remaining budget is near-zero
		// (Go's time.Sleep guarantees AT LEAST the duration; on darwin, very
		// small sleeps still take ~50µs of overhead, but 0-budget skips here
		// would be a tight retry loop).
		if !turboMode {
			elapsed := time.Since(frameStart)
			target := time.Second / targetFPS
			remaining := target - elapsed
			if remaining > 100*time.Microsecond {
				time.Sleep(remaining)
			}
		}
	}
}

// sdlKeyToApple converts an SDL keyboard event to an Apple II key code.
// Returns 0 if the key has no Apple II equivalent.
func sdlKeyToApple(e *sdl.KeyboardEvent) uint8 {
	sym := e.Keysym.Sym
	mod := e.Keysym.Mod

	// Control key combinations
	if mod&sdl.KMOD_CTRL != 0 {
		if sym >= sdl.K_a && sym <= sdl.K_z {
			return uint8(sym-sdl.K_a) + 1 // Ctrl+A=1, Ctrl+B=2, etc.
		}
	}

	// Special keys
	switch sym {
	case sdl.K_RETURN, sdl.K_KP_ENTER:
		return 0x0D
	case sdl.K_BACKSPACE:
		return 0x08
	case sdl.K_DELETE:
		return 0x7F
	case sdl.K_TAB:
		return 0x09
	}
	// Note: arrow keys (K_LEFT, K_RIGHT, K_UP, K_DOWN) are intentionally NOT
	// mapped here — they drive the virtual paddle via handleArrowKey* instead.

	// Printable ASCII characters
	if int32(sym) >= 32 && int32(sym) <= 126 {
		ch := uint8(sym)

		if mod&sdl.KMOD_SHIFT != 0 {
			if ch >= 'a' && ch <= 'z' {
				ch -= 32
			} else {
				switch ch {
				case '1':
					ch = '!'
				case '2':
					ch = '@'
				case '3':
					ch = '#'
				case '4':
					ch = '$'
				case '5':
					ch = '%'
				case '6':
					ch = '^'
				case '7':
					ch = '&'
				case '8':
					ch = '*'
				case '9':
					ch = '('
				case '0':
					ch = ')'
				case '-':
					ch = '_'
				case '=':
					ch = '+'
				case '[':
					ch = '{'
				case ']':
					ch = '}'
				case '\\':
					ch = '|'
				case ';':
					ch = ':'
				case '\'':
					ch = '"'
				case ',':
					ch = '<'
				case '.':
					ch = '>'
				case '/':
					ch = '?'
				case '`':
					ch = '~'
				}
			}
		} else {
			// Apple II is uppercase-only — auto-convert
			if ch >= 'a' && ch <= 'z' {
				ch -= 32
			}
		}
		return ch
	}

	return 0
}

// Arrow-key held state for virtual paddle axis tracking.
// Tracked so releasing one key while the opposite is still held restores the
// correct direction rather than snapping to center prematurely.
var (
	leftHeld, rightHeld bool
	upHeld, downHeld    bool
)

// turboMode is toggled by F12. While true, the frame cap is bypassed so
// the CPU emulation speed scales with host capacity. Render still happens
// every frame; frame-skip is a future tuning knob only useful if profiling
// shows render dominates host cost.
var turboMode bool
var prevTurbo bool

// handleArrowKeyDown maps arrow key presses to virtual paddle positions.
// Returns true if the key was consumed (caller should not pass to sdlKeyToApple).
func handleArrowKeyDown(sym sdl.Keycode, sw *appleio.SoftSwitches) bool {
	switch sym {
	case sdl.K_LEFT:
		leftHeld = true
		sw.SetPaddle(0, 0)
		return true
	case sdl.K_RIGHT:
		rightHeld = true
		sw.SetPaddle(0, 255)
		return true
	case sdl.K_UP:
		upHeld = true
		sw.SetPaddle(1, 0)
		return true
	case sdl.K_DOWN:
		downHeld = true
		sw.SetPaddle(1, 255)
		return true
	}
	return false
}

// handleArrowKeyUp maps arrow key releases to virtual paddle positions.
// If the opposite direction key is still held, that direction is maintained;
// otherwise the axis snaps back to center (128).
func handleArrowKeyUp(sym sdl.Keycode, sw *appleio.SoftSwitches) {
	switch sym {
	case sdl.K_LEFT:
		leftHeld = false
		if rightHeld {
			sw.SetPaddle(0, 255)
		} else {
			sw.SetPaddle(0, 128)
		}
	case sdl.K_RIGHT:
		rightHeld = false
		if leftHeld {
			sw.SetPaddle(0, 0)
		} else {
			sw.SetPaddle(0, 128)
		}
	case sdl.K_UP:
		upHeld = false
		if downHeld {
			sw.SetPaddle(1, 255)
		} else {
			sw.SetPaddle(1, 128)
		}
	case sdl.K_DOWN:
		downHeld = false
		if upHeld {
			sw.SetPaddle(1, 0)
		} else {
			sw.SetPaddle(1, 128)
		}
	}
}

// handleButtonKeyDown maps left-alt / right-alt to joystick button press.
// Returns true if the key was consumed.
func handleButtonKeyDown(sym sdl.Keycode, sw *appleio.SoftSwitches) bool {
	switch sym {
	case sdl.K_LALT:
		sw.PressButton(0) // $C061 — punch / fire 1 (Open-Apple)
		return true
	case sdl.K_RALT:
		sw.PressButton(1) // $C062 — kick / fire 2 (Closed-Apple)
		return true
	}
	return false
}

// handleButtonKeyUp maps left-alt / right-alt to joystick button release.
func handleButtonKeyUp(sym sdl.Keycode, sw *appleio.SoftSwitches) {
	switch sym {
	case sdl.K_LALT:
		sw.ReleaseButton(0)
	case sdl.K_RALT:
		sw.ReleaseButton(1)
	}
}

// handleTurboKeyDown / handleTurboKeyUp: F12 held = turbo on.
// Hold-to-activate (not toggle) so releasing always returns to 60 fps.
func handleTurboKeyDown(sym sdl.Keycode) bool {
	if sym == sdl.K_F12 {
		turboMode = true
		return true
	}
	return false
}

func handleTurboKeyUp(sym sdl.Keycode) {
	if sym == sdl.K_F12 {
		turboMode = false
	}
}

// labelMax is the rune-width budget for a disk label in the window title.
// 8 runes fits "karateka" exactly; longer names are truncated by truncateLabel.
const labelMax = 8

// truncateLabel applies the title-bar rune-width budget to a label already
// derived by disk.DriveLabel (basename without extension). Truncates to
// maxRunes using rune slicing so multi-byte UTF-8 filenames don't get
// corrupted mid-sequence. Returns "" if label is empty (empty drive).
// If maxRunes <= 0, returns label unmodified.
func truncateLabel(label string, maxRunes int) string {
	if label == "" {
		return ""
	}
	if maxRunes <= 0 {
		return label
	}
	runes := []rune(label)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return label
}

// buildTitle composes the SDL window title, showing FPS plus per-drive
// activity indicators. dc may be nil if no disk image was loaded.
//
// Format examples:
//
//	Apple II Emulator — 60 fps                         (no disks)
//	Apple II Emulator — 60 fps — [D1:karateka ●]       (drive 1 reading)
//	Apple II Emulator — 60 fps — [D1:dos33 ◉] [D2 ]   (drive 1 writing, drive 2 idle)
//	Apple II Emulator — TURBO — [D1:karateka ●]        (turbo engaged, drive 1 active)
func buildTitle(fps int, dc *disk.Controller, turbo bool) string {
	rate := fmt.Sprintf("%d fps", fps)
	if turbo {
		rate = "TURBO"
	}
	if dc == nil {
		return fmt.Sprintf("Apple II Emulator — %s", rate)
	}
	parts := []string{}
	for i := 0; i < 2; i++ {
		if !dc.HasDisk(i) {
			continue
		}
		glyph := " "
		switch {
		case dc.HadRecentWrite(i):
			glyph = "◉" // write activity (higher priority than read)
		case dc.HadRecentRead(i):
			glyph = "●" // read activity
		}
		// DriveLabel returns the pre-computed basename-minus-ext (no filepath
		// calls here at title cadence). truncateLabel applies the rune budget.
		label := truncateLabel(dc.DriveLabel(i), labelMax)
		if label != "" {
			parts = append(parts, fmt.Sprintf("[D%d:%s %s]", i+1, label, glyph))
		} else {
			parts = append(parts, fmt.Sprintf("[D%d%s]", i+1, glyph))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Apple II Emulator — %s", rate)
	}
	return fmt.Sprintf("Apple II Emulator — %s — %s", rate, strings.Join(parts, " "))
}

// handleDropFile is called when the user drags a file onto the window.
// Drive selection: the left half of the window maps to drive 1, the right
// half to drive 2. SHIFT held at drop time also forces drive 2 — useful for
// within-app drags where keyboard focus is reliable. File extension must be
// one of .dsk/.do/.po (case-insensitive); anything else is rejected with a
// stderr warning.
//
// Position-based selection exists because on macOS cross-app drags (e.g.
// Finder → our window) the source app owns keyboard focus, so SDL's
// GetKeyboardState() returns a stale snapshot and SHIFT is not observable.
// The mouse position at drop is always accurate.
//
// MUST be called on the SDL main goroutine (same invariant as the rest of
// the event-loop handlers).
//
// installCard is a closure that wires the slot-6 PROM + softswitches if not
// already installed (see installDiskCard). Allows launching with no -disk1/-disk2
// flags and later dropping a disk to bring the card online. After the card is
// installed mid-session, the user must press Ctrl+R to re-run the PROM slot scan.
func handleDropFile(path string, dc *disk.Controller, installCard func()) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".dsk", ".do", ".po":
		// accepted
	default:
		fmt.Fprintf(os.Stderr, "disk: drop rejected — unsupported extension %q (expected .dsk/.do/.po)\n", ext)
		return
	}

	driveIdx := dropDriveFromPosition()
	if isShiftHeld() {
		driveIdx = 1
	}

	installCard()

	if err := dc.Swap(driveIdx, path); err != nil {
		// Swap atomically ejects the old image before attempting the new load,
		// so a load failure leaves the drive empty. Tell the user so they
		// aren't surprised that the previously-working disk is gone.
		fmt.Fprintf(os.Stderr, "disk: swap into drive %d failed: %v — drive is now empty\n", driveIdx+1, err)
		return
	}
	fmt.Fprintf(os.Stderr, "disk: drive %d loaded %s — press Ctrl+R to reboot\n",
		driveIdx+1, dc.DriveLabel(driveIdx))
}

// isShiftHeld returns true if either Shift key is currently down at the
// moment the drop event is processed. Reliable for within-app drags; on
// macOS cross-app drags (Finder → emulator) the source app owns focus
// and the snapshot is stale — see dropDriveFromPosition for the primary
// selector. Verified: go-sdl2@v0.4.40/sdl/keyboard.go:47.
func isShiftHeld() bool {
	st := sdl.GetKeyboardState()
	return st[sdl.SCANCODE_LSHIFT] != 0 || st[sdl.SCANCODE_RSHIFT] != 0
}

// dropDriveFromPosition returns 0 (drive 1) if the mouse is in the left
// half of the emulator window at drop time, else 1 (drive 2). Mouse
// position is always accurate even when keyboard focus is elsewhere, so
// this is the primary drive selector for cross-app drag-drops from Finder.
func dropDriveFromPosition() int {
	x, _, _ := sdl.GetMouseState()
	if x < int32(windowW/2) {
		return 0
	}
	return 1
}

// handleDiskEjectKey ejects a disk when Ctrl+1 / Ctrl+2 is pressed.
// Returns true if the key was consumed so the caller does not emit it as
// a character code.
func handleDiskEjectKey(sym sdl.Keycode, mod uint16, dc *disk.Controller) bool {
	if mod&sdl.KMOD_CTRL == 0 {
		return false
	}
	driveIdx := -1
	switch sym {
	case sdl.K_1:
		driveIdx = 0
	case sdl.K_2:
		driveIdx = 1
	default:
		return false
	}
	if !dc.HasDisk(driveIdx) {
		fmt.Fprintf(os.Stderr, "disk: drive %d already empty\n", driveIdx+1)
		return true
	}
	if err := dc.Eject(driveIdx); err != nil {
		fmt.Fprintf(os.Stderr, "disk: eject drive %d failed: %v\n", driveIdx+1, err)
	}
	return true
}

// loadROM detects the ROM size and sets the correct base address.
func loadROM(path string) (*memory.ROM, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	size := info.Size()
	switch size {
	case 12288:
		return memory.LoadROM(path, 0xD000)
	case 16384:
		return memory.LoadROM(path, 0xC000)
	default:
		if size > 0 && size <= 12288 {
			return memory.LoadROM(path, uint16(0x10000-size))
		}
		return nil, fmt.Errorf("unexpected ROM size %d bytes (expected 12288 or 16384)", size)
	}
}

package main

import (
	"flag"
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/apple2emu/bus"
	"github.com/apple2emu/cpu"
	"github.com/apple2emu/disk"
	appleio "github.com/apple2emu/io"
	"github.com/apple2emu/memory"
	"github.com/apple2emu/speaker"
	"github.com/apple2emu/video"
)

const (
	// Apple II runs at 1.023 MHz, display refreshes at ~60 Hz.
	cpuClock       = 1023000
	targetFPS      = 60
	cyclesPerFrame = cpuClock / targetFPS // ~17050

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
	ram := memory.NewRAM()
	sw := appleio.NewSoftSwitches()
	b := bus.NewBus()

	b.Map(0x0000, 0xBFFF, ram)
	b.Map(0xC000, 0xC0FF, sw)
	b.Map(rom.Base, rom.End(), rom)

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

	renderer, err := sdl.CreateRenderer(window, -1,
		sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
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
	var diskCycle uint64
	dc := disk.NewController(&diskCycle)
	if *diskTrace {
		dc.SetTracer(disk.NewStderrTracer(os.Stderr, *diskTraceN))
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
	defer dc.Close() // flushes dirty tracks

	// Softswitch window for slot 6: $C0E0-$C0EF (overlays SoftSwitches; last-wins).
	b.Map(0xC0E0, 0xC0EF, disk.NewSwitches(dc))

	// Boot PROM for slot 6: $C600-$C6FF.
	prom, err := memory.LoadROM("roms/DISK2.rom", 0xC600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "disk PROM: %v\n", err)
		os.Exit(1)
	}
	b.Map(prom.Base, prom.End(), prom)
	fmt.Printf("Disk II PROM: roms/DISK2.rom (%d bytes at $%04X–$%04X)\n", prom.Size(), prom.Base, prom.End())

	fmt.Println("Running... (Esc to quit, Ctrl+R to reset)")

	// --- Main loop ----------------------------------------------------------
	running := true
	frameCount := 0
	fpsTimer := time.Now()

	for running {
		frameStart := time.Now()

		// 1. Poll SDL events (keyboard, quit)
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch e := event.(type) {
			case *sdl.QuitEvent:
				running = false

			case *sdl.KeyboardEvent:
				if e.Type == sdl.KEYDOWN {
					if key := sdlKeyToApple(e); key != 0 {
						sw.PressKey(key)
					}
					// Escape to quit
					if e.Keysym.Sym == sdl.K_ESCAPE {
						running = false
					}
					// Ctrl+R to reset
					// reset does not interfere with audio: per-frame buffers drain naturally
					if e.Keysym.Sym == sdl.K_r && e.Keysym.Mod&sdl.KMOD_CTRL != 0 {
						c.Reset()
					}
				}

			case *sdl.WindowEvent:
				switch e.Event {
				case sdl.WINDOWEVENT_FOCUS_LOST, sdl.WINDOWEVENT_HIDDEN, sdl.WINDOWEVENT_MINIMIZED:
					spk.PauseFromHost(true)
				case sdl.WINDOWEVENT_FOCUS_GAINED, sdl.WINDOWEVENT_SHOWN, sdl.WINDOWEVENT_RESTORED:
					spk.PauseFromHost(false)
				}
			}
		}

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

		// 3. Render the screen
		vid.Render(sw.TextMode, sw.MixedMode, sw.HiRes, sw.Page2)

		// 4. Upload pixel buffer to SDL texture and present
		texture.Update(nil, unsafe.Pointer(&vid.Pixels[0]), int(video.ScreenW)*4)
		renderer.Clear()
		renderer.Copy(texture, nil, nil)
		renderer.Present()

		// 5. FPS counter in title bar
		frameCount++
		if time.Since(fpsTimer) >= time.Second {
			window.SetTitle(fmt.Sprintf("Apple II Emulator — %d fps", frameCount))
			frameCount = 0
			fpsTimer = time.Now()
		}

		// If vsync isn't working, manually cap to 60 fps
		elapsed := time.Since(frameStart)
		target := time.Second / targetFPS
		if elapsed < target {
			sdl.Delay(uint32((target - elapsed).Milliseconds()))
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
	case sdl.K_LEFT:
		return 0x08
	case sdl.K_RIGHT:
		return 0x15
	case sdl.K_UP:
		return 0x0B
	case sdl.K_DOWN:
		return 0x0A
	case sdl.K_TAB:
		return 0x09
	}

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

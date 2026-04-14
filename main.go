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
	appleio "github.com/apple2emu/io"
	"github.com/apple2emu/memory"
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
	flag.Parse()

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

	fmt.Printf("Apple II Emulator — Iteration 3\n")
	fmt.Printf("ROM: %s (%d bytes at $%04X–$%04X)\n", *romPath, rom.Size(), rom.Base, rom.End())

	// --- Init CPU -----------------------------------------------------------
	c := cpu.NewCPU(b)
	c.Reset()
	fmt.Printf("Reset vector: $%04X\n", c.PC)

	// --- Init video ---------------------------------------------------------
	vid := video.NewVideo(ram.Data[:])

	// --- Init SDL2 ----------------------------------------------------------
	if err := sdl.Init(uint32(sdl.INIT_VIDEO) | uint32(sdl.INIT_EVENTS)); err != nil {
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
					if e.Keysym.Sym == sdl.K_r && e.Keysym.Mod&sdl.KMOD_CTRL != 0 {
						c.Reset()
					}
				}
			}
		}

		// 2. Run CPU for one frame's worth of cycles
		for cycles := 0; cycles < cyclesPerFrame; {
			cycles += c.Step()
		}

		// 3. Render the screen
		vid.RenderText()

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

# Educational Walkthrough: `video/video.go` -- The Text Screen Renderer

> This is the eighth walkthrough in the series and the second document in
> the video rendering package. It covers `video/video.go`, which is the
> direct successor to `walkthrough_chargen.md`. As promised at the end of
> that document, this walkthrough introduces the flash timer, the
> `textLineAddr()` row-interleaving formula, and the phosphor green color
> palette. It closes all four forward pointers left open by the chargen
> walkthrough. You should have read all seven prior walkthroughs before
> starting this one. Each section follows the familiar What It Is / Why It
> Matters / How the Code Works / Real-World Analogy pattern.

---

## Section 0: Background -- What Is a Video Renderer?

### What It Is

On the real Apple II, the video circuitry is a dedicated hardware block that
operates independently of the 6502 CPU. During the horizontal blanking
interval between scanlines, the video circuitry performs DMA (Direct Memory
Access) -- it steals bus cycles from the CPU and reads screen bytes directly
from RAM at `$0400`-`$07FF` (1,024-2,047 decimal). For each byte it reads, it
immediately feeds that byte as an address into the Signetics 2513 character
generator ROM on the video side of the machine. The ROM returns a row of pixel
bits; the video circuitry shifts those bits out to the composite NTSC signal
at the 14.318 MHz dot clock. Horizontal and vertical counters track position.
The CPU never participates in this loop -- it keeps executing instructions
during the active display period and yields the bus only during horizontal
blanking.

This walkthrough closes the forward pointer from `walkthrough_chargen.md`
Section 7, which promised: "The next walkthrough covers `video/video.go`,
which calls `CharGenROM` for every character on every scanline and converts
the pixel bits into an RGBA frame buffer for SDL to display. It also
introduces the flash timer, the `textLineAddr()` row-interleaving formula,
and the phosphor green color palette." All four of those promises are
delivered here.

Our emulator has no hardware video circuit. Instead, `video.go` approximates
the hardware behavior with a software loop: it iterates over the 960 bytes of
screen RAM, calls `CharGenROM` for each character cell and each scanline,
applies the flash state override for blinking characters, and writes RGBA
pixel values into a flat byte buffer. Once the buffer is complete, `main.go`
uploads it to an SDL texture for display in the desktop window. The end result
is visually identical to the original hardware output -- 280x192 pixels of
phosphor green text on a black background.

### Why It Matters

This is the last piece that makes the emulator visual. Without this file, the
CPU executes instructions correctly (verified by the Klaus Dormann test suite),
the bus routes reads and writes to the right devices, the softswitches record
video mode changes -- but nothing appears on screen. `video.go` is the
translation layer from the emulator's internal state (RAM bytes) to the thing
the user actually sees.

### Diagram 1: The Complete Video Pipeline (from RAM to SDL window)

```text
  Screen RAM        CharGenROM()       video.go           main.go
  ($0400-$07FF)     (chargen.go)       RenderText()       SDL upload
  +-----------+     +-----------+     +-------------+     +----------+
  | 960 bytes |---->| 7 pixel   |---->| RGBA buffer |---->| texture  |
  | 40x24     |     | bits/row  |     | 280x192x4   |     | display  |
  +-----------+     +-----------+     +-------------+     +----------+
       |                 |                  |                   |
  CPU writes        pure lookup        flash timer,        SDL2 API
  characters        no state           color mapping
```

Cross-reference: `walkthrough_main_phase3.md` Section 2 shows how `main.go`
calls `vid.RenderText()` and uploads `vid.Pixels` to the SDL texture. The SDL
upload step is examined more closely in Section 7 of this document.

---

## Section 1: Screen Constants

### What It Is

At the top of `video.go`, six constants define the Apple II's 40-column text
display geometry:

```go
const (
    TextCols = 40
    TextRows = 24
    CharW    = 7                // pixels per character width
    CharH    = 8                // pixels per character height
    ScreenW  = TextCols * CharW // 280 pixels
    ScreenH  = TextRows * CharH // 192 pixels
)
```

### Why It Matters

These numbers are not arbitrary -- they come directly from the Apple II
hardware. The display is 40 columns because the Apple II was designed to
connect to a standard TV set, and a 40-character line was the maximum that
produced legible text on the typical 1977 consumer television. The 24-row
count comes from NTSC vertical timing: the screen has 192 active pixel lines,
divided by 8 pixels per character row, giving 24 character rows.

The character width of 7 pixels is perhaps the most hardware-constrained
constant of all. The Apple II's 14.318 MHz master clock (derived from the
NTSC color burst frequency) drives the horizontal video timing. After
accounting for blanking and sync periods, there are exactly 280 pixel clocks
available for active display per scanline. 280 divided by 40 columns is
exactly 7 pixels per character column. This is why `CharW = 7` and not 8.
For the full derivation, see `walkthrough_chargen.md` Section 1.

The 40-column text mode is also what the TextMode softswitch (`$C050`-`$C051`,
49,232-49,233 decimal) selects. See `walkthrough_io.md` Section 1 for the
TextMode field definition and Section 3c for how the softswitch address is
decoded.

### How the Code Works

The derived constants (`ScreenW = 280`, `ScreenH = 192`) are computed by the
compiler at build time. The Go compiler evaluates constant expressions
involving only constants, so `TextCols * CharW` becomes the literal `280`
before any code runs. This means the values can be used as array sizes,
bounds in range checks, and offsets throughout the package without any
runtime cost.

The constants are also exported (capitalized), which means `main.go` can
reference `video.ScreenW` without repeating the magic number. You will see
`video.ScreenW` appear in the SDL texture pitch calculation in `main.go`.

### Real-World Analogy

Grid paper with fixed dimensions. You buy a pad that is 40 columns wide and
24 rows tall, with each cell being 7 dots wide and 8 dots tall. The total
pixel count is fixed by the paper's physical size -- you cannot add an extra
row just by wishing. All calculations on the grid (where is column 17? how
many bytes is one full row?) derive from these four base measurements.

### Diagram 2: Screen Geometry Breakdown

```text
  +-- 280 pixels (40 cols x 7 px) --+
  |                                  |
  |  col 0    col 1    ...  col 39   |  row 0
  |  7 px     7 px          7 px     |  8 px tall
  |                                  |
  |  col 0    col 1    ...  col 39   |  row 1
  |  7 px     7 px          7 px     |  8 px tall
  |          ...                     |
  |  col 0    col 1    ...  col 39   |  row 23
  |  7 px     7 px          7 px     |  8 px tall
  +----------------------------------+
  |
  192 pixels (24 rows x 8 px)

  Total pixel buffer: 280 x 192 x 4 (RGBA) = 215,040 bytes
```

---

## Section 2: The Color Palette

### What It Is

Two RGBA color values define every pixel on screen:

```go
var (
    ColorOn  = [4]uint8{0x33, 0xFF, 0x33, 0xFF} // bright green
    ColorOff = [4]uint8{0x00, 0x00, 0x00, 0xFF} // black
)
```

Note that these are declared with `var`, not `const`. This is a Go language
constraint, not a design decision. Go only permits `const` for scalar types:
integers, floating-point numbers, strings, booleans, and runes. Array types
like `[4]uint8` cannot be declared as constants in Go. The `var` declaration
is the only available option. The values are never modified after package
initialization -- they are effectively constant -- but the language cannot
express that guarantee for non-scalar types. This is a common Go idiom for
compile-time-known, never-mutated values that happen to be arrays or structs.

### Why It Matters

The Apple II's original monochrome monitor -- the Apple Monitor II and Monitor
III -- used a P1 phosphor screen. P1 phosphor emits green light when struck
by an electron beam. This is the classic "green screen" appearance that
defined the Apple II's visual identity. Every character, every pixel, was
either phosphor-green (lit) or black (unlit). There were no shades, no
intermediate intensities, and no colors. It is a 1-bit display.

The specific green chosen here is `{$33, $FF, $33, $FF}` = (51, 255, 51, 255)
in decimal. This is not pure green `{0, 255, 0}`. The partial red and blue
channels (51 each) soften the color slightly, producing a warmer tone that
more closely approximates the glow of real P1 phosphor, which has a yellowish
tinge compared to a pure mathematical green. This is a stylistic choice that
improves visual authenticity.

### How the Code Works

Each color value is a `[4]uint8` array, one byte per RGBA channel:

- Byte 0: Red -- `0x33` (51) for lit pixels, `0x00` (0) for dark
- Byte 1: Green -- `0xFF` (255) for lit pixels, `0x00` (0) for dark
- Byte 2: Blue -- `0x33` (51) for lit pixels, `0x00` (0) for dark
- Byte 3: Alpha -- `0xFF` (255) always, meaning fully opaque

The Alpha channel is always 255 because the Apple II display is fully opaque --
there is no transparency or blending. A black pixel is a black phosphor cell
that is simply not excited; it has full opacity.

The `[4]uint8` array type was chosen because it matches SDL's RGBA32 pixel
format byte-for-byte. SDL's texture upload expects the pixel buffer in RGBA32
format, with four consecutive bytes per pixel in R-G-B-A order. Because the
`Pixels` buffer is filled with these four-byte color values directly, the
texture upload in `main.go` can use `unsafe.Pointer` to pass the buffer
address directly to SDL without any conversion or shuffling. The layout is
already what SDL expects.

### Real-World Analogy

Two paint pots: one phosphor green, one black. Every pixel on screen gets
exactly one coat of paint -- either the green pot or the black pot. No
mixing, no gradients, no transparency. The 215,040-byte pixel buffer is a
record of which pot was used for each of the 53,760 pixels.

### Diagram 3: RGBA Pixel Byte Layout

```text
  One pixel in the Pixels buffer (4 bytes):

  offset+0   offset+1   offset+2   offset+3
  +--------+--------+--------+--------+
  |  Red   | Green  |  Blue  | Alpha  |
  +--------+--------+--------+--------+

  ColorOn:   0x33      0xFF      0x33      0xFF
             (51)      (255)     (51)      (255)

  ColorOff:  0x00      0x00      0x00      0xFF
             (0)       (0)       (0)       (255)

  Alpha is always 0xFF (255) -- the display is fully opaque.
```

---

## Section 3: The Video Struct and NewVideo()

### What It Is

The `Video` struct holds all rendering state, and its constructor wires it
to the emulator's RAM:

```go
type Video struct {
    RAM          []uint8 // Direct reference to main RAM (64 KB)
    Pixels       []uint8 // RGBA pixel buffer (280x192x4 bytes)
    FlashState   bool    // Toggled at ~1.875 Hz for flashing characters
    flashCounter int     // Frame counter for flash timing
}

func NewVideo(ram []uint8) *Video {
    return &Video{
        RAM:    ram,
        Pixels: make([]uint8, ScreenW*ScreenH*4),
    }
}
```

### Why It Matters

The `Video` struct is the bridge between the CPU's world and the display.
The `RAM` field is a direct slice reference into the emulator's main 64 KB
RAM array -- not a copy. This means any byte the CPU writes to screen memory
at `$0400`-`$07FF` (1,024-2,047) is immediately visible to `RenderText()` on
the very next call. No synchronization, no message passing, no snapshot
required. The two subsystems share the same backing array by design.

### How the Code Works

**`RAM []uint8`**: A slice that points into the same backing array as
`memory.RAM.Data`. In `main.go`, the constructor is called as:
`vid := video.NewVideo(ram.Data[:])`. This bypasses the bus -- the video
renderer reads screen bytes directly from the RAM data array rather than
going through the bus's `Read` method. For the current text-mode-only
implementation, where text page 1 is always at `$0400`-`$07FF` in unbanked
RAM, this is correct and efficient. It will need reworking if bank switching
(the language card) is ever added, because bank switching can remap the
address space so that `$0400`-`$07FF` no longer refers to the first 960
bytes after offset `$0400` in the physical RAM array.

**`Pixels []uint8`**: Allocated by `make()` with size
`ScreenW * ScreenH * 4 = 280 * 192 * 4 = 215,040 bytes`. Go's `make`
zero-initializes the slice, so before the first call to `RenderText()` the
entire buffer is all zeros. Note that `ColorOff[3]` is `$FF` (255, fully
opaque), so the alpha bytes technically start wrong (0 instead of 255).
This is harmless because `RenderText()` writes every pixel before SDL ever
reads the buffer -- the zero-alpha frame is never displayed. The buffer is
reused every frame -- no allocation happens after construction.

**`FlashState bool`**: Exported (capital F). It is toggled by the flash
timer inside `RenderText()`. The initial value is `false` (Go's zero value
for bool), meaning characters in the flash range `$40`-`$7F` (64-127) start
in normal (non-inverted) display mode on frame zero.

**`flashCounter int`**: Unexported (lowercase f). This is an internal
implementation detail -- the frame counter that drives the flash timer. Its
initial value is `0` (Go's zero value for int). External code has no reason
to read or write it; only `RenderText()` touches it.

**Constructor pattern**: Identical to `memory.NewRAM()` and `bus.New()` --
allocate on the heap, initialize fields, return a pointer. See
`walkthrough_memory.md` for the NewRAM constructor pattern that established
this idiom.

### Real-World Analogy

A TV set connected to the Apple II via a composite video cable. The `RAM`
slice is the cable -- it gives the TV a live view of whatever bytes the CPU
writes to screen memory. The `Pixels` buffer is the phosphor coating on the
inside of the CRT tube. The `FlashState` field and `flashCounter` are the
blink-rate timer built into the TV's video circuitry. The struct holds the
TV; `NewVideo` is the act of plugging the cable in.

---

## Section 4: The Flash Timer

### What It Is

The first five lines of `RenderText()` implement the flash timer -- the
mechanism that makes characters in the `$40`-`$7F` screen byte range blink
on and off:

```go
v.flashCounter++
if v.flashCounter >= 16 {
    v.flashCounter = 0
    v.FlashState = !v.FlashState
}
```

### Why It Matters

This is the flash timer promised in `walkthrough_chargen.md` Section 4
("See the next walkthrough for the flash timer implementation") and Section 6
Q4 ("This separation of concerns is covered in the next walkthrough").
Both of those forward pointers close here.

`CharGenROM` is a pure function with no state. When it receives a screen byte
in the `$40`-`$7F` range, it returns `inverse = false` -- it does NOT decide
whether the character should be shown inverted at this particular moment in
time. Doing so would require `CharGenROM` to know what frame it is, which
would break its purity and make it harder to test. Instead, `CharGenROM`
signals "this character CAN flash" by returning the screen byte as-is
(not masking the upper bit), and `RenderText()` owns the decision of whether
to invert it right now based on the flash timer.

This is the separation of concerns that `walkthrough_chargen.md` Section 6 Q4
described: `CharGenROM` handles the static encoding rules (inverse range,
normal range, character index mapping), and `RenderText()` handles the
time-varying behavior (flash state).

The magic number `16` means: toggle `FlashState` every 16 frames. At 60
frames per second, that is 60 / 16 = 3.75 toggles per second. One full flash
cycle requires two toggles -- one to go from normal to inverse, one to return
from inverse to normal. So the visible blink rate is 3.75 / 2 = 1.875 Hz.
The real Apple II's flash circuitry runs at approximately 1.875 Hz, driven
by the vertical blanking interval counter in the video hardware. The value
`16` produces the correct frequency for a 60 fps emulator.

### How the Code Works

**Initial conditions**: Both `flashCounter` and `FlashState` start at their
Go zero values -- `0` and `false` respectively. No explicit initialization
is needed.

**State machine trace**:

- Frame 0: `flashCounter` increments from 0 to 1. `1 < 16`, no toggle.
  `FlashState` remains `false`.
- Frames 1-14: counter increments each frame. No toggle.
- Frame 15: counter increments from 15 to 16. `16 >= 16`, trigger fires.
  Counter resets to 0. `FlashState` toggles from `false` to `true`.
- Frames 16-30: counter increments 1-15, no toggle. `FlashState` is `true`,
  so flash-range characters are shown inverted.
- Frame 31: counter reaches 16 again. `FlashState` toggles back to `false`.
- The cycle repeats every 32 frames.

**The override in the render loop** (examined in detail in Section 6):

```go
if screenByte >= 0x40 && screenByte < 0x80 && v.FlashState {
    inverse = true
}
```

When `FlashState` is `true` and the screen byte is in the flash range, the
`inverse` flag returned by `CharGenROM` is overridden to `true`. This causes
the character to render in inverse video for that half-cycle. When
`FlashState` returns to `false`, the override does not fire, and the
character renders normally.

### Diagram 4: Flash Timer State Machine

```text
  Frame:   0   1   2  ...  14   15   16   17  ...  30   31   32
  Count:   1   2   3  ...  15   16    1    2  ...  15   16    1
  Flash:   F   F   F  ...   F    T    T    T  ...   T    F    F
                              ^                       ^
                         toggle to               toggle to
                         true here               false here

  Screen byte $48 ('H' in flash range):
    FlashState = false  -->  draws normal 'H'   (inverse = false)
    FlashState = true   -->  draws inverse 'H'  (inverse = true)

  Result: 'H' blinks at ~1.875 Hz (one full on/off cycle every ~32 frames)

  Derivation:
    Toggle rate = 60 fps / 16 frames = 3.75 toggles/sec
    Blink rate  = 3.75 toggles / 2 toggles per cycle = 1.875 Hz
```

### Real-World Analogy

A relay timer on a neon sign. Every N ticks of the clock, the relay flips and
the sign alternates between lit and dark. The sign itself (CharGenROM) does
not know about timing -- it just contains the letter shapes. The relay
(flashCounter) is the only component that knows about time and controls
whether the sign is currently on or off. The relay and the sign are separate
devices connected by a wire (the `inverse` flag).

---

## Section 5: textLineAddr() -- The Interleaved Memory Layout

### What It Is

This private function maps a text row number (0-23) to its base address in
screen RAM:

```go
func textLineAddr(row int) uint16 {
    return 0x0400 + uint16(row%8)*0x80 + uint16(row/8)*0x28
}
```

### Why It Matters

The Apple II's screen memory is not laid out linearly. Row 0 starts at
`$0400` (1,024), but row 1 starts at `$0480` (1,152) -- not `$0428` (1,064)
as you might expect. Row 8 jumps back to `$0428`. This non-linear,
interleaved layout was a hardware optimization by Steve Wozniak that reduced
the chip count on the motherboard. The horizontal and vertical video timing
counters share address bits in a way that produces this interleaving as a
natural consequence, without requiring additional counter logic.

The consequence for software (and for emulators) is that you cannot find any
row's screen memory by simple arithmetic like `$0400 + row * 40`. You must
use the interleaving formula, or maintain a lookup table. Every Apple II
program that ever scrolled the screen or positioned a cursor had to know this
formula.

Cross-reference: `walkthrough_main_phase3.md` Section 4 introduced this same
formula in the context of `textRowAddr` in `main.go`. Two differences are
worth noting between the two implementations:

(a) **Name**: The function is called `textRowAddr` in `main.go` and
`textLineAddr` in `video.go`. The names are synonyms; both refer to the
starting address of a text row.

(b) **Term order**: `main.go` writes `row/8 * 0x28` before `row%8 * 0x80`,
while `video.go` writes `row%8 * 0x80` first. Since these terms are added
together and addition is commutative, the formulas are mathematically
equivalent and produce identical results for all 24 row values. The order
difference is purely stylistic.

### How the Code Works

The formula breaks into three additive terms:

**Term 1 -- Base**: `0x0400` ($0400 = 1,024 decimal). Text page 1 always
starts here. This is fixed.

**Term 2 -- Position within group**: `uint16(row%8) * 0x80`. The 24 rows
are divided into three groups of 8. Within each group, successive rows are
`$80` (128 decimal) bytes apart. `row % 8` gives the position within the
current group (0 through 7).

**Term 3 -- Which third**: `uint16(row/8) * 0x28`. The three groups start
at offsets `$00`, `$28`, and `$50` from the base (that is, 0, 40, and 80
decimal bytes past `$0400`). `row / 8` gives the group index (0, 1, or 2)
in integer (floor) division. `$28` = 40 decimal = exactly one row of 40
character columns.

Three examples worked out:

- **Row 0**: `$0400 + (0%8)*$80 + (0/8)*$28` = `$0400 + 0 + 0` = `$0400` (1,024)
- **Row 1**: `$0400 + (1%8)*$80 + (1/8)*$28` = `$0400 + $80 + 0` = `$0480` (1,152)
- **Row 8**: `$0400 + (8%8)*$80 + (8/8)*$28` = `$0400 + 0 + $28` = `$0428` (1,064)

Row 8's address (`$0428`, 1,064) is less than row 1's address (`$0480`, 1,152).
This confirms the non-linear nature of the layout: higher row numbers do not
always mean higher addresses.

The `uint16()` casts on `row%8` and `row/8` are required because `row` is
declared as `int`, and `0x0400` is a `uint16`. Go does not implicitly convert
between numeric types, so the cast is explicit.

The test `TestTextLineAddresses` in `video_test.go` verifies all 24 addresses
against the known table, confirming the formula is correct for every row.

### Diagram 5: The Full Address Table (annotated)

```text
  Third 0 (row/8=0)    Third 1 (row/8=1)    Third 2 (row/8=2)
  offset +$00          offset +$28          offset +$50
  ----------------     ----------------     ----------------
  Row  0: $0400        Row  8: $0428        Row 16: $0450
  Row  1: $0480        Row  9: $04A8        Row 17: $04D0
  Row  2: $0500        Row 10: $0528        Row 18: $0550
  Row  3: $0580        Row 11: $05A8        Row 19: $05D0
  Row  4: $0600        Row 12: $0628        Row 20: $0650
  Row  5: $0680        Row 13: $06A8        Row 21: $06D0
  Row  6: $0700        Row 14: $0728        Row 22: $0750
  Row  7: $0780        Row 15: $07A8        Row 23: $07D0

  Formula: addr = $0400 + (row % 8) * $80 + (row / 8) * $28
                   base    position gap      third gap
                   (1024)  (128 bytes)       (40 bytes)

  Note: "third gap" $28 = 40 decimal = exactly one row of text columns.
        "position gap" $80 = 128 decimal = 3 thirds * 40 columns + 8 unused gap bytes.

  Maximum address: row 23, col 39 = $07D0 + 39 = $07F7 (2,039 decimal)
  This is well within the 65,536-byte RAM array (max index 65,535).
```

### Diagram 6: Why the Interleaving Exists (hardware explanation)

```text
  The Apple II video counter reuses address bits between horizontal and
  vertical timing to save chips. The screen address is composed as:

  Vertical counter high bits  -->  row / 8  (which third: 0, 1, or 2)
  Vertical counter low bits   -->  row % 8  (position within third: 0..7)
  Horizontal counter bits     -->  column offset (0..39)

  Linear layout (what programmers wanted):
    Row 0: $0400, Row 1: $0428, Row 2: $0450, ...

  Interleaved layout (what the hardware produces):
    Row 0: $0400, Row 1: $0480, Row 2: $0500, ...

  The shared counter logic saved several 74LS-series TTL chips.
  The cost: software must use a formula or lookup table to find
  each row's starting address. The hardware is simpler; the software
  pays the complexity tax.
```

### Real-World Analogy

A library where books are shelved not by catalog number, but by a hash of
their call number. The shelving system optimizes for the librarian's workflow
(hardware counter sharing) at the expense of the patron who must consult a
map (the formula) to find each shelf. The books are all there and correctly
placed -- but you cannot find them by walking straight through the shelves
in order.

---

## Section 6: RenderText() -- The Triple-Nested Render Loop

### What It Is

`RenderText()` is the main rendering function. It converts 960 bytes of Apple
II screen RAM into 215,040 bytes of RGBA pixel data, called 60 times per
second. It is the heart of the video package.

```go
func (v *Video) RenderText() {
    // flash timer (Section 4)
    v.flashCounter++
    if v.flashCounter >= 16 {
        v.flashCounter = 0
        v.FlashState = !v.FlashState
    }

    for row := 0; row < TextRows; row++ {
        baseAddr := textLineAddr(row)
        for col := 0; col < TextCols; col++ {
            screenByte := v.RAM[baseAddr+uint16(col)]
            for scanline := 0; scanline < CharH; scanline++ {
                pixels, inverse := CharGenROM(screenByte, scanline)
                if screenByte >= 0x40 && screenByte < 0x80 && v.FlashState {
                    inverse = true
                }
                py := row*CharH + scanline
                for px := 0; px < CharW; px++ {
                    lit := (pixels>>uint(px))&1 != 0
                    if inverse { lit = !lit }
                    offset := (py*ScreenW + col*CharW + px) * 4
                    if lit {
                        v.Pixels[offset+0] = ColorOn[0]
                        // ... etc
                    } else {
                        v.Pixels[offset+0] = ColorOff[0]
                        // ... etc
                    }
                }
            }
        }
    }
}
```

### Why It Matters

This is where all prior walkthroughs converge into a single operation. The
bus routes CPU writes to RAM (`walkthrough_bus.md` Section 6). Screen bytes
use the Apple II encoding -- inverse in `$00`-`$3F`, flash in `$40`-`$7F`,
normal in `$80`-`$FF` (`walkthrough_chargen.md` Section 3). `CharGenROM`
decodes them into pixel bits (`walkthrough_chargen.md` Section 4). And
`RenderText()` consumes those bits, applies the flash override, and writes
colored pixels to the frame buffer. Everything prior was preparation for this
function.

### How the Code Works

**Layer 1 -- Row loop** (lines 45-46 of video.go):

```go
for row := 0; row < TextRows; row++ {
    baseAddr := textLineAddr(row)
```

Iterates 24 rows (0-23). For each row, calls `textLineAddr(row)` to get the
interleaved base address. This is the formula from Section 5. `baseAddr` is
`uint16`, matching the type returned by `textLineAddr`.

**Layer 2 -- Column loop** (lines 48-49):

```go
for col := 0; col < TextCols; col++ {
    screenByte := v.RAM[baseAddr+uint16(col)]
```

Iterates 40 columns (0-39). Reads the screen byte directly from RAM by
indexing `v.RAM[baseAddr + uint16(col)]`. The `uint16(col)` cast is required
because `col` is `int` and `baseAddr` is `uint16`; Go does not auto-promote
between types in arithmetic expressions. The maximum index reached here is
`$07D0 + 39 = $07F7` (2,039 decimal) for row 23, col 39 -- well within the
65,536-byte RAM slice. Go's runtime bounds checking provides the safety net
for any unexpected out-of-range value.

**Layer 3 -- Scanline loop** (lines 51-52):

```go
for scanline := 0; scanline < CharH; scanline++ {
    pixels, inverse := CharGenROM(screenByte, scanline)
```

Iterates 8 scanlines (0-7) per character cell. Calls `CharGenROM` for each
scanline. `CharGenROM` returns a `uint8` of pixel bits (`pixels`) and a
`bool` indicating whether the character is in inverse video (`inverse`). See
`walkthrough_chargen.md` Section 4 for the full `CharGenROM` implementation
and its worked examples.

**Flash override** (lines 55-57):

```go
if screenByte >= 0x40 && screenByte < 0x80 && v.FlashState {
    inverse = true
}
```

The condition checks three things simultaneously:
1. `screenByte >= 0x40` (64): the byte is in the flash range, not the fixed
   inverse range (`$00`-`$3F`).
2. `screenByte < 0x80` (128): strict less-than -- `$80` and above are normal
   characters, not flash characters.
3. `v.FlashState`: the flash timer is currently in the "show inverted" phase.

If all three are true, `inverse` is forced to `true` regardless of what
`CharGenROM` returned. During the other half-cycle when `FlashState` is
`false`, this condition is never true, and the character renders normally.

**Layer 4 -- Pixel loop** (lines 60-79):

```go
py := row*CharH + scanline
for px := 0; px < CharW; px++ {
    lit := (pixels>>uint(px))&1 != 0
    if inverse { lit = !lit }
    offset := (py*ScreenW + col*CharW + px) * 4
    if lit {
        v.Pixels[offset+0] = ColorOn[0]
        v.Pixels[offset+1] = ColorOn[1]
        v.Pixels[offset+2] = ColorOn[2]
        v.Pixels[offset+3] = ColorOn[3]
    } else {
        v.Pixels[offset+0] = ColorOff[0]
        v.Pixels[offset+1] = ColorOff[1]
        v.Pixels[offset+2] = ColorOff[2]
        v.Pixels[offset+3] = ColorOff[3]
    }
}
```

Key details line by line:

- `py = row*CharH + scanline`: the absolute pixel Y coordinate in the frame
  buffer. Row 0 scanline 0 gives `py = 0`. Row 1 scanline 3 gives
  `py = 1*8 + 3 = 11`. Row 23 scanline 7 gives `py = 23*8 + 7 = 191`.
  Range: 0-191.

- `(pixels>>uint(px))&1`: extract one bit from the 7-bit pixel row. The
  `>>` operator shifts `pixels` right by `px` positions, placing the target
  bit at position 0, then `& 1` masks off all other bits. The `uint(px)` cast
  is required because Go's shift operator demands an unsigned shift count.
  `px` is declared as `int`, which is signed -- hence the explicit cast.

  **Bit order**: Bit 0 (`px = 0`) is the leftmost pixel. Bit 6 (`px = 6`)
  is the rightmost. This is the OPPOSITE of the usual convention where bit 7
  (MSB) is the leftmost position. This bit ordering was established in the
  chargen array encoding -- see `walkthrough_chargen.md` Section 1 for the
  full derivation. The important takeaway: the bit extraction and the chargen
  array encoding use the same convention, so they cancel out correctly.

- `if inverse { lit = !lit }`: a boolean flip. A lit bit in an inverse
  character becomes dark; a dark bit becomes lit. This is what produces the
  black-on-green appearance of inverse-mode characters.

- `offset = (py*ScreenW + col*CharW + px) * 4`: the pixel buffer index
  formula. Breaking it down:
  - `py * ScreenW` = byte offset to the start of pixel row `py`
    (280 pixels per row).
  - `col * CharW` = horizontal offset to the start of character column `col`
    (7 pixels per column).
  - `+ px` = the specific pixel within the character cell (0-6).
  - `* 4` = four bytes per pixel (RGBA).

- Four individual byte writes copy R, G, B, A from `ColorOn` or `ColorOff`.
  The four scalar writes are the fastest way to copy 4 known bytes in a hot
  inner loop -- a `copy()` call would add function call overhead.

### Diagram 7: The Triple-Nested Loop Structure

```text
  for row = 0..23:                          (24 text rows)
    baseAddr = textLineAddr(row)
    |
    +-- for col = 0..39:                    (40 text columns)
    |     screenByte = RAM[baseAddr + col]
    |     |
    |     +-- for scanline = 0..7:          (8 pixel rows per char)
    |     |     pixels, inverse = CharGenROM(screenByte, scanline)
    |     |     [flash override if $40-$7F and FlashState]
    |     |     |
    |     |     +-- for px = 0..6:          (7 pixels per char row)
    |     |           bit = (pixels >> px) & 1
    |     |           if inverse: flip bit
    |     |           write RGBA to Pixels[offset..offset+3]

  Total iterations: 24 * 40 * 8 * 7 = 53,760 pixel writes per frame
  Total bytes written: 53,760 * 4 = 215,040 = ScreenW * ScreenH * 4
```

### Diagram 8: Pixel Offset Calculation -- Worked Example

This example matches `TestRenderTextBasic` in `video_test.go`, which places
`$C1` (193, normal 'A') at `$0400` (row 0, col 0) and checks pixel (3, 0).

```text
  Character 'A' at row=0, col=0, scanline=0, px=3 (the peak of 'A'):

  CharGenROM($C1, 0) returns pixels = 0x08 (binary 00001000)
    bit 3 is set => column 3 is lit

  py = 0 * 8 + 0 = 0
  offset = (0 * 280 + 0 * 7 + 3) * 4 = 3 * 4 = 12

  Pixels buffer (first 16 bytes, row 0, col 0, scanline 0):
  byte:  0  1  2  3 | 4  5  6  7 | 8  9 10 11 | 12 13 14 15 | ...
  pixel: px=0 (off) | px=1 (off) | px=2 (off) | px=3 (ON)   | ...
         R  G  B  A | R  G  B  A | R  G  B  A |  R  G  B  A |
         00 00 00 FF| 00 00 00 FF| 00 00 00 FF| 33 FF 33 FF |

  TestRenderTextBasic checks:
    offset = (0*280 + 3) * 4 = 12
    v.Pixels[12] == ColorOn[0] == 0x33  --> PASS (pixel (3,0) is green)
    v.Pixels[0]  == ColorOff[0] == 0x00 --> PASS (pixel (0,0) is black)

  Pixel (0,0) is dark because bit 0 of 0x08 (binary 00001000) is 0.
  Pixel (3,0) is lit because bit 3 of 0x08 (binary 00001000) is 1.
```

### Real-World Analogy

A dot-matrix printer printing a page of text. The print head moves row by row
(outer loop), column by column (middle loop), and for each character cell it
strikes 8 rows of 7 dots (inner loops). The character ROM (CharGenROM) tells
the head which dots to strike for each row. The ink ribbon is either phosphor
green (ColorOn) or absent (ColorOff). The result is 24 rows of 40 characters
rendered in 215,040 colored dots.

---

## Section 7: Putting It Together

This section closes the forward pointers for Steps 2 and 5 from the pipeline
table in `walkthrough_chargen.md` Section 5. That table listed two steps as
"covered in walkthrough_video.md (next document)." Both are now resolved.

### The Complete Pipeline

```text
  1. CPU (or ROM code) writes $C1 to address $0428
     (walkthrough_bus.md Section 6 -- write routing through the bus)
     ($0428 is row 8, col 0 -- the interleaved address for the start of row 8)

  2. RenderText() iterates over screen RAM                [THIS document, Section 6]
     baseAddr = textLineAddr(8) = $0428
     screenByte = v.RAM[$0428] = $C1

  3. CharGenROM($C1, scanline) called for each of 8 scanlines
     (walkthrough_chargen.md Section 4)

  4. CharGenROM decodes $C1 -> charIndex $41 (65), inverse = false
     Looks up charGen[65*8 + scanline] -> pixel bits
     (walkthrough_chargen.md Section 4)

  5. RenderText() converts pixel bits to RGBA             [THIS document, Section 6]
     offset = (py*280 + 0*7 + px) * 4
     Writes ColorOn or ColorOff bytes to v.Pixels[offset..offset+3]
```

### Cross-Reference Table -- All Pointers Resolved

| Step | Walkthrough                  | Section                     |
|------|------------------------------|-----------------------------|
| 1    | walkthrough_bus.md           | Section 6 (write routing)   |
| 1    | walkthrough_main_phase3.md   | Section 4 (screen memory)   |
| 2    | THIS document                | Section 6 (RenderText loop) |
| 3-4  | walkthrough_chargen.md       | Section 4 (CharGenROM)      |
| 5    | THIS document                | Section 6 (pixel writes)    |

### The SDL Upload Step

After `RenderText()` fills `v.Pixels`, `main.go` uploads the buffer to the
SDL texture with:

```go
texture.Update(nil, unsafe.Pointer(&vid.Pixels[0]), int(video.ScreenW)*4)
```

The second argument passes a raw pointer to the first byte of the `Pixels`
slice. The third argument is the pitch -- the number of bytes per row of
pixels. `video.ScreenW * 4 = 280 * 4 = 1,120 bytes per row`. SDL uses the
pitch to advance its internal pointer by one pixel row at a time when
uploading the texture. Because `v.Pixels` is in RGBA32 format with exactly
`ScreenW * 4` bytes per row, the pitch matches the buffer layout exactly and
no row-padding or format conversion is needed.

The `unsafe.Pointer` cast bypasses Go's type system to pass a Go slice header
as a C-style memory pointer. This is the standard pattern for Go-to-SDL
memory transfer. The safety guarantee comes from the fact that `v.Pixels` is
a 215,040-byte slice that outlives the `texture.Update` call.

For the full SDL window setup and the `main.go` render loop, see
`walkthrough_main_phase3.md` Section 2.

---

## Section 8: Design Decisions and Hardware Reality

**Q1: Why does Video take a RAM slice instead of going through the bus?**

Direct slice access is faster than bus reads. The bus's `Read` method scans
a slice of registered devices from tail to head on every call -- a useful
abstraction for routing, but expensive for reading 960 bytes per frame at 60
fps. For text page 1, which is always at `$0400`-`$07FF` in unbanked RAM,
skipping the bus is correct. The trade-off is coupling: the `Video` struct
depends on the internal layout of `memory.RAM.Data`, rather than going
through the bus abstraction. This will need reworking if bank switching
(the language card) is ever added, since the language card can remap the
lower 48 KB of address space.

**Q2: Why 4 bytes per pixel instead of 1 bit per pixel?**

SDL's RGBA32 texture format expects 4 bytes per pixel. Converting from 1-bit
source data (CharGenROM output) to 4-byte RGBA at render time -- 53,760
conversions per frame -- is simpler and more maintainable than packing bits
into a 1-bit buffer and converting at upload time. The 215,040-byte buffer
is trivial on modern hardware. Simplicity wins.

**Q3: Why is the flash counter threshold 16, not some other number?**

60 fps / 16 frames = 3.75 toggles/sec. One blink cycle = 2 toggles,
so 3.75 / 2 = 1.875 Hz. The real Apple II's flash rate is approximately
1.875 Hz, derived from the vertical blanking counter in the video hardware.
16 is a convenient threshold that produces a close match to the real
hardware's flash rate. Note that `16` is a magic number in the
source -- a named constant like `flashPeriodFrames = 16` would be cleaner,
but the current code includes the frequency derivation in the surrounding
comment.

**Q4: Why does the pixel loop write 4 individual bytes instead of using copy()?**

Four scalar writes (`v.Pixels[offset+0] = ...` through `offset+3`) avoid a
function call. `copy()` introduces function call overhead and a slice header
allocation for a 4-byte copy. In a hot inner loop that runs 53,760 times per
frame, the overhead is measurable. The Go compiler may also vectorize or
inline the four scalar writes but cannot as easily optimize a `copy()` call
in the same context. This is a deliberate micro-optimization in a
performance-critical path.

**Q5: How does the real Apple II video circuitry compare?**

The real Apple II uses hardware DMA -- the video circuitry steals bus cycles
from the CPU on every horizontal blanking interval to read screen bytes
directly. It reads one byte per character clock, feeds it to the 2513
character ROM, and shifts 7 pixels per character clock to the composite NTSC
output. The CPU and video circuitry share the bus on alternating clock phases.
The CPU is not involved in the video data path at all during display. Our
emulator replaces this entire hardware pipeline with a software loop that
reads RAM and writes RGBA pixels. The mechanism is completely different; the
observable result -- 280x192 green-on-black text -- is identical.

**Q6: Why are ColorOn/ColorOff declared with `var` instead of `const`?**

Go only allows `const` for scalar types: integers, floating-point numbers,
strings, booleans, and runes. Array types like `[4]uint8` cannot be declared
as constants in Go -- the language specification does not support it. The
`var` declaration is the only available option. The values are never modified
after package initialization, so they are effectively constant, but the
language cannot enforce that guarantee for array types. This is a common Go
idiom for "compile-time-known, never-mutated" non-scalar values.

**Q7: Why does the full screen re-render every frame?**

Simplicity. A dirty-region tracker would skip re-rendering character cells
whose screen bytes have not changed since the last frame, potentially saving
most of the per-frame work. But a dirty tracker requires recording which RAM
addresses the CPU has written since the last frame -- either by intercepting
every bus write (adding overhead to the hot path) or by comparing screen RAM
to a shadow copy on every frame (equivalent work to just re-rendering). At
60 fps, 53,760 pixel writes take microseconds on any modern machine. The full
re-render is correct, easy to reason about, and performant enough. Complexity
is not justified by the performance gain.

---

## Section 9: Summary and What Is Next

`video.go` is 99 lines of Go that bridge the gap between 960 bytes of Apple
II screen RAM and a 215,040-byte RGBA frame buffer that SDL can display. The
`textLineAddr()` function handles the Apple II's famously non-linear
interleaved memory layout, turning a simple row number into the correct RAM
address. `CharGenROM()` (from `chargen.go`) converts each screen byte and
scanline into 7 pixel bits. The flash timer in `RenderText()` toggles
`FlashState` every 16 frames to make `$40`-`$7F` screen bytes blink at
1.875 Hz. The pixel loop extracts each bit, applies the inverse or flash
override, and writes the appropriate RGBA color to the pixel buffer. The
result is an authentic 280x192 green-screen Apple II display.

> **Take-home idea**: The video renderer is a four-deep nested loop that
> converts 960 bytes of Apple II screen memory into 215,040 bytes of RGBA
> pixel data. The `textLineAddr()` formula handles the Apple II's notorious
> interleaved memory layout. `CharGenROM` provides the pixel bits. The flash
> timer makes `$40`-`$7F` characters blink at 1.875 Hz. Two RGBA colors --
> phosphor green and black -- paint every pixel. The result is an authentic
> 280x192 green-screen display.

### Quick Reference

| Item              | Value                                        |
|-------------------|----------------------------------------------|
| File              | `video/video.go`                             |
| Screen size       | 280 x 192 pixels (40x24 chars x 7x8 px)     |
| Pixel buffer      | 215,040 bytes (280 * 192 * 4 RGBA)           |
| ColorOn           | {$33, $FF, $33, $FF} = (51, 255, 51, 255)   |
| ColorOff          | {$00, $00, $00, $FF} = (0, 0, 0, 255)       |
| Flash rate        | ~1.875 Hz (toggle every 16 frames at 60 fps) |
| Screen RAM        | $0400-$07FF (1,024-2,047), text page 1       |
| Address formula   | $0400 + (row%8)*$80 + (row/8)*$28            |
| Max RAM address   | $07D0 + 39 = $07F7 (row 23, col 39)          |
| Public API        | `NewVideo(ram)`, `RenderText()`              |
| Per-frame work    | 53,760 pixel writes (24 * 40 * 8 * 7)       |
| ColorOn/Off type  | `var [4]uint8` (Go cannot `const` arrays)    |

### What Is Next

This is the final walkthrough in the series. The emulator now has a working
CPU (verified by the Klaus Dormann test suite), a bus router, RAM and ROM
devices, I/O softswitches, a character generator, and a video renderer. The
series is complete.

The natural next areas for the emulator are: lo-resolution graphics mode
(GR, using the same `$0400`-`$07FF` memory range as the text display but
interpreting each byte as two 4-pixel color blocks), hi-resolution graphics
mode (HGR, using `$2000`-`$3FFF`), text page 2 at `$0800`-`$0BFF`, a
functional keyboard input loop, and eventually Disk II support. Each of these
would be a new package alongside `video/`, following the same Clean
Architecture pattern established here: a struct, a constructor, and a
render or update method.

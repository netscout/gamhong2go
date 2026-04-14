# Educational Walkthrough: `video/chargen.go` -- The Character Generator ROM

> This is the first walkthrough in iteration 3 of the series -- the video
> rendering package. It covers `video/chargen.go`, which emulates the
> Apple II's character generator ROM in software. You should have read all
> six previous walkthroughs before starting this one. Each section follows
> the familiar What It Is / Why It Matters / How the Code Works /
> Real-World Analogy pattern.

---

## Section 0: Background -- What Is a Character Generator ROM?

### What It Is

In the main walkthrough (`walkthrough_main_phase3.md` Section 4, the
`textRowAddr` section), we saw a raw hex dump of text screen memory at
`$0400`-`$07FF` (1,024 through 2,047). Those bytes represent characters --
each byte stands for one character cell on the 40-column display. But how
do characters become pixels on screen? A number like `$C1` (193) tells you
nothing about how to draw a letter. That translation is the job of the
**character generator ROM**.

On the real Apple II, a chip called the **Signetics 2513 Character
Generator ROM** holds bitmap data for 64 characters. Each character is
stored as 5 columns by 8 rows. The Apple II video circuitry adds two blank
columns to pad each row to 7 pixels total (5 from the chip + 2 blank = 7).
The video circuitry reads a screen byte from RAM, uses the lower bits of
that byte as an address into this ROM, and receives one row of pixel dots
for each scanline. The CPU never accesses this chip; it lives entirely on
the video side of the machine, invisible to the 6502 and the bus.

Our emulator has no physical chip, so `chargen.go` encodes the same
bitmaps in a Go array and exposes one function for the video renderer to
call -- the same strategy used for the firmware ROM in `walkthrough_memory.md`
(replacing a physical chip with a Go data structure). Our code stores 7-bit
patterns directly, pre-padded -- the 5+2 column splitting is handled at
encoding time, and the effect is identical to the original hardware.

**Diagram 1: Where the Character Generator Sits in the Video Pipeline**

```text
  Screen RAM         Character           Pixel
  ($0400-$07FF)      Generator ROM       Output
  +----------+       +----------+       +----------+
  | byte $C1 | ----> | look up  | ----> | 7 pixels |
  | (= 'A')  |       | row 0-7  |       | per row  |
  +----------+       +----------+       +----------+
       |                  |                   |
  CPU writes here    video HW reads      electron beam
  (or ROM does)      automatically       draws dots
```

For comparison: on MS-DOS CGA/EGA systems, the character ROM is located in
the video BIOS ROM at segment `$F000` (61,440 in decimal -- near the top of
the first 64 KB). Same concept, different location. Every text-mode computer
of that era needed one.

In this emulator, `chargen.go` is the character generator ROM. The file
`video/video.go` (covered in the next walkthrough) is the video circuitry
that calls it -- it iterates over screen memory and, for every character
cell on every scanline, asks `chargen.go` for a row of pixel bits.

---

## Section 1: The `charGen` Array -- 128 Characters as Bitmaps

### What It Is

```go
var charGen [128 * 8]uint8
```

This is a package-level array of 1,024 bytes (128 characters multiplied by
8 rows each). Every character in the Apple II's character set occupies
exactly 8 consecutive bytes in this array -- one byte per pixel row, from
top (row 0) to bottom (row 7). Each byte uses only 7 of its 8 bits. Bit 0
is the leftmost pixel column; bit 6 is the rightmost. Bit 7 is never used.

### Why It Matters

The original Signetics 2513 chip held only 64 characters -- uppercase
letters, digits, and punctuation symbols. The Apple II+ expanded this to
128 characters with the Applesoft firmware revision. Our array holds the
full 128-entry table.

The 7-bit width is not a mistake and not a simplification. It is a direct
reflection of the Apple II hardware. The display is exactly 280 pixels wide
in 40-column text mode. 280 divided by 40 columns equals 7 pixels per
character column. Wozniak chose this geometry to maximize horizontal
resolution on a consumer television. The 8th bit of each row byte is
simply unused -- there is no 8th pixel column in text mode.

Because `charGen` is declared at package level and is an array (not a
slice), Go zero-initializes every byte before `main()` runs. Any character
code that has no glyph defined will render as a blank row of all zeroes.
This is the same behavior as the empty cells on the physical ROM.

### How the Code Works

`[128 * 8]uint8` is a fixed-size array. Go requires the size to be a
constant expression, and `128 * 8` is evaluated at compile time to 1,024.
A slice (`[]uint8`) would also hold 1,024 bytes, but a fixed-size array
makes the structure explicit: there are exactly 128 characters and each
takes exactly 8 bytes. No bounds ambiguity, no length field overhead.

The index formula for any character row is:

```text
  index = charCode * 8 + row
```

To find row 0 of the letter 'A' (ASCII code `$41`, decimal 65):
`65 * 8 + 0 = 520`. Row 7 of 'A' is at index `65 * 8 + 7 = 527`.

**Diagram 2: How 8 bytes encode the letter 'A'**

```text
  Pixel columns:    0 1 2 3 4 5 6       (7 pixels wide, bit 7 unused)
  Bit positions:    0 1 2 3 4 5 6       (bit 0 = LEFT, bit 6 = RIGHT)

  charGen[520] = 0x08 = 0b0001000       ...*...   row 0  top of the peak
  charGen[521] = 0x14 = 0b0010100       ..*.*..   row 1  shoulders
  charGen[522] = 0x22 = 0b0100010       .*...*.   row 2  sides
  charGen[523] = 0x22 = 0b0100010       .*...*.   row 3  sides
  charGen[524] = 0x3E = 0b0111110       .*****.   row 4  crossbar
  charGen[525] = 0x22 = 0b0100010       .*...*.   row 5  legs
  charGen[526] = 0x22 = 0b0100010       .*...*.   row 6  legs
  charGen[527] = 0x00 = 0b0000000       .......   row 7  blank descender

  index = charCode * 8 + row
  'A' = 0x41 (65), so base index = 65 * 8 = 520
```

The bit order deserves extra attention because it is the opposite of what
most programmers expect. In a normal binary number, bit 7 is the most
significant (leftmost) bit. Here, **bit 0 is the leftmost pixel column**.
The Apple II video hardware shifts out pixels starting from the low bit.

Working through row 0 as an example: `0x08` in 8-bit binary is `00001000`.
We only care about the lower 7 bits, so we read it as 7-bit binary
`0001000`. That means:

```text
  bit 0 = 0  ->  column 0 off
  bit 1 = 0  ->  column 1 off
  bit 2 = 0  ->  column 2 off
  bit 3 = 1  ->  column 3 ON   <-- the tip of the 'A' peak
  bit 4 = 0  ->  column 4 off
  bit 5 = 0  ->  column 5 off
  bit 6 = 0  ->  column 6 off
```

Result: a single dot at column 3, which is the center of the 7-pixel cell.
That is the topmost point of the letter 'A'.

Row 4 (`0x3E` = `0b0111110`) has bits 1 through 5 set -- columns 1, 2, 3,
4, and 5 all lit. That is the horizontal crossbar of the letter 'A',
spanning five columns with one blank column on each side.

### Real-World Analogy

Think of a rubber-stamp set for addresses or signs. Each stamp is a
pre-carved 7x8 dot pattern -- you cannot change the shape, only choose
which stamp to press. The character generator is the box holding all the
stamps. The video circuitry picks the right stamp by character code and
presses it onto the screen row by row, one strip of dots at a time. The
`charGen` array is that box of stamps, permanently shaped at the factory.

---

## Section 2: The `init()` Function -- Building the Glyph Table

### What It Is

```go
func init() {
    glyphs := map[byte][8]uint8{ ... }
    for ch, rows := range glyphs {
        base := int(ch) * 8
        for row := 0; row < 8; row++ {
            charGen[base+row] = rows[row]
        }
    }
}
```

Go's `init()` function runs automatically before `main()` begins. The
`walkthrough_main_phase3.md` Section 5 explains this pattern in the context
of the disassembler tables. The same mechanism is used here: a package-
level array needs to be filled before any code can use it, and `init()` is
the guaranteed-automatic way to do that without requiring a caller.

This particular `init()` builds the glyph table from a map literal and
copies the results into the flat `charGen` array.

### Why It Matters

The glyph definitions are the soul of the character generator. Without
this function running, `charGen` is 1,024 zero bytes and every character
on screen renders as a blank row. The visual output of the entire text mode
depends on this one `init()` call completing successfully.

Using `init()` means that any package in the emulator that imports `video`
automatically gets the glyph table populated. No explicit call to
`InitCharGen()` or `LoadGlyphs()` is needed. The contract is: import the
package, the glyphs are there. This is a deliberate Go idiom, analogous to
how `memory.LoadROM` populates ROM data as part of its constructor (see
`walkthrough_memory.md`).

### How the Code Works

**The map literal**: The function begins by declaring a local variable:

```go
glyphs := map[byte][8]uint8{
    ' ': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
    '!': {0x08, 0x08, 0x08, 0x08, 0x08, 0x00, 0x08, 0x00},
    // ... 62 more entries ...
    '_': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3E, 0x00},
}
```

The key type is `byte`, and Go allows byte literals like `'A'` as map
keys -- the compiler converts them to their ASCII numeric values
automatically. `'A'` becomes `0x41` (65), `'!'` becomes `0x21` (33), and
so on. The value type `[8]uint8` is a fixed-size array of 8 bytes, one per
pixel row.

This map-literal style is a readability choice. Writing `'A': {0x08, 0x14,
0x22, 0x22, 0x3E, 0x22, 0x22, 0x00}` is self-documenting -- any reader
immediately sees which character the bitmap belongs to. The alternative
would be a raw 1,024-byte array where index 520 happens to be row 0 of
'A'. The map key is thrown away after `init()` completes; the entire
`glyphs` variable is garbage-collected. Only the flat `charGen` array
survives.

**The copy loop**:

```go
for ch, rows := range glyphs {
    base := int(ch) * 8
    for row := 0; row < 8; row++ {
        charGen[base+row] = rows[row]
    }
}
```

`ch` is the ASCII code of the character -- a `byte`. `base` is the
starting index in the flat array: `int(ch) * 8`. The `int()` conversion is
necessary because `ch` is `uint8` and `uint8` arithmetic wraps at 256 --
for `ch = 127`, the product `127 * 8 = 1016` fits in an `int` but overflows
a `uint8`. The inner loop copies all 8 row bytes sequentially.

**Diagram 3: `charGen` array memory layout**

```text
  Index:  0    7  8   15  16  23        504  511  512  519  520  527
         +------+------+------+-- ... --+------+------+------+------+
         | 0x00 | 0x01 | 0x02 |         | 0x3F | 0x40 | 0x41 | ...
         | (NUL)| (SOH)| (STX)|         | '?'  | '@'  | 'A'  |
         | 8 row| 8 row| 8 row|         | 8 row| 8 row| 8 row|
         +------+------+------+-- ... --+------+------+------+------+

  Character code * 8 = starting index
  Only codes 0x20 (32) through 0x5F (95) are populated by the glyph map
  All others remain zero (blank)
```

**Character coverage**: The map defines characters from `0x20` (32, space)
through `0x5F` (95, underscore) -- that is exactly 64 characters. This
range covers the printable ASCII symbols, digits, uppercase letters, and
the characters `@`, `[`, `\`, `]`, `^`, and `_`. Character codes
`0x00`-`0x1F` (control characters) and `0x60`-`0x7F` (lowercase letters
and symbols) are not in the map and remain zero-filled.

**A note on the source comment**: Line 14 of `chargen.go` reads:

```text
  // Define printable ASCII characters 0x20-0x7F.
```

If you scroll to the end of the map, the last entry is `'_'` (0x5F, 95).
Characters `0x60` through `0x7F` -- lowercase `a`-`z`, braces, pipe, and
tilde -- are NOT defined. The comment claims a range of 96 characters; the
actual range is 64. The Apple II+ had no lowercase in its base character
ROM; lowercase came later with the Apple IIe (1983) and aftermarket
character generator chips. Always read the code, not just the comments.

### Real-World Analogy

Like burning a PROM chip (Programmable Read-Only Memory -- a chip you can
write once using a hardware programmer and voltage pulses) at the factory.
You write the pattern data once, apply the programming voltage, and the
data is sealed in silicon forever after. `init()` is the "burning" step;
the `charGen` array is the sealed ROM. Once `init()` returns, `charGen`
is never written again during emulator execution.

---

## Section 3: The Apple II Character Encoding

### What It Is

The Apple II does **not** use ASCII directly for screen display. Screen
memory at `$0400`-`$07FF` uses its own encoding that maps each of the 256
possible byte values into one of three display modes: inverse, flashing,
or normal. This encoding is baked into the byte value itself -- the same
number that selects which glyph to show also determines how to draw it.

### Why It Matters

This encoding is the reason `CharGenROM()` needs a decode step before
looking up the glyph. A programmer coming from a DOS background might
expect to write ASCII `$41` (65, 'A') to screen memory and see a normal
letter A. On the Apple II, writing `$41` produces a **flashing** 'A'. To
get a normal 'A', you write `$C1` (193). To get an inverse 'A' (black
letter on white background), you write `$01` (1).

Without understanding this encoding, the switch statement in `CharGenROM`
looks arbitrary. With it, every case is obvious.

### How the Code Works

**Diagram 4: The 256-byte screen code map**

```text
  Screen byte     Display mode     Character source
  -----------     ------------     ----------------
  $00-$3F         INVERSE          chars $40-$7F (@ A-Z [ \ ] ^ _
   (0-63)         (black on white    plus $60-$7F which are blank)
                   background)       add $40 (64) to get charGen index

  $40-$7F         FLASHING         chars $40-$7F (@ A-Z [ \ ] ^ _)
   (64-127)       alternates        charGen index = screen byte as-is
                  inverse/normal
                  at ~1.875 Hz

  $80-$9F         NORMAL           chars $00-$1F (control char symbols)
   (128-159)                        mask off bit 7 -> index $00-$1F

  $A0-$DF         NORMAL           chars $20-$5F (space ! " ... _)
   (160-223)                        mask off bit 7 -> index $20-$5F

  $E0-$FF         NORMAL           chars $60-$7F (` a-z { | } ~)
   (224-255)                        mask off bit 7 -> index $60-$7F
```

The key rule is: if bit 7 is set (`$80`-`$FF`), the character is normal
display. If bit 7 is clear (`$00`-`$7F`), the character is either inverse
(if the byte is below `$40`) or flashing (if the byte is `$40` or above).
This is completely unlike ASCII, where bit 7 marks "extended" or
"high-ASCII" characters.

The normal range (`$80`-`$FF`) can address all 128 charGen entries by
masking off bit 7. In practice, indices `$00`-`$1F` and `$60`-`$7F` have
no glyphs defined (all-zero rows), so normal display of those codes shows
blank cells.

For comparison, MS-DOS used a separate attribute byte for color, blink, and
inverse -- one byte for the character code, one byte for its display style.
The Apple II bakes both pieces of information into a single byte, which
halves the number of displayable characters per mode but uses less memory
per screen cell.

**Why no lowercase in inverse or flash mode**: The inverse and flash ranges
(`$00`-`$7F`) index only into charGen entries `$40`-`$7F`, which are the
64 glyphs `@`, `A`-`Z`, `[`, `\`, `]`, `^`, and `_`. There is simply no
path from a `$00`-`$7F` screen byte to a charGen entry for a lowercase
letter. Lowercase text could only appear in the normal range (`$80`-`$FF`),
and only if the lowercase glyphs were defined -- which they are not in this
emulator (the Apple II+ era character set).

A real-world example: the Applesoft BASIC prompt `]` appears in screen
memory as byte `$DD` (221). `$DD >= $80` so it is normal mode.
`$DD & $7F = $5D` (93). CharGen index 93 is `']'`. Correct.

### Real-World Analogy

Like a color-coded filing system where the same document can appear in
three cabinet zones -- a red zone (0-63) for inverse copies, a yellow zone
(64-127) for blinking copies, and a green zone (128-255) for normal copies.
The file number within each zone tells you which document. The zone itself
tells you how to present it. You cannot mix presentation styles between
zones; the zone determines the look.

---

## Section 4: `CharGenROM()` -- The Lookup Function

### What It Is

```go
func CharGenROM(screenByte uint8, row int) (pixels uint8, inverse bool)
```

This is the only exported (capitalized) function in the file. It is the
public interface between the video renderer and the character bitmap data.
The caller passes in a raw byte from screen memory and a scanline number
(0-7), and gets back a byte of pixel bits plus a flag indicating whether
the character should be drawn in inverse video.

### Why It Matters

`CharGenROM` is called 40 columns multiplied by 24 rows multiplied by 8
scanlines per character equals 7,680 times per frame. The emulator aims
for 60 frames per second, which means this function runs approximately
460,800 times per second. It must be a simple, branch-predictable lookup
with no side effects.

It is also the sole encapsulation boundary for the Apple II screen
encoding. The caller (`video.RenderText()`, covered in the next
walkthrough) does not need to know anything about the `$00`-`$3F`
inverse range or the `& 0x7F` masking. It passes a raw screen byte and
receives ready-to-use pixel data. This is the same separation-of-concerns
principle described in `walkthrough_memory.md` -- devices expose a clean
interface and hide their internal organization.

### How the Code Works

**Step 1 -- the switch statement** decodes the screen byte into a charGen
index and an inverse flag:

```go
var charIndex uint8
switch {
case screenByte < 0x40:
    charIndex = screenByte + 0x40
    inverse = true
case screenByte < 0x80:
    charIndex = screenByte
    inverse = false // flashing: alternate with inverse at ~1.875 Hz
default:
    charIndex = screenByte & 0x7F
    inverse = false
}
```

- **Case 1** (`$00`-`$3F`, 0-63): Add `$40` (64) to get the real charGen
  index. Screen byte `$01` (1) becomes charIndex `$41` (65), which is 'A'.
  `inverse = true` tells the caller to draw the glyph black-on-white.

- **Case 2** (`$40`-`$7F`, 64-127): Use the byte as-is for the charGen
  index. Flashing is NOT handled here. The function returns `inverse =
  false`. The caller (`video.RenderText()`) maintains a flash timer and
  overrides the inverse flag based on that timer. `CharGenROM` is a pure
  lookup with no state -- it knows nothing about time or frame count.
  See the next walkthrough for the flash timer implementation.

- **Default** (`$80`-`$FF`, 128-255): Mask off bit 7 with `& 0x7F` (127).
  Screen byte `$C1` (193) becomes charIndex `$41` (65), which is 'A'.
  Normal display.

**Step 2 -- bounds check**:

```go
if row < 0 || row > 7 {
    return 0, inverse
}
```

A defensive guard. If the caller passes a row outside 0-7, the function
returns zero pixels (a blank row) rather than indexing out of bounds. The
`inverse` flag is still returned as already decoded, in case the caller
needs it for other purposes.

**Step 3 -- the lookup**:

```go
pixels = charGen[int(charIndex)*8+row]
```

The `int(charIndex)` cast is important. `charIndex` is `uint8`. The maximum
value after decoding is `$7F` (127). `127 * 8 = 1016`, which is well within
`int` range but overflows `uint8` (max 255). Without the cast, the
multiplication would wrap around and produce a wrong index. The cast forces
the arithmetic to happen in `int`, where 1016 is just a normal number.

The formula `int(charIndex)*8 + row` is exactly the same index formula
used in the `init()` copy loop -- the same arithmetic that put the data
in is the same arithmetic that retrieves it.

**Diagram 5: `CharGenROM` decode flow -- worked example: normal 'A'**

```text
  Input: screenByte = $C1 (193), row = 0

  Step 1: Decode screen byte
          $C1 >= $80 --> normal mode
          charIndex = $C1 & $7F = $41 (65)
          inverse = false

  Step 2: Bounds check
          row = 0, which is in 0..7 --> OK

  Step 3: Look up pixel row
          index = 65 * 8 + 0 = 520
          charGen[520] = 0x08

  Output: pixels = 0x08 (binary 0001000), inverse = false

  Pixel rendering (done by video.go, not here):
          columns:  0 1 2 3 4 5 6
          bits:     0 0 0 1 0 0 0
          display:  . . . * . . .     <-- top of the 'A' peak
          (bit 0 = column 0 = leftmost)
```

**Second worked example: inverse 'A'**

```text
  Input: screenByte = $01 (1), row = 4

  Step 1: $01 < $40 --> inverse mode
          charIndex = $01 + $40 = $41 (65)
          inverse = true

  Step 2: row = 4 --> in 0..7 --> OK

  Step 3: index = 65 * 8 + 4 = 524
          charGen[524] = 0x3E (binary 0111110)

  Output: pixels = 0x3E (binary 0111110), inverse = true

  Pixel rendering (video.go flips all bits because inverse = true):
          columns:  0 1 2 3 4 5 6
          bits:     0 1 1 1 1 1 0   (from 0x3E)
          flipped:  1 0 0 0 0 0 1   (because inverse = true)
          display:  * . . . . . *

  This is row 4 of 'A' (the crossbar) shown in inverse video:
  the crossbar pixels are OFF and the background cells are ON.
```

### Real-World Analogy

Like a multilingual dictionary with a lookup table at the front. The screen
byte is the word you bring to the table -- but first you check the lookup
table at the front to find which page number to turn to. The encoding table
redirects you from the screen byte to the right charGen entry. The entry
itself contains the pixel pattern. The `inverse` flag is like a sticky note
clipped to the entry saying "print this page in white-on-black instead of
black-on-white."

---

## Section 5: Putting It Together -- From Screen Byte to Pixel

The complete pipeline from a CPU write to a lit pixel on screen crosses
three components. Two of them are covered here and in previous walkthroughs;
the last one is in the next document.

```text
  1. CPU (or ROM code) writes $C1 to address $0428
     (see walkthrough_bus.md Section 6 for write routing)
     (see walkthrough_main_phase3.md Section 4 for the text screen layout)

  2. video.RenderText() iterates over screen memory
     (covered in walkthrough_video.md)

  3. For each character cell, calls CharGenROM($C1, scanline)
     (THIS file)

  4. CharGenROM decodes: $C1 -> charIndex $41 (65), inverse = false
     Looks up charGen[65*8 + scanline] -> pixel bits

  5. video.RenderText() converts bits to RGBA pixels
     (covered in walkthrough_video.md)
```

Cross-reference summary:

| Step | Walkthrough                  | Section                     |
|------|------------------------------|-----------------------------|
| 1    | walkthrough_bus.md           | Section 6 (Write routing)   |
| 1    | walkthrough_main_phase3.md   | Section 4 (screen memory)   |
| 2    | walkthrough_video.md         | (next document)             |
| 3-4  | THIS document                | Section 4                   |
| 5    | walkthrough_video.md         | (next document)             |

The address `$0428` (1,064) deserves a brief note. Text page 1 starts at
`$0400` (1,024). Due to the Apple II's interleaved screen layout (covered
in `walkthrough_main_phase3.md` Section 4), `$0428` is actually the first
byte of screen row 8, not row 1. Row 1 starts at `$0480` (1,152). The
interleaving is notoriously non-linear, but for the purpose of
`CharGenROM`, none of that matters -- it receives a single byte and returns
pixel bits, regardless of which row or column that byte came from.

---

## Section 6: Design Decisions and Hardware Reality

**1. Why a flat array instead of a 2D array `[128][8]uint8`?**

A flat array with `index = char*8 + row` mirrors how the physical 2513 ROM
chip works: a single address bus with linear addressing. The real Apple II
video circuitry computes a ROM address using exactly this formula, combining
the character code and the scanline counter into a single address value.
A 2D array `[128][8]uint8` would be equally correct in Go and might read
more naturally, but the flat representation is more hardware-faithful and
makes the index arithmetic explicit.

**2. Why does the glyph map only define `0x20`-`0x5F` (64 chars)?**

The original Apple II (and Apple II+) character generator ROM contained
only uppercase letters, digits, and punctuation symbols -- no lowercase.
Lowercase arrived with the Apple IIe in 1983 and through third-party
aftermarket character generator chips (such as the Videx Videoterm) before
that. This emulator targets the Apple II+ era and matches that ROM's
64-character set.

**3. Why 7 bits wide, not 8?**

The Apple II display is 280 pixels wide in 40-column text mode.
280 divided by 40 columns equals exactly 7 pixels per column. This was a
deliberate Wozniak hardware choice to maximize horizontal resolution on a
standard NTSC television using the available oscillator frequency. The 8th
bit of each row byte is simply unused. There is no 8th pixel column.

**4. Why does `CharGenROM` not handle flashing?**

Flashing requires a timer that toggles the display state at approximately
1.875 Hz. The emulator toggles every 16 frames at 60 fps, which means the
inverse flag flips 60/16 = 3.75 times per second; since one full flash
cycle is two toggles (on then off), the visible flash rate is 3.75/2 =
1.875 Hz. That is a frame-counter concern, not a ROM-lookup concern.
`CharGenROM` is a pure function: given the same inputs, it always returns
the same outputs, with no internal state and no side effects. The flash
timer lives in `video.RenderText()`, which checks a frame counter and
overrides the `inverse` flag for screen bytes in the `$40`-`$7F` range.
This separation of concerns is covered in the next walkthrough.

**5. Why use `map[byte][8]uint8` in `init()` instead of a raw array literal?**

Readability. Writing `'A': {0x08, 0x14, 0x22, 0x22, 0x3E, 0x22, 0x22, 0x00}`
is self-documenting: any reader immediately sees which character the bitmap
describes. Writing the equivalent as a 1,024-byte flat literal with no keys
would require the reader to count bytes from the beginning to figure out
which character is at index 520. The map key is a Go byte literal that the
compiler converts to its ASCII numeric value. The map itself is a temporary
variable -- it is garbage-collected after `init()` returns. Only the flat
`charGen` array survives in memory.

**6. How does the real 2513 chip compare to this code?**

The Signetics 2513 is a 2,560-bit mask ROM (64 characters multiplied by
5 columns multiplied by 8 rows). It outputs 5-bit-wide pixel patterns per
access -- not 7. The Apple II video circuitry pads each 5-bit row with two
blank columns on the right to produce the 7-pixel output. Our code
simplifies this by storing 7-bit patterns directly in `charGen`, with the
padding pre-baked in. The visual result is identical. This is the same
kind of hardware-detail-hiding that `memory.ROM` performs: the physical
chip has specific timing and bus protocols that the emulator replaces with
a simpler Go abstraction that produces the same observable behavior.

---

## Section 7: Summary and What Is Next

`chargen.go` is 128 lines of Go that replace a physical silicon chip with
a package-level array and two functions. The `init()` function fills the
array from a readable map literal; `CharGenROM()` is the single public
entry point that translates a raw Apple II screen byte and a scanline
number into a row of pixel bits plus an inverse flag. Everything the video
renderer needs to draw a character is here.

> **Take-home idea**: A character generator is just a lookup table from
> (character code, scanline) to a row of pixel bits. The Apple II's unusual
> screen encoding -- inverse in `$00`-`$3F`, flashing in `$40`-`$7F`,
> normal in `$80`-`$FF` -- is decoded in a single switch statement before
> the lookup. The bit order (bit 0 = leftmost pixel) and the 7-pixel column
> width are hardware constraints that flow directly from the Apple II's
> display geometry.

**Quick reference**:

| Item           | Value                            |
|----------------|----------------------------------|
| File           | `video/chargen.go`               |
| Array size     | 1024 bytes (128 * 8)             |
| Char cell      | 7 wide x 8 tall pixels           |
| Defined glyphs | 64 (0x20 through 0x5F)           |
| Public API     | `CharGenROM(byte, int)`          |
| Returns        | `(pixels uint8, inverse bool)`   |
| Bit order      | bit 0 = leftmost (column 0)      |
| Real chip      | Signetics 2513 (5-bit output)    |

**What is next**: The next walkthrough covers `video/video.go`, which calls
`CharGenROM` for every character on every scanline and converts the pixel
bits into an RGBA frame buffer for SDL to display. It also introduces the
flash timer (the frame counter that makes `$40`-`$7F` screen bytes blink),
the `textLineAddr()` row-interleaving formula, and the phosphor green color
palette.

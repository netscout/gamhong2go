# Educational Walkthrough: `cpu/cpu.go` -- The MOS 6502 CPU Emulator

> This document walks through every section of `cpu/cpu.go` for someone who has never implemented a CPU before. Each section explains **what** the code is, **why** it exists, **how** it works line by line, and provides a **real-world analogy** to build intuition.

## Background: What Is the 6502?

The MOS 6502 is an 8-bit microprocessor from 1975. It powered the Apple II, the Nintendo Entertainment System (NES), the Commodore 64, and the Atari 2600. Despite having only ~3,500 transistors (a modern CPU has billions), it could run real programs -- games, word processors, even operating systems.

"8-bit" means the CPU works with data 8 bits (1 byte) at a time. A byte can hold values 0-255. For memory addresses, the 6502 uses 16 bits, meaning it can address up to 65,536 bytes (64 KB) of memory.

Our Go file implements a **software replica** of this chip. Every register, every timing quirk, even the hardware bugs are faithfully reproduced.

---

## Section 1: Status Register Flags (Lines 4-13)

### What It Is

```go
const (
    FlagC uint8 = 1 << iota // Carry
    FlagZ                    // Zero
    FlagI                    // Interrupt disable
    FlagD                    // Decimal mode
    FlagB                    // Break command
    FlagU                    // Unused (always 1)
    FlagV                    // Overflow
    FlagN                    // Negative
)
```

These constants define eight individual **flags** -- single yes/no indicators that live inside one 8-bit register (the "P" register, or "Processor Status" register). Each flag occupies exactly one bit:

| Constant | Bit Position | Binary Value | Decimal |
|----------|-------------|-------------|---------|
| FlagC    | 0           | 00000001    | 1       |
| FlagZ    | 1           | 00000010    | 2       |
| FlagI    | 2           | 00000100    | 4       |
| FlagD    | 3           | 00001000    | 8       |
| FlagB    | 4           | 00010000    | 16      |
| FlagU    | 5           | 00100000    | 32      |
| FlagV    | 6           | 01000000    | 64      |
| FlagN    | 7           | 10000000    | 128     |

### Why It Matters

The CPU needs to remember the results of its calculations so it can make decisions. For example:

- **Carry (C)**: Did the last addition overflow past 255? Did the last subtraction need to borrow? This is also used for multi-byte arithmetic (adding numbers larger than 255).
- **Zero (Z)**: Was the last result exactly zero? Used constantly in loops ("keep going until the counter hits zero").
- **Interrupt disable (I)**: Should the CPU ignore external interrupt signals? Set during startup and critical code sections.
- **Decimal (D)**: Should addition/subtraction use Binary-Coded Decimal (for representing decimal numbers)? The Apple II mostly ignores this.
- **Break (B)**: Was the current interrupt caused by a BRK software instruction (as opposed to a hardware interrupt)? Only meaningful when the status is pushed to the stack.
- **Unused (U)**: This bit is always 1. It has no function -- it is a quirk of the hardware.
- **Overflow (V)**: Did a signed arithmetic operation produce a wrong result? (e.g., adding two positive numbers and getting a negative).
- **Negative (N)**: Is bit 7 of the result set? In two's complement (how the 6502 represents signed numbers), bit 7 being set means the number is negative.

### How the Code Works

```go
FlagC uint8 = 1 << iota
```

Go's `iota` is a counter that starts at 0 and increments for each constant in a `const` block. The expression `1 << iota` means "shift the number 1 left by `iota` positions":

- `iota = 0`: `1 << 0` = `00000001` = 1 (FlagC)
- `iota = 1`: `1 << 1` = `00000010` = 2 (FlagZ)
- `iota = 2`: `1 << 2` = `00000100` = 4 (FlagI)
- ... and so on up to ...
- `iota = 7`: `1 << 7` = `10000000` = 128 (FlagN)

Each subsequent constant inherits the `1 << iota` expression (this is a Go feature), so we only need to write it once. Each flag value is a **bitmask** -- a number with exactly one bit set, used to read or write individual bits within the P register.

### Real-World Analogy

Think of a car dashboard with 8 warning lights. Each light is independent -- the "check engine" light can be on while the "low fuel" light is off. The P register is like viewing all 8 lights as a single snapshot. The flag constants are like labels telling you which position on the dashboard corresponds to which warning light.

---

## Section 2: Addressing Modes (Lines 16-32)

### What It Is

```go
type addrMode uint8

const (
    mImplied addrMode = iota
    mAccumulator
    mImmediate
    mZeroPage
    mZeroPageX
    mZeroPageY
    mAbsolute
    mAbsoluteX
    mAbsoluteY
    mIndirect
    mIndirectX // (Indirect,X) -- indexed indirect
    mIndirectY // (Indirect),Y -- indirect indexed
    mRelative
)
```

An **addressing mode** tells the CPU **how to find the data** an instruction operates on. The 6502 has 13 addressing modes. This is a lot -- modern CPUs have far fewer. The reason is that the 6502 had very few registers, so it compensated with flexible ways to access memory.

### Why It Matters

Consider the instruction "add a number to the accumulator." But which number? The answer depends on the addressing mode:

| Mode | Meaning | Example Instruction |
|------|---------|-------------------|
| **Implied** | No operand needed; the instruction knows what to do | `CLC` (clear carry flag) |
| **Accumulator** | Operate on the A register itself | `ASL A` (shift A left) |
| **Immediate** | The operand IS the data (a literal number) | `LDA #$42` (load the value 0x42) |
| **ZeroPage** | 1-byte address in the first 256 bytes of memory | `LDA $10` (load from address 0x0010) |
| **ZeroPageX** | ZeroPage + X register (wraps within page 0) | `LDA $10,X` (load from 0x0010+X) |
| **ZeroPageY** | ZeroPage + Y register (wraps within page 0) | `LDX $10,Y` (load from 0x0010+Y) |
| **Absolute** | Full 2-byte address anywhere in memory | `LDA $1234` (load from 0x1234) |
| **AbsoluteX** | Absolute + X register | `LDA $1234,X` (load from 0x1234+X) |
| **AbsoluteY** | Absolute + Y register | `LDA $1234,Y` (load from 0x1234+Y) |
| **Indirect** | Address points to another address (pointer) | `JMP ($1234)` (jump to address stored at 0x1234) |
| **IndirectX** | Zero-page pointer + X (indexed indirect) | `LDA ($10,X)` (complex; see resolve section) |
| **IndirectY** | Zero-page pointer, then + Y (indirect indexed) | `LDA ($10),Y` (complex; see resolve section) |
| **Relative** | Signed offset from current position (for branches) | `BEQ $10` (branch 16 bytes forward if zero flag set) |

**Why so many?** The 6502 only has three general-purpose registers (A, X, Y). Contrast this with modern CPUs that have 16 or more. To compensate, the 6502 gives you many ways to compute addresses, especially using the "zero page" (the first 256 bytes of RAM), which acts as an extension of the register file.

**Why is zero page special?** Instructions that use zero page addressing only need a 1-byte operand (addresses 0x00-0xFF) instead of a 2-byte operand (addresses 0x0000-0xFFFF). This makes them 1 byte shorter and 1 cycle faster. Programmers stored their most-used variables in zero page for speed.

### How the Code Works

The code uses a Go type alias (`type addrMode uint8`) and `iota` to create an enumeration. Each addressing mode gets a numeric ID (0 through 12). These IDs are used later in the `resolve()` function (Section 7) to determine how to calculate the effective address for each instruction.

### Real-World Analogy

Imagine you are told to "deliver a package." The addressing mode is the type of directions you receive:

- **Implied**: "Deliver it to yourself." (No address needed.)
- **Immediate**: "The package IS the directions." (The data is right there.)
- **ZeroPage**: "Go to apartment 42." (Short address, fast lookup, within the nearby building.)
- **Absolute**: "Go to 1234 Main Street." (Full address, anywhere in the city.)
- **AbsoluteX**: "Go to 1234 Main Street, then walk X doors down." (Full address + offset.)
- **Indirect**: "Go to 1234 Main Street. There you'll find a note with the real address." (Pointer.)
- **Relative**: "Walk 10 blocks forward from where you are now." (Used for branches.)

---

## Section 2.5: The 6502 Memory Map

### What It Is

The 6502 can address exactly 64 kilobytes of memory -- 65,536 individual byte locations numbered $0000 through $FFFF. This entire range is divided into 256 equal "pages," each holding exactly 256 bytes.

```
Address (Hex)    Address (Dec)     Page    What lives here
─────────────────────────────────────────────────────────────
$0000 - $00FF        0 -   255    Page 0   "Zero Page" - fast variable slots
$0100 - $01FF      256 -   511    Page 1   Stack
$0200 - $02FF      512 -   767    Page 2   ┐
$0300 - $03FF      768 -  1023    Page 3   │
  ...                                      ├─ General purpose RAM
$7E00 - $7EFF    32256 - 32511    Page 126 │
$7F00 - $7FFF    32512 - 32767    Page 127 ┘
─────────────────────────────────────────────────────────────
$8000 - $80FF    32768 - 33023    Page 128 ┐
  ...                                      ├─ ROM / Cartridge / I/O
$FE00 - $FEFF    65024 - 65279    Page 254 │
$FF00 - $FFFF    65280 - 65535    Page 255 ┘
─────────────────────────────────────────────────────────────
                                  256 pages × 256 bytes = 65,536 bytes = 64KB
```

### Why It Matters

Nearly every addressing mode in Section 2 references pages -- zero page instructions are faster, the stack is locked to page 1, and page crossings add a cycle penalty. Understanding the page structure makes all of those design decisions click into place.

### How It Works

A 16-bit address is two bytes side by side. The **high byte** is the page number; the **low byte** is the offset within that page:

```
$03A5  →  high byte $03 = page 3,  low byte $A5 = offset 165

$00FF = page 0, offset 255    (last byte of zero page)
$0100 = page 1, offset 0      (first byte of stack)
$FFFF = page 255, offset 255  (last byte of memory)

Pattern: $[page][offset]  →  $03A5 = page $03 (3), offset $A5 (165)
```

This is exactly why hex is used instead of decimal. In hex, the page/offset split maps perfectly onto the two digit pairs of the address. In decimal, $03A5 = 933 -- the page boundary is invisible. In hex, it jumps off the screen.

**Special pages:**

- **Page 0 (Zero Page, $0000-$00FF):** The 6502 has special opcodes that take a 1-byte address instead of 2 bytes. They only work within page 0. The payoff is shorter code and one fewer clock cycle per instruction. Programmers crammed their most-used variables into these 256 slots.
- **Page 1 (Stack, $0100-$01FF):** The stack is hardwired here. The Stack Pointer (SP) register is only 8 bits wide, so it can only describe an offset within a single page. The 6502 designers fixed that page to 1. The SP counts down from $FF, so the stack starts at $01FF and grows toward $0100.
- **Upper half ($8000-$FFFF, pages 128-255):** What lives here varies by system. On an Apple II, this region holds the BIOS ROM ("Monitor"), memory-mapped I/O slots for peripherals, and the language cards. The last two pages ($FE00-$FFFF) always contain the interrupt vectors that tell the CPU where to jump on reset, IRQ, and NMI.

### Real-World Analogy

Think of the 6502's memory as a hotel with 256 floors (pages) and 256 rooms per floor (offsets). The room number on your key card is always written as two pairs of digits: floor then room. Floor 0 is the VIP floor -- special elevators go there faster and the key cards are shorter. Floor 1 is where luggage storage (the stack) lives. The top floors are hotel services (ROM, I/O) rather than guest rooms.

---

## Section 3: Memory Interface (Lines 35-39)

### What It Is

```go
type Memory interface {
    Read(addr uint16) uint8
    Write(addr uint16, val uint8)
}
```

This is a Go **interface** that defines how the CPU communicates with the outside world. It declares exactly two operations: read a byte from an address, and write a byte to an address.

### Why It Matters

The real 6502 chip has 16 address pins and 8 data pins. When it wants to read memory location 0x1234, it puts 0x1234 on the address pins and reads whatever appears on the data pins. It has **no idea** what is actually connected to those pins -- it could be RAM, ROM, a keyboard controller, a video chip, or a sound chip.

This is called **memory-mapped I/O**: everything the CPU talks to looks like memory. The CPU doesn't have special "talk to keyboard" instructions. Instead, reading from address 0xC000 on an Apple II happens to give you the last key pressed, because the keyboard controller is wired to respond at that address.

By using a Go interface, our emulator mirrors this hardware design:
- In **tests**, Memory can be a simple 64 KB byte array.
- In the **full emulator**, Memory becomes an address bus that routes reads/writes to the correct device (RAM, ROM, keyboard, display, etc.).

The CPU code never changes -- only the Memory implementation does.

### How the Code Works

- `Read(addr uint16) uint8`: Takes a 16-bit address (0x0000-0xFFFF), returns an 8-bit value. This matches the 6502's 16-bit address bus and 8-bit data bus.
- `Write(addr uint16, val uint8)`: Takes a 16-bit address and an 8-bit value. Stores the value at that address.

The `uint16` type ensures addresses are exactly 16 bits (0-65535). The `uint8` type ensures data values are exactly 8 bits (0-255). Go's type system enforces the same constraints as the hardware.

### Real-World Analogy

Think of a wall with a mail slot. You can push a letter in (Write) or pull one out (Read). The CPU doesn't know or care what's behind the wall -- it could be a person, a filing cabinet, or a pneumatic tube to another building. The interface is the contract: "I give you an address, you give me data."

This is the **Dependency Inversion Principle** (the "D" in SOLID) in action: the high-level module (CPU) does not depend on low-level modules (RAM, ROM, I/O chips). Both depend on the abstraction (the Memory interface).

---

## Section 4: CPU Struct (Lines 42-55)

### What It Is

```go
type CPU struct {
    A  uint8  // Accumulator
    X  uint8  // X index register
    Y  uint8  // Y index register
    SP uint8  // Stack pointer (offset within page $01)
    PC uint16 // Program counter
    P  uint8  // Processor status flags

    Mem    Memory // Attached memory / bus
    Cycles uint64 // Total elapsed cycles

    pageCrossed bool // Set by resolve() when indexing crosses a page
    extraCycles int  // Extra cycles added by branches
}
```

This struct holds the **complete state** of the 6502 processor. If you saved every field in this struct, you could pause the CPU, shut down your program, restart it later, restore the fields, and the CPU would continue exactly where it left off.

### Why It Matters

A real CPU is a physical chip with wires, transistors, and tiny flip-flop circuits that store bits. Each register is a set of flip-flops. Our struct replaces those physical circuits with Go variables.

Here is what each register does:

**A -- Accumulator (8-bit)**
The main "working" register. Most arithmetic and logic operations happen here. If you want to add two numbers, you load one into A, then add the other to it. The result stays in A. The name "accumulator" means it "accumulates" results.

**X -- X Index Register (8-bit)**
A helper register, primarily used as a counter or an offset for addressing modes (ZeroPageX, AbsoluteX, IndirectX). Think of it as a "column index" when walking through tables of data.

**Y -- Y Index Register (8-bit)**
Similar to X but used with different addressing modes (ZeroPageY, AbsoluteY, IndirectY). Think of it as a "row index." Having two index registers lets you traverse 2D structures.

**SP -- Stack Pointer (8-bit)**
Points to the current top of the stack. Critically, it is only 8 bits, not 16. The stack is **hardwired** to memory page $01 (addresses $0100-$01FF). The SP value is the low byte -- the high byte is always $01. So if SP is $FD, the stack pointer points to address $01FD.

The stack grows **downward**: pushing data decrements SP; pulling data increments SP. This is a hardware design choice common across many architectures.

**PC -- Program Counter (16-bit)**
The **only** 16-bit register. It holds the address of the next instruction to execute. After the CPU fetches an instruction byte, PC advances to point to the next byte. When the CPU executes a jump or branch, PC changes to the target address.

**P -- Processor Status (8-bit)**
The flags register from Section 1. All eight flags packed into one byte.

**Mem -- Memory**
The interface from Section 3. This is how the CPU reads and writes the outside world.

**Cycles -- Total Elapsed Cycles (64-bit)**
A running total of how many clock cycles have passed. The real 6502 runs at ~1 MHz (1 million cycles per second). Different instructions take different numbers of cycles (2-7). This counter is essential for synchronizing the CPU with other chips (video, sound).

**pageCrossed / extraCycles -- Internal State**
These are not part of the real 6502's register set. They are bookkeeping variables our emulator uses to calculate cycle-accurate timing. More on these in the Step and Branch sections.

### Real-World Analogy

Imagine a very simple office worker at a desk:
- **A (Accumulator)**: The piece of paper on the desk where you do your math.
- **X and Y**: Two fingers you use to keep your place in a spreadsheet -- one for the column, one for the row.
- **SP (Stack Pointer)**: A spring-loaded paper tray. You can put papers on top (push) or take the top paper off (pull). The pointer tells you how high the stack is.
- **PC (Program Counter)**: Your finger running along a printed list of instructions. It points to "what to do next."
- **P (Status Flags)**: Post-it notes stuck to your monitor: "last result was zero," "last result was negative," etc.

---

## Section 5: New() and Reset() (Lines 58-73)

### What It Is

```go
func New(mem Memory) *CPU {
    c := &CPU{
        SP:  0xFD,
        P:   FlagU | FlagI,
        Mem: mem,
    }
    return c
}

func (c *CPU) Reset() {
    c.PC = c.read16(0xFFFC)
    c.SP = 0xFD
    c.P = FlagU | FlagI
}
```

`New()` creates a CPU in its post-reset state. `Reset()` simulates pressing the reset button -- it reinitializes the CPU without creating a new struct.

### Why It Matters

When a real 6502 chip powers on (or the reset line is pulled low), it performs a specific hardware sequence:

1. It sets the stack pointer to $FD.
2. It sets the status register to `FlagU | FlagI` (unused flag always on, interrupts disabled).
3. It reads the **reset vector** -- a 16-bit address stored at memory locations $FFFC and $FFFD.
4. It sets PC to whatever address it read from the reset vector.

**Why $FD and not $FF?** During the reset sequence, the hardware goes through the motions of pushing PC and the status register onto the stack (3 bytes), but the writes are suppressed (they don't actually store anything). The stack pointer still decrements 3 times: $FF -> $FE -> $FD -> $FC... wait, but it's $FD. That is because the decrement happens after the (suppressed) write, so: write at $FF, SP becomes $FE; write at $FE, SP becomes $FD; write at $FD, SP becomes $FC -- actually, the final SP value is $FD because only the push of the high byte and low byte of PC happens, plus the status push, but the exact hardware behavior leaves SP at $FD. This is a hardware-verified value.

**Why `FlagU | FlagI`?** The unused flag (bit 5) is always set -- there is no way to clear it. Interrupts are disabled on startup so the CPU can set up its interrupt handler before any interrupts fire.

**What is the reset vector?** The ROM (read-only memory) that comes with the computer stores a special address at memory location $FFFC-$FFFD. This address tells the CPU "start executing code here." On an Apple II, this points to the Monitor ROM. On an NES, it points to the game's startup routine. The vector is the bridge between the hardware (CPU) and the software (program).

### How the Code Works

**New():**
- Creates a `CPU` struct on the heap (`&CPU{...}`).
- Sets SP to 0xFD (post-reset stack pointer).
- Sets P to `FlagU | FlagI` = `00100000 | 00000100` = `00100100` = 0x24.
- Stores the Memory reference.
- A, X, Y default to 0 (Go zero values). PC defaults to 0 and is expected to be set via `Reset()` or directly.

**Reset():**
- `c.read16(0xFFFC)` reads two bytes from $FFFC and $FFFD, combines them into a 16-bit address (little-endian), and sets PC to that address.
- Resets SP and P to their startup values.
- Does NOT clear A, X, or Y -- on real hardware, these registers contain garbage after reset.

### Real-World Analogy

Think of a game console. When you power it on:
1. The console hardware initializes itself to a known state (SP, P).
2. It looks at a specific spot on the game cartridge (the reset vector at $FFFC) to find out where the game's code starts.
3. It begins executing from that location.

Pressing the reset button does the same thing -- it doesn't erase the game's memory, it just tells the CPU "start over from the beginning."

---

## Section 6: Step() -- The Fetch-Decode-Execute Cycle (Lines 76-92)

### What It Is

```go
func (c *CPU) Step() int {
    opcode := c.read(c.PC)
    c.PC++
    c.pageCrossed = false
    c.extraCycles = 0

    inst := &opcodeTable[opcode]
    inst.exec(c, inst.mode)

    cycles := int(inst.cycles)
    if c.pageCrossed && inst.pagePenalty {
        cycles++
    }
    cycles += c.extraCycles
    c.Cycles += uint64(cycles)
    return cycles
}
```

This is the **heartbeat** of the CPU. Every single instruction the 6502 ever executes goes through this function. It implements the **fetch-decode-execute cycle**, the fundamental loop that ALL CPUs perform:

1. **Fetch**: Read the next instruction byte from memory.
2. **Decode**: Look up what that byte means (which instruction, which addressing mode).
3. **Execute**: Perform the instruction.

### Why It Matters

The 6502 has 256 possible opcodes (since an opcode is one byte, 0x00-0xFF). Not all 256 are used -- some are "illegal" opcodes. Each valid opcode maps to a specific instruction AND addressing mode combination. For example:
- Opcode $A9 = LDA Immediate (load accumulator with a literal value)
- Opcode $AD = LDA Absolute (load accumulator from a full address)
- Opcode $A5 = LDA ZeroPage (load accumulator from zero page)

These are all "LDA" (Load Accumulator) but with different addressing modes, so they are different opcodes.

### How the Code Works

**Line by line:**

```go
opcode := c.read(c.PC)
```
**FETCH**: Read one byte from the address PC points to. This byte is the opcode.

```go
c.PC++
```
Advance PC to point past the opcode. Now PC points to the operand (if any).

```go
c.pageCrossed = false
c.extraCycles = 0
```
Reset the cycle-adjustment flags. These will be set by `resolve()` or `branch()` if needed.

```go
inst := &opcodeTable[opcode]
```
**DECODE**: Look up the opcode in a table (defined elsewhere in the codebase). The table entry contains:
- `exec`: a function pointer to the Go function that implements this instruction
- `mode`: which addressing mode this opcode uses
- `cycles`: base number of cycles this instruction takes
- `pagePenalty`: whether this instruction takes an extra cycle on page crossing

```go
inst.exec(c, inst.mode)
```
**EXECUTE**: Call the instruction's implementation function, passing the CPU and the addressing mode. The instruction function will call `resolve()` to get the operand address, then do its work (load, store, add, compare, etc.).

```go
cycles := int(inst.cycles)
if c.pageCrossed && inst.pagePenalty {
    cycles++
}
cycles += c.extraCycles
```
**Cycle calculation**: Start with the base cycle count. Add 1 if a page boundary was crossed AND this instruction has a page penalty. Add any extra cycles from branch instructions.

```go
c.Cycles += uint64(cycles)
return cycles
```
Accumulate total cycles and return how many this instruction consumed.

### Real-World Analogy

Imagine a cook in a kitchen with a recipe book:
1. **Fetch**: Read the next instruction from the recipe book ("chop the onion").
2. **Decode**: Understand what "chop" means and what tools/ingredients are needed.
3. **Execute**: Actually chop the onion.
4. Move your finger to the next line of the recipe.
5. Repeat forever.

The cycle count is like timing -- chopping takes 3 seconds, boiling takes 7 seconds. Some operations take longer if you have to walk to a different counter (page crossing penalty).

---

## Section 7: resolve() -- Address Resolution (Lines 98-176)

### What It Is

```go
func (c *CPU) resolve(mode addrMode) uint16 {
    switch mode {
    // ... 13 cases
    }
}
```

This function takes an addressing mode and returns the **effective address** -- the actual memory location the instruction should operate on. It reads any operand bytes that follow the opcode and advances PC past them.

### Why It Matters

The opcode tells the CPU **what** to do (load, store, add, etc.). The addressing mode tells it **where** the data is. `resolve()` is the "where" calculator. It is called by instruction implementations when they need to know which memory address to read from or write to.

### How the Code Works -- Each Mode

#### Immediate (line 103-106)
```go
case mImmediate:
    addr := c.PC
    c.PC++
    return addr
```
The operand IS the data. PC currently points at the data byte (right after the opcode), so we return PC as the address and advance past it. When the instruction reads from this address, it gets the literal value from the program.

#### ZeroPage (line 108-111)
```go
case mZeroPage:
    addr := uint16(c.read(c.PC))
    c.PC++
    return addr
```
Read one byte from the program. This byte is an address in the range 0x00-0xFF (the "zero page"). Converting a `uint8` to `uint16` just adds a high byte of 0x00. This is faster and smaller than a full 16-bit address.

#### ZeroPageX (line 113-116)
```go
case mZeroPageX:
    addr := uint16(c.read(c.PC) + c.X) // wraps within zero page
    c.PC++
    return addr
```
Read a zero-page base address, add X to it. The addition happens in `uint8` arithmetic, meaning it **wraps around within zero page**. If the base is $F0 and X is $20, the result is $10 (not $110). This wrapping is intentional hardware behavior -- zero page indexing never leaves zero page.

#### ZeroPageY (line 118-121)
```go
case mZeroPageY:
    addr := uint16(c.read(c.PC) + c.Y)
    c.PC++
    return addr
```
Same as ZeroPageX but using the Y register. Only used by a few instructions (LDX, STX).

#### Understanding Zero Page Indexed Modes

ZeroPageX and ZeroPageY work like array indexing -- `base[index]` -- but constrained entirely to zero page. That constraint is the point.

**What zero page indexed modes are good at:**

1. **Small lookup tables.** If you store a 16-entry color table at $20-$2F (zero page), you can load any entry with `LDA $20,X` -- set X to the index and you are done. One instruction, fast, no page-crossing penalty.

2. **Struct-like variable groups.** Suppose you have a "player" object: health at $50, score-low at $51, score-high at $52, lives at $53. Set X to 0 for health, 1 for score-low, and so on. `LDA $50,X` picks the field. This is exactly how C structs work, but done by hand in 1970s assembly.

3. **Pointer selection tables (using IndirectX mode).** Zero page can hold several 16-bit pointers (two bytes each, stored in little-endian order). With X selecting the right pair, `LDA ($00,X)` uses the IndirectX addressing mode — note the parentheses, which mean "indirect." The resolve step reads 2 bytes from zero page to build a 16-bit pointer, then LDA loads 1 byte from that pointer's target address. This gives you an array of pointers in ultra-fast memory.

**What zero page indexed modes are NOT for:**

Large data structures -- screen buffers, text strings, sound samples -- live in regular RAM at $0200 and above. You access those with `AbsoluteX` / `AbsoluteY` (full 16-bit base + 8-bit index) or with `IndirectIndexed` (16-bit pointer stored in zero page, Y added at access time). Zero page simply isn't large enough for bulk data.

**Mental model:**

Think of zero page as a bank of 256 very fast "register-like" slots. Most of the time each slot holds one named variable. ZeroPageX/Y lets you say "go to slot base + X" -- turning a group of adjacent slots into a tiny array or struct. The moment your data grows beyond 256 bytes, you outgrow zero page and switch to one of the absolute or indirect modes.

#### Absolute (line 123-126)
```go
case mAbsolute:
    addr := c.read16(c.PC)
    c.PC += 2
    return addr
```
Read a full 16-bit address from the program (2 bytes, little-endian). This can address any location in the 64 KB space. PC advances by 2 because we consumed 2 operand bytes.

**Absolute vs Indirect -- what's the difference?** Both produce a 16-bit address, but Absolute embeds the address directly in the instruction, while Indirect reads it from memory at runtime. Compare:

```asm
; ABSOLUTE - fixed destination, always jumps to GameLoop
GameLoop:
    JSR ReadInput
    JSR UpdateGame
    JSR DrawScreen
    JMP GameLoop           ; always goes to the same place

; INDIRECT - destination changes at runtime (function pointer)
; $30/$31 in zero page holds the current state handler address
MainLoop:
    JMP ($0030)            ; jump to whatever $30/$31 points to

TitleScreen:
    ; ... handle title screen ...
    ; Player presses start → update pointer to Gameplay:
    LDA #<Gameplay
    STA $30
    LDA #>Gameplay
    STA $31
    JMP MainLoop           ; now JMP ($0030) goes to Gameplay

Gameplay:
    ; ... handle gameplay ...
```

The flow: `MainLoop → TitleScreen` (because `$30/$31` = TitleScreen address) → TitleScreen updates `$30/$31` to point to Gameplay → `JMP MainLoop` → `MainLoop → Gameplay` (same `JMP ($0030)` instruction, different destination). This is the 6502 equivalent of a function pointer or a state machine.

#### AbsoluteX (line 128-133)
```go
case mAbsoluteX:
    base := c.read16(c.PC)
    c.PC += 2
    addr := base + uint16(c.X)
    c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
    return addr
```
Read a full address, then add X. The key detail is the **page-crossing check**: `(base & 0xFF00) != (addr & 0xFF00)`. A "page" is a 256-byte aligned block (0x0000-0x00FF is page 0, 0x0100-0x01FF is page 1, etc.). The expression `addr & 0xFF00` extracts the page number. If adding X pushes us into a different page, `pageCrossed` is set to `true`, which will add 1 extra cycle in `Step()`.

**Why does page crossing cost an extra cycle?** The 6502 has a performance optimization: when adding an 8-bit index to a 16-bit base address, it starts reading from the address computed using only the low byte addition. If there is no carry into the high byte (same page), the read is correct and the instruction is done. If there IS a carry (page crossed), the CPU has to fix the high byte and re-read, costing one extra cycle.

#### AbsoluteY (line 135-140)
```go
case mAbsoluteY:
    base := c.read16(c.PC)
    c.PC += 2
    addr := base + uint16(c.Y)
    c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
    return addr
```
Identical to AbsoluteX but using Y.

#### Indirect (line 142-145)
```go
case mIndirect:
    ptr := c.read16(c.PC)
    c.PC += 2
    return c.read16Wrap(ptr) // reproduces the NMOS page-boundary bug
```
Read a 16-bit pointer address from the program, then read the actual target address FROM that pointer location. This is a double dereference -- like following a forwarding address.

The call to `read16Wrap` (not `read16`) is critical -- it reproduces the famous NMOS JMP bug. See Section 8.

Only the JMP instruction uses this mode.

#### IndirectX -- Indexed Indirect (line 147-153)
```go
case mIndirectX:
    base := c.read(c.PC)
    c.PC++
    ptr := uint16(base + c.X) // wraps within zero page
    lo := uint16(c.read(ptr))
    hi := uint16(c.read((ptr + 1) & 0x00FF))
    return hi<<8 | lo
```
This is the most complex mode. Step by step:
1. Read a zero-page base address from the program.
2. Add X to it (wrapping within zero page, since both are `uint8`).
3. Read a 16-bit pointer from that zero-page location. The `& 0x00FF` on `ptr + 1` ensures the high byte read also wraps within zero page.
4. Return the 16-bit address stored at that pointer.

**Use case**: You have an array of pointers in zero page. X selects which pointer. The pointer tells you where the actual data is.

#### IndirectY -- Indirect Indexed (line 155-163)
```go
case mIndirectY:
    ptr := uint16(c.read(c.PC))
    c.PC++
    lo := uint16(c.read(ptr))
    hi := uint16(c.read((ptr + 1) & 0x00FF))
    base := hi<<8 | lo
    addr := base + uint16(c.Y)
    c.pageCrossed = (base & 0xFF00) != (addr & 0xFF00)
    return addr
```
The mirror of IndirectX, but the order is reversed:
1. Read a zero-page address from the program.
2. Read a 16-bit base address from that zero-page location.
3. Add Y to the base address.
4. Check for page crossing.

**Use case**: You have a pointer in zero page. Y is the offset into the data that pointer points to. This is by far the most common indirect mode -- it is how the 6502 accesses arrays and data structures.

**Key difference from IndirectX**: IndirectX adds the index BEFORE the pointer lookup (selecting which pointer). IndirectY adds the index AFTER (offsetting into the pointed-to data).

#### Relative (line 165-172)
```go
case mRelative:
    offset := uint16(c.read(c.PC))
    c.PC++
    if offset&0x80 != 0 {
        offset |= 0xFF00 // sign-extend
    }
    return c.PC + offset
```
Used only by branch instructions. Read a 1-byte signed offset (-128 to +127) and add it to the current PC.

The **sign extension** on line 168-170 deserves explanation: The offset byte is signed (e.g., 0xFE means -2), but we need to add it to a 16-bit PC. If bit 7 is set (negative number), we set all the upper bits to 1 (`offset |= 0xFF00`). This is how you convert a negative 8-bit number to a negative 16-bit number in two's complement.

Example: offset 0xFE (binary: 11111110, meaning -2).
After sign extension: 0xFFFE (binary: 1111111111111110, still -2 in 16-bit).
If PC is 0x1000: target = 0x1000 + 0xFFFE = 0x0FFE (two bytes back, due to unsigned overflow wrapping).

### Real-World Analogy

Think of resolve() as a GPS that accepts different types of directions:

- **Immediate**: "The treasure is IN this envelope" (the data is the directions themselves).
- **ZeroPage**: "Go to locker 42" (a nearby, quick-to-reach location).
- **Absolute**: "Go to 1234 Main Street, Anytown" (full address, anywhere).
- **AbsoluteX**: "Go to 1234 Main Street, then walk X doors east" (full address + offset).
- **Indirect**: "Go to 1234 Main Street. Inside you'll find a note with the real address" (pointer).
- **IndirectY**: "Go to locker 42. Inside you'll find a note with an address. Go there, then walk Y steps further" (pointer + offset).
- **Relative**: "From where you are, walk 10 steps north" (relative to current position).

---

## Section 8: Memory Helpers (Lines 180-204)

### What It Is

```go
func (c *CPU) read(addr uint16) uint8 {
    return c.Mem.Read(addr)
}

func (c *CPU) write(addr uint16, val uint8) {
    c.Mem.Write(addr, val)
}

func (c *CPU) read16(addr uint16) uint16 {
    lo := uint16(c.read(addr))
    hi := uint16(c.read(addr + 1))
    return hi<<8 | lo
}

func (c *CPU) read16Wrap(addr uint16) uint16 {
    lo := uint16(c.read(addr))
    hiAddr := (addr & 0xFF00) | uint16(uint8(addr)+1)
    hi := uint16(c.read(hiAddr))
    return hi<<8 | lo
}
```

Four helper functions for reading from and writing to memory.

### Why It Matters

**read() and write()** are thin wrappers around the Memory interface. They exist to keep the CPU code concise -- writing `c.read(addr)` is cleaner than `c.Mem.Read(addr)` throughout the codebase.

**read16()** reads a 16-bit (2-byte) value from memory. The 6502 is **little-endian**, meaning the low byte comes first, followed by the high byte. So if memory at address $1000 contains $34 and $1001 contains $12, then `read16(0x1000)` returns $1234.

The expression `hi<<8 | lo` combines two bytes: shift the high byte left by 8 bits (moving it to the upper byte), then OR in the low byte.

**read16Wrap()** exists solely to reproduce a famous hardware bug in the NMOS 6502.

### The Famous NMOS JMP Bug

This is one of the most well-known bugs in computing history. Here is what happens:

The `JMP` instruction in indirect mode (`JMP ($xxFF)`) is supposed to read a 16-bit pointer from memory. If the pointer is stored at $02FF, it should read:
- Low byte from $02FF
- High byte from $0300

But the NMOS 6502 has a bug: instead of reading the high byte from $0300, it reads from $0200. The address wraps within the same page instead of crossing to the next page.

**How `read16Wrap` implements this:**
```go
hiAddr := (addr & 0xFF00) | uint16(uint8(addr)+1)
```

Breaking this down:
- `addr & 0xFF00`: Extracts the page number (e.g., $02FF becomes $0200).
- `uint8(addr) + 1`: Takes the low byte of addr, adds 1, and wraps within 8 bits ($FF + 1 = $00 in uint8).
- `|`: Combines them. So $02FF becomes $0200 | $00 = $0200. The high byte is fetched from the same page.

Compare with `read16`:
- `addr + 1`: $02FF + 1 = $0300. The high byte is correctly fetched from the next page.

**Why must we reproduce this bug?** Because real 6502 programs were written knowing about this bug. Programmers avoided placing JMP indirect operands at page boundaries. If our emulator "fixed" the bug, those programs might break because they were designed around the bug's existence. Accurate emulation means reproducing the hardware exactly, warts and all.

### Real-World Analogy

Imagine a two-page letter where page 1 ends mid-sentence. Normally, you'd flip to page 2 to continue reading. But this particular photocopier has a bug: if the sentence ends at the very bottom of page 1, instead of looking at page 2, it loops back to the top of page 1 for the next word. `read16Wrap` replicates this exact paper-jam behavior.

---

## Section 9: Stack Helpers (Lines 208-229)

### What It Is

```go
func (c *CPU) push(val uint8) {
    c.write(0x0100|uint16(c.SP), val)
    c.SP--
}

func (c *CPU) pull() uint8 {
    c.SP++
    return c.read(0x0100 | uint16(c.SP))
}

func (c *CPU) push16(val uint16) {
    c.push(uint8(val >> 8))
    c.push(uint8(val))
}

func (c *CPU) pull16() uint16 {
    lo := uint16(c.pull())
    hi := uint16(c.pull())
    return hi<<8 | lo
}
```

These functions manage the 6502's hardware stack.

### Why It Matters

The **stack** is a Last-In-First-Out (LIFO) data structure used for:
- Saving the return address when calling a subroutine (JSR/RTS)
- Saving the CPU state during an interrupt (IRQ/NMI)
- Temporarily saving register values (PHA/PLA, PHP/PLP)

On the 6502, the stack is **hardwired** to memory page $01 (addresses $0100-$01FF). This is not a software convention -- the hardware forces it. The stack pointer (SP) is only 8 bits, representing an offset within this page.

The stack grows **downward**: when you push data, SP decreases. When you pull data, SP increases. A fresh stack starts at SP=$FD (pointing to $01FD), and data is pushed to $01FD, $01FC, $01FB, and so on.

**Why only 256 bytes?** The 6502 designers had to make trade-offs. A larger stack would require a wider stack pointer register (more transistors, more expensive). 256 bytes was enough for most programs. It does mean that deeply nested subroutine calls or heavy stack usage can overflow the stack -- there is no hardware protection against this.

### How the Code Works

**push(val uint8):**
```go
c.write(0x0100|uint16(c.SP), val)
c.SP--
```
1. Write the value to address `$0100 + SP`. The `|` (OR) operation is equivalent to addition here because the high byte ($01) and SP (the low byte) never overlap.
2. Decrement SP. The stack grows downward.

Note: The 6502 writes THEN decrements. SP always points to the next free location after a push... actually, SP points to the last occupied location. Wait -- let me be precise: SP points to the next free slot. After push, the data is at `$0100+SP_before` and SP has moved down. On a pull, SP moves up first, then reads. This means SP always points to the next available (empty) position.

**pull() uint8:**
```go
c.SP++
return c.read(0x0100 | uint16(c.SP))
```
1. Increment SP first (move up to the most recently pushed value).
2. Read the value from that address.

The order is reversed from push: push writes then decrements; pull increments then reads.

**push16(val uint16):**
```go
c.push(uint8(val >> 8)) // push high byte first
c.push(uint8(val))       // push low byte second
```
Pushes a 16-bit value as two bytes. High byte is pushed first (ending up at the higher address), low byte second (ending up at the lower address). This means the 16-bit value is stored in memory in little-endian order, consistent with the 6502's byte ordering.

**pull16() uint16:**
```go
lo := uint16(c.pull()) // pull low byte first
hi := uint16(c.pull()) // pull high byte second
return hi<<8 | lo
```
Reverses push16: pulls the low byte first, then the high byte, and combines them.

### Real-World Analogy

Think of a spring-loaded plate dispenser in a cafeteria:
- **Push**: Place a plate on top. The spring compresses and the stack goes down.
- **Pull**: Take the top plate. The spring extends and the stack comes up.
- The dispenser has a fixed location (page $01) and a fixed capacity (256 plates).
- You can only access the top plate -- you cannot reach into the middle of the stack.

---

## Section 10: Flag Helpers (Lines 233-251)

### What It Is

```go
func (c *CPU) setFlag(flag uint8, on bool) {
    if on {
        c.P |= flag
    } else {
        c.P &^= flag
    }
}

func (c *CPU) getFlag(flag uint8) bool {
    return c.P&flag != 0
}

func (c *CPU) setZN(val uint8) {
    c.setFlag(FlagZ, val == 0)
    c.setFlag(FlagN, val&0x80 != 0)
}
```

Three helper functions for reading and writing individual flag bits in the P register.

### Why It Matters

The P register packs 8 independent boolean values into a single byte. We need a clean way to set, clear, and test individual bits without disturbing the others. These helpers hide the bit-manipulation details so instruction implementations can say `c.setFlag(FlagC, true)` instead of `c.P |= FlagC`.

`setZN` is especially important -- it is called after almost every instruction that produces a result, because the Zero and Negative flags must reflect the most recent value.

### How the Code Works

**setFlag:**

```go
c.P |= flag   // Set a bit: OR with the bitmask
```
The OR operation sets the target bit to 1 without changing any other bits. Example:
- P = `00100100` (bits 2 and 5 are set)
- flag = `00000001` (FlagC, bit 0)
- Result: `00100101` (bit 0 is now set, everything else unchanged)

```go
c.P &^= flag   // Clear a bit: AND-NOT with the bitmask
```
Go's `&^` operator is "AND-NOT" (also called "bit clear"). It clears the target bit to 0 without changing others. Example:
- P = `00100101`
- flag = `00000001` (FlagC)
- `&^` means: keep all bits EXCEPT those set in flag
- Result: `00100100` (bit 0 cleared)

**getFlag:**
```go
return c.P&flag != 0
```
AND the P register with the bitmask. If the bit is set, the result is non-zero (true). If the bit is clear, the result is zero (false).

**setZN:**
```go
c.setFlag(FlagZ, val == 0)
c.setFlag(FlagN, val&0x80 != 0)
```
- Set the Zero flag if `val` is exactly 0.
- Set the Negative flag if bit 7 of `val` is set (`0x80` = `10000000`). In two's complement representation, bit 7 indicates a negative number.

### Real-World Analogy

Imagine a panel of 8 light switches. Each switch controls one light independently:
- **setFlag(flag, true)**: Flip one specific switch ON, leaving all others as they are.
- **setFlag(flag, false)**: Flip one specific switch OFF, leaving all others as they are.
- **getFlag(flag)**: Look at one specific switch to see if it is ON or OFF.
- **setZN**: After every task, automatically update two specific switches -- "was the result zero?" and "was the result negative?"

---

## Section 11: Branch Helper (Lines 255-268)

### What It Is

```go
func (c *CPU) branch(mode addrMode, condition bool) {
    target := c.resolve(mode)
    if condition {
        c.extraCycles++
        if (c.PC & 0xFF00) != (target & 0xFF00) {
            c.extraCycles++
        }
        c.PC = target
    }
}
```

This function implements conditional branching -- the 6502's way of making decisions ("if this, go there").

### Why It Matters

Without branches, a CPU can only execute instructions in a straight line. Branches are what make programs able to:
- Loop: "go back to the beginning if the counter isn't zero yet"
- Make decisions: "skip this code if the comparison failed"
- Implement if/else, while, for, switch -- every control structure

The 6502 has 8 branch instructions, each testing one flag:
- BCC/BCS: Branch if Carry Clear/Set
- BEQ/BNE: Branch if Equal (Zero set) / Not Equal (Zero clear)
- BMI/BPL: Branch if Minus (Negative set) / Plus (Negative clear)
- BVC/BVS: Branch if Overflow Clear/Set

All 8 work identically except for which flag and which value they test. The `branch()` helper unifies them all -- each instruction just calls `c.branch(mode, c.getFlag(FlagX))` or `c.branch(mode, !c.getFlag(FlagX))`.

### How the Code Works

```go
target := c.resolve(mode)
```
Calculate the branch target address using relative addressing. This always happens, even if the branch is not taken -- the operand byte must be consumed (PC must advance past it).

```go
if condition {
```
Check whether the branch condition is met.

```go
    c.extraCycles++
```
A taken branch costs **1 extra cycle**. The 6502 speculatively skips the branch (assumes not taken). If the branch IS taken, it needs an extra cycle to load the new PC.

```go
    if (c.PC & 0xFF00) != (target & 0xFF00) {
        c.extraCycles++
    }
```
If the branch target is on a **different page** than the current PC, add ANOTHER extra cycle. Crossing a page boundary means the high byte of PC changes, which requires additional internal processing.

So the total cost of a branch instruction is:
- **Not taken**: base cycles only (typically 2)
- **Taken, same page**: base + 1 = 3
- **Taken, cross page**: base + 2 = 4

```go
    c.PC = target
```
Set PC to the target address. The next `Step()` call will fetch the instruction from this new location.

### Real-World Analogy

Imagine you are reading a "choose your own adventure" book:
- You reach a decision point: "If you have the magic key, turn to page 42. Otherwise, continue to the next paragraph."
- **Not taken** (you don't have the key): You keep reading the next line. Fast -- no page flipping.
- **Taken, same chapter** (you have the key, page 42 is nearby): You flip to page 42. Slightly slower -- you had to find the page.
- **Taken, different chapter** (page 42 is in a different section of the book): You flip to a whole different part of the book. Slowest -- you had to find the chapter AND the page.

---

## Summary: How It All Fits Together

When the emulator runs, the flow is:

1. `New(mem)` creates the CPU and wires it to memory.
2. `Reset()` reads the reset vector and sets PC to the program's entry point.
3. `Step()` is called in a loop, once per instruction:
   a. **Fetch** the opcode byte from `mem[PC]`.
   b. **Decode** using the opcode table to find the instruction function and addressing mode.
   c. The instruction calls **`resolve()`** to compute the effective address.
   d. `resolve()` uses the **memory helpers** (`read`, `read16`, etc.) to fetch operand bytes.
   e. The instruction uses **memory helpers** to read/write data, **stack helpers** for push/pull, **flag helpers** to update status flags, and the **branch helper** for conditional jumps.
   f. `Step()` calculates the total cycles (base + page crossing penalty + branch penalty).
4. The caller uses the returned cycle count to synchronize with other emulated hardware.

The elegance of this design is its simplicity: 269 lines of Go code faithfully replicate a physical chip that contains 3,510 transistors. Each section has a single, clear responsibility, and they compose together to create a complete, cycle-accurate CPU emulator.

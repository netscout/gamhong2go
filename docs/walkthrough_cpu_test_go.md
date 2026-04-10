# Educational Walkthrough: `cpu/cpu_test.go` -- Testing the 6502 CPU Emulator

> This document walks through every test in `cpu/cpu_test.go` for someone who has read `walkthrough_cpu_go.md` but is new to emulator testing. Each section explains **what** the code is, **why** it exists, **how** it works step by step, and provides a **real-world analogy** to build intuition.
>
> **Prerequisite**: Read the [CPU walkthrough](walkthrough_cpu_go.md) first. This document assumes you know what the P register, flags, stack, zero page, and addressing modes are.

## Why Test an Emulator?

Correctness is everything in emulator development. The 6502 CPU is the foundation that everything else runs on. If your LDA instruction loads the wrong value, every program that runs on your emulator will produce wrong results -- silently, everywhere, all the time. There is no error message from the CPU saying "hey, I loaded the wrong byte." The program just runs wrong.

Tests are your safety net. They let you change the CPU's internals -- optimize the instruction dispatch, refactor the cycle counting, fix a subtle addressing mode bug -- and know immediately if anything broke. Without tests, you are flying blind.

The test suite in `cpu_test.go` is organized in three layers:

1. **Sanity tests**: Test individual instructions in isolation (is LDA correct? Is ADC correct?)
2. **Edge case tests**: Test tricky behaviors (BCD arithmetic, hardware bugs, page crossings)
3. **Integration test**: The Klaus Dormann test -- a comprehensive real-world 6502 test suite that tests every opcode

This is the classic "test pyramid" applied to hardware emulation.

---

## Section 1: Go Testing Basics

### What It Is

Go has a built-in testing framework accessed via the `testing` package. Test functions must follow a specific naming convention and signature:

```go
import (
    "fmt"
    "os"
    "testing"
)
```

- `testing`: The standard Go test framework. Provides `*testing.T` for reporting failures.
- `fmt`: Used in the trace helper to print CPU state to stdout.
- `os`: Used to read the Dormann binary from disk and check environment variables.

Any function named `TestXxx` (where `Xxx` starts with an uppercase letter) is automatically discovered and run by `go test`. It receives a `*testing.T` argument used to report failures.

Key methods on `*testing.T`:

| Method | What It Does |
|--------|-------------|
| `t.Fatalf(format, args...)` | Fails the test immediately and stops execution with a formatted message |
| `t.Fatal(msg)` | Fails the test immediately and stops execution |
| `t.Skipf(format, args...)` | Skips the test (marks it as skipped, not failed) with a message |
| `t.Logf(format, args...)` | Prints a message that only appears in verbose mode (`go test -v`) |

### Why It Matters

Automated tests prevent **regressions** -- bugs that creep back in after you fix them. Without automated tests, you would need to manually verify every instruction works after every change to the CPU code. With tests, you run `go test ./cpu` and get a pass/fail result in seconds.

### How the Code Works

The test framework discovers and runs test functions automatically. You never call `TestLDAImmediate()` yourself -- `go test` does. The test file uses `package cpu` (the same package as `cpu.go`), not `package cpu_test`. This means tests have direct access to unexported fields like `c.A`, `c.X`, `c.SP`, and `c.P`. This is called **white-box testing** -- the tests can see inside the implementation, not just its public API.

### Real-World Analogy

Think of a car factory's quality control checklist. After every car rolls off the assembly line, a technician goes through 200 checkpoints: do the brakes work? Do the lights come on? Does the speedometer read correctly? The checklist never changes, but the production process can be refined. If a new supplier changes the brake pads, the brake test will catch any problem before the car ships.

---

## Section 2: Test Infrastructure -- FlatMemory

### What It Is

```go
type FlatMemory struct {
    Data [65536]uint8
}

func (m *FlatMemory) Read(addr uint16) uint8       { return m.Data[addr] }
func (m *FlatMemory) Write(addr uint16, val uint8) { m.Data[addr] = val }
```

`FlatMemory` is a minimal, test-only implementation of the `Memory` interface from `cpu.go`. It is a flat 64 KB byte array -- every address is readable and writable, with no restrictions.

### Why It Matters

As we saw in the Memory Interface section of the CPU walkthrough, the CPU communicates with the outside world through the `Memory` interface. It does not care what is behind the interface -- RAM, ROM, keyboard controller, or this test double.

`FlatMemory` provides the simplest possible memory: a plain array. It has two key properties that make it ideal for testing:

1. **No restrictions**: Every address is writable. On a real Apple II, addresses like `$C000-$FFFF` are ROM -- you cannot write to them. In tests, we need to set up programs and data anywhere in the address space.
2. **Direct inspection**: Tests can read `mem.Data[0x10]` to verify what the CPU wrote, bypassing the `Memory` interface. This lets tests confirm the CPU stored the right value at the right address.

### How the Code Works

**Line by line:**

```go
type FlatMemory struct {
    Data [65536]uint8
}
```

- `[65536]uint8` is a Go **array** (not a slice). Arrays in Go are fixed-size and zero-initialized. This means every byte in the 64 KB space starts as `0x00` -- equivalent to NOP instructions and zero data.
- `65536` = 2^16 = the number of unique 16-bit addresses the 6502 can reference.
- Using an array (not `[]uint8` slice) means it is stack-allocated when small enough, and the compiler knows its exact size at compile time.

```go
func (m *FlatMemory) Read(addr uint16) uint8       { return m.Data[addr] }
func (m *FlatMemory) Write(addr uint16, val uint8) { m.Data[addr] = val }
```

These two methods satisfy the `Memory` interface from `cpu.go` implicitly -- Go uses **structural typing** ("duck typing"). Because `FlatMemory` has both `Read` and `Write` methods with the exact right signatures, it automatically implements `Memory`. No `implements` keyword needed.

The `addr uint16` parameter ensures that address 65535 (`0xFFFF`) indexes `Data[65535]`, not out of bounds. Go array indexing is bounds-checked at runtime, so an out-of-range address would panic rather than corrupt memory silently.

### Real-World Analogy

A blank notebook with 65,536 numbered pages. You can write any value on any page and read it back. Unlike a real library where some shelves are "reference only -- no borrowing allowed" (ROM), this notebook has no restricted sections. Every slot is yours to use. This makes it perfect for experiments, but you would never use it in production (the real Apple II needs its ROM to boot).

---

## Section 3: Test Infrastructure -- setupCPU()

### What It Is

```go
func setupCPU(program []byte, origin uint16) (*CPU, *FlatMemory) {
    mem := &FlatMemory{}
    copy(mem.Data[origin:], program)
    mem.Data[0xFFFC] = uint8(origin)
    mem.Data[0xFFFD] = uint8(origin >> 8)
    c := New(mem)
    c.Reset()
    return c, mem
}
```

`setupCPU` is a test helper that prepares a CPU and memory for testing. It loads a program at a given address and wires the reset vector to point there.

### Why It Matters

Every test needs the same setup: create memory, load the program, set the reset vector, create a CPU, call Reset. Without this helper, every test would repeat 6+ lines of boilerplate. This is the **DRY principle** (Don't Repeat Yourself) in action. Tests become one line of setup instead of six.

### How the Code Works

**Step by step:**

**1. Create blank 64 KB memory:**
```go
mem := &FlatMemory{}
```
All 65,536 bytes are zero. `0x00` happens to be `BRK` (Break instruction) on the 6502, so accidentally running off the end of the program will trigger a BRK rather than silently doing the wrong thing.

**2. Load the program at the requested origin address:**
```go
copy(mem.Data[origin:], program)
```
Go's `copy` copies bytes from `program` into `mem.Data` starting at index `origin`. If `origin` is `0x0400` and `program` is `[0xA9, 0x42, 0xEA]`, then:
- `mem.Data[0x0400]` = `0xA9` (LDA opcode)
- `mem.Data[0x0401]` = `0x42` (immediate operand)
- `mem.Data[0x0402]` = `0xEA` (NOP opcode)

**3. Set the reset vector (little-endian):**
```go
mem.Data[0xFFFC] = uint8(origin)
mem.Data[0xFFFD] = uint8(origin >> 8)
```
The 6502's reset vector lives at `$FFFC`-`$FFFD`. When `Reset()` is called, the CPU reads these two bytes and combines them into a 16-bit address to set PC.

**Little-endian** means the low byte comes first. For origin `0x0400`:
- `uint8(0x0400)` = `uint8(1024)` = `0x00` (low byte, stored at `$FFFC`)
- `uint8(0x0400 >> 8)` = `uint8(4)` = `0x04` (high byte, stored at `$FFFD`)

The CPU reads `$FFFC` = `0x00`, `$FFFD` = `0x04`, combines them as `(high << 8) | low` = `0x0400`. PC is set to `0x0400`.

**4. Create the CPU and reset it:**
```go
c := New(mem)
c.Reset()
```
`New(mem)` creates a CPU wired to our memory. `Reset()` reads the reset vector and initializes the CPU state:
- PC = `0x0400` (read from `$FFFC`/`$FFFD`)
- SP = `$FD`
- P = `FlagU | FlagI` = `0x24` (Unused always set, Interrupt disable set)
- A, X, Y = `0x00`

**Memory after setupCPU([]byte{0xA9, 0x42, 0xEA}, 0x0400):**

```
Address   Content   Meaning
-------   -------   -------
$0400:    [A9]      LDA opcode (immediate mode)
$0401:    [42]      Immediate operand: $42
$0402:    [EA]      NOP opcode
  ...
$FFFC:    [00]      Reset vector low byte  (0x00)
$FFFD:    [04]      Reset vector high byte (0x04)
          ----
          Combined: 0x0400  <-- CPU jumps here on Reset()

CPU state after Reset():
  PC = $0400   (loaded from reset vector)
  SP = $FD     (hardwired initial value)
  A  = $00
  X  = $00
  Y  = $00
  P  = $24     (FlagU | FlagI = 0b00100100)
```

### Real-World Analogy

Setting up a laboratory workbench. You place the experiment (program bytes) at a known location on the bench (`origin`). You write "start here" on a label (`$FFFC`/`$FFFD`) and attach it to where the experiment begins. Then you power on the instrument (call `Reset()`). The instrument reads the label and jumps to the correct starting point. Now you are ready to observe what happens.

---

## Section 4: Test Infrastructure -- runN()

### What It Is

```go
func runN(c *CPU, n int) {
    for i := 0; i < n; i++ {
        c.Step()
    }
}
```

`runN` executes exactly `n` instructions on the CPU.

### Why It Matters

Many tests involve a sequence of instructions. For example, `TestADCSimple` loads `CLC; LDA #$10; ADC #$20` -- that is 3 instructions before you can check the result. Without `runN`, every test would need a chain of `c.Step(); c.Step(); c.Step()` calls. `runN(c, 3)` is cleaner and makes it obvious how many steps are being executed.

### How the Code Works

A simple `for` loop calls `c.Step()` `n` times. The return value of `Step()` (cycle count) is discarded -- if you need the cycle count, call `Step()` directly, as `TestCycleCount` does.

Note that `runN` does not validate that the CPU is in a good state between steps. If an instruction triggers unexpected behavior, the test will still run all `n` steps before the assertions fire. This is fine for these simple tests.

### Real-World Analogy

"Press the 'Step' button on a debugger N times." In a debugger, each press of the step button executes one instruction. If you want to advance 5 instructions, you press 5 times. `runN(c, 5)` does exactly that.

---

## Section 5: Sanity Tests -- Basic Instructions

These tests verify the fundamental instruction behaviors. Each one tests a single instruction (or a small combination) in complete isolation.

---

### 5.1 TestLDAImmediate -- Load Accumulator, Basic Case

#### What It Is

```go
func TestLDAImmediate(t *testing.T) {
    // LDA #$42; NOP
    c, _ := setupCPU([]byte{0xA9, 0x42, 0xEA}, 0x0400)
    c.Step()
    if c.A != 0x42 { ... }
    if c.getFlag(FlagZ) { ... }
    if c.getFlag(FlagN) { ... }
}
```

**Program bytes decoded:**

```
Byte    Hex    Assembly        Meaning
----    ---    --------        -------
$0400:  A9     LDA             Load Accumulator, Immediate mode opcode
$0401:  42     #$42            Immediate operand: the value 66 decimal
$0402:  EA     NOP             No Operation (not executed in this test)
```

#### Why It Matters

`LDA #immediate` is the most fundamental instruction on the 6502. If this is broken, nothing works. This test verifies three things:

1. **The right value loads**: A must become `$42`.
2. **Z flag is clear**: `$42` is not zero, so the Zero flag must be 0.
3. **N flag is clear**: `$42` = `0b01000010` -- bit 7 is 0, so the Negative flag must be 0.

If `LDA` did not set flags correctly, every conditional branch (`BEQ`, `BNE`, `BMI`, `BPL`) would make wrong decisions. Programs would loop forever, or exit loops too early.

#### Register/Flag Trace

```
Initial state:  PC=$0400  A=$00  Z=0  N=0

Step 1: LDA #$42
  Reads opcode $A9 from $0400, increments PC to $0401
  Reads operand $42 from $0401, increments PC to $0402
  A = $42
  Z flag: $42 != 0, so Z = 0  (clear)
  N flag: $42 = 0b01000010, bit 7 = 0, so N = 0  (clear)

Final state:    PC=$0402  A=$42  Z=0  N=0  (2 cycles consumed)
```

#### Bug This Catches

If `LDA` forgot to update the N flag: loading `$80` would not set N. Then `BMI` (Branch if Minus) would never branch correctly for negative numbers. All signed comparisons would fail.

---

### 5.2 TestLDAZero -- Zero Flag Detection

#### What It Is

```go
func TestLDAZero(t *testing.T) {
    // LDA #$00
    c, _ := setupCPU([]byte{0xA9, 0x00}, 0x0400)
    c.Step()
    if c.A != 0x00 { ... }
    if !c.getFlag(FlagZ) { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate mode
$0401:  00     #$00            Immediate operand: zero
```

#### Why It Matters

The Zero flag is critical for loops. Classic 6502 loop pattern:

```
  LDX #$0A      ; X = 10 (loop counter)
loop:
  ; ... do something ...
  DEX           ; X = X - 1, sets Z if result is 0
  BNE loop      ; branch back if Z=0 (not zero yet)
```

If `LDA #$00` does not set the Z flag, `BEQ` and `BNE` branches will malfunction. Loops that should terminate will keep running forever, or loops that should continue will exit immediately.

#### Register/Flag Trace

```
Initial state:  PC=$0400  A=$00  Z=0

Step 1: LDA #$00
  Reads opcode $A9 from $0400, PC to $0401
  Reads operand $00 from $0401, PC to $0402
  A = $00
  Z flag: $00 == 0, so Z = 1  (SET)
  N flag: $00 = 0b00000000, bit 7 = 0, so N = 0  (clear)

Final state:    PC=$0402  A=$00  Z=1  N=0
```

---

### 5.3 TestLDANegative -- Negative Flag Detection

#### What It Is

```go
func TestLDANegative(t *testing.T) {
    // LDA #$FF
    c, _ := setupCPU([]byte{0xA9, 0xFF}, 0x0400)
    c.Step()
    if !c.getFlag(FlagN) { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate mode
$0401:  FF     #$FF            Immediate operand: $FF (255 unsigned, -1 signed)
```

#### Why It Matters

The 6502 uses **two's complement** for signed numbers. In two's complement, any value with bit 7 set is "negative":

```
$00 = 0b00000000 = +0    (bit 7 = 0, positive)
$01 = 0b00000001 = +1    (bit 7 = 0, positive)
$7F = 0b01111111 = +127  (bit 7 = 0, positive, maximum positive)
$80 = 0b10000000 = -128  (bit 7 = 1, negative, most negative)
$FE = 0b11111110 = -2    (bit 7 = 1, negative)
$FF = 0b11111111 = -1    (bit 7 = 1, negative)
```

The N (Negative) flag is simply a copy of bit 7 of the result. If `LDA #$FF` does not set N, branches like `BMI` (Branch if Minus) will fail. All signed comparisons and negative number detection will be wrong.

#### Register/Flag Trace

```
Initial state:  PC=$0400  A=$00  N=0

Step 1: LDA #$FF
  Reads opcode $A9 from $0400, PC to $0401
  Reads operand $FF from $0401, PC to $0402
  A = $FF = 0b11111111
  N flag: bit 7 of $FF = 1, so N = 1  (SET)
  Z flag: $FF != 0, so Z = 0  (clear)

Final state:    PC=$0402  A=$FF  N=1  Z=0
```

---

### 5.4 TestSTAZeroPage -- Store Accumulator to Memory

#### What It Is

```go
func TestSTAZeroPage(t *testing.T) {
    // LDA #$99; STA $10
    c, mem := setupCPU([]byte{0xA9, 0x99, 0x85, 0x10}, 0x0400)
    runN(c, 2)
    if mem.Data[0x10] != 0x99 { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate mode
$0401:  99     #$99            Immediate operand: $99
$0402:  85     STA             Store Accumulator, Zero Page mode opcode
$0403:  10     $10             Zero page address: $0010
```

#### Why It Matters

`STA` (Store Accumulator) writes A to memory. This is the companion instruction to `LDA`. If `STA` is broken, your program can compute correct values in A but cannot save any of them -- every computation is thrown away. Programs cannot build data structures, update variables, or pass data between routines.

Notice the test checks `mem.Data[0x10]` directly -- it reads the memory array raw, without going through the CPU. This verifies that the CPU actually wrote to the right address in memory, not just that it "thought" it did.

#### Register/Flag Trace and Memory Diagram

```
Memory BEFORE:
  $0010: [00]   (zero-initialized)

Step 1: LDA #$99
  PC: $0400->$0402   A: $00->$99   Z=0  N=1

Step 2: STA $10
  Reads opcode $85 from $0402, PC to $0403
  Reads zero-page address $10 from $0403, PC to $0404
  Writes A ($99) to memory address $0010
  (STA does NOT modify any flags)

Memory AFTER:
  $0010: [99]   <-- CPU wrote A here

Final state:    PC=$0404  A=$99  mem[$10]=$99
```

#### Bug This Catches

If `STA` wrote to the wrong address (e.g., off by one), variables would silently be stored in the wrong location. If `STA` failed to write at all, A would contain the right value but memory would be untouched. Both bugs would cause crashes in any real program.

---

### 5.5 TestADCSimple -- Addition Without Carry

#### What It Is

```go
func TestADCSimple(t *testing.T) {
    // CLC; LDA #$10; ADC #$20
    c, _ := setupCPU([]byte{0x18, 0xA9, 0x10, 0x69, 0x20}, 0x0400)
    runN(c, 3)
    if c.A != 0x30 { ... }
    if c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  18     CLC             Clear Carry Flag (Implied mode)
$0401:  A9     LDA             Load Accumulator, Immediate mode
$0402:  10     #$10            Immediate operand: $10 (16 decimal)
$0403:  69     ADC             Add with Carry, Immediate mode opcode
$0404:  20     #$20            Immediate operand: $20 (32 decimal)
```

#### Why It Matters

`ADC` (Add with Carry) does not just add A + M. It adds A + M + Carry. The Carry flag is an **input** to ADC, not just an output. This design enables multi-byte addition: you can chain multiple ADC instructions to add 16-bit, 24-bit, or larger numbers.

Before any addition where you do not intend to add in an old carry, you must clear the carry first with `CLC`. If you skip `CLC`:
- If carry happens to be 1 from a previous operation, your result will be 1 too high.
- This is a real bug that real 6502 programmers made.

`$10 + $20 = $30` (16 + 32 = 48). No overflow, carry stays clear.

#### Register/Flag Trace

```
Step 1: CLC
  PC: $0400->$0401   C: ? -> 0   (Carry cleared)

Step 2: LDA #$10
  PC: $0401->$0403   A: $00->$10   Z=0  N=0

Step 3: ADC #$20
  PC: $0403->$0405
  Computation: A ($10) + operand ($20) + carry (0) = $30
  $10 + $20 + 0 = $30   (no overflow, no carry out)
  A: $10->$30
  C=0  (sum fits in 8 bits, no carry out)
  Z=0  ($30 != 0)
  N=0  (bit 7 of $30 = 0b00110000 is 0)

Final state:    PC=$0405  A=$30  C=0  Z=0  N=0  (total: 2+2+2=6 cycles)
```

---

### 5.6 TestADCOverflow -- 8-bit Arithmetic Wrap

#### What It Is

```go
func TestADCOverflow(t *testing.T) {
    // CLC; LDA #$FF; ADC #$01
    c, _ := setupCPU([]byte{0x18, 0xA9, 0xFF, 0x69, 0x01}, 0x0400)
    runN(c, 3)
    if c.A != 0x00 { ... }
    if !c.getFlag(FlagC) { ... }
    if !c.getFlag(FlagZ) { ... }
}
```

**Program bytes decoded:**

```
$0400:  18     CLC             Clear Carry
$0401:  A9     LDA             Load Accumulator, Immediate
$0402:  FF     #$FF            $FF = 255 decimal
$0403:  69     ADC             Add with Carry, Immediate
$0404:  01     #$01            $01 = 1 decimal
```

#### Why It Matters

8-bit arithmetic wraps at 256. When A holds `$FF` (255) and you add 1:

```
$FF + $01 = $100  (256 decimal)

But A is only 8 bits wide!
$100 in binary = 1_0000_0000
                 ^ this 9th bit doesn't fit in A

The 8-bit result: 0000_0000 = $00
The 9th bit (overflow out): 1 -> stored in the Carry flag
```

The **Carry flag captures the overflow bit**. This is exactly how multi-byte addition works: after adding the low bytes, the carry "ripples up" into the addition of the high bytes. Like an odometer rolling from 999 to 000 -- the overflow wraps around, but you know it happened.

Also: the result `$00` triggers the Zero flag. Testing that Z is set here confirms that the Z flag is evaluated on the final 8-bit result (not the 9-bit sum).

#### Register/Flag Trace

```
Step 1: CLC           C: ?->0
Step 2: LDA #$FF      A: $00->$FF   Z=0  N=1  (bit 7 of $FF is 1)

Step 3: ADC #$01
  Computation:
    $FF + $01 + carry(0) = $100
    Binary:
      1111 1111   ($FF)
    + 0000 0001   ($01)
    + 0000 0000   (carry in = 0)
    -----------
    1 0000 0000   ($100)
    ^
    Carry out = 1 (this bit doesn't fit in 8 bits)
    8-bit result = $00

  A: $FF->$00
  C=1  (carry out SET -- the 9th bit)
  Z=1  ($00 == 0, Zero SET)
  N=0  (bit 7 of $00 = 0)

Final state:    PC=$0405  A=$00  C=1  Z=1  N=0
```

#### Bug This Catches

If `ADC` did not set the Carry flag on overflow: multi-byte addition would silently lose the carry between bytes. Adding `$FFFF + 1` (the largest 16-bit number plus one) would give `$FFFF` instead of wrapping to `$0000`. Every 16-bit arithmetic operation would produce wrong results for numbers near the boundary.

---

### 5.7 TestSBCSimple -- Subtraction with Borrow

#### What It Is

```go
func TestSBCSimple(t *testing.T) {
    // SEC; LDA #$30; SBC #$10
    c, _ := setupCPU([]byte{0x38, 0xA9, 0x30, 0xE9, 0x10}, 0x0400)
    runN(c, 3)
    if c.A != 0x20 { ... }
    if !c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  38     SEC             Set Carry Flag (Implied mode)
$0401:  A9     LDA             Load Accumulator, Immediate
$0402:  30     #$30            $30 = 48 decimal
$0403:  E9     SBC             Subtract with Carry, Immediate mode opcode
$0404:  10     #$10            $10 = 16 decimal
```

#### Why It Matters

`SBC` (Subtract with Carry) uses the carry flag in an **inverted** sense compared to `ADC`. The SBC formula is:

```
A = A - M - (1 - C)
```

Which means:
- If **C = 1** (no borrow), the formula is `A - M - 0` = pure subtraction.
- If **C = 0** (borrow in), the formula is `A - M - 1` = subtract an extra 1 (the borrow).

So before a subtraction where you want a "clean" subtraction without borrow, you must **set** carry with `SEC` first. This is the opposite of addition (which needs `CLC`).

After the subtraction:
- **C = 1** means "no borrow was needed" (the result was >= 0). This is also called "no borrow" or "carry out."
- **C = 0** means "a borrow was needed" (the result went negative).

For `$30 - $10 = $20`: no borrow needed, C stays 1.

#### Register/Flag Trace

```
Step 1: SEC           C: 0->1   (Set carry = "no borrow" starting state)

Step 2: LDA #$30      A: $00->$30   Z=0  N=0

Step 3: SBC #$10
  Formula: A - M - (1 - C) = $30 - $10 - (1 - 1) = $30 - $10 - 0 = $20
  $30 - $10 = $20   (48 - 16 = 32, no borrow needed)
  A: $30->$20
  C=1  (no borrow, carry stays set)
  Z=0  ($20 != 0)
  N=0  (bit 7 of $20 = 0b00100000 is 0)

Final state:    PC=$0405  A=$20  C=1  Z=0  N=0
```

---

### 5.8 TestJMPAbsolute -- Unconditional Jump

#### What It Is

```go
func TestJMPAbsolute(t *testing.T) {
    c, mem := setupCPU([]byte{0x4C, 0x00, 0x05}, 0x0400)
    mem.Data[0x0500] = 0xEA  // NOP at $0500
    c.Step()
    if c.PC != 0x0500 { ... }
}
```

**Program bytes decoded:**

```
$0400:  4C     JMP             Jump to Absolute address opcode
$0401:  00     $__00           Low byte of target address (little-endian)
$0402:  05     $05__           High byte of target address
                               Combined: $0500
```

**Little-endian encoding:**

```
JMP $0500 is encoded as: 4C 00 05
                                ^^ high byte of $0500
                             ^^ low byte of $0500
```

Always low byte first, then high byte. This is the 6502's little-endian convention for all 16-bit values in memory.

#### Why It Matters

`JMP` changes PC to an arbitrary address. This is how programs implement loops (jump back to the start), switch statements (jump to a case), and go-to logic. Without working `JMP`, programs can only execute sequentially from top to bottom.

The test also places a `NOP` at `$0500`. This ensures the landing address has a valid instruction, preventing accidental crashes if the test were extended to execute more steps.

#### Register/Flag Trace

```
Step 1: JMP $0500
  Reads opcode $4C from $0400, PC to $0401
  Reads low byte $00 from $0401, PC to $0402
  Reads high byte $05 from $0402, PC to $0403
  Combines: target = ($05 << 8) | $00 = $0500
  PC = $0500  (jumps directly -- does NOT push return address)

Final state:    PC=$0500  (3 cycles consumed)
```

---

### 5.9 TestJSRAndRTS -- Subroutine Call and Return

#### What It Is

```go
func TestJSRAndRTS(t *testing.T) {
    c, mem := setupCPU([]byte{0x20, 0x00, 0x05}, 0x0400)
    mem.Data[0x0500] = 0xA9  // LDA #$77
    mem.Data[0x0501] = 0x77
    mem.Data[0x0502] = 0x60  // RTS
    runN(c, 3) // JSR, LDA, RTS
    if c.A != 0x77 { ... }
    if c.PC != 0x0403 { ... }
}
```

**Program bytes decoded:**

```
$0400:  20     JSR             Jump to SubRoutine, Absolute mode opcode
$0401:  00     $__00           Low byte of subroutine address
$0402:  05     $05__           High byte: target = $0500

$0500:  A9     LDA             Load Accumulator, Immediate
$0501:  77     #$77            Operand: $77
$0502:  60     RTS             Return from Subroutine
```

#### Why It Matters

`JSR`/`RTS` enable **subroutines** -- reusable chunks of code that can be called from anywhere and always return to where they were called from. Without subroutines, programs cannot have functions. Every piece of functionality would need to be copy-pasted everywhere it was needed.

`JSR` pushes the **return address** onto the stack so that `RTS` knows where to go back. This is the exact same mechanism used by function calls in every programming language -- the "call stack" is just a software version of this 6502 hardware stack.

#### The JSR Return Address Convention

There is a subtle but important detail: **JSR pushes `PC - 1`**, not `PC`.

After reading the JSR opcode ($0400) and the two address bytes ($0401, $0402), PC = $0403. But JSR pushes `$0402` (PC - 1 = the last byte of the JSR instruction itself). Then RTS pulls the address and **adds 1**, giving `$0403`.

This is a hardware quirk: JSR and RTS are designed to compensate for each other. The net result is that execution returns to `$0403` -- the byte after the JSR instruction.

#### Stack Trace Diagram

The stack lives at page 1 (`$0100`-`$01FF`). SP starts at `$FD`. Pushing decrements SP first.

```
BEFORE JSR:
  SP = $FD
  Stack: $01FC = ??   $01FD = ??   $01FE = ??

JSR $0500:
  PC has advanced to $0403 during opcode fetch
  JSR pushes PC-1 = $0402 onto stack (high byte first, then low byte)

  Push high byte ($04):
    mem[$01FD] = $04     SP: $FD -> $FC
  Push low byte ($02):
    mem[$01FC] = $02     SP: $FC -> $FB

Stack AFTER JSR:
  SP = $FB
  $01FC: [02]   <-- low byte of return address
  $01FD: [04]   <-- high byte of return address
  PC = $0500   (jumped to subroutine)

Step 2: LDA #$77 at $0500
  A = $77,  PC = $0502

Step 3: RTS at $0502:
  Pull low byte from $01FC: $02   SP: $FB -> $FC
  Pull high byte from $01FD: $04  SP: $FC -> $FD
  Combined address: $0402
  RTS adds 1: PC = $0402 + 1 = $0403

Stack AFTER RTS:
  SP = $FD   (restored to original value)
  PC = $0403  (returned to instruction after JSR)

Final state:    PC=$0403  A=$77  SP=$FD
```

---

### 5.10 TestBranch -- Conditional Branch (BEQ)

#### What It Is

```go
func TestBranch(t *testing.T) {
    // $0400: A9 00     LDA #$00
    // $0402: F0 02     BEQ $0406
    // $0404: A9 FF     LDA #$FF  (should be skipped)
    // $0406: EA        NOP
    c, _ := setupCPU([]byte{0xA9, 0x00, 0xF0, 0x02, 0xA9, 0xFF, 0xEA}, 0x0400)
    runN(c, 3) // LDA, BEQ (taken), NOP
    if c.A != 0x00 { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate
$0401:  00     #$00            Operand: zero
$0402:  F0     BEQ             Branch if Equal (Zero flag set), Relative mode opcode
$0403:  02     +2              Signed branch offset: +2 bytes
$0404:  A9     LDA             Load Accumulator, Immediate  (SKIPPED)
$0405:  FF     #$FF            Operand: $FF  (SKIPPED)
$0406:  EA     NOP             No Operation  (branch lands here)
```

#### Why It Matters

Branch instructions implement `if` statements, loops, and all conditional logic. `BEQ` (Branch if Equal to Zero) branches when the Z flag is set. Since `LDA #$00` sets Z=1, the branch is taken.

**How the signed offset works:**

After reading the BEQ opcode at `$0402` and the offset at `$0403`, PC has advanced to `$0404`. The branch destination is:

```
branch target = PC_after_fetch + signed_offset
             = $0404 + $02
             = $0406
```

The offset `$02` means "skip 2 bytes forward from the instruction after BEQ." This bypasses the 2-byte `LDA #$FF` instruction at `$0404`-`$0405`.

**Offsets are signed**: `$02` = +2 (forward branch). Values like `$FE` would be -2 (backward branch, used for loops). The range is -128 to +127.

#### Memory Layout Diagram

```
$0400: [A9][00]   LDA #$00      -- executes, sets Z=1
$0402: [F0][02]   BEQ +2        -- Z=1, branch TAKEN
                  |
                  | offset $02 added to PC=$0404
                  |
                  v (skips the next 2 bytes)
$0404: [A9][FF]   LDA #$FF      -- SKIPPED (PC jumps over this)
$0406: [EA]       NOP           -- LANDS HERE (step 3)
```

#### Register/Flag Trace

```
Step 1: LDA #$00    PC: $0400->$0402   A: $00->$00   Z=1  N=0
Step 2: BEQ +2      PC: $0402->$0406   (branch taken because Z=1, skips $0404-$0405)
Step 3: NOP         PC: $0406->$0407   (no change to registers or flags)

Final state:    PC=$0407  A=$00  (LDA #$FF was never executed)
```

---

### 5.11 TestStackPushPull -- PHA and PLA

#### What It Is

```go
func TestStackPushPull(t *testing.T) {
    // LDA #$AB; PHA; LDA #$00; PLA
    c, _ := setupCPU([]byte{0xA9, 0xAB, 0x48, 0xA9, 0x00, 0x68}, 0x0400)
    runN(c, 4)
    if c.A != 0xAB { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate
$0401:  AB     #$AB            Operand: $AB (171 decimal)
$0402:  48     PHA             Push Accumulator to Stack (Implied mode)
$0403:  A9     LDA             Load Accumulator, Immediate
$0404:  00     #$00            Operand: $00 (overwrite A with zero)
$0405:  68     PLA             Pull Accumulator from Stack (Implied mode)
```

#### Why It Matters

`PHA`/`PLA` let you save and restore the accumulator across operations that would otherwise clobber it. This is essential for:
- Preserving A before calling a subroutine that uses A
- Temporarily storing A while computing something else in A
- Passing values between code sections

As we saw in the CPU walkthrough, the stack is at page 1 (`$0100`-`$01FF`). SP starts at `$FD` and **decrements** before each push (full descending stack).

#### Stack Diagram at Each Step

```
Initial stack state:
  SP = $FD
  $01FE = ??   (uninitialized, don't care)

Step 1: LDA #$AB
  A = $AB

  Stack:
  SP = $FD (unchanged)
  $01FE = ??

Step 2: PHA  (Push Accumulator)
  Write A ($AB) to memory[$0100 | SP] = mem[$01FD]
  Decrement SP: $FD -> $FC

  Stack:
  SP = $FC
  $01FD = [AB]  <-- pushed value
  $01FE = ??

Step 3: LDA #$00
  A = $00   (A is now clobbered!)

  Stack:
  SP = $FC   (stack unchanged)
  $01FD = [AB]  <-- still there

Step 4: PLA  (Pull Accumulator)
  Increment SP: $FC -> $FD
  Read A from memory[$0100 | SP] = mem[$01FD] = $AB
  A = $AB   (restored!)
  Z flag: $AB != 0, Z=0
  N flag: bit 7 of $AB = 0b10101011 = 1, N=1

  Stack:
  SP = $FD   (restored to original)
  $01FD = [AB]  <-- still in memory, but considered "above" the stack pointer now

Final state:    PC=$0406  A=$AB  SP=$FD
```

Note: The old value `$AB` is still physically in memory at `$01FD`, but the stack pointer now points above it, so it is "gone" from the stack's perspective. The next push will overwrite it.

---

### 5.12 TestIndexedIndirect -- LDA (Indirect,X)

#### What It Is

```go
func TestIndexedIndirect(t *testing.T) {
    c, mem := setupCPU([]byte{0xA2, 0x10, 0xA1, 0x10}, 0x0400)
    mem.Data[0x20] = 0x00
    mem.Data[0x21] = 0x03
    mem.Data[0x0300] = 0x42
    runN(c, 2)
    if c.A != 0x42 { ... }
}
```

**Program bytes decoded:**

```
$0400:  A2     LDX             Load X Register, Immediate mode
$0401:  10     #$10            Operand: $10 (16 decimal)
$0402:  A1     LDA             Load Accumulator, (Indirect,X) mode opcode
$0403:  10     ($10,X)         Zero page base address for pointer lookup
```

**Memory pre-loaded by the test:**

```
$0020: [00]   Low byte of pointer (= $0300)
$0021: [03]   High byte of pointer
$0300: [42]   The actual data value
```

#### Why It Matters

IndirectX (`(zp,X)`) is one of the most powerful addressing modes on the 6502. It implements **an array of pointers** -- you use X to select which pointer, then dereference it.

As explained in the `resolve()` section of the CPU walkthrough, the full resolution chain has four steps:
1. Take the zero-page base address from the instruction ($10)
2. Add X register ($10) to get the zero-page address of the pointer ($10 + $10 = $20)
3. Read the 16-bit pointer from zero page (low byte from $20, high byte from $21 = $0300)
4. Read the final value from the pointed-to address ($0300 = $42)

The addition in step 2 **wraps within zero page** -- if the sum exceeds $FF, it wraps to $00 (staying within page 0), not crossing into page 1.

#### Pointer Chain Diagram

```
Zero Page                     RAM
─────────────────────         ─────────────
Addr  Value                   Addr  Value
$10:  [--]  base address      $0300: [42]  <-- final data
$11:  [--]  (not used)                         (what we load into A)
...
$20:  [00]  pointer low byte ─────────────────┐
$21:  [03]  pointer high byte                 │
                                              │ pointer = $0300
Step-by-step:                                 │
  1. Instruction operand = $10  (ZP base)     │
  2. Add X ($10): $10 + $10 = $20 ────────────┘
  3. Read pointer @ $20/$21: low=$00, high=$03 -> $0300
  4. Read value @ $0300: $42
  5. A = $42
```

#### Register/Flag Trace

```
Step 1: LDX #$10    PC: $0400->$0402   X: $00->$10   Z=0  N=0
Step 2: LDA ($10,X)
  PC: $0402->$0404
  Pointer address = $10 + X($10) = $20 (within zero page)
  Pointer value   = mem[$20]<<0 | mem[$21]<<8 = $0300
  Final value     = mem[$0300] = $42
  A: $00->$42   Z=0  N=0

Final state:    PC=$0404  A=$42  X=$10
```

---

### 5.13 TestIndirectIndexed -- LDA (Indirect),Y

#### What It Is

```go
func TestIndirectIndexed(t *testing.T) {
    c, mem := setupCPU([]byte{0xA0, 0x05, 0xB1, 0x30}, 0x0400)
    mem.Data[0x30] = 0x00
    mem.Data[0x31] = 0x02
    mem.Data[0x0205] = 0x99
    runN(c, 2)
    if c.A != 0x99 { ... }
}
```

**Program bytes decoded:**

```
$0400:  A0     LDY             Load Y Register, Immediate mode
$0401:  05     #$05            Operand: $05
$0402:  B1     LDA             Load Accumulator, (Indirect),Y mode opcode
$0403:  30     ($30),Y         Zero page address where pointer lives
```

**Memory pre-loaded:**

```
$0030: [00]   Low byte of base pointer
$0031: [02]   High byte of base pointer (= $0200)
$0205: [99]   Final data ($0200 + Y($05) = $0205)
```

#### Why It Matters

IndirectY (`(zp),Y`) is the other indirect mode. Its resolution order is **reversed** from IndirectX:
- **IndirectX**: add index FIRST, then dereference -- selects which pointer to use
- **IndirectY**: dereference FIRST, then add index -- selects an offset into the pointed-to array

IndirectY is used for iterating through arrays pointed to by a zero-page pointer. The pointer in ZP says "where does my array start?" and Y is the index into that array. This is the classic pattern for processing strings and data buffers.

#### Comparison: IndirectX vs IndirectY

```
IndirectX -- (zp, X):
  1. [ZP base addr] + X  = pointer address in ZP
  2. Read pointer from ZP
  3. Read value from pointer
  "Pick pointer #X, then dereference it"
  Use case: dispatch tables, arrays of function pointers

IndirectY -- (zp), Y:
  1. Read pointer from [ZP base addr]
  2. pointer + Y = final address
  3. Read value from final address
  "Dereference the pointer, then index Y bytes into the result"
  Use case: iterate through a buffer at a dynamic base address
```

#### Pointer Chain Diagram

```
Zero Page                       RAM
──────────────────              ─────────────────
Addr  Value                     Addr  Value
$30:  [00]  pointer low  ──┐    $0200: [--]
$31:  [02]  pointer high ──┘    $0201: [--]
           pointer = $0200      $0202: [--]
                          +Y    $0203: [--]
                     (Y=$05)    $0204: [--]
                                $0205: [99]  <-- final data
                                       ^
                                       $0200 + $05 = $0205
```

#### Register/Flag Trace

```
Step 1: LDY #$05    PC: $0400->$0402   Y: $00->$05   Z=0  N=0
Step 2: LDA ($30),Y
  PC: $0402->$0404
  Base pointer = mem[$30]<<0 | mem[$31]<<8 = $0200
  Final address = $0200 + Y($05) = $0205
  Value = mem[$0205] = $99
  A: $00->$99   N=1 (bit 7 of $99 = 1)  Z=0

Final state:    PC=$0404  A=$99  Y=$05
```

---

### 5.14 TestCycleCount -- Cycle Accuracy

#### What It Is

```go
func TestCycleCount(t *testing.T) {
    // LDA #imm = 2 cycles, STA zp = 3 cycles, NOP = 2 cycles -> total 7
    c, _ := setupCPU([]byte{0xA9, 0x01, 0x85, 0x00, 0xEA}, 0x0400)
    total := 0
    for i := 0; i < 3; i++ {
        total += c.Step()
    }
    if total != 7 { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Immediate mode (2 cycles)
$0401:  01     #$01
$0402:  85     STA             Zero Page mode  (3 cycles)
$0403:  00     $00             Zero page address
$0404:  EA     NOP             Implied mode    (2 cycles)
                               Total: 2+3+2 = 7 cycles
```

**Why each instruction takes different cycles:**

| Instruction | Mode | Cycles | Why |
|-------------|------|--------|-----|
| `LDA #imm` | Immediate | 2 | 1 fetch opcode + 1 fetch operand |
| `STA $zp` | Zero Page | 3 | 1 fetch opcode + 1 fetch address + 1 write memory |
| `NOP` | Implied | 2 | 1 fetch opcode + 1 internal cycle |

#### Why It Matters

Cycle counts are critical for emulator correctness beyond just "does the instruction work?" The original Apple II's video generation, disk drive timing, and sound chips all depend on **exactly** how many cycles have elapsed. If your CPU runs too fast or too slow:
- Video scanlines will be drawn at wrong positions (graphical glitches)
- Disk drive read timing will be wrong (disks won't load)
- Sound will be at the wrong pitch or stutter

This test confirms the cycle counter is accurate, not just that the instructions produce correct results.

#### Cycle Trace

```
Step 1: LDA #$01    returns 2 cycles    total = 2
Step 2: STA $00     returns 3 cycles    total = 5
Step 3: NOP         returns 2 cycles    total = 7

Assertion: total == 7 ✓
```

---

## Section 6: Edge Case Tests

These tests go beyond basic instruction behavior and verify tricky corner cases: BCD arithmetic, hardware bugs, page crossings, bit manipulation, and interrupt handling.

---

### 6.1 TestADCDecimalMode -- BCD Addition

#### What It Is

```go
func TestADCDecimalMode(t *testing.T) {
    // SED; CLC; LDA #$15; ADC #$27 -> BCD: 15+27 = 42
    c, _ := setupCPU([]byte{0xF8, 0x18, 0xA9, 0x15, 0x69, 0x27}, 0x0400)
    runN(c, 4)
    if c.A != 0x42 { ... }
    if c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  F8     SED             Set Decimal Flag (enables BCD mode)
$0401:  18     CLC             Clear Carry
$0402:  A9     LDA             Load Accumulator, Immediate
$0403:  15     #$15            Operand: $15
$0404:  69     ADC             Add with Carry, Immediate
$0405:  27     #$27            Operand: $27
```

#### Why It Matters

**BCD (Binary-Coded Decimal)** is an alternative way to represent decimal numbers in binary. In BCD, each **nibble** (4 bits) holds one decimal digit (0-9):

```
Normal binary: $15 = 0001 0101 = 21 decimal
BCD encoding:  $15 = 0001 0101 = "15" decimal (one-five, not twenty-one)
               high nibble: 0001 = 1
               low  nibble: 0101 = 5
               BCD value: "15"
```

In BCD mode (D flag set), `ADC` adds two BCD-encoded values and produces a BCD-encoded result:

```
BCD:  $15 + $27:
  Lower nibbles: 5 + 7 = 12 → record 2, carry 1 to upper nibble
  Upper nibbles: 1 + 2 + 1(carry) = 4
  Result: $42  (which means "forty-two" in BCD)
```

#### Nibble-by-Nibble Addition

```
  $15  =  0001 | 0101  (BCD: 1 | 5)
+ $27  =  0010 | 0111  (BCD: 2 | 7)
────────────────────────────
Low nibble:  5 + 7 = 12
             12 >= 10: record (12 - 10) = 2, carry 1 to high nibble
High nibble: 1 + 2 + 1(carry) = 4
             4 < 10: record 4, no carry out

Result: $42  (BCD: 4 | 2 = "forty-two") ✓
Carry: 0 (no overflow past "99")
```

#### Register/Flag Trace

```
Step 1: SED         D flag: 0->1   (Decimal mode ON)
Step 2: CLC         C flag: ?->0
Step 3: LDA #$15    A: $00->$15   Z=0  N=0
Step 4: ADC #$27    A: $15->$42   C=0  Z=0  N=0
        (BCD: 15 + 27 = 42, no carry)

Final state:    PC=$0406  A=$42  D=1  C=0
```

---

### 6.2 TestADCDecimalCarry -- BCD Carry

#### What It Is

```go
func TestADCDecimalCarry(t *testing.T) {
    // SED; CLC; LDA #$99; ADC #$01 -> BCD: 99+1 = 00 with carry
    c, _ := setupCPU([]byte{0xF8, 0x18, 0xA9, 0x99, 0x69, 0x01}, 0x0400)
    runN(c, 4)
    if c.A != 0x00 { ... }
    if !c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  F8     SED             Set Decimal mode
$0401:  18     CLC             Clear Carry
$0402:  A9     LDA             Load Accumulator, Immediate
$0403:  99     #$99            BCD value "ninety-nine"
$0404:  69     ADC             Add with Carry, Immediate
$0405:  01     #$01            BCD value "one"
```

#### Why It Matters

The BCD equivalent of the binary overflow test (5.6). In BCD, $99 is the maximum two-digit decimal number (99). Adding 1 overflows to $00 with a carry. This is the BCD odometer rolling over from 99 to 00.

#### BCD Carry Trace

```
BCD: 99 + 01:
  Lower nibbles: 9 + 1 = 10 → record 0, carry 1
  Upper nibbles: 9 + 0 + 1(carry) = 10 → record 0, carry 1 OUT

Result: $00  (BCD "zero")  ✓
Carry: 1    (overflow past "99")  ✓
```

#### Register/Flag Trace

```
Step 1: SED         D=1
Step 2: CLC         C=0
Step 3: LDA #$99    A=$99
Step 4: ADC #$01    A: $99->$00   C=1  Z=0  N=0
        (BCD: 99 + 1 = 100, but only 2 BCD digits: 00, carry out)

Final state:    PC=$0406  A=$00  D=1  C=1  Z=0
```

**NMOS 6502 BCD flag quirk:** You might expect Z=1 because the BCD result is $00. But on the NMOS 6502, the Z flag in BCD mode is based on the **binary** intermediate result, not the BCD-corrected result. Binary: `$99 + $01 = $9A`. Since `$9A != 0`, Z=0. This is a well-known hardware quirk -- the test does not assert on Z for this exact reason.

---

### 6.3 TestSBCDecimalMode -- BCD Subtraction

#### What It Is

```go
func TestSBCDecimalMode(t *testing.T) {
    // SED; SEC; LDA #$42; SBC #$15 -> BCD: 42-15 = 27
    c, _ := setupCPU([]byte{0xF8, 0x38, 0xA9, 0x42, 0xE9, 0x15}, 0x0400)
    runN(c, 4)
    if c.A != 0x27 { ... }
    if !c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  F8     SED             Set Decimal mode
$0401:  38     SEC             Set Carry (= "no borrow" for SBC)
$0402:  A9     LDA             Load Accumulator, Immediate
$0403:  42     #$42            BCD value "forty-two"
$0404:  E9     SBC             Subtract with Carry, Immediate
$0405:  15     #$15            BCD value "fifteen"
```

#### BCD Subtraction Trace

```
BCD: 42 - 15:
  Lower nibbles: 2 - 5 → need borrow: (12 - 5) = 7, borrow 1 from upper
  Upper nibbles: 4 - 1 - 1(borrow) = 2

Result: $27  (BCD "twenty-seven")  ✓
Carry: 1 (no overall borrow, result >= 0)  ✓
```

#### Register/Flag Trace

```
Step 1: SED         D=1
Step 2: SEC         C=1
Step 3: LDA #$42    A=$42
Step 4: SBC #$15    A: $42->$27   C=1  Z=0  N=0
        (BCD: 42 - 15 = 27, no borrow)

Final state:    PC=$0406  A=$27  D=1  C=1
```

---

### 6.4 TestJMPIndirectBug -- The Famous NMOS 6502 Hardware Bug

#### What It Is

```go
func TestJMPIndirectBug(t *testing.T) {
    c, mem := setupCPU([]byte{0x6C, 0xFF, 0x02}, 0x0400)
    mem.Data[0x02FF] = 0x80  // low byte of target
    mem.Data[0x0200] = 0x06  // high byte (BUG: wraps to $0200, not $0300!)
    mem.Data[0x0300] = 0x99  // this would be read without the bug
    c.Step()
    if c.PC != 0x0680 { ... }
}
```

**Program bytes decoded:**

```
$0400:  6C     JMP             Jump Indirect, Indirect mode opcode
$0401:  FF     $__FF           Low byte of pointer address
$0402:  02     $02__           High byte: pointer address = $02FF
```

#### Why It Matters -- The Most Famous 6502 Bug

`JMP ($02FF)` in indirect mode tells the CPU: "go to address $02FF, read two bytes (low then high), and jump to the combined address." A **correct** implementation would:
1. Read low byte of target from `$02FF` = `$80`
2. Read high byte of target from `$0300` = `$99`
3. Jump to `$9980`

But the NMOS 6502 has a silicon bug: **when the pointer address ends in $FF (i.e., is at a page boundary), the CPU reads the high byte from $xx00 instead of $(xx+1)00**. It wraps within the same page rather than crossing to the next page.

What actually happens:
1. Read low byte of target from `$02FF` = `$80` (correct)
2. Should read high byte from `$0300`, but **bug**: reads from `$0200` = `$06`
3. Jumps to `$0680` (WRONG!)

```
Memory layout:
──────────────────────────────────────────────────────────
Address   Value   What it is
$02FF:    $80    Low byte of target pointer  (read correctly)
$0200:    $06    High byte -- BUG reads HERE (wraps within page 2)
$0300:    $99    High byte -- should be read HERE (next page)
──────────────────────────────────────────────────────────

Buggy 6502 behavior:
  Low byte:  mem[$02FF] = $80
  High byte: mem[$02__+1 & $FF00 | 0x00] = mem[$0200] = $06
             ^^^^^^^^^^^^^^^^^^ (same page, offset wraps to $00)
  Target PC = ($06 << 8) | $80 = $0680

Correct behavior would be:
  Low byte:  mem[$02FF] = $80
  High byte: mem[$0300] = $99
  Target PC = ($99 << 8) | $80 = $9980

The test EXPECTS the buggy behavior ($0680) because
that is what a real 6502 does!
```

This bug was documented in the original MOS Technology 6502 data sheet. Every Apple II programmer knew: **never place a JMP indirect pointer at an address ending in $FF**. If you had a pointer at `$1EFF`, move it to `$1F00` or `$1EFD`.

Our emulator **faithfully replicates this bug** because Apple II software was written to work around it. If our emulator were "too correct" and fixed the bug, programs that worked around it might behave differently.

---

### 6.5 TestAbsoluteXPageCross -- Page Crossing Penalty

#### What It Is

```go
func TestAbsoluteXPageCross(t *testing.T) {
    // LDA $10F0,X with X=$20 -> address $1110, crosses page.
    c, mem := setupCPU([]byte{0xBD, 0xF0, 0x10}, 0x0400)
    c.X = 0x20
    mem.Data[0x1110] = 0xAB
    cycles := c.Step()
    if c.A != 0xAB { ... }
    if cycles != 5 { ... }
}
```

**Program bytes decoded:**

```
$0400:  BD     LDA             Load Accumulator, Absolute,X mode opcode
$0401:  F0     $__F0           Low byte of base address
$0402:  10     $10__           High byte: base = $10F0

X register = $20 (set manually before Step())
Effective address = $10F0 + $20 = $1110
```

#### Why It Matters

Most absolute indexed instructions take 4 cycles. But if the base address and the indexed address are on **different pages** (different high bytes), the CPU takes an extra cycle -- 5 total.

Page crossing: `$10F0` is on page `$10`. `$10F0 + $20 = $1110` is on page `$11`. The high bytes differ (`$10` != `$11`), so a page boundary was crossed.

**Why does a page cross cost an extra cycle?**

The 6502 is pipelined: while computing the effective address, it optimistically adds only the low byte of the offset to start fetching early. If there is no carry out of the low byte addition (no page cross), the fetch is already at the right address. If there IS a carry (page cross), the high byte is wrong, and the CPU must redo the memory fetch with the corrected address. That correction costs one extra cycle.

```
Base address: $10F0
        Low byte: F0
Add X:          +20
        Low byte sum: F0 + 20 = 110  (overflow! carry out = 1)

Without carry: CPU would read from $10 | 10 = $1010 (WRONG page)
With carry:    CPU adds 1 to high byte: $10 + 1 = $11
               Final address: $1110 (correct)

The correction = 1 extra cycle
```

#### Cycle Trace

```
Base address:    $10F0  (page $10)
X offset:        $0020
Effective addr:  $1110  (page $11)

Page cross: $10 != $11  → YES, page boundary crossed

LDA abs,X base cycles:  4
Page cross penalty:     +1
Total:                  5

mem[$1110] = $AB  →  A = $AB  ✓
cycles = 5  ✓
```

---

### 6.6 TestROLCarryChain -- Rotate Left Through Carry

#### What It Is

```go
func TestROLCarryChain(t *testing.T) {
    // SEC; LDA #$80; ROL A -> carry in, bit 7 out to carry, result = $01
    c, _ := setupCPU([]byte{0x38, 0xA9, 0x80, 0x2A}, 0x0400)
    runN(c, 3)
    if c.A != 0x01 { ... }
    if !c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  38     SEC             Set Carry (C=1)
$0401:  A9     LDA             Load Accumulator, Immediate
$0402:  80     #$80            $80 = 0b10000000
$0403:  2A     ROL A           Rotate Left through Carry, Accumulator mode
```

#### Why It Matters

`ROL` (Rotate Left) is not just a shift -- it is a **rotation through the carry flag**. The 9-bit rotate works like this:

```
9-bit register: [C][bit7][bit6][bit5][bit4][bit3][bit2][bit1][bit0]

ROL shifts everything left by one position:
- Old bit 7 → new Carry
- Old Carry  → new bit 0
- All other bits shift one position left
```

This lets you do multi-byte shifts: by chaining ROL across multiple bytes, each ROL passes its overflow bit (via carry) to the next byte's ROL. You can shift a 16-bit or 32-bit value left by any number of positions this way.

#### Bit-Level Diagram

```
BEFORE ROL:
  Carry = 1
  A = $80 = 1 0 0 0 0 0 0 0
              ^
              bit 7 (MSB)

The 9-bit register: [C=1][1][0][0][0][0][0][0][0]
                                                  (bit 0, LSB)

ROL shifts everything left:
[old bit7][bit6][bit5][bit4][bit3][bit2][bit1][bit0][old C]
    │                                               │
    └─ goes to new C                                └─ becomes new bit 0

AFTER ROL:
  New Carry = old bit 7 = 1  (SET)
  New A     = [old bits 6-0][old C]
            = [0][0][0][0][0][0][0][1]
            = 0b00000001
            = $01

Final bit diagram:
  C = 1
  A = $01 = 0 0 0 0 0 0 0 1
                            ^
                            bit 0 (LSB) = old carry (1)
```

#### Register/Flag Trace

```
Step 1: SEC         C: 0->1
Step 2: LDA #$80    A: $00->$80   N=1  Z=0  (bit 7 of $80 = 1)
Step 3: ROL A
  Old A:   $80 = 10000000
  Old C:   1
  New A:   00000001  = $01  (shifted left, old carry inserted at bit 0)
  New C:   1  (old bit 7 of $80 was 1)
  A: $80->$01   C=1  N=0  Z=0

Final state:    PC=$0404  A=$01  C=1  N=0  Z=0
```

---

### 6.7 TestBITFlags -- BIT Instruction Flag Testing

#### What It Is

```go
func TestBITFlags(t *testing.T) {
    c, mem := setupCPU([]byte{0xA9, 0x00, 0x24, 0x10}, 0x0400)
    mem.Data[0x10] = 0xC0
    runN(c, 2)
    if !c.getFlag(FlagZ) { ... }
    if !c.getFlag(FlagN) { ... }
    if !c.getFlag(FlagV) { ... }
}
```

**Program bytes decoded:**

```
$0400:  A9     LDA             Load Accumulator, Immediate
$0401:  00     #$00            A = $00
$0402:  24     BIT             Bit Test, Zero Page mode opcode
$0403:  10     $10             Zero page address to test
```

**Memory setup:**

```
$0010: [C0]   The memory value being tested: $C0 = 0b11000000
```

#### Why It Matters

`BIT` is a unique instruction that tests bits in memory **without changing A**. It sets three flags based on the memory value and the AND of A with the memory value:

| Flag | Set by | Meaning |
|------|--------|---------|
| Z | `A AND mem` == 0 | Zero: no bits in common between A and mem |
| N | bit 7 of mem | Negative: memory's sign bit is set |
| V | bit 6 of mem | Overflow: memory's "overflow" bit is set |

The key insight: **N and V are set directly from the memory value's bits 7 and 6, not from the AND result**. Only Z uses the AND.

This makes `BIT` useful for:
- Testing hardware status registers (where bit 7 and 6 have specific meanings)
- Checking whether any bits in A match bits in memory (Z flag)
- Without touching A (which you might need for the next operation)

#### Bit Diagram

```
A   = $00 = 0 0 0 0 0 0 0 0
M   = $C0 = 1 1 0 0 0 0 0 0
             ^ ^
             | └── bit 6 = 1  →  V flag = 1
             └──── bit 7 = 1  →  N flag = 1

AND = A & M = $00 & $C0 = 0 0 0 0 0 0 0 0 = $00
                                             Z flag: $00 == 0 → Z = 1

Summary:
  Z = 1  (A AND M is zero -- no bits in common)
  N = 1  (bit 7 of M is 1)
  V = 1  (bit 6 of M is 1)
  A = $00  (UNCHANGED by BIT instruction)
```

---

### 6.8 TestOverflowFlag -- Signed Arithmetic Overflow

#### What It Is

```go
func TestOverflowFlag(t *testing.T) {
    // CLC; LDA #$7F; ADC #$01 -> 127 + 1 = 128 -> signed overflow
    c, _ := setupCPU([]byte{0x18, 0xA9, 0x7F, 0x69, 0x01}, 0x0400)
    runN(c, 3)
    if c.A != 0x80 { ... }
    if !c.getFlag(FlagV) { ... }
    if !c.getFlag(FlagN) { ... }
}
```

**Program bytes decoded:**

```
$0400:  18     CLC             Clear Carry
$0401:  A9     LDA             Load Accumulator, Immediate
$0402:  7F     #$7F            $7F = 127 (maximum positive signed 8-bit)
$0403:  69     ADC             Add with Carry, Immediate
$0404:  01     #$01            1
```

#### Why It Matters

The **V (Overflow) flag** detects signed arithmetic overflow -- when the mathematical result is correct for unsigned arithmetic but incorrect for signed interpretation. This is different from the carry flag (which detects unsigned overflow).

Signed overflow rule: V is set when you add two numbers of the same sign and get a result of the opposite sign.

```
Two's complement signed ranges:
  $00-$7F = 0 to +127  (positive, bit 7 = 0)
  $80-$FF = -128 to -1  (negative, bit 7 = 1)

$7F = +127  (the largest positive 8-bit signed number)
$01 = +1

$7F + $01 = $80 = -128  (!!!)

Both inputs are positive, but result is negative.
This is mathematically wrong for signed arithmetic.
The V flag captures this: V=1 means "the signed result is wrong."
```

The carry flag is NOT set here (`$7F + $01 = $80`, which fits in 8 bits, no bit 8 overflow). This demonstrates the difference:
- C flag = unsigned overflow (did the result exceed 255?)
- V flag = signed overflow (did the sign flip incorrectly?)

#### Register/Flag Trace

```
Step 1: CLC         C: ?->0
Step 2: LDA #$7F    A: $00->$7F   N=0  Z=0   ($7F = 0b01111111, bit 7 = 0)
Step 3: ADC #$01
  Computation: $7F + $01 + 0 = $80
  Binary:
    0111 1111   ($7F = +127)
  + 0000 0001   ($01 = +1)
  -----------
    1000 0000   ($80 = -128 in signed!)

  A: $7F->$80
  C=0  (no carry out from bit 7, unsigned result $80 fits in 8 bits)
  V=1  (positive + positive = negative: signed overflow!)
  N=1  (bit 7 of $80 = 1)
  Z=0  ($80 != 0)

Final state:    PC=$0405  A=$80  C=0  V=1  N=1  Z=0
```

---

### 6.9 TestRTI -- Return from Interrupt

#### What It Is

```go
func TestRTI(t *testing.T) {
    c, mem := setupCPU([]byte{0x40}, 0x0400)  // RTI at $0400
    // Manually set up stack: status, then PClo, PChi
    c.SP = 0xFD
    mem.Data[0x01FB] = FlagC | FlagU  // status byte (P)
    mem.Data[0x01FC] = 0x34           // PC low byte
    mem.Data[0x01FD] = 0x12           // PC high byte
    c.SP = 0xFA                       // SP points 3 bytes below top
    c.Step()
    if c.PC != 0x1234 { ... }
    if !c.getFlag(FlagC) { ... }
}
```

**Program bytes decoded:**

```
$0400:  40     RTI             Return from Interrupt (Implied mode)
```

**Stack pre-loaded by the test:**

```
Address   Value       What it is
$01FB:    FlagC|FlagU  Status register P to restore
$01FC:    $34          PC low byte
$01FD:    $12          PC high byte
SP = $FA  (3 bytes above SP are the RTI data)
```

#### Why It Matters

`RTI` (Return from Interrupt) is the partner to the `BRK` instruction and hardware interrupts. When an interrupt occurs, the 6502 pushes PC high, PC low, and P (status) onto the stack, then jumps to the interrupt handler. `RTI` reverses this: it pulls P, then PC low, then PC high, and jumps to that address.

Unlike `RTS` (which adds 1 to the pulled address), `RTI` does **NOT** add 1. The interrupt mechanism pushes the address of the instruction that was interrupted (or the next instruction after BRK), so RTI returns exactly to that address.

#### Stack Layout Before RTI

```
Stack before RTI (SP = $FA):

$01FF: [--]  (initial stack boundary)
$01FE: [--]
$01FD: [12]  <-- PC high byte  (to be pulled last)
$01FC: [34]  <-- PC low byte   (to be pulled second)
$01FB: [21]  <-- Status P = FlagC|FlagU  (to be pulled first)
           ^
           SP = $FA points here (below the 3 items)
           RTI will pull from $01FB, $01FC, $01FD

FlagC = 0x01  (Carry)
FlagU = 0x20  (Unused, always 1)
FlagC | FlagU = 0x21 = 0b00100001
```

#### RTI Pull Sequence

```
RTI execution:
  1. Pull P (status) from mem[$01FB] = $21 = FlagC|FlagU
     SP: $FA -> $FB
     P = $21  (Carry is now set)

  2. Pull PC low  from mem[$01FC] = $34
     SP: $FB -> $FC

  3. Pull PC high from mem[$01FD] = $12
     SP: $FC -> $FD

  4. PC = ($12 << 8) | $34 = $1234
     (NO +1 added, unlike RTS)

Final state:    PC=$1234  P=$21  SP=$FD  C=1
```

**Comparison RTI vs RTS:**

| | RTS | RTI |
|--|-----|-----|
| Pulls | PC only | P, then PC |
| PC adjustment | Adds 1 | No adjustment |
| Restores P | No | Yes |
| Used after | JSR | Interrupt handler |

---

### 6.10 TestZeroPageWrap -- Zero Page Address Wrapping

#### What It Is

```go
func TestZeroPageWrap(t *testing.T) {
    // LDA $FF,X with X=$01 should wrap to $00, not $100
    c, mem := setupCPU([]byte{0xB5, 0xFF}, 0x0400)
    c.X = 0x01
    mem.Data[0x00] = 0x77    // wrapped address
    mem.Data[0x0100] = 0x99  // non-wrapped (should NOT be read)
    c.Step()
    if c.A != 0x77 { ... }
}
```

**Program bytes decoded:**

```
$0400:  B5     LDA             Load Accumulator, Zero Page,X mode opcode
$0401:  FF     $FF,X           Zero page base address $FF
X register = $01 (set manually)
```

#### Why It Matters

Zero page indexed addressing (`LDA $FF,X`) adds X to the zero-page address. The addition **wraps within zero page** -- it never crosses into page 1. This is by design: zero page addressing uses an 8-bit address calculation, so the result is always masked to 8 bits.

```
$FF + $01 = $100  (9-bit result)

But zero page arithmetic: ($FF + $01) & $FF = $100 & $FF = $00

The effective address is $0000, NOT $0100.
```

This matters because page 1 is the stack. If zero page indexed addressing could accidentally read from page 1, then operations near address `$FF` would silently read from the stack -- a catastrophic bug. The wrapping keeps zero page access confined to page 0.

This behavior comes directly from the `resolve()` function in `cpu.go`. As documented in the CPU walkthrough, the zero-page indexed mode uses `uint8` arithmetic to force the wrap:

```go
// From cpu.go resolve():
addr := uint16(c.read(c.PC) + c.X)  // ZeroPageX -- wraps within zero page
//             ^^^^^^^^^^^^^^^^^^^
//             Addition in uint8 arithmetic forces wrap within 0x00-0xFF
```

#### Memory Diagram

```
Address   Value   Notes
$00:      [77]    <-- CORRECT: $FF + $01 wraps to $00
$FF:      [--]    Zero page address $FF (base)
$0100:    [99]    <-- WRONG: this is stack territory, must NOT be read

Calculation:
  Base:  $FF = 11111111 (8-bit)
  X:     $01 = 00000001 (8-bit)
  Sum:   $FF + $01 = $100 = 100000000 (9-bit)
  Wrap:  $100 & $00FF = $00 (truncate to 8 bits)

Effective address: $0000  →  A = mem[$0000] = $77  ✓

If wrap were NOT applied:
  Effective address would be: $0100
  A = mem[$0100] = $99  (WRONG! Reading from stack!)
```

#### Register/Flag Trace

```
Step 1: LDA $FF,X
  Base ZP address: $FF
  Add X: $FF + $01 = $100, wrapped to $00 (uint8 truncation)
  Effective address: $0000
  Value: mem[$0000] = $77
  A: $00->$77   N=0  Z=0

Final state:    PC=$0402  A=$77  X=$01
```

---

## Section 7: The Klaus Dormann Test

### What It Is

```go
const dormannBin = "testdata/6502_functional_test.bin"
const dormannStart   uint16 = 0x0400
const dormannSuccess uint16 = 0x3469
const maxDormannCycles = 100_000_000

func TestDormann(t *testing.T) {
    data, err := os.ReadFile(dormannBin)
    if err != nil {
        t.Skipf("Dormann test binary not found at %s ...", dormannBin)
    }
    // ... load, execute, check for trap at $3469 ...
}
```

The Klaus Dormann 6502 Functional Test is the gold-standard verification suite for 6502 emulators. It is a 65,536-byte binary that tests every official opcode, every addressing mode, decimal mode, edge cases, and interrupt handling. It was written by Klaus Dormann and is widely used by every serious 6502 emulator project.

### Why It Matters

The unit tests in Sections 5 and 6 test individual behaviors in isolation. Dormann tests everything together in a realistic execution environment. It exercises behaviors that are hard to think of in isolation: the interaction between flags and subsequent operations, the exact timing of page-cross cycles across all addressing modes, BCD mode edge cases across hundreds of operand combinations.

If your emulator passes Dormann, it is almost certainly compatible with real 6502 software. If it fails, the trap address tells you which test group failed, narrowing down the bug.

### How the Code Works

**Step 1: Load or skip**

```go
data, err := os.ReadFile(dormannBin)
if err != nil {
    t.Skipf("Dormann test binary not found at %s -- skipping.\n"+
        "Download it with:\n" + ..., dormannBin)
}
```

`t.Skipf` marks the test as skipped (not failed) if the binary is missing. This is correct behavior -- the test cannot run without the binary, but the absence of the binary is not a bug in the emulator. The test output will show `--- SKIP: TestDormann` rather than `--- FAIL`.

**Step 2: Load the full 64 KB binary**

```go
mem := &FlatMemory{}
copy(mem.Data[:], data)
```

Unlike `setupCPU`, this copies the entire 65,536-byte Dormann binary directly over all of memory. The binary is pre-linked to run at `$0000`-`$FFFF` with the program starting at `$0400`. It contains its own reset vector data.

**Step 3: Set PC directly (no reset vector)**

```go
c := New(mem)
c.PC = dormannStart  // $0400
```

Notice that `Reset()` is NOT called. The Dormann binary's internal reset vector might not be set up correctly for our use, so we bypass it and set PC directly to `$0400`.

**Step 4: Execute and watch for traps**

```go
prevPC := c.PC
for totalCycles < maxDormannCycles {
    cycles := c.Step()
    totalCycles += uint64(cycles)

    if c.PC == prevPC {
        // CPU is in a trap (JMP to self)
        if c.PC == dormannSuccess {
            t.Logf("PASS -- all tests passed at $%04X", c.PC)
            return
        }
        t.Fatalf("TRAP at $%04X -- test failed.", c.PC)
    }
    prevPC = c.PC
}
```

**Trap detection**: A "trap" is when the CPU executes `JMP $xxxx` where `$xxxx` is the current PC. The instruction jumps to itself, causing an infinite loop at the same address. `c.PC == prevPC` detects this.

The Dormann test is designed so that:
- **Success**: The program reaches `$3469` and executes `JMP $3469` -- it loops forever at the success address.
- **Failure**: If any test fails, the program executes `JMP` to the address of the failing test, trapping there. By looking up the trap address in the Dormann source, you can identify which specific test failed.

**Timeout**: `maxDormannCycles = 100_000_000` (100 million cycles) prevents infinite loops from hanging the test runner. If the CPU is still running after 100M cycles without hitting a trap, something is seriously wrong.

#### Dormann Test Groups (Reference)

When a test fails, the trap address identifies which group:

```
$0400   Start of all tests
$040B   Load/store tests (LDA, STA, LDX, STX, LDY, STY)
$0D00   AND, OR, EOR
$0EA0   ADC
$11C0   SBC
$157F   CMP, CPX, CPY
$18A7   BIT
$1C00   ROL, ROR, ASL, LSR
$28D8   JMP, JSR, RTS
$2A82   Branch tests
$2EB4   Stack tests
$3124   Interrupt tests (BRK, RTI)
$3469   SUCCESS -- all tests passed
```

### How to Download and Run

```
# 1. Create the testdata directory
mkdir -p cpu/testdata

# 2. Download the Dormann binary
curl -L -o cpu/testdata/6502_functional_test.bin \
  https://github.com/Klaus2m5/6502_65C02_functional_tests/raw/master/bin_files/6502_functional_test.bin

# 3. Run the test
go test ./cpu -v -run TestDormann
```

Expected output on success:
```
--- PASS: TestDormann (2.34s)
    cpu_test.go:407: PASS -- all tests passed at $3469 after 96241248 cycles
```

### Real-World Analogy

The final exam for your CPU. The unit tests are like pop quizzes on individual topics. Dormann is the comprehensive final that covers everything at once in a realistic setting. It is like a driving test that makes you perform every possible maneuver -- parallel parking, highway merging, U-turns, night driving, emergency stops. Pass it and you are road-legal. Fail it and the examiner tells you exactly which maneuver caused the failure (the trap address).

---

## Section 8: The Trace Helper (TestDormannTrace)

### What It Is

```go
func TestDormannTrace(t *testing.T) {
    if os.Getenv("TRACE") == "" {
        t.Skip("Set TRACE=1 to enable the Dormann trace test")
    }
    // ... load binary, execute up to 2000 instructions, print each one ...
    fmt.Printf("%04X  %02X    A:%02X X:%02X Y:%02X SP:%02X P:%02X\n",
        pc, op, c.A, c.X, c.Y, c.SP, c.P)
}
```

`TestDormannTrace` is a debugging tool that prints the CPU's state at each instruction step. It is not meant to be run as part of normal testing -- it is only activated when you set the `TRACE=1` environment variable.

### Why It Matters

When Dormann fails -- when the CPU traps at some address other than `$3469` -- you know there is a bug somewhere, but you do not know what went wrong leading up to the trap. The trace gives you a "black box recording" of the CPU's execution:

```
0400  F8    A:00 X:00 Y:00 SP:FD P:24
0401  18    A:00 X:00 Y:00 SP:FD P:2C
0402  A9    A:00 X:00 Y:00 SP:FD P:2C
0404  69    A:15 X:00 Y:00 SP:FD P:2C
0406  D0    A:42 X:00 Y:00 SP:FD P:2C
...
```

By comparing your trace output with a known-good 6502 trace (from a reference emulator or real hardware), you can find exactly which instruction first diverges. The line before the divergence is where the bug is.

### How the Code Works

**Line by line:**

```go
if os.Getenv("TRACE") == "" {
    t.Skip("Set TRACE=1 to enable the Dormann trace test")
}
```

`os.Getenv("TRACE")` reads the `TRACE` environment variable. If it is empty (not set), the test skips immediately. This prevents the trace from printing thousands of lines during normal `go test` runs. Only when you explicitly set `TRACE=1` will the trace activate.

```go
for i := 0; i < 2000; i++ {
    pc := c.PC
    op := mem.Data[pc]
    fmt.Printf("%04X  %02X    A:%02X X:%02X Y:%02X SP:%02X P:%02X\n",
        pc, op, c.A, c.X, c.Y, c.SP, c.P)
    c.Step()
    if c.PC == prevPC {
        fmt.Printf("TRAP at $%04X\n", c.PC)
        break
    }
    prevPC = c.PC
}
```

The loop prints **before** stepping, so you see the state the CPU is in **when it fetches each instruction**. The format shows:
- `%04X` -- 4-digit hex PC (e.g., `0400`)
- `%02X` -- 2-digit hex opcode byte at that PC (e.g., `A9`)
- Registers in hex: A, X, Y, SP, P

The loop runs at most 2000 iterations. For a failing test early in Dormann, this is enough to capture the divergence. For later tests, you might increase this limit.

**Trap detection**: Same as TestDormann -- if `c.PC == prevPC`, execution is trapped.

### How to Run the Trace

```bash
# Basic trace (first 2000 instructions)
TRACE=1 go test ./cpu -v -run TestDormannTrace

# Redirect output to a file for comparison
TRACE=1 go test ./cpu -v -run TestDormannTrace 2>&1 | head -100
```

### Real-World Analogy

An airplane's flight data recorder (the "black box"). During normal flight, nobody looks at it. But when something crashes, investigators pull the black box and replay every second of the flight to find the exact moment when something went wrong. `TestDormannTrace` is your CPU's black box -- you only activate it when you need to debug a crash, and it gives you a frame-by-frame replay of what happened.

---

## Section 9: Summary -- The Testing Strategy

The test suite in `cpu_test.go` exemplifies a layered testing strategy:

### Layer 1: Unit Tests (Sections 5.1-5.14)
Test each instruction's basic behavior in complete isolation. One instruction, one scenario, one or two assertions. These tests:
- Run in microseconds
- Are easy to write and read
- Catch simple implementation mistakes immediately
- Form the safety net for further development

### Layer 2: Edge Case Tests (Sections 6.1-6.10)
Test the tricky corner cases that are easy to get wrong:
- BCD arithmetic (Sections 6.1-6.3)
- Hardware bugs that must be faithfully replicated (Section 6.4)
- Performance-affecting page crossings (Section 6.5)
- Bit manipulation subtleties (Sections 6.6-6.7)
- Signed arithmetic semantics (Section 6.8)
- Interrupt handling (Section 6.9)
- Memory wrapping behavior (Section 6.10)

### Layer 3: Integration Test (Section 7)
The Dormann test exercises the entire CPU across thousands of instruction combinations. It catches interactions between instructions that unit tests cannot find -- for example, a flag set incorrectly by one instruction that causes the wrong branch decision three instructions later.

### The Test Pyramid

```
         /\
        /  \
       / 🏆 \     Integration: Dormann (1 test, ~96M cycles)
      /──────\
     /        \
    / ⚙⚙⚙⚙⚙ \   Edge cases: 10 tests
   /────────────\
  /              \
 / 📋📋📋📋📋📋📋 \  Unit tests: 14 tests
/──────────────────\
```

- **Bottom (wide)**: Many fast unit tests. Easy to write, cheap to run.
- **Middle**: Fewer edge case tests. Test complex interactions.
- **Top (narrow)**: One comprehensive integration test. Expensive but definitive.

### White-Box Testing

The test file uses `package cpu` (not `package cpu_test`). This gives tests direct access to internal struct fields (`c.A`, `c.X`, `c.SP`, `c.P`) and internal constants (`FlagC`, `FlagZ`, etc.) without exporting them.

**White-box** tests can verify internal state that is invisible from the outside. For an emulator, internal state IS the behavior -- the register values and flag states are the program-visible results of every instruction. It makes no sense to test these through an opaque public API.

This is in contrast to **black-box** testing (using `package cpu_test`) where tests can only call exported functions and observe exported results. For library code, black-box testing is preferred. For CPU emulators, white-box testing matches the nature of the thing being tested.

### What Makes a Good CPU Emulator Test?

Looking at the tests in this file, each good test has:

1. **A minimal program**: Only the bytes needed for the specific behavior. No extra setup.
2. **Direct state inspection**: Check registers and memory directly after execution.
3. **Specific assertions**: Not just "did it work?" but "is A = $42? Is Z clear? Is C set?"
4. **A clear failure message**: Every `t.Fatalf` includes the actual value (`got 0x%02X`) so failures are immediately diagnosable.
5. **One behavior per test**: `TestLDAZero` tests the Z flag on LDA. `TestLDANegative` tests the N flag. Separate tests are easier to diagnose when they fail.

The combination of these properties -- minimal programs, specific state checks, clear messages, and layered coverage -- forms a test suite that catches bugs quickly and makes them easy to fix.

---

## Appendix: Quick Reference -- Opcodes Used in Tests

| Hex | Mnemonic | Mode | Description |
|-----|----------|------|-------------|
| $18 | CLC | Implied | Clear Carry Flag |
| $20 | JSR | Absolute | Jump to Subroutine |
| $24 | BIT | Zero Page | Bit Test |
| $2A | ROL A | Accumulator | Rotate Left through Carry |
| $38 | SEC | Implied | Set Carry Flag |
| $40 | RTI | Implied | Return from Interrupt |
| $48 | PHA | Implied | Push Accumulator |
| $4C | JMP | Absolute | Jump to Address |
| $60 | RTS | Implied | Return from Subroutine |
| $68 | PLA | Implied | Pull Accumulator |
| $69 | ADC | Immediate | Add with Carry |
| $6C | JMP | Indirect | Jump Indirect (pointer) |
| $85 | STA | Zero Page | Store Accumulator |
| $A0 | LDY | Immediate | Load Y Register |
| $A1 | LDA | (Indirect,X) | Load Accumulator, Indexed Indirect |
| $A2 | LDX | Immediate | Load X Register |
| $A9 | LDA | Immediate | Load Accumulator |
| $B1 | LDA | (Indirect),Y | Load Accumulator, Indirect Indexed |
| $B5 | LDA | Zero Page,X | Load Accumulator, ZP Indexed |
| $BD | LDA | Absolute,X | Load Accumulator, Absolute Indexed |
| $E9 | SBC | Immediate | Subtract with Carry |
| $EA | NOP | Implied | No Operation |
| $F0 | BEQ | Relative | Branch if Equal (Z=1) |
| $F8 | SED | Implied | Set Decimal Flag |

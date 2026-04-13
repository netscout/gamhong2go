# Educational Walkthrough: `bus/bus.go` -- The Apple II Address Decoder

> This document walks through every section of `bus/bus.go` for a reader who has just finished the memory walkthrough. It explains how the bus routes CPU reads and writes to the right device, why "last match wins" is the whole point, and how the `Bus` type finally closes the loop opened at the end of the memory walkthrough. Each code section follows the familiar What It Is / Why It Matters / How the Code Works / Real-World Analogy pattern.

---

## Section 0: Background -- What Is an Address Decoder?

On the real Apple II motherboard, the 6502 CPU exposes 16 address lines (A0-A15) and 8 data lines (D0-D7). When the CPU wants to read from address `$D100` (53,504 decimal), it puts the bit pattern for `$D100` on the address lines, asserts its R/W line to "read," and waits for some chip on the board to drive the data lines with a byte value.

"Some chip" is the key phrase. The motherboard contains multiple chips -- RAM chips, ROM chips, the peripheral interface adapter (PIA), the video circuitry, and expansion slot hardware. **Only one** of them must respond on any given bus cycle, or multiple chips would fight to drive the data lines simultaneously, which causes corruption. The hardware component that decides which chip responds to which address is called the **address decoder**.

On real 6502 motherboards, decoding was done with a handful of 74LSxx TTL logic chips -- typically a 74138-style 3-to-8 decoder combined with some NAND gates, wired to the upper address lines A12-A15 to generate chip-select signals. That was the 1977 solution.

Our emulator does not simulate TTL gates. Instead, we replace the entire decoder with a single 68-line Go file: `bus/bus.go`. The `Bus` type is the software analog of that silicon.

Here is where the bus fits in the cross-layer architecture of the emulator:

```text
+------+
| CPU  |  only sees cpu.Memory (Read, Write)
+------+
   |
   v
+------+
| Bus  |  satisfies cpu.Memory; owns a slice of bus.Device values
+------+
   |
   +-----------------+-----------------+-----------------+
   v                 v                 v                 v
+--------+      +--------+        +--------+       +---------+
|  RAM   |      |  ROM   |        |  I/O   |  ...  |  future |
| $0000- |      | $D000- |        | $C000- |       | devices |
| $FFFF  |      | $FFFF  |        | $CFFF  |       |         |
+--------+      +--------+        +--------+       +---------+
```

The CPU only ever sees one thing: the `cpu.Memory` interface (two methods: `Read` and `Write`). It has no idea whether the thing on the other end is a single flat array, a ROM chip, or a routing table with ten entries. The bus satisfies `cpu.Memory`, so the CPU is happy. From there, the bus fans out to multiple `bus.Device` values -- each device also exposing the same two methods.

The memory walkthrough ended its Section 0 with a forward pointer: "How the Bus Stacks These." This is the walkthrough that section promised.

---

## Section 1: The `Device` Interface

### What It Is

```go
// Device is anything that can be mapped into the address space.
type Device interface {
    Read(addr uint16) uint8
    Write(addr uint16, val uint8)
}
```

Two methods, the same signatures you already saw on `cpu.Memory` in Section 3 of the CPU walkthrough (The Memory Interface).

### Why It Matters

This is the **contract** between the bus and everything that wants to live inside the 64 KB address space. A type is "pluggable into the bus" if and only if it has these two methods with these exact signatures.

It is structurally identical to `cpu.Memory`. Because Go uses **structural typing** (duck typing by method set), the same concrete value -- `*memory.RAM`, `*memory.ROM`, a future `*io.Softswitches` -- automatically satisfies both interfaces without any explicit declaration. One type, two interfaces, zero boilerplate.

This means the bus is a **cpu.Memory** when the CPU looks at it, and it **owns** a list of `bus.Device` values when its own internals look downward. The two roles share the same method shape. The bus is simultaneously consumer (of devices) and provider (to the CPU) of the same two-method vocabulary.

A natural question: why define `bus.Device` at all if it is shape-for-shape identical to `cpu.Memory`? The answer is **package boundaries**. The `bus` package cannot import `cpu` without creating a coupling problem -- the CPU already depends on its own `Memory` interface, and creating a dependency in the other direction would tie the packages together in a tangle. By declaring `bus.Device` locally, the `bus` package stays decoupled from `cpu`. Go's structural typing makes this cost-free: the same `*memory.RAM` satisfies both interfaces with zero extra code.

This is the **Dependency Inversion Principle** applied at the package level, not just the type level. Section 3 of the CPU walkthrough (The Memory Interface) covers Dependency Inversion in detail; we are seeing that same principle at work here, one layer up.

### How the Code Works

The method set (`Read(uint16) uint8`, `Write(uint16, uint8)`) is the entire vocabulary a device needs to speak. Everything else -- knowing its own base address, its own size, whether its writes are destructive -- is the device's private business. The bus only asks for these two verbs.

The interface is defined at the top of the file, before `mapping` or `Bus`. This is intentional: the interface is the public contract; the struct types below it are implementation details.

### Real-World Analogy

A postal sorting facility (the bus) accepts mail from the sender (the CPU) and forwards it to one of many tenants in an apartment building (the devices). The sorting facility does not care what tenants do with their mail -- one tenant throws junk mail away (ROM), another keeps every letter in a scrapbook (RAM), a third has a gadget that lights up when an envelope arrives (an I/O device). The sorter only requires that each tenant has a mailbox slot (`Read`) and a pickup point (`Write`). The mailbox shape is the interface; what happens behind it is the tenant's business.

---

## Section 2: The `mapping` Struct

### What It Is

```go
// mapping ties a Device to a contiguous address range.
type mapping struct {
    start  uint16
    end    uint16 // inclusive
    device Device
}
```

Three fields. A plain data carrier pairing an address range with the device that owns that range.

### Why It Matters

Every row in the bus's routing table is one `mapping`. The bus itself is nothing more than a slice of these.

**`end` is inclusive.** The range `$D000` (53,248) to `$FFFF` (65,535) includes both endpoints. This matches how humans and hardware datasheets describe address ranges: "the monitor ROM lives from `$F800` to `$FFFF`" means both of those addresses are part of the ROM. An exclusive upper bound would force every reader to track an off-by-one adjustment. There is also a practical boundary case: the full 64 KB range is `$0000` to `$FFFF`. With inclusive ends, that fits perfectly in two `uint16` values (`start=0, end=0xFFFF`). With an exclusive upper bound you would need `end=0x10000`, which does not fit in a `uint16`, forcing either a wider field or a special-case. Inclusive ends sidestep the issue entirely.

The struct is **unexported** (`mapping`, lowercase). This is a private implementation detail of the bus. Callers never construct a `mapping` directly -- they call `Map()`, which builds one internally.

### How the Code Works

`device Device` holds an interface value. Interface values in Go carry a two-word header (type descriptor + pointer to the concrete value). The `mapping` does not own the device -- it holds a reference to it. Deleting a mapping would not free the underlying `RAM` or `ROM` object.

The comment `// inclusive` on the `end` field is load-bearing documentation. Without it, a future reader might assume exclusive bounds and introduce an off-by-one error in a range check.

### Real-World Analogy

A mapping is a single row in a telephone directory: "Smith, John -- 555-1234." The row does not own John Smith; it just records where to reach him. Deleting the row does not delete the person. Adding a second row "Smith, J. Jr. -- 555-1234" creates a collision that some lookup rule (last entry wins, first entry wins) must resolve -- which is exactly what `Read` and `Write` do.

---

## Section 3: The `Bus` Struct and `NewBus()`

### What It Is

```go
// Bus is the Apple II address decoder. It implements cpu.Memory and routes
// every read/write to the device that owns that address range.
type Bus struct {
    mappings []mapping
}

// New returns an empty bus. Attach devices with Map() before use.
func NewBus() *Bus {
    return &Bus{}
}
```

A struct with one field -- a slice of mappings -- and a constructor that returns an empty one.

### Why It Matters

A **slice** (not a fixed-size array) means the bus has no baked-in limit on how many devices can register. Three devices? Fine. Thirty? Also fine. The slice grows as `Map` is called.

`NewBus()` returns a bus with **zero** mappings. Out of the box, a bus routes nothing: every `Read` returns the open-bus fallback (`$FF`), and every `Write` silently disappears. This is a deliberate "blank motherboard" model. If the constructor pre-wired a default Apple II memory map, the `bus` package would become Apple-II-specific and could not be reused for any other machine.

The job of "know the Apple II memory map" belongs in `main.go`, not in the bus package. Keeping that concern outside `bus` follows the Single Responsibility Principle: the bus package is responsible for routing, not for policy about which chips a particular machine contains.

### How the Code Works

The `mappings` field is **unexported**. Callers outside the `bus` package cannot read or modify the slice directly. The only way to grow the routing table is through `Map`.

`NewBus()` returns `&Bus{}`. The slice field is left as `nil` (Go's zero value for a slice). Appending to a `nil` slice is safe -- the first `append` call allocates backing storage automatically. No explicit `make` is needed.

The return type is a **pointer** (`*Bus`). This matters because `Map`, `Read`, `Write`, and `Dump` all have pointer receivers, and because every caller expects to share a single bus value. Returning a pointer ensures all parts of the program hold a reference to the same underlying `mappings` slice.

### Real-World Analogy

`NewBus()` is handing you a blank breadboard. There are no chips on it yet. Power rails exist but no wire goes anywhere. You are the technician who decides what to plug in and in what order. The bus itself has no opinion about whether this breadboard will become an Apple II, an Atari 2600, or a debugging testbed.

---

## Section 4: `Map()` -- Registering Devices

### What It Is

```go
// Map registers a device for the address range [start, end] (inclusive).
// Later mappings take priority over earlier ones for overlapping ranges,
// so you can map RAM for the full range first, then overlay ROM on top.
func (b *Bus) Map(start, end uint16, dev Device) {
    b.mappings = append(b.mappings, mapping{start, end, dev})
}
```

A single-line body: append a new mapping to the slice.

### Why It Matters

**`Map` is where the Apple II memory map is born.** Every `Map` call is the software equivalent of soldering a chip onto the motherboard and wiring its chip-select signal to the address decoder.

**Ordering matters.** The comment says it plainly: "Later mappings take priority over earlier ones." `Read` and `Write` both walk the slice backwards, so the last registered mapping that contains an address wins. This is called the **overlay pattern**, and it is the core idea that makes the entire design work. The Apple II memory map is constructed with a short sequence of `Map` calls, bottom layer first:

```text
bus.Map($0000, $FFFF, ram)    // #1  RAM spans the entire 64 KB
bus.Map($C000, $CFFF, io)     // #2  I/O region overlaid on top
bus.Map($D000, $FFFF, rom)    // #3  ROM overlaid above the I/O region
```

After those three calls, the routing table looks like this:

```text
index  range              device
-----  -----------------  ------------------
  0    $0000..$FFFF       *memory.RAM     (0 to 65535; full address space)
  1    $C000..$CFFF       *io.Softswitches (49152 to 53247)
  2    $D000..$FFFF       *memory.ROM     (53248 to 65535)
```

The priority rule then produces the final memory map:

- `$D000`-`$FFFF` (53,248-65,535): ROM wins (index 2, checked first in backward walk).
- `$C000`-`$CFFF` (49,152-53,247): I/O wins (index 1).
- `$0000`-`$BFFF` (0-49,151): RAM wins (index 0, checked last).

This is the stacking behaviour the memory walkthrough described in its "How the Bus Stacks These (Forward Reference)" subsection. That forward reference is now closed.

### How the Code Works

One line: `b.mappings = append(b.mappings, mapping{start, end, dev})`. `append` returns a (possibly reallocated) slice, which is reassigned to `b.mappings`. This is idiomatic Go -- the pattern also works correctly the first time `Map` is called on an empty (nil) slice.

The operation is **O(1) amortized**: slice append occasionally triggers a reallocation when the backing array grows, but for a handful of mappings this cost is negligible.

There is **no validation, no overlap error, no sort**. These omissions are deliberate. Overlap is the whole point -- reporting overlap as an error would forbid the exact pattern the design requires. Ordering responsibility belongs with the caller (`main.go`), the only place that knows the intended memory layout.

Notice that `Map` does not check whether `start <= end`. If you pass `Map($FFFF, $0000, dev)` by mistake with `start > end`, the range check `addr >= $FFFF && addr <= $0000` requires `addr` to be simultaneously the largest and smallest 16-bit value -- an impossible condition. The device silently receives no traffic. This is a programmer error that the test suite would catch.

### Real-World Analogy

Think of a stack of tracing-paper overlays placed on top of a blueprint. The blueprint at the bottom is the RAM layer, covering the whole page. You drop a tracing-paper overlay for I/O on top of it, then another for ROM. To find "what is at coordinate X?" you look down from the top and report whichever overlay covers X. The blueprint (RAM) is only visible where no overlay sits above it.

---

## Section 5: `Read()` -- The Priority Overlay in Action

### What It Is

```go
// Read finds the last-registered device whose range contains addr.
func (b *Bus) Read(addr uint16) uint8 {
    // Walk backwards so later mappings (higher priority) win.
    for i := len(b.mappings) - 1; i >= 0; i-- {
        m := &b.mappings[i]
        if addr >= m.start && addr <= m.end {
            return m.device.Read(addr)
        }
    }
    return 0xFF // open bus
}
```

Ten lines. The entire address-decoding algorithm for reads lives here.

### Why It Matters

Every CPU memory read funnels through this function. Ten lines decide who answers the door for every single byte the CPU ever fetches -- opcodes, operands, stack values, zero-page variables, the reset vector at `$FFFC` (65,532).

The **backward walk** implements the "last match wins" rule. Without it, the ROM overlay would be invisible: the bus would always find the full-range RAM mapping first (at index 0) and return RAM's contents even for addresses inside the ROM range.

The fallback `return 0xFF` implements **open-bus behaviour**: if no registered device covers the requested address, pretend the data bus floated high (all bits set to 1). The memory walkthrough's ROM section ("open-bus discussion") explains the hardware origin: when no chip drives the data lines, pull-up resistors hold them at logic 1, and `0xFF` is the byte you get. That document also notes the honest caveat: on the real Apple II, open-bus reads can reflect video DMA residue rather than a clean `0xFF`. Returning `0xFF` is a common, deterministic emulator simplification.

### How the Code Works

Line by line:

- `for i := len(b.mappings) - 1; i >= 0; i--` -- the classic Go backward-iteration idiom. When the slice is empty, `len(b.mappings) - 1` is `-1`, and the condition `i >= 0` is immediately false. The loop body never runs. The empty-bus case falls through naturally to `return 0xFF`.
- `m := &b.mappings[i]` -- take the address of the slice element rather than copying the struct on every iteration. This is safe because `Read` never calls `append`, so the backing array cannot be reallocated while the loop is running. Taking a pointer into a slice element is only dangerous when something else might grow the slice during the loop's lifetime; here, nothing can.
- `if addr >= m.start && addr <= m.end` -- inclusive bound check on both ends. These are `uint16` comparisons; there are no signedness surprises.
- `return m.device.Read(addr)` -- dispatch the call. Note that **the original, unmodified address** is passed to the device. The bus does not translate `addr` to a device-local offset. Each device does its own offset arithmetic using its own base address. The ROM section of the memory walkthrough shows exactly this: `ROM.Read` subtracts `Base` to compute an index into its slice. The bus stays ignorant of that detail.
- `return 0xFF // open bus` -- the fallback after the loop exits.

**Why pass the full address instead of an offset?** Because it keeps the bus stateless and dumb. If the bus had to subtract each device's base address, it would duplicate information the device already has. RAM treats `addr` as a direct array index into its 64 KB backing store (as shown in the RAM section of the memory walkthrough). ROM subtracts its `Base`. The bus only cares about one question: "does this device's range cover this address?"

**Performance note:** the loop is O(n) per read, where n is the number of mapped devices. For a typical Apple II emulator with fewer than ten devices, this is essentially free: the entire `mappings` slice fits in a single CPU cache line, the backward walk is cleanly branch-predicted, and each iteration costs two `uint16` comparisons. Production emulators sometimes precompute a 256-entry page table (one entry per 256-byte page) for O(1) lookup. This emulator does not need that optimization; adding it now would obscure the routing logic without measurable benefit.

### Detailed Execution Traces

All three traces use the three-mapping setup from Section 4: RAM covering `$0000`-`$FFFF`, I/O at `$C000`-`$CFFF`, ROM at `$D000`-`$FFFF`.

**Trace 1: `bus.Read($D042)` -- ROM wins**

```text
bus.Read($D042)        ; $D042 = 53,314

mappings slice (len=3):
  [0] RAM  $0000..$FFFF  -> *memory.RAM
  [1] I/O  $C000..$CFFF  -> *io.Softswitches
  [2] ROM  $D000..$FFFF  -> *memory.ROM

loop i=2: m = mappings[2]  (ROM, $D000..$FFFF)
          $D042 >= $D000?  yes
          $D042 <= $FFFF?  yes
          MATCH -> return rom.Read($D042)
          (loop exits immediately via return)

Result: the byte that memory.ROM returns for $D042.
        RAM is shadowed; its $D042 contents are never observed.
```

**Trace 2: `bus.Read($0042)` -- RAM wins**

```text
bus.Read($0042)        ; $0042 = 66

loop i=2: m = mappings[2]  (ROM, $D000..$FFFF)
          $0042 >= $D000?  no
          skip

loop i=1: m = mappings[1]  (I/O, $C000..$CFFF)
          $0042 >= $C000?  no
          skip

loop i=0: m = mappings[0]  (RAM, $0000..$FFFF)
          $0042 >= $0000?  yes
          $0042 <= $FFFF?  yes
          MATCH -> return ram.Read($0042)

Result: the byte RAM last stored at $0042 (zero if never written).
```

**Trace 3: `bus.Read($1234)` on an empty bus -- open-bus fallback**

```text
bus.Read($1234)        ; $1234 = 4,660

mappings slice (len=0):
  (empty)

loop: i starts at -1, condition i >= 0 is false, body never runs

fall through -> return 0xFF

Result: $FF (open bus). No device was consulted.
```

### Real-World Analogy

A hotel front desk keeps a stack of room-assignment memos with the newest memo on top. When a guest asks "which room is John Smith in?", the concierge flips through from the top down, stopping at the first memo that names John Smith. Older, contradicting memos underneath are ignored. If no memo names John Smith at all, the concierge shrugs and says "nobody by that name" -- that shrug is the `$FF` open-bus response.

---

## Section 6: `Write()` -- Symmetric Routing

### What It Is

```go
// Write finds the last-registered device whose range contains addr.
// ROM devices simply ignore writes internally.
func (b *Bus) Write(addr uint16, val uint8) {
    for i := len(b.mappings) - 1; i >= 0; i-- {
        m := &b.mappings[i]
        if addr >= m.start && addr <= m.end {
            m.device.Write(addr, val)
            return
        }
    }
}
```

Same backward loop, same range check. The differences: it accepts a `val` argument, calls `Write` on the matching device, and silently falls through when no device matches (no fallback value, since there is nothing to return).

### Why It Matters

Write routing uses the **same priority order** as reads, so the memory map behaves consistently: if a read at `$D042` is answered by ROM, a write to `$D042` also goes to ROM -- and is silently discarded inside ROM.

Writes to unmapped addresses are **silently dropped**. This is correct hardware behaviour: on the real Apple II, writing to an address that no chip responds to simply evaporated into the bus. There is no "write failed" signal on a 6502 bus cycle.

The ROM case is important. The "write is a no-op" subsection of the ROM section in the memory walkthrough covers this: `ROM.Write` has an empty body. The byte vanishes **inside ROM**, not inside the bus. The bus does not second-guess devices -- it trusts each device to handle writes however it sees fit. A RAM device stores the byte; a ROM device discards it; a softswitch device might toggle internal state and ignore `val` entirely. All three look the same to the bus.

### How the Code Works

- Backward loop, same idiom as `Read`.
- `m.device.Write(addr, val); return` -- after dispatching, immediately return. Without the `return`, the loop would continue walking and deliver the same write to every earlier overlapping mapping. The `return` enforces the rule: exactly one device receives each write.
- **No fallback constant.** `Write` returns nothing. On an empty bus or an unmapped address, the function exits quietly. The write becomes a no-op.

### Execution Traces

**Trace 1: `bus.Write($D042, $AB)` -- write into ROM, silently discarded**

```text
bus.Write($D042, $AB)   ; $D042 = 53,314, value $AB = 171

loop i=2: ROM $D000..$FFFF contains $D042? YES
          -> rom.Write($D042, $AB)
          -> (ROM.Write body is empty; byte is discarded)
          -> return

Result: nothing stored anywhere. RAM at $D042 is NOT touched,
        because the backward walk returned before reaching index 0.
```

**Trace 2: `bus.Write($0042, $AB)` -- write into RAM**

```text
bus.Write($0042, $AB)   ; $0042 = 66, value $AB = 171

loop i=2: ROM $D000..$FFFF contains $0042? no
loop i=1: I/O $C000..$CFFF contains $0042? no
loop i=0: RAM $0000..$FFFF contains $0042? YES
          -> ram.Write($0042, $AB)
          -> RAM stores $AB into its backing array at index $0042
          -> return

Result: ram.Data[$0042] == $AB.
```

### Real-World Analogy

Reuse the sorting-facility picture: an envelope addressed to `$D042` (53,314) is routed to the ROM tenant, who has a standing rule of "throw all incoming mail in the shredder." The sorter does not know or care. Writes that match no tenant at all are dropped into a trash can at the end of the hallway -- no forwarding address, no error report, no receipt.

---

## Section 7: `Dump()` -- The Debugging Helper

### What It Is

```go
// Dump prints the current device map for debugging.
func (b *Bus) Dump() {
    for _, m := range b.mappings {
        fmt.Printf("  $%04X-$%04X  %T\n", m.start, m.end, m.device)
    }
}
```

A forward loop over all mappings that prints each address range and the concrete Go type of the device assigned to it.

(Note: the actual source file uses a Unicode en-dash character between the two addresses in the format string -- that is a cosmetic choice for readable console output. In the diagrams throughout this walkthrough we use plain ASCII hyphens, in keeping with the ASCII-only convention.)

### Why It Matters

During development, a wrong answer from the emulator is often caused by a wrong memory map -- for example, forgetting to overlay the ROM, or registering the I/O region in the wrong order so RAM wins instead. `Dump` lets you print the current routing table at any point and eyeball it for mistakes before running a single instruction.

Iteration order here is **forward** (index 0 upward), not backward. This matches the registration order, which is the most natural mental model for a developer reading the output. The "last entry wins" priority rule is easy to apply by eye when the list is printed in the order in which `Map` was called.

### How the Code Works

- `%04X` formats a `uint16` as uppercase hexadecimal, zero-padded to at least four characters. `$0042` prints as `0042`, not `42`.
- `%T` prints the concrete Go type of an interface value. For a `*memory.RAM`, it prints `*memory.RAM`; for a `*memory.ROM`, `*memory.ROM`. When you see two overlapping ranges, you can immediately tell which type wins which region -- far more useful than printing just the address range.

Sample output for the standard three-device setup (using ASCII hyphens in this document):

```text
  $0000-$FFFF  *memory.RAM
  $C000-$CFFF  *io.Softswitches
  $D000-$FFFF  *memory.ROM
```

Reading this top-to-bottom (registration order), the last entry that covers any address wins. `$D042` is covered by both `$0000-$FFFF` (index 0) and `$D000-$FFFF` (index 2), but index 2 is last, so ROM wins. This output makes that clear at a glance.

### Real-World Analogy

`Dump` is like asking a library to print its current shelving plan: "Reference runs from call number A to Z; the reserve shelf shadows K through M." You can read the shelving plan in five seconds and spot a misplaced section before opening any books.

---

## Section 8: Putting It Together -- The Bus as the Motherboard's Address Decoder

Now that all five functions are understood, step back and see the whole picture.

The complete Apple II memory layout, as discussed in Section 2.5 of the CPU walkthrough (The 6502 Memory Map), maps to three bus entries:

```text
Address range         Decimal           Wins via          Device type
--------------------  ----------------  ----------------  -----------------
$0000 - $BFFF         0 to 49151        index 0 (RAM)     *memory.RAM
$C000 - $CFFF         49152 to 53247    index 1 (I/O)     *io.Softswitches
$D000 - $FFFF         53248 to 65535    index 2 (ROM)     *memory.ROM
```

Here is the full routing picture with the mapping slice annotated:

```text
+------+
| CPU  |  only sees cpu.Memory (Read, Write)
+------+
   |
   v
+-----Bus-----+
|  mappings:  |
|  [0] RAM    |  $0000-$FFFF (bottom layer)
|  [1] I/O    |  $C000-$CFFF (middle overlay)
|  [2] ROM    |  $D000-$FFFF (top overlay)
+-------------+
   |
   +------------------+------------------+
   v                  v                  v
+--------+        +--------+        +--------+
|  RAM   |        |  I/O   |        |  ROM   |
| $0000- |        | $C000- |        | $D000- |
| $FFFF  |        | $CFFF  |        | $FFFF  |
+--------+        +--------+        +--------+
 index 0           index 1           index 2
 (bottom)                            (top)
```

**Four-read summary block** showing how the backward walk resolves each address:

```text
Read at $D042 (53,314):
  i=2: ROM ($D000-$FFFF) contains $D042? YES -> read from ROM, done.
       (RAM at index 0 is shadowed by ROM overlay.)

Read at $C080 (49,280):
  i=2: ROM ($D000-$FFFF) contains $C080? NO
  i=1: I/O ($C000-$CFFF) contains $C080? YES -> read from I/O, done.
       (RAM is shadowed by I/O overlay.)

Read at $0042 (66):
  i=2: ROM  no
  i=1: I/O  no
  i=0: RAM  yes -> read from RAM, done.

Read at $FFFF (65,535): same range as $D042 -- ROM wins.
```

**Dependency Inversion revisited.** The CPU file was written without knowing the bus existed: it only depends on `cpu.Memory`. The bus file is written without knowing what devices exist: it only depends on `bus.Device`. The memory package is written without knowing how it will be routed: its types satisfy whichever interfaces match their method sets. Each layer depends on an abstraction, not a concrete neighbour. Adding a new device -- a keyboard controller, a disk card -- requires writing one new type and adding one `Map` call. Zero lines of bus code change.

This mirrors the "Elegance of the Design" discussion in Section 4 of the memory walkthrough, but now we have the actual routing code in front of us to prove it works.

---

## Section 9: Design Decisions and Hardware Reality

#### Why "last match wins" instead of reporting an overlap error?

Layered overlays are the whole point. Apple II hardware used exactly this idea at the silicon level -- ROM chips selected by the upper-address decoder sat "on top of" the RAM that nominally occupied the same range. Reporting an overlap as an error would forbid the exact pattern the design requires. Making later registrations win gives callers a simple, sequential recipe: map the bottom layer first, then map overlays in priority order. The result is a routing table that reads like a diff: each new `Map` call says "from here on, this region is handled by this device."

#### Why inclusive `end` instead of exclusive?

Every hardware datasheet in history describes address ranges inclusively. Matching the domain language reduces off-by-one bugs. The practical boundary case also favours inclusive ends: the full 64 KB address space runs from `$0000` (0) to `$FFFF` (65,535). With inclusive ends, `start=0` and `end=0xFFFF` represent this perfectly in two `uint16` values. With an exclusive upper bound, you would need `end=0x10000`, which is 65,536 -- one more than the maximum value of a `uint16`. That would force either a `uint32` field or a special-case in the range check. Inclusive ends sidestep the problem entirely.

#### Why does the bus own no memory itself?

It is a pure router, like a mailroom that forwards envelopes without opening them. Owning no bytes means no duplicated storage, no "is the answer stored in the bus or in the device?" ambiguity, and no second location where a bug can quietly corrupt data. The bus is stateless with respect to content: its only state is the small `mappings` slice, which describes topology (who owns what range), not the bytes at those addresses.

#### Why a slice of mappings instead of a 256-entry page table?

Simplicity and generality. A slice works for any range granularity, including sub-page ranges like `$C000`-`$CFFF` (49,152-53,247) for I/O. A 256-entry page table -- one entry per 256-byte page -- would make lookup O(1) at the cost of more complex setup code and a table that occupies at least one separate cache line. For a machine with fewer than ten mappings, the current design is cache-friendly, simpler to read, and measurably fast enough. The right time to add a page-table optimization is when a profiler says so -- not now.

#### Why does `NewBus()` return an empty bus instead of a pre-wired "Apple II" bus?

Separation of concerns. Wiring the memory map is a policy decision that depends on which machine you are emulating, which ROM image was loaded, and whether the user plugged in a language card. That policy belongs in `main.go`, the top-level assembly point. Keeping the `bus` package **policy-free** means it is reusable: you could use the exact same `Bus` type to build a Commodore 64 emulator by changing only the wiring in `main.go`. A constructor that pre-wired an Apple II memory map would couple the bus package to one specific machine, making it harder to reason about and impossible to reuse.

#### How does this relate to the real Apple II's 74LSxx glue logic?

On the real motherboard, a handful of TTL decoder chips -- a 74138-style 3-to-8 decoder combined with NAND gates -- were wired to the upper address lines A12-A15. Each output of the decoder became a chip-select signal for a particular address range. Our `Map` calls are the software analog of wiring those decoder outputs to the right chips. The analogy is not perfect -- real hardware decoders fire all chip-select signals simultaneously (in parallel) while our code walks a list serially -- but the **effect** is identical: each address is routed to exactly one device, and the mapping from addresses to chips is expressed once and reused on every bus cycle.

---

## Section 10: Summary and What's Next

The bus is a tiny address decoder built from a slice of `(start, end, device)` mappings. Reads and writes walk the slice backwards so that later mappings overlay earlier ones -- "last match wins." Unmatched reads return `$FF` (the open-bus convention); unmatched writes silently disappear. Devices are anything that implements two methods, `Read` and `Write`, the same shape as `cpu.Memory`. The bus itself satisfies `cpu.Memory`, so from the CPU's perspective the bus is "just memory." Three types (`Device`, `mapping`, `Bus`), five functions (`New`, `Map`, `Read`, `Write`, `Dump`), 68 lines -- that is the complete address decoder.

> **Take-home idea:** The bus is where software discovers that "the CPU talks to memory" is actually "the CPU talks to a dispatch table of devices, and one of them happens to be called RAM."

### `bus.Bus` at a Glance

| Item             | Value                                                              |
|------------------|--------------------------------------------------------------------|
| Backing store    | `[]mapping` (slice, grown by `Map` calls)                          |
| Constructor      | `bus.NewBus()`                                                        |
| Register device  | `Map(start, end uint16, dev Device)`                               |
| Read cost        | O(n) backward walk; n = number of mappings (under 10 in practice)  |
| Unmatched read   | Returns `$FF` (open bus)                                           |
| Unmatched write  | Silently dropped                                                   |
| Satisfies        | `cpu.Memory`                                                       |
| Accepts          | Any `bus.Device` (Read + Write method set)                         |

### What Comes Next

The next walkthrough covers `io/softswitches.go`, where we will see an entirely new kind of device -- one whose `Read` and `Write` methods have **side effects** beyond storing or returning bytes. Reading address `$C030` (49,200 decimal), for example, produces a click on the Apple II speaker. The bus routing you just learned is what delivers those reads to the softswitch object in the first place.

After that, the wiring in `main.go` will tie RAM, ROM, the bus, and the softswitches together into a runnable machine.

The language card, which lets Apple II programs switch in a second bank of RAM above `$C000` (49,152), is one of the things a future walkthrough will add -- mechanically it is just another set of `Map` calls swapped in at runtime, but that is a later story.

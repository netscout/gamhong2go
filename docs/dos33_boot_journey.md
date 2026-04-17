# The DOS 3.3 Boot Journey: A Debugging Detective Story

*An illustrated post-mortem of a hard-won one-line fix*

---

## TL;DR

DOS 3.3 would not boot on our Apple II+ emulator. Instead of the friendly `]` prompt, users saw the bare `*` monitor prompt. After four sessions chasing increasingly exotic hypotheses — GCR XOR chains, RAM power-up patterns, stack pointer semantics, pre-execution hooks — the real bug turned out to be stunningly small: our `dosInterleave` table in `disk/gcr.go` was *inverted*. It contained the physical-to-logical mapping, but the code treated it as logical-to-physical. Fixing the table values (and doing nothing else) made DOS boot cleanly. Everything else we "fixed" during the hunt was scaffolding — useful, but not the bug.

---

## The Symptom

The user ran the emulator with a standard DOS 3.3 disk image and expected this:

```
APPLE ][
DOS VERSION 3.3  SYSTEM MASTER
]
```

Instead they got:

```
APPLE ][

*
```

That lone `*` is the Apple II *monitor prompt* — the machine-language debugger baked into the ROM. It appears when the machine hits a `BRK` instruction or falls into an undefined state. Seeing it after a boot attempt means: "something in the boot sequence crashed into unmapped territory, and the ROM caught the fall."

Getting to the monitor does *not* mean "nothing happened." It means boot almost worked. It got far enough to load code, run it, and then execute some instruction that no longer made sense. That narrow window between "runs" and "makes sense" is where this bug lived for 4 sessions.

---

## Background — Just Enough Apple II to Understand the Bug

If you have never written an emulator before, here are the five concepts you need. Each one matters for the bug.

### 1. What is a `.dsk` file?

A `.dsk` file is a raw dump of an Apple II floppy disk. It is exactly **143,360 bytes** = 35 tracks × 16 sectors × 256 bytes. The bytes are stored in a specific logical order, one sector after another:

```
.dsk layout (DOS order):
+-----------+-----------+-----------+     +-----------+
| T0 S0     | T0 S1     | T0 S2     | ... | T0 S15    |   <- track 0 (16 sectors)
+-----------+-----------+-----------+     +-----------+
| T1 S0     | T1 S1     | ...       |                     <- track 1
+-----------+-----------+-----------+
...
| T34 S0    | ...       | T34 S15   |                     <- track 34
+-----------+-----------+-----------+
```

Each sector is 256 bytes. "Sector N" inside the .dsk is the **logical** sector N — the index into the file, in order.

### 2. Physical vs Logical Sectors, and Why Interleave Exists

A real Apple II floppy has 16 sectors arranged in a circle around a track:

```
            phys 0
        phys 15   phys 1
      phys 14       phys 2
    phys 13           phys 3
   phys 12             phys 4
    phys 11           phys 5
      phys 10        phys 6
        phys 9   phys 7
            phys 8
```

The disk spins at 300 RPM. Each sector flies under the head at a specific moment. When DOS reads "logical sector 1" (the second sector of the file), does the disk controller read the physically-adjacent sector (physical sector 1)? **No.** That would be too slow.

Why? When DOS reads a sector, it takes time to copy the bytes out of the controller buffer and do something with them. By the time DOS is ready for the *next* sector, the disk has rotated past the physically-next sector. DOS would have to wait for a full revolution just to pick it up.

Solution: lay the sectors out in a **skewed** order. Put logical sector 1 *physically further away* so the rotation delay lines up with DOS's processing delay.

The DOS 3.3 RWTS interleave table is:

```
logical sector ->  0   1   2   3   4   5   6   7   8   9  10  11  12  13  14  15
physical sector ->  0  13  11   9   7   5   3   1  14  12  10   8   6   4   2  15
```

Read this as: "logical sector 1 is stored at physical sector 13." When DOS asks for logical sector 1, the controller waits until physical sector 13 comes under the head, then reads it.

The inverse mapping (physical-to-logical) is what you need when you are *receiving* raw sectors off the disk and want to decide where to store them:

```
physical sector -> 0   1   2   3   4   5   6   7   8   9  10  11  12  13  14  15
logical sector  -> 0   7  14   6  13   5  12   4  11   3  10   2   9   1   8  15
```

Notice these are **both permutations of 0..15** — and they happen to look very similar at a glance. That similarity is the whole reason this bug lived for four sessions. Two different tables, and they both "look right" if you don't stare.

### 3. How Apple II Booting Works (Three Stages)

```
+----------+      +----------+      +----------+      +--------+
|  Boot0   | ---> |  Boot1   | ---> |  Boot2   | ---> |  DOS   |
|  (PROM)  |      | (T0 S0)  |      |   (RAM)  |      | (full) |
+----------+      +----------+      +----------+      +--------+
   $C600            $0801             $3700/$B700       $9D84
  256 bytes      256 bytes          10 sectors         ~10KB
```

**Boot0** is a tiny program burned into a chip on the Disk II controller card ("PROM" = Programmable ROM). When you reset with a disk inserted, execution jumps to `$C600`. This PROM knows just enough to:
1. Spin up the motor.
2. Seek to track 0.
3. Read physical sector 0 into `$0800-$08FF`.
4. Jump to `$0801`.

That's it. 256 bytes of code. It reads exactly *one* sector.

**Boot1** lives inside T0 S0 — the sector Boot0 just loaded. It is responsible for reading the *other* 9 sectors on track 0 (and on some versions, a few from track 1) into RAM. These 10 sectors contain the RWTS (Read/Write Track and Sector) — the heart of DOS, plus boot2. Boot1 ends with `JMP ($08FD)` — an indirect jump whose target is decided by two bytes the programmer stored at `$08FD`/`$08FE`. On our disk that target is `$3700`.

**Boot2** lives at `$3700` (on a 16K-layout disk) or `$B700` (on a 48K-layout disk). It sets up the RWTS I/O block, calls a subroutine called `RWPAGES` to load the *rest* of DOS from tracks 0-2 into low memory, then resets the stack (`LDX #$FF; TXS`) and jumps to DOS cold-start.

**DOS cold-start** prints the `DOS VERSION 3.3` banner, connects to Applesoft BASIC, and prints the `]` prompt.

Crucially: if *any* of those stages lands on the wrong bytes — because a sector was stored in the wrong place — the entire boot collapses, usually ending up in the monitor.

### 4. GCR Encoding (6-and-2)

The Apple Disk II has no per-bit clock signal. It just reads a stream of magnetic flux transitions. The hardware can only reliably distinguish 64 specific byte patterns (those with certain bit spacing constraints). So raw data bytes (which can be any of 256 values) cannot be written directly.

The solution: encode 8-bit data into 8-bit "nibbles" where only 64 specific nibble values are used. Each sector byte is split into a top-6-bits piece and a bottom-2-bits piece; three sector bytes' low bits are combined into one nibble; the 86 "pre-nibbles" (low bits) are written first, then the 256 "sixes" nibbles (high bits). That is 6-and-2 encoding.

Our encoder and decoder both implement 6-and-2 correctly — we *proved* this with exhaustive byte-level roundtrip tests during session 3. That's important later.

### 5. The `JMP ($08FD)` Trick

At the end of boot1, this instruction runs:

```
$084A  JMP ($08FD)    ; indirect jump: read $08FD/$08FE, jump there
```

The bytes at `$08FD` and `$08FE` are *not* code — they are stored data at the end of the boot1 sector. They say: "boot2 is over at address X." On this disk, those bytes are `$00 $37`, so execution jumps to `$3700`.

This is a `JMP`, not a `JSR`. It does **not** push a return address. So if the code at `$3700` ever executes `RTS` (return from subroutine), it will pull *whatever happens to be on the stack* and jump there. Remember this.

---

## The Journey — Four Sessions of Wrong Answers

### Session 1-2: "It must be the XOR chain"

The original symptom was slightly different. Boot was crashing at `$085F` — inside boot1 itself, in the sector-reading loop. The first reviewer looked at our GCR encoder and found what looked like a smoking gun.

Canonical Apple DOS 3.3 RWTS uses a **running XOR** encoding:

```
last = 0
for k in 0..342:
    raw[k] = v[k] XOR last
    last = last XOR v[k]       ; last now accumulates the running XOR of all values
```

Our encoder did:

```go
last := uint8(0)
for i := 85; i >= 0; i-- {
    buf = append(buf, gcr62[pre[i]^last])
    last = pre[i]              // OVERWRITE — just the previous raw value
}
```

This is a **pairwise XOR**, not running. "Aha," we said, "that's the bug. RWTS expects running XOR; we're emitting pairwise. No wonder it can't decode."

We rewrote the encoder to use running XOR. Added a PROM-style independent decoder test. Everything still passed, but the boot now crashed in a *different* place — `$0005` instead of `$085F`. Different symptom, same failure.

**The twist**: pairwise XOR and running XOR are *algebraically equivalent* for this algorithm. The telescoping identity means both forms produce byte-for-byte identical data once RWTS's decoder processes them:

```
pairwise: raw[k] = v[k] XOR v[k-1]
decoded = last XOR raw[k] where last is running XOR of decoded so far
        = (v[0] XOR v[1] XOR ... XOR v[k-1]) XOR (v[k] XOR v[k-1])
        = v[k] XOR v[0] XOR ... XOR v[k-2]    (the v[k-1]s cancel)

hmm, that's not v[k]. Let me redo this...
```

Actually, the working reconstruction is: the PROM's decoder is `decoded[k] = raw[k] XOR decoded[k-1]`. Our pairwise encoding is `raw[k] = v[k] XOR v[k-1]`. So `decoded[k] = (v[k] XOR v[k-1]) XOR decoded[k-1]` — which by induction equals `v[k]` if `decoded[k-1] = v[k-1]`. And `decoded[0] = raw[0] XOR 0 = v[0] XOR 0 = v[0]`. Induction holds. **Both schemes decode correctly.**

So we "fixed" a bug that wasn't a bug. The symptom moved (because we simultaneously changed something else — the RAM init) but we were still looking in the wrong place.

**Lesson**: When your fix makes the crash address change but the crash persists, you probably didn't fix anything. You moved the lever.

### Session 3: "It must be uninitialized RAM"

Crash is now at `$0005`. We traced it carefully:

- Boot1 ends with `JMP ($08FD)` → `JMP $3700`.
- At `$3700` there is a tiny subroutine called `DELAY` that counts down in a loop and then does `RTS`.
- `RTS` pulls two bytes from the stack (`$01FE` then `$01FF`) and jumps there.
- Our emulator zero-initialized RAM, so `$01FE = $00, $01FF = $00`, so `RTS` jumps to address `$0001`.
- At `$0001` there is the byte `$C6` (left over from the ROM's slot-scan code which did `STA $01`).
- `$C6` is the opcode for `DEC zp` (decrement zero-page). It reads the next byte `$00` at `$0002` as an operand.
- Then the CPU hits `BRK` at `$0003`, which pushes PC+2 = `$0005` onto the stack.
- The BRK handler in ROM sees the break, jumps via `($03F0)` to the monitor entry — and the monitor prints `*`.

Crash address `$0005` matched the symptom exactly. We had a compelling story: **the real Apple II has non-zero power-up RAM values that happen to provide a valid-enough return path; our zero-RAM doesn't.**

So we implemented the classic Apple II 64-byte alternating power-up pattern:

```
$0000-$003F: $00 $00 $00 ... (block 0, even → zeros)
$0040-$007F: $FF $FF $FF ... (block 1, odd  → ones)
$0080-$00BF: $00 $00 $00 ... (block 2, even → zeros)
...
$01C0-$01FF: $FF $FF $FF ... (block 7, odd  → ones)
```

Now `$01FE = $FF, $01FF = $FF`, so `RTS` returns to `$FFFF + 1 = $0000`. Better! `$0000` is a BRK opcode, so it cleanly reaches the monitor instead of going sideways through `DEC $00`.

**But DOS still didn't boot.** We had a tidy crash-to-monitor, but no `]` prompt. Pressing Ctrl-Reset at the monitor warm-started to Applesoft BASIC — that worked. But Applesoft without DOS is just BASIC without commands like `CATALOG` or `LOAD`. Not a real boot.

Worse, the 64-byte pattern introduced a new bug: in some configurations execution fell into an infinite BEQ loop at `$0000`. We reverted to zero-fill.

We were now deep in a rabbit hole of stack semantics, `TXS`, `SP=$FD` vs `SP=$FF`, whether `Ctrl+R` should trigger warm-start. None of it was wrong per se — but none of it was the bug.

**Lesson**: A hypothesis that "real hardware happens to be lucky" should set off alarm bells. Real hardware is not lucky. It is deterministic. If our emulator disagrees with real hardware on a subject as fundamental as "where boot2 starts," the hardware isn't being lucky — *we're* reading the disk wrong.

### Session 4, First Half: "We need a pre-execution hook to seed the stack"

Now very frustrated. We wrote a long investigation document (`plan_boot2_investigation.md`) that concluded: "Maybe the disk image is a 16K layout that expects DOS at `$3600-$3FFF`, but internal cross-references assume `$B600-$BFFF` for 48K. The disk image has `$36` at `$08FE` but should have `$B6`. We need to patch it."

We proposed three options — all ugly:
- Option A: Add a CPU pre-execution hook that writes to the stack right before `JMP ($08FD)`, so that when DELAY's `RTS` fires, it returns to `$3A00` (boot2 init, we assumed).
- Option B: Detect the `JMP` and convert it to a `JSR` at runtime.
- Option C: A full address relocator that rewrites every absolute address inside the loaded DOS bytes.

At this point the plan was to add a `PreExecHook` field to the CPU, detect `JMP ($08FD)`, push a synthetic return address, and also patch a `JSR $3793` to `JSR $3A93` at runtime because `$3793` was "clearly in the middle of a data table."

That last sentence was the loose thread. Why would a shipping DOS 3.3 disk call `JSR $3793` if `$3793` is in the middle of a data table? That doesn't happen. Original Apple engineers were not *that* sloppy.

### Session 4, The Breakthrough: Disassemble The Actual Bytes

We stopped guessing and just disassembled. We wrote a script that read the raw `.dsk` file and dumped, for each logical sector, what bytes it contained and what instructions those bytes would execute *if loaded at each plausible address*.

Three findings changed everything:

**Finding 1.** `RWPAGES` — the RWTS subroutine that boot2 calls to load the rest of DOS — exists as actual 6502 code only at **file offset `$0193`** in T0. That is *file sector 1* (offset `$100` + `$93`).

**Finding 2.** Boot2, which lives at `$3A00`, contains this instruction at `$3A38`:
```
JSR $3793
```
For this JSR to be valid, the bytes of *file sector 1* must be loaded at `$3700`. Then `$3793` = `$3700 + $93` = the start of `RWPAGES` inside that sector. The JSR works. DOS loads. Everything is fine.

**Finding 3.** With our *current* interleave table, boot1 loads file sector 1 to `$3A00`, not `$3700`. And boot1 loads file sector 4 to `$3700`, not `$3A00`. These two sectors are *swapped* from where they should be.

### The "Aha" Moment

Here was the mental image that cracked it. With the current (buggy) interleave, here is what sat at each boot-critical address after boot1:

```
WITH BUGGY INTERLEAVE:
  $3700 <- file sector 4 = DELAY + GCR tables      (data, not a valid boot2)
  $3793 <- middle of a data table                  (not a valid subroutine)
  $3A00 <- file sector 1 = Boot2 init              (unreachable — we never jump here)
  $3A93 <- RWPAGES                                  (unreachable)
```

So when boot1 did `JMP ($08FD)` = `JMP $3700`, it landed on DELAY. DELAY did its counting loop and then `RTS`ed to garbage. Monitor.

**That was the crash we'd been chasing for three sessions.** The DELAY RTS was not some awful flaw in DOS; it was simply *never meant to be executed here*. DELAY is a motor-timing subroutine that RWTS calls with `JSR` during normal operation — it has a matching return address on the stack. Boot1 was not supposed to jump to DELAY at all. Boot1 was supposed to jump to boot2 init.

With the interleave fixed, after boot1:

```
WITH CORRECT INTERLEAVE:
  $3700 <- file sector 1 = Boot2 init              (Boot2 runs here!)
  $3793 <- RWPAGES subroutine                      (JSR $3793 works!)
  $3A00 <- file sector 4 = DELAY + GCR tables      (data — accessed by RWTS)
  $3A93 <- mid-data (no longer a jump target)
```

Now the flow is:
1. `JMP ($08FD)` → `JMP $3700`
2. `$3700` starts boot2 init — sets up the RWTS I/O block.
3. Boot2 does `JSR $3793` → calls RWPAGES → loads the rest of DOS from tracks 0-2.
4. Boot2 does `LDX #$FF; TXS` → resets the stack (so any earlier garbage is irrelevant).
5. `JMP $3FC8` → DOS cold-start → `]` prompt.

No stack seeding. No runtime patches. No power-up RAM pattern. No address relocator. Nothing. Just correct sector placement.

---

## The Actual Root Cause (With Diagrams)

### The Tables

Before (wrong) — this is what was in `disk/gcr.go`:

```go
var dosInterleave = [16]int{0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15}
```

After (right):

```go
var dosInterleave = [16]int{0, 13, 11, 9, 7, 5, 3, 1, 14, 12, 10, 8, 6, 4, 2, 15}
```

Both tables are correct permutations of `0..15`. Both have `[0]=0` and `[15]=15`. They are inverses of each other. The old one was the physical-to-logical map; the new one is the logical-to-physical map. Our code wanted the latter.

### Side-by-side: What lived at $3700?

```
                      BUGGY TABLE                   CORRECT TABLE
logical sector 1 ->   physical 7                    physical 13
logical sector 4 ->   physical 13                   physical 7

When boot1 reads phys 13 and stores it at $3700:
  BUGGY:    $3700 = file sector 4 = DELAY   -> crash
  CORRECT:  $3700 = file sector 1 = Boot2   -> boots DOS
```

Visual of the complete mapping damage:

```
     file sector      OLD (wrong) table     NEW (right) table
     (index in .dsk)  loaded to address     loaded to address
     -------------------------------------------------------------
     sec 0            $3F00                 $3F00   (same — boot0 code)
     sec 1            $3A00  <- boot2!!!    $3700   (correct: boot2 at $3700)
     sec 2            $3800                 $3E00
     sec 3            $3D00                 $3900
     sec 4            $3700  <- DELAY!!!    $3A00   (correct: DELAY at $3A00)
     sec 5            $3B00                 $3B00   (same)
     sec 6            $3E00                 $3800
     sec 7            $3C00                 $3D00
     sec 8            $3900                 $3C00
     sec 9            $3600                 $3600   (same — not actually loaded)

     The key swap: sectors 1 and 4 change addresses.
```

### Why The Comment Confused Everyone

The old table's comment was:

```go
// dosInterleave maps logical sector index -> physical sector on a DOS 3.3 track.
var dosInterleave = [16]int{0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15}
```

The comment said "logical → physical." The **values** in the array were the physical → logical mapping. Over several rewrites, someone either flipped the values or flipped the comment but not both. The code treated `dosInterleave[logSec]` as physical sector — consistent with the comment — so encoding and decoding both used the *physical* meaning of values but indexed by logical. Since both halves agreed, the roundtrip test passed. But the external world — boot1's sector-loading behavior — expected logical-to-physical.

---

## Lessons Learned

### 1. When a bug looks like hardware, double-check your I/O first.

We spent three sessions on RAM, stack, and CPU semantics — very low-level, hardware-adjacent stuff. The real bug was one layer up, in how we arranged bytes we already had. The disk data was on the filesystem, in memory, perfect. It just went to the wrong address.

If the symptom is "code is at the wrong address after I load it," always check how you loaded it before you question the CPU.

### 2. Round-trip tests cannot catch table-direction bugs.

```go
func TestRoundTrip(t *testing.T) {
    original := randomSectorData()
    encoded := Encode(original)
    decoded := Decode(encoded)
    require.Equal(t, original, decoded)
}
```

This test was green the entire time. It passes if encode and decode are **self-consistent**. If both halves use the same wrong table, the wrong cancels out. The test proves "our encoder and decoder agree" — it does *not* prove "our encoder writes what real RWTS expects."

The fix is a **conformance test** against an independent oracle — either the PROM itself (we did eventually build one for GCR), or a canonical reference like AppleWin's implementation, or — for interleave specifically — the DOS 3.3 RWTS source code from *Beneath Apple DOS*.

### 3. In boot code, the crash site is almost never the bug site.

When DELAY crashes, the bug is not in DELAY. When you execute a NOP that wasn't supposed to exist, the bug is not the NOP — it's that control flow went somewhere it shouldn't have. Boot sequences are long trains of jumps; one derailment produces a crash far downstream.

Every time we studied the *crash* (DELAY's RTS) we learned something that *seemed* relevant but wasn't causal. The bug was not where the crash was. The bug was upstream, in how bytes were laid out in memory before any code ran.

### 4. Cross-reference with canonical sources *early*.

*Beneath Apple DOS* by Don Worth and Pieter Lechner is the de-facto reference for DOS 3.3 internals. Page 3-21 has the RWTS sector translation table. If we had pasted that table into our test file in session 1 as a canonical reference — not just used it to author our own table — the bug would have been caught on day one.

Similarly, the GCR encode/decode is fully documented. The boot flow is fully documented. We were essentially rediscovering published material and making mistakes in the transcription.

### 5. When your plausible theory needs dozens of workarounds, the theory is wrong.

By session 4 we were contemplating: a pre-execution hook, runtime code patching, address relocators, stack seeding. Each workaround was "plausible." None was simple. That in aggregate should have been a loud signal: **real DOS 3.3 does not require any of this to boot on real hardware. If our emulator needs all this, we are diverging from hardware somewhere much more fundamental than we think.**

A good heuristic: *if your fix would require modifying the disk image, you haven't found the fix.*

---

## Why This Bug Was So Hard To Find

A short catalog of the things that made this bug a perfect storm:

### The DELAY subroutine has no absolute addresses.

DELAY at `$3700` is pure relative code — loops, branches, INCs of zero-page. It "runs correctly" no matter where you load it. So when boot1 jumped to DELAY instead of boot2, DELAY happily executed. It didn't crash immediately. It completed its loop and cleanly `RTS`ed. The crash was in DELAY's caller, not DELAY itself. This made DELAY look like the problem.

If DELAY had contained even one `JSR $xxxx` to a sibling subroutine, it would have crashed inside itself, and we would have noticed immediately that DELAY was at the wrong address.

### Round-trip tests passed.

`TestDOS33_EncodeDecodeRoundTrip_AllTracks` tested that encoding and decoding were inverses. They were. Both used the same (wrong) table direction. The test was technically correct and completely useless for this bug class.

### T0 S0 always works regardless of interleave.

Physical sector 0 is always logical sector 0 in any sensible interleave table — the first slot never moves. So boot0 (which reads only physical sector 0) always loaded boot1 correctly. The PROM + T0S0 path — the easiest part to test — was unaffected. We validated T0S0 loading extensively and it was always green.

### The symptom pointed to RAM/stack issues.

The DELAY RTS reading from `$01FE/$01FF` was a real mechanism — it was really happening. It was really pulling zero bytes. The crash at `$0005` really was explained by that mechanism. Everything we said about uninitialized RAM was factually true. It just wasn't the *root cause*. It was the *last link in a chain* whose first link was a swapped sector.

Root-cause analysis that stops at "this explains the crash" misses cases where the crash has a valid proximate cause but an invalid upstream one.

### Only logical sectors 1 and 4 mattered for early boot.

The swap of sectors 1 ↔ 4 was what broke `$3700`. But many other sector swaps in the table affected places like `$3800`, `$3900`, `$3C00`, etc. — which are touched later, by code that *we were never reaching*. So from the emulator's perspective, only one "visible" swap was happening. If the interleave had broken boot0 (physical sector 0), we would have noticed in the first minute. Instead it broke exactly the next-most-important thing and nothing more visible.

### The comment lied.

```go
// dosInterleave maps logical sector index -> physical sector
```

The comment claimed one direction. The values implemented the other. A careful reader would check the values against *Beneath Apple DOS*, not trust the comment. But when debugging, you read the comment first and skim the values, because the values are opaque 16-element arrays.

---

## What The Final Fix Actually Looked Like

Three lines of real change. One table swap, plus a regression test and a comment fix.

```go
// In disk/gcr.go:
// Before:
var dosInterleave = [16]int{0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15}
// After:
var dosInterleave = [16]int{0, 13, 11, 9, 7, 5, 3, 1, 14, 12, 10, 8, 6, 4, 2, 15}
```

Regression test to lock it:

```go
// TestDOSInterleaveDirection guards against re-inverting the table.
func TestDOSInterleaveDirection(t *testing.T) {
    if dosInterleave[1] != 13 {
        t.Errorf("dosInterleave[1] = %d, want 13 (logical 1 -> physical 13)", dosInterleave[1])
    }
    if dosInterleave[4] != 7 {
        t.Errorf("dosInterleave[4] = %d, want 7 (logical 4 -> physical 7)", dosInterleave[4])
    }
}
```

That's it. The emulator boots DOS 3.3 to `]`. `CATALOG` works. `LOAD HELLO` works. Full BASIC + DOS.

Everything else that got built during the hunt — the RAM power-up pattern investigation, the pre-execution hook plan, the BRK-handler tracing, the standalone PROM + mini-6502 simulation for testing GCR conformance — was scaffolding. Some of it we kept (the integration test, the cleaned-up comments, the disk tracer). Some of it we reverted (RAM pattern, CPU hooks). None of it *was* the fix.

---

## Closing

Emulator bugs love to hide in boring places. The exciting places — the CPU, the ROM, the memory timing — are where you look first because that's where the interesting bugs *could* be. But most shipped emulator bugs aren't in those places, because those places are rigorously tested against reference hardware and reference emulators.

The boring places — "does my 16-entry lookup table go left or right?" — are where the bugs actually live, because they are tested only against themselves. And when they are wrong, they are silently wrong in ways that produce perfectly plausible-looking failures far from the cause.

Every time you write a permutation table, ask: **Is there an independent source of truth for this, and have I compared against it?** Not "does it round-trip," not "does it look symmetric," not "did I copy it from somewhere." *Does it match Beneath Apple DOS page 3-21.*

That's the whole story.

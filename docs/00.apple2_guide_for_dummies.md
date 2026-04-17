# Apple II+ for Dummies

A beginner-friendly guide for MS-DOS users who just booted an Apple II+ for the first time.

---

## 1. What Is the `]` Prompt?

You see a blinking cursor next to a `]` character. Congratulations, you have
booted into **Applesoft BASIC** -- the Apple II's built-in programming language
and primary user interface.

Think of it this way:

| What you know (MS-DOS)         | What you are looking at (Apple II)        |
|--------------------------------|-------------------------------------------|
| `C:\>` prompt                  | `]` prompt                                |
| COMMAND.COM (the DOS shell)    | Applesoft BASIC (the ROM interpreter)     |
| You type DOS commands          | You type BASIC statements                 |
| Commands operate on files      | Statements operate on memory              |

The crucial difference: **the `]` prompt IS the operating system.** There is no
file manager underneath. There are no directories. The Apple II boots directly
into a BASIC interpreter that lives in ROM. Everything you type either executes
immediately or gets stored as a numbered program line in RAM.

If you turn the machine off, everything you typed is gone. There is no hard
drive. In this emulator there is not even a floppy disk (yet).


---

## 2. Your First Commands

Type each of these at the `]` prompt and press RETURN (the Apple II word for
ENTER). Every command must be in UPPERCASE -- the Apple II keyboard has no
lowercase. The emulator auto-converts for you, so just type normally.

### Say hello

```
PRINT "HELLO WORLD"
```

This is the Apple II equivalent of `ECHO HELLO WORLD` in DOS. It prints text to
the screen and returns you to the `]` prompt.

### Use it as a calculator

```
PRINT 2 + 2
PRINT 256 * 256
PRINT 3.14159 * 10 * 10
```

Applesoft BASIC does math for you. Integers, floating point, the usual
operators: `+`, `-`, `*`, `/`, `^` (exponent).

### Clear the screen

```
HOME
```

This is `CLS` in DOS. The screen is cleared and the cursor moves to the top-left
corner.

### List files (sort of)

```
CATALOG
```

This is `DIR` in DOS. But since there is no disk drive attached, you will get an
error or see nothing. That is normal.

### Check how much memory you have

```
PRINT FRE(0)
```

This prints the number of free bytes in RAM. A stock Apple II+ has about 48 KB
total, with roughly 38,000 bytes free for your BASIC programs.

### Floating-point math (Apple II+ only)

The original Apple II shipped with Integer BASIC -- no decimals allowed. The
Apple II+ replaced it with Applesoft BASIC (written by Microsoft), which adds
full floating-point arithmetic. Try these:

```
PRINT 22 / 7
PRINT SQR(2)
PRINT SIN(3.14159 / 4)
PRINT LOG(100)
PRINT 1.5E6 * 2
```

Scientific notation works too: `1.5E6` is 1,500,000. This was a big deal in 1979
-- the original Apple II could not do any of this without loading a separate
language from a floppy disk.


---

## 3. Writing a BASIC Program

In DOS, you write batch files with a text editor. On the Apple II, you write
programs by typing numbered lines directly at the `]` prompt. The line numbers
determine the order of execution.

### Enter a program

```
10 PRINT "HELLO"
20 GOTO 10
```

Nothing happens yet. You have stored two lines in memory. The `]` prompt comes
back after each line, waiting for more.

### See your program

```
LIST
```

This displays all stored program lines, similar to `TYPE HELLO.BAT` in DOS. You
will see:

```
10  PRINT "HELLO"
20  GOTO 10
```

### Run your program

```
RUN
```

The screen fills with HELLO HELLO HELLO... forever. This is an infinite loop.

### Stop a running program

Press **Ctrl+C**. This is the Apple II equivalent of Ctrl+Break in DOS. The
program stops, and you get the `]` prompt back with a message like `BREAK IN 10`.

### Edit a line

There is no line editor. To change line 10, just retype it:

```
10 PRINT "GOODBYE"
```

The old line 10 is replaced. Type `LIST` to confirm.

### Delete a line

Type the line number by itself:

```
20
```

Line 20 is deleted. Type `LIST` to confirm only line 10 remains.

### Erase the entire program

```
NEW
```

All program lines are gone. `LIST` will show nothing. This is like closing a file
without saving in DOS -- except there was never a file to begin with.


---

## 4. The Keyboard Is WEIRD

The Apple II keyboard is nothing like a modern PC keyboard. Here are the things
that will trip you up.

### No lowercase

The original Apple II and Apple II+ have no lowercase letters. Everything you
type appears in UPPERCASE. The emulator handles this automatically -- you can
type in lowercase on your real keyboard and it converts to uppercase.

### Backspace does NOT erase

On the Apple II, the left-arrow key (mapped to Backspace in this emulator) moves
the cursor left but does NOT delete the character. The old character is still
there. This is maddening at first.

**To fix a typo:** Do not try to backspace and correct it. Instead, press RETURN
to abandon the line (it will produce a SYNTAX ERROR, which is fine), then retype
the entire line with the correct text. If it was a numbered program line, just
retype it with the same line number.

### Key reference for the emulator

| Key on your keyboard | What the Apple II sees | What it does              |
|----------------------|------------------------|---------------------------|
| Letters a-z          | A-Z (uppercase)        | Auto-converted            |
| Return / Enter       | RETURN                 | Submit the current line   |
| Backspace            | Left arrow             | Move cursor left (no erase)|
| Arrow keys           | Arrow keys             | Move cursor around        |
| Ctrl+C               | Ctrl+C                 | Break / stop program      |
| Ctrl+R               | (emulator only)        | Reset the Apple II        |
| Escape               | (emulator only)        | Quit the emulator         |

### Ctrl+C is your best friend

Stuck in an infinite loop? Screen going crazy? Just press Ctrl+C. It breaks out
of any running BASIC program and returns you to the `]` prompt.


---

## 5. Why It Is Not Like DOS

If you are expecting something like DOS, here is why the Apple II feels alien.

### DOS has a file system. The Apple II (in this emulator) does not.

In DOS, your programs are `.EXE` or `.BAT` files on a disk. You navigate
directories with `CD`, list files with `DIR`, and run programs by typing their
name.

The Apple II has none of that without a disk drive. Your BASIC program lives only
in RAM. When you type `NEW` or reset the machine, it is gone. There is no `SAVE`
command that works without a disk. There is no `LOAD`. There are no directories.

### The `]` prompt is both the OS and the programming language

In DOS, you have `COMMAND.COM` (the shell) and then you run separate programs.
On the Apple II, the `]` prompt is Applesoft BASIC. It is simultaneously:

- A command prompt (you can type `HOME`, `CATALOG`, etc.)
- A calculator (`PRINT 2+2`)
- A program editor (type numbered lines)
- A program runner (`RUN`)

### Programs are numbered lines in RAM, not files on disk

DOS batch file:

```
@ECHO OFF
ECHO HELLO
PAUSE
```

Apple II BASIC program:

```
10 HOME
20 PRINT "HELLO"
30 END
```

The line numbers (10, 20, 30) serve double duty: they define execution order AND
they are how you address lines for editing. Convention is to count by 10 so you
can insert lines between existing ones (e.g., line 15 goes between 10 and 20).

### Memory map, not file system

The Apple II thinks in terms of memory addresses, not files. The 64 KB address
space is divided roughly as:

| Address Range    | What Lives There            |
|------------------|-----------------------------|
| $0000 - $00FF   | Zero page (fast variables)  |
| $0100 - $01FF   | CPU stack                   |
| $0200 - $02FF   | Keyboard input buffer       |
| $0400 - $07FF   | Text screen memory          |
| $0800 - $BFFF   | RAM (your BASIC program)    |
| $C000 - $C0FF   | I/O soft switches           |
| $D000 - $FFFF   | ROM (BASIC + Monitor)       |


---

## 6. Fun Programs to Try

Copy these programs line by line. Type `RUN` to execute. Press Ctrl+C to stop
any that loop forever.

### A counting machine

```
10 FOR I = 1 TO 100
20 PRINT I
30 NEXT I
```

This prints numbers 1 through 100. The `FOR`/`NEXT` loop is like a DOS `FOR`
loop but built into the language.

### Guess the number game

```
10 HOME
20 N = INT(RND(1) * 100) + 1
30 PRINT "I AM THINKING OF A NUMBER"
40 PRINT "BETWEEN 1 AND 100."
50 PRINT
60 INPUT "YOUR GUESS? "; G
70 IF G < N THEN PRINT "TOO LOW!": GOTO 60
80 IF G > N THEN PRINT "TOO HIGH!": GOTO 60
90 PRINT "YOU GOT IT!"
```

Type a number and press RETURN when prompted. The program tells you if your
guess is too high or too low.

### Scrolling banner

```
10 HOME
20 FOR I = 1 TO 1000
30 PRINT "APPLE II ";
40 NEXT I
```

The semicolon at the end of PRINT on line 30 prevents a line break, so the text
flows across the screen continuously.

### Multiplication table

```
10 HOME
20 FOR R = 1 TO 10
30 FOR C = 1 TO 10
40 PRINT R * C; TAB(C * 5);
50 NEXT C
60 PRINT
70 NEXT R
```

### Random number art

```
10 HOME
20 X = INT(RND(1) * 40)
30 HTAB X + 1
40 PRINT "*";
50 GOTO 20
```

Stars appear at random positions across each line, scrolling down the screen.

### Lo-res graphics teaser

Once the emulator supports lo-res graphics mode, try:

```
10 GR
20 FOR I = 0 TO 15
30 COLOR= I
40 FOR Y = 0 TO 39
50 PLOT I * 2, Y
60 PLOT I * 2 + 1, Y
70 NEXT Y
80 NEXT I
```

This draws 16 colored vertical stripes. The Apple II has 16 colors in lo-res
mode on a 40x48 grid. But this requires graphics rendering to be implemented in
the emulator first.

### Hi-res graphics teaser (Apple II+ only)

The original Apple II had no BASIC commands for hi-res graphics. You had to POKE
raw bytes into screen memory. Applesoft BASIC on the II+ added dedicated
commands. Once the emulator supports hi-res mode, try:

```
10 HGR
20 HCOLOR= 3
30 FOR I = 0 TO 279
40 HPLOT I, 0 TO I, 191
50 NEXT I
```

Hi-res mode gives you 280x192 pixels with 6 colors. The key commands:

| Command             | What It Does                                    |
|---------------------|-------------------------------------------------|
| `HGR`               | Switch to hi-res page 1 (clears screen)        |
| `HGR2`              | Switch to hi-res page 2                        |
| `HCOLOR= n`         | Set drawing color (0-7)                        |
| `HPLOT x, y`        | Plot a single pixel                            |
| `HPLOT x1,y1 TO x2,y2` | Draw a line between two points             |
| `HPLOT TO x, y`     | Continue a line from the last plotted point     |

These commands alone made the Apple II+ the preferred machine for educational
software and early graphical games. Integer BASIC on the original Apple II had
`PLOT` and `COLOR` for lo-res only -- hi-res required machine language.


---

## 7. Apple II vs MS-DOS Quick Reference

| Apple II Command  | MS-DOS Equivalent    | What It Does                         |
|-------------------|----------------------|--------------------------------------|
| `PRINT "text"`    | `ECHO text`          | Display text on screen               |
| `HOME`            | `CLS`                | Clear the screen                     |
| `LIST`            | `TYPE file.bas`      | Show the current program             |
| `RUN`             | `program.exe`        | Execute the program in memory        |
| `NEW`             | (close without save) | Erase the program from memory        |
| `CATALOG`         | `DIR`                | List files on disk                   |
| `SAVE "NAME"`     | `COPY CON file`      | Save program to disk (needs disk)    |
| `LOAD "NAME"`     | `TYPE file`          | Load program from disk (needs disk)  |
| `PRINT FRE(0)`    | `MEM`                | Show free memory                     |
| `CALL -151`       | `DEBUG`              | Enter machine language monitor       |
| Ctrl+C            | Ctrl+Break           | Stop a running program               |
| Ctrl+R            | Ctrl+Alt+Del         | Reset the machine (emulator only)    |
| `INPUT "? "; X`   | `SET /P X=?`         | Read user input into a variable      |
| `PEEK(addr)`      | `DEBUG` then `d`     | Read a memory address                |
| `POKE addr,val`   | `DEBUG` then `e`     | Write to a memory address            |
| `FOR...NEXT`      | `FOR...IN`           | Counted loop                         |
| `GOTO line`       | `GOTO :label`        | Jump to a line/label                 |
| `GOSUB line`      | `CALL :label`        | Call a subroutine                    |


---

## 8. How Memory Works

The Apple II+ has **48 KB of RAM** and **12 KB of ROM** (or 16 KB with some ROM
versions), fitting into a 64 KB address space.

### RAM ($0000 - $BFFF): 48 KB

This is your workspace. The first 2 KB or so is used by the system (zero page,
stack, input buffer, screen memory). The rest, from about $0800 to $BFFF, is
where your BASIC program and variables live.

When you type `10 PRINT "HELLO"`, the Apple II tokenizes that line and stores it
starting around $0801. Variables are stored at the end of the program, growing
upward. The stack for `GOSUB`/`RETURN` and `FOR`/`NEXT` grows downward from the
top of available RAM.

### Screen memory ($0400 - $07FF): 1 KB

The 40-column by 24-row text display is mapped directly to memory addresses
$0400 through $07FF. When you `PRINT "HELLO"`, the BASIC interpreter writes the
ASCII codes for H, E, L, L, O into this memory region. The video hardware reads
those bytes and draws the characters on screen.

This is why you can do tricks like `POKE 1024, 193` -- that writes the letter
"A" directly to the top-left corner of the screen, bypassing PRINT entirely.

### ROM ($D000 - $FFFF): 12 KB

This contains two things:

1. **Applesoft BASIC** ($D000 - $F7FF): The interpreter that gives you the `]`
   prompt and understands commands like PRINT, LIST, RUN, etc.

2. **The Monitor** ($F800 - $FFFF): A low-level machine language tool, like
   DEBUG.COM in DOS. It lets you examine and modify memory, enter machine code,
   and do other hardware-level tasks.

### No disk, no persistence

In this emulator, there is no disk drive emulation yet. That means:

- `SAVE` and `LOAD` will not work
- `CATALOG` will show an error or nothing
- Everything you type is lost when you quit

Think of it as a computer with only RAM and ROM -- a very expensive calculator
with a TV screen.


---

## 9. The Autostart Monitor (Apple II+ Only)

The Apple II+ replaced the original Monitor ROM with the **Autostart Monitor**.
Two practical differences:

### Warm reset preserves your program

On the original Apple II, pressing Reset (Ctrl+R in this emulator) wiped your
program and dropped you into the Monitor (`*` prompt). You lost everything.

On the Apple II+, pressing Reset does a **warm start** -- it returns to the `]`
prompt but your BASIC program is still in memory. Type `LIST` and it is still
there. Type `RUN` and it picks up where you left off. This was a huge quality-of-
life improvement.

To force a **cold start** (wipe everything), hold Ctrl while pressing Reset, or
type `NEW` at the prompt.

### Auto-boot from disk

When the Apple II+ powers on, the Autostart Monitor checks slot 6 for a disk
controller. If found, it automatically boots the disk -- no need to type
`6 CTRL+P` like on the original Apple II. This is why Apple II+ users just turn
on the machine and a game loads. (Not relevant yet in this emulator since there
is no disk drive.)


---

## 10. The Apple II Monitor

If you are the curious type (and since you are building an emulator, you probably
are), there is a hidden power tool inside the Apple II.

### Entering the Monitor

At the `]` prompt, type:

```
CALL -151
```

The prompt changes from `]` to `*`. You are now in the **Apple II Monitor**, a
machine-language environment similar to `DEBUG.COM` in DOS.

`CALL -151` works because -151 in 16-bit unsigned math is $FF69, which is the
Monitor's entry point in ROM.

### What can you do in the Monitor?

**Examine memory:**

```
0400
```

Type a hex address and press RETURN. The Monitor displays the byte at that
address. This is like `D 0400` in DEBUG.

**Examine a range:**

```
0400.0410
```

Shows bytes from $0400 to $0410. You will see the text screen memory.

**Write to memory:**

```
0400: C1
```

This writes the byte $C1 (the letter "A" with high bit set) to address $0400,
the top-left corner of the text screen. You should see an "A" appear instantly.

**Disassemble code:**

```
FF69L
```

The `L` suffix disassembles instructions starting at the given address. You can
see the actual 6502 machine code of the Monitor itself.

### Getting back to BASIC

Type one of these:

- **Ctrl+C** then RETURN -- soft return to BASIC
- **3D0G** -- jumps to the BASIC warm-start entry point at $03D0

The `G` command in the Monitor means "Go" -- execute code starting at the given
address. $03D0 is where BASIC's warm-start routine lives.

### Monitor vs DEBUG.COM comparison

| Monitor (Apple II)  | DEBUG (MS-DOS)    | What It Does                |
|---------------------|-------------------|-----------------------------|
| `0400`              | `D 0400`          | Examine memory              |
| `0400: C1`          | `E 0400 C1`       | Write byte to memory        |
| `FF69L`             | `U FF69`          | Disassemble code            |
| `300G`              | `G 300`           | Execute code at address     |
| Ctrl+C              | `Q`               | Return to command prompt    |


---

## 11. What Is Different from the Original Apple II?

You are running an Apple II+ ROM. Here is what changed from the 1977 original:

| | Original Apple II (1977) | Apple II+ (1979) |
|---|---|---|
| BASIC | Integer BASIC (no decimals) | Applesoft BASIC (floating-point) |
| Prompt | `>` | `]` |
| Hi-res cmds | None (machine language only) | `HGR`, `HPLOT`, `HCOLOR` |
| Reset | Cold start, drops to Monitor | Warm start, preserves program |
| Boot | Manual disk boot | Auto-boot from slot 6 |
| Hardware | 6502, 48KB RAM, 8 slots | Identical -- ROM swap only |

The motherboard, CPU, RAM, video, keyboard, and I/O are all the same. Woz got
it right the first time. Apple just shipped better ROMs two years later.


---

## What Next?

You have an Apple II running Applesoft BASIC. Here is a rough progression:

1. **Play with BASIC.** Type the example programs above. Modify them. Break them.
   Fix them. This is how people learned to program in the early 1980s.

2. **Explore the Monitor.** Use `CALL -151` to poke around memory. Change screen
   memory directly. Watch how BASIC stores your program in RAM.

3. **Wait for disk support.** Once the emulator supports disk images (.DSK
   files), you will be able to load real Apple II software -- games, utilities,
   and applications from the 1980s.

4. **Wait for graphics.** Lo-res (40x48, 16 colors) and hi-res (280x192, 6
   colors) graphics modes will open up a whole world of visual programming.
   The II+ has `HGR`/`HPLOT` built in -- no machine language needed.

The Apple II was the machine that launched the personal computer revolution. In
1977, it was the most capable and expandable home computer you could buy. Thirty
thousand of them sold in the first year. Millions followed. And it all started
with that blinking cursor at the `]` prompt.

Welcome to 1977.

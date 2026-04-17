# DOS 3.3 for Dummies

A warm, practical guide for someone who just saw a blinking `]` on a freshly booted Apple II emulator and is wondering "now what?"

You know modern programming. You know shells, files, editors. You know nothing about 1980. This guide gets you productive in an hour.

---

## 1. Welcome — What Am I Looking At?

### The blinking `]`

That thing on your screen is the **Applesoft BASIC prompt**. It's waiting for you to type something. It is the entire user interface of this computer.

There is no shell. There is no file manager. There is no window. The prompt is where everything happens — running programs, saving files, playing games, writing code. All of it.

### What is DOS 3.3?

**DOS 3.3** is Apple's disk operating system from 1980. "DOS" stands for Disk Operating System, but don't confuse it with MS-DOS (which is a different, unrelated DOS from 1981 on IBM PCs). Apple's DOS 3.3 was the third major revision of the software Apple wrote to let the Apple II read and write floppy disks.

Here's the key insight that will save you a lot of confusion:

> **DOS 3.3 is NOT a shell. It is a thin layer of disk commands bolted onto BASIC.**

When you type `CATALOG` or `LOAD HELLO`, those aren't programs DOS executes. They aren't shell built-ins. They are **BASIC commands that DOS intercepts**. DOS hooked itself into the BASIC interpreter's input routine. Every line you type is really a BASIC statement; DOS just watches for the ones that start with its command words and handles those itself.

So really there are two things talking to you through that `]` prompt:

1. **Applesoft BASIC** — a full programming language (variables, loops, arithmetic, strings, graphics, sound)
2. **DOS 3.3** — disk commands (load, save, catalog, delete, etc.)

They share the same prompt. You don't switch between them. You just type.

### What is Applesoft BASIC?

Applesoft BASIC is the programming language built into the Apple II+. It was written by Microsoft (yes, that Microsoft, in 1977) and licensed to Apple. It's a dialect of BASIC — line-numbered, interpreted, with GOTO and GOSUB. It handles floating point math, strings, low-res and high-res graphics, and sound.

You can type BASIC statements interactively (they run immediately), or you can type them with a line number at the front and they become part of a program.

Examples:

```
]PRINT 2 + 2
4

]10 PRINT "HELLO"
]RUN
HELLO
```

The first line ran immediately. The second was stored as line 10 of a program. `RUN` executed it.

### The two prompts: `]` vs `*`

There are actually **two prompts** on the Apple II+ and they mean different things:

| Prompt | Who | What it is |
|---|---|---|
| `]` | Applesoft BASIC | Where you spend 99% of your time. |
| `*` | System Monitor | A low-level debugger. Think: a raw 6502 machine monitor. |

If you ever see `*`, don't panic. You dropped into the monitor. Type `3D0G` (three-dee-zero-G, enter) and you'll return to BASIC. We'll cover the monitor later.

> **Analogy for modern folks:** `]` is like a Python REPL that also happens to be your shell. `*` is like `gdb` for the whole machine.

### Why all the UPPERCASE?

The Apple II+ keyboard literally has no lowercase letters. Neither does the screen. Everything is uppercase — commands, variable names, text, strings. Don't fight it. Just use caps.

---

## 2. Your First Session — Five Minutes to Feel at Home

Let's actually DO things. Type each of these and watch what happens.

### Your first command

```
]PRINT "HELLO"
HELLO

]
```

Congratulations, you programmed an Apple II. `PRINT` outputs something. Strings go in double quotes. The computer printed `HELLO` and gave you the prompt back.

Short version — `?` is a built-in abbreviation for `PRINT`:

```
]? 2*3
6
```

### See what's on the disk

```
]CATALOG
```

You'll see a listing like this:

```
DISK VOLUME 254

 A 002 HELLO
 B 034 COPY.OBJ0
 T 003 APPLESOFT
 I 009 LITTLE BRICK OUT
 ...
```

Each line is one file. The first column is the **file type** (A/B/T/I/S). The second is the **size in sectors** (256 bytes each). Then the filename.

> **Modern analogy:** `CATALOG` is `ls`. The file types replace file extensions.

Tip: filenames can be up to 30 characters and can contain spaces. Yes, really. `RUN LITTLE BRICK OUT` is a valid command.

### Clear the screen

```
]HOME
```

Screen clears. `HOME` means "home the cursor" — move it to the top-left and clear everything. It's your `clear` / `cls`.

### Write a tiny program

Type these lines exactly. Notice the line numbers.

```
]NEW
]10 HOME
]20 PRINT "HI THERE"
]30 GOTO 20
```

Now look at what you typed:

```
]LIST

10  HOME
20  PRINT "HI THERE"
30  GOTO 20
```

`LIST` shows your current program. Let's run it:

```
]RUN
```

The screen clears and fills with "HI THERE" forever. It's an infinite loop.

**To stop it:** press `Ctrl+C`. You'll see:

```
BREAK IN 20
]
```

That's BASIC telling you "you stopped me in line 20." If `Ctrl+C` doesn't work for some reason, try the Reset key (if your emulator has one mapped — often it's mapped to some F-key or Ctrl+key). Reset is the nuclear option — it may drop you to the `*` monitor, in which case type `3D0G` to get back.

### Useful housekeeping

```
]NEW
```

Erases the program in memory. Doesn't touch the disk. Use this when you want to start a fresh program.

```
]LIST 20
```

Lists just line 20. You can also list ranges: `LIST 10,30` lists lines 10 through 30.

Congratulations. You can already read files, write programs, run them, stop them, and clear memory. That's more than most 1980 parents could do.

---

## 3. Essential Commands Reference

Grouped by what they're for. All of these are typed at the `]` prompt.

### Disk commands (DOS 3.3's contribution)

| Command | What it does |
|---|---|
| `CATALOG` | List all files on the currently inserted disk. |
| `CATALOG,S6,D1` | Same but force slot 6, drive 1. |
| `CATALOG,S6,D2` | List the disk in drive 2 of slot 6 (if you have one). |
| `LOAD name` | Load a BASIC program from disk into memory. Doesn't run it. |
| `SAVE name` | Save the current BASIC program to disk with that name. |
| `RUN name` | Load and run a BASIC program in one step. |
| `RUN` | Run the program already in memory. |
| `DELETE name` | Delete a file from the disk. Gone forever. |
| `RENAME old,new` | Rename a file. No space after the comma. |
| `LOCK name` | Write-protect an individual file (so you can't overwrite or delete it). |
| `UNLOCK name` | Unlock a locked file. |
| `BSAVE name,A$addr,L$length` | Save raw bytes from memory (for machine code, images, etc.) |
| `BLOAD name` | Load a binary file back to the address it was saved from. |
| `BLOAD name,A$addr` | Load a binary file to a specific address. |
| `BRUN name` | Load and jump to a binary program. |
| `EXEC name` | Execute a text file as if it were typed at the keyboard. |
| `INIT HELLO` | Format a new blank disk and install DOS on it. Destroys the disk. |
| `VERIFY name` | Check that a file can be read without errors. |
| `MAXFILES n` | Reserve n file buffers (default 3). |

**About the `A$` and `L$` on BSAVE:** the `$` means "hex". `A$300` means address `$0300` = 768 decimal. `L$10` means length `$10` = 16 bytes. You can also write them in decimal without the `$`: `A768,L16`.

Example:
```
]BSAVE SPRITE,A$2000,L$400
```
Saves 1024 bytes (`$400`) starting at address `$2000` (8192) to a file named SPRITE.

> **Gotcha:** Under DOS 3.3, disk commands only work when typed at the prompt OR inside a BASIC program using `PRINT CHR$(4)"COMMAND"`. The `CHR$(4)` is Ctrl-D, DOS's secret handshake. More on this later.

### BASIC commands (Applesoft)

**Program control:**
- `RUN` — run the program from the start
- `RUN 100` — run starting at line 100
- `STOP` — pause the program (shows `BREAK IN nnn`)
- `CONT` — continue after a stop (not after editing the program)
- `END` — stop the program cleanly
- `NEW` — erase the program from memory
- `LIST` — show the program
- `LIST 20,50` — show lines 20 through 50

**Screen & graphics modes:**
- `HOME` — clear the text screen, cursor to top
- `TEXT` — switch to 40-column text mode (the default)
- `GR` — switch to lo-res graphics (40x40 color blocks, 4 lines of text at bottom)
- `HGR` — switch to hi-res graphics (280x160, 6 colors, 4 lines of text at bottom)
- `HGR2` — full-screen hi-res (280x192, no text)
- `INVERSE` — subsequent PRINTs are black-on-white
- `NORMAL` — back to white-on-black
- `FLASH` — flashing text (yes, really)
- `SPEED= 255` — max print speed (default). Lower numbers print slower.

**Loops and conditionals:**
```
FOR I = 1 TO 10 ... NEXT I
IF X > 5 THEN PRINT "BIG"
IF X = 3 THEN GOTO 100
GOTO 50
GOSUB 1000  / RETURN
```

**Input:**
- `INPUT A` — read a number into A
- `INPUT "PROMPT?"; A$` — print a prompt, read a string into A$
- `GET A$` — read a single keystroke (no Enter needed)

**Line numbers:** BASIC stores your program by line number. Typing a line with a new number inserts it. Typing a line with an existing number replaces it. Typing just a line number (no code after) **deletes** that line.

```
]10 PRINT "A"
]10 PRINT "B"   <- replaces
]10              <- deletes
```

Tradition: number your lines 10, 20, 30, 40... That way you can squeeze 11, 12 in later without renumbering.

### Monitor commands (the `*` prompt)

The monitor is a tiny machine-code debugger. You enter it one of two ways:

1. `CALL -151` — from BASIC, the polite way
2. Reset (or Ctrl+Reset) — rude but works from anywhere

Exit it by typing `3D0G` (return to BASIC warm start).

Basic monitor commands:

| What you type | What happens |
|---|---|
| `300` | Show one byte at address $300 |
| `300.310` | Show bytes from $300 to $310 |
| `300:A9 41 60` | Write bytes `$A9 $41 $60` starting at $300 |
| `300G` | Go (execute) starting at $300 |
| `FF59G` | Go to $FF59 — this does the Apple II power-on beep |
| `3D0G` | Return to BASIC |
| `F666G` | Enter mini-assembler (on II+ ROMs that have it) |

Example session:

```
]CALL -151

* 300:A9 C1 20 ED FD 60

* 300L
0300-   A9 C1     LDA #$C1
0302-   20 ED FD  JSR $FDED
0305-   60        RTS

* 300G
A

* 3D0G
]
```

We poked a tiny machine-code program (`LDA #$C1 / JSR $FDED / RTS`) that calls the ROM's COUT routine to print character `$C1` which is "A". Then we ran it.

`L` after an address **disassembles** starting there (the II+ monitor has a disassembler).

---

## 4. File Types Explained

When you `CATALOG`, the first column is a single letter. It tells DOS how to load that file.

| Type | Name | What it is |
|---|---|---|
| `A` | Applesoft | An Applesoft BASIC program. Use `LOAD`/`RUN`. |
| `I` | Integer | An Integer BASIC program (older BASIC, pre-Applesoft). Rare today — loading one needs `INT` first. |
| `B` | Binary | Raw bytes. Machine code, images, data. Use `BLOAD`/`BRUN`. |
| `T` | Text | A sequential text file. Read with `OPEN`/`READ` from a BASIC program, or use `EXEC`. |
| `S` | System | Reserved type, rarely used in practice. |
| `R` | Relocatable | Relocatable object code (produced by some assemblers). |
| `AA` | — | New/Appended type used by some tools. |

Think of the letter as the file extension you never had to type.

Two extra flags you may see before the type letter:

- `*` — the file is **locked** (can't delete or overwrite until you `UNLOCK`)
- ` ` (space) — normal, unlocked

Example:
```
*A 002 HELLO        <- locked Applesoft program, 2 sectors
 B 034 COPY.OBJ0    <- unlocked binary, 34 sectors
```

---

## 5. Fun Things to Try on the System Master

If you booted the **"DOS 3.3 System Master - 680-0210-A.dsk"**, you're on the official Apple master disk. It has a bunch of demos, games, and utilities. The actual catalog varies, but here are things commonly on it that you can try.

**First step — verify what's really there.** Never trust a memory; always check:

```
]CATALOG
```

Common entries (type these at the `]` prompt):

- `RUN APPLESOFT` — the boot "Applesoft" program (often a demo/menu)
- `RUN COLOR DEMOSOFT` — colorful lo-res demo
- `RUN APPLE PROMS` — promotional demo
- `RUN BRIAN'S THEME` — plays a melody through the speaker
- `RUN PHONE LIST` — simple address book demo
- `RUN LITTLE BRICK OUT` — Woz-style Breakout clone, classic Apple II game
- `RUN RANDOM` — random number demo
- `RUN RENUMBER` — utility that renumbers the lines of a BASIC program in memory
- `RUN COPY` — disk copy utility (be careful, this writes)
- `BRUN COPYA` — another copy utility (binary)
- `RUN FID` — File Developer, a file manager (the 1980 version of Finder)
- `RUN HELLO` — whatever the disk's greeter is

If a name contains spaces, type it with spaces exactly:
```
]RUN LITTLE BRICK OUT
```

Some programs are games; some are demos; some are utilities. If you run something and it hangs or looks weird, press Ctrl+C to break out, then `NEW` to clear and `CATALOG` to look again.

---

## 6. Mini Programming Tutorial

Let's ramp up through small programs. Type each one and run it.

### Hello world

```
]NEW
]10 PRINT "HELLO WORLD"
]RUN
HELLO WORLD
```

`NEW` clears the old program first. Always a good habit before starting fresh.

### Variables

Numbers don't need declaration. Strings end in `$`.

```
]NEW
]10 LET N = 10
]20 PRINT N * 2
]30 LET NAME$ = "APPLE"
]40 PRINT NAME$
]RUN
20
APPLE
```

The `LET` is optional — `N = 10` also works. You'll see both styles in old code.

### Loops

```
]NEW
]10 FOR I = 1 TO 10
]20 PRINT I
]30 NEXT I
]RUN
1
2
3
4
5
6
7
8
9
10
```

`STEP` changes the increment. Count down:

```
10 FOR I = 10 TO 1 STEP -1
20 PRINT I
30 NEXT I
```

### Input

```
]NEW
]10 INPUT "WHAT'S YOUR NAME? "; N$
]20 PRINT "HI, "; N$
]RUN
WHAT'S YOUR NAME? ? BOB
HI, BOB
```

(Yes, there's a weird extra `?` after the prompt. That's Applesoft. Live with it.)

### Conditional logic

```
]NEW
]10 INPUT "NUMBER? "; N
]20 IF N > 10 THEN PRINT "BIG"
]30 IF N <= 10 THEN PRINT "SMALL"
]40 IF N = 7 THEN PRINT "LUCKY"
]RUN
```

Applesoft's `IF ... THEN` only does one statement on the same line (no multi-line blocks, no `ELSE`). To do more, use `GOTO`:

```
10 IF N > 10 THEN GOTO 100
20 PRINT "SMALL BRANCH"
30 END
100 PRINT "BIG BRANCH"
110 PRINT "STILL IN BIG BRANCH"
```

### Lo-res graphics

```
]NEW
]10 GR
]20 COLOR = 13
]30 PLOT 20, 20
]40 COLOR = 9
]50 HLIN 5,35 AT 10
]60 VLIN 5,35 AT 20
]70 GET A$
]80 TEXT
]RUN
```

- `GR` enters lo-res (40 columns × 40 rows of color blocks)
- `COLOR =` sets the current drawing color (0-15)
- `PLOT x, y` draws a single block
- `HLIN x1,x2 AT y` draws a horizontal line
- `VLIN y1,y2 AT x` draws a vertical line
- `GET A$` waits for one keypress
- `TEXT` returns to text mode

Play with different COLOR values from 0 to 15. Each is a different color (one of which is literally called "magenta" and another "orange").

### Hi-res graphics

```
]NEW
]10 HGR
]20 HCOLOR = 3
]30 HPLOT 0,0 TO 279,159
]40 HPLOT 279,0 TO 0,159
]50 GET A$
]60 TEXT
]RUN
```

- `HGR` enters hi-res (280x160 with 4 lines of text)
- `HCOLOR =` sets color 0-7
- `HPLOT x,y` plots one dot
- `HPLOT x1,y1 TO x2,y2` draws a line

Two X's on the screen!

### Saving your program

```
]SAVE MYPROG
```

Writes the current BASIC program in memory to a new file called `MYPROG`. Next session you can:

```
]LOAD MYPROG
]RUN
```

Or combine them:

```
]RUN MYPROG
```

### Random numbers

```
10 N = INT(RND(1) * 100) + 1
20 PRINT N
```

`RND(1)` returns a float 0–1. Multiply, floor with `INT`, add 1 to get 1–100.

### A tiny guessing game

```
]NEW
]10 HOME
]20 SECRET = INT(RND(1) * 100) + 1
]30 INPUT "GUESS 1-100? "; G
]40 IF G = SECRET THEN PRINT "YES!": END
]50 IF G < SECRET THEN PRINT "HIGHER"
]60 IF G > SECRET THEN PRINT "LOWER"
]70 GOTO 30
]RUN
```

Note: `PRINT "YES!": END` — the colon lets you put two statements on one line.

---

## 7. Cool Tricks & Gotchas for Modern Programmers

### Gotchas

**No lowercase, ever.** Type in caps. The screen can't show lowercase letters on a II+.

**String variables end in `$`.** `NAME$`, `N$`, `A$`. The `$` is part of the name.

**Integer variables end in `%`.** `X%`, `COUNT%`. Range -32768 to 32767, faster than floats. Most code just uses plain variables (floats).

**Only the first two letters of variable names matter** in Applesoft.
`COUNTER` and `COUNTY` are the same variable. `COLOR` is the same as `COLD`. Watch out.

**Reserved words embedded in names break things.** `SCORE` contains `OR`, a keyword. Applesoft *might* parse it as `SC OR E`. This was a famous footgun. Safer name: `SC` or `SCOR`.

**`IF ... THEN` is single-line.** No `ELSE`, no block. Use `GOTO` for branches.

**Array declaration.** `DIM A(10)` creates array A with indices 0 through 10 (that's 11 elements). Same for strings: `DIM NAMES$(20)`.

**Arrays default to size 11.** If you use `A(I)` without `DIM` it's autosized to 11 (0..10). Exceed that and you get `?BAD SUBSCRIPT ERROR`.

**`PEEK` and `POKE` — direct memory access.** `PEEK(addr)` reads a byte. `POKE addr, value` writes one. This is how you talk to hardware.

**DOS commands inside programs need Ctrl-D.** At the prompt, `SAVE` works. Inside a BASIC program, you need:

```
10 PRINT CHR$(4); "SAVE MYPROG"
```

`CHR$(4)` is Ctrl-D, the signal DOS looks for to intercept a command. Without it, DOS lets the `PRINT` go to the screen untouched. This is how BASIC programs use disk.

**No directories.** DOS 3.3 is flat. One disk, one list of files. No folders.

**Disks are 140K.** That's 143,360 bytes. 35 tracks × 16 sectors × 256 bytes. Plan accordingly.

### Tricks

**`?` is `PRINT`.** Save your fingers: `?2+2` works.

**`PR#6`** — reboot from slot 6 (the disk drive). Same as cold booting.
**`PR#0`** — send output back to the 40-column screen.
**`PR#1`** — send output to the printer in slot 1 (if configured).
**`IN#0`** — read input from the keyboard (default).

**`HIMEM:` and `LOMEM:`** set the memory bounds available to BASIC. Move HIMEM down before using the space above it for machine code or data.

**`FRE(0)`** returns how many bytes of free memory you have. Good sanity check.

**`POP`** pops the top GOSUB return off the stack. Useful for clean exits from subroutines via GOTO.

**`VTAB n` / `HTAB n`** — move the text cursor to row n (1-24) / column n (1-40). Like ANSI cursor moves.

**`PEEK(-16384)`** — check for a keypress without waiting. Bit 7 is set when a key is pressed. This is `$C000` in hex but `PEEK` wants signed numbers.

**`POKE -16368, 0`** — clear the keyboard strobe (`$C010`). After handling a key, poke this to acknowledge.

**`CALL -936`** — same as `HOME` (clear screen).
**`CALL -958`** — clear from cursor to end of line.
**`CALL -868`** — clear from cursor to end of screen.
**`CALL -1184`** — print the prompt `]`.

### Common POKE/PEEK addresses worth knowing

| Decimal | Hex | What |
|---|---|---|
| -16384 | $C000 | Read: keyboard register (bit 7 = key ready) |
| -16368 | $C010 | Write: clear keyboard strobe |
| -16336 | $C030 | Read: click the speaker |
| -16304 | $C050 | Graphics mode on |
| -16303 | $C051 | Text mode on |
| -16302 | $C052 | Full screen (no text at bottom) |
| -16301 | $C053 | Mixed mode (text at bottom) |
| -16300 | $C054 | Show page 1 |
| -16299 | $C055 | Show page 2 |
| -16298 | $C056 | Lo-res mode |
| -16297 | $C057 | Hi-res mode |

Click the speaker 200 times for a beep:

```
10 FOR I = 1 TO 200
20 X = PEEK(-16336)
30 NEXT I
```

Each `PEEK(-16336)` toggles the speaker diaphragm. Vary the delay in the loop to change pitch.

### Entering / leaving the monitor

From BASIC:
```
]CALL -151
*
```

From monitor back to BASIC:
```
* 3D0G
]
```

`3D0G` means "jump to address $03D0". That's a known location containing `JMP warmstart` to return to BASIC without clobbering your program.

---

## 8. Where to Learn More

The real classics. Most are freely available online as scans.

**Books:**

- *"Applesoft BASIC Programmer's Reference Manual"* (Apple, 1982) — the official language manual. Clear, thorough, full of examples.
- *"Apple II Reference Manual"* (Apple, 1979) — the full Apple II+ hardware manual. Schematics, memory map, monitor ROM listing.
- *"Beneath Apple DOS"* by Don Worth and Pieter Lechner (1981) — legendary. Explains how DOS 3.3 actually works inside, sector by sector, byte by byte. If you ever want to dig into RWTS, disk format, or write your own DOS tools, start here.
- *"Apple Machine Language"* by Don Inman and Kurt Inman — gentle intro to 6502 and the Apple II monitor.
- *"What's Where in the Apple"* by William F. Luebbert — exhaustive memory map reference.

**Online:**

- **Apple II Documentation Project** (mirrored at `apple2.org.za` and elsewhere) — PDFs of original manuals.
- **Internet Archive** — hundreds of Apple II disk images and scanned manuals.
- **Asimov / apple.asimov.net** — the canonical archive of Apple II software.
- **6502.org** — best resource for 6502 assembly if you want to go deep.

---

## 9. Quick Reference Card

A one-page cheat sheet. Print it out, tape it to your monitor, pretend it's 1982.

### Disk (DOS 3.3)
```
CATALOG                list files
LOAD name              load BASIC program
SAVE name              save BASIC program
RUN  name              load + run
DELETE name            delete file
RENAME old,new         rename
LOCK name / UNLOCK name
BSAVE name,A$addr,L$len     save raw bytes
BLOAD name[,A$addr]         load raw bytes
BRUN name                    run binary
INIT HELLO             format a blank disk
```

### BASIC
```
NEW                    erase program
LIST [a[,b]]           show program
RUN [line]             run program
STOP / END / CONT      control
HOME / TEXT / GR / HGR screen modes
PRINT (or ?) expr      print
INPUT "?"; A$          ask user
IF cond THEN stmt      conditional
FOR I=1 TO N ... NEXT  loop
GOTO n / GOSUB n / RETURN
DIM A(10)              array
REM comment            comment (or use ')
LET x = 5              assign (LET optional)
```

### Editing programs
```
10 PRINT "HI"   adds / replaces line 10
10              deletes line 10
LIST 10,50      show range
```

### Screen & graphics
```
HOME           clear screen
VTAB n / HTAB n   cursor to row/col
INVERSE / FLASH / NORMAL
GR / COLOR=c / PLOT x,y / HLIN / VLIN
HGR / HCOLOR=c / HPLOT x,y / HPLOT ... TO ...
TEXT           back to text
```

### Monitor (the `*` prompt)
```
CALL -151      enter monitor from BASIC
3D0G           return to BASIC
300            show byte at $0300
300.310        show bytes $0300-$0310
300:A9 41 60   write bytes at $0300
300G           go (execute) at $0300
300L           disassemble at $0300
```

### Keys & reboot
```
Ctrl-C         break out of a program
Reset          reset the machine
PR#6           reboot from slot 6 (disk)
PR#0           screen output
?              shorthand for PRINT
CHR$(4)        Ctrl-D — DOS command marker (use in programs)
```

### Useful POKE/PEEK
```
PEEK(-16384)         keyboard: bit 7 = key ready
POKE -16368,0        clear keyboard strobe
PEEK(-16336)         click speaker
POKE -16304,0        graphics on
POKE -16303,0        text on
FRE(0)               free bytes in BASIC
```

### File types in CATALOG
```
A  Applesoft BASIC    I  Integer BASIC
B  Binary             T  Text
S  System             *  locked
```

---

That's it. You know enough to be dangerous. Go write a game, corrupt some memory, beep the speaker, save a program, delete it by accident, recover by being more careful next time. This is how everyone in 1982 learned.

Welcome to the Apple II.

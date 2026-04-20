# apple2emu

An Apple II / II+ emulator written in Go. CPU, video, speaker, Disk II,
and Language Card (16 KB RAM at $D000–$FFFF with $C080–$C08F bank switching).

## Build & run

```sh
go build ./...
go run .                                    # defaults: Apple2_Plus.rom, no disks
go run . -disk1=disks/dos33.dsk             # boot DOS 3.3
go run . -disk1="games/Ultima IV (4am crack) side A - Program.dsk" -paddle=off
```

## Command-line flags

| Flag           | Default                  | Description                                                                |
|----------------|--------------------------|----------------------------------------------------------------------------|
| `-rom`         | `roms/Apple2_Plus.rom`   | Apple II ROM image (12 KB at $D000 or 16 KB at $C000).                     |
| `-disk1`       | *(empty)*                | Disk image for drive 1 (`.dsk` / `.do` / `.po`).                           |
| `-disk2`       | *(empty)*                | Disk image for drive 2.                                                    |
| `-order`       | *(infer from extension)* | Sector order override: `dos` or `prodos`.                                  |
| `-volume`      | `0.25`                   | Speaker volume in `[0.0, 1.0]`.                                            |
| `-samplerate`  | `44100`                  | Audio sample rate in Hz.                                                   |
| `-paddle`      | `arrows`                 | `arrows` = arrow keys drive virtual paddle. `off` = arrows act as cursor keys (Apple II codes $08/$15/$0B/$0A). Use `off` for Ultima, Wizardry, AppleWorks, etc. |
| `-disktrace`   | `false`                  | Log disk activity to stderr.                                               |
| `-disktracen`  | `200`                    | Max nibble-read lines to emit when `-disktrace` is set. `0` = unlimited.   |

## Keyboard

### System

| Key          | Action                                                            |
|--------------|-------------------------------------------------------------------|
| `Shift+Esc`  | Quit. (Plain `Esc` is forwarded to the Apple II as `$1B` — Ultima IV, Wizardry, AppleWorks and many others need it.) |
| `Cmd+Q` / close window | Quit (macOS).                                           |
| `Ctrl+R`     | Reset (CPU + Language Card).                                      |
| `F12` (hold) | Turbo mode — bypasses 60 fps frame cap while held.                |

### Disk

| Key / Gesture         | Action                                                                |
|-----------------------|-----------------------------------------------------------------------|
| Drag `.dsk`/`.do`/`.po` onto **left half** of window  | Mount / swap in drive 1.         |
| Drag onto **right half** of window                    | Mount / swap in drive 2.         |
| `Shift` + drop        | Force drive 2 (within-app drags where keyboard focus is reliable).    |
| `Ctrl+1`              | Eject drive 1.                                                        |
| `Ctrl+2`              | Eject drive 2.                                                        |

After swapping a disk mid-session you may need `Ctrl+R` to re-run the slot-6 scan for a cold boot.

### Apple II keyboard passthrough

Your host keyboard maps to the Apple II keyboard latch. The Apple II is uppercase-only; lowercase is auto-upshifted.

| Host key            | Apple II code  | Notes                                     |
|---------------------|----------------|-------------------------------------------|
| Letters / digits    | ASCII          | Auto-uppercase.                           |
| `Shift` + key       | Shifted ASCII  | Standard US layout.                       |
| `Ctrl+A`…`Ctrl+Z`   | `$01`…`$1A`    | Control codes.                            |
| `Return` / `Enter`  | `$0D`          |                                           |
| `Backspace`         | `$08`          | Same code as Apple II left-arrow.         |
| `Delete`            | `$7F`          |                                           |
| `Tab`               | `$09`          |                                           |
| `Esc`               | `$1B`          | Use `Shift+Esc` to quit the emulator.     |

### Arrow keys

Mode is set by `-paddle`:

**`-paddle=arrows` (default)** — arrows drive the virtual paddle axes (for joystick games like Choplifter, Karateka).

| Key     | Paddle axis   |
|---------|---------------|
| `←`     | Paddle 0 → 0 (full left)   |
| `→`     | Paddle 0 → 255 (full right) |
| `↑`     | Paddle 1 → 0               |
| `↓`     | Paddle 1 → 255             |

**`-paddle=off`** — arrows are sent as Apple II cursor codes (for keyboard-driven games like Ultima, AppleWorks).

| Key     | Apple II code | Equivalent |
|---------|---------------|------------|
| `←`     | `$08`         | Ctrl+H / backspace |
| `→`     | `$15`         | Ctrl+U |
| `↑`     | `$0B`         | Ctrl+K |
| `↓`     | `$0A`         | Ctrl+J |

### Joystick buttons

| Key          | Apple II button                |
|--------------|--------------------------------|
| `Left Alt`   | Button 0 — Open-Apple (fire 1) |
| `Right Alt`  | Button 1 — Closed-Apple (fire 2) |

## Window title

The title bar reports frame rate and disk activity:

```
Apple II Emulator — 60 fps — [D1:dos33 ●] [D2:karateka ◉]
```

- `●` — recent read from that drive
- `◉` — recent write
- `TURBO` replaces the fps field while `F12` is held
- Labels are truncated to 8 characters

## Language Card

16 KB RAM is always installed in slot 0, overlaying ROM at `$D000–$FFFF`:

- 4 KB bank 1 + 4 KB bank 2 at `$D000–$DFFF` (one at a time)
- 8 KB shared at `$E000–$FFFF`
- 16 softswitches at `$C080–$C08F` control read-select, write-enable, and bank selection
- Two-consecutive-read write-enable quirk implemented
- Cold/warm reset returns the card to ROM-read / bank 2 / write-disabled

## Project layout

```
bus/          Memory-mapped bus (last-mapping-wins dispatch)
cpu/          6502 CPU
memory/       RAM + ROM
io/           $C000-page softswitches (keyboard, video modes, speaker strobe, paddles)
speaker/      $C030 click speaker + audio mixing
video/        Text / Lores / Hires rendering
disk/         Disk II controller + .dsk/.do/.po loading + nibble GCR
languageCard/ 16 KB Language Card at $D000–$FFFF
main.go       SDL2 window, event loop, flag parsing, wiring
```

## Testing

```sh
go test ./...
```

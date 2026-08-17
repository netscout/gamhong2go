# gamhong2go

Yet another Apple II emulator, this one in Go.

6502 CPU, video, speaker, Disk II, and a Language Card (16 KB RAM at `$D000–$FFFF`
with `$C080–$C08F` bank switching) — enough to boot DOS 3.3 and play Karateka.

### Why "gamhong2go"?

It's "Apple II in Go", with the apple swapped for a Korean one.

| Piece     | Meaning                                                                 |
|-----------|-------------------------------------------------------------------------|
| `gamhong` | 감홍 — a premium Korean apple cultivar. Very sweet, very crisp, and priced accordingly. |
| `2`       | The `II` in Apple II.                                                   |
| `go`      | The language it's written in.                                           |

Say it quickly and it also comes out as "gamhong, to go."

## Prerequisites

You need two things before the first build: **Go 1.21+** and **SDL2** (plus `pkg-config`).

Why SDL2? The emulator's window, keyboard, and sound all go through SDL2, which is a
C library — not Go code. Go reaches it through cgo, and cgo asks a small helper program
called `pkg-config` where SDL2's headers and libraries live on your machine. If either
one is missing, the build stops before it compiles a single line:

```
github.com/veandco/go-sdl2/sdl: exec: "pkg-config": executable file not found in $PATH
```

### macOS (Homebrew)

```sh
brew install pkg-config sdl2
```

Homebrew's `sdl2` formula now installs **sdl2-compat** — the SDL2 API implemented on top
of SDL3. That is what go-sdl2 expects, so nothing extra is needed.

### Linux

```sh
sudo apt install pkg-config libsdl2-dev        # Debian / Ubuntu
sudo dnf install pkgconf-pkg-config SDL2-devel # Fedora
sudo pacman -S pkgconf sdl2                    # Arch
```

### Windows (MSYS2)

```sh
pacman -S mingw-w64-x86_64-pkg-config mingw-w64-x86_64-SDL2
```

Build from the MinGW64 shell so the toolchain and `pkg-config` are on `PATH`.

### Check that it worked

```sh
pkg-config --modversion sdl2
```

A version number (e.g. `2.32.70`) means you're ready. `Package sdl2 was not found`
means SDL2 is installed somewhere `pkg-config` doesn't look — point `PKG_CONFIG_PATH`
at the directory holding `sdl2.pc`.

Only the core `go-sdl2/sdl` package is used, so you do **not** need `sdl2_image`,
`sdl2_mixer`, or `sdl2_ttf`.

### ROMs and disk images

This repo ships **no ROMs, disk images, or games**, and gives no download links. They are
copyrighted by their owners; the Apple II community's preservation archives keep them alive,
and finding copies you have the right to use is up to you.

That's why `roms/`, `disks/`, and `games/` are gitignored — you create them and drop your
files in. What this section gives you instead: the exact file names the emulator expects,
plus byte sizes and MD5 checksums, so you can verify that what you found is the right thing.

```sh
md5 roms/* disks/*   # md5sum on Linux
```

#### ROMs

Two ROMs matter:

- The **system ROM** — picked with `-rom`, default `roms/Apple2_Plus.rom`. The emulator
  can't boot without it. A 12 KB image maps at `$D000`, a 16 KB image at `$C000` — the
  size decides, no flag needed.
- The **Disk II controller PROM** — loaded from the fixed path `roms/DISK2.rom` whenever
  a disk is mounted. No PROM, no disk boot.

Known-good images:

| File in `roms/`   | Bytes | MD5                                |
|-------------------|-------|------------------------------------|
| `Apple2_Plus.rom` **(default)** | 12288 | `572b3005a4fa49bc54917b069b82c1ab` |
| `Apple2.rom`      | 12288 | `3c406514b9806a7c57ee65fb0b0c39b4` |
| `Apple2e.rom`     | 16384 | `346bc782c6a08a531c460e33bc03daf4` |
| `DISK2.rom`       |   256 | `2020aa1413ff77fe29353f3ee72dc295` |

#### Disk images

One image is verified to boot with this emulator — the DOS 3.3 System Master, the
January 1983 revision:

| File in `disks/` | Bytes  | MD5                                |
|------------------|--------|------------------------------------|
| `DOS_3_3.dsk`    | 143360 | `b13de32fd7a97d817744bf2dd71d5479` |

Archives publish it under its own name (usually mentioning *January 1983*), but every
example in this README says `-disk1=disks/DOS_3_3.dsk` — so rename your copy to match.

Other DOS 3.3 masters and ProDOS images in `.dsk` / `.do` / `.po` format should load the
same way, but they aren't part of this README's tested set.

#### Games

`games/` is yours to fill — nothing in the build depends on it. The Karateka and Ultima IV
examples in this README refer to cracked images that circulate in Apple II community archives.

Both are commercial games still under copyright. Karateka in particular is sold today as part of
*The Making of Karateka*, which includes the original Apple II release. Use copies you have the
right to use.

## Build & run

```sh
go build ./...
go run .                                    # defaults: Apple2_Plus.rom, no disks
go run . -disk1=disks/DOS_3_3.dsk           # boot DOS 3.3
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

**On macOS, `Alt` is the `⌥ Option` key** — same physical key, different label. Use the
Option keys either side of the space bar, not `⌘ Command` (`Cmd+Q` quits).

| Key                          | Apple II button                  |
|------------------------------|----------------------------------|
| `Left Alt` / `⌥ Option` (left)  | Button 0 — Open-Apple (fire 1)   |
| `Right Alt` / `⌥ Option` (right) | Button 1 — Closed-Apple (fire 2) |

Fitting, historically: these are the Apple II's Open-Apple and Closed-Apple keys, the
ancestors of the Mac's Command and Option keys.

## Game notes

### Karateka

Karateka is a joystick game, so the **default** `-paddle=arrows` mode is already correct —
you don't need to pass any flag:

```sh
go run . -disk1="games/Karateka (The Racketeers Crack).dsk"
```

Do **not** add `-paddle=off` here. That switches the arrow keys over to text cursor codes,
and the game then sees no joystick movement at all.

| Host key      | What the Apple II sees        | In game        |
|---------------|-------------------------------|----------------|
| `←`           | Paddle 0 → 0 (full left)      | Move / turn    |
| `→`           | Paddle 0 → 255 (full right)   | Move / turn    |
| `↑`           | Paddle 1 → 0 (full up)        | Aim high       |
| `↓`           | Paddle 1 → 255 (full down)    | Aim low        |
| *release arrow* | that axis → 128 (center)    | Stop           |
| `Left Alt` (`⌥` on Mac)  | Button 0 (`$C061`) | Punch |
| `Right Alt` (`⌥` on Mac) | Button 1 (`$C062`) | Kick  |

Attacks are the usual joystick-game combination: hold a direction to pick the height, then
press a button to strike. The exact move list belongs to the game, not the emulator — the
table above is what the emulator feeds it.

Three details make this actually playable:

- **Paddles start centered** at 128, so the fighter stands still until you press something.
  A real Apple II joystick left off-center would have him walking at boot.
- **Releasing an arrow re-centers that axis.** Without this the character would keep walking
  forever, because a joystick has no "key up" — it just sits wherever you left it. If you're
  holding `←` and `→` together and let go of one, the other direction takes over instead of
  snapping to center.
- **Losing window focus releases both buttons and re-centers both axes**, so switching apps
  mid-fight doesn't leave you stuck running or throwing a punch.

One limitation: a real Apple II joystick is analog, but arrow keys are only on or off. You
always get full deflection — there is no half-speed walk.

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

## License

[MIT](LICENSE). This covers the emulator's code only — ROMs, disk images, and games
stay under their own owners' copyright, which is why the repo doesn't ship them.

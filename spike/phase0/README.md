# Phase 0 spike

Validates the blocking assumptions in [`DESIGN.md`](../../DESIGN.md) §14 with a small
CLI, before any UI or product code exists.

It is **read-only by default**. Only `press` and `watch -fire -yes` inject input.

## Build

```sh
go build -o wami-spike ./spike/phase0
```

## Commands

Run from the repo root with the game windowed/borderless and visible.

```sh
# report backend, screen size, displays, active X window title, spot list
./wami-spike probe

# KDE Wayland only: authorize fast capture (KWin ScreenShot2) for this binary path.
# Run once per binary location, after building. Falls back to the slow portal without it.
./wami-spike setup-kwin

# calibrate: for each key, Alt-Tab to the game, hover the pointer over the CENTER of
# that prompt box, and wait for the countdown. The position is read while you are in
# the game (not after Alt-Tabbing back), so it cannot capture the terminal instead.
./wami-spike calibrate

# pointer tracker (target-local readout) to eyeball that it follows the mouse
./wami-spike selftest -seconds 8

# save full.png + one PNG per spot into out/ (default spike/phase0/out)
./wami-spike capture -out /tmp/wami-phase0

# live per-spot score table, no injection (10-20 fps with the KWin fast path)
./wami-spike watch -seconds 10 -fps 10

# live table + inject the mapped key on detection (focus the game first)
./wami-spike watch -fire -yes -fps 10

# inject a single key after a countdown, to prove keys reach the game
./wami-spike press -key a -delay 3 -yes
```

## Calibrating the spots

A **spot** is a rectangle `(x, y, w, h)` in screen pixels around where a prompt box
appears. Only the center matters; the box size defaults to 180×56.

Recommended: `./wami-spike calibrate`. For each key (`a, w, d, s`) it runs a countdown
while you:

1. Alt-Tab to the game and hover the pointer over the **center** of that prompt box.
2. Wait there — the position is read at the end of the countdown, **while you are still
   in the game**, so it can never capture the terminal by mistake.
3. Alt-Tab back and press Enter to move to the next spot.

It records the pointer position, builds the ROI, and writes `spike/phase0/spots.json`.
Then verify with `capture` (open the crops) and `watch` (live scores). Use `-delay 5`
for more time, and `-w`/`-h` if the prompt box is larger than 180×56.

Manual alternative: edit `spots.json` directly. `x`/`y` are the top-left corner of the
box, `w`/`h` its size. Two ways to read coordinates:

- `./wami-spike selftest -seconds 20` prints the live pointer position in **target-local**
  coordinates — hover the top-left and bottom-right corners and read the numbers.
- KDE `kdotool getmouselocation` reports **global** coordinates; subtract the capture
  offset printed by `probe` (e.g. global x 3155 − 1920 = target-local 1235).

## What each result answers (DESIGN §14)

| Command | Phase 0 question |
|---|---|
| `probe` | capture backend, display geometry, active window title (items 1, 5) |
| `capture` | capture works and each ROI frames a prompt box (item 1, 3) |
| `watch` | per-spot detection reliability; which spot carries which key (items 3, 6) |
| `press` / `watch -fire` | keys reach the Proton/Steam window (item 2) |
| (visual) | prompt-visible → key-landed latency (item 4) |

## Coordinates

Everything is relative to the **target monitor** (default: the primary), with `(0,0)` at
its top-left. `capture` saves only that monitor, and `calibrate` translates the pointer
into the same space, so the two line up.

On a Wayland session, the fallback capture (`org.freedesktop.portal.Screenshot`) returns
the **whole desktop** and starts at the layout's top-left, not the primary. That offset
(the `virtual origin` printed by `probe`) is applied automatically. On KDE, the **fast
path** (`org.kde.KWin.ScreenShot2`) uses the same global coordinate space and captures a
region directly.

The built-in spots are the approximate 1080p centers from DESIGN Appendix A and are
**not shipped truth** — run `calibrate`.

## Fast capture on KDE Wayland

`kbinani`/portal captures cost ~500 ms/frame (~2 fps). KWin's `ScreenShot2` captures a
spot union in **~2 ms** and the whole monitor in ~43 ms, but KWin only authorizes a
process whose `.desktop` has `X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2`
and whose `Exec` matches the binary. `setup-kwin` writes that file for the current binary
path and rebuilds the KDE service cache. Run it once per binary location:

```sh
go build -o bin/wami-spike ./spike/phase0
./bin/wami-spike setup-kwin
./bin/wami-spike probe    # capture backend should read "kwin screenshots2"
```

`go run` is not authorized (its executable path is temporary); build first.

## Findings so far (Kubuntu/Plasma Wayland, 5360×1440)

- **Key injection works** through `robotgo/x11` XTEST into the Proton game.
- **Capture**: KWin `ScreenShot2` is the fast path (~2 ms/region). Without authorization
  it falls back to the portal (~2 fps). kbinani's X11 path (`XGetImage` via XWayland)
  is all-black under Wayland.
- `robotgo/x11` reads screen size, display count (`2`), and the active X window title, so
  the WAMI window is reachable through XWayland.
- XTEST synthetic pointer motion is ignored by the compositor, so move-and-click cannot
  reposition the pointer on Wayland; the key action is the reliable one.
- Windows cross-build (`GOOS=windows go build ./...`) succeeds; the Windows backend is a
  stub pending the Windows Phase 0 pass (`source_other.go`).

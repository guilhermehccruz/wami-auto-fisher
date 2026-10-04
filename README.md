# WAMI Auto Fisher

Automates the **active fishing** minigame in *Wizard and Minion Idle* (WAMI).

While fishing, a prompt box appears about once a second at one of four fixed
spots, each asking for a key (`W`, `A`, `S`, `D`). The app watches those four
spots on screen and presses the right key. It has no calibration, no global
hotkeys and no pointer interaction — just a **Start** and a **Stop** button.

- **Windows 10** and **Kubuntu (KDE Plasma)**; the game runs under Steam/Proton.
- One executable per OS, no installer.
- If no prompt is detected for **10 s**, the run stops on its own.

## Build

Requirements: **Go 1.27**, **Node 22**, the **Wails v3 CLI**, and (Linux only)
GTK3 + WebKit2GTK-4.1 dev packages.

```sh
# Linux: GTK3/WebKit2GTK-4.1 headers (Ubuntu/Debian)
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev

# Wails v3 CLI (Linux builds use the GTK3 backend)
go install -tags gtk3 github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27

wails3 build                 # Linux  -> bin/wami-auto-fisher
wails3 build GOOS=windows    # Windows -> bin/wami-auto-fisher.exe
```

`wails3 build` compiles the frontend, generates bindings and embeds the assets.
Run tests with `go test -tags gtk3 ./...`.

## Run

```sh
./bin/wami-auto-fisher
```

1. Start the game and click **START** (active fishing).
2. In the app, click **Start**.
3. Alt-Tab to the game. To stop, Alt-Tab back and click **Stop** (or wait for
   the 10 s idle auto-stop).

### KDE fast capture (Linux only)

On KDE Wayland the app can capture a small region at high speed via KWin. It
needs a one-time authorization per binary path: click
**"Authorize fast KWin capture"** in the app. Without it the app falls back to
the desktop portal, which is much slower (~2 fps). The section is hidden on
Windows.

## Fixed coordinates

There is **no calibration in the app**. The four spot rectangles are fixed
constants (target-monitor pixels, origin at the primary monitor's top-left):

| key | ROI `(x, y, w, h)` |
|---|---|
| `a` | `969, 581, 109, 42` |
| `w` | `1102, 506, 111, 43` |
| `d` | `1235, 580, 109, 43` |
| `s` | `1102, 655, 109, 41` |

These are used on **both Linux and Windows**, measured with the game window at
the primary monitor's top-left. **If the game window moves or is resized, the
boxes no longer line up** — put it back, or update `fixedSpots` in `service.go`
and rebuild. The live `white`/`blue` readout per spot shows whether each box
still frames a prompt (the active one highlights green).

Detector tunables (poll interval, thresholds, idle timeout) are editable in the
app and saved to `~/.config/wami-auto-fisher/config.json` (Linux) or
`%AppData%\wami-auto-fisher\config.json` (Windows).

## Notes

- Play in **borderless-windowed**. Exclusive fullscreen can return a black
  capture, so the app would see nothing.
- No global hotkeys: stopping is the on-screen **Stop** button (or the idle
  auto-stop).
- Single-instance: launching the app again brings the running window forward.
- Only one instance should drive input at a time.
- This is for a single-player idle game. Automating online games may violate
  their rules; you assume the risk.

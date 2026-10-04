# WAMI Auto Fisher — Design Document

Automates the **active fishing minigame** in *Wizard and Minion Idle* (WAMI) on
**Windows 10 and Kubuntu (KDE Plasma)**. It watches the four spots where the
"press the key" prompt can appear, decides **which** one is up from the screen itself,
and presses the mapped key (`W`, `A`, `S`, `D`). The game stays focused during a run;
the app is configured beforehand and controlled with global hotkeys. One executable per
OS, no installer, no runtime deps beyond the OS webview.

**Status:** design phase. The input-injection, global-hotkey and storage layers rest on
techniques already validated on Kubuntu/Plasma Wayland: pure-Go, cgo-free robotgo
backends (`x11`/`libei`/`win`), a global-shortcut layer for Windows/X11/Wayland, and
atomic JSON storage. Those layers are specified here in their own right; this app adds
one new subsystem on top: **vision** (screen capture + prompt detection). Sections
marked **[unproven]** need a short spike on the real game before implementation
(see [§14](#14-phases)).

**Self-contained by design.** This document is the full specification for this app and
does not depend on any other document or repository. The platform notes in
[Appendix B](#appendix-b--platform-notes-worth-not-rediscovering) are hard-won,
silent-failure knowledge and are carried here in full so they are not lost. The input
`Driver`, global-hotkey sources, atomic JSON storage, single-instance lock, logging and
Wails shell are all part of this project's own codebase. The default action is a
keyboard key, which needs no pointer at all.

---

## 1. The problem

### 1.1 What the game does

Fish are caught with an **active** minigame. The player clicks a `START` button; from
then on, roughly once per second, a prompt box pops up over the fishing scene reading
**"Click or Press <key>"**. There are four prompt spots and four keys — **`W`, `A`,
`S`, `D`**, one per spot:

| Spot | Key | Screen area (approx, 1080p) |
|---|---|---|
| S1 | `A` | left-of-water, mid height |
| S2 | `W` | over trees, mid height |
| S3 | `D` | far right, mid height |
| S4 | `S` | lower middle-right |

*(The screenshots show the boxes reading `A/Q` and `Z/W` — presumably the game also
accepts the AZERTY-style second key. This app standardizes on **WASD** and presses only
the primary key. Actual pixel values come from the user's calibration, not from a
shipped constant; see [§5](#5-calibration). Which spot carries which key is confirmed
during calibration.)*

Pressing the commanded key reduces the fish's stamina. A **wrong key** fails that fish
and the game immediately throws **another random prompt**; letting the prompt time out
does the same. So a wrong press is not session-ending, but it still throws away the
catch and the time spent on it. If nothing is pressed for 50 s the game drops into idle
fishing. Active fishing earns mastery far faster than idle, which is the whole point of
automating it.

### 1.2 Why this cannot be a blind clicker

The prompt is **random among four spots every time**. Blindly clicking all four spots
(or spamming all four keys) is wrong for two reasons: the extra presses are themselves
wrong keys (each fails the fish and spawns a fresh random prompt), and three of the four
spots sit near interactive UI (`START`/`STOP`, the bait list) that blind clicks can hit.
A wrong press now *recovers* on its own, so the penalty is a lost fish rather than a
dead session — but the app should still read the screen and press the right key. That is
the one hard requirement a click-only tool never had.

### 1.3 What automating it actually buys, and the honesty tax

The minigame is designed to be *actively* played; the developer explicitly tolerates
automation of repetitive tasks (see the WAMI community notes). Still, the README must
say the obvious: this is for a single-player idle game, and the user assumes any risk
with the platform's rules. This is not an aimbot; it reads a white box and presses a
key.

---

## 2. Goals & Non-Goals

### Goals
- Watch up to **four fixed prompt spots** by screen capture; classify **which** (if
  any) holds a prompt; press that spot's key.
- React fast enough that the fish almost never escapes (target: act within ~50 ms of a
  prompt appearing; prompts live ~1 s).
- Never press a key when the screen is ambiguous — a missed press just means one
  stamina tick; a wrong press loses the fish.
- Cross-platform: **Windows 10** and **Kubuntu (KDE Plasma)** only, with the game
  running under **Steam / Proton**.
- Dead-simple to use: calibrate once, press Start, walk away. The **game is always the
  focused window** while fishing, so control during a run is via global hotkeys.
- Ship as one binary per OS.

### Non-Goals
- Reading the game's memory, injecting JS into the game, or any process tampering.
- OCR of the prompt text: the **spot** identifies the key, so pixels suffice
  ([§4](#4-detection)).
- Automating other WAMI activities (combat, spells, minion lab, …).
- Working with WAMI in a mobile browser. Target OSes are Windows 10 and Kubuntu (KDE
  Plasma) only; macOS and other Linux desktops are out of scope.
- A general macro/vision framework — it is tuned to this one minigame.
- Multi-fish/multi-account parallelism. One game window at a time.

---

## 3. Stack

Well-established, cgo-free foundations, plus one capture library.

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.26 | single binary, cross-compile, `CGO_ENABLED=0` |
| Input | `github.com/go-vgo/robotgo` (pinned master) | pure-Go `x11`/`libei`/`win` sub-packages behind the `Driver` interface |
| Capture | `github.com/kbinani/screenshot` | pure Go on Windows + Linux/X11; region capture; see [§8](#8-screen-capture) |
| Image processing | pure Go (stdlib `image`, hand-rolled NCC) | no OpenCV, no cgo |
| Shell / IPC | Wails v3 | native WebView2 (Win) / WebKitGTK (Linux), auto TS bindings |
| UI | React + TypeScript + Tailwind | shared component vocabulary |
| Storage | JSON, one file per profile | atomic, field-preserving codec |
| Packaging | Wails build | `linux/amd64`, `windows/amd64` from one machine |

The pure-Go, cgo-free robotgo backends exist only on `master`, pinned to this exact
pseudo-version:

```
github.com/go-vgo/robotgo v1.0.3-0.20260921150940-12f16b7c5d82  // master @ 2026-09-21
```

The same discipline applies to the Linux Wails backend (`-tags gtk3`,
GTK3/WebKit2GTK-4.1) and the CI/deploy shape.

### 3.1 Why keyboard press is the default action

The user chose keyboard press. Reasons it is the right default here:
- **No pointer involvement.** Nothing to move, so no risk of a stray click on `STOP`
  or the bait list, and no dependence on the hairy Wayland absolute-motion work
  (`libei` + ScreenCast stream) that absolute-cursor clicking requires. Key injection is
  the well-trodden path on all three backends.
- **No cursor-position read.** The feature that most constrains absolute-cursor input —
  `CursorPos()` being unavailable under `libei` — simply does not apply.
- **Focus requirement is a non-issue.** The game is the foreground window while
  playing; keys go to it.

**Mouse-click action is still supported** (per-spot configurable), for cases where the
game window cannot hold keyboard focus or the user prefers it. It reuses the `ClickAt`
path and its Wayland capability gating. Click mode is the fallback, not the default.

---

## 4. Detection

### 4.1 The signal

The prompt is visually distinctive and stable: a **near-white filled box with a
double blue border** and dark text, rendered at one of four fixed positions over a
scene whose background (trees, water, the fisherman) is greenish/blue and much darker
than the box. That white box is the signal; the text inside is irrelevant because the
position already identifies the key.

### 4.2 Regions of interest (ROIs)

Each spot owns a small rectangular **ROI** (default ~180×56 px at 1080p, tight around
the prompt box). ROIs are captured relative to the **game window's client area**, so
the window can move without invalidating calibration ([§7](#7-window-binding-and-coordinates)).
At runtime the app captures the **union bounding box of all enabled ROIs once per
poll** and crops the four sub-regions — one capture call per tick, not four.

### 4.3 Classifier

Per ROI, from the captured RGBA:

| Metric | Definition | Discriminates |
|---|---|---|
| `whiteFrac` | fraction of pixels with HSV `V ≥ 0.80` and `S ≤ 0.20` | the white box fill |
| `blueFrac` | fraction of pixels in the border-blue hue band | the double border |
| `diffFrac` | fraction of pixels differing from the stored *empty* reference by > Δ | motion / occlusion |

```
score = whiteFrac + 0.25*blueFrac
```

A spot is a **candidate** when `whiteFrac ≥ whiteThreshold` (default `0.35`). The
runner picks the candidate with the highest score, but only if the **ambiguity
margin** is satisfied:

```
best.score ≥ whiteThreshold
best.score ≥ runnerUp.score * margin        # margin default 1.25
```

If the margin fails (e.g. a banner or a bright animation touches two ROIs), the poll is
**ignored** rather than guessed. Since a prompt persists for ~1 s and polls run at
~30 Hz, the next frame almost always resolves cleanly; ignoring one frame costs
nothing, a wrong key loses the fish.

**Optional template mode.** For stubborn backgrounds, calibration can store a grayscale
crop of each prompt and classify by normalized cross-correlation (NCC ≥ `templateThreshold`,
default `0.80`). Pure Go, tiny templates, cheap. `brightBox` is the default; template
mode is an escape hatch.

### 4.4 Why not the obvious alternatives
- **OCR the text** — heavier, needs a font model, and unnecessary: position ⇒ key.
- **Whole-scene template matching** — more data to capture and compare for the same
  answer; four small ROIs are cheaper and tighter.
- **Rolling background subtraction alone** — the water animates, so a fixed reference
  plus the white-fill test is more stable.
- **Read the game's DOM/memory** — out of scope and brittle across the Steam wrapper.

### 4.5 Live debug view

The UI (and an optional always-on-top debug overlay) shows each ROI with its live
`whiteFrac`/`blueFrac` and a pass/fail chip, so threshold tuning is a five-second job
and not a guessing game. This is the single most useful support feature: every
mis-detection is visible as a number.

---

## 5. Calibration

Calibration is how four rectangles and four keys become a profile. It runs once per
resolution/layout.

1. The app captures a screenshot of the game window (or full screen if the window
   cannot be bound) and shows it scaled-to-fit in the calibration panel.
2. The user draws a rectangle around each of the four prompt spots and assigns the
   key it commands. The app defaults to the **`W`, `A`, `S`, `D`** mapping of
   [§1.1](#11-what-the-game-does).
3. The app stores each ROI in window-client coordinates plus the client-area size it
   was measured at (for later resize scaling, [§7](#7-window-binding-and-coordinates)).
4. The app optionally captures an **empty reference** of each ROI (a "no prompt"
   snapshot) for `diffFrac`, and optionally a prompt **template** if the user wants
   template mode.
5. The user hits **Test**: the app runs the detector live and shows the debug view
   while the game throws prompts, so the boxes and thresholds can be nudged before
   trusting a run.

A profile can hold more than one of these (e.g. one per window size or per OS), which
is why "profile" rather than "the config" ([§9](#9-profiles-and-settings)).

---

## 6. Runner

### 6.1 State machine

```
        Start                    Stop / Panic / maxRuntime
 idle ─────────► running ──────────────────────────────────────► idle
```

One goroutine owns the loop; capture and injection are serialized through it. Stop and
Panic are `context`-cancelled with interruptible sleeps (stop before the next
injection). Panic releases any held input immediately.

### 6.2 The poll loop

Deadline-accumulating scheduler (`next += pollMs; sleepUntil(next)` — never
`sleep(pollMs)`), so timing does not drift over long sessions.

```
each tick:
  frame  = capture(union(ROIs))
  scores = classify(frame)                    # per spot
  best   = argmax(scores) with threshold + margin
  for each spot s:
      if s == best:
          if armed[s] and now - lastFire[s] >= refractoryMs:
              fire(s)                          # TapKey(s.key) or ClickAt(roi center)
              armed[s] = false
              lastFire[s] = now
      else if scores[s] < clearThreshold:
          armed[s] = true                      # the spot went clear → it can fire again
```

- **`armed`** is the anti-double-tap guard: a spot fires **once** per appearance. It
  re-arms only after the spot is observed clear, or after `retryMs` while the prompt
  *persists* (which means the first tap probably did not register, or the fish needs
  another hit on the same key) — default `retryMs = 250`.
- **`refractoryMs`** (default `100`) is a hard floor between two fires of the same
  spot, a backstop against a detector flicker.
- Different spots may each fire in the same tick-resolution window; that is correct,
  since only one prompt is up at a time anyway.

### 6.3 Timing budget

| Stage | Cost |
|---|---|
| Capture union ROI (~250×620 px worst case, but typically ~600×250) | < 1 ms |
| Classify 4 ROIs (pure Go, ~40k px total) | < 1 ms |
| Key injection (`x11`/`libei`/`win`) | sub-millisecond |
| Poll period | default 40 ms |

Average latency from a prompt appearing to the key landing is therefore
`pollMs/2 + classify + inject ≈ 20–25 ms`. Prompts live ~1 s. Reaction latency is not
the risk; *detection correctness* is. `pollMs` and the thresholds are profile fields so
the user can trade CPU for latency.

### 6.4 Modes
- **Until stopped** (default): run forever, watch prompts.
- **Max runtime**: profile-level `failsafe.maxRuntimeSec` (default 3600,
  `0` = unlimited), measured on active time, auto-stopping with a notice.

---

## 7. Window binding and coordinates

### 7.1 Find the game window

The app locates the WAMI window by title (`"Wizard and Minion Idle"`, configurable) and
reads its **client area** rect. Since the game is the focused window during a run, the
**foreground/active window is tried first**, with title matching as the fallback:

| OS | Mechanism |
|---|---|
| Windows | `GetForegroundWindow`; fall back to `FindWindowW`; then `GetClientRect` + `ClientToScreen` |
| Linux X11 / XWayland | `_NET_ACTIVE_WINDOW`; fall back to enumerating `_NET_CLIENT_LIST` and matching `_NET_WM_NAME`/`WM_NAME`; then `XGetGeometry` + `XTranslateCoordinates` |

On Linux this works because the game runs under **Proton**, i.e. an **XWayland (X11)
window**, even inside a Wayland session. A *native*-Wayland game window cannot be
located (or captured) this way; that case is degraded, not supported
([§8](#8-screen-capture)).

### 7.2 Coordinates

ROIs are stored in **client-area pixels** plus the client size at calibration time:

```jsonc
"calibratedAt": { "w": 1920, "h": 1080 },
"spots": [ { "roi": { "x": 840, "y": 580, "w": 180, "h": 56 } }, … ]
```

At runtime each ROI is scaled if the window was resized:

```
x' = roi.x * clientW / calibratedAt.w       (same for y, w, h)
```

then offset by the client-area origin. This assumes the game scales its canvas
uniformly with the window, which is the norm for the HTML5 game; the **Test** view in
[§5](#5-calibration) confirms it before a run.

If the window cannot be found or bound, the app falls back to **absolute screen pixels**:
the user keeps the window at a fixed place and picks ROIs
against the screen. The profile records which mode it is in, and the UI says so.

On X11/XWayland there are **two coordinate systems** and mixing them shifts every ROI
by a whole monitor: the pointer query returns **root-window** coordinates (origin at the
left-most monitor), while per-monitor Xinerama geometry can have **negative** origins
for a monitor left of / above the primary. Convert once at the boundary
(`root = xinerama − unionMin`) and express everything in a single canonical space —
either the primary monitor's own pixels (origin `0,0`) or window-client pixels.

### 7.3 DPI on Windows

The app is per-monitor DPI aware. Window-client coordinates are physical pixels, and
`kbinani/screenshot` captures physical pixels, so ROI and capture space agree. Multi-
monitor with mixed scaling must be verified in Phase 0 **[unproven]**.

---

## 8. Screen capture

Capture is the one genuinely new, platform-sensitive dependency.

| Platform | Capture path | Works for |
|---|---|---|
| Windows 10 | `kbinani/screenshot` (GDI `BitBlt`) of the desktop or a region | windowed / **borderless** games |
| Linux X11 session | `kbinani/screenshot` (`XGetImage` of root, MIT-SHM fast path) | X11 games |
| Linux Wayland session, Proton game | `org.kde.KWin.ScreenShot2` **region capture** on KDE (fast: ~2 ms per ROI, ~43 ms full monitor) when authorized; otherwise `org.freedesktop.portal.Screenshot` (~2 fps) | XWayland/Proton games |
| Linux Wayland, native-Wayland game | none — out of scope (WAMI via Proton is XWayland) | — |
| Windows exclusive-fullscreen DirectX/Vulkan | often **black** (GDI cannot read the exclusive surface) | requires borderless windowed |

Consequences, stated up front in the README:
- **Play WAMI in borderless-windowed** (on Windows 10 and Kubuntu). This is the single most
  important setup instruction. Exclusive fullscreen is the classic "capture is black"
  trap and will be surfaced as such by the debug view.
- On **KDE Plasma Wayland** the fast path is `org.kde.KWin.ScreenShot2`, which captures a
  region directly. It requires KWin to authorize the process, which it grants from a
  `.desktop` file with `X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2` whose
  `Exec` matches the running binary (the app ships this; see Appendix B). Without it the
  app falls back to the portal at ~2 fps, where an X11 session or the KDE authorization is
  needed for responsiveness.
- The portal image origin is the **layout top-left, not the primary**, so a primary-local
  ROI `x` must be cropped at `x + virtualOrigin` (see [§7.2](#72-coordinates)); KWin uses
  the same global layout space, so the same offset applies.
- The app captures the **screen region where the game is**, which is correct because
  the game must be foreground for key injection to reach it. Occlusion is therefore not
  a supported case; minimizing the game stops detection (and the run).
- A capture failure (all-black region where the game should be) aborts the run with a
  clear banner rather than pressing keys blindly.

`kbinani/screenshot` is `cgo`-free on Linux and Windows, which keeps the single-binary,
cross-compiled release story intact. (On Darwin it needs cgo; macOS is out of scope.)

### 8.1 Capture interface

Capture is behind a small interface so the engine is testable and the backend is
swappable (e.g. adding the Wayland ScreenCast backend later):

```go
package vision

type Source interface {
    // CaptureRect returns a region in virtual-desktop physical pixels.
    CaptureRect(r image.Rectangle) (*image.RGBA, error)
    // Bounds returns the virtual desktop rectangle.
    Bounds() (image.Rectangle, error)
    // Name reports the backend for diagnostics ("x11", "gdi", "screencast").
    Name() string
    Close() error
}
```

---

## 9. Profiles and settings

One JSON file per profile in a user-visible folder, resolved through the OS API;
atomic write (temp + `fsync` + rename); unknown-field preservation on round-trip;
`schemaVersion` gate; a single-instance lock; structured rotating logs in the OS state
dir. This discipline is defined once and applies to every file the app writes.

### 9.1 Fishing profile schema

```jsonc
{
  "schemaVersion": 1,
  "name": "WAMI 1920x1080",
  "window": {
    "title": "Wizard and Minion Idle",
    "bind": true,                 // false → use absolute screen coords
    "calibratedAt": { "w": 1920, "h": 1080 }
  },
  "display": { "width": 5360, "height": 1440 },   // snapshot for the portable warning
  "detection": {
    "mode": "brightBox",          // "brightBox" | "template"
    "pollMs": 40,
    "whiteThreshold": 0.35,
    "clearThreshold": 0.15,
    "margin": 1.25,
    "templateThreshold": 0.80,
    "refractoryMs": 100,
    "retryMs": 250
  },
  "action": {
    "kind": "key",                // "key" | "click"
    "holdMs": 40                  // key down→up duration (click mode: same meaning)
  },
  "spots": [
    { "id": "s_a", "key": "a", "enabled": true,
      "roi": { "x": 840,  "y": 590, "w": 180, "h": 56 },
      "emptyRef": "refs/s_a_empty.png", "template": "" },
    { "id": "s_w", "key": "w", "enabled": true,
      "roi": { "x": 1060, "y": 480, "w": 180, "h": 56 },
      "emptyRef": "refs/s_w_empty.png", "template": "" },
    { "id": "s_d", "key": "d", "enabled": true,
      "roi": { "x": 1280, "y": 560, "w": 180, "h": 56 },
      "emptyRef": "refs/s_d_empty.png", "template": "" },
    { "id": "s_s", "key": "s", "enabled": true,
      "roi": { "x": 1150, "y": 700, "w": 180, "h": 56 },
      "emptyRef": "refs/s_s_empty.png", "template": "" }
  ],
  "failsafe": { "maxRuntimeSec": 3600 }
}
```

**The `roi` values above are illustrative only** — they are *not* shipped defaults. Real
values come from calibration and depend on the window layout. Shipping fake constants
would be worse than shipping none, so a new profile starts with no spots and the
calibration wizard.

### 9.2 App settings

Machine-specific, in the OS config dir: a `settings.json` holding no click-specific
fields — window geometry, selected profile,
profiles dir override, hotkeys and log level.

---

## 10. UI

A single dense window (the project's dark Tailwind vocabulary).

```
┌───────────────────────────────────────────────────────────────────────┐
│  WAMI Auto Fisher                                    ⚙ Settings       │
├───────────────────────────────┬───────────────────────────────────────┤
│  Profiles                     │  Prompt spots                         │
│  ▸ WAMI 1920x1080             │  ┌─────────────────────────────────┐  │
│  ▸ WAMI 1366x768              │  │ ☑ A    roi 180×56   score 0.82 │  │
│  + New                        │  │ ☑ W    roi 180×56   score 0.04 │  │
│                               │  │ ☑ D    roi 180×56   score 0.03 │  │
│  [ Calibrate ]  [ Test ]      │  │ ☑ S    roi 180×56   score 0.11 │  │
│                               │  └─────────────────────────────────┘  │
│                               │  [ Capture rectangle on screen ]      │
├───────────────────────────────┴───────────────────────────────────────┤
│  ● RUNNING · WAMI 1920x1080 · detected A · presses 342 · 5:12          │
│  action: key ▼   poll 40ms   threshold 0.35   [ ⏹ F8 ] [ Panic ]      │
└───────────────────────────────────────────────────────────────────────┘
```

- **Profiles sidebar** — select, rename, duplicate, delete, plus a New profile that
  opens calibration.
- **Spot list** — the four (or fewer) spots with ROI, key, enable switch and the live
  score from the debug view, so mis-detection is obvious.
- **Calibration** — screenshot + rectangle drawing + key assignment ([§5](#5-calibration)).
- **Test** — runs the detector and shows live scores/verdicts with the game visible;
  **no keys are injected** in Test mode, so it is always safe.
- **Bottom bar** — Start/Stop, action selector (key/click), poll and threshold, live
  telemetry (current detected spot, press count, elapsed).

Interaction rules: no native `<select>`, tooltips + `aria-label` on
icon buttons, `Esc` closes overlays, UI locked while running except Stop/Panic, focus
rings. No drag-and-drop complexity is needed here.

### 10.1 Control while the game is focused

The game is the **foreground window for the whole run**, so the app's window is hidden
behind it (or minimized) and the mouse must stay free for the user. That fixes the
control model:

- **Global hotkeys are the only control surface during a run** — `F8` Start/Stop,
  `F9` Pause/Resume, `Ctrl+Shift+F12` Panic. They must work while the game has focus
  (the portal / `XGrabKey` / `RegisterHotKey` layer).
- The **app window is for setup**: calibrate, Test, pick profile, set thresholds. The
  user configures, presses `F8`, then clicks into the game.
- A full UI is not needed over the game. If live reassurance is wanted, a tiny
  always-on-top **status strip** (current detected spot, press count, `F8` to stop) is a
  Phase 2 option; it is not required for v1.
- Because the game is focused, window location can prefer the **foreground/active
  window** (`GetForegroundWindow` on Windows, `_NET_ACTIVE_WINDOW` on X11/XWayland),
  with title matching as the fallback ([§7](#7-window-binding-and-coordinates)).

---

## 11. Safety

| Mechanism | Behavior | Phase |
|---|---|---|
| Panic hotkey (`Ctrl+Shift+F12`) | abort immediately, release held input | v1 |
| Stop (`F8`) | stop before the next injection | v1 |
| Start (`F8` while idle) | begin watching | v1 |
| Max runtime | profile `failsafe.maxRuntimeSec`, active-time based | v1 |
| Single-instance lock | second launch refuses to run | v1 |
| **Ambiguity guard** | ignore a poll when two ROIs are close in score | v1 |
| **Ambiguity/no-blind-press policy** | never press without a confident detection | v1 |
| Capture-failure abort | all-black game region → stop with a banner, not blind presses | v1 |
| UI locked while running | only Stop/Panic live; the game is foreground anyway | v1 |
| `ReleaseAll()` on exit | no held key on any exit path | v1 |
| Hotkey-failure warning | visible startup warning when no global source registered | v1 |

The global hotkey layer (`RegisterHotKey` / `XGrabKey` / `GlobalShortcuts` portal) has
several silent-failure traps, all listed in
[Appendix B](#appendix-b--platform-notes-worth-not-rediscovering). Because
the game is focused during a run, hotkeys must be global; this is a hard requirement,
not a nicety.

---

## 12. Code structure

```
go.mod                       module wami-auto-fisher
main.go                      Wails bootstrap; single-instance lock; hotkey registration;
                             service bindings; startup/shutdown
build/                       Wails config + appicon
internal/model/              Profile, Spot, Roi, Detection, Action; validation; JSON with
                             unknown-field preservation; schema migrations
internal/vision/             Source interface; classify.go (whiteFrac/blueFrac/NCC);
                             source_windows.go (kbinani/screenshot); source_linux.go
                             (kbinani/screenshot; screencast.go reserved for Phase 2)
internal/window/             window locate + client rect: windows.go / x11_linux.go;
                             scaling to absolute coords
internal/engine/             poll loop, per-spot armed/retry state, stop/panic, progress
internal/input/              Driver, x11/libei/win backends, FakeDriver
internal/cursor/             CursorPos query path (only for click mode + window picking)
internal/hotkey/             portal / XGrabKey / RegisterHotKey sources
internal/store/              paths, atomic writes, settings, single-instance lock
internal/logging/            rotating file log + tail for diagnostics
internal/service/            the operation surface the UI binds to (no Wails import)
frontend/                    React + TS app (sidebar, spots, calibration, debug, bottom bar)
frontend/bindings/           Wails-generated TS bindings (do not edit)
.github/workflows/           ci.yml, release.yml
```

`frontend/src` layout (`api/` WailsApi + MockApi, `components/`,
`state/`, `lib/`). `createApi()` returns `WailsApi` inside the webview and
`MockApi` under `vite dev`.

### 12.1 Package boundaries

The `Driver` interface is the deliberate seam between the engine and the OS input
backends: the engine depends only on the interface, and a `FakeDriver` implements it in
tests. Keep that interface stable — it is what makes the engine deterministic to test and
the backend swappable without touching the loop.

### 12.2 UI ↔ Go bindings (sketch)

```go
type Service struct{ /* engine, classifier, window, store, hotkeys */ }

func (s *Service) ListProfiles() ([]ProfileSummary, error)
func (s *Service) LoadProfile(id string) (*model.Profile, error)
func (s *Service) SaveProfile(p *model.Profile) error
func (s *Service) CreateProfile(name string) (string, error)
func (s *Service) DeleteProfile(id string) error

func (s *Service) CalibrationShot() ([]byte, error)   // PNG of the game window/screen
func (s *Service) ProbeSpots(p *model.Profile) ([]SpotScore, error)  // Test mode
func (s *Service) Start(profileID string) error
func (s *Service) Stop() error
func (s *Service) Panic() error
func (s *Service) Status() Progress
func (s *Service) Capabilities() Capabilities         // capture backend, window binding, input
func (s *Service) Diagnostics() Diagnostics
```

Events pushed to the UI: `spot:scores` (debug/telemetry), `run:progress`, `run:state`,
`profiles:changed`, `capabilities:changed`. `spot:scores` coalesces at ~10 Hz.

---

## 13. Testing

| Layer | Approach |
|---|---|
| `internal/vision` | golden-image tests: synthetic frames with a white box over green/blue backgrounds classify correctly; `whiteFrac` thresholds; ambiguity margin rejects two-box frames |
| `internal/window` | ROI scaling math (calibrated size → current size), origin translation, absolute/window-relative modes |
| `internal/engine` | `FakeCapture` (a scripted frame sequence) + `FakeInput`/`FakeDriver`: assert exactly one key tap per prompt appearance, re-arm after clear, retry while persisting, correct key per spot, no tap on ambiguous frames, stop before the next tap, `ReleaseAll` on every exit path |
| `internal/store` | atomic write, field preservation, single-instance lock |
| UI | `vitest` for score-chip rendering and calibration state; manual pass for screen-rectangle capture |

The engine test is the one that matters: feed frames
`[]→A→A→clear→D→D→clear` and assert the taps are `[A, D]`, with none on the ambiguous
frame.

### Manual release matrix
1. **Windows 10, borderless windowed**: calibrate, run, verify prompt keys land and
   the debugging view reads scores; verify `RegisterHotKey` stops it while the game is
   focused; launch a second instance → refused.
2. **Linux KDE Wayland, Proton/XWayland**: same; verify X11 capture of the Proton
   window and XTEST key injection; verify hotkeys via the portal.
3. **Linux X11 session**: same, X11-only.
4. **Regression**: resize the game window after calibrating → ROIs still line up
   (§7.2); exclusive-fullscreen → capture black → aborts with the banner, no blind
   presses.

---

## 14. Phases

### Phase 0 — blocking spike
Answer the questions that could change the design, on the real game, both OSes.
A runnable harness lives at `spike/phase0/` (see its README).

**Results (Kubuntu/Plasma Wayland, Steam/Proton, 2026-10-04):**

1. **Capture the Proton window** — ✅ Linux: KWin `ScreenShot2` region capture (~2 ms/spot
   union, ~43 ms full monitor) with the portal (~2 fps) as fallback; kbinani's
   `XGetImage` path is all-black on a Wayland session. ⏳ Windows (GDI) pending the
   Windows pass.
2. **Key injection reaches the game** — ✅ Linux: `robotgo/x11` XTEST taps land in the
   Proton game. ⏳ Windows pending.
3. **Detection reliability** — ✅ Linux: the user confirmed a full run works; the
   `white/blue` classifier reads the correct spot. Golden frames captured separately.
4. **Timing** — ✅ capture is ~2 ms/region and injection sub-millisecond; latency is
   dominated by the poll period (10–20 fps is comfortable).
5. **Window locate** — ✅ Linux: the WAMI window is reachable via XWayland/Xinerama; the
   game sat at global `(1920,0)` `1861x985`. Window-client ROI scaling is still to wire.
6. **Spot ⇒ key mapping** — ✅ the four spots are fixed and position identifies the key;
   the user calibrated exact boxes with `kdotool` and detection matched.

The original six questions:
1. **Capture the Proton window.** Does the capture backend read the WAMI window on
   Windows (borderless) and on Linux (KDE Wayland / X11)? Record whether exclusive
   fullscreen is black (expected).
2. **Key injection reaches the Proton game.** XTEST/`libei` on Linux, `SendInput` on
   Windows.
3. **Detection reliability.** Each spot classifies cleanly with the others clear.
4. **Timing.** Prompt-visible → key-landed latency.
5. **Window locate.** Exact title and readable client rect on both OSes.
6. **Spot ⇒ key mapping.** Which spot carries `W`, `A`, `S`, `D`, and does it rotate?

### Phase 1 — MVP
Profiles CRUD + calibration wizard + Test/debug view; 4 spots; `brightBox` classifier;
key action with click fallback; poll loop with armed/retry; Start/Stop/Panic + global
hotkeys; max runtime; single-instance lock; capture-failure abort; telemetry;
diagnostics; Wails shell; Linux + Windows builds.

**Status (in progress).** A first end-to-end shell exists:

| Area | State |
|---|---|
| `internal/fisher` — classifier, capture/input interfaces, detect loop | ✅ done, builds Linux + Windows |
| Linux capture — KWin ScreenShot2 (fast) + portal fallback; XTEST input | ✅ done, validated |
| Windows capture — kbinani GDI + robotgo/win SendInput | ✅ compiles; runtime test pending |
| Wails app (`main.go`, `service.go`) + React UI | ✅ builds (`wails3 build`, `-tags gtk3`), runs on Linux |
| UI: spot table (editable ROI/key), live white/blue, Start/Stop, threshold, KWin authorize, capture preview overlay | ✅ first cut |
| Global hotkeys, profiles, calibration wizard, max runtime, single-instance, telemetry events | ⏳ next |

Build: `wails3 build` (the Linux task defaults to `EXTRA_TAGS=gtk3`), producing
`bin/wami-auto-fisher`. Windows cross-build: `wails3 build GOOS=windows`.

### Phase 2 — polish
Template mode; auto-start (detect and click the `START` button after endurance
recovery); auto-learn the four spots from change detection; multiple profiles per
resolution; tray icon; run history.

### Deferred / maybe never
OCR of prompt text; other WAMI activities; mobile/browser; OSes other than Windows 10
and Kubuntu.

---

## 15. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Proton game capture returns black (exclusive fullscreen) | no detection | require borderless windowed; detect all-black and abort; README + debug view make it obvious |
| Native-Wayland game window unsupported | no detection on a Wayland session without XWayland | target is Proton (XWayland); Phase 2 ScreenCast backend; document |
| Window title/canvas geometry differs or the game rescales oddly | ROIs miss | calibration `Test` view; ROI scaling; window-relative coords |
| Detection false positive on bright/white UI near an ROI | wrong key → fish lost | tight ROIs, blue-border component, ambiguity margin, never-guess policy |
| Mixed-DPI multi-monitor | ROIs off by a scale factor | Phase 0 (items 1 and 5); capture and ROIs both in physical px; diagnostics show geometry |
| Steam overlay / input remapping intercepts keys | key does not land | README: disable overlay/key remapping for WAMI; Test mode verifies |
| robotgo master pin regresses | build/runtime breakage | exact pseudo-version pinned; input behind `Driver` |
| Anti-cheat / platform rules | account risk | single-player idle game, dev tolerant; README disclaimer |

---

## 16. Open questions

1. **Is the spot ⇒ key mapping truly fixed**, or can the same key appear at a different
   spot? If it can, position-only detection is wrong and the text must be read. Phase 0.6
   must confirm; the design assumes fixed, and the fallback is OCR or a per-key template.
2. **Auto-start scope.** Should v1 press `START` automatically when idle fishing begins
   (50 s of no prompt) or when endurance refills? Currently Phase 2, since it is a second
   detection target.
3. **Endurance exhaustion.** When endurance hits 0, fishing stops and `START` reappears.
   Does the user want the app to keep the session alive indefinitely (auto-restart), or
   stop when endurance runs out? Phase 2 toggle.
4. **Name/branding.** Working name "WAMI Auto Fisher", slug `wami-auto-fisher`; the
   display name and app id should be confirmed before the first release (the global-
   shortcut app id is hard to change later, see
   [Appendix B](#appendix-b--platform-notes-worth-not-rediscovering)).
5. **Windows runtime validation** of capture + `RegisterHotKey` on the actual game —
   pending the Phase 0 pass.

---

## Appendix A — Prompt spot reference

Approximate spot locations observed in the 1080p screenshots (illustrative; calibrate
for real values):

| Spot | Prompt (primary key) | Key | Approx center (1080p) | Background |
|---|---|---|---|---|
| S1 | `Click or Press A` | `a` | ~(1116, 698) | water (blue) |
| S2 | `Click or Press W` | `w` | ~(1253, 604) | trees (green) |
| S3 | `Click or Press D` | `d` | ~(1373, 683) | trees/grass (green) |
| S4 | `Click or Press S` | `s` | ~(1242, 800) | water/shorts (blue) |

Each prompt is a near-white box with a double blue border, ~180×56 px at 1080p. The
box is the only near-white, low-saturation object at those positions, which is what
makes `whiteFrac` a sufficient discriminator.

## Appendix B — Platform notes worth not rediscovering

Validated platform notes, carried here in full because every one causes a **silent**
(or catastrophic) failure and cost a debugging session each.

**Screen capture**
1. **Capture on a Wayland session is the hard part.** kbinani/screenshot picks its
   backend from `XDG_SESSION_TYPE`: on `wayland` it calls
   `org.freedesktop.portal.Screenshot`, which returns the **whole desktop** and works,
   but re-captures and PNG-encodes the full desktop every call — measured ~500 ms/frame
   (~2 fps) at 5360×1440 (2026-10-04). Its X11 path (`XGetImage` through XWayland) and
   robotgo's X11 `Capture` both fail on a Wayland session (all-black / `BadMatch`), and
   robotgo's X11 `Capture` also returned `BadMatch` on a plain XWayland target
   (2026-10-03). So: (a) fast capture needs the KDE ScreenShot2 path below, an X11
   session, or a ScreenCast + PipeWire stream;
   (b) the portal image origin is the **layout top-left, not the primary**, so crops
   must add the virtual origin; (c) key injection via XTEST still works.
   **KDE fast path:** when KWin authorizes the process, `org.kde.KWin.ScreenShot2.
   CaptureArea(x,y,w,h, options, pipe:fd)` captures a region in ~2 ms for a spot union
   and ~43 ms full monitor, writing the raw image into the pipe you pass. Authorization
   is resolved from `/proc/<pid>/exe` against a `.desktop` whose `Exec` matches and which
   carries `X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2`. A changed `Exec`
   is not picked up from a stale KDE service cache: clear `ksycoca6*` and run
   `kbuildsycoca6 --noincremental` (the app's `setup-kwin` does this).
2. Windows GDI cannot read an **exclusive-fullscreen** DirectX/Vulkan surface. Borderless
   windowed is mandatory for capture. The debug view's all-black detection is the
   user-facing explanation.

**Input backends**
3. Import the robotgo sub-packages (`x11`/`libei`/`win`) **directly, never the root
   package**: the root package's backends are mutually exclusive build tags that declare
   overlapping symbols, while the sub-packages coexist in one binary. Build with
   `CGO_ENABLED=0` and the pin in [§3](#3-stack).
4. `libei.Location()` returns the last *injected* position, not the real cursor. Source
   `CursorPos()` from the X11 query path when XWayland is present, and report
   `ok == false` otherwise.
5. libei absolute motion and screen geometry require a linked **ScreenCast** stream;
   without it, `Move` degrades to relative deltas that park the pointer top-left and
   drift after physical mouse movement. Gate absolute clicking on `CanMoveAbsolute`.
   Relatedly, under a Wayland session robotgo's X11 `Move` (XTEST `MotionNotify`) is
   **ignored by the compositor** — the physical pointer does not move (verified
   2026-10-04) — so the click fallback cannot reposition the pointer on Wayland; key
   injection is the reliable action there.
6. The libei consent token is a plain file at `$XDG_STATE_HOME/robotgo/portal_token`
   (fallback `~/.local/state/robotgo/portal_token`), **shared by any robotgo app**.
   Check it before init rather than probing with a dialog.

**Global-shortcuts portal** (every item below fails silently if done wrong)
7. `CreateSession` must pass **`session_handle_token`** (distinct from `handle_token`);
   missing it triggers `g_assert(token != NULL)` → abort → **core dump of
   `xdg-desktop-portal`**.
8. The `session_handle` in the response arrives as D-Bus type **`s`** (string), even
   though later methods take it as an object path.
9. `preferred_trigger` uses **UPPERCASE modifiers** — `"CTRL+SHIFT+F12"`, not
   `"Ctrl+Shift+F12"`. Wrong case logs `Unknown modifier` and produces an **empty
   binding**: the shortcut registers but never fires. Bare keys (`F8`) are
   case-insensitive, which masks the bug.
10. `CreateSession` refuses an **empty app id**. `org.freedesktop.host.portal.Registry.
    Register` needs a matching `~/.local/share/applications/<id>.desktop`. Launching from
    a **snap terminal poisons the app id** via the inherited snap cgroup; launch from a
    native terminal, a `.desktop` launcher, or `systemd-run --user --scope`.
11. Subscribe to `Activated` **without an object-path match** (the signal path is not
    reliably the session handle); match interface + member only and validate the session
    in the body.
12. `BindShortcuts` must register **every shortcut in a single call**: each call deletes
    the component's shortcuts absent from that call, so one call per shortcut silently
    leaves only the last. It is **asynchronous**: granted shortcuts arrive in the
    `Response` signal, *not* the method return, and `a(sa{sv})` needs a concrete struct
    (`[]interface{}` is marshalled as `av` and rejected).
13. One `BindShortcuts` call with new shortcuts = **one KDE consent dialog**, and the
    reply is deferred until the user answers it. Do not treat that as a hang. Shortcuts
    already present in the component are "returning" and skip the dialog.
14. Shortcuts are **session-scoped**: closing the session removes them from
    `kglobalshortcutsrc`. Re-bind on every start (the returning path makes it
    dialog-free once the ids exist).
15. On Plasma 6 Wayland, `org.kde.kglobalaccel` is owned by **`kwin_wayland` itself**;
    `kglobalacceld` starts and immediately exits. That is normal.
16. `XGrabKey` (X11 fallback) only fires while an XWayland/X11 client has focus, which
    **does** cover Steam/Proton games. `RegisterHotKey` (Windows): register with a
    **NULL hwnd on a dedicated thread** and pump `WM_HOTKEY` from that thread's message
    queue (force the queue with a `PeekMessage`); a message-only window is unnecessary.
    `GetSystemMetrics(SM_CXSCREEN)` is the **primary monitor only** — the virtual desktop
    is the union of the enumerated display rectangles.

**Single-instance**
17. A single-instance lock is mandatory for an input injector: two instances would
    double-press keys and fight over hotkeys. Hold it for the whole process lifetime so
    the OS releases it if the process dies.

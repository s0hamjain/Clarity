# Clarity for macOS

Clarity lives in your menu bar. Press **⌘⇧E**, drag a box around any problem on your screen, type what's confusing you, and a floating window explains it — in words within seconds, and as a short animation about a minute later.

## Install

1. Download `Clarity.dmg` from the latest [GitHub Release](https://github.com/s0hamjain/Clarity/releases).
2. Open it and drag **Clarity** to **Applications**.
3. Open Clarity. On macOS 15 and later the first launch is blocked because the app isn't notarized:
   **System Settings → Privacy & Security**, scroll down, click **Open Anyway**, then open Clarity again.
4. Grant the two permissions below. Each one needs a relaunch.

## The two permissions

macOS asks for these the first time each feature is used. They are tied to the app that asks, so Clarity only has to be granted once per install.

| Permission | When macOS asks | Why Clarity needs it |
|---|---|---|
| **Screen Recording** | The first time you press ⌘⇧E (or choose **Capture**) | To take the region screenshot. Nothing is recorded; a single still image of the box you drag is taken. |
| **Input Monitoring** | On first launch, when the global hotkey is registered | So ⌘⇧E works while any other app is in front. |

After granting either one, **quit and reopen Clarity**. macOS only applies these permissions to a freshly launched process.

If ⌘⇧E does nothing or the crosshair never appears: **System Settings → Privacy & Security → Screen Recording** and **→ Input Monitoring**, check that Clarity is listed and switched on, then relaunch.

## Using it

- **⌘⇧E** → a crosshair. Drag around the problem. **Esc** cancels.
- Type context (optional) and press **Enter**. Press **↓** in the empty box to pick a recent screenshot instead.
- The result box shows the explanation first, then the video. Drag it anywhere by its background. **X** or **Esc** closes it.
- Menu bar icon: **Capture**, **Recents**, **Guardrails** (teach the method, withhold the final answer), **Server…** (change the coordinator URL), **Clear Recents**, **Quit**.

### Asking again about an old screenshot

Your last 50 captures stay on your Mac, so you can come back to one without
re-taking it. Open the list three ways: **↓** (or **/**) in an empty box, the
**Recents** menu item, or **⌘⇧E** followed by **Esc** at the crosshair.

| In the list | What happens |
|---|---|
| Type | Filters by the problem text |
| **↑ ↓** | Move between rows; ↑ past the top goes back to the text field |
| **Enter** on a row with a blue dot | Reopens that explanation and video — no server needed |
| **Enter** on any other row, or **Tab** on any row | Loads that screenshot into the box so you can ask something new about it |
| **Esc** | Closes the list; **Esc** again closes the box |

The backend has to be running — see [docs/SETUP.md](../docs/SETUP.md). Clarity keeps no API keys; it only talks to the coordinator URL you set.

## Privacy

Screenshots stay on your Mac in `~/Library/Application Support/Clarity/recents/` (last 50) so you can re-ask about an old one. The only time an image leaves your machine is when you press Enter to submit it to your coordinator. **Clear Recents** deletes them all.

---

## For developers

### Run from source

```sh
cd desktop
python3 -m venv .venv && source .venv/bin/activate    # Python 3.12 or 3.13
pip install -r requirements.txt
python -m clarity              # menu bar icon appears; ⌘⇧E is live
python -m clarity --once       # one capture through the whole flow: spotlight box,
                               #   submit, result box. Waits until you close it.
python -m clarity --once --no-ask          # just the capture; prints its size and exits
python -m clarity --once --save ~/Desktop/cap.png
python -m clarity --recents    # the spotlight box on the recents list, no capture.
                               #   Prints the stored entries first. Needs no server —
                               #   this is the path to test with the coordinator stopped.
```

`CLARITY_DEBUG=1` turns on debug logging inside the window processes, and lets
the app ask a result box what it is currently showing — status line, note,
failure text, video source — so the states in FRD §19 can be tested without a
person reading them off the screen.

### You need a coordinator running

The app talks to one thing: P3's coordinator (`docs/API.md` §2). It needs no
Atlas, no Docker and no API keys in fake mode, which walks any job through
every status on a one-second timer with a sample explanation and video:

```sh
cd ../server
PORT=8081 FAKE_AGENT=1 FAKE_RENDER=1 go run ./cmd/server
```

Port 8081 rather than the default 8080 only because something else may already
have 8080; point the app at it with the **Server…** menu item. In fake mode
`/healthz` answers `ok: false` — Atlas, Docker, S3 and the agent really are
absent — so the app logs that on launch instead of raising a notification
about it, and only reports it when you ask via **Server…**.

**Use Terminal.app, and keep using it.** When running from source, macOS grants Screen Recording and Input Monitoring to the *terminal application* that launched Python, not to Python. Granting in Terminal and then running from iTerm or VS Code's terminal means granting again. After each grant, **quit and reopen the terminal app**.

### Building the app and the DMG

```sh
cd desktop && source .venv/bin/activate
./scripts/build_app.sh          # → dist/Clarity.app  (~44 MB)
./scripts/build_dmg.sh          # → dist/Clarity.dmg  (~24 MB)
```

Both are idempotent and take about a minute together. P3 packages `dist/Clarity.app`
into the `.pkg` and publishes the `.dmg` — that hand-off is the built files in
`dist/`, which is gitignored.

**The signature is the point of the build, not a formality.** macOS keys the
Screen Recording permission to the app's code signature, so an unsigned build
gets a new identity every time and the user is asked again after every rebuild.
`build_app.sh` signs with, in order: `$CODESIGN_IDENTITY`, a certificate named
`Clarity Dev` (SETUP §11.1), or the only codesigning identity in your keychain.
Any of them works — what matters is that it's the *same* one each time, because
the requirement macOS remembers names the identity, not the build:

```
designated => identifier "com.clarity.app" and anchor apple generic
               and certificate leaf[subject.CN] = "<your identity>"
```

`create-dmg` lays the window out by driving Finder over AppleScript, so the
first `build_dmg.sh` on a machine raises **"Terminal wants to control Finder"**.
Allow it, or the icons land wherever Finder likes and the background art won't
line up.

Three things about the bundle that don't apply when running from source — all
handled in `scripts/build_app.sh`, all silent failures if you drop them:

- `clarity/__main__.py` is PyInstaller's entry script, so it is compiled as
  `__main__` with no parent package. Its imports are absolute for that reason;
  relative ones raise `ImportError` in the bundle only.
- pywebview ships its own JavaScript, and the PyInstaller hook that collects it
  only fires on Windows. Without `--collect-data webview` every window opens
  blank.
- `create-dmg` makes whatever you hand it the root of the volume, so it gets a
  staging directory containing only `Clarity.app`. Passing the `.app` itself puts
  `Contents/` at the top level of the DMG.

### What granting looked like (Sprint 1 notes, macOS 26.5)

On the dev Mac both permissions had already been granted to Terminal.app from
earlier `screencapture -i` use, so Sprint 1 saw **no prompts** and needed no
restarts: `python -m clarity --once` went straight to the crosshair and the
hotkey registered on the first `python -m clarity`. A fresh Mac will see one
prompt each, and each one needs a relaunch of the launching app (SETUP §10.2).
The two-relaunch flow in the user section above is written from that spec and
will be confirmed on the second-Mac install in Sprint 4.

### Transparency spike (FRD §24) — done, code removed

Sprint 1 tested whether `pywebview` can draw a frameless, see-through, blurred
window (`frameless=True, transparent=True, vibrancy=True`) on macOS 26.5.

**Result:** it renders and is slightly translucent, but the blur is faint and
setting an explicit `NSVisualEffectView` material (HUD, popover, sidebar, …)
made no visible difference. White text on it was readable. Esc did not reach
the frameless window when launched as a bare script from Terminal.

**Decision:** Sprint 2 builds the spotlight and result boxes on a solid
`rgba(20,20,24,0.92)` background, keeping `frameless` + `transparent` for
rounded corners. Purely visual; revisit real vibrancy later if wanted. Esc and
Enter handling will be verified inside the real menu-bar app in Sprint 2.

Note for anyone testing pywebview windows: Ctrl+C does not stop them, because
Cocoa owns the main thread. Close the window itself.

### Does the signature keep the permission stable? (Sprint 4, FRD §24) — yes

The open question was whether a self-signed certificate really keeps Screen
Recording granted across rebuilds. It does, and the reason is visible in the
signature: the requirement macOS stores names the *identity*, never the build.

```
designated => identifier "com.clarity.app" and anchor apple generic
               and certificate leaf[subject.CN] = "…"
```

Two consecutive `build_app.sh` runs produce byte-identical requirements, so TCC
sees the same app both times even though every file inside changed. Built and
verified here with an existing **Apple Development** identity rather than a
self-signed `Clarity Dev` root — either satisfies the requirement, and neither is
notarized, so both still need one **Open Anyway** per install.

An unsigned bundle has no requirement to match, which is the whole failure mode
SETUP §11.1 is warning about: the permission resets on every rebuild and the app
silently stops capturing.

### Layout

| Path | What |
|---|---|
| `clarity/__main__.py` | Entry point: `--once` or the app |
| `clarity/app.py` | `rumps` menu-bar app and menu actions |
| `clarity/config.py` | `~/Library/Application Support/Clarity/config.json` |
| `clarity/hotkey.py` | `pynput` global hotkey, 2 s debounce, worker thread |
| `clarity/capture.py` | `screencapture -i -x` → Pillow ≤ 1568 px → PNG data URL |
| `clarity/recents.py` | The last 50 captures: `recents.json` + two PNGs each |
| `clarity/client.py` | The coordinator: create, poll, cancel, health |
| `clarity/session.py` | The flow: capture → spotlight → job → result box |
| `clarity/window_host.py` | One process per window, and the protocol to it |
| `clarity/spotlight_window.py` | The input box |
| `clarity/result_window.py` | The output box |
| `clarity/ui/` | The two windows' HTML, CSS and JS; vendored `marked.min.js` |
| `assets/` | App icon, menu bar template icons, the DMG background |
| `scripts/build_app.sh` | PyInstaller → `dist/Clarity.app`, `LSUIElement`, code signature |
| `scripts/build_dmg.sh` | `create-dmg` → `dist/Clarity.dmg` |
| `scripts/make_dmg_background.py` | Draws `assets/dmg_background.png` |

### Why each window is its own process

`rumps` and `pywebview` both want the macOS main thread — `rumps.App.run()`
runs the NSApplication loop for the menu bar, and pywebview refuses to start
anywhere else. They can't share a process, so the app re-runs itself as
`python -m clarity --window-host {spotlight,result}` per window and talks to it
over one JSON object per line on stdin/stdout (`clarity/window_host.py`).
Importing `webview` costs about 50 ms, so the box still appears at once. A
window that crashes takes nothing with it, and in the `.app` bundle the same
trick works because `sys.executable` *is* the entry point.

Three things this cost us, all handled in `window_host.py`, all worth knowing
before touching that file:

- **A closed window doesn't end the process.** pywebview closes the NSWindow
  and calls `NSApplication.stop_()`, but AppKit only acts on that when the run
  loop handles its next event. A window the user clicked closed has one; a
  window closed by the app has none, and the process would linger with nothing
  on screen. `close_window()` posts a no-op event to wake the loop and exits
  outright if that somehow doesn't unwind it.
- **Stdout is the protocol.** Never `print` in a window process. Logs go to
  stderr, which is inherited and shows up next to the app's own.
- **The pages' CSP needs `'unsafe-eval'`.** `window.evaluate_js()` runs a
  string, so the host can't talk to the page without it. Everything else stays
  `'self'`, which is what keeps rule 20 true — no script, style or image in
  `ui/` comes from the network.
- **Only the app process writes `recents.json`.** Window processes read it —
  the spotlight box needs the rows, and inlines the thumbnails because a page
  loaded from `ui/` can't fetch a `file://` image. Anything a window wants
  *written* goes back over the protocol as an event and the app does it, since
  two processes rewriting one JSON file would lose entries.
- **Growing the spotlight box happens in the window.** The page asks
  `js_api.resize(height)` for the room it wants; the host clamps that to what
  is left below the box on that display and returns what it applied, which is
  what the list sizes its scroll area from. On a short display the list shows
  fewer than eight rows and scrolls, rather than running off the screen.

Sprint plan and the API this app consumes: [docs/P4_DESKTOP.md](../docs/P4_DESKTOP.md), [docs/API.md §2](../docs/API.md#2-coordinator-api).

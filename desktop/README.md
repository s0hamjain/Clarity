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
python -m clarity --once       # one capture without the hotkey; prints size, exits
python -m clarity --once --save ~/Desktop/cap.png
```

**Use Terminal.app, and keep using it.** When running from source, macOS grants Screen Recording and Input Monitoring to the *terminal application* that launched Python, not to Python. Granting in Terminal and then running from iTerm or VS Code's terminal means granting again. After each grant, **quit and reopen the terminal app**.

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

### Layout

| Path | What |
|---|---|
| `clarity/__main__.py` | Entry point: `--once` or the app |
| `clarity/app.py` | `rumps` menu-bar app and menu actions |
| `clarity/config.py` | `~/Library/Application Support/Clarity/config.json` |
| `clarity/hotkey.py` | `pynput` global hotkey, 2 s debounce, worker thread |
| `clarity/capture.py` | `screencapture -i -x` → Pillow ≤ 1568 px → PNG data URL |
| `assets/` | App icon, menu bar template icons |
| `scripts/` | `build_app.sh`, `build_dmg.sh` (Sprint 4) |

Sprint plan and the API this app consumes: [docs/P4_DESKTOP.md](../docs/P4_DESKTOP.md), [docs/API.md §2](../docs/API.md#2-coordinator-api).

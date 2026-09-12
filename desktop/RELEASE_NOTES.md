# Clarity for macOS — desktop notes for v0.1.0

P4's hand-off to P3. The published notes are `release/RELEASE_NOTES.md`, which P3
owns; this file is what the desktop app contributes to them — the artifacts, the
install path a user actually walks, and what is known to be rough.

## Artifacts

| File | Size | Built by |
|---|---|---|
| `desktop/dist/Clarity.app` | ~44 MB | `desktop/scripts/build_app.sh` |
| `desktop/dist/Clarity.dmg` | ~24 MB | `desktop/scripts/build_dmg.sh` |

`Clarity.app` is a PyInstaller one-directory bundle: Python 3.13, `rumps`,
`pywebview`, `pynput`, `Pillow`, `requests`. `LSUIElement` is set, so it lives in
the menu bar with no Dock icon. Bundle identifier `com.clarity.app` — the same
string as the LaunchAgent label, so the `.pkg` and the `.dmg` install the same
identity.

**Not notarized.** Doing that needs a paid Apple Developer Program membership
(FRD §15.2 defers it), so every user does one **Open Anyway** per install. The
app is code-signed, which is a different thing: the signature is what keeps the
Screen Recording permission from resetting on every update.

## What the user does, in order

1. Download `Clarity.dmg`, open it, drag **Clarity** to **Applications**.
2. Open Clarity. macOS blocks it because it isn't notarized: **System Settings →
   Privacy & Security**, scroll down, **Open Anyway**, then open it again.
3. macOS asks for **Screen Recording** the first time a capture runs. Grant it,
   then **quit and reopen Clarity** — macOS only applies it to a freshly launched
   process.
4. Press `⌘⇧E`. macOS asks for **Input Monitoring** so the hotkey works while
   another app is in front. Grant it, quit and reopen again.
5. `⌘⇧E` → drag a box → type → Enter.

Two relaunches, one per permission. Worth saying plainly in the published notes:
people read a permission prompt that repeats as a bug.

**A coordinator has to be running** for anything past step 5 — the app ships no
API keys and talks only to the URL in its **Server…** menu item, default
`http://localhost:8080`.

## Known rough edges

- **The spotlight and result boxes are solid dark, not blurred.** `pywebview`'s
  `vibrancy=True` produced almost no blur on macOS 26 and setting an explicit
  `NSVisualEffectView` material changed nothing, so both windows use a solid
  `rgba(20,20,24,0.92)` panel (the Sprint 1 fallback, FRD §24). It looks
  deliberate; it just isn't the frosted glass the FRD describes.
- **Each window is its own process**, so Activity Monitor shows two or three
  `Clarity` entries while a result box is open. Expected — `rumps` and
  `pywebview` can't share a main thread.
- **A rebuild does not re-prompt for permissions** as long as it is signed with
  the same identity. If a user reports being asked again after an update, the
  build wasn't signed.
- The app keeps the last 50 screenshots in
  `~/Library/Application Support/Clarity/recents/`. **Clear Recents** deletes
  them. Nothing leaves the Mac until the user presses Enter on a capture.

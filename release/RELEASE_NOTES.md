# Clarity 0.1.0

Press `⌘⇧E` on any problem on your screen. Get it explained in words within
seconds, and as a short animated video about a minute later.

> **Draft.** Written from installing on a second Mac during Sprint 4. Finalise
> at the Sprint 5 sync point, once the DMG has been installed from the release
> URL by someone who has never run the project from source.

---

## Install

1. Download **`Clarity.dmg`** (the app) or **`Clarity.pkg`** (the app plus
   start-at-login).
2. **DMG:** open it and drag **Clarity** to Applications.
   **PKG:** double-click it and follow the installer.
3. Open **Clarity** from Applications.

### macOS 15 blocks it the first time

The build is signed with a self-signed certificate, not an Apple Developer ID,
so Gatekeeper refuses it on first launch. This is expected.

**System Settings → Privacy & Security →** scroll to the bottom **→ Open Anyway**.

### Two permissions, two restarts

macOS grants these to the app, and only takes effect after a relaunch. Clarity
asks for them in order:

1. **Screen Recording** — to capture the region you drag. Grant it, then
   **quit and reopen Clarity**.
2. **Input Monitoring** — to see the `⌘⇧E` hotkey. Grant it, then **quit and
   reopen Clarity** again.

Until both are granted and the app has been relaunched, the hotkey does
nothing and no capture window appears.

## Using it

`⌘⇧E` → drag a box around the problem → type what is confusing you (optional)
→ **Enter**. Press **↓** in the box to pick a recent screenshot instead.

The written explanation arrives first. The animation appears in the same window
when it is ready. Drag the window anywhere; close it with **X**.

## You need the backend running

This release is the desktop app only. It talks to a coordinator on your own
Mac at `http://localhost:8080`, which needs MongoDB Atlas, Docker, MinIO and
API keys. See `docs/SETUP.md`. Without it, captures fail with a network error.

## Known issues

- **Not notarized.** Every fresh install needs the "Open Anyway" step above.
- **Permissions can reset after an upgrade** if the build's signature changes.
  Re-grant both and relaunch.
- **The animation does not always arrive.** A scene that cannot be rendered is
  dropped. If none survive, the explanation stays on screen with a quiet note
  instead of a video — this is by design, the explanation is the product.
- **Start-at-login is PKG-only.** The DMG does not install a LaunchAgent.
- **Apple Silicon only**, macOS 14+.

## Uninstall

Drag **Clarity** from Applications to the Trash. If you installed the PKG, also
remove the login item:

```sh
launchctl bootout "gui/$(id -u)/com.clarity.app" 2>/dev/null
rm -f ~/Library/LaunchAgents/com.clarity.app.plist
```

Your screenshots and history are local; remove them with:

```sh
rm -rf ~/Library/Application\ Support/Clarity
```

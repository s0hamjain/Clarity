"""One process per window, and the line protocol between it and the app.

`rumps` and `pywebview` both want the macOS main thread: `rumps.App.run()`
runs the NSApplication loop for the menu bar, and pywebview refuses to start
anywhere but the main thread (`webview/__init__.py`, "pywebview must be run on
a main thread"). They cannot share one process, so every window — the spotlight
box and each result box — is a child process that the menu-bar app spawns,
talks to over stdin/stdout, and outlives.

Importing `webview` costs about 50 ms, so a window still appears immediately.

**The protocol.** One JSON object per line, in both directions. The child's
stdout is the channel, so the child must never `print`; it logs to stderr,
which is inherited and shows up in the terminal next to the app's own logs.

    child → app     {"event": "submit", "text": "…"}
    app → child     {"cmd": "accepted"}

Unparseable lines are logged and skipped rather than killing either side.
"""

from __future__ import annotations

import json
import logging
import os
import subprocess
import sys
import threading
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

log = logging.getLogger(__name__)

PACKAGE_DIR = Path(__file__).resolve().parent
UI_DIR = PACKAGE_DIR / "ui"


# --------------------------------------------------------------------------- #
# Geometry. The app decides where windows go — it is the only side that knows
# how many are already open — and passes a screen index the child re-resolves.
# --------------------------------------------------------------------------- #


@dataclass(frozen=True)
class Box:
    """A window's placement. `x` and `y` are relative to the top-left of screen
    `screen_index`, which is what pywebview's cocoa backend expects alongside a
    `screen=` argument."""

    screen_index: int
    x: int
    y: int
    width: int
    height: int

    def as_dict(self) -> dict[str, int]:
        return {
            "screen_index": self.screen_index,
            "x": self.x,
            "y": self.y,
            "width": self.width,
            "height": self.height,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Box":
        return cls(
            screen_index=int(d.get("screen_index", 0)),
            x=int(d["x"]),
            y=int(d["y"]),
            width=int(d["width"]),
            height=int(d["height"]),
        )


def cursor_screen() -> tuple[int, int, int]:
    """(index, width, height) of the display the mouse is on.

    Cocoa's mouse location and screen frames share a bottom-left origin, so
    `NSMouseInRect` with `flipped=False` is the right test. Falls back to the
    main display if the cursor is somewhere no screen claims.
    """
    import AppKit

    point = AppKit.NSEvent.mouseLocation()
    screens = AppKit.NSScreen.screens()
    for index, screen in enumerate(screens):
        if AppKit.NSMouseInRect(point, screen.frame(), False):
            size = screen.frame().size
            return index, int(size.width), int(size.height)

    size = AppKit.NSScreen.mainScreen().frame().size
    return 0, int(size.width), int(size.height)


def centered_box(width: int, height: int) -> Box:
    """A box of this size centered on the display the cursor is on (FRD §15.1)."""
    index, screen_w, screen_h = cursor_screen()
    return Box(
        screen_index=index,
        x=max(0, (screen_w - width) // 2),
        y=max(0, (screen_h - height) // 2),
        width=width,
        height=height,
    )


def child_screen(index: int):
    """Rebuild a pywebview `Screen` for `index` inside the child process.

    `webview.screens()` needs the GUI library initialized, which has not
    happened before `create_window`, so this builds the same object the cocoa
    backend's `get_screens()` would have.
    """
    import AppKit
    from webview.screen import Screen

    screens = AppKit.NSScreen.screens()
    ns = screens[index] if 0 <= index < len(screens) else AppKit.NSScreen.mainScreen()
    frame = ns.frame()
    return Screen(
        frame.origin.x,
        frame.origin.y,
        frame.size.width,
        frame.size.height,
        frame,
        ns.backingScaleFactor(),
    )


# --------------------------------------------------------------------------- #
# App side
# --------------------------------------------------------------------------- #


class WindowProcess:
    """A live window, from the menu-bar app's point of view.

    `on_event` is called on a reader thread for every event the window emits,
    plus a synthetic `{"event": "exited"}` when the process goes away — so a
    window that crashes still resolves whatever the app was waiting on.
    """

    def __init__(
        self,
        kind: str,
        payload: dict[str, Any],
        on_event: Callable[[dict[str, Any]], None],
        name: str = "window",
    ) -> None:
        self.kind = kind
        self._on_event = on_event
        self._write_lock = threading.Lock()

        self._proc = subprocess.Popen(
            self._argv(kind),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=None,  # inherit, so the child's logs land next to ours
            env=self._env(),
            text=True,
            bufsize=1,
        )
        log.info("%s window up (pid %d)", kind, self._proc.pid)

        # The payload is the first line in, rather than a command-line argument
        # or a temp file, because it carries a base64 thumbnail.
        self.send({"payload": payload})

        self._reader = threading.Thread(target=self._read_loop, name=f"clarity-{name}-reader", daemon=True)
        self._reader.start()

    @staticmethod
    def _argv(kind: str) -> list[str]:
        if getattr(sys, "frozen", False):
            # In the .app bundle sys.executable *is* the entry point.
            return [sys.executable, "--window-host", kind]
        return [sys.executable, "-m", "clarity", "--window-host", kind]

    @staticmethod
    def _env() -> dict[str, str]:
        env = os.environ.copy()
        if not getattr(sys, "frozen", False):
            # `-m clarity` has to resolve without depending on the app's cwd.
            parent = str(PACKAGE_DIR.parent)
            existing = env.get("PYTHONPATH", "")
            env["PYTHONPATH"] = f"{parent}{os.pathsep}{existing}" if existing else parent
        return env

    # -- talking to the window ----------------------------------------------

    def send(self, message: dict[str, Any]) -> bool:
        """Write one line to the window. False if it has already gone away."""
        if self._proc.poll() is not None or self._proc.stdin is None:
            return False
        line = json.dumps(message)
        try:
            with self._write_lock:
                self._proc.stdin.write(line + "\n")
                self._proc.stdin.flush()
            return True
        except (BrokenPipeError, ValueError, OSError):
            return False

    def command(self, cmd: str, **fields: Any) -> bool:
        return self.send({"cmd": cmd, **fields})

    @property
    def alive(self) -> bool:
        return self._proc.poll() is None

    def close(self, timeout: float = 2.0) -> None:
        """Ask the window to close; kill it if it won't."""
        self.command("close")
        try:
            self._proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            log.warning("%s window ignored close, killing pid %d", self.kind, self._proc.pid)
            self._proc.kill()

    # -- listening ----------------------------------------------------------

    def _read_loop(self) -> None:
        stdout = self._proc.stdout
        try:
            for line in iter(stdout.readline, ""):
                line = line.strip()
                if not line:
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    log.debug("%s window said: %s", self.kind, line)
                    continue
                if isinstance(event, dict):
                    self._dispatch(event)
        finally:
            if stdout is not None:
                stdout.close()
            self._proc.wait()
            log.info("%s window closed (exit %s)", self.kind, self._proc.returncode)
            self._dispatch({"event": "exited", "code": self._proc.returncode})

    def _dispatch(self, event: dict[str, Any]) -> None:
        # A callback must never take down the reader thread (FRD §23 rule 19).
        try:
            self._on_event(event)
        except Exception:  # noqa: BLE001
            log.exception("handling %s from the %s window", event.get("event"), self.kind)


# --------------------------------------------------------------------------- #
# Child side
# --------------------------------------------------------------------------- #

_emit_lock = threading.Lock()


def emit(event: str, **fields: Any) -> None:
    """Send one event to the app. stdout is the protocol — never print here."""
    with _emit_lock:
        sys.stdout.write(json.dumps({"event": event, **fields}) + "\n")
        sys.stdout.flush()


def read_payload() -> dict[str, Any]:
    """Block for the first line, which is always the payload."""
    for line in iter(sys.stdin.readline, ""):
        line = line.strip()
        if not line:
            continue
        try:
            message = json.loads(line)
        except json.JSONDecodeError:
            log.warning("payload was not JSON, ignoring: %s", line[:120])
            continue
        if isinstance(message, dict) and "payload" in message:
            return message["payload"] or {}
    raise SystemExit("no payload; the app went away before the window opened")


def listen(handler: Callable[[dict[str, Any]], None]) -> None:
    """Read commands on a daemon thread and hand each to `handler`.

    Runs in the background because `webview.start()` owns the main thread. When
    stdin closes the app is gone, so the window closes itself.
    """

    def loop() -> None:
        for line in iter(sys.stdin.readline, ""):
            line = line.strip()
            if not line:
                continue
            try:
                message = json.loads(line)
            except json.JSONDecodeError:
                log.debug("ignoring non-JSON command: %s", line[:120])
                continue
            if not isinstance(message, dict):
                continue
            log.debug("command %s", message.get("cmd"))
            try:
                handler(message)
            except Exception:  # noqa: BLE001
                log.exception("handling command %s", message.get("cmd"))
        log.info("app closed the pipe; shutting the window")
        handler({"cmd": "close"})

    threading.Thread(target=loop, name="clarity-window-commands", daemon=True).start()


def ui_url(*parts: str) -> str:
    """A file:// URL into the bundled ui/ directory."""
    return (UI_DIR.joinpath(*parts)).as_uri()


def js_literal(value: Any) -> str:
    """A Python value as a JavaScript literal, safe to interpolate into a call."""
    return json.dumps(value)


def evaluate(window, script: str) -> Any:
    """`window.evaluate_js`, but a broken page can't take the window down.

    pywebview raises whatever the page threw, and this runs on event and reader
    threads where an exception would be lost anyway (FRD §23 rule 19).
    """
    try:
        return window.evaluate_js(script)
    except Exception:  # noqa: BLE001
        log.exception("evaluating %s", script[:60])
        return None


def close_window(window) -> None:
    """Destroy the window and make sure the process actually goes away.

    pywebview closes the NSWindow and calls `NSApplication.stop_()`, but AppKit
    only acts on `stop_` when the run loop handles its *next* event. A window
    the user clicked closed has that event; one closed on our own say-so — the
    app accepted a submit, the user quit, the pipe closed — has none, and the
    process would sit there with no window. So: ask AppKit to close, wake the
    loop with a no-op event, and if the loop still hasn't unwound shortly after,
    leave anyway. The only state a window host owns is its window.
    """
    try:
        window.destroy()
    except Exception:  # noqa: BLE001 — already gone
        log.debug("destroy failed; exiting anyway", exc_info=True)

    try:
        import AppKit

        wake = AppKit.NSEvent.otherEventWithType_location_modifierFlags_timestamp_windowNumber_context_subtype_data1_data2_(
            AppKit.NSEventTypeApplicationDefined,
            AppKit.NSMakePoint(0, 0),
            0,
            0,
            0,
            None,
            0,
            0,
            0,
        )
        AppKit.NSApplication.sharedApplication().postEvent_atStart_(wake, True)
    except Exception:  # noqa: BLE001
        log.debug("could not wake the event loop", exc_info=True)

    bail = threading.Timer(1.5, lambda: os._exit(0))
    bail.daemon = True
    bail.start()


def start(webview) -> None:
    """Run the window, first hiding the host process from the Dock.

    pywebview runs `NSApplication.setActivationPolicy_(0)` — the regular
    policy — when its cocoa backend is imported, which would flash a Dock icon
    and an app menu every time the user presses ⌘⇧E. Importing the backend here
    and switching to the accessory policy before `start()` beats it to the
    screen; accessory windows still take keyboard focus, which the text field
    needs. Cosmetic, so any failure just runs with the Dock icon.
    """
    try:
        import AppKit

        import webview.platforms.cocoa  # noqa: F401 — runs the policy line above

        AppKit.NSApplication.sharedApplication().setActivationPolicy_(
            AppKit.NSApplicationActivationPolicyAccessory
        )
    except Exception:  # noqa: BLE001
        log.debug("could not hide the window host from the Dock", exc_info=True)

    webview.start(gui="cocoa")


def run(kind: str) -> int:
    """Child entry point: `python -m clarity --window-host {spotlight,result}`.

    Logs go to stderr so stdout stays clean for the protocol.
    """
    logging.basicConfig(
        level=logging.DEBUG if os.environ.get("CLARITY_DEBUG") else logging.INFO,
        stream=sys.stderr,
        format=f"%(asctime)s %(levelname)-7s {kind}-window: %(message)s",
        datefmt="%H:%M:%S",
    )

    if kind == "spotlight":
        from .spotlight_window import run_window
    elif kind == "result":
        from .result_window import run_window
    else:
        log.error("unknown window kind %r", kind)
        return 2

    try:
        run_window(read_payload())
    except SystemExit:
        raise
    except Exception:  # noqa: BLE001 — never a traceback in the user's face
        log.exception("the %s window failed", kind)
        emit("error", message="This window couldn't open.")
        return 1
    return 0

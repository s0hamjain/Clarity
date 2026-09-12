"""The spotlight box — the translucent input box that follows a capture.

FRD §15.1 (Spotlight box, Recents in the box), P4_DESKTOP Sprint 2 Step 1 and
Sprint 3 Step 3. 680×96, frameless, centered on the display the cursor is on,
animates in over 180 ms, one text field, Enter submits and Esc cancels. With an
empty field, ↓ or `/` grows the box downward into the recents list.

Both halves live here: `Spotlight` is what the menu-bar app holds, and
`run_window` is what runs inside the child process (see `window_host`).

**Why Enter doesn't close the window immediately.** FRD §15.1 says Enter
animates the box out and closes it; it also says a submit that can't reach the
server leaves the box open to retry. So Enter puts the field in a sending state
and the app answers with `accepted` (animate out, close) or `error` (restore
the field with a message). Over loopback the round trip is a few milliseconds,
so what the user sees is the box leaving on Enter.

**Why the window reads recents but never writes them.** Growing the list has to
be instant and the page can't load a `file://` image (rule 20), so the window
process reads `recents.json` and inlines the thumbnails itself. Acting on a row
is the app's business — it owns the capture and the result boxes — so Enter and
Tab on a row come back as events and the app answers with a `capture` command
or a new result box.
"""

from __future__ import annotations

import logging
import threading
from typing import Any, Callable

from . import window_host
from .window_host import Box, WindowProcess, js_literal as _js

log = logging.getLogger(__name__)

WIDTH = 680
HEIGHT = 96

PLACEHOLDER = "Add context… (why is my binary search not working? visualize where it's messing up)"

# Opened from the Recents menu item, or by pressing Esc at the crosshair: there
# is no capture yet, so the field can't be submitted until a recent is picked.
PLACEHOLDER_NO_CAPTURE = "Pick a recent screenshot (↓), or press ⌘⇧E to capture something new"

# The list must not run off the bottom of the display. The window is centered,
# so this is how much room is left below it (FRD §15.1 caps the list at 8 rows;
# on a short display it caps lower and the list scrolls).
BOTTOM_MARGIN = 24


# --------------------------------------------------------------------------- #
# App side
# --------------------------------------------------------------------------- #


class Spotlight:
    """One open spotlight box.

    `on_submit(text)` runs on the window's reader thread and must answer with
    `accept()` or `show_error()`. `on_close()` fires once, whether the user
    cancelled, the submit went through, or the window died.

    The two recents callbacks are FRD §15.1's two ways of using a row:
    `on_open_recent(id)` — the row has a result, so its result box reopens from
    local data — and `on_pick_recent(id)` — its screenshot becomes the capture
    in this box, for a new question about an old problem (§16.5).
    """

    def __init__(
        self,
        thumbnail: str | None,
        on_submit: Callable[[str], None],
        on_close: Callable[[], None] | None = None,
        on_open_recent: Callable[[str], None] | None = None,
        on_pick_recent: Callable[[str], None] | None = None,
        placeholder: str | None = None,
        expanded: bool = False,
        has_capture: bool | None = None,
    ) -> None:
        # A thumbnail that couldn't be built is still a capture the box can
        # submit, so the caller gets to say so (`capture.thumbnail_data_url`
        # returns None rather than failing).
        if has_capture is None:
            has_capture = thumbnail is not None
        self.box: Box = window_host.centered_box(WIDTH, HEIGHT)
        self._on_submit = on_submit
        self._on_close = on_close
        self._on_open_recent = on_open_recent
        self._on_pick_recent = on_pick_recent
        self._closed = threading.Event()

        self._proc = WindowProcess(
            "spotlight",
            {
                "box": self.box.as_dict(),
                "thumbnail": thumbnail,
                "placeholder": placeholder
                or (PLACEHOLDER if has_capture else PLACEHOLDER_NO_CAPTURE),
                "has_capture": bool(has_capture),
                "expanded": bool(expanded),
            },
            self._handle,
            name="spotlight",
        )

    # -- answers to a submit -------------------------------------------------

    def accept(self) -> None:
        """The job was created: animate out and close."""
        self._proc.command("accepted")

    def show_error(self, message: str) -> None:
        """The submit failed: keep the box open with a message (FRD §19)."""
        self._proc.command("error", message=message)

    def set_capture(self, thumbnail: str | None, note: str | None = None) -> None:
        """A recent's screenshot is now what this box will submit: the thumbnail
        swaps, the list collapses, the field takes focus (FRD §15.1)."""
        self._proc.command("capture", thumbnail=thumbnail, note=note)

    def close(self) -> None:
        self._proc.close()

    @property
    def alive(self) -> bool:
        return self._proc.alive

    def wait_closed(self, timeout: float | None = None) -> bool:
        return self._closed.wait(timeout)

    # -- events --------------------------------------------------------------

    def _handle(self, event: dict[str, Any]) -> None:
        kind = event.get("event")
        if kind == "submit":
            self._on_submit(str(event.get("text") or ""))
        elif kind == "open_recent":
            if self._on_open_recent is not None:
                self._on_open_recent(str(event.get("recent_id") or ""))
        elif kind == "pick_recent":
            if self._on_pick_recent is not None:
                self._on_pick_recent(str(event.get("recent_id") or ""))
        elif kind in ("cancel", "exited"):
            if not self._closed.is_set():
                self._closed.set()
                if self._on_close is not None:
                    self._on_close()


# --------------------------------------------------------------------------- #
# Child side
# --------------------------------------------------------------------------- #


class _JsApi:
    """What `window.pywebview.api` exposes to ui/spotlight/spotlight.js."""

    def __init__(self, box: Box) -> None:
        self.window = None
        self.box = box
        self._max_height = HEIGHT

    def submit(self, text: str) -> None:
        window_host.emit("submit", text=str(text or "")[:2000])

    def cancel(self) -> None:
        window_host.emit("cancel")
        self._destroy()

    def dismissed(self) -> None:
        """The out animation finished after an accepted submit."""
        self._destroy()

    # -- recents -------------------------------------------------------------

    def list_recents(self) -> list[dict[str, Any]]:
        """The rows for the list: label, relative time, inline thumbnail, and
        whether the entry has a result or has lost its screenshot.

        `recents` is imported here rather than at module scope because it pulls
        in Pillow, and the box has to be on screen in well under the 180 ms it
        spends animating in.
        """
        try:
            from .recents import Recents

            return Recents().rows()
        except Exception:  # noqa: BLE001 — an unreadable store means no list
            log.exception("could not read recents")
            return []

    def open_recent(self, recent_id: str) -> None:
        """Enter on a row that already has a result: the app reopens its result
        box from local data (FRD §15.1)."""
        window_host.emit("open_recent", recent_id=str(recent_id or ""))

    def pick_recent(self, recent_id: str) -> None:
        """Enter on a row without a result, or Tab on any row: the app loads
        that screenshot as the capture behind this box (FRD §16.5)."""
        window_host.emit("pick_recent", recent_id=str(recent_id or ""))

    def resize(self, height: int) -> int:
        """Grow or collapse the window, and report the height actually applied.

        pywebview's default fix point holds the top-left corner, so the box
        grows downward. The clamp is what keeps the list on screen; the page
        sizes its scroll area from the number it gets back.
        """
        try:
            wanted = int(height)
        except (TypeError, ValueError):
            return HEIGHT
        applied = max(HEIGHT, min(wanted, self._max_height))
        if self.window is not None:
            try:
                self.window.resize(self.box.width, applied)
            except Exception:  # noqa: BLE001 — a box that won't grow still works
                log.exception("could not resize to %d", applied)
                return HEIGHT
        return applied

    def measure(self) -> None:
        """Work out how far the box may grow, once the GUI can tell us about
        the display it is on."""
        try:
            screen = window_host.child_screen(self.box.screen_index)
            self._max_height = max(HEIGHT, int(screen.height) - self.box.y - BOTTOM_MARGIN)
        except Exception:  # noqa: BLE001
            log.debug("could not measure the display; the list won't expand", exc_info=True)
            self._max_height = HEIGHT

    def _destroy(self) -> None:
        if self.window is not None:
            window_host.close_window(self.window)


def run_window(payload: dict[str, Any]) -> None:
    import webview

    box = Box.from_dict(payload["box"])
    api = _JsApi(box)
    loaded = threading.Event()

    window = webview.create_window(
        "Clarity",
        url=window_host.ui_url("spotlight", "index.html"),
        js_api=api,
        width=box.width,
        height=box.height,
        x=box.x,
        y=box.y,
        screen=window_host.child_screen(box.screen_index),
        # min_size defaults to 200×100, which would stretch a 96 px box.
        min_size=(360, box.height),
        frameless=True,
        easy_drag=False,  # the field is the whole window; dragging would fight typing
        transparent=True,
        vibrancy=True,
        on_top=True,
        resizable=False,
        shadow=False,  # the panel draws its own shadow, and a frameless
        focus=True,  # ...system shadow squares off the rounded corners
    )
    api.window = window

    def on_loaded() -> None:
        api.measure()
        window_host.evaluate(
            window,
            "window.clarityInit({})".format(
                _js(
                    {
                        "thumbnail": payload.get("thumbnail"),
                        "placeholder": payload.get("placeholder") or PLACEHOLDER,
                        "has_capture": bool(payload.get("has_capture", payload.get("thumbnail"))),
                        "expanded": bool(payload.get("expanded")),
                        "base_height": HEIGHT,
                    }
                )
            ),
        )
        loaded.set()

    window.events.loaded += on_loaded

    def on_command(message: dict[str, Any]) -> None:
        cmd = message.get("cmd")
        if cmd in ("accepted", "error", "capture"):
            loaded.wait(timeout=3)
        if cmd == "accepted":
            window_host.evaluate(window, "window.clarityAccepted()")
        elif cmd == "error":
            window_host.evaluate(window, f"window.clarityError({_js(message.get('message'))})")
        elif cmd == "capture":
            window_host.evaluate(
                window,
                "window.clarityCapture({})".format(
                    _js({"thumbnail": message.get("thumbnail"), "note": message.get("note")})
                ),
            )
        elif cmd == "close":
            api._destroy()

    window_host.listen(on_command)
    window_host.start(webview)

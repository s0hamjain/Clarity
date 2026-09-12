"""The spotlight box — the translucent input box that follows a capture.

FRD §15.1 (Spotlight box row), P4_DESKTOP Sprint 2 Step 1. 680×96, frameless,
centered on the display the cursor is on, animates in over 180 ms, one text
field, Enter submits and Esc cancels.

Both halves live here: `Spotlight` is what the menu-bar app holds, and
`run_window` is what runs inside the child process (see `window_host`).

**Why Enter doesn't close the window immediately.** FRD §15.1 says Enter
animates the box out and closes it; it also says a submit that can't reach the
server leaves the box open to retry. So Enter puts the field in a sending state
and the app answers with `accepted` (animate out, close) or `error` (restore
the field with a message). Over loopback the round trip is a few milliseconds,
so what the user sees is the box leaving on Enter.
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


# --------------------------------------------------------------------------- #
# App side
# --------------------------------------------------------------------------- #


class Spotlight:
    """One open spotlight box.

    `on_submit(text)` runs on the window's reader thread and must answer with
    `accept()` or `show_error()`. `on_close()` fires once, whether the user
    cancelled, the submit went through, or the window died.
    """

    def __init__(
        self,
        thumbnail: str | None,
        on_submit: Callable[[str], None],
        on_close: Callable[[], None] | None = None,
        placeholder: str = PLACEHOLDER,
    ) -> None:
        self.box: Box = window_host.centered_box(WIDTH, HEIGHT)
        self._on_submit = on_submit
        self._on_close = on_close
        self._closed = threading.Event()

        self._proc = WindowProcess(
            "spotlight",
            {
                "box": self.box.as_dict(),
                "thumbnail": thumbnail,
                "placeholder": placeholder,
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

    def __init__(self) -> None:
        self.window = None

    def submit(self, text: str) -> None:
        window_host.emit("submit", text=str(text or "")[:2000])

    def cancel(self) -> None:
        window_host.emit("cancel")
        self._destroy()

    def dismissed(self) -> None:
        """The out animation finished after an accepted submit."""
        self._destroy()

    def list_recents(self) -> list[dict[str, Any]]:
        """Sprint 3 (recents store). Empty means the box never expands."""
        return []

    def pick_recent(self, recent_id: str) -> None:
        """Sprint 3."""
        log.info("pick_recent(%s) — not until Sprint 3", recent_id)

    def _destroy(self) -> None:
        if self.window is not None:
            try:
                self.window.destroy()
            except Exception:  # noqa: BLE001 — already gone
                pass


def run_window(payload: dict[str, Any]) -> None:
    import webview

    box = Box.from_dict(payload["box"])
    api = _JsApi()
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
        window_host.evaluate(
            window,
            "window.clarityInit({{thumbnail: {}, placeholder: {}}})".format(
                _js(payload.get("thumbnail")), _js(payload.get("placeholder") or PLACEHOLDER)
            ),
        )
        loaded.set()

    window.events.loaded += on_loaded

    def on_command(message: dict[str, Any]) -> None:
        cmd = message.get("cmd")
        if cmd in ("accepted", "error"):
            loaded.wait(timeout=3)
        if cmd == "accepted":
            window_host.evaluate(window, "window.clarityAccepted()")
        elif cmd == "error":
            window_host.evaluate(window, f"window.clarityError({_js(message.get('message'))})")
        elif cmd == "close":
            api._destroy()

    window_host.listen(on_command)
    window_host.start(webview)

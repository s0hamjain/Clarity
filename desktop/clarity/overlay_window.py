"""The full-screen dim + glow overlay, shown while Clarity is actively
working with the user — from the moment a capture starts (the screenshot
crosshair is up) until nothing is open any more.

Same window-process pattern as the spotlight and result boxes (see
`window_host`'s protocol docstring): `Overlay` is what the app holds,
`run_window` is what runs inside the child process.

It is visible on screen *during* the actual screenshot too — `session.py`
opens it before calling `capture.capture_region`, so the user sees the
border while dragging the crosshair — but it never ends up *in* the captured
pixels: `_tune_native_window` sets the native window's sharing type to
`NSWindowSharingNone`, which excludes a window from any screen capture
(`screencapture`, `CGWindowListCreateImage`, screen recording) while leaving
it fully visible to the human eye. Confirmed by taking a real capture with
the overlay up and diffing it against one with the overlay closed.

Click-through and non-activating: this window only paints a dim layer and a
flowing border along the screen edge (`ui/overlay`); it must never steal a
click or keyboard focus from the spotlight/result windows drawn above it, or
from any other app the user might still want to use while a job runs.
"""

from __future__ import annotations

import logging
import threading
from typing import Any

from . import window_host
from .window_host import Box, WindowProcess

log = logging.getLogger(__name__)


class Overlay:
    """One open overlay window, covering a single display."""

    def __init__(self, screen_index: int, width: int, height: int) -> None:
        self.box = Box(screen_index=screen_index, x=0, y=0, width=width, height=height)
        self._closed = threading.Event()
        self._proc = WindowProcess(
            "overlay",
            {"box": self.box.as_dict()},
            self._handle,
            name="overlay",
        )

    def close(self) -> None:
        self._proc.close()

    @property
    def alive(self) -> bool:
        return self._proc.alive

    def _handle(self, event: dict[str, Any]) -> None:
        if event.get("event") == "exited" and not self._closed.is_set():
            self._closed.set()


# --------------------------------------------------------------------------- #
# Child side
# --------------------------------------------------------------------------- #

# How long the fade-out animation (overlay.css) gets before the window is
# actually torn down.
_FADE_OUT_SEC = 0.25


def run_window(payload: dict[str, Any]) -> None:
    import webview

    box = Box.from_dict(payload["box"])

    window = webview.create_window(
        "Clarity",
        url=window_host.ui_url("overlay", "index.html"),
        width=box.width,
        height=box.height,
        x=box.x,
        y=box.y,
        screen=window_host.child_screen(box.screen_index),
        frameless=True,
        easy_drag=False,
        transparent=True,
        on_top=True,
        resizable=False,
        shadow=False,
        focus=False,  # must never take focus from the spotlight field
    )

    def on_loaded() -> None:
        # Deferred, not called straight from this delegate callback: PyObjC's
        # void-argument AppKit setters (setLevel_, setIgnoresMouseEvents_) hit
        # a reentrancy crash (SIGILL) when called synchronously from inside a
        # WKWebView delegate callback. pywebview's own `on_top` handling hits
        # the same hazard and works around it the same way — see
        # `webview/platforms/cocoa.py`'s `set_on_top`, which schedules its
        # `setLevel_` call via `AppHelper.callAfter` rather than calling it
        # directly from wherever it's invoked.
        from PyObjCTools import AppHelper

        AppHelper.callAfter(_tune_native_window, window, box.screen_index)
        window_host.evaluate(window, "window.clarityShow()")

    window.events.loaded += on_loaded

    def on_command(message: dict[str, Any]) -> None:
        if message.get("cmd") == "close":
            window_host.evaluate(window, "window.clarityHide()")
            threading.Timer(_FADE_OUT_SEC, lambda: window_host.close_window(window)).start()

    window_host.listen(on_command)
    window_host.start(webview)


def _tune_native_window(window, screen_index: int) -> None:
    """Snap to the screen's true bounds, then make it click-through, below
    Clarity's own windows, and visible on every Space.

    Cosmetic and best-effort (FRD §23 rule 19): a failure here just leaves the
    overlay misplaced or interactive, which is a worse look, not a broken app.
    """
    try:
        import AppKit

        native = window.native

        # pywebview's `frameless=True` still keeps NSWindowStyleMaskTitled
        # under the hood (title bar hidden via a textured/full-size-content
        # style, not true borderless — see webview/platforms/cocoa.py's
        # BrowserView init). AppKit auto-constrains any titled window to
        # `visibleFrame` — it can never be placed to cover the menu bar strip
        # — so a window created at (0, 0) full-screen silently lands ~25pt
        # too low no matter what frame is requested afterwards; confirmed by
        # comparing NSScreen.frame() to window.native.frame() directly and
        # finding origin.y off by exactly the title-bar height, unmovable via
        # setFrame_display_ alone. Switching to true Borderless lifts that
        # constraint, since only titled windows are auto-clamped this way —
        # then the frame can actually be set to the screen's real bounds.
        native.setStyleMask_(AppKit.NSWindowStyleMaskBorderless)
        screens = AppKit.NSScreen.screens()
        screen = screens[screen_index] if 0 <= screen_index < len(screens) else AppKit.NSScreen.mainScreen()
        native.setFrame_display_(screen.frame(), True)

        native.setIgnoresMouseEvents_(True)
        # NSFloatingWindowLevel sits below the system menu bar's own layer,
        # which then paints over — hides — the top edge of a full-screen
        # border at that level, confirmed by screenshot: left/right/bottom
        # showed the glow, only the top strip under the menu bar didn't. The
        # spotlight/result windows' own NSStatusWindowLevel (pywebview's
        # `on_top=True`) renders above the menu bar, so matching it here
        # covers all four edges. This never ends up on top of those windows
        # in practice regardless of matching their level: the overlay never
        # takes focus (`focus=False`) and is click-through, so AppKit's
        # front-to-back order within one level — driven by which window was
        # last made key — always keeps it behind whichever of them is open.
        native.setLevel_(AppKit.NSStatusWindowLevel)
        native.setCollectionBehavior_(
            AppKit.NSWindowCollectionBehaviorCanJoinAllSpaces
            | AppKit.NSWindowCollectionBehaviorStationary
            | AppKit.NSWindowCollectionBehaviorIgnoresCycle
        )
        # Visible to the user, invisible to any screen capture — including the
        # native `screencapture -i` crosshair this overlay is now up during.
        # Without this, dragging out a region while the border is on screen
        # would bake the border into the captured image.
        native.setSharingType_(AppKit.NSWindowSharingNone)
    except Exception:  # noqa: BLE001
        log.debug("could not tune the overlay window", exc_info=True)

"""The popped-out video — the finished animation in its own window, draggable
anywhere on screen.

The result box is a fixed 440×680 panel that the user positions once; the video
inside it is stuck wherever that panel happens to be. That is the wrong
constraint for the one thing they actually want to place carefully: the
animation explains the problem they are still looking at, so it needs to sit
*next to* that problem, not wherever the text happens to live.

So the video gets its own window. Same one-process-per-window pattern as the
spotlight, result and overlay boxes (see `window_host`'s protocol docstring):
`VideoWindow` is what the app holds, `run_window` is what runs in the child.

It plays from `video_url` directly, exactly as the result box does — the
coordinator never proxies video (FRD §11.4), so popping out costs no extra
fetch and no local copy. Rule 20 still holds: nothing else on the page comes
from the network.
"""

from __future__ import annotations

import logging
import threading
from typing import Any, Callable
from urllib.parse import quote

from . import window_host
from .window_host import Box, WindowProcess

log = logging.getLogger(__name__)

# 16:9 video plus the drag bar above it. The real clip is 854×480 (-ql) or
# 1280×720 (-qm), both 16:9, and the page letterboxes anything else.
WIDTH = 520
CHROME_HEIGHT = 34
HEIGHT = CHROME_HEIGHT + round(WIDTH * 9 / 16)


# --------------------------------------------------------------------------- #
# App side
# --------------------------------------------------------------------------- #


class VideoWindow:
    """One popped-out video.

    `on_close(job_id)` fires when it goes away for any reason, so the result
    box it came from can put its inline player back.
    """

    def __init__(
        self,
        job_id: str,
        video_url: str,
        box: Box,
        on_close: Callable[[str], None] | None = None,
    ) -> None:
        self.job_id = job_id
        self.box = box
        self._on_close = on_close
        self._closed = threading.Event()

        self._proc = WindowProcess(
            "video",
            {"box": box.as_dict(), "job_id": job_id, "video_url": video_url},
            self._handle,
            name=f"video-{job_id}",
        )

    def close(self) -> None:
        self._proc.close()

    @property
    def alive(self) -> bool:
        return self._proc.alive

    def _handle(self, event: dict[str, Any]) -> None:
        if event.get("event") == "exited" and not self._closed.is_set():
            self._closed.set()
            if self._on_close is not None:
                self._on_close(self.job_id)


def box_beside(anchor: Box | None) -> Box:
    """Where a popped-out video opens: just right of the box it came from, or
    centered on the cursor's display if that would run off the screen."""
    index, screen_w, screen_h = window_host.cursor_screen()

    if anchor is not None:
        index = anchor.screen_index
        x, y = anchor.x + anchor.width + 16, anchor.y
        if x + WIDTH > screen_w:
            # No room to the right — sit just left of it instead.
            x = anchor.x - WIDTH - 16
    else:
        x, y = (screen_w - WIDTH) // 2, (screen_h - HEIGHT) // 2

    return Box(
        screen_index=index,
        x=max(0, min(x, screen_w - WIDTH)),
        y=max(0, min(y, screen_h - HEIGHT)),
        width=WIDTH,
        height=HEIGHT,
    )


# --------------------------------------------------------------------------- #
# Child side
# --------------------------------------------------------------------------- #


class _JsApi:
    """What `window.pywebview.api` exposes to ui/video/video.js."""

    def __init__(self) -> None:
        self.window = None

    def close(self) -> None:
        if self.window is not None:
            window_host.close_window(self.window)


def run_window(payload: dict[str, Any]) -> None:
    import webview

    box = Box.from_dict(payload["box"])
    video_url = str(payload["video_url"])

    api = _JsApi()
    url = "{}?src={}".format(
        window_host.ui_url("video", "index.html"),
        quote(video_url, safe=""),
    )

    window = webview.create_window(
        "Clarity",
        url=url,
        js_api=api,
        width=box.width,
        height=box.height,
        x=box.x,
        y=box.y,
        screen=window_host.child_screen(box.screen_index),
        frameless=True,
        # The whole window is a drag handle. The player and its controls opt
        # back out with .no-drag, or the first click on play would move the
        # window instead of starting the video.
        easy_drag=True,
        transparent=True,
        on_top=True,
        resizable=False,
        shadow=False,  # the page draws its own rounded shadow
        focus=False,  # popping out must not steal focus from what's underneath
    )
    api.window = window

    def on_loaded() -> None:
        from PyObjCTools import AppHelper

        # Deferred for the reason overlay_window.py documents: PyObjC's
        # void-argument AppKit setters crash (SIGILL) when called straight
        # from a WKWebView delegate callback.
        AppHelper.callAfter(_tune_native_window, window)

    window.events.loaded += on_loaded

    def on_command(message: dict[str, Any]) -> None:
        if message.get("cmd") == "close":
            window_host.close_window(window)

    window_host.listen(on_command)
    window_host.start(webview)


def _tune_native_window(window) -> None:
    """Let the video follow the user to any Space and sit above ordinary
    windows without being a full-screen-blocking panel.

    Cosmetic and best-effort (FRD §23 rule 19): failing here leaves a video
    window that behaves like a normal one, which is worse, not broken.
    """
    try:
        import AppKit

        native = window.native
        # The point of this window is to sit beside the problem being
        # explained, which may be on another Space or a full-screen app.
        native.setCollectionBehavior_(
            AppKit.NSWindowCollectionBehaviorCanJoinAllSpaces
            | AppKit.NSWindowCollectionBehaviorFullScreenAuxiliary
        )
    except Exception:  # noqa: BLE001
        log.debug("could not tune the video window", exc_info=True)

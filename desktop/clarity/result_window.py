"""The result box — the floating window that shows the explanation, then the video.

FRD §15.1 (Result box, Result content rows), P4_DESKTOP Sprint 2 Step 2.
440×680, frameless, always on top, draggable by its whole background, opens
where the spotlight box was and offset from any box already open.

The polling lives in `ui/result/result.js`, not here: FRD §15.1 loads the page
as `index.html?job=&server=` and the page polls `GET /api/jobs/{id}` every
second itself. This module's job is the window, the X/Esc close that cancels a
job still running (API.md §2.3), and telling the app when the explanation lands
so it can post a notification.

Both halves live here: `ResultBox` is what the menu-bar app holds, `run_window`
is what runs in the child process.
"""

from __future__ import annotations

import logging
import os
import threading
from typing import Any, Callable
from urllib.parse import quote

from . import window_host
from .window_host import Box, WindowProcess

log = logging.getLogger(__name__)

WIDTH = 440
HEIGHT = 680

# Each job gets its own box, stepped down-right from the last so several stay
# usable at once (FRD §15.1).
STACK_OFFSET = 24


# --------------------------------------------------------------------------- #
# App side
# --------------------------------------------------------------------------- #


class ResultBox:
    """One open result box.

    `on_cancel(job_id)` fires when the user closed the box before the job
    reached a terminal status — the app answers with `DELETE /api/jobs/{id}`.
    `on_explanation(job_id)` fires once, the first time the page renders an
    explanation, so the app can post the notification from FRD §15.1.
    `on_update_recent(recent_id, fields)` fires on every poll that learned
    something, and the app writes it to the local recent (rule 21).
    `on_pop_out_video(job_id, url)` fires when the user detaches the video, and
    `on_close_popped_video(job_id)` when they send it back — the app owns that
    window, because only it can spawn a process.
    """

    def __init__(
        self,
        job_id: str,
        server_url: str,
        box: Box,
        on_cancel: Callable[[str], None] | None = None,
        on_explanation: Callable[[str], None] | None = None,
        on_close: Callable[[str], None] | None = None,
        on_retry: Callable[[str], None] | None = None,
        on_update_recent: Callable[[str, dict[str, Any]], None] | None = None,
        on_pop_out_video: Callable[[str, str], None] | None = None,
        on_close_popped_video: Callable[[str], None] | None = None,
        local: dict[str, Any] | None = None,
        recent_id: str | None = None,
    ) -> None:
        self.job_id = job_id
        self.box = box
        self.recent_id = recent_id
        self._on_pop_out_video = on_pop_out_video
        self._on_close_popped_video = on_close_popped_video
        self._on_cancel = on_cancel
        self._on_explanation = on_explanation
        self._on_close = on_close
        self._on_retry = on_retry
        self._on_update_recent = on_update_recent
        self._closed = threading.Event()

        self._proc = WindowProcess(
            "result",
            {
                "box": box.as_dict(),
                "job_id": job_id,
                "server_url": server_url,
                # A reopened recent hands its saved fields over here, so the box
                # paints before the first poll answers (FRD §15.1).
                "local": local or None,
            },
            self._handle,
            name=f"result-{job_id}",
        )

    def close(self) -> None:
        self._proc.close()

    def video_returned(self) -> None:
        """The popped-out video window is gone; show the inline player again."""
        self._proc.command("video_returned")

    @property
    def alive(self) -> bool:
        return self._proc.alive

    def _handle(self, event: dict[str, Any]) -> None:
        kind = event.get("event")
        if kind == "explanation":
            if self._on_explanation is not None:
                self._on_explanation(self.job_id)
        elif kind == "cancel":
            # The window is closing on a job that never finished.
            if self._on_cancel is not None:
                self._on_cancel(self.job_id)
        elif kind == "retry":
            if self._on_retry is not None:
                self._on_retry(self.job_id)
        elif kind == "pop_out_video":
            url = event.get("video_url")
            if self._on_pop_out_video is not None and url:
                self._on_pop_out_video(self.job_id, str(url))
        elif kind == "close_popped_video":
            if self._on_close_popped_video is not None:
                self._on_close_popped_video(self.job_id)
        elif kind == "recent":
            # Only the app process writes recents.json, so the page's updates
            # come back here rather than being written in the window (rule 21).
            if self._on_update_recent is not None and self.recent_id:
                fields = event.get("fields")
                if isinstance(fields, dict):
                    self._on_update_recent(self.recent_id, fields)
        elif kind == "exited":
            if not self._closed.is_set():
                self._closed.set()
                if self._on_close is not None:
                    self._on_close(self.job_id)


def stacked_box(previous: Box | None, anchor: Box | None = None) -> Box:
    """Where the next result box goes.

    The first one opens at the spotlight box's top-left (`anchor`); each later
    one steps 24 px down and right from the one before, clamped so a long
    session can't walk a box off the bottom of the display.
    """
    index, screen_w, screen_h = window_host.cursor_screen()

    if previous is not None:
        base_x, base_y, index = previous.x + STACK_OFFSET, previous.y + STACK_OFFSET, previous.screen_index
    elif anchor is not None:
        base_x, base_y, index = anchor.x, anchor.y, anchor.screen_index
    else:
        base_x, base_y = (screen_w - WIDTH) // 2, (screen_h - HEIGHT) // 2

    return Box(
        screen_index=index,
        x=max(0, min(base_x, screen_w - WIDTH)),
        y=max(0, min(base_y, screen_h - HEIGHT)),
        width=WIDTH,
        height=HEIGHT,
    )


# --------------------------------------------------------------------------- #
# Child side
# --------------------------------------------------------------------------- #


class _JsApi:
    """What `window.pywebview.api` exposes to ui/result/result.js."""

    def __init__(self, job_id: str) -> None:
        self.window = None
        self.job_id = job_id
        self._explained = False

    def close(self, cancel: bool = False) -> None:
        """X or Esc. `cancel` is true while the job has no terminal status, and
        the app turns that into DELETE /api/jobs/{id} (FRD §15.1)."""
        if cancel:
            window_host.emit("cancel", job_id=self.job_id)
        self._destroy()

    def minimize(self) -> None:
        """Genie the box into the Dock.

        This window's process runs as an accessory app (window_host.start's
        docstring) precisely so a Dock icon never flashes just from opening a
        capture — but a *miniaturized* window needs a Dock tile to animate
        into and be restored from, so minimizing is the one action that
        promotes this one process to a regular, Dock-visible app. That's a
        one-way trip for the lifetime of this process, which is fine: it
        already exists for exactly one result box, and promoting only
        happens on an explicit click, never on every window open.
        """
        from PyObjCTools import AppHelper

        def do_minimize() -> None:
            try:
                import AppKit

                AppKit.NSApplication.sharedApplication().setActivationPolicy_(
                    AppKit.NSApplicationActivationPolicyRegular
                )
            except Exception:  # noqa: BLE001
                log.debug("could not show a Dock icon to minimize into", exc_info=True)
            if self.window is not None:
                try:
                    self.window.minimize()
                except Exception:  # noqa: BLE001
                    log.debug("could not minimize", exc_info=True)

        # Deferred for the same reason overlay_window.py defers its native
        # calls: PyObjC's void-argument AppKit setters crash if called
        # straight from a bridge/delegate callback (confirmed empirically —
        # see overlay_window.py's docstring for the repro).
        AppHelper.callAfter(do_minimize)

    def pop_out_video(self, video_url: str) -> None:
        """Detach the video into its own draggable window.

        Only the menu-bar app can spawn a window process, so this window asks
        rather than opens (window_host's protocol docstring).
        """
        window_host.emit("pop_out_video", job_id=self.job_id, video_url=video_url)

    def close_popped_video(self) -> None:
        """Bring the video back inline — the app closes the popped window."""
        window_host.emit("close_popped_video", job_id=self.job_id)

    def explanation_shown(self) -> None:
        """First non-null explanation — the app posts a notification."""
        if not self._explained:
            self._explained = True
            window_host.emit("explanation", job_id=self.job_id)

    def retry(self, job_id: str = "") -> None:
        """Retry after a failed job. A job can't be restarted, so the app
        submits the same capture again and opens a fresh box (FRD §19)."""
        window_host.emit("retry", job_id=job_id or self.job_id)

    def update_recent(self, fields: dict[str, Any]) -> None:
        """Every poll that adds data updates the local recent (rule 21).

        The app does the writing — see `ResultBox._handle`. This fires once a
        second while a job runs, so the app drops anything that didn't change
        rather than rewriting the file on every tick.
        """
        if isinstance(fields, dict) and fields:
            window_host.emit("recent", job_id=self.job_id, fields=fields)

    def log(self, message: str) -> None:
        """So the page can report trouble into the app's terminal."""
        log.info("page: %s", message)

    def _destroy(self) -> None:
        if self.window is not None:
            window_host.close_window(self.window)


def run_window(payload: dict[str, Any]) -> None:
    import webview

    box = Box.from_dict(payload["box"])
    job_id = str(payload["job_id"])
    server_url = str(payload["server_url"]).rstrip("/")

    api = _JsApi(job_id)
    url = "{}?job={}&server={}".format(
        window_host.ui_url("result", "index.html"),
        quote(job_id, safe=""),
        quote(server_url, safe=""),
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
        easy_drag=True,  # the whole background is the drag handle (FRD §15.1)
        transparent=True,
        vibrancy=True,
        on_top=True,
        resizable=False,
        shadow=False,  # the panel draws its own; a system shadow would square
        focus=True,  # ...off the rounded corners
        text_select=True,  # the user should be able to copy the explanation
    )
    api.window = window

    def on_loaded() -> None:
        local = payload.get("local")
        if local:
            window_host.evaluate(window, f"window.clarityLocal({window_host.js_literal(local)})")

    window.events.loaded += on_loaded

    def on_command(message: dict[str, Any]) -> None:
        if message.get("cmd") == "dump" and os.environ.get("CLARITY_DEBUG"):
            # Read back what the page is actually showing, so the failure states
            # in FRD §19 can be checked without a person looking at them.
            window_host.emit("dump", state=window_host.evaluate(window, "window.clarityDump()"))
            return
        if message.get("cmd") == "video_returned":
            # The popped-out window went away — by its own X, or because the
            # app closed it. Put the inline player back either way.
            window_host.evaluate(window, "window.clarityVideoReturned && window.clarityVideoReturned()")
            return
        if message.get("cmd") != "close":
            return
        # Go through the page so this behaves exactly like the X button —
        # including cancelling a job that hasn't finished. If the page can't
        # answer, close the window anyway.
        if window_host.evaluate(window, "window.clarityClose && window.clarityClose(), true") is None:
            api._destroy()

    window_host.listen(on_command)
    window_host.start(webview)

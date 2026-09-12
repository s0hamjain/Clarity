"""The capture → spotlight → job → result box flow.

FRD §16.2–16.3, P4_DESKTOP Sprint 2. This is the whole user-facing sequence in
one place so that both callers drive the same code: the menu-bar app
(`app.py`), and `python -m clarity --once` for testing without the hotkey.

It owns the windows and the coordinator client. Everything it can't do — a
macOS notification, knowing when the last window closed — it hands back through
callbacks, because `rumps` isn't available in every caller.
"""

from __future__ import annotations

import logging
import threading
from typing import Any, Callable

from . import capture as capture_mod
from . import result_window
from .capture import Capture
from .client import ApiError, Client, Unreachable
from .config import Config
from .result_window import ResultBox
from .spotlight_window import Spotlight
from .window_host import Box

log = logging.getLogger(__name__)

Notify = Callable[[str, str, str], None]


def _no_notify(title: str, subtitle: str, message: str) -> None:
    log.info("notification: %s — %s %s", title, subtitle, message)


class Session:
    """One app's worth of state: the client, the spotlight box, the open
    result boxes, and the captures behind them."""

    def __init__(
        self,
        config: Config,
        notify: Notify | None = None,
        on_all_closed: Callable[[], None] | None = None,
    ) -> None:
        self.config = config
        self.client = Client(lambda: self.config.server_url)
        self._notify = notify or _no_notify
        self._on_all_closed = on_all_closed

        self._lock = threading.Lock()
        self._spotlight: Spotlight | None = None
        self._results: dict[str, ResultBox] = {}
        self._last_box: Box | None = None
        # Set between accepting a submit and the result box existing, so the
        # spotlight box closing in that gap doesn't look like "nothing is open".
        self._opening = False
        # The capture behind each job, so Retry can submit it again (FRD §19).
        self._submissions: dict[str, tuple[str, str]] = {}

    # -- the flow ------------------------------------------------------------

    def capture_and_ask(self) -> bool:
        """Region capture, then the spotlight box. False if the user pressed Esc
        at the crosshair, or the capture failed (both already reported)."""
        cap = capture_mod.capture_region()
        if cap is None:
            log.info("capture cancelled")
            return False
        log.info("captured %dx%d, %d bytes", cap.width, cap.height, cap.size_bytes)
        self.open_spotlight(cap)
        return True

    def open_spotlight(self, cap: Capture) -> Spotlight:
        """Show the input box for a capture. Only one is ever open."""
        with self._lock:
            existing = self._spotlight
        if existing is not None and existing.alive:
            existing.close()

        spotlight = Spotlight(
            thumbnail=capture_mod.thumbnail_data_url(cap.png_bytes),
            on_submit=lambda text: self._submit(cap, text),
            on_close=self._spotlight_closed,
        )
        with self._lock:
            self._spotlight = spotlight
        return spotlight

    def _submit(self, cap: Capture, user_prompt: str) -> None:
        """POST the job, then open its result box. Runs on the spotlight
        window's reader thread."""
        with self._lock:
            spotlight = self._spotlight
        if spotlight is None:
            return

        try:
            job_id = self.client.create_job(
                cap.data_url,
                user_prompt=user_prompt,
                guardrails=self.config.guardrails,
            )
        except Unreachable:
            # FRD §19: notification, and the box stays open to retry.
            log.warning("coordinator unreachable at %s", self.client.base_url)
            self._notify("Clarity", "Can't reach the server", self.client.base_url)
            spotlight.show_error(f"Can't reach {self.client.base_url}")
            return
        except ApiError as exc:
            log.warning("job refused: %s", exc)
            self._notify("Clarity", "The server refused the job", exc.message)
            spotlight.show_error(exc.message)
            return

        log.info("job %s created", job_id)
        self._submissions[job_id] = (cap.data_url, user_prompt)
        anchor = spotlight.box
        with self._lock:
            self._opening = True
        try:
            # The box leaves first, then the result box arrives in its place
            # (FRD §16.2).
            spotlight.accept()
            self.open_result(job_id, anchor=anchor)
        finally:
            with self._lock:
                self._opening = False

    def open_result(
        self,
        job_id: str,
        anchor: Box | None = None,
        local: dict[str, Any] | None = None,
    ) -> ResultBox:
        """Open a result box for a job. Boxes stack so several can stay open."""
        with self._lock:
            box = result_window.stacked_box(self._last_box, anchor)
            self._last_box = box

        result = ResultBox(
            job_id,
            server_url=self.client.base_url,
            box=box,
            on_cancel=self._cancel,
            on_explanation=self._explained,
            on_close=self._result_closed,
            on_retry=self._retry,
            local=local,
        )
        with self._lock:
            self._results[job_id] = result
        return result

    # -- window events -------------------------------------------------------

    def _spotlight_closed(self) -> None:
        with self._lock:
            self._spotlight = None
            idle = not self._results and not self._opening
        if idle:
            self._idle()

    def _result_closed(self, job_id: str) -> None:
        with self._lock:
            self._results.pop(job_id, None)
            self._submissions.pop(job_id, None)
            idle = not self._results and self._spotlight is None and not self._opening
            if not self._results:
                # Nothing is open, so the next box starts from the middle again
                # instead of continuing to march down the screen.
                self._last_box = None
        if idle:
            self._idle()

    def _cancel(self, job_id: str) -> None:
        """The user closed a box before the job finished (API.md §2.3)."""
        self.client.cancel_job(job_id)

    def _explained(self, job_id: str) -> None:
        """The explanation is on screen — tell the user, who may have looked
        away (FRD §15.1, Notification row)."""
        self._notify("Clarity", "Explanation ready", "")

    def _retry(self, job_id: str) -> None:
        """Retry on a failed job: same capture, same question, new job."""
        submission = self._submissions.get(job_id)
        if submission is None:
            log.info("nothing to retry for %s", job_id)
            return
        data_url, prompt = submission
        try:
            new_id = self.client.create_job(
                data_url, user_prompt=prompt, guardrails=self.config.guardrails
            )
        except (Unreachable, ApiError) as exc:
            self._notify("Clarity", "Retry failed", str(exc))
            return
        log.info("retrying %s as %s", job_id, new_id)
        self._submissions[new_id] = submission
        self.open_result(new_id)

    def _idle(self) -> None:
        if self._on_all_closed is not None:
            self._on_all_closed()

    # -- housekeeping --------------------------------------------------------

    @property
    def busy(self) -> bool:
        with self._lock:
            return bool(self._results) or (self._spotlight is not None and self._spotlight.alive)

    def close_all(self) -> None:
        with self._lock:
            windows = list(self._results.values())
            spotlight = self._spotlight
        for window in windows:
            window.close()
        if spotlight is not None:
            spotlight.close()

    def check_server(self) -> tuple[bool, str | None]:
        """`GET /healthz` for launch and the Server… menu item (API.md §2.6).

        Returns (reachable, problem). The two are separate because a coordinator
        in fake mode answers `ok: false` — Atlas, Docker, S3 and the agent are
        all absent — while still walking a job through every status. Warning
        about that on every launch would cry wolf, so only the caller that the
        user asked for (Server…) reports a degraded server.
        """
        try:
            health = self.client.health()
        except Unreachable:
            return False, f"Can't reach {self.client.base_url}"
        if not health.get("ok"):
            down = [k for k in ("atlas", "docker", "s3", "agent") if health.get(k) is False]
            return True, "Server is degraded: " + (", ".join(down) if down else "not ready")
        return True, None

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
from . import window_host
from .capture import Capture
from .client import ApiError, Client, Unreachable
from .config import Config
from .overlay_window import Overlay
from .recents import Recents
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
        self.recents = Recents()
        self._notify = notify or _no_notify
        self._on_all_closed = on_all_closed

        self._lock = threading.Lock()
        self._spotlight: Spotlight | None = None
        # Full-screen dim + glow shown for as long as anything below is open
        # (FRD §15.1's "the screen tells you Clarity is working" affordance).
        self._overlay: Overlay | None = None
        # What the open spotlight box would submit. It changes when the user
        # picks a recent (FRD §16.5), so the submit reads it rather than
        # closing over the capture it was opened with.
        self._pending: Capture | None = None
        self._results: dict[str, ResultBox] = {}
        self._last_box: Box | None = None
        # Set between accepting a submit and the result box existing, so the
        # spotlight box closing in that gap doesn't look like "nothing is open".
        self._opening = False
        # The capture behind each job, so Retry can submit it again (FRD §19).
        self._submissions: dict[str, tuple[str, str]] = {}

    # -- the flow ------------------------------------------------------------

    def capture_and_ask(self) -> bool:
        """Region capture, then the spotlight box.

        The overlay goes up before the crosshair even appears, not after: it
        is excluded from screen capture itself (overlay_window.py), so it can
        safely be on screen for the whole selection without ending up in the
        image.

        Esc at the crosshair produces no capture. FRD §16.5 makes that the way
        into recents — but only when there is something to list, because a user
        who pressed Esc to back out shouldn't be handed an empty box.
        """
        self._ensure_overlay()
        cap = capture_mod.capture_region()
        if cap is None:
            log.info("capture cancelled")
            if self.recents.list():
                self.open_spotlight(None, expanded=True)
                return True
            self._close_overlay()
            return False
        log.info("captured %dx%d, %d bytes", cap.width, cap.height, cap.size_bytes)
        self.open_spotlight(cap)
        return True

    def open_spotlight(self, cap: Capture | None, expanded: bool = False) -> Spotlight:
        """Show the input box. Only one is ever open.

        `cap` is None for the Recents menu item and for Esc at the crosshair:
        the box opens with no thumbnail and the list already out, and it can't
        submit until the user picks a row.
        """
        self._ensure_overlay()
        with self._lock:
            existing = self._spotlight
        if existing is not None and existing.alive:
            existing.close()

        spotlight = Spotlight(
            thumbnail=capture_mod.thumbnail_data_url(cap.png_bytes) if cap else None,
            has_capture=cap is not None,
            on_submit=self._submit,
            on_close=self._spotlight_closed,
            on_open_recent=self._open_recent,
            on_pick_recent=self._pick_recent,
            expanded=expanded,
        )
        with self._lock:
            self._spotlight = spotlight
            self._pending = cap
        return spotlight

    def _submit(self, user_prompt: str) -> None:
        """Write the recent, POST the job, then open its result box. Runs on the
        spotlight window's reader thread."""
        with self._lock:
            spotlight = self._spotlight
            cap = self._pending
        if spotlight is None:
            return
        if cap is None:
            # The page guards this too; a window that raced the guard shouldn't
            # send an empty job.
            spotlight.show_error("Pick a recent screenshot first — press ↓.")
            return

        # Rule 21: on disk before the request goes out, so a submit that never
        # reaches the server still leaves the capture in recents.
        recent_id = self.recents.add(cap.data_url, question=user_prompt)

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
        if recent_id:
            self.recents.update(recent_id, job_id=job_id)
        anchor = spotlight.box
        with self._lock:
            self._opening = True
        try:
            # The box leaves first, then the result box arrives in its place
            # (FRD §16.2).
            spotlight.accept()
            self.open_result(job_id, anchor=anchor, recent_id=recent_id)
        finally:
            with self._lock:
                self._opening = False

    def open_result(
        self,
        job_id: str,
        anchor: Box | None = None,
        local: dict[str, Any] | None = None,
        recent_id: str | None = None,
    ) -> ResultBox:
        """Open a result box for a job. Boxes stack so several can stay open."""
        self._ensure_overlay()
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
            on_update_recent=self._update_recent,
            local=local,
            recent_id=recent_id,
        )
        with self._lock:
            self._results[job_id] = result
        return result

    # -- recents -------------------------------------------------------------

    def _open_recent(self, recent_id: str) -> None:
        """Enter on a row with a result: its result box opens from the local
        copy, whether or not the server still remembers the job (F43)."""
        entry = self.recents.get(recent_id)
        if entry is None:
            log.info("recent %s is gone", recent_id)
            return
        # A local-only entry still needs an id for the box's single poll; the
        # page treats the 404 that follows as expected (FRD §19).
        job_id = entry.job_id or f"local-{entry.id}"

        with self._lock:
            spotlight = self._spotlight
            already = self._results.get(job_id)
            # Boxes are keyed by job, so reopening one that is already on screen
            # would leave two boxes sharing a key, and the first to close would
            # make the app forget the other.
            reopening = already is not None and already.alive
            self._opening = not reopening

        if spotlight is not None:
            spotlight.accept()
        if reopening:
            log.info("recent %s is already open", entry.id)
            return

        try:
            self.open_result(
                job_id,
                anchor=spotlight.box if spotlight is not None else None,
                local=entry.local_result(),
                recent_id=entry.id,
            )
        finally:
            with self._lock:
                self._opening = False
        log.info("reopened recent %s (job %s) from local data", entry.id, job_id)

    def _pick_recent(self, recent_id: str) -> None:
        """Tab on a row, or Enter on one with no result: that screenshot becomes
        what the open box will submit (FRD §16.5)."""
        with self._lock:
            spotlight = self._spotlight
        if spotlight is None:
            return

        cap = self.recents.load_capture(recent_id)
        if cap is None:
            # The entry outlived its file (FRD §19). The row is already marked
            # in the list; say it plainly here too.
            spotlight.show_error("That screenshot is no longer on disk.")
            return

        with self._lock:
            self._pending = cap
        spotlight.set_capture(capture_mod.thumbnail_data_url(cap.png_bytes))
        log.info("recent %s loaded as the current capture", recent_id)

    def _update_recent(self, recent_id: str, fields: dict[str, Any]) -> None:
        """What a poll learned, written to the local entry (rule 21)."""
        self.recents.update(recent_id, **fields)

    def clear_recents(self) -> None:
        self.recents.clear()

    # -- window events -------------------------------------------------------

    def _spotlight_closed(self) -> None:
        with self._lock:
            self._spotlight = None
            self._pending = None
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
        recent_id = self.recents.add(data_url, question=prompt)
        try:
            new_id = self.client.create_job(
                data_url, user_prompt=prompt, guardrails=self.config.guardrails
            )
        except (Unreachable, ApiError) as exc:
            self._notify("Clarity", "Retry failed", str(exc))
            return
        log.info("retrying %s as %s", job_id, new_id)
        self._submissions[new_id] = submission
        if recent_id:
            self.recents.update(recent_id, job_id=new_id)
        self.open_result(new_id, recent_id=recent_id)

    def _idle(self) -> None:
        self._close_overlay()
        if self._on_all_closed is not None:
            self._on_all_closed()

    def _ensure_overlay(self) -> None:
        """Open the dim + glow overlay if it isn't already up. Best-effort: a
        window that fails to open is a missing visual flourish, not a reason
        to stop the capture flow (FRD §23 rule 19)."""
        with self._lock:
            existing = self._overlay
            if existing is not None and existing.alive:
                return
        try:
            index, width, height = window_host.cursor_screen()
            overlay = Overlay(index, width, height)
        except Exception:  # noqa: BLE001
            log.exception("could not open the overlay")
            return
        with self._lock:
            self._overlay = overlay

    def _close_overlay(self) -> None:
        with self._lock:
            overlay, self._overlay = self._overlay, None
        if overlay is not None:
            overlay.close()

    # -- housekeeping --------------------------------------------------------

    @property
    def busy(self) -> bool:
        with self._lock:
            return bool(self._results) or (self._spotlight is not None and self._spotlight.alive)

    def close_all(self) -> None:
        with self._lock:
            windows = list(self._results.values())
            spotlight = self._spotlight
            overlay = self._overlay
        for window in windows:
            window.close()
        if spotlight is not None:
            spotlight.close()
        if overlay is not None:
            overlay.close()

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

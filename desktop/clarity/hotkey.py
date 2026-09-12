"""Global hotkey — pynput GlobalHotKeys with a 2 s debounce.

FRD §15.1 (Hotkey row), F1. The listener callback must never block: the
handler is dispatched to a daemon worker thread and the listener returns
immediately.
"""

from __future__ import annotations

import logging
import threading
import time
from typing import Callable

from pynput import keyboard

log = logging.getLogger(__name__)

DEBOUNCE_SEC = 2.0


class Hotkey:
    """Register one global hotkey. `on_fire` runs on a worker thread."""

    def __init__(self, combo: str, on_fire: Callable[[], None], debounce_sec: float = DEBOUNCE_SEC) -> None:
        self.combo = combo
        self._on_fire = on_fire
        self._debounce = debounce_sec
        self._last_fired = 0.0
        self._lock = threading.Lock()
        self._listener: keyboard.GlobalHotKeys | None = None

    # -- lifecycle ---------------------------------------------------------

    def start(self) -> None:
        if self._listener is not None:
            return
        try:
            self._listener = keyboard.GlobalHotKeys({self.combo: self._handle})
            self._listener.daemon = True
            self._listener.start()
            log.info("hotkey %s registered", self.combo)
        except Exception:  # noqa: BLE001 — bad combo string or missing permission
            log.exception("could not register hotkey %s", self.combo)
            self._listener = None

    def stop(self) -> None:
        if self._listener is not None:
            try:
                self._listener.stop()
            finally:
                self._listener = None

    @property
    def active(self) -> bool:
        return self._listener is not None

    # -- internals ---------------------------------------------------------

    def _handle(self) -> None:
        now = time.monotonic()
        with self._lock:
            if now - self._last_fired < self._debounce:
                log.debug("hotkey debounced")
                return
            self._last_fired = now
        threading.Thread(target=self._safe_fire, name="clarity-hotkey", daemon=True).start()

    def _safe_fire(self) -> None:
        try:
            self._on_fire()
        except Exception:  # noqa: BLE001 — never let a capture error kill the listener
            log.exception("hotkey handler raised")

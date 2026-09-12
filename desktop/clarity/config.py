"""User configuration — ~/Library/Application Support/Clarity/config.json.

FRD §15.1 (Config row), §22 (Desktop keys), SETUP §12.3. No secrets live here:
only the coordinator URL and preferences (FRD §23 rule 17).
"""

from __future__ import annotations

import json
import os
import threading
from pathlib import Path
from typing import Any

APP_SUPPORT_DIR = Path.home() / "Library" / "Application Support" / "Clarity"
CONFIG_PATH = APP_SUPPORT_DIR / "config.json"
RECENTS_DIR = APP_SUPPORT_DIR / "recents"

DEFAULTS: dict[str, Any] = {
    "server_url": "http://localhost:8080",
    "guardrails": False,
    "hotkey": "<cmd>+<shift>+e",
}

_lock = threading.Lock()


class Config:
    """A small dict-backed config with typed properties. Loads on construction,
    creates the file with defaults on first run, and writes through on every set."""

    def __init__(self, path: Path = CONFIG_PATH) -> None:
        self.path = path
        self._data: dict[str, Any] = dict(DEFAULTS)
        self.load()

    # -- persistence -------------------------------------------------------

    def load(self) -> None:
        with _lock:
            try:
                raw = json.loads(self.path.read_text(encoding="utf-8"))
                if isinstance(raw, dict):
                    self._data.update({k: raw[k] for k in DEFAULTS if k in raw})
            except FileNotFoundError:
                self._write_locked()  # first run: create with defaults
            except (json.JSONDecodeError, OSError):
                # A corrupt file must never take the app down. Keep defaults and
                # rewrite so the next launch is clean.
                self._write_locked()

    def save(self) -> None:
        with _lock:
            self._write_locked()

    def _write_locked(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        tmp = self.path.with_suffix(".json.tmp")
        tmp.write_text(json.dumps(self._data, indent=2) + "\n", encoding="utf-8")
        os.replace(tmp, self.path)

    # -- typed accessors ---------------------------------------------------

    @property
    def server_url(self) -> str:
        return str(self._data["server_url"]).rstrip("/")

    @server_url.setter
    def server_url(self, value: str) -> None:
        value = (value or "").strip().rstrip("/")
        if not value:
            value = DEFAULTS["server_url"]
        if not value.startswith(("http://", "https://")):
            value = "http://" + value
        self._data["server_url"] = value
        self.save()

    @property
    def guardrails(self) -> bool:
        return bool(self._data["guardrails"])

    @guardrails.setter
    def guardrails(self, value: bool) -> None:
        self._data["guardrails"] = bool(value)
        self.save()

    @property
    def hotkey(self) -> str:
        return str(self._data["hotkey"]) or DEFAULTS["hotkey"]

    @hotkey.setter
    def hotkey(self, value: str) -> None:
        self._data["hotkey"] = value or DEFAULTS["hotkey"]
        self.save()

    def as_dict(self) -> dict[str, Any]:
        return dict(self._data)

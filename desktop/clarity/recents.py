"""The last 50 captures, on the user's Mac.

FRD §15.1 (Recents in the box, Reopen from recents), §19, §20 (nothing here is
ever uploaded unless the user re-asks), F41–F45, FRD §23 rule 21.
P4_DESKTOP Sprint 3 Step 2.

Three files per entry, all under `~/Library/Application Support/Clarity/`:

    recents.json                one object per entry, newest first
    recents/<id>.png            the downscaled capture, ready to re-submit
    recents/<id>_thumb.png      what the spotlight box's list draws

**Who writes.** Only the menu-bar app process. The window processes read —
the spotlight box for its list, the result box not at all — and every write a
window wants goes back to the app as an event (`window_host`), because two
processes rewriting one JSON file would lose entries.

**Why it's written before the POST.** Rule 21: a submit that never reaches the
server still leaves the capture in recents, and reopening a recent renders from
these files alone, so a job record that expired after 24 h doesn't matter.
"""

from __future__ import annotations

import base64
import json
import logging
import os
import secrets
import threading
from dataclasses import dataclass, replace
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from . import capture as capture_mod
from .capture import Capture
from .config import APP_SUPPORT_DIR, RECENTS_DIR

log = logging.getLogger(__name__)

RECENTS_PATH = APP_SUPPORT_DIR / "recents.json"

# FRD §15.1: the list holds 50; the oldest entry and its two files go when the
# 51st arrives.
MAX_ENTRIES = 50

# What a poll is allowed to add to an entry (rule 21). Anything else the result
# box sends is ignored rather than trusted.
UPDATABLE = ("job_id", "problem_hash", "problem_text", "explanation", "video_url", "question")

# The label a row shows when the server never told us what the problem was.
UNTITLED = "Untitled capture"

_write_lock = threading.Lock()


@dataclass(frozen=True)
class Recent:
    """One entry. Every field past `id` may be unset — an entry exists from the
    moment of the submit, long before the job has anything to say."""

    id: str
    created_at: str
    question: str = ""
    job_id: str | None = None
    problem_hash: str | None = None
    problem_text: str | None = None
    explanation: str | None = None
    video_url: str | None = None
    screenshot_path: str = ""
    thumb_path: str = ""

    @property
    def has_result(self) -> bool:
        """True when reopening this entry can show something without the server
        (FRD §15.1, Reopen from recents)."""
        return bool(self.explanation or self.video_url)

    @property
    def screenshot_exists(self) -> bool:
        """False after someone cleaned out the directory by hand. The row still
        lists — with its thumbnail — but re-asking is off (FRD §19)."""
        return bool(self.screenshot_path) and Path(self.screenshot_path).is_file()

    @property
    def label(self) -> str:
        """One line for the list: the problem, else the question, else nothing
        useful to say."""
        for text in (self.problem_text, self.question):
            first = (text or "").strip().splitlines()
            if first and first[0].strip():
                return first[0].strip()[:120]
        return UNTITLED

    def as_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "created_at": self.created_at,
            "question": self.question,
            "job_id": self.job_id,
            "problem_hash": self.problem_hash,
            "problem_text": self.problem_text,
            "explanation": self.explanation,
            "video_url": self.video_url,
            "screenshot_path": self.screenshot_path,
            "thumb_path": self.thumb_path,
        }

    def local_result(self) -> dict[str, Any]:
        """What the result box paints from before its first poll answers."""
        return {
            "job_id": self.job_id,
            "problem_text": self.problem_text,
            "explanation": self.explanation,
            "video_url": self.video_url,
        }

    @classmethod
    def from_dict(cls, raw: dict[str, Any]) -> "Recent | None":
        """None for anything that isn't a usable entry — a hand-edited or
        half-written file must not stop the list from opening."""
        recent_id = str(raw.get("id") or "").strip()
        if not recent_id:
            return None
        return cls(
            id=recent_id,
            created_at=str(raw.get("created_at") or ""),
            question=str(raw.get("question") or ""),
            job_id=_opt_str(raw.get("job_id")),
            problem_hash=_opt_str(raw.get("problem_hash")),
            problem_text=_opt_str(raw.get("problem_text")),
            explanation=_opt_str(raw.get("explanation")),
            video_url=_opt_str(raw.get("video_url")),
            screenshot_path=str(raw.get("screenshot_path") or ""),
            thumb_path=str(raw.get("thumb_path") or ""),
        )


def _opt_str(value: Any) -> str | None:
    if value is None:
        return None
    text = str(value)
    return text or None


class Recents:
    """The store. Cheap to construct — it reads on demand, so the app and each
    window process can each hold one."""

    def __init__(self, path: Path = RECENTS_PATH, directory: Path = RECENTS_DIR) -> None:
        self.path = path
        self.dir = directory

    # -- reading -------------------------------------------------------------

    def list(self) -> list[Recent]:
        """Newest first. Never raises: a missing or corrupt file is an empty
        list, which just means the box doesn't expand."""
        try:
            raw = json.loads(self.path.read_text(encoding="utf-8"))
        except FileNotFoundError:
            return []
        except (json.JSONDecodeError, OSError):
            log.warning("recents.json is unreadable; treating it as empty")
            return []
        if not isinstance(raw, list):
            return []
        entries = [Recent.from_dict(item) for item in raw if isinstance(item, dict)]
        return [e for e in entries if e is not None][:MAX_ENTRIES]

    def get(self, recent_id: str) -> Recent | None:
        for entry in self.list():
            if entry.id == recent_id:
                return entry
        return None

    def rows(self) -> list[dict[str, Any]]:
        """The list as the spotlight box needs it — label, relative time, a flag
        for a video, and the thumbnail inline, because the window process can't
        load a file:// image from a page served out of `ui/` (rule 20)."""
        rows: list[dict[str, Any]] = []
        for entry in self.list():
            rows.append(
                {
                    "id": entry.id,
                    "label": entry.label,
                    "question": entry.question,
                    "when": relative_time(entry.created_at),
                    "thumbnail": self._thumb_data_url(entry),
                    "has_result": entry.has_result,
                    "has_video": bool(entry.video_url),
                    "missing": not entry.screenshot_exists,
                }
            )
        return rows

    def load_capture(self, recent_id: str) -> Capture | None:
        """A stored screenshot as a fresh capture, for a new question about an
        old problem (FRD §16.5). None if the file is gone (FRD §19)."""
        entry = self.get(recent_id)
        if entry is None or not entry.screenshot_exists:
            log.info("recent %s has no screenshot on disk", recent_id)
            return None
        path = Path(entry.screenshot_path)
        try:
            png_bytes = path.read_bytes()
            width, height = _png_size(png_bytes)
        except (OSError, ValueError):
            log.exception("could not read the screenshot for recent %s", recent_id)
            return None
        return Capture(
            data_url=capture_mod.to_data_url(png_bytes),
            png_bytes=png_bytes,
            width=width,
            height=height,
            raw_path=path,
        )

    def _thumb_data_url(self, entry: Recent) -> str | None:
        try:
            return capture_mod.to_data_url(Path(entry.thumb_path).read_bytes())
        except OSError:
            return None

    # -- writing (app process only) ------------------------------------------

    def add(self, image_data_url: str, question: str = "") -> str | None:
        """Write a capture and its thumbnail, prepend the entry, drop the 51st.

        Called immediately before `POST /api/jobs` (rule 21). None means nothing
        could be written — the submit still goes ahead; recents are a
        convenience, never a precondition.
        """
        try:
            png_bytes = _decode_data_url(image_data_url)
        except ValueError:
            log.warning("not an image data URL; no recent written")
            return None

        recent_id = _new_id()
        screenshot = self.dir / f"{recent_id}.png"
        thumb = self.dir / f"{recent_id}_thumb.png"

        try:
            self.dir.mkdir(parents=True, exist_ok=True)
            screenshot.write_bytes(png_bytes)
            thumb.write_bytes(capture_mod.thumbnail_png(png_bytes))
        except Exception:  # noqa: BLE001 — a full disk, or Pillow on an odd PNG
            log.exception("could not write recent %s", recent_id)
            _unlink(screenshot, thumb)
            return None

        entry = Recent(
            id=recent_id,
            created_at=datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z"),
            question=question or "",
            screenshot_path=str(screenshot),
            thumb_path=str(thumb),
        )

        with _write_lock:
            entries = self.list()
            entries.insert(0, entry)
            dropped = entries[MAX_ENTRIES:]
            self._save_locked(entries[:MAX_ENTRIES])
        for old in dropped:
            _unlink(Path(old.screenshot_path), Path(old.thumb_path))
        log.info("recent %s written (%d entries)", recent_id, min(len(entries), MAX_ENTRIES))
        return recent_id

    def update(self, recent_id: str, **fields: Any) -> bool:
        """Merge what a poll learned into an entry (rule 21).

        The result box polls once a second, so this only touches the disk when
        something actually changed. Unknown keys are dropped; a value already
        set is never replaced with `None`, because a later poll must not erase
        an explanation the user is reading.
        """
        changes = {k: v for k, v in fields.items() if k in UPDATABLE and v not in (None, "")}
        if not recent_id or not changes:
            return False

        with _write_lock:
            entries = self.list()
            for index, entry in enumerate(entries):
                if entry.id != recent_id:
                    continue
                merged = replace(entry, **changes)
                if merged == entry:
                    return False
                entries[index] = merged
                self._save_locked(entries)
                log.debug("recent %s updated: %s", recent_id, sorted(changes))
                return True
        log.debug("recent %s is gone; nothing to update", recent_id)
        return False

    def clear(self) -> None:
        """The Clear Recents menu item (F45, FRD §20): the list and every file."""
        with _write_lock:
            entries = self.list()
            self._save_locked([])
        for entry in entries:
            _unlink(Path(entry.screenshot_path), Path(entry.thumb_path))
        # Anything left behind is a file no entry claims — a crash mid-add, or a
        # leftover from an older layout. The user asked for all of it to go.
        try:
            for stray in self.dir.glob("*.png"):
                stray.unlink(missing_ok=True)
        except OSError:
            log.exception("could not clear %s", self.dir)
        log.info("recents cleared (%d entries)", len(entries))

    def _save_locked(self, entries: list[Recent]) -> None:
        payload = json.dumps([e.as_dict() for e in entries], indent=2) + "\n"
        try:
            self.path.parent.mkdir(parents=True, exist_ok=True)
            tmp = self.path.with_suffix(".json.tmp")
            tmp.write_text(payload, encoding="utf-8")
            os.replace(tmp, self.path)
        except OSError:
            log.exception("could not write %s", self.path)


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #


def _new_id() -> str:
    """Sortable by eye in the directory listing, and unique within a second."""
    return datetime.now().strftime("%Y%m%d-%H%M%S") + "-" + secrets.token_hex(3)


def _decode_data_url(data_url: str) -> bytes:
    head, _, payload = (data_url or "").partition(",")
    if not payload or not head.startswith("data:image/"):
        raise ValueError("not an image data URL")
    try:
        return base64.b64decode(payload, validate=True)
    except (ValueError, TypeError) as exc:
        raise ValueError("data URL payload isn't base64") from exc


def _png_size(png_bytes: bytes) -> tuple[int, int]:
    """Width and height from the IHDR chunk, so re-asking doesn't pay for a
    Pillow decode of an image we already have bytes for."""
    if len(png_bytes) < 24 or png_bytes[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    width = int.from_bytes(png_bytes[16:20], "big")
    height = int.from_bytes(png_bytes[20:24], "big")
    return width, height


def _unlink(*paths: Path) -> None:
    for path in paths:
        try:
            if str(path):
                path.unlink(missing_ok=True)
        except OSError:
            log.debug("could not delete %s", path, exc_info=True)


def relative_time(created_at: str) -> str:
    """"2 h ago" for the list (FRD §15.1). Empty for an unparseable stamp —
    a row with no time still lists."""
    try:
        when = datetime.fromisoformat((created_at or "").replace("Z", "+00:00"))
    except ValueError:
        return ""
    if when.tzinfo is None:
        when = when.replace(tzinfo=timezone.utc)

    seconds = (datetime.now(timezone.utc) - when).total_seconds()
    if seconds < 0:
        return "just now"
    if seconds < 60:
        return "just now"
    minutes = seconds / 60
    if minutes < 60:
        return f"{int(minutes)} min ago"
    hours = minutes / 60
    if hours < 24:
        return f"{int(hours)} h ago"
    days = hours / 24
    if days < 7:
        return f"{int(days)} d ago"
    return when.astimezone().strftime("%b %-d")

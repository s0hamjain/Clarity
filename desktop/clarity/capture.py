"""Region screenshot → downscaled PNG data URL.

FRD §15.1 (Capture, Image prep rows), F1, F2. Uses the built-in macOS
`screencapture -i` for region select. Esc at the crosshair makes screencapture
exit 1 with no file — that is the user cancelling, not an error, and returns None.
"""

from __future__ import annotations

import base64
import io
import logging
import os
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from PIL import Image

log = logging.getLogger(__name__)

# Longest side after downscale. Keeps retina captures well under the 8 MB API
# limit and matches the vision model's sweet spot (FRD §15.1).
MAX_SIDE = 1568

# Hard cap from API.md §2.1: the decoded image must be ≤ 8 MB.
MAX_BYTES = 8 * 1024 * 1024

# How long the user may sit at the crosshair before we give up.
CAPTURE_TIMEOUT_SEC = 120


@dataclass(frozen=True)
class Capture:
    """One successful region capture."""

    data_url: str          # data:image/png;base64,…  — what goes to POST /api/jobs
    png_bytes: bytes       # the downscaled PNG, for writing the recent
    width: int
    height: int
    raw_path: Path         # the original screencapture output on disk (caller may delete)

    @property
    def size_bytes(self) -> int:
        return len(self.png_bytes)


def _run_screencapture(out_path: Path) -> bool:
    """Run interactive region select. True if a file was produced."""
    try:
        proc = subprocess.run(
            ["screencapture", "-i", "-x", str(out_path)],
            capture_output=True,
            timeout=CAPTURE_TIMEOUT_SEC,
            check=False,
        )
    except FileNotFoundError:
        log.error("screencapture not found — not macOS?")
        return False
    except subprocess.TimeoutExpired:
        log.info("screencapture timed out waiting for a selection")
        return False

    if proc.returncode != 0 or not out_path.exists() or out_path.stat().st_size == 0:
        # Exit code 1 + no file is Esc at the crosshair. Silent abort.
        if proc.stderr:
            log.debug("screencapture stderr: %s", proc.stderr.decode(errors="replace").strip())
        return False
    return True


def _prepare(raw_path: Path) -> tuple[bytes, int, int]:
    """Downscale to ≤ MAX_SIDE on the longest side and re-encode as PNG."""
    with Image.open(raw_path) as im:
        im.load()
        # Drop alpha to RGB; screenshots are opaque and RGB PNGs are smaller.
        if im.mode not in ("RGB", "L"):
            im = im.convert("RGB")
        im.thumbnail((MAX_SIDE, MAX_SIDE), Image.Resampling.LANCZOS)
        buf = io.BytesIO()
        im.save(buf, format="PNG", optimize=True)
        return buf.getvalue(), im.width, im.height


def to_data_url(png_bytes: bytes) -> str:
    return "data:image/png;base64," + base64.b64encode(png_bytes).decode("ascii")


def capture_region(keep_raw: bool = False) -> Capture | None:
    """Interactive region capture.

    Returns None when the user pressed Esc (or nothing was captured). Never raises
    for user-facing reasons — FRD §23 rule 19.
    """
    tmp_dir = Path(tempfile.mkdtemp(prefix="clarity-"))
    raw_path = tmp_dir / "capture.png"

    if not _run_screencapture(raw_path):
        _cleanup(tmp_dir)
        return None

    try:
        png_bytes, w, h = _prepare(raw_path)
    except Exception:  # noqa: BLE001 — a bad image must not crash the app
        log.exception("failed to process the screenshot")
        _cleanup(tmp_dir)
        return None

    if len(png_bytes) > MAX_BYTES:
        # Should be unreachable at 1568 px, but the API will reject it, so say so.
        log.warning("capture is %d bytes, over the 8 MB limit", len(png_bytes))

    cap = Capture(
        data_url=to_data_url(png_bytes),
        png_bytes=png_bytes,
        width=w,
        height=h,
        raw_path=raw_path,
    )
    if not keep_raw:
        _cleanup(tmp_dir)
    return cap


def _cleanup(tmp_dir: Path) -> None:
    try:
        for p in tmp_dir.iterdir():
            p.unlink(missing_ok=True)
        os.rmdir(tmp_dir)
    except OSError:
        pass

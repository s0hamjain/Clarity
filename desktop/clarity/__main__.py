"""`python -m clarity` — run the menu-bar app.

    python -m clarity              menu bar icon appears; the hotkey is live
    python -m clarity --once       one region capture, print the result, exit
    python -m clarity --once --save out.png
                                   same, and write the downscaled PNG to disk
"""

from __future__ import annotations

import argparse
import logging
import sys
from pathlib import Path


def _parse(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(prog="clarity", description="Clarity menu-bar app")
    p.add_argument("--once", action="store_true", help="run one capture without the hotkey and exit")
    p.add_argument("--save", metavar="PATH", help="with --once: write the downscaled PNG here")
    p.add_argument("-v", "--verbose", action="store_true", help="debug logging")
    return p.parse_args(argv)


def _run_once(save: str | None) -> int:
    from . import capture

    cap = capture.capture_region()
    if cap is None:
        print("cancelled (Esc at the crosshair, or nothing captured)")
        return 1

    mb = cap.size_bytes / (1024 * 1024)
    print(f"captured {cap.width}x{cap.height}, {cap.size_bytes} bytes ({mb:.2f} MB) as PNG")
    print(f"data URL: {cap.data_url[:48]}… ({len(cap.data_url)} chars)")
    if cap.size_bytes > capture.MAX_BYTES:
        print("WARNING: over the 8 MB API limit", file=sys.stderr)

    if save:
        out = Path(save).expanduser()
        out.write_bytes(cap.png_bytes)
        print(f"saved {out}")
    return 0


def main(argv: list[str] | None = None) -> int:
    args = _parse(sys.argv[1:] if argv is None else argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)-7s %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )

    if args.once:
        return _run_once(args.save)

    from .app import main as app_main

    app_main()
    return 0


if __name__ == "__main__":
    sys.exit(main())

"""`python -m clarity` — run the menu-bar app.

This module is also PyInstaller's entry script (`scripts/build_app.sh`), which
compiles it as `__main__` with no parent package — so every import here is
absolute. The rest of the package uses relative imports as usual.

    python -m clarity              menu bar icon appears; the hotkey is live
    python -m clarity --once       one capture through the whole flow — spotlight
                                   box, submit, result box — then exit
    python -m clarity --once --no-ask
                                   just the capture: print its size and exit
    python -m clarity --once --save out.png
                                   same, and write the downscaled PNG to disk
    python -m clarity --recents    the spotlight box with no capture and the
                                   recents list open — works with no server
"""

from __future__ import annotations

import argparse
import logging
import sys
from pathlib import Path


def _parse(argv: list[str]) -> argparse.Namespace:
    # LaunchServices can append a `-psn_0_…` process serial number when it opens
    # an .app. argparse would exit(2) on it, and a windowed bundle has nowhere to
    # show the error, so the app would just never appear.
    argv = [a for a in argv if not a.startswith("-psn_")]

    p = argparse.ArgumentParser(prog="clarity", description="Clarity menu-bar app")
    p.add_argument("--once", action="store_true", help="run one capture without the hotkey and exit")
    p.add_argument(
        "--no-ask",
        action="store_true",
        help="with --once: stop after the capture instead of opening the spotlight box",
    )
    p.add_argument("--save", metavar="PATH", help="with --once: write the downscaled PNG here")
    p.add_argument(
        "--recents",
        action="store_true",
        help="open the spotlight box on the recents list, without a capture, and exit when it closes",
    )
    p.add_argument("-v", "--verbose", action="store_true", help="debug logging")
    # Not for people. The app re-runs itself with this to put each window in its
    # own process, because rumps and pywebview can't share a main thread
    # (clarity/window_host.py).
    p.add_argument("--window-host", metavar="KIND", choices=("spotlight", "result", "overlay"), help=argparse.SUPPRESS)
    return p.parse_args(argv)


def _run_session(open_windows) -> int:
    """Run the flow outside the menu-bar app and wait for the last window.

    Every window is its own process, so this thread only has to wait for them
    all to close (`clarity/window_host.py`).
    """
    import threading

    from clarity.config import Config
    from clarity.session import Session

    config = Config()
    idle = threading.Event()
    session = Session(config, on_all_closed=idle.set)
    print(f"coordinator: {config.server_url}")
    open_windows(session)

    try:
        idle.wait()
    except KeyboardInterrupt:
        session.close_all()
    return 0


def _run_recents() -> int:
    """The recents list on its own — the one path that needs no server and no
    capture (FRD §16.5)."""
    from clarity.recents import Recents

    entries = Recents().list()
    print(f"{len(entries)} recent(s)")
    if not entries:
        print("nothing to list yet; run --once first")
        return 1
    for entry in entries[:8]:
        mark = "•" if entry.video_url else " "
        missing = "" if entry.screenshot_exists else "  (screenshot missing)"
        print(f" {mark} {entry.id}  {entry.label[:56]}{missing}")
    return _run_session(lambda session: session.open_spotlight(None, expanded=True))


def _run_once(save: str | None, ask: bool) -> int:
    """One capture without the hotkey — the end-to-end test hook.

    With `ask` (the default) it runs the real flow: spotlight box, submit to the
    coordinator, result box, and it waits until the last window is closed.
    """
    from clarity import capture

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

    if not ask:
        return 0

    return _run_session(lambda session: session.open_spotlight(cap))


def main(argv: list[str] | None = None) -> int:
    args = _parse(sys.argv[1:] if argv is None else argv)

    if args.window_host:
        # A window process: it sets up its own logging to stderr, because stdout
        # is the protocol back to the app.
        from clarity.window_host import run

        return run(args.window_host)

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)-7s %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )

    if args.recents:
        return _run_recents()

    if args.once:
        return _run_once(args.save, ask=not args.no_ask)

    from clarity.app import main as app_main

    app_main()
    return 0


if __name__ == "__main__":
    sys.exit(main())

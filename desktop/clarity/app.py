"""The menu-bar app.

FRD §5 (Menu bar icon row), §15.1. Menu: Capture · Recents · Guardrails ☐ ·
Server… · Clear Recents · Quit. Sprint 1 wires Capture, Guardrails, Server…
and Quit; Recents / Clear Recents are placeholders until Sprint 3's recents
store lands, and the spotlight/result windows arrive in Sprint 2.
"""

from __future__ import annotations

import logging
import shutil
import threading
from pathlib import Path

import rumps

from . import __version__, capture as capture_mod
from .config import RECENTS_DIR, Config
from .hotkey import Hotkey

log = logging.getLogger(__name__)

ASSETS_DIR = Path(__file__).resolve().parent.parent / "assets"
ICON_IDLE = ASSETS_DIR / "menubar_idle.png"
ICON_WORKING = ASSETS_DIR / "menubar_working.png"


def _icon(path: Path) -> str | None:
    return str(path) if path.exists() else None


class ClarityApp(rumps.App):
    def __init__(self, config: Config | None = None) -> None:
        self.config = config or Config()
        super().__init__(
            "Clarity",
            icon=_icon(ICON_IDLE),
            template=True,
            quit_button=None,  # we add our own Quit so it sits last
        )

        self._capturing = threading.Lock()
        self._build_menu()

        self.hotkey = Hotkey(self.config.hotkey, self.on_hotkey)
        self.hotkey.start()
        if not self.hotkey.active:
            rumps.notification(
                "Clarity",
                "Hotkey not registered",
                "Grant Input Monitoring to your terminal (or Clarity.app) and relaunch.",
            )

    # -- menu ----------------------------------------------------------------

    def _build_menu(self) -> None:
        self.item_capture = rumps.MenuItem(f"Capture\t{self._hotkey_label()}", callback=self.on_capture)
        self.item_recents = rumps.MenuItem("Recents", callback=self.on_recents)
        self.item_guardrails = rumps.MenuItem("Guardrails", callback=self.on_toggle_guardrails)
        self.item_guardrails.state = self.config.guardrails
        self.item_server = rumps.MenuItem("Server…", callback=self.on_server)
        self.item_clear = rumps.MenuItem("Clear Recents", callback=self.on_clear_recents)
        self.item_quit = rumps.MenuItem("Quit Clarity", callback=self.on_quit, key="q")

        self.menu = [
            self.item_capture,
            self.item_recents,
            None,
            self.item_guardrails,
            self.item_server,
            None,
            self.item_clear,
            None,
            self.item_quit,
        ]

    def _hotkey_label(self) -> str:
        pretty = (
            self.config.hotkey.replace("<cmd>", "⌘")
            .replace("<shift>", "⇧")
            .replace("<alt>", "⌥")
            .replace("<ctrl>", "⌃")
            .replace("+", "")
        )
        return pretty.upper()

    # -- state ---------------------------------------------------------------

    def _set_working(self, working: bool) -> None:
        icon = _icon(ICON_WORKING if working else ICON_IDLE)
        if icon:
            self.icon = icon

    # -- actions -------------------------------------------------------------

    def on_hotkey(self) -> None:
        """Called on the hotkey worker thread (never the listener thread)."""
        self.run_capture()

    def on_capture(self, _sender: rumps.MenuItem) -> None:
        threading.Thread(target=self.run_capture, name="clarity-capture", daemon=True).start()

    def run_capture(self) -> capture_mod.Capture | None:
        """The whole Sprint 1 pipeline: region select → downscale → data URL.

        Sprint 2 hands the Capture to the spotlight box instead of logging it.
        """
        if not self._capturing.acquire(blocking=False):
            log.info("capture already in progress; ignoring")
            return None
        try:
            self._set_working(True)
            cap = capture_mod.capture_region()
            if cap is None:
                log.info("capture cancelled")
                return None
            log.info("captured %dx%d, %d bytes as PNG", cap.width, cap.height, cap.size_bytes)
            # TODO(sprint 2): open the spotlight box with this capture.
            return cap
        except Exception:  # noqa: BLE001 — FRD §23 rule 19
            log.exception("capture failed")
            rumps.notification("Clarity", "Capture failed", "Check Screen Recording permission and try again.")
            return None
        finally:
            self._set_working(False)
            self._capturing.release()

    def on_recents(self, _sender: rumps.MenuItem) -> None:
        # Sprint 3: open the spotlight box with the recents list expanded.
        rumps.notification("Clarity", "Recents", "Coming in a later sprint.")

    def on_toggle_guardrails(self, sender: rumps.MenuItem) -> None:
        sender.state = not sender.state
        self.config.guardrails = bool(sender.state)
        log.info("guardrails = %s", self.config.guardrails)

    def on_server(self, _sender: rumps.MenuItem) -> None:
        win = rumps.Window(
            message="Coordinator URL",
            title="Clarity — Server",
            default_text=self.config.server_url,
            ok="Save",
            cancel="Cancel",
            dimensions=(320, 24),
        )
        resp = win.run()
        if resp.clicked:
            self.config.server_url = resp.text
            log.info("server_url = %s", self.config.server_url)

    def on_clear_recents(self, _sender: rumps.MenuItem) -> None:
        # Sprint 3 replaces this with recents.clear(); for now wipe the directory
        # so the menu item does what it says from day one.
        try:
            if RECENTS_DIR.exists():
                shutil.rmtree(RECENTS_DIR)
            rumps.notification("Clarity", "Recents cleared", "")
        except OSError:
            log.exception("could not clear recents")

    def on_quit(self, _sender: rumps.MenuItem) -> None:
        self.hotkey.stop()
        rumps.quit_application()


def main() -> None:
    log.info("Clarity %s starting", __version__)
    ClarityApp().run()

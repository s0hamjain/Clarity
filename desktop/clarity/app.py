"""The menu-bar app.

FRD §5 (Menu bar icon row), §15.1. Menu: Capture · Recents · Guardrails ☐ ·
Server… · Clear Recents · Quit. Every action that touches a window or the
network runs through `session.Session`, so `python -m clarity --once` drives
exactly the same code as the menu.
"""

from __future__ import annotations

import logging
import threading
from pathlib import Path

import rumps

from . import __version__
from .config import Config
from .hotkey import Hotkey
from .session import Session

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

        # The visible "Clarity is running" signal lives in the Dock, not the
        # menu bar: a Dock icon carries the OS's own running indicator (the
        # dot under the icon) for as long as this process is alive, without
        # us drawing anything ourselves. LSUIElement in Info.plist still
        # starts the app as an accessory (no Dock icon, no Cmd-Tab entry) so
        # a plain `python -m clarity` or a debug run doesn't surprise anyone;
        # this promotes it at launch. window_host.py's child processes
        # (spotlight/result/overlay) explicitly do the opposite — they stay
        # accessory — so only this one Dock icon ever appears.
        try:
            import AppKit

            AppKit.NSApplication.sharedApplication().setActivationPolicy_(
                AppKit.NSApplicationActivationPolicyRegular
            )
        except Exception:  # noqa: BLE001 — cosmetic; the app still runs from the menu bar
            log.debug("could not show the Dock icon", exc_info=True)

        self._capturing = threading.Lock()
        self.session = Session(self.config, notify=self._notify, on_all_closed=self._on_idle)
        self._build_menu()

        self.hotkey = Hotkey(self.config.hotkey, self.on_hotkey)
        self.hotkey.start()
        if not self.hotkey.active:
            self._notify(
                "Clarity",
                "Hotkey not registered",
                "Grant Input Monitoring to your terminal (or Clarity.app) and relaunch.",
            )

        # FRD §15.1: check the coordinator on launch, so the first capture isn't
        # where the user finds out it's down.
        threading.Thread(target=self._check_server, name="clarity-healthz", daemon=True).start()

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

    def run_capture(self) -> bool:
        """Region select → downscale → spotlight box (FRD §16.2)."""
        if not self._capturing.acquire(blocking=False):
            log.info("capture already in progress; ignoring")
            return False
        try:
            self._set_working(True)
            return self.session.capture_and_ask()
        except Exception:  # noqa: BLE001 — FRD §23 rule 19
            log.exception("capture failed")
            self._notify("Clarity", "Capture failed", "Check Screen Recording permission and try again.")
            return False
        finally:
            self._set_working(self.session.busy)
            self._capturing.release()

    def on_recents(self, _sender: rumps.MenuItem) -> None:
        """The spotlight box with no capture and the list already out (FRD §16.5)."""
        threading.Thread(target=self.show_recents, name="clarity-recents", daemon=True).start()

    def show_recents(self) -> None:
        try:
            if not self.session.recents.list():
                self._notify("Clarity", "No recent captures yet", "Press ⌘⇧E to capture a problem.")
                return
            self._set_working(True)
            self.session.open_spotlight(None, expanded=True)
        except Exception:  # noqa: BLE001 — FRD §23 rule 19
            log.exception("could not open recents")
            self._notify("Clarity", "Couldn't open recents", "")
        finally:
            self._set_working(self.session.busy)

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
        # Either way, report what's at that URL now (FRD §15.1, Config row).
        threading.Thread(target=self._check_server, args=(True,), name="clarity-healthz", daemon=True).start()

    def on_clear_recents(self, _sender: rumps.MenuItem) -> None:
        """Every screenshot Clarity kept, gone (F45, FRD §20)."""
        try:
            self.session.clear_recents()
            self._notify("Clarity", "Recents cleared", "")
        except Exception:  # noqa: BLE001
            log.exception("could not clear recents")
            self._notify("Clarity", "Couldn't clear recents", "")

    def on_quit(self, _sender: rumps.MenuItem) -> None:
        self.hotkey.stop()
        self.session.close_all()  # window processes outlive us otherwise
        rumps.quit_application()

    # -- server --------------------------------------------------------------

    def _check_server(self, asked: bool = False) -> None:
        """`asked` is true when the user chose Server… — then say something
        either way. On launch, only an unreachable server is worth a
        notification (see Session.check_server)."""
        reachable, problem = self.session.check_server()
        if not reachable:
            log.warning("%s", problem)
            self._notify("Clarity", problem or "Can't reach the server", "Set the URL with Server…")
        elif problem:
            log.info("%s", problem)
            if asked:
                self._notify("Clarity", problem, self.config.server_url)
        else:
            log.info("coordinator ok at %s", self.config.server_url)
            if asked:
                self._notify("Clarity", "Server is up", self.config.server_url)

    # -- helpers -------------------------------------------------------------

    def _on_idle(self) -> None:
        self._set_working(False)

    @staticmethod
    def _notify(title: str, subtitle: str, message: str) -> None:
        """A notification must never be the thing that crashes the app — it
        fails when the app isn't bundled, among other reasons (FRD §23 rule 19)."""
        try:
            rumps.notification(title, subtitle, message)
        except Exception:  # noqa: BLE001
            log.info("notification (not shown): %s — %s %s", title, subtitle, message)


def main() -> None:
    log.info("Clarity %s starting", __version__)
    ClarityApp().run()

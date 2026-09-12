"""The coordinator client — the only thing in this app that talks to a network.

API.md §2. Four calls: create a job, poll it, cancel it, check health. Nothing
here raises for a reason the user should see; callers get either a value or one
of the two exceptions below, and FRD §23 rule 19 turns those into a
notification or a line in the result box.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any

import requests

log = logging.getLogger(__name__)

# API.md §7. Generous on POST because the image is up to 8 MB over loopback;
# tight on polling because a poll happens every second and a slow one should be
# skipped rather than queue up behind the next.
POST_TIMEOUT = (5, 30)
POLL_TIMEOUT = (2, 8)
CANCEL_TIMEOUT = (2, 5)
HEALTH_TIMEOUT = (2, 5)

MAX_PROMPT_CHARS = 2000  # API.md §7 — the coordinator answers 400 past this


class Unreachable(Exception):
    """The coordinator isn't answering — wrong URL, not running, or loopback is
    blocked. The desktop app shows "Can't reach the server" for this one."""


@dataclass(frozen=True)
class ApiError(Exception):
    """A non-2xx answer that carried the standard error envelope (API.md §4)."""

    status: int
    code: str
    message: str
    details: dict[str, Any] | None = None

    def __str__(self) -> str:
        return f"{self.code}: {self.message}"

    @property
    def is_not_found(self) -> bool:
        return self.status == 404


class Client:
    """Talks to one coordinator. The URL is read fresh from Config on every call
    so the Server… menu item takes effect without a relaunch."""

    def __init__(self, base_url_getter) -> None:
        self._base_url = base_url_getter
        self._session = requests.Session()

    @property
    def base_url(self) -> str:
        return self._base_url().rstrip("/")

    # -- calls ---------------------------------------------------------------

    def create_job(
        self,
        image_data_url: str,
        user_prompt: str = "",
        guardrails: bool = False,
        source: str = "desktop",
    ) -> str:
        """POST /api/jobs → job_id. API.md §2.1."""
        body = {
            "image": image_data_url,
            "user_prompt": (user_prompt or "")[:MAX_PROMPT_CHARS],
            "guardrails": bool(guardrails),
            "source": source,
        }
        data = self._request("POST", "/api/jobs", json=body, timeout=POST_TIMEOUT)
        job_id = data.get("job_id")
        if not job_id:
            raise ApiError(502, "internal", "The server accepted the job but returned no job_id.")
        return str(job_id)

    def get_job(self, job_id: str) -> dict[str, Any]:
        """GET /api/jobs/{id}. API.md §2.2."""
        return self._request("GET", f"/api/jobs/{job_id}", timeout=POLL_TIMEOUT)

    def cancel_job(self, job_id: str) -> None:
        """DELETE /api/jobs/{id}, fire and forget. API.md §2.3.

        The user has already closed the window; there is nothing to report, so
        every failure here is swallowed after a log line.
        """
        try:
            self._request("DELETE", f"/api/jobs/{job_id}", timeout=CANCEL_TIMEOUT)
            log.info("cancelled %s", job_id)
        except (Unreachable, ApiError) as exc:
            log.info("cancel %s failed, ignoring: %s", job_id, exc)

    def health(self) -> dict[str, Any]:
        """GET /healthz. API.md §2.6. Answers 503 with a body when not ok, so a
        503 is data rather than an error."""
        try:
            resp = self._session.get(f"{self.base_url}/healthz", timeout=HEALTH_TIMEOUT)
        except requests.RequestException as exc:
            raise Unreachable(str(exc)) from exc
        try:
            data = resp.json()
        except ValueError:
            raise Unreachable(f"{self.base_url} answered {resp.status_code}, but not with JSON") from None
        return data if isinstance(data, dict) else {"ok": False}

    # -- plumbing ------------------------------------------------------------

    def _request(self, method: str, path: str, *, timeout, json: dict | None = None) -> dict[str, Any]:
        url = f"{self.base_url}{path}"
        try:
            resp = self._session.request(
                method,
                url,
                json=json,
                timeout=timeout,
                headers={"Accept": "application/json"},
            )
        except requests.RequestException as exc:
            # Connection refused, DNS, timeout — all "can't reach the server".
            log.warning("%s %s unreachable: %s", method, url, exc)
            raise Unreachable(str(exc)) from exc

        if resp.status_code >= 400:
            raise self._envelope(resp)

        try:
            data = resp.json()
        except ValueError as exc:
            raise ApiError(resp.status_code, "internal", "The server's reply wasn't JSON.") from exc
        return data if isinstance(data, dict) else {}

    @staticmethod
    def _envelope(resp: requests.Response) -> ApiError:
        """Pull code/message out of API.md §4's envelope, falling back to the
        status line when a proxy or a crash answers with something else."""
        code, message, details = "internal", f"The server answered {resp.status_code}.", None
        try:
            err = resp.json().get("error") or {}
            code = str(err.get("code") or code)
            message = str(err.get("message") or message)
            raw_details = err.get("details")
            details = raw_details if isinstance(raw_details, dict) else None
        except (ValueError, AttributeError):
            pass
        return ApiError(resp.status_code, code, message, details)

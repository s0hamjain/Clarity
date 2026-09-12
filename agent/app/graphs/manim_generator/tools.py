from typing import Optional
from pydantic import BaseModel
import httpx
from app.config import settings


class RenderResult(BaseModel):
    ok: bool
    clip_path: Optional[str] = None
    duration_seconds: Optional[float] = None
    stage: Optional[str] = None
    traceback: Optional[str] = None


def render_tool(source: str, work_dir: str, quality: str = "-ql") -> RenderResult:
    """Invokes the coordinator internal render tool endpoint POST /internal/render."""
    url = f"{settings.COORDINATOR_URL}/internal/render"
    payload = {
        "source": source,
        "work_dir": work_dir,
        "quality": quality,
    }

    try:
        resp = httpx.post(url, json=payload, timeout=130.0)
        if resp.status_code == 200:
            data = resp.json()
            return RenderResult(
                ok=data.get("ok", False),
                clip_path=data.get("clip_path"),
                duration_seconds=data.get("duration_seconds"),
                stage=data.get("stage"),
                traceback=data.get("traceback"),
            )
        else:
            return RenderResult(
                ok=False,
                stage="coordinator_error",
                traceback=f"HTTP {resp.status_code} from /internal/render: {resp.text}",
            )
    except (httpx.ConnectError, httpx.TimeoutException, httpx.RequestError) as e:
        return RenderResult(
            ok=False,
            stage="coordinator_unreachable",
            traceback=f"Failed to reach coordinator /internal/render at {url}: {e}",
        )

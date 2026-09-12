from fastapi import APIRouter
from app.schemas import SceneRenderRequest, SceneRenderResponse

router = APIRouter()


@router.post("/scenes/render", response_model=SceneRenderResponse)
async def render_scene(req: SceneRenderRequest):
    """POST /scenes/render - Manim Generator agent endpoint (Stubbed for Sprint 1)."""
    return SceneRenderResponse(
        ok=True,
        clip_path=f"{req.work_dir}/scene{req.scene.index}.mp4",
        attempts=1,
        lint_retries=0,
        snippets_used=[],
        snippet_id=None,
    )

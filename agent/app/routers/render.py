from fastapi import APIRouter, HTTPException
from app.graphs.manim_generator.graph import manim_generator_graph
from app.graphs.manim_generator.state import RenderState
from app.schemas import RenderRequest, RenderResponse

router = APIRouter()


@router.post("/render", response_model=RenderResponse)
async def render_video(req: RenderRequest):
    """POST /render - Manim Generator agent endpoint: retrieve -> generate ->
    lint -> render -> repair -> ingest, over the whole storyboard as one
    continuous script (not one call per scene — see FRD §10.4)."""
    try:
        initial_state = RenderState(
            job_id=req.job_id,
            scenes=req.scenes,
            storyboard_title=req.storyboard_title,
            category=req.category,
            guardrails=req.guardrails,
            work_dir=req.work_dir,
            quality=req.quality,
        )

        final_state = manim_generator_graph.invoke(
            initial_state.model_dump(),
            config={"configurable": {"thread_id": req.job_id}},
        )

        clip_path = final_state.get("clip_path")
        ok = bool(clip_path)

        return RenderResponse(
            ok=ok,
            clip_path=clip_path,
            attempts=final_state.get("attempts", 1),
            lint_retries=final_state.get("lint_retries", 0),
            snippets_used=final_state.get("snippets_used", []),
            snippet_id=final_state.get("snippet_id"),
            stage=final_state.get("stage") if not ok else None,
            last_traceback=final_state.get("traceback") if not ok else None,
        )

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=502,
            detail={"error": {"code": "model_error", "message": str(e), "details": {"provider": "anthropic"}}},
        )

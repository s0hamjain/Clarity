from fastapi import APIRouter, HTTPException
from app.graphs.manim_generator.nodes import generate_node
from app.graphs.manim_generator.state import RenderState
from app.schemas import CodegenRequest, CodegenResponse

router = APIRouter()


@router.post("/codegen", response_model=CodegenResponse)
async def debug_codegen(req: CodegenRequest):
    """POST /codegen - Debug endpoint running generate node alone."""
    try:
        state = RenderState(
            job_id="debug_job",
            scenes=req.scenes,
            storyboard_title=req.storyboard_title or "Debug Codegen",
            category="algorithm",
            guardrails=req.guardrails,
            work_dir="/tmp/debug",
            snippets=req.snippets,
            source=req.previous_source,
            traceback=req.traceback,
        )

        res = generate_node(state)
        source = res.get("source", "")
        return CodegenResponse(manim_source=source, scene_class="GeneratedScene")
    except Exception as e:
        raise HTTPException(
            status_code=502,
            detail={"error": {"code": "model_error", "message": str(e), "details": {"provider": "anthropic"}}},
        )

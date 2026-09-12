from fastapi import APIRouter, HTTPException
from app.graphs.intake.chain import run_intake
from app.schemas import VisionRequest, VisionResponse

router = APIRouter()


@router.post("/vision", response_model=VisionResponse)
async def vision_intake(req: VisionRequest):
    """POST /vision - Intake chain for verbatim problem transcription."""
    try:
        result = run_intake(
            image_b64=req.image_b64,
            media_type=req.media_type,
            guardrails=req.guardrails,
            user_prompt=req.user_prompt,
        )
        return result
    except Exception as e:
        raise HTTPException(
            status_code=502,
            detail={
                "error": {
                    "code": "model_error",
                    "message": str(e),
                    "details": {"provider": "gemini"},
                }
            },
        )

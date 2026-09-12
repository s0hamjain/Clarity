from fastapi import APIRouter
from app.schemas import ExplainRequest, ExplainResponse, Storyboard, Scene

router = APIRouter()


@router.post("/explain", response_model=ExplainResponse)
async def explain_problem(req: ExplainRequest):
    """POST /explain - Explainer agent endpoint (Stubbed for Sprint 1)."""
    return ExplainResponse(
        explanation=f"**Explanation for:** {req.problem_text[:100]}\n\nStep 1: Analyze the input problem statement carefully.\nStep 2: Apply standard algorithmic principles.",
        storyboard=Storyboard(
            title="Algorithm Visualization",
            scenes=[
                Scene(
                    index=0,
                    narration="Initialize pointer at index 0.",
                    visual="A row of array elements with a pointer arrow at index 0.",
                    duration_seconds=6.0,
                ),
                Scene(
                    index=1,
                    narration="Advance pointer through the array.",
                    visual="Pointer shifts smoothly to index 1.",
                    duration_seconds=8.0,
                ),
            ],
        ),
        revisions=0,
    )

from fastapi import APIRouter, HTTPException
from app.graphs.explainer.graph import explainer_graph
from app.graphs.explainer.state import ExplainerState
from app.schemas import ExplainRequest, ExplainResponse, Storyboard, Scene

router = APIRouter()


@router.post("/explain", response_model=ExplainResponse)
async def explain_problem(req: ExplainRequest):
    """POST /explain - Explainer agent endpoint with draft -> critique -> revise loop."""
    try:
        initial_state = ExplainerState(
            problem_text=req.problem_text,
            category=req.category,
            user_prompt=req.user_prompt or "",
            guardrails=req.guardrails,
        )

        final_state = explainer_graph.invoke(initial_state.model_dump())

        draft = final_state.get("draft")
        if not draft:
            raise HTTPException(
                status_code=502,
                detail={"error": {"code": "model_error", "message": "Explainer agent failed to produce a valid draft."}},
            )

        # Handle dict vs Pydantic model response from LangGraph invoke
        if isinstance(draft, dict):
            explanation = draft.get("explanation", "")
            sb_data = draft.get("storyboard", {})
            if isinstance(sb_data, dict):
                storyboard = Storyboard(
                    title=sb_data.get("title", ""),
                    scenes=[Scene(**s) for s in sb_data.get("scenes", [])],
                )
            else:
                storyboard = sb_data
        else:
            explanation = draft.explanation
            storyboard = draft.storyboard

        revisions = final_state.get("revisions", 0)

        return ExplainResponse(
            explanation=explanation,
            storyboard=storyboard,
            revisions=revisions,
        )

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=502,
            detail={"error": {"code": "model_error", "message": str(e), "details": {"provider": "gemini"}}},
        )

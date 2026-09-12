from fastapi import FastAPI
import httpx
from app.config import settings
from app.schemas import HealthResponse
from app.routers import vision, explain, scenes, snippets, debug
from app.vectorstore import get_mongo_client

app = FastAPI(title="Clarity Agent Service", version="0.1.0")

app.include_router(vision.router)
app.include_router(explain.router)
app.include_router(scenes.router)
app.include_router(snippets.router)
app.include_router(debug.router)


@app.get("/healthz", response_model=HealthResponse)
async def healthz():
    """GET /healthz - Health check for liveness and dependency status."""
    gemini_ok = bool(settings.GEMINI_API_KEY)
    anthropic_ok = bool(settings.ANTHROPIC_API_KEY)
    voyage_ok = bool(settings.VOYAGE_API_KEY)

    atlas_ok = False
    snippets_verified = 0
    try:
        client = get_mongo_client()
        res = client.admin.command("ping")
        atlas_ok = bool(res.get("ok"))
        db = client[settings.MONGODB_DB]
        snippets_verified = db["manim_snippets"].count_documents({"verified": True})
    except Exception:
        atlas_ok = False

    coordinator_ok = False
    try:
        async with httpx.AsyncClient() as client_http:
            resp = await client_http.get(f"{settings.COORDINATOR_URL}/healthz", timeout=2.0)
            coordinator_ok = (resp.status_code == 200)
    except Exception:
        coordinator_ok = False

    overall_ok = gemini_ok and anthropic_ok and voyage_ok and atlas_ok

    return HealthResponse(
        ok=overall_ok,
        gemini=gemini_ok,
        anthropic=anthropic_ok,
        voyage=voyage_ok,
        atlas=atlas_ok,
        coordinator=coordinator_ok,
        snippets_verified=snippets_verified,
        graphs=["intake", "explainer", "manim_generator"],
        models={
            "vision": settings.VISION_MODEL,
            "explain": settings.EXPLAIN_MODEL,
            "codegen": settings.CODEGEN_MODEL,
            "embed": settings.EMBED_MODEL,
        },
        version="0.1.0",
    )

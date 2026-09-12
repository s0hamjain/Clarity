import argparse
from typing import Dict
from fastapi import FastAPI, Body
import uvicorn

app = FastAPI(title="Fake Coordinator Server", version="0.1.0")

# Counter per work_dir/job to toggle render failure on first try
render_attempts: Dict[str, int] = {}


@app.get("/healthz")
async def healthz():
    return {"ok": True, "service": "fake_coordinator"}


@app.post("/internal/render")
async def fake_render(payload: dict = Body(...)):
    """Fake render tool endpoint simulating render crash on 1st attempt, success on 2nd attempt."""
    work_dir = payload.get("work_dir", "default")
    attempts = render_attempts.get(work_dir, 0) + 1
    render_attempts[work_dir] = attempts

    print(f"[Fake Coordinator] Render attempt #{attempts} for work_dir: '{work_dir}'")

    if attempts == 1:
        return {
            "ok": False,
            "stage": "render",
            "traceback": "NameError: name 'RIGHT_ARROW' is not defined. Did you mean 'RIGHT' or 'Arrow'?",
        }
    else:
        return {
            "ok": True,
            "clip_path": f"{work_dir}/fake_scene.mp4",
            "duration_seconds": 8.5,
        }


def main():
    parser = argparse.ArgumentParser(description="Fake Coordinator HTTP server for local testing")
    parser.add_argument("--port", type=int, default=8080, help="Port to run on (default 8080)")
    args = parser.parse_args()

    print(f"Starting Fake Coordinator server on http://localhost:{args.port}...")
    uvicorn.run(app, host="127.0.0.1", port=args.port)


if __name__ == "__main__":
    main()

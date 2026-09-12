# Ambient Visual Learning Tool — File Structure

## Root

```
HackCMU/
├── .gitignore
├── README.md
├── AGENTS.md                  # Start here. Project summary, doc map, rules, protocol, sprint index.
├── FILE_STRUCTURE.md          # This file.
├── docs/
│   ├── FRD.md                 # Technical specification — every shape, schema, route, rule. Source of truth.
│   ├── SETUP.md               # Environment setup for every service and tool; verification checklist.
│   └── WORK_SPLIT.md          # P1–P4 across five sprints; sync points; merge protocol; ownership.
├── samples/                   # P2 · verified Manim seed scenes = the RAG corpus seed
├── docker/                    # P2 · the manim-worker image
├── agent/                     # P1 · Python agent service
├── server/                    # P3 coordinator + P2 render package (one Go module)
└── desktop/                   # P4 · macOS menu bar app + installer scripts
```

Nothing under `samples/`, `docker/`, `agent/`, `server/`, or `desktop/` exists at the start. Each person creates their own directory in Sprint 1.

---

## samples/ (P2)

```
samples/
├── 001_mathtex_side_by_side.py     # Each file: one `class GeneratedScene(Scene)`, relative positioning only,
├── 002_axes_plot_moving_dot.py     #   and a docstring header the seed script parses:
├── 003_array_walk_pointer.py       #     title: / description: / category: / tags:
├── 004_transform_derivative.py     # Every file has been rendered in manim-worker AND watched.
├── ...                             # Target: 5 by Sprint 1, 20 by Sprint 2. See FRD §13.
└── README.md                       # The docstring format and the "watched, not just rendered" rule.
```

---

## docker/ (P2)

```
docker/
└── manim-worker/
    ├── Dockerfile                  # python:3.12-slim + TeX (latex-base, latex-extra, fonts-recommended, science)
    │                               #   + dvisvgm + ffmpeg + cairo/pango deps + pip install manim
    └── README.md                   # Build command, smoke test, S3 lifecycle rule note (14 d on renders/)
```

---

## agent/ (P1)

```
agent/
├── .env                            # Never committed. See docs/SETUP.md §12.1
├── .env.example
├── requirements.txt                # anthropic fastapi uvicorn pydantic pydantic-settings voyageai pymongo python-dotenv
│
├── app/
│   ├── __init__.py
│   ├── main.py                     # FastAPI app; mounts routers; /healthz pings Anthropic, Voyage, Atlas
│   ├── config.py                   # pydantic-settings: ANTHROPIC_API_KEY, VOYAGE_API_KEY, MONGODB_URI, MONGODB_DB, EMBED_MODEL
│   ├── schemas.py                  # Pydantic models = FRD §10 shapes, exactly. These are the contract.
│   │
│   ├── clients/
│   │   ├── claude.py               # One anthropic.Anthropic(); structured-output helper; stream helper for codegen
│   │   ├── embed.py                # voyageai.Client().embed(...) with input_type document|query
│   │   └── atlas.py                # pymongo client; collection handles; $vectorSearch helper
│   │
│   ├── routers/
│   │   ├── vision.py               # POST /vision      — verbatim transcription, effort low
│   │   ├── explain.py              # POST /explain     — explanation + 2–5 scene storyboard; guardrails variant
│   │   ├── snippets.py             # POST /snippets/search, POST /snippets/ingest
│   │   └── codegen.py              # POST /codegen     — with snippets; repair when previous_source+traceback present
│   │
│   └── prompts/
│       ├── vision.md
│       ├── explain_math.md
│       ├── explain_algorithm.md
│       ├── explain_guardrails.md   # Appended when guardrails=true: teach the method, withhold the final answer
│       ├── codegen.md              # Constraints (FRD §10.4) + "Reference — imitate these" + retrieved snippets
│       └── repair.md               # Minimal fix, no rewrite; last 40 lines of traceback
│
├── scripts/
│   ├── seed_snippets.py            # Parse samples/*.py docstrings → embed → upsert verified=true origin=seed
│   │                               #   --create-index creates snippets_vector (FRD §9.3). Idempotent.
│   └── promote_snippet.py          # Flip one generated snippet to verified=true after a human watched it
│
└── experiments/
    ├── cache_collision.py          # 6 captures of one problem → /vision → normalize → count distinct (Sprint 1)
    └── retrieval_ablation.py       # 10 scenes, codegen with vs without snippets → first-attempt render rate (Sprint 3)
```

---

## server/ (P3 coordinator · P2 render)

```
server/
├── .env                            # Never committed. See docs/SETUP.md §12.2
├── .env.example
├── go.mod
├── go.sum
│
├── cmd/
│   └── server/
│       └── main.go                 # Reads env, connects Atlas, creates TTL indexes, boot health checks, starts HTTP (P3)
│
└── internal/
    ├── api/                        # P3
    │   ├── handlers.go             # POST /api/jobs, GET /api/jobs/{id}, GET /healthz — FRD §11
    │   └── cors.go                 # Access-Control-Allow-Origin: * on every route incl. 404/5xx
    │
    ├── store/                      # P3 · MongoDB Atlas
    │   ├── mongo.go                # Client, database handle
    │   ├── jobs.go                 # Create/Get/Update — every Update sets updated_at
    │   ├── cache.go                # Get/Put by problem_hash
    │   └── indexes.go              # TTL indexes on jobs.updated_at (24 h) and cache.created_at (7 d), idempotent
    │
    ├── cache/                      # P3
    │   ├── key.go                  # PromptVersion const · Normalize() · Hash() — FRD §12
    │   └── key_test.go             # Multi-line/tab/case input; guardrails true vs false must differ
    │
    ├── agent/                      # P3
    │   └── client.go               # Typed HTTP client for FRD §10; strips data-URL prefix; per-endpoint timeouts
    │
    ├── jobs/                       # P3
    │   ├── job.go                  # Job struct = FRD §9.1; status enum
    │   ├── worker.go               # vision → hash → cache? → explain → write explanation → fan out → concat → upload → done
    │   └── scenes.go               # Bounded fan-out over scenes; defer recover() per goroutine; scenes_done increments
    │
    └── render/                     # P2 · the boundary with P3 is the three signatures in FRD §14.1
        ├── precheck.go             # python ast.parse; banned imports/calls; requires class GeneratedScene(Scene)
        ├── docker.go               # Render(): temp dir → docker run --network none … manim-worker → clip path | RenderError
        ├── repair.go               # RenderWithRepair(): ≤3 attempts; retrieve → codegen → Render per attempt
        ├── semaphore.go            # Buffered channel from RENDER_CONCURRENCY, held around docker run only
        ├── validate.go             # Clip size floor; ffprobe duration
        ├── concat.go               # Concat(): ffprobe param check → concat demuxer -c copy
        ├── s3.go                   # AWS SDK v2, S3_ENDPOINT override, path-style; upload renders/<hash>.mp4; public URL
        └── render_test.go          # Renders every samples/*.py through a real container
```

---

## desktop/ (P4)

```
desktop/
├── requirements.txt                # rumps pynput pywebview pillow requests pyinstaller
├── README.md                       # User-facing: install from DMG, "Open Anyway" on macOS 15, the two permissions + restarts
├── RELEASE_NOTES.md                # Written from the second-Mac install in Sprint 4
│
├── avlt/
│   ├── __init__.py
│   ├── __main__.py                 # `python -m avlt`; `--once` runs a single capture without the hotkey
│   ├── app.py                      # rumps.App: Capture · Recents · Guardrails (checkbox) · Server… · Clear Recents · Quit
│   ├── config.py                   # ~/Library/Application Support/AVLT/config.json — server_url, guardrails, hotkey
│   ├── hotkey.py                   # pynput GlobalHotKeys; 2 s debounce; dispatches capture to a worker thread
│   ├── capture.py                  # screencapture -i -x; Esc → None; Pillow thumbnail ≤1568 px; PNG data URL
│   ├── recents.py                  # recents.json + recents/<id>.png + <id>_thumb.png; add() before POST; update() from
│   │                               #   polls (job_id, problem_text, explanation, video_url); rolling 50; clear()
│   ├── client.py                   # POST /api/jobs (source="desktop", guardrails); ConnectionError → notification
│   ├── spotlight_window.py         # pywebview frameless+transparent+vibrancy 680×96, centered; js_api: submit/cancel/
│   │                               #   list_recents/pick_recent; expands for the recents list
│   ├── result_window.py            # pywebview frameless+on_top+vibrancy 440×680; loads ui/result/?job=&server=;
│   │                               #   js_api: update_recent; new job closes the previous box
│   └── ui/
│       ├── shared/
│       │   ├── marked.min.js       # Vendored. Nothing in ui/ loads from the network except video_url.
│       │   └── base.css            # Vibrancy-friendly translucent surfaces, type scale, focus ring
│       ├── spotlight/
│       │   ├── index.html
│       │   ├── spotlight.js        # Thumbnail · input · Enter/Esc · ↓ or "/" expands recents · filter · Enter/Tab on a recent
│       │   └── spotlight.css
│       └── result/
│           ├── index.html
│           ├── result.js           # Poll 1 s · status words · explanation on first non-null · scene progress ·
│           │                       #   <video autoplay muted controls> · done+null note · 404 · 180 s retry ·
│           │                       #   renders from local recent first when reopened
│           └── result.css
│
├── stub/
│   └── stub_server.py              # Fake coordinator on :8080; walks a job through every status on a timer
│
├── assets/
│   ├── icon.icns
│   ├── menubar_idle.png
│   ├── menubar_working.png
│   └── dmg_background.png
│
├── scripts/
│   ├── build_app.sh                # pyinstaller --windowed --hidden-import … --add-data ui → dist/AVLT.app;
│   │                               #   patch Info.plist LSUIElement=true; codesign -s "AVLT Dev" --deep --force
│   ├── build_dmg.sh                # create-dmg → dist/AVLT.dmg
│   ├── build_pkg.sh                # pkgbuild + productbuild → dist/AVLT.pkg; postinstall writes LaunchAgent
│   └── postinstall.sh              # ~/Library/LaunchAgents/com.avlt.app.plist
│
└── dist/                           # Build output. Gitignored.
```

---

## Rules

| Rule | Detail |
|---|---|
| One path, one directory | `agent/` is P1, `docker/` + `samples/` + `server/internal/render/` are P2, the rest of `server/` is P3, `desktop/` is P4. Touching another path's directory requires telling them first. |
| `server/internal/render/` boundary | Three function signatures in FRD §14.1. P2 implements, P3 calls. Signature changes are agreed before either side edits. |
| `samples/` docstring format | `title:` / `description:` / `category:` / `tags:` — the seed script depends on it. Documented in `samples/README.md`. |
| Contract changes | `docs/FRD.md` only, own commit, straight to `main`, announced. |
| Generated output | `dist/`, `build/`, `*.spec`, `media/`, `renders/`, `.venv/`, `.env` are gitignored. Videos live in S3, never in the repo. |
| Branches | `pN/sprint-M-description`. Merged to `main` only at sync points, in order P3 → P1 → P2 → P4. See WORK_SPLIT.md → Merge Protocol. |

---

## Ownership Summary

| Path | Person | Primary Files |
|---|---|---|
| A — Agent | P1 | `agent/**` |
| B — Render | P2 | `docker/**`, `samples/**`, `server/internal/render/**` |
| C — Coordinator | P3 | `server/cmd/**`, `server/internal/{api,store,cache,agent,jobs}/**`, `server/go.mod` |
| D — Desktop | P4 | `desktop/**` |
| Shared | All | `README.md`, `AGENTS.md`, `FILE_STRUCTURE.md`, `docs/**` |

# Clarity — File Structure

Where every file lives and who owns it. Nothing under `agent/`, `server/`, `desktop/`, `docker/`, or `samples/` exists yet — each person creates their own directory in Sprint 1 following this layout, so that paths cited in the other docs stay true. Comments after `#` say what each file is for.

## Root

```
Clarity/
├── .gitignore
├── README.md
├── AGENTS.md                  # Start here. Project summary, doc map, rules, protocol, sprint index.
├── FILE_STRUCTURE.md          # This file.
├── docs/
│   ├── FRD.md                 # Technical specification — every shape, schema, route, rule. Source of truth.
│   ├── API.md                 # REST API reference — every endpoint on both services, errors, lifecycle, limits.
│   ├── SETUP.md               # Environment setup for every service and tool; verification checklist.
│   ├── WORK_SPLIT.md          # Team view: roles, contracts, sprint calendar, sync checklists, merge protocol.
│   ├── P1_AI.md               # P1's complete task list — agent service.
│   ├── P2_RENDER.md           # P2's complete task list — Docker, samples, render pipeline.
│   ├── P3_BACKEND.md          # P3's complete task list — coordinator.
│   └── P4_DESKTOP.md          # P4's complete task list — desktop app + installer.
├── samples/                   # P2 · verified Manim seed scenes = the RAG corpus seed
├── docker/                    # P2 · the manim-worker image
├── agent/                     # P1 · Python agent service
├── server/                    # P3 coordinator + P2 render package (one Go module)
├── release/                   # P3 · .pkg installer, LaunchAgent, release notes, publish script
└── desktop/                   # P4 · macOS menu bar app, .app and .dmg build scripts
```

Nothing under `samples/`, `docker/`, `agent/`, `server/`, `release/`, or `desktop/` exists at the start. Each person creates their own directory in Sprint 1.

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
├── requirements.txt                # langgraph langchain-core langchain-google-genai langchain-anthropic langchain-mongodb
│                                   #   langchain-voyageai fastapi uvicorn pydantic pydantic-settings pymongo python-dotenv
│
├── app/
│   ├── __init__.py
│   ├── main.py                     # FastAPI; one route per graph; /healthz pings Gemini, Anthropic, Voyage, Atlas, coordinator
│   ├── config.py                   # pydantic-settings: keys, MONGODB_URI/DB, COORDINATOR_URL, VISION/EXPLAIN/CODEGEN/EMBED_MODEL, LANGSMITH_*
│   ├── schemas.py                  # API request/response models = API.md §3, exactly. These are the contract.
│   ├── llm.py                      # gemini_flash(), gemini_flash_deterministic(), sonnet() — LangChain chat models
│   ├── prompts.py                  # load_prompt(name) → ChatPromptTemplate from prompts/*.md (cached)
│   ├── vectorstore.py              # the ONE MongoDBAtlasVectorSearch + VoyageAIEmbeddings; retriever(category)
│   │
│   ├── graphs/
│   │   ├── intake/
│   │   │   └── chain.py            # vision_prompt | gemini_flash_deterministic.with_structured_output(VisionResponse)
│   │   ├── explainer/
│   │   │   ├── state.py            # ExplainerState(BaseModel): draft, critique, revisions
│   │   │   ├── nodes.py            # draft · critique (python hard-rules + Gemini rubric) · revise
│   │   │   └── graph.py            # draft → critique →(fail, revisions<1)→ revise → critique ; else END
│   │   └── manim_generator/
│   │       ├── state.py            # SceneState(BaseModel): snippets, source, traceback, attempts, lint_retries, clip_path…
│   │       ├── nodes.py            # retrieve · generate (Sonnet) · lint · render · ingest
│   │       ├── lint.py             # ast.parse, banned imports/calls, class GeneratedScene, literal-coordinate heuristics
│   │       ├── tools.py            # render_tool(): POST <COORDINATOR_URL>/internal/render
│   │       └── graph.py            # retrieve → generate → lint ⇄ generate (≤2) → render ⇄ retrieve (≤3) → ingest → END
│   │
│   ├── routers/
│   │   ├── vision.py               # POST /vision          → intake chain
│   │   ├── explain.py              # POST /explain         → explainer graph
│   │   ├── scenes.py               # POST /scenes/render   → manim_generator graph
│   │   ├── snippets.py             # POST /snippets/ingest · GET /snippets · GET/PATCH/DELETE /snippets/{id} · debug POST /snippets/search
│   │   └── debug.py                # POST /codegen — runs the generate node alone
│   │
│   └── prompts/
│       ├── vision.md
│       ├── explain_math.md
│       ├── explain_algorithm.md
│       ├── explain_guardrails.md   # Appended when guardrails=true
│       ├── critique.md             # The rubric the Explainer grades itself against
│       ├── codegen.md              # Constraints + "Reference — imitate these" + scene
│       └── repair.md               # Minimal fix, no rewrite; last 40 traceback lines
│
├── dev/
│   └── fake_coordinator.py         # POST /internal/render: fail once with a traceback, then succeed — exercises the repair edge
│
├── scripts/
│   ├── seed_snippets.py            # samples/*.py docstrings → Documents → vectorstore.add_documents (ids = slug) ; --create-index
│   └── promote_snippet.py          # PATCH /snippets/{id} {"verified": true}
│
└── experiments/
    ├── cache_collision.py          # 6 captures → /vision → normalize → count distinct (Sprint 1)
    └── retrieval_ablation.py       # 10 scenes, generate with vs without snippets → first-attempt render rate (Sprint 3)
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
    │   ├── handlers.go             # POST /api/jobs, GET /api/jobs/{id}, DELETE, GET /api/cache/{hash}, GET /healthz — FRD §11
    │   ├── internal_render.go      # POST /internal/render — localhost only; semaphore → precheck → render.Render() (agent's tool)
    │   ├── errors.go               # The one error envelope + the code constants — API.md §4
    │   ├── health.go               # Cached dependency checks behind /healthz; refreshed at boot and every 60 s
    │   ├── cors.go                 # Access-Control-Allow-Origin: * on every route incl. 404/5xx; X-Request-Id echo/generate
    │   ├── s3health.go            # Unauthenticated HEAD on the render bucket: exists AND public-read (FRD §14.6)
    │   ├── stores_test.go         # In-memory job/cache doubles for the handler tests
    │   ├── handlers_test.go        # Status codes, the error envelope, CORS, and the exact GET /api/jobs/{id} field set
    │   └── internal_render_test.go # Loopback-only; precheck gate; work_dir containment; semaphore 429
    │
    ├── config/                     # P3 · env → Config (FRD §22), read once at boot
    │   ├── config.go               # Defaults and validation; MONGODB_URI is required
    │   ├── dotenv.go               # Reads server/.env, as SETUP §9 assumes; the real environment wins
    │   └── dotenv_test.go          # Parsing rules; environment beats file; a missing file is fine
    │
    ├── store/                      # P3 · MongoDB Atlas; implements the interfaces in jobs/store.go
    │   ├── mongo.go                # Client, database handle, Ping for /healthz
    │   ├── jobs.go                 # Create/Get/Update — every Update sets updated_at
    │   ├── cache.go                # Get/Put by problem_hash
    │   └── indexes.go              # TTL indexes on jobs.updated_at (24 h) and cache.created_at (7 d), idempotent
    │
    ├── cache/                      # P3
    │   ├── key.go                  # PromptVersion const · Normalize() · Hash() — FRD §12
    │   └── key_test.go             # Multi-line/tab/case input; guardrails true vs false must differ
    │
    ├── agent/                      # P3
    │   ├── client.go               # Typed HTTP client for API.md §3; strips data-URL prefix; per-endpoint timeouts
    │   └── client_test.go          # Request/response shapes; both error-envelope shapes; cancellation
    │
    ├── jobs/                       # P3
    │   ├── job.go                  # Job struct = FRD §9.1; status enum; RFC 3339 wire shape
    │   ├── store.go                # The Store / CacheStore interfaces the worker consumes; store/ implements them
    │   ├── worker.go               # vision → hash → cache? → explain → write explanation → fan out → concat → upload → done
    │   ├── scenes.go               # Per-scene work_dir + goroutine calling agent POST /scenes/render; defer recover(); scenes_done
    │   ├── scenefunc.go            # AgentSceneFunc: one POST /scenes/render per scene; validates the clip_path it gets back
    │   ├── worker_test.go          # The status walk; rule 8 ordering; rule 10 (no cache on a failure path); cancel
    │   ├── pipeline_test.go        # The real path against a stand-in agent: unknown, cache hit, agent down, dropped scenes
    │   └── scenefunc_test.go       # Request fidelity incl. the job-level quality flag; every way a clip_path is rejected
    │
    └── render/                     # P2 · the boundary with P3 is Render / Concat / Semaphore in FRD §14.1
        ├── precheck.go             # python ast.parse; banned imports/calls; requires class GeneratedScene(Scene)
        ├── docker.go               # Render(src, workDir, quality): docker run --network none … manim-worker → clip path | RenderError
        ├── semaphore.go            # Buffered channel from RENDER_CONCURRENCY; P3 holds it around Render() in /internal/render
        ├── validate.go             # ffprobe: duration, resolution/fps for the quality flag, size floor
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
├── clarity/
│   ├── __init__.py
│   ├── __main__.py                 # `python -m clarity`; `--once` runs a single capture without the hotkey
│   ├── app.py                      # rumps.App: Capture · Recents · Guardrails (checkbox) · Server… · Clear Recents · Quit
│   ├── config.py                   # ~/Library/Application Support/Clarity/config.json — server_url, guardrails, hotkey
│   ├── hotkey.py                   # pynput GlobalHotKeys; 2 s debounce; dispatches capture to a worker thread
│   ├── capture.py                  # screencapture -i -x; Esc → None; Pillow thumbnail ≤1568 px; PNG data URL
│   ├── recents.py                  # recents.json + recents/<id>.png + <id>_thumb.png; add() before POST; update() from
│   │                               #   polls (job_id, problem_text, explanation, video_url); rolling 50; clear()
│   ├── client.py                   # POST /api/jobs (source="desktop", guardrails); ConnectionError → notification
│   ├── window_host.py              # One process per window, because rumps and pywebview both need the main thread.
│   │                               #   Spawn, one-JSON-object-per-line protocol, screen geometry, clean shutdown.
│   ├── session.py                  # capture → spotlight → POST → result box, shared by app.py and `--once`
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
├── assets/
│   ├── icon.icns
│   ├── menubar_idle.png
│   ├── menubar_working.png
│   └── dmg_background.png
│
├── scripts/
│   ├── build_app.sh                # pyinstaller --windowed --hidden-import … --add-data ui → dist/Clarity.app;
│   │                               #   patch Info.plist LSUIElement=true; codesign -s "Clarity Dev" --deep --force
│   └── build_dmg.sh                # create-dmg → dist/Clarity.dmg
│
└── dist/                           # Build output. Gitignored.
```

---

## release/ (P3)

```
release/
├── build_pkg.sh                    # pkgbuild --component desktop/dist/Clarity.app … + productbuild → dist/Clarity.pkg
├── publish.sh                      # gh release create v0.1.0 dist/Clarity.dmg dist/Clarity.pkg --notes-file RELEASE_NOTES.md
├── RELEASE_NOTES.md                # install steps (Open Anyway, two permissions + restarts), known issues
└── scripts/
    └── postinstall                 # writes + loads ~/Library/LaunchAgents/com.clarity.app.plist (start at login)
```

P4 builds the `.app` and `.dmg`; P3 packages and publishes them. The hand-off is the built files in `desktop/dist/`.

---

## Rules

| Rule | Detail |
|---|---|
| One path, one directory | `agent/` is P1, `docker/` + `samples/` + `server/internal/render/` are P2, the rest of `server/` + `release/` is P3, `desktop/` is P4. Touching another path's directory requires telling them first. |
| `server/internal/render/` boundary | `Render`, `Concat`, `Semaphore` in FRD §14.1. P2 implements, P3 calls from `/internal/render` and the job tail. Signature changes are agreed before either side edits. |
| The agent's render tool | `POST /internal/render` (API.md §2.7). P3 implements, P1's Manim Generator calls. |
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
| C — Coordinator + Release | P3 | `server/cmd/**`, `server/internal/{api,store,cache,agent,jobs}/**`, `server/go.mod`, `release/**` |
| D — Desktop | P4 | `desktop/**` |
| Shared | All | `README.md`, `AGENTS.md`, `FILE_STRUCTURE.md`, `docs/**` |

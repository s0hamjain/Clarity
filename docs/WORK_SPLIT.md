# Clarity — Work Split

**Version:** 3.0
**Engineers:** 4 (P1, P2, P3, P4)
**Structure:** 5 sprints. Everyone works on their own branch until the end of a sprint, then everyone's work is merged together at the sync point. Nobody is ever blocked — every path fakes its dependencies until the real thing lands.

### How to read this

- **Find your role** in the Path Overview below — P1, P2, P3, or P4. That's your one-sentence job and your directory.
- **Jump to the current sprint** and read only your section. Each step names the files to create and cites the exact spec section (e.g. "FRD §10.3" = `docs/FRD.md`, section 10.3) so you never have to guess a field name.
- **Check "What each path fakes."** If something you depend on isn't built yet, that table says what to stub so you can keep going.
- **Before the sync point**, run through your items in that sprint's checklist. Then follow the Merge Protocol at the bottom.

A *sprint* is a time-boxed chunk of work. A *sync point* is the meeting at its end where everyone's branch is merged into `main` and we check the whole thing still works together.

All shapes referenced below are in `docs/FRD.md`. Section numbers (e.g. "FRD §10.3") point there.

---

## Path Overview

Four people, four roles, four directories. Each role is one sentence. Nobody edits another role's directory.

| Person | Role — one sentence | Owns | Does **not** touch |
|---|---|---|---|
| **P1 — AI** | Makes the model produce correct JSON: transcription, explanation + storyboard, snippet retrieval, Manim code, repair. | `agent/` — FastAPI service, prompts, Claude/Voyage/Atlas clients, seed + promote scripts, experiments | Go, Docker, desktop UI |
| **P2 — Render** | Turns a string of Manim source into an MP4 on S3, safely: image, pre-check, container, repair loop, concat, upload; and writes the verified seed scenes. | `docker/`, `samples/`, `server/internal/render/` | HTTP handlers, job store, prompts, desktop UI |
| **P3 — Backend** | Runs the job: public API, Atlas job store + cache, calls P1's endpoints and P2's functions in order, fans scenes out, reports status. | `server/` except `internal/render/` | Prompts, Dockerfile, render internals, desktop UI |
| **P4 — Desktop** | Everything the user sees and installs: hotkey, region capture, spotlight box, recents, result box, build, DMG/PKG, release. | `desktop/` | Anything server-side |

**Where roles meet** (the only shared surfaces):
- P3 ↔ P1: the HTTP shapes in FRD §10. P1 implements, P3 calls.
- P3 ↔ P2: the three Go signatures in FRD §14.1. P2 implements, P3 calls. Same Go module, different packages.
- P4 ↔ P3: the HTTP shapes in FRD §11. P3 implements, P4 calls.
- P1 ↔ P2: the `samples/*.py` docstring format in FRD §13. P2 writes files, P1's script reads them.

A change to any of those four surfaces is an FRD change: own commit, straight to `main`, announced.

### What each path fakes

| Path | Fakes | Until |
|---|---|---|
| P1 | Nothing — it's a leaf. Testable with `curl` and a PNG. | — |
| P2 | `/codegen` and `/snippets/search`, via callback functions that return a `samples/*.py` file and an empty snippet list. | Sprint 3 |
| P3 | Everything: hardcoded `/vision`, `/explain`, `/codegen` responses; a `Render` that copies a sample MP4. | Sprint 2 (agent), Sprint 3 (render) |
| P4 | The coordinator, via `desktop/stub/stub_server.py` — walks a job through every status on a timer. | Sprint 2 |

---

## Sprint 1 — Foundation (Hours 0–3)

**Goal:** every path runs end-to-end *within itself* on fakes, every external service is reachable, and the two riskiest assumptions (LaTeX in the container, transcription collisions) have been tested.

---

### P1 — Agent Skeleton, Atlas + Voyage Wiring, Collision Experiment

**Deliverables:** FastAPI service with `/healthz` reporting real Anthropic, Voyage, and Atlas connectivity; one real `/vision` call; the collision experiment result.

**Step 1 — Service skeleton** (ref: FRD §10, §22)
Create `agent/app/main.py` (FastAPI app, routers, CORS off — only the coordinator calls it), `agent/app/config.py` (pydantic-settings loading `agent/.env`), `agent/app/schemas.py` (pydantic models for every request/response in FRD §10 — these *are* the contract, get them exact). Stub every endpoint to return a valid hardcoded response.

**Step 2 — Clients** (ref: FRD §10.7, SETUP §3–5)
`agent/app/clients/claude.py` — one `anthropic.Anthropic()` client, a helper that calls `messages.create` with structured outputs and returns the parsed object. `agent/app/clients/embed.py` — `voyageai.Client().embed(texts, model=EMBED_MODEL, input_type=...)`. `agent/app/clients/atlas.py` — `pymongo.MongoClient(MONGODB_URI)[MONGODB_DB]`. `/healthz` pings all three.

**Step 3 — Real `/vision`** (ref: FRD §10.1)
Prompt in `agent/app/prompts/vision.md`. Verbatim transcription, nothing else. `effort: "low"`. Structured output schema = `VisionResponse`. Test with a real screenshot via `curl`.

**Step 4 — Collision experiment** (ref: FRD §24)
`agent/experiments/cache_collision.py`: take six captures of one problem (two zoom levels × three crop widths), run each through `/vision`, apply `normalize()` (reimplemented here, identical to FRD §12), count distinct strings. Record the number in the PR description. This decides whether Haiku-with-`temperature=0` is needed.

---

### P2 — `manim-worker` Image, Smoke Test, First Seed Scenes

**Deliverables:** the Docker image builds and renders `MathTex` inside a container; 5 seed scenes render and look right.

**Step 1 — Dockerfile** (ref: FRD §14.3, SETUP §6)
`docker/manim-worker/Dockerfile`: `python:3.12-slim`, `apt-get install texlive-latex-base texlive-latex-extra texlive-fonts-recommended texlive-science dvisvgm ffmpeg libcairo2-dev libpango1.0-dev pkg-config`, `pip install manim`. Build it. Run the SETUP §6.2 smoke test. **Nothing else in this path starts until the MP4 exists.**

**Step 2 — Seed scenes** (ref: FRD §13)
`samples/001_mathtex_side_by_side.py` … `samples/005_*.py`. One `class GeneratedScene(Scene)` per file. Relative positioning only. Each file starts with a docstring in this exact shape — the seed script parses it:

```python
"""
title: MathTex with relative positioning
description: Two MathTex objects placed side by side with next_to, then one transforms into a derivative.
category: math
tags: MathTex, next_to, Transform
"""
```

Render each in the container. **Watch each one.** A scene that renders but overlaps or falls off-frame is not a seed.

**Step 3 — Precheck** (ref: FRD §14.2)
`server/internal/render/precheck.go`: shells out to `python -c "import ast,sys; ast.parse(sys.stdin.read())"`, then scans the AST dump (or the source) for banned imports and calls, and requires `class GeneratedScene(Scene)`. Table-driven test with one good sample and six bad inputs.

---

### P3 — Coordinator Skeleton, Atlas Job Store, Fake Pipeline

**Deliverables:** `POST /api/jobs` returns a job ID; `GET /api/jobs/{id}` walks through every status on a timer with a fake explanation and a fake video URL; jobs persist in Atlas with TTL indexes.

**Step 1 — HTTP skeleton** (ref: FRD §11)
`server/cmd/server/main.go`, `server/internal/api/handlers.go`, `server/internal/api/cors.go`. Routes exactly as FRD §11. Read config from `server/.env` via `os.Getenv` with the defaults in FRD §22.

**Step 2 — Atlas job store** (ref: FRD §9.1, §9.2)
`server/internal/store/mongo.go` (client), `server/internal/store/jobs.go` (`Create`, `Get`, `Update` — **every `Update` sets `updated_at`**), `server/internal/store/cache.go` (`Get`, `Put`), `server/internal/store/indexes.go` (creates both TTL indexes at boot, idempotent).

**Step 3 — Fake worker** (ref: FRD §11.2)
`server/internal/jobs/worker.go`: a goroutine that sleeps 1 s per status, writes a hardcoded explanation at `generating`, increments `scenes_done` at `rendering`, sets a hardcoded `video_url` at `done`. `defer recover()` marks the job `failed`. This is the stub P4 can point at instead of their own.

**Step 4 — Cache key** (ref: FRD §12)
`server/internal/cache/key.go`: `PromptVersion`, `Normalize`, `Hash`. `key_test.go` with a multi-line, tab-indented, mixed-case input and a guardrails true/false pair that must differ.

---

### P4 — Menu Bar App, Hotkey, Capture, Permissions

**Deliverables:** `python -m clarity` puts an icon in the menu bar; `⌘⇧E` opens a region select; the PNG lands on disk downscaled; both permissions granted and documented.

**Step 1 — Permissions first** (ref: SETUP §10.2)
From Terminal.app, run `screencapture -i` and a `pynput` listener; grant both; restart Terminal both times. Write down exactly what happened in `desktop/README.md` — this becomes the user-facing install note.

**Step 2 — App skeleton** (ref: FRD §15.1)
`desktop/clarity/__main__.py`, `desktop/clarity/app.py` (a `rumps.App` subclass: menu items **Capture**, **Recents**, **Guardrails** (checkbox), **Server…**, **Clear Recents**, **Quit**), `desktop/clarity/config.py` (reads/writes `~/Library/Application Support/Clarity/config.json`). `--once` flag runs one capture and exits.

**Step 2b — Prove the window trick** (ref: FRD §24)
Ten-line spike: `webview.create_window("x", html="<body style='background:transparent'>hi</body>", frameless=True, transparent=True, vibrancy=True, on_top=True)`. If it's translucent and blurred over your desktop, the spotlight design works as specified. If not, note it — Sprint 2 uses the solid fallback.

**Step 3 — Hotkey + capture** (ref: FRD §15.1)
`desktop/clarity/hotkey.py` (`pynput.keyboard.GlobalHotKeys`, 2 s debounce, runs the capture on a worker thread so the listener isn't blocked), `desktop/clarity/capture.py` (`screencapture -i -x`, Esc → `None`, `Pillow` thumbnail to 1568 px, returns a PNG data URL).

**Step 4 — Stub coordinator** (ref: FRD §11.2)
`desktop/stub/stub_server.py`: 40 lines, `http.server`, walks a job through every status on a timer with a sample explanation and a sample MP4 URL. This is what the result box is built against in Sprint 2.

---

### Sprint 1 Sync Point

**All four meet to verify:**
1. `docker run manim-worker` renders the `MathTex` smoke test to MP4.
2. Five `samples/*.py` render and were watched.
3. `curl localhost:8000/healthz` → `anthropic`, `voyage`, `atlas` all `true`.
4. One real `/vision` call returns verbatim text for a real screenshot.
5. Collision experiment number is known: **N distinct out of 6.**
6. `curl -X POST localhost:8080/api/jobs` → job ID; polling walks through every status; the job is visible in Atlas.
7. `python -m clarity --once` → crosshair → PNG on disk under 8 MB.
7b. The transparent/vibrancy spike window rendered (or the fallback decision is recorded).
8. Merge all branches to `main` per the Merge Protocol. Tag `sprint-1`.

---

## Sprint 2 — Vertical Slice (Hours 3–7)

**Goal:** a real capture produces a real explanation in the real result box. Rendering is still faked. The snippet corpus is seeded and searchable.

---

### P1 — Real `/explain`, Seed + Search, Determinism Decision

**Step 1 — `/explain`** (ref: FRD §10.2)
Prompts: `explain_math.md`, `explain_algorithm.md`, plus a guardrails addendum appended when `guardrails: true`. Structured output = `ExplainResponse` with 2–5 scenes. Every scene's `visual` must describe motion or a plot, not algebra — put that in the prompt as a rule and as an example pair (bad scene / good scene).

**Step 2 — Seed script + vector index** (ref: FRD §9.3, §13, SETUP §5.5)
`agent/scripts/seed_snippets.py`: parses each `samples/*.py` docstring, embeds `title + description + tags` with `input_type="document"`, upserts into `manim_snippets` with `verified: true, origin: "seed"`. `--create-index` creates `snippets_vector` via `create_search_index`. Idempotent — safe to re-run.

**Step 3 — `/snippets/search`** (ref: FRD §10.3)
Embed the query with `input_type="query"`; `$vectorSearch` with `numCandidates: 50, limit: k`, filter `verified: true` and `category ∈ {cat, "general"}`; project `score`. Test: a query about plotting a derivative should return a plotting seed above a pointer-walking seed.

**Step 4 — `/snippets/ingest`** (ref: FRD §10.5)
Embed and insert. `verified` defaults `false` unless the request says otherwise. `agent/scripts/promote_snippet.py <id>` flips it.

**Step 4b — Corpus management endpoints** (ref: API.md §3.6–3.9)
`GET /snippets`, `GET /snippets/{id}`, `PATCH /snippets/{id}`, `DELETE /snippets/{id}`. `promote_snippet.py` becomes a one-line `PATCH {\"verified\": true}`. `PATCH` on title/description/tags re-embeds.

**Step 5 — Decide determinism** (ref: FRD §10.1, §24)
Using Sprint 1's number: if ≥ 4 of 6 collided, stay on Opus 5. If not, switch `/vision` to `claude-haiku-4-5` with `temperature=0` and re-run the experiment. Record the decision in the PR.

---

### P2 — `Render()` Against Docker, Seed Corpus to 20

**Step 1 — `Render()`** (ref: FRD §14.1, §14.3)
`server/internal/render/docker.go`: per-scene temp dir, write `scene.py`, `docker run --rm --network none --memory 1g --cpus 1 -v tmp:/work manim-worker manim <MANIM_QUALITY> ...`, `exec.CommandContext` with a 120 s timeout that kills the container (`docker kill` on ctx done, not just the CLI process), return the clip path or a `RenderError{Stage, Traceback}`. Precheck runs first. Clip validation: file size > 20 KB; `ffprobe` duration > 1 s.

**Step 2 — Semaphore** (ref: FRD §14.1)
`server/internal/render/semaphore.go`: buffered channel sized from `RENDER_CONCURRENCY`, acquired around `docker run` only — not around codegen.

**Step 3 — `render_test.go`**
Renders every `samples/*.py` through the real container. This doubles as the corpus regression test.

**Step 4 — Seed corpus to 20** (ref: FRD §13)
Cover: `Axes` + `plot` with a moving dot; `Transform` between two `MathTex`; `VGroup.arrange` of boxes with a highlighted pointer (array walk); `Table`; `Code` block with a line highlight; a shape morph; a `NumberLine`; a `Graph` with BFS-style highlighting; a `Brace` with label; a `ValueTracker`-driven quantity. Docstring on every file. Watched, every one.

---

### P3 — Real Agent Calls, Real Cache, Fan-out Skeleton

**Step 1 — Agent client** (ref: FRD §10)
`server/internal/agent/client.go`: typed functions for every endpoint, 30 s timeout on `/vision` and `/explain`, 90 s on `/codegen`, base64 prefix stripping before `/vision`.

**Step 2 — Real worker, through `/explain`** (ref: FRD §11.2, §12, §23 rules 8–11)
`worker.go`: `transcribing` → `/vision` → hash → cache lookup → on hit: `done`, `cached: true`, both fields from `cache`, return. On miss: `explaining` → `/explain` → **write `explanation` to `jobs` immediately** → `generating`. Everything after `generating` still fake for now.

**Step 3 — Fan-out skeleton** (ref: FRD §14.1)
`server/internal/jobs/scenes.go`: `errgroup`-style fan-out over `storyboard.scenes` bounded by P2's semaphore, each goroutine with `defer recover()`, incrementing `scenes_done` via the store. Calls a `SceneFunc` that is a fake this sprint.

---

### P4 — Spotlight Box, Result Box, Real Submit

**Step 1 — Spotlight box** (ref: FRD §5, §15.1)
`desktop/clarity/spotlight_window.py` + `desktop/clarity/ui/spotlight/{index.html,spotlight.js,spotlight.css}`. `webview.create_window(..., frameless=True, transparent=True, vibrancy=True, on_top=True, width=680, height=96)` centered on the display the cursor is on. **Animates in** (CSS: opacity 0→1, scale 0.96→1, 180 ms ease-out) and out on submit (120 ms). Thumbnail of the capture on the left (data URL passed in via `js_api`), one text input on the right, placeholder *"Add context… (why is my binary search not working? visualize where it's messing up)"*. **Enter** → `js_api.submit(text)`; **Esc** → `js_api.cancel()`. Input focused on open. **First thing to verify:** transparency + vibrancy actually render on your macOS version; if not, fall back to a solid dark box with 92% opacity and move on (FRD §24).

**Step 2 — Result box** (ref: FRD §5, §15.1, §11.2)
`desktop/clarity/result_window.py` + `desktop/clarity/ui/result/{index.html,result.js,result.css}` + `ui/shared/marked.min.js` (vendored). `webview.create_window(url=…/index.html?job=<id>&server=<url>, width=440, height=680, frameless=True, transparent=True, on_top=True, vibrancy=True, easy_drag=True)`. **Whole box drags** (`easy_drag`), video and text excluded. **X** top-right and **Esc** → `window.close()`. Each job opens a new box offset 24 px; old ones stay until closed. `result.js`: poll every 1 s; status line in plain words; render `explanation` the first time it's non-null; scene progress; `<video controls autoplay muted>` on `video_url`; `done` + null video → quiet note; 404 → "expired"; 180 s → retry button.

**Step 3 — Submit + notification** (ref: FRD §15.1)
`desktop/clarity/client.py`: `POST /api/jobs` with `source: "desktop"` and the guardrails setting; on `ConnectionError` → `rumps.notification("Can't reach the server", ...)` and the spotlight box stays open. Notification when the explanation first appears.

**Step 4 — Point at the real coordinator**
Switch from the stub to `localhost:8080`. A real capture should now produce a real explanation in the result box.

---

### Sprint 2 Sync Point

1. `⌘⇧E` → drag region → spotlight box animates in → type context → Enter → **real explanation in the result box in under 10 s.** Drag the box off the problem. Click X. This is the product.
1b. The spotlight box is actually translucent and blurred over whatever is behind it (or the solid fallback is in place and the FRD §24 question is answered).
2. `snippets_verified ≥ 20` in `/healthz`; `/snippets/search` returns the right seed for a plotting query and for an array-walk query.
3. `Render()` renders every seed through a real container; `render_test.go` passes.
4. Cache hit path: submit the same capture twice; second poll is `cached: true` with explanation and (fake) video.
5. Determinism decision recorded.
6. Merge to `main`. Tag `sprint-2`.

---

## Sprint 3 — Real Rendering with Retrieval (Hours 7–11)

**Goal:** one capture goes all the way to a real video, rendered per scene in containers with retrieved snippets in every codegen prompt, concatenated, uploaded, and playing in the result box.

---

### P1 — `/codegen` with Snippets, Repair Path

**Step 1 — `/codegen`** (ref: FRD §10.4)
Prompt `codegen.md`: constraints from FRD §10.4; the retrieved snippets pasted verbatim under "Reference — imitate these"; the scene's narration and visual; the output schema. Use `messages.stream()` and `get_final_message()`. Assert `scene_class == "GeneratedScene"` and that the source contains `class GeneratedScene(Scene)`; otherwise raise so the coordinator counts a failed attempt.

**Step 2 — Repair** (ref: FRD §10.4)
When `previous_source` and `traceback` are present, prompt `repair.md`: fix the minimal thing, do not rewrite. Include the last 40 lines of the traceback only.

**Step 3 — Measure retrieval** (ref: FRD §24)
`agent/experiments/retrieval_ablation.py`: 10 storyboard scenes, codegen with and without snippets, render each through P2's container, count first-attempt successes. Record both numbers.

---

### P2 — `RenderWithRepair`, `Concat`, S3 Upload

**Step 1 — `RenderWithRepair`** (ref: FRD §14.1)
`repair.go`: up to 3 attempts. Each: `retrieve(scene, lastTraceback)` → `codegen(scene, snippets, prevSource, traceback)` → `Render`. On the third failure return the error; the caller drops the scene.

**Step 2 — `Concat`** (ref: FRD §14.4)
`concat.go`: write `concat_list.txt`, `ffmpeg -f concat -safe 0 -i list -c copy out.mp4`. Refuse to run if `ffprobe` reports differing resolution/fps across inputs — fail loudly rather than emit a broken file.

**Step 3 — S3 upload** (ref: FRD §14.6)
`s3.go`: AWS SDK v2 with `S3_ENDPOINT` override and path-style addressing (MinIO needs it). Upload to `renders/<hash>.mp4`. Return the public URL.

**Step 4 — Bucket lifecycle**
Document (in `docker/README.md`) the 14-day lifecycle rule for real S3. MinIO locally doesn't need it.

---

### P3 — Wire the Real Pipeline End to End

**Step 1 — `SceneFunc` is real** (ref: FRD §14.1)
The fan-out's per-scene function calls P2's `RenderWithRepair` with `retrieve` = agent `/snippets/search` and `codegen` = agent `/codegen`. Increments `scenes_done` on success or drop.

**Step 2 — Tail of the pipeline** (ref: FRD §11.2, §14.5, §14.6)
`concatenating` → `Concat` → `uploading` → upload → write `cache` → `done` with `video_url`. Zero clips → `done` with `video_url: null`, no cache write. Post-render: POST each successful `{source, title, description: scene.visual, category, origin: "generated", verified: false}` to `/snippets/ingest` in a goroutine; ignore errors.

**Step 3 — `/healthz` real** (ref: FRD §11.3)
Boot-time checks: Atlas ping, `docker info`, S3 `HeadBucket`, agent `/healthz`.

**Step 4 — Cancel + cache inspect** (ref: API.md §2.3, §2.5)
`DELETE /api/jobs/{id}`: cancel the job's `context.Context` so pending scenes are skipped and running containers are killed; status `cancelled`; nothing cached; idempotent. `GET /api/cache/{hash}` for pre-warm checks. Error envelope from API.md §4 on every non-2xx, including 404s.

---

### P4 — Video Playback, Recents, Failure States

**Step 1 — Video** (ref: FRD §15.1)
`<video>` appears on `video_url` in the result box without reloading the page; autoplay muted; controls visible.

**Step 2 — Progress** — "Rendering scene 2 of 3…" from `scenes_done`/`scenes_total`; "Joining scenes…" and "Uploading…" for the two new statuses.

**Step 3 — Recents store** (ref: FRD §8, §15.1, §20, §23 rule 21)
`desktop/clarity/recents.py`: `~/Library/Application Support/Clarity/recents.json` + `recents/<id>.png` + `recents/<id>_thumb.png`. `add(capture, question) → id` is called **before** `POST /api/jobs`; `update(id, job_id=…)`, then `update(id, problem_text=…)`, `update(id, explanation=…)`, `update(id, video_url=…)` from the result box's poll loop via `js_api`. Rolling cap 50 — oldest entry and its files are deleted. `clear()` for the menu item.

**Step 4 — Recents in the spotlight box** (ref: FRD §5, §15.1, §16.5)
With an empty field, **↓** or `/` expands the box downward into a list (max 8 visible): thumbnail · problem text or "Untitled capture" · relative time · a dot if `video_url` exists. Typing filters by problem text. **Enter** on a recent with a result → close spotlight, open the result box **from local data** (render `explanation`/`video_url` immediately, then one `GET /api/jobs/{id}` — a 404 is fine). **Enter** on a recent without a result, or **Tab** on any → load its screenshot as the current capture; the user types and submits a new job. Menu bar **Recents** item opens the spotlight box with no capture and the list expanded.

**Step 5 — Failure states** — `done` + null video (quiet note); `failed` with `error: "no_problem_found"` ("No problem found in that capture."); generic `failed` (plain message + retry); 404 on a live job ("expired"); 404 on a reopened recent (local copy wins); 180 s; recent's screenshot file missing (thumbnail shown, re-ask disabled).

**Step 6 — Guardrails toggle wired** — the menu checkbox writes `config.json` and every job sends the current value.

**Step 7 — Cancel on X** (ref: API.md §2.3)
Closing the result box before `done` sends `DELETE /api/jobs/{id}`. Fire-and-forget; a failure is logged, not shown.

---

### Sprint 3 Sync Point

1. **One capture → explanation → real video playing in the result box.** All real, no fakes anywhere.
1b. Press `⌘⇧E`, Esc at the crosshair, ↓ → yesterday's capture is listed; Enter reopens its result from local data with the server stopped.
2. Every codegen prompt contained ≥ 1 retrieved snippet (check agent logs).
3. Retrieval ablation numbers recorded: first-attempt success with vs without snippets.
4. Kill one scene deliberately (inject a bad import) → that scene drops, the other scenes' video still plays.
5. At least one `origin: "generated"` snippet exists in Atlas with `verified: false`.
6. Merge to `main`. Tag `sprint-3`.

---

## Sprint 4 — Cache, Guardrails, Installer (Hours 11–15)

**Goal:** everything that makes it survive real use — the cache verified across two machines, guardrails mode checked on real output, and a DMG someone else can install.

---

### P1 — Guardrails Verification, Corpus Review, Prompt Tuning

**Step 1 — Guardrails check** (ref: FRD §10.2, §24)
Run 5 math and 5 algorithm problems with `guardrails: true`. For each, does the explanation reveal the final answer? Does any scene? Record a pass rate. Tune `explain_guardrails.md` until ≥ 8/10.

**Step 2 — Promote generated snippets** (ref: FRD §10.5)
Watch every `origin: "generated"` clip from Sprint 3. Promote the good ones. Delete the bad ones. Note what made them bad — that becomes a codegen prompt rule.

**Step 3 — Prompt tuning from real failures**
Read the repair tracebacks from Sprint 3. Every recurring error class becomes either a prompt constraint or a seed snippet request to P2. **Bump `PromptVersion` on every prompt change** (tell P3 or make the one-line commit yourself).

---

### P2 — Hardening

**Step 1** — Temp-dir cleanup on every exit path, including timeout. `docker ps` after a batch of renders should show nothing.
**Step 2** — Container timeout actually kills the container: start a scene with `while True: pass`, confirm `docker ps` is empty after 120 s.
**Step 3** — `MANIM_QUALITY=-qm` path works and concat still succeeds.
**Step 4** — Seed snippets for whatever error classes P1 reports.

---

### P3 — Cache Across Machines, Robustness

**Step 1 — Two-machine cache test** (ref: FRD §12)
Two laptops, same Atlas, same problem captured at different zoom levels. Second one must be `cached: true`. If it isn't, log both normalized strings and diff them.

**Step 2 — Failure injection**
Stop the agent service mid-job → job resolves (`failed` or `done`-without-video), never hangs. Stop Docker mid-render → same. Panic in a scene goroutine → siblings finish.

**Step 3 — `503` on overload** — a bounded job queue; over the bound returns `503`.

---

### P4 — Build the Installer

**Step 1 — Self-signed cert** (ref: SETUP §11.1). Create `Clarity Dev`.
**Step 2 — `build_app.sh`** (ref: FRD §15.2) — PyInstaller with hidden imports, `--add-data` for `ui/`, patch `Info.plist` with `LSUIElement`, `codesign -s "Clarity Dev" --deep --force`. `open dist/Clarity.app` → menu bar icon. **Confirm the spotlight box is still translucent inside the bundle** — `pywebview`'s transparency depends on the window server treating the process correctly, and a bundled `.app` behaves differently from `python -m clarity`.
**Step 3 — `build_dmg.sh`** — `create-dmg` with a background and drag-to-Applications layout.
**Step 4 — Install on a second Mac** — someone who hasn't run from source. Walk the macOS 15 "Open Anyway" path. Grant permissions. Capture. Fix whatever broke. Write `desktop/RELEASE_NOTES.md` from what they hit.
**Step 5 — Rebuild, reinstall, confirm the permission persisted** (the whole point of the cert).

---

### Sprint 4 Sync Point

1. Two machines, same problem, second is `cached: true` in under 1 s.
2. Guardrails pass rate recorded and ≥ 8/10.
3. Agent killed mid-job → job resolves cleanly, result box shows the explanation.
4. `Clarity.dmg` installed on a machine that never ran from source; `⌘⇧E` works there.
5. Rebuild + reinstall did not re-prompt for Screen Recording.
6. Merge to `main`. Tag `sprint-4`.

---

## Sprint 5 — Freeze, Release, Integration (Hours 15–19)

**Goal:** everything running together on one machine, a tagged release with the installer attached, and a README a stranger can follow.

---

### All — Integration (first two hours)
Everyone on `main`. Full stack on one machine. Fix only what is broken end-to-end. No new features.

### P1 — Freeze prompts. Final `PromptVersion` bump. Pre-warm the cache with 4–5 representative problems so they're instant.
### P2 — `MANIM_QUALITY=-qm` for release. Confirm the image builds from scratch on a clean machine (`docker build --no-cache`).
### P3 — Confirm `/healthz` is all-green from a fresh boot. Confirm TTL indexes exist on the shared cluster.
### P4 — `build_pkg.sh` with the LaunchAgent (if time). `gh release create v0.1.0` with DMG, PKG, and release notes. Install from the release URL, not from `dist/`.

### All — Freeze (last hour)
No code changes. README reviewed by someone who didn't write it. Tag `v0.1.0`.

### Sprint 5 Sync Point
1. Fresh clone + SETUP.md → full stack running, by someone following the doc.
2. GitHub Release exists with `Clarity.dmg` attached; installing from it works.
3. Cache is pre-warmed; a cold problem still works end to end.
4. `main` is tagged `v0.1.0`.

---

## Merge Protocol

Everyone works until the end of the sprint on their own branch, then everyone's work is merged together. This is the whole procedure:

1. **Before the sync point**, each person: `git fetch origin && git rebase origin/main` on their branch, fix conflicts locally, run their own tests, push.
2. **At the sync point**, the merge captain merges branches into `main` **in this order**: P3 → P1 → P2 → P4. The coordinator defines the shapes everyone else consumes, so it lands first; the desktop app consumes everything, so it lands last.
3. After each merge the captain runs the sync-point checklist items that touch that path. A failure stops the merge; the owner fixes on their branch; retry.
4. Captain tags `sprint-N` and pushes.
5. Everyone: `git checkout main && git pull && git checkout -b pN/sprint-(N+1)-<desc>`.

**Merge captain rotates:** Sprint 1 → P3 · Sprint 2 → P1 · Sprint 3 → P2 · Sprint 4 → P4 · Sprint 5 → P3.

**Between sync points, nothing goes to `main`** except a change to `docs/FRD.md`, which goes there immediately in its own commit and is announced — everyone is building against it.

---

## File Ownership Summary

| Path | Person | Primary Files | Secondary (may touch with notice) |
|---|---|---|---|
| A — AI | P1 | `agent/**` | `server/internal/cache/key.go` — the one-line `PromptVersion` bump only |
| B — Render | P2 | `docker/**`, `samples/**`, `server/internal/render/**` | — |
| C — Backend | P3 | `server/cmd/**`, `server/internal/{api,store,jobs,cache,agent}/**`, `server/go.mod` | — |
| D — Desktop | P4 | `desktop/**` | — |
| All | — | `docs/**`, `AGENTS.md`, `FILE_STRUCTURE.md`, `README.md` | — |

---

## Dependency Graph

| Sprint | Who needs whom | If it's late |
|---|---|---|
| 1 | Nobody needs anybody. | — |
| 2 | P3 needs P1's `/vision` + `/explain` real. P1 needs P2's first 5 seeds to have something to embed. P4 needs P3's real coordinator for the final step. | P3 keeps hardcoded agent responses; P1 seeds from the smoke-test scene alone; P4 stays on the stub. |
| 3 (P4) | Recents needs nothing from anyone — it's local. | — |
| 3 | P3 needs P2's `RenderWithRepair`/`Concat` and P1's `/codegen`. P2 needs P1's `/codegen` for the real repair loop. | P3 keeps `SceneFunc` fake; P2 tests repair with a callback that returns a deliberately broken sample then a good one. |
| 4 | P4 needs nothing new. P3's two-machine test needs a second person's laptop. P1's promote step needs P2's renders from Sprint 3. | — |
| 5 | Everyone needs `main` green. | Fix forward; nobody branches. |

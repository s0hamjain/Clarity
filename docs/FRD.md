# Ambient Visual Learning Tool
## Functional Requirements Document
**A desktop tool that turns any on-screen math or algorithm problem into a written explanation and a custom-rendered animated video**
**Version 3.0**  |  **Status: Build-ready specification**
**Audience:** the four engineers building it, and future maintainers

### Document intent
This FRD is the single source of truth for implementation. It contains the product scope, architecture, every JSON shape and HTTP route at every component boundary, the data model, the render pipeline internals, the desktop app and installer requirements, and the implementation rules. Never guess at a schema, endpoint, or configuration value — look it up here. If something here has to change, change it here first, in its own commit, and tell the team.

---

# 1. Product Overview

A student is stuck on a hard problem — a derivative in a PDF, a recurrence on a lecture slide, a binary search that returns the wrong index in their IDE. They press a hotkey. They drag a box around the problem. A translucent Spotlight-style box appears where they can type context — *"why is my binary search not working? visualize where it's messing up"* — or pull up a recent screenshot and ask again. Within seconds a result box appears with a written, step-by-step explanation. About a minute later, a custom animated video plays in that same box, walking through the same problem visually — generated fresh for that specific problem by a model writing Manim code, not pulled from a library of pre-made clips.

| Dimension | Definition |
|---|---|
| Core problem | Written explanations are fast but abstract; good visual explanations are slow to make and only exist for common problems. Students get stuck on the specific problem in front of them, for which no video exists. |
| Primary user | A university student working a problem set, in any application on their Mac. |
| Primary value | A visual explanation of *this exact problem*, on demand, without leaving the screen it's on. |
| Product form | A macOS menu-bar application distributed as a downloadable installer, backed by a local coordinator service, a model-calling agent service, and a render pipeline. |

### One-line pitch
Press a hotkey on any problem on your screen, get a written explanation in seconds and an animated walkthrough a minute later.

---

# 2. Objectives and Non-Objectives

| Type | Item | Priority |
|---|---|---|
| Goal | Capture any region of the screen, in any application, with one hotkey. | Must |
| Goal | Deliver the written explanation the moment it exists, before any rendering starts. | Must |
| Goal | Generate and render a per-problem animation as several scenes in parallel, joined into one video. | Must |
| Goal | Ground generated Manim code in retrieved, verified snippets (RAG over MongoDB Atlas) so it renders on the first try more often. | Must |
| Goal | Never render the same problem twice — cache by problem hash. | Must |
| Goal | Ship as an installable macOS app, not a script. | Must |
| Goal | Guardrails mode: teach the method and stop short of the final answer. | Should |
| Non-goal | A browser extension or web app. The desktop app covers every surface a browser would, plus PDFs and IDEs. | Out |
| Non-goal | Windows or Linux builds. | Out |
| Goal | Recent screenshots and problems are one keystroke away, so a student can re-ask about something they captured earlier. | Should |
| Non-goal | Accounts, cloud sync, or any server-side per-user persistence. Recents live only on the user's machine. | Out |
| Non-goal | Verifying that the explanation is mathematically correct. | Out |
| Non-goal | Narration audio. Narration is on-screen text. | Out |

---

# 3. Target Users and Primary Use Cases

- **Student on a problem set:** has a PDF or a web page open, is stuck on one problem, wants to see it worked — visually — without hunting for a video that may not exist.
- **Student learning an algorithm:** has code or pseudocode in an IDE, wants to see the data structure move rather than read another paragraph about it.
- **Student who wants to be taught, not told:** turns on guardrails mode so the explanation and animation show the technique and leave the final step to them.

---

# 4. Success Criteria

- **Functional:** hotkey → region capture → spotlight box → explanation in the result box in under 10 seconds → video playing in the result box in under two minutes, on a cold request, on one laptop running the full stack.
- **Cache:** a second request for the same problem returns the existing explanation and video in under one second.
- **Resilience:** if rendering fails entirely, the user still sees the explanation with a note. The explanation never depends on rendering.
- **Installable:** a teammate who has never seen the repo downloads the DMG, drags to Applications, grants two permissions, and captures a problem.
- **Grounded generation:** generated Manim source renders without repair more often with retrieved snippets in the prompt than without. Measure it.

---

# 5. Product Surface

The product is one macOS menu-bar app. It has no main window — it lives in the menu bar and appears when summoned.

| Surface | Behavior |
|---|---|
| Menu bar icon | Always present while running. Shows idle / working state. Menu: **Capture** (same as the hotkey), **Recents**, **Guardrails** toggle, **Server…**, **Quit**. |
| Hotkey | Global, default `⌘⇧E`. Triggers a region-select capture via `screencapture -i`. |
| Spotlight box | A frameless, semi-transparent, blurred (macOS vibrancy) box centered on screen, ~680×96, opened the moment the capture finishes. Left: thumbnail of the capture. Right: a single text field — *"Add context… (why is my binary search not working? visualize where it's messing up)"*. **Enter** submits, **Esc** cancels. With the field empty, **↓** or typing `/` opens the **Recents** list below the box: the last 50 captures with thumbnail, transcribed problem text (once known), and time. Selecting a recent either reopens its result box (if a result exists) or loads that screenshot into the box so the user can ask a new question about it. |
| Result box | A second frameless always-on-top box (~440×680, `pywebview`), appears when the job is accepted. Shows status in plain words, the explanation as rendered markdown the moment it exists, scene progress, then the video player when the video is ready. One result box per job; a new job replaces the previous box. |
| Installer | A `.dmg` (drag to Applications) and optionally a `.pkg` that also installs a LaunchAgent so the app starts at login. |

**Why a desktop app, not a browser extension:** an extension sees one tab's visible viewport and refuses on `chrome://` pages. The desktop app sees everything — PDFs, IDEs, slides — and region select means the user chooses exactly what leaves the machine.

---

# 6. Core Architecture

### Architecture principle
Four components, each with one job, meeting at HTTP boundaries defined in this document. The desktop app is a thin capture-and-display surface. The coordinator is a dispatcher with no interesting logic. The agent service owns every model call. The render pipeline owns everything between "Manim source" and "MP4 on S3".

### High-level architecture flow
```text
Desktop app (macOS menu bar, Python)
  hotkey → screencapture -i → spotlight box (context + recents) → POST /api/jobs → poll → result box
        │  HTTP :8080
        ▼
Coordinator (Go)
  job lifecycle · cache lookup · per-scene fan-out · repair loop · concat · upload
        │  HTTP :8000                    │
        ▼                                 ├──► MongoDB Atlas   jobs · cache
Agent service (Python, FastAPI)           ├──► Docker          one manim-worker container per scene
  /vision  /explain  /snippets  /codegen  ├──► ffmpeg          concat, stream copy
        │                                 └──► S3 / MinIO      finished MP4s, served directly
        ├──► Claude API (Anthropic)
        ├──► Voyage AI (embeddings)
        └──► MongoDB Atlas Vector Search   manim_snippets corpus (RAG)
```

Two decisions shape everything downstream:

- **The explanation is the product; the video is a reward that arrives after.** Rendering is slow and CPU-bound (60–90 s); the explanation is fast (~8 s). They are delivered separately — the explanation the moment it exists.
- **The same problem is only rendered once.** Every transcribed problem is hashed with the user's question and the guardrails flag and checked against the cache before any work happens.

---

# 7. Technology Stack

| Layer | Technology | Role |
|---|---|---|
| Desktop app | Python 3.12 · `rumps` · `pynput` · `pywebview` (frameless, transparent, vibrancy) · `Pillow` | Menu bar app, global hotkey, spotlight box, result box, recents, image prep |
| Screen capture | macOS `screencapture -i` | Region select, built in, no dependency |
| Installer | PyInstaller → `.app` · `create-dmg` → `.dmg` · `pkgbuild` → `.pkg` (optional) | Distributable build |
| Coordinator | Go 1.22+ · `net/http` · `mongo-driver/v2` · AWS SDK v2 | Public API, job orchestration, cache, render dispatch |
| Agent service | Python 3.12 · FastAPI · `anthropic` · `voyageai` · `pymongo` | All model calls, embeddings, retrieval, corpus ingest |
| LLM | Claude Opus 5 (`claude-opus-5`) | Transcription, explanation + storyboard, codegen, repair |
| Embeddings | Voyage AI `voyage-code-3` (1024 dims) | Embeds snippet corpus and scene queries |
| Database | MongoDB Atlas (M0 free tier) | `jobs`, `cache`, `manim_snippets` collections; Atlas Vector Search index |
| Render isolation | Docker · `manim-worker` image (Python + Manim CE + LaTeX + ffmpeg) | One container per scene, `--network none` |
| Video join | ffmpeg concat demuxer, `-c copy` | No re-encode |
| Object storage | S3 (MinIO locally) | Finished videos, public-read on `renders/` |

---

# 8. Data and Storage Architecture

One database (MongoDB Atlas) holds three collections. One object store (S3, MinIO locally) holds video files. Nothing else persists.

| Store | Holds | Lifetime |
|---|---|---|
| Atlas `jobs` | One document per job; the exact shape returned by `GET /api/jobs/{id}` | 24 h TTL index on `updated_at` |
| Atlas `cache` | Problem hash → `{ video_url, explanation }` | 7 d TTL index on `created_at` |
| Atlas `manim_snippets` | RAG corpus: verified Manim CE snippets with embeddings | Permanent |
| S3 `renders/<hash>.mp4` | Finished videos | 14 d bucket lifecycle rule |
| Desktop `recents.json` + `thumbs/` | Local only: last 50 captures — thumbnail, full screenshot path, question, `job_id`, `problem_hash`, `problem_text`, `explanation`, `video_url`, timestamp | Rolling 50; user-clearable from the menu |

**Why recents store the explanation and `video_url` locally:** `jobs` expire in 24 h but the video lives 14 d. Storing the result locally means a recent can be reopened after the job record is gone, without a network call.

**Why the S3 lifetime is longer than the cache TTL:** the cache must stop pointing at a video before S3 deletes it. 7 d < 14 d guarantees that with no reaper process.

**Why Atlas and not Redis for jobs/cache:** Atlas is already in the stack for retrieval. TTL indexes give the same expiry semantics. One database is one fewer thing to install, run, and explain.

---

# 9. Database Schema (MongoDB Atlas)

Database: `avlt`. Connection string in `MONGODB_URI`.

### 9.1 `jobs`

```js
{
  _id:            "j_7f3a9c21",          // job_id, string, coordinator-generated
  status:         "rendering",            // see §11 status table
  problem_hash:   "a3f9c1d2e4b57680",     // null until /vision returns
  explanation:    "**Step 1.** ...",      // null until /explain returns; never cleared after
  scenes_total:   3,
  scenes_done:    2,
  video_url:      null,                   // set on done
  cached:         false,
  error:          null,                   // string on failed
  guardrails:     false,
  source:         "desktop",
  created_at:     ISODate(),
  updated_at:     ISODate()               // TTL index, expireAfterSeconds: 86400
}
```

Index: `{ updated_at: 1 }` with `expireAfterSeconds: 86400`. **Every write must set `updated_at` to now** or a long render can expire mid-job.

### 9.2 `cache`

```js
{
  _id:          "a3f9c1d2e4b57680",       // problem_hash
  video_url:    "http://localhost:9000/avlt-renders/renders/a3f9c1d2e4b57680.mp4",
  explanation:  "**Step 1.** ...",
  created_at:   ISODate()                 // TTL index, expireAfterSeconds: 604800
}
```

Written only after a successful render **and** upload. Never written on failure. A hit returns both fields — a video with nothing to read while it loads defeats the point.

### 9.3 `manim_snippets` (RAG corpus)

```js
{
  _id:          ObjectId(),
  title:        "MathTex with relative positioning",
  description:  "Two MathTex objects placed side by side with next_to, then transformed.",
  category:     "math",                   // "math" | "algorithm" | "general"
  tags:         ["MathTex", "next_to", "Transform"],
  source:       "from manim import *\n\nclass GeneratedScene(Scene):\n    ...",
  verified:     true,                     // true = a human confirmed it renders AND looks right
  origin:       "seed",                   // "seed" | "manim_docs" | "generated"
  embedding:    [0.0123, ...],            // 1024 floats, voyage-code-3, input_type="document"
  created_at:   ISODate()
}
```

**Vector Search index** `snippets_vector` on this collection:

```json
{
  "fields": [
    { "type": "vector", "path": "embedding", "numDimensions": 1024, "similarity": "cosine" },
    { "type": "filter", "path": "verified" },
    { "type": "filter", "path": "category" }
  ]
}
```

The text embedded for each document is `title + "\n" + description + "\n" + tags.join(" ")` — **not** the source code. Queries are natural-language scene descriptions; matching them against natural-language descriptions of what a snippet does retrieves better than matching against code.

---

# 10. Agent Service API (internal, Python, `:8000`)

Not exposed to the desktop app — only the coordinator calls it. Every response is JSON produced via structured outputs (`output_config={"format": ...}`); no markdown fences, no prose. Every request carries `guardrails: bool`.

### 10.1 `POST /vision`

```json
// request
{ "image_b64": "iVBORw0KG...", "media_type": "image/png", "guardrails": false }

// response
{ "problem_text": "Differentiate f(x) = x^2 sin(x) using the product rule.", "category": "math", "confidence": 0.94 }
```

| Field | Rule |
|---|---|
| `problem_text` | **Verbatim transcription.** No paraphrase, no added context. This string is hashed — editorializing destroys the cache. |
| `category` | `"math"` \| `"algorithm"` \| `"unknown"`. Selects the explainer prompt and filters snippet retrieval. |
| `confidence` | 0.0–1.0. Below 0.5 the coordinator proceeds but flags the job. |

Nothing problem-like on screen → `category: "unknown"`, `problem_text: ""`; the coordinator fails the job cleanly.

The coordinator strips the `data:image/png;base64,` prefix before calling this endpoint. The agent receives raw base64 only.

**Determinism (decided):** the cache needs two captures of the same problem to transcribe identically. `temperature` is not available on Claude Opus 5 (sampling parameters return 400). This call therefore runs on Opus 5 with structured outputs and `output_config={"effort": "low"}`, and relies on the schema plus `normalize()` (§12). If the collision experiment in Sprint 1 shows that isn't enough, the fallback is `claude-haiku-4-5` with `temperature=0` for this endpoint only.

### 10.2 `POST /explain`

```json
// request
{ "problem_text": "...", "category": "math", "user_prompt": "why a cosine?", "guardrails": false }

// response
{
  "explanation": "**Step 1.** The product rule says ...",
  "storyboard": {
    "title": "The Product Rule",
    "scenes": [
      { "index": 0, "narration": "Two functions multiplied together.", "visual": "Show f(x) = x^2 sin(x). Split into u = x^2 and v = sin(x), each moving to its own side.", "duration_seconds": 8 },
      { "index": 1, "narration": "The derivative is u'v + uv'.", "visual": "Transform the split expression into u'v + uv', highlighting each term as it appears.", "duration_seconds": 10 }
    ]
  }
}
```

| Field | Rule |
|---|---|
| `explanation` | Markdown. Written to `jobs` the instant it lands. |
| `storyboard.scenes` | **2–5 items.** Each is an independent Manim `Scene`, rendered in its own container, concatenated in order. |
| `scenes[].narration` | ≤ 90 chars. Becomes on-screen text. |
| `scenes[].visual` | Relative terms only ("below", "next to", "replacing"). Never coordinates. |
| `scenes[].duration_seconds` | 5–15. A budget. |

Every scene must show something text cannot: a function and its derivative plotted together, a pointer walking a list, a shape transforming. A scene that restates algebra is cut.

**Guardrails mode:** explanation and scenes teach the *method*, not the *result*. Math: show the rule, work the setup, leave the final substitution. Code: walk the logic and data flow, emit a skeleton with decision points named, never a complete solution. This is a prompt instruction, not a filter — verify it on real output.

### 10.3 `POST /snippets/search`

Retrieval, not generation. Given one scene, return the most relevant verified snippets from `manim_snippets`.

```json
// request
{ "scene": { "index": 0, "narration": "...", "visual": "Show f(x) = x^2 sin(x), split into u and v." }, "category": "math", "k": 3 }

// response
{ "snippets": [ { "title": "MathTex with relative positioning", "description": "...", "source": "from manim import *\n...", "score": 0.87 } ] }
```

Implementation: embed `narration + " " + visual` with `voyage-code-3`, `input_type="query"`; run `$vectorSearch` on `snippets_vector` with `numCandidates: 50`, `limit: k`, filter `verified: true` and `category ∈ {request.category, "general"}`. Return `score` from `$meta: "vectorSearchScore"`.

### 10.4 `POST /codegen`

Per scene. Snippets are passed in so the coordinator controls retrieval and the call is reproducible.

```json
// request
{
  "scene": { "index": 0, "narration": "...", "visual": "..." },
  "snippets": [ { "title": "...", "source": "..." } ],
  "previous_source": null,
  "traceback": null,
  "guardrails": false
}

// response
{ "manim_source": "from manim import *\n\nclass GeneratedScene(Scene):\n    ...", "scene_class": "GeneratedScene" }
```

| Field | Rule |
|---|---|
| `manim_source` | Complete runnable Manim CE Python. One file. Imports: `from manim import *` and stdlib only. |
| `scene_class` | **Always the literal `"GeneratedScene"`.** Anything else = failure, trigger repair. |

Repair: same endpoint with `previous_source` and `traceback` set. On repair, the coordinator re-runs `/snippets/search` with the traceback's first line appended to the query, so a `MathTex` LaTeX error retrieves a snippet showing correct `MathTex` usage.

Prompt constraints: relative positioning only (`next_to`, `arrange`, `to_edge`, `shift` by fractions of `config.frame_width`); never literal coordinates; never set resolution or frame rate; `narration` → `Text(...)` at `to_edge(DOWN)`; retrieved snippets included verbatim as the reference to imitate.

### 10.5 `POST /snippets/ingest`

Adds a snippet to the corpus. Used by the seed script and by the coordinator after a successful render.

```json
// request
{ "title": "...", "description": "...", "category": "math", "tags": ["..."], "source": "...", "origin": "generated", "verified": false }

// response
{ "id": "66f1...", "embedded": true }
```

Generated snippets land with `verified: false` and are **excluded from retrieval** until a human runs `scripts/promote_snippet.py <id>` after watching the clip. A scene that renders but looks wrong must not become a reference.

### 10.6 `GET /healthz`

`{ "ok": true, "anthropic": true, "voyage": true, "atlas": true, "snippets_verified": 42 }`

### 10.7 Models

`claude-opus-5` for every call. Adaptive thinking is on by default. `output_config={"effort": "low"}` on `/vision`; default effort elsewhere. Structured outputs for every response. No assistant prefill (returns 400 on Opus 5). Use `client.messages.stream(...)` for `/codegen` — output can be long.

---

# 11. Coordinator API (public, Go, `:8080`)

### 11.1 `POST /api/jobs`

Returns before any model call happens.

```json
// request
{ "image": "data:image/png;base64,iVBORw0KG...", "user_prompt": "why does the second term have a cosine?", "guardrails": false, "source": "desktop" }

// 202 Accepted
{ "job_id": "j_7f3a9c21" }
```

Errors: `400` bad body · `413` image over 8 MB · `503` queue full.

### 11.2 `GET /api/jobs/{job_id}`

The only endpoint the desktop app polls. Every **1 s**, give up at **180 s**. `404` on unknown or expired.

```json
{
  "job_id": "j_7f3a9c21", "status": "rendering", "problem_hash": "a3f9c1d2e4b57680",
  "explanation": "**Step 1.** ...", "scenes_total": 3, "scenes_done": 2,
  "video_url": null, "cached": false, "error": null, "updated_at": "2026-09-12T02:14:03Z"
}
```

| Status | Meaning | `explanation` | `scenes_done` | `video_url` |
|---|---|---|---|---|
| `queued` | accepted, not started | null | 0 | null |
| `transcribing` | `/vision` in flight | null | 0 | null |
| `explaining` | `/explain` in flight | null | 0 | null |
| `generating` | per-scene `/snippets/search` + `/codegen` in flight | **set** | 0 | null |
| `rendering` | scenes rendering in Docker, concurrently | set | 0..N | null |
| `concatenating` | ffmpeg joining clips | set | N | null |
| `uploading` | pushing MP4 to S3 | set | N | null |
| `done` | video live at `video_url` — **or** `video_url: null` if zero scenes survived | set | N | set or null |
| `failed` | ended before `/explain` returned | null | 0 | null |

**Two rules the desktop app must honor:**
1. Render `explanation` the instant it is non-null, regardless of status.
2. `done` with `video_url: null` shows the explanation plus "The animation didn't render this time." Not an error state.

### 11.3 `GET /healthz`

`{ "ok": true, "atlas": true, "docker": true, "s3": true, "agent": true }` — `docker`, `s3`, `agent` checked at boot.

### 11.4 Video delivery

`video_url` is a direct S3 (or MinIO) URL. The desktop app's result box streams from it; the coordinator never proxies video.

---

# 12. Cache Key

```
sha256(PromptVersion + "||" + normalize(problem_text) + "||" + normalize(user_prompt) + "||" + guardrails)[:16]
```

Hex, first 16 chars. `normalize` = lowercase, trim, collapse every whitespace run (including newlines) to one space. Nothing else. `guardrails` serializes as `"true"` / `"false"`. Implemented once, in the coordinator (`server/internal/cache/key.go`), with a unit test.

- `PromptVersion` is a hand-bumped `const`. Bump it whenever *any* prompt changes — including prompts in the agent service. Without it, a prompt improvement appears to do nothing.
- Matching is exact. Normalization is the only defense against transcription jitter.

---

# 13. RAG Design — the Manim Snippet Corpus

### Why
Generated Manim fails two ways. Code that crashes is recoverable via the repair loop. Code that runs but produces a bad video (overlapping text, objects off the 14.2-unit frame) exits zero and cannot be detected. The only lever on the second failure is what the model imitates. Retrieval puts verified, good-looking, relevant code in front of it for every scene.

### Corpus
| Origin | Content | Count target | Who |
|---|---|---|---|
| `seed` | Hand-written scenes, each rendered and viewed. `samples/*.py`, one scene per file, docstring header = title/description/tags/category. | 20–30 by end of Sprint 2 | P2 writes, P1 ingests |
| `manim_docs` | Worked examples from the Manim CE docs (Text, MathTex, Axes/plotting, Transform, VGroup/arrange, Table, Code, Graph) | 30–50 | P1 |
| `generated` | Source from successful renders, ingested `verified: false`, promoted by hand | grows | coordinator → P1's ingest endpoint |

### Pipeline
```text
seed .py files ──► scripts/seed_snippets.py ──► embed (voyage-code-3, "document") ──► manim_snippets
scene {narration, visual} ──► embed ("query") ──► $vectorSearch k=3, verified=true ──► codegen prompt
render succeeded ──► POST /snippets/ingest (verified=false) ──► promote_snippet.py after review
```

### Rules
- Embed `title + description + tags`, not source. Queries are prose; match prose.
- Only `verified: true` is ever retrieved. No exceptions in code paths.
- `numCandidates` ≥ 10× `limit`. Start at 50 / 3.
- Filter by `category` ∈ {scene's category, `"general"`}. A pointer-walking-a-list snippet should not be retrieved for a derivative.
- Same embedding model for index and query, always. Changing models means re-embedding the whole corpus.

---

# 14. Render Pipeline

### 14.1 Per-scene fan-out
After `/explain` returns N scenes, the coordinator starts N goroutines bounded by a semaphore (`RENDER_CONCURRENCY`). Each independently: `/snippets/search` → `/codegen` → static pre-check → `docker run` → on failure, repair up to 3 times **for that scene only**. The coordinator waits for all N, then concatenates whatever succeeded.

```go
func Render(ctx context.Context, src string, workDir string) (clipPath string, err *RenderError)
type RenderError struct { Stage string /* "precheck"|"container"|"timeout" */; Traceback string }
func RenderWithRepair(ctx context.Context, scene Scene, retrieve RetrieveFunc, codegen CodegenFunc) (clipPath string, err *RenderError)
func Concat(ctx context.Context, clipPaths []string, outKey string) (videoURL string, err error)
```

A scene that exhausts repair is **dropped, not the job**. Zero surviving scenes → `done` with `video_url: null`.

### 14.2 Static pre-check (before every `docker run`)
Parses as Python (`python -c "import ast,sys; ast.parse(sys.stdin.read())"`); rejects any import not `manim`/stdlib; rejects `os.system`, `subprocess`, `open(` for writing, `__import__`, `eval`, `exec`; requires `class GeneratedScene(Scene)`. Cheap, and turns most failures into a fast traceback instead of a slow container.

### 14.3 Docker isolation
```sh
docker run --rm --network none --memory 1g --cpus 1 -v <tmpdir>:/work manim-worker \
  manim -qm /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media
```
Hard per-scene timeout 120 s via `context.Context` + `docker kill`. Quality flag is a **job-level constant** passed identically to every scene.

### 14.4 Concat — no re-encode
```sh
ffmpeg -f concat -safe 0 -i concat_list.txt -c copy final.mp4
```
Works only because every scene shares resolution/fps. Guarded by 14.3's job-level constant.

### 14.5 Post-render ingest
On a successful clip, the coordinator POSTs `{ source, title: storyboard.title + " — scene " + index, description: scene.visual, category, origin: "generated", verified: false }` to `/snippets/ingest`. Fire-and-forget; a failed ingest never fails the job.

### 14.6 Storage
Upload to `s3://<RENDER_BUCKET>/renders/<hash>.mp4`, public-read on the prefix. Bucket lifecycle: 14 d. Write `cache` and update `jobs` only after upload succeeds.

---

# 15. Desktop App and Installer

### 15.1 Runtime behavior
| Step | Requirement |
|---|---|
| Launch | Menu bar icon appears. No dock icon (`LSUIElement = true`). First launch triggers Screen Recording and Input Monitoring permission prompts; the app explains that a restart is needed after granting. |
| Hotkey | `⌘⇧E` by default, via `pynput.keyboard.GlobalHotKeys`. Debounced (2 s). Also available as a menu item. |
| Capture | `screencapture -i -x <tmp>.png`. Esc cancels (exit code 1, no file) → abort silently. |
| Image prep | `Pillow` downscales to ≤ 1568 px on the longest side (Claude's sweet spot; keeps retina captures under the 8 MB cap). Encoded as PNG data URL. |
| Spotlight box | `pywebview.create_window(frameless=True, transparent=True, vibrancy=True, on_top=True, easy_drag=False)`, ~680×96, centered on the display the cursor is on, loads `ui/spotlight/index.html`. Input focused on open. **Enter** submits, **Esc** cancels and closes. Thumbnail of the capture on the left. |
| Recents in the box | With an empty field, **↓** or `/` expands the box downward into a list (max 8 visible, scroll for 50): thumbnail · problem text or "Untitled capture" · relative time · a dot if a video exists. **Enter** on a recent with a result → open its result box. **Enter** on a recent without one, or **Tab** on any recent → load that screenshot into the box for a new question. Typing with the list open filters by problem text. |
| Submit | `POST /api/jobs` with `source: "desktop"` and the current guardrails setting. Write the recent immediately (before the response) so a failed submit is still visible in recents. On connection failure: notification "Can't reach the server", spotlight box stays open. |
| Result box | `pywebview` window, ~440×680, `frameless=True, on_top=True, vibrancy=True`, appears when the job ID comes back, loads `ui/result/index.html?job=&server=`. Polls every 1 s. Draggable by its header. Close button. A new job closes the previous result box. |
| Result content | Status line in plain words ("Reading the problem…", "Writing explanation…", "Rendering scene 2 of 3…", "Done"). Explanation rendered from markdown (`marked.min.js`, bundled — no CDN). `<video controls autoplay muted>` when `video_url` arrives. `done` with null video → explanation + quiet note. 180 s timeout → message + retry. Every poll that adds `problem_text`, `explanation`, or `video_url` updates the local recent. |
| Reopen from recents | If the local recent has `explanation`/`video_url`, render from local data first, then poll `GET /api/jobs/{id}` once; a `404` is fine — the local copy is the source. |
| Notification | macOS notification when the explanation lands, so the user doesn't have to watch the window. |
| Config | `SERVER_URL` from `~/Library/Application Support/AVLT/config.json`, default `http://localhost:8080`, editable via the **Server…** menu item. |

### 15.2 Build and installer
| Artifact | How | Priority |
|---|---|---|
| `AVLT.app` | `pyinstaller --windowed --name AVLT --icon assets/icon.icns desktop/avlt/main.py`, with `--hidden-import` for `rumps`, `pynput.keyboard._darwin`, `webview`. `LSUIElement` added to `Info.plist` post-build. `ui/` bundled via `--add-data`. | Must |
| Stable identity | Sign with a **free self-signed certificate** (`codesign -s "AVLT Dev"`) so macOS keys the Screen Recording permission to a stable identity and it survives rebuilds. | Must |
| `AVLT.dmg` | `create-dmg` with drag-to-Applications layout and background. | Must |
| `AVLT.pkg` | `pkgbuild` + `productbuild`; post-install script writes `~/Library/LaunchAgents/com.avlt.app.plist` so it starts at login. | Should |
| Release | Attached to a GitHub Release with install instructions including the "Open Anyway" step for unsigned apps on macOS 15. | Must |
| Notarization | Deferred. Requires Apple Developer Program. Everything above works without it; the user does one "Open Anyway" per install. | Out (for now) |

---

# 16. Functional Requirements — End-to-End Flow

## 16.1 Install
1. User downloads `AVLT.dmg`, drags to Applications, opens.
2. macOS 15 blocks the unsigned app → System Settings → Privacy & Security → Open Anyway.
3. App requests Screen Recording; user grants; app asks to restart. App requests Input Monitoring on first hotkey registration; same.
4. Menu bar icon appears. `⌘⇧E` is live.

## 16.2 Capture
1. User presses `⌘⇧E` on any screen. Crosshair appears. User drags a box around the problem.
2. The spotlight box appears with the capture's thumbnail. User types context — or presses ↓ to pick a recent screenshot instead — and presses Enter.
3. The spotlight box closes; the result box opens with "Reading the problem…".

## 16.3 Explain
1. Coordinator returns a job ID immediately and starts the pipeline.
2. `/vision` transcribes. Coordinator hashes and checks `cache`.
3. On a hit: the result box shows explanation and video within one poll.
4. On a miss: `/explain` runs; the result box shows the explanation the moment it is written to `jobs`. A notification fires. The local recent is updated with `problem_text` and `explanation`.

## 16.4 Render
1. For each scene, concurrently: retrieve snippets, generate source, pre-check, render in a container, repair on failure up to 3×.
2. The result box shows "Rendering scene k of N…" as `scenes_done` advances.
3. Successful clips are concatenated and uploaded. `cache` is written. Successful sources are ingested as unverified snippets.
4. The result box plays the video. Job is `done`. The local recent gets `video_url`.

## 16.5 Re-ask
1. User presses `⌘⇧E`, then **Esc** at the crosshair (or opens **Recents** from the menu). The spotlight box opens with no fresh capture and the recents list expanded.
2. User picks yesterday's binary-search screenshot, types *"now show me the fix"*, Enter.
3. New job with the stored screenshot and the new question. Different `user_prompt` → different cache key → new explanation and video.

## 16.6 Degrade
- Zero scenes survive → `done`, `video_url: null`, explanation shown with a note.
- `/vision` returns `unknown` → `failed`, result box shows "No problem found in that capture."
- Server unreachable → notification; spotlight box stays open so the user can retry.
- Reopening a recent whose `jobs` record expired → rendered from the local copy; if the local copy has no result, the user is offered to re-ask.

---

# 17. Feature-by-Feature Requirements

### Desktop app
| # | Requirement | Priority |
|---|---|---|
| F1 | Global hotkey triggers a region-select capture in any application. | Must |
| F2 | Captured image is downscaled to ≤ 1568 px and sent as a PNG data URL. | Must |
| F3 | A translucent, frameless, centered spotlight box collects optional context before submission; Enter submits, Esc cancels. | Must |
| F4 | A guardrails toggle in the menu bar controls the `guardrails` flag on every job. | Should |
| F5 | The result box renders the explanation the instant it is non-null. | Must |
| F6 | The result box shows scene progress from `scenes_done` / `scenes_total`. | Should |
| F7 | The result box plays the video inline when `video_url` is set. | Must |
| F8 | `done` with no video shows the explanation and a non-error note. | Must |
| F9 | Server unreachable produces a notification, never a crash or traceback. | Must |
| F10 | Server URL is user-configurable and persisted. | Should |
| F11 | App runs without a dock icon and appears only in the menu bar. | Must |
| F41 | Every capture is recorded locally as a recent (thumbnail, screenshot, question, job id) before the request is sent. | Should |
| F42 | The spotlight box lists recents (↓ or `/` with an empty field), filterable by problem text, showing thumbnail, problem text, time, and whether a video exists. | Should |
| F43 | Selecting a recent with a result reopens its result box from local data, even if the server job has expired. | Should |
| F44 | Selecting a recent without a result, or Tab on any recent, loads its screenshot into the spotlight box for a new question. | Should |
| F45 | Recents are capped at 50 and can be cleared from the menu bar. | Could |

### Installer
| # | Requirement | Priority |
|---|---|---|
| F12 | `AVLT.app` builds reproducibly from `scripts/build_app.sh`. | Must |
| F13 | The app is signed with a stable (self-signed) identity so permissions survive rebuilds. | Must |
| F14 | `AVLT.dmg` is produced by `scripts/build_dmg.sh` and attached to a GitHub Release. | Must |
| F15 | `AVLT.pkg` installs a LaunchAgent for start-at-login. | Should |
| F16 | README documents the macOS 15 "Open Anyway" step. | Must |

### Agent service
| # | Requirement | Priority |
|---|---|---|
| F17 | `/vision` returns verbatim problem text, category, confidence via structured outputs. | Must |
| F18 | `/explain` returns markdown explanation and a 2–5 scene storyboard. | Must |
| F19 | `/snippets/search` returns top-k verified snippets from Atlas Vector Search. | Must |
| F20 | `/codegen` returns runnable Manim CE source with `class GeneratedScene`, using provided snippets. | Must |
| F21 | `/codegen` repair path takes previous source and traceback. | Must |
| F22 | `/snippets/ingest` embeds and stores a snippet; generated snippets land unverified. | Must |
| F23 | Guardrails mode changes `/explain` and `/codegen` output to teach the method without the final answer. | Should |
| F24 | Seed script ingests every `samples/*.py` with its docstring metadata. | Must |
| F25 | Promote script flips a snippet to `verified: true`. | Should |

### Coordinator
| # | Requirement | Priority |
|---|---|---|
| F26 | `POST /api/jobs` returns in < 50 ms; all work happens after the response. | Must |
| F27 | Job status advances through every state in §11.2 and is readable at every poll. | Must |
| F28 | `explanation` is written to `jobs` before any scene work starts. | Must |
| F29 | Cache key matches §12 exactly, with a unit test. | Must |
| F30 | Cache hit returns explanation and video without calling `/explain`. | Must |
| F31 | Scenes fan out concurrently, bounded by `RENDER_CONCURRENCY`. | Must |
| F32 | A scene exhausting repair is dropped; the job completes with the rest. | Must |
| F33 | Successful sources are POSTed to `/snippets/ingest`, fire-and-forget. | Should |
| F34 | Every `jobs` write sets `updated_at`. | Must |

### Render pipeline
| # | Requirement | Priority |
|---|---|---|
| F35 | `manim-worker` image builds and renders a `MathTex` scene inside a container. | Must |
| F36 | Static pre-check rejects banned imports and calls before any container starts. | Must |
| F37 | Each scene renders in its own `--network none` container with a 120 s timeout. | Must |
| F38 | Clips concatenate with `-c copy` and no re-encode. | Must |
| F39 | Final MP4 uploads to S3 and `video_url` is a direct URL. | Must |
| F40 | Output clips are validated (size floor; `ffprobe` duration if cheap) before being counted as success. | Should |

---

# 18. Non-Functional Requirements

| # | Requirement |
|---|---|
| N1 | Explanation latency < 10 s p50, cold. |
| N2 | Video latency < 120 s p50, cold, including repair. |
| N3 | Cache hit < 1 s. |
| N4 | Concurrent renders capped at `RENDER_CONCURRENCY`, default `NumCPU/2`. |
| N5 | No scene can wedge another — per-container timeout, `defer recover()` per goroutine. |
| N6 | Full stack runs on one laptop; no cloud dependency except Claude, Voyage, and Atlas. |
| N7 | One command per service to start (§SETUP). |
| N8 | Desktop app cold start < 2 s to menu bar icon. |

---

# 19. Error Handling and Fallbacks

| Failure | Behavior |
|---|---|
| `/vision` → `unknown` | Job `failed`, `error: "no_problem_found"`. Panel: "No problem found in that capture." |
| `/explain` error | Job `failed`, `error` set. Panel: plain error + retry. |
| `/snippets/search` error | Log, proceed with empty snippets. Retrieval is an improvement, not a dependency. |
| `/codegen` error | Counts as a failed attempt for that scene; repair loop continues. |
| Pre-check reject | Counts as a failed attempt; the rejection reason is the traceback. |
| Container timeout | `docker kill`, failed attempt. |
| Scene exhausts 3 attempts | Dropped. Job continues. |
| Zero scenes survive | `done`, `video_url: null`. Cache **not** written. |
| Concat or upload error | `done`, `video_url: null`, `error` set. Cache not written. |
| Ingest error | Logged, ignored. |
| Atlas unreachable | Coordinator refuses new jobs with `503`; `/healthz` reports `atlas: false`. |
| Desktop: server unreachable | Notification. Spotlight box stays open for retry. |
| Desktop: poll 404 on a live job | Result box: "This job expired." |
| Desktop: 404 when reopening a recent | Render from the local copy. If none, offer to re-ask. |
| Desktop: 180 s without `done` | Result box: message + retry. |
| Desktop: recent's screenshot file missing | Show the thumbnail, disable re-ask for that entry. |

---

# 20. Security and Credential Handling

- `ANTHROPIC_API_KEY`, `VOYAGE_API_KEY`, `MONGODB_URI`, S3 credentials live in `agent/.env` and `server/.env`. Never committed. Never in the desktop app.
- The desktop app holds no secrets. It talks only to the coordinator.
- Generated code runs only inside `--network none` containers, after a static pre-check. Never on the host.
- Server side, screenshots are processed and discarded. The coordinator logs the hash, never the image. Images are not written to Atlas or S3.
- Client side, screenshots are kept **only on the user's machine** in `~/Library/Application Support/AVLT/recents/` for the recents feature. Rolling 50. The menu bar has **Clear Recents**. Nothing in recents is ever uploaded except when the user explicitly re-asks.
- The coordinator has no auth. It is bound to `localhost` by default. Exposing it beyond the machine requires a bearer token and rate limiting first — out of scope here, documented so nobody does it by accident.

---

# 21. MVP vs Stretch Scope

| MVP (must ship) | Stretch (if time) | Deferred |
|---|---|---|
| Hotkey → region capture → spotlight box → result box | Guardrails toggle end-to-end | Notarization |
| `/vision`, `/explain`, `/codegen` real | `/snippets/ingest` from successful renders | Windows / Linux |
| Atlas Vector Search retrieval with seeded corpus | PKG installer with LaunchAgent | Narration audio |
| Per-scene Docker render, repair, concat, S3 | `ffprobe` clip validation | Correctness verification |
| Cache hit path | Promote script + review flow | Auth on the coordinator |
| DMG installer, self-signed | Notification on explanation | Accounts / history |

---

# 22. Environment Variables

### `agent/.env`
| Var | Example | Purpose |
|---|---|---|
| `ANTHROPIC_API_KEY` | `sk-ant-…` | Claude |
| `VOYAGE_API_KEY` | `pa-…` | Embeddings |
| `MONGODB_URI` | `mongodb+srv://…/avlt` | Atlas |
| `MONGODB_DB` | `avlt` | Database name |
| `EMBED_MODEL` | `voyage-code-3` | Must match the vector index dims |

### `server/.env`
| Var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | |
| `AGENT_URL` | `http://localhost:8000` | |
| `MONGODB_URI` | — | Atlas |
| `MONGODB_DB` | `avlt` | |
| `S3_ENDPOINT` | `http://localhost:9000` | MinIO locally |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `minioadmin` / `minioadmin` | |
| `RENDER_BUCKET` | `avlt-renders` | |
| `RENDER_CONCURRENCY` | `NumCPU/2` | |
| `RENDER_TIMEOUT_SEC` | `120` | |
| `MANIM_QUALITY` | `-ql` | `-qm` for release |

### Desktop (`~/Library/Application Support/AVLT/config.json`)
| Key | Default |
|---|---|
| `server_url` | `http://localhost:8080` |
| `guardrails` | `false` |
| `hotkey` | `<cmd>+<shift>+e` |

---

# 23. Key Implementation Rules

### Agent service
1. Every response uses structured outputs. Never parse prose for JSON.
2. `/vision` prompt contains no instruction to interpret, summarize, or contextualize. Verbatim only.
3. `scene_class` is asserted equal to `"GeneratedScene"` before returning from `/codegen`.
4. Retrieval filters on `verified: true` in every code path. No debug flag disables it.
5. Index and query use the same `EMBED_MODEL`. Changing it means re-running the seed script.
6. Use `client.messages.stream()` for `/codegen`; do not use assistant prefill; do not pass `temperature` to Opus 5.

### Coordinator
7. `POST /api/jobs` writes the job and returns. Everything else is in a goroutine with `defer recover()`.
8. `explanation` is written to `jobs` before any `/snippets/search` or `/codegen` call.
9. Every `jobs` write sets `updated_at = now`.
10. `cache` is written only after upload succeeds. Never on any failure path.
11. Cache hit path never calls `/explain`.
12. The manim quality flag is one job-level value passed to every scene.
13. `PromptVersion` is bumped in the same commit as any prompt change, in any component.

### Render
14. Static pre-check runs before every `docker run`, including repairs.
15. `docker run` always has `--network none`, `--memory`, `--cpus`, and a context timeout.
16. A scene failure never propagates to sibling scenes.

### Desktop
17. No secrets in the app. Only `server_url` and preferences.
18. Poll interval 1 s, timeout 180 s. Render `explanation` on first non-null.
19. Every network error becomes a notification or a message in the result box. Never an unhandled exception.
20. `ui/` (spotlight and result) loads nothing from the network except `video_url`. `marked.min.js` is bundled.
21. A recent is written to disk before `POST /api/jobs` is sent, and updated on every poll that adds `problem_text`, `explanation`, or `video_url`. Reopening a recent never depends on the server.

---

# 24. Open Questions

| Question | Owner | Decide by |
|---|---|---|
| Does transcription collide often enough for the cache to matter? Run the six-capture experiment. | P1 | End of Sprint 1 |
| Does retrieval measurably reduce repair attempts? Compare 10 scenes with and without snippets. | P1 | End of Sprint 3 |
| Does guardrails mode actually withhold the answer on real problems? | P1 | End of Sprint 4 |
| Is `voyage-code-3` the right embedding model for prose→prose matching, or should it be `voyage-3.5`? | P1 | End of Sprint 2 (before the corpus is large) |
| Does the self-signed cert keep Screen Recording permission stable across rebuilds in practice? | P4 | End of Sprint 4 |
| Does `pywebview` `transparent=True` + `vibrancy=True` render correctly inside a PyInstaller bundle, or does the spotlight box need a solid fallback? | P4 | End of Sprint 2 |

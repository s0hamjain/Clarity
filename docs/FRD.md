# Clarity
## Functional Requirements Document
**A desktop tool that turns any on-screen math or algorithm problem into a written explanation and a custom-rendered animated video**
**Version 3.0**  |  **Status: Build-ready specification**
**Audience:** the four engineers building it, and future maintainers

### Document intent
This FRD is the single source of truth for implementation. It contains the product scope, architecture, every JSON shape and HTTP route at every component boundary, the data model, the render pipeline internals, the desktop app and installer requirements, and the implementation rules. Never guess at a schema, endpoint, or configuration value — look it up here. If something here has to change, change it here first, in its own commit, and tell the team.

---

# 0. Read This First

This is the spec. It's long because it's exact — every JSON field, every status string, every Docker flag. You are not expected to read it top to bottom. **Open the section your task points to.** Section numbers are stable; other docs cite them as "FRD §10.3".

If a word here is unfamiliar, the [README glossary](../README.md#glossary) defines it. The ones that matter most:

| Term | In one line |
|---|---|
| **Job** | One request from screenshot to video, with an ID and a status that advances through fixed steps (§11.2). |
| **Coordinator** | The Go server the desktop app talks to. Orchestrates; does no AI or rendering itself (§11). |
| **Agent service** | The Python server that makes every AI call — Gemini reads the screenshot and writes the explanation, Claude Sonnet writes the Manim code (§10). |
| **Storyboard / scene** | The model's plan for the animation: 2–5 scenes, each rendered separately then stitched (§10.2, §14). |
| **Snippet corpus** | Verified working Manim examples in MongoDB; the 3 most similar are shown to the model before it writes code. This is the RAG part (§13). |
| **Cache key** | The fingerprint of a problem; same fingerprint means reuse the existing video (§12). |
| **Spotlight box / result box** | The two floating windows of the desktop app: input, then output (§5, §15). |

How the sections group:

| If you're working on… | Read |
|---|---|
| Anything | §1–§6 (what and why), §23 (rules) |
| The agent service (P1) | §9.3, §10, §13, §22 |
| The render pipeline (P2) | §13 (corpus you seed), §14 |
| The coordinator (P3) | §8, §9.1–9.2, §11, §12, §14.1, §19 |
| The desktop app (P4) | §5, §15, §16, §19 (desktop rows) |

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
The user flow, in order:

1. **Hotkey** `⌘⇧E` → crosshair → the user **drags a region** around the problem → screenshot taken.
2. **Spotlight box animates in** (fade + scale, ~180 ms) — a clear, blurred, frameless box with the capture's thumbnail and one text field for context (*"why is my binary search not working? visualize where it's messing up"*). Optional. **↓** shows recent screenshots.
3. **Enter** → spotlight box animates out → job submitted.
4. **Result box appears** — a clear, frameless, always-on-top box. Explanation first (seconds), then the video (about a minute). **Drag it anywhere** by its background so it never blocks the problem.
5. **X** (top-right) or **Esc** closes the result box. Done.

| Surface | Definition |
|---|---|
| Menu bar icon | Always present. Idle / working state. Menu: **Capture**, **Recents**, **Guardrails**, **Server…**, **Clear Recents**, **Quit**. |
| Hotkey | Global `⌘⇧E`. Region select via `screencapture -i`. Esc at the crosshair cancels. |
| Spotlight box | Frameless · transparent · vibrancy · ~680×96 · centered. Thumbnail left, text field right. Enter submits, Esc cancels. Empty field + **↓** or `/` expands into the **Recents** list (last 50: thumbnail · problem text · time · video dot). Enter on a recent with a result reopens it; Enter/Tab on one without a result loads its screenshot for a new question. |
| Result box | Frameless · transparent · vibrancy · always on top · ~440×680. Opens on job accept, near the spotlight box's position. **Draggable anywhere** (whole background is a drag handle; the video and text are not). Status line → explanation (markdown) → scene progress → video. **X** top-right and **Esc** close it. Each job gets its own box; boxes stack offset so several can be open. |
| Installer | `.dmg` (drag to Applications); optional `.pkg` with a LaunchAgent for start at login. |

**Why a desktop app, not a browser extension:** an extension sees one tab's visible viewport and refuses on `chrome://` pages. The desktop app sees everything — PDFs, IDEs, slides — and region select means the user chooses exactly what leaves the machine.

---

# 6. Core Architecture

### Architecture principle
Four components, each with one job, meeting at HTTP boundaries defined in this document. The desktop app is a thin capture-and-display surface. The coordinator owns the **job**: create it, track it, fan scenes out, stitch and upload — no model calls, no loops of its own. The agent service owns the **thinking**: three LangGraph graphs that call models, retrieve from the vector store, and run their own bounded loops (critique-and-revise; generate-lint-render-repair). The render pipeline owns everything between "Manim source" and "MP4 on S3", exposed to the agent as a single internal HTTP tool.

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
Agent service (Python, FastAPI, LangGraph)◄┤  POST /internal/render (agent's render tool)
  /vision   Intake chain                  ├──► Docker          one manim-worker container per scene
  /explain  Explainer agent               ├──► ffmpeg          concat, stream copy
  /scenes/render  Manim Generator agent   └──► S3 / MinIO      finished MP4s, served directly
        │
        ├──► Google Gemini API (OCR + explanation)
        ├──► Claude API (Sonnet 5 — Manim code only)
        ├──► Voyage AI (embeddings)
        └──► MongoDB Atlas Vector Search   manim_snippets corpus — the agent's vector store
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
| Agent service | Python 3.12 · FastAPI · **LangGraph** · **LangChain** (`langchain-core`, `langchain-google-genai`, `langchain-anthropic`, `langchain-mongodb`, `langchain-voyageai`) · **Pydantic v2** | Three agents (Intake, Explainer, Manim Generator); every model call; the vector store; corpus ingest |
| OCR / vision | Google Gemini 3.8 Flash (`gemini-3.8-flash`) via `google-genai` | Reads the problem off the screenshot, verbatim. `temperature=0` for deterministic transcription. |
| Explanation | Google Gemini 3.8 Flash (`gemini-3.8-flash`) | Written explanation + storyboard |
| Code generation | Claude Sonnet 5 (`claude-sonnet-5`) | Manim source and repair |
| Embeddings | Voyage AI `voyage-code-3` (1024 dims) | Embeds snippet corpus and scene queries |
| Database | MongoDB Atlas (M0 free tier) | `jobs`, `cache` (coordinator); `manim_snippets` — the Manim Generator's **vector store**, via `langchain_mongodb.MongoDBAtlasVectorSearch` over the `snippets_vector` index |
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

Database: `clarity`. Connection string in `MONGODB_URI`.

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
  video_url:    "http://localhost:9000/clarity-renders/renders/a3f9c1d2e4b57680.mp4",
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

# 10. Agent Service (internal, Python, `:8000`)

> Full endpoint reference — errors, limits, timeouts, corpus-management endpoints, curl examples — is in [API.md](API.md) §3. The shapes below are the summary.

The agent service is where every model call happens, and it is built as **agents**: LangGraph graphs whose nodes call models, tools, and a vector store, and whose edges decide what happens next based on what came back. The coordinator (Go) still owns the *job* — creating it, tracking status, fanning scenes out, stitching and uploading — but the *thinking* loops live here.

### 10.1 Architecture

| Layer | Library | Used for |
|---|---|---|
| Orchestration | **LangGraph** (`langgraph`) | Each agent is a `StateGraph`: typed state, nodes as functions, conditional edges, bounded retry loops |
| Prompt infrastructure | **LangChain** (`langchain-core`) | `ChatPromptTemplate`s loaded from `prompts/*.md`, `.with_structured_output(PydanticModel)` on every model call, `Runnable` composition for the single-step chains |
| Models | `langchain-google-genai` · `langchain-anthropic` | `ChatGoogleGenerativeAI(model="gemini-3.8-flash")` · `ChatAnthropic(model="claude-sonnet-5")` |
| Vector store | `langchain-mongodb` · `langchain-voyageai` | `MongoDBAtlasVectorSearch` over `manim_snippets` with `VoyageAIEmbeddings(model="voyage-code-3")`, exposed as a **retriever** |
| Data shapes | **Pydantic v2** | Every request, response, graph state, and node output is a `BaseModel`. Nothing is a bare `dict`. |
| HTTP | FastAPI | One route per graph; the route builds the initial state, runs the graph, returns the final state's output model |
| Observability | LangSmith (optional) | `LANGSMITH_TRACING=true` traces every node; `thread_id` = `job_id` (or `job_id/scene_index`) so a job's graphs group together |

Three graphs, one per endpoint:

| Graph | Endpoint | Kind | Models | Loop |
|---|---|---|---|---|
| **Intake** | `POST /vision` | Single-step chain (`prompt \| model.with_structured_output`) | Gemini 3.8 Flash, `temperature=0` | none |
| **Explainer** | `POST /explain` | Agent: draft → critique → (revise → critique)? | Gemini 3.8 Flash | ≤ 1 revision |
| **Manim Generator** | `POST /scenes/render` | Agent: retrieve → generate → lint → render → (repair)… → ingest | Sonnet 5 generates; Gemini Flash never touches code | ≤ 3 render attempts, ≤ 2 lint retries each |

Principles that hold for every graph:

- **State is a Pydantic model**, declared in `agent/app/graphs/<name>/state.py`. Nodes take the state and return a partial update. No `TypedDict`, no `dict`.
- **Every model call uses `.with_structured_output(SomeModel)`.** The prompt never asks for JSON in prose; the schema enforces it. Validation failure is a node error, handled by the graph, never a 500.
- **Prompts are files.** `agent/app/prompts/*.md` with `{placeholders}`, loaded once into `ChatPromptTemplate`s. No prompt text in Python.
- **Graphs are stateless across requests.** No checkpointer. The job's durable state is the coordinator's record in Atlas. A graph runs to completion inside one HTTP request.
- **Tools are explicit nodes**, not model-chosen function calls. The render step is a node that calls the coordinator's internal render endpoint. The model writes code; the graph decides to run it.
- **Bounded loops.** Every conditional edge that can loop has a counter in state and a hard cap. A graph can never spin.

### 10.2 `POST /vision` — Intake chain

```json
// request
{ "image_b64": "iVBORw0KG...", "media_type": "image/png", "guardrails": false }

// response
{ "problem_text": "def binary_search(arr, target): ...", "category": "algorithm", "confidence": 0.91 }
```

One `Runnable`: `vision_prompt | gemini_flash.with_structured_output(VisionResponse)`, image passed as an inline `Part`. `temperature=0`, thinking `low`. **`problem_text` is verbatim** — this string is hashed by the coordinator; the prompt contains no instruction to interpret or summarize. Nothing problem-like → `category: "unknown"`, `problem_text: ""`.

### 10.3 `POST /explain` — Explainer agent

```json
// request
{ "problem_text": "…", "category": "algorithm", "user_prompt": "why is my binary search not working?", "guardrails": false }

// response
{
  "explanation": "**Step 1.** …",
  "storyboard": { "title": "Where the Binary Search Goes Wrong", "scenes": [
    { "index": 0, "narration": "lo and hi start at the ends.", "visual": "A row of 8 boxes; pointers lo and hi under the first and last.", "duration_seconds": 8 },
    { "index": 1, "narration": "mid rounds down — and hi never moves past it.", "visual": "mid pointer appears; hi jumps to mid instead of mid-1; the box checked twice is highlighted.", "duration_seconds": 10 }
  ] },
  "revisions": 1
}
```

```
draft ──► critique ──► pass? ──► END
                        │ fail, revisions < 1
                        ▼
                      revise ──► critique
                        (fail, revisions ≥ 1 → END with the best draft, flagged in logs)
```

| Node | Model | Does |
|---|---|---|
| `draft` | Gemini Flash → `ExplainDraft` | Explanation + 2–5 scene storyboard from the category-specific prompt (`explain_math.md` / `explain_algorithm.md`, + `explain_guardrails.md` when `guardrails`). |
| `critique` | Gemini Flash → `Critique` | Checks the rules as a rubric: 2–5 scenes · `narration` ≤ 90 chars · every scene shows something text can't (motion, a plot, a pointer, a transform — **not** restated algebra) · relative positioning language only · if `guardrails`, the final answer is genuinely absent. Returns `passed: bool` and a list of `issues`. Hard rules (counts, lengths) are also checked in Python before the model is asked. |
| `revise` | Gemini Flash → `ExplainDraft` | Re-drafts with the issues appended. Increments `revisions`. |

### 10.4 `POST /scenes/render` — Manim Generator agent

One call per scene. The coordinator fans these out concurrently, bounded by its render semaphore. The agent owns everything from "here is a scene" to "here is a finished clip or a final failure" — retrieval, generation, linting, rendering through the coordinator's internal endpoint, repair, and ingesting the result back into the corpus.

```json
// request
{ "job_id": "j_7f3a9c21", "scene": { "index": 1, "narration": "…", "visual": "…" }, "category": "algorithm", "guardrails": false, "work_dir": "/tmp/clarity/j_7f3a9c21/scene1", "quality": "-ql" }

// response — success
{ "ok": true, "clip_path": "/tmp/clarity/j_7f3a9c21/scene1/scene1.mp4", "attempts": 2, "snippets_used": ["66f1…", "66f2…"], "snippet_id": "66f9…" }

// response — gave up
{ "ok": false, "attempts": 3, "stage": "render", "last_traceback": "NameError: name 'RIGHT_ARROW' is not defined …" }
```

```
retrieve ──► generate ──► lint ──► ok? ──► render ──► ok? ──► ingest ──► END
                ▲           │ fail            │ fail
                │           │ (lint_retries<2)│ (attempts<3)
                └───────────┘                 │
                ▲                             │
                └──── retrieve (hint = traceback line 1) ◄──┘
                                              │ attempts ≥ 3 → END (ok: false)
```

| Node | Model / tool | Does |
|---|---|---|
| `retrieve` | `MongoDBAtlasVectorSearch.as_retriever(k=3, pre_filter={verified: true, category ∈ {cat, "general"}})` | Query = `narration + " " + visual` (+ `" " + hint` on repair). Stores `snippets` in state. Empty result is fine. |
| `generate` | **Claude Sonnet 5** → `ManimSource` | `codegen.md` on first attempt; `repair.md` (with `previous_source` + last 40 traceback lines) when `traceback` is set. Snippets pasted verbatim under "Reference — imitate these." Asserts `class GeneratedScene(Scene)` is present — otherwise it's a lint failure. |
| `lint` | Python, no model | `ast.parse`; imports only `manim`/`numpy`/stdlib; no `os.system`, `os.popen`, `subprocess`, `eval`, `exec`, `__import__`, `open(` for writing, `shutil.rmtree`; literal coordinates (`np.array([`, `.move_to([`) flagged. Failure → back to `generate` with the lint message as `traceback`, up to 2 times per render attempt. |
| `render` | Tool: `POST <COORDINATOR_URL>/internal/render` (§14.1) | Coordinator runs the source in `manim-worker` (its own pre-check runs again — defense in depth), returns `clip_path` or `{stage, traceback}`. Increments `attempts`. |
| `ingest` | Atlas upsert | On success: `{title: storyboard title + " — scene " + i, description: visual, category, tags: [], source, origin: "generated", verified: false}` via the same code path as `/snippets/ingest`. Failure is logged, never fatal. |

### 10.5 Snippet corpus endpoints

Unchanged from the corpus-management design: `POST /snippets/ingest`, `GET /snippets`, `GET /snippets/{id}`, `PATCH /snippets/{id}`, `DELETE /snippets/{id}` (API.md §3.6–3.10). All go through the same `MongoDBAtlasVectorSearch` instance the retriever uses, so index and query embeddings can never drift.

Two **debug endpoints** expose single nodes so they can be tested without running a whole graph: `POST /snippets/search` (the `retrieve` node) and `POST /codegen` (the `generate` node). The coordinator never calls them.

### 10.6 `GET /healthz`

`{ "ok": true, "gemini": true, "anthropic": true, "voyage": true, "atlas": true, "coordinator": true, "snippets_verified": 42, "models": {...}, "graphs": ["intake", "explainer", "manim_generator"], "version": "0.1.0" }`

### 10.7 Models and cost policy

| Where | Model | Settings |
|---|---|---|
| Intake `/vision` | **Gemini 3.8 Flash** | `temperature=0`, thinking `low` |
| Explainer `draft`, `revise`, `critique` | **Gemini 3.8 Flash** | default temperature, thinking `medium` (`critique`: `low`) |
| Manim Generator `generate` | **Claude Sonnet 5** | streaming, structured output |
| Embeddings | Voyage `voyage-code-3` | 1024 dims |

**No Opus-tier models anywhere.** Gemini Flash is the default for everything; Sonnet is used only in the `generate` node, where code quality measurably reduces render attempts. If the Sprint 3 ablation shows Flash matches Sonnet on first-render success, `generate` moves to Flash and the Anthropic dependency goes away.

Claude rules: `.with_structured_output()` always; never assistant prefill; never `temperature`. Gemini rules: `.with_structured_output()` always; `temperature=0` on Intake only.

### 10.8 The retriever is the vector-store mechanism

The Manim Generator's memory of "what working Manim looks like" is the `manim_snippets` collection in MongoDB Atlas, indexed by `snippets_vector` (§9.3) and wrapped by `langchain_mongodb.MongoDBAtlasVectorSearch`. Seeding, retrieval, and post-render ingest all go through that one object. §13 has the corpus design; the point here is that it is a first-class part of the agent, not a side lookup — every generation is grounded in it, and every success feeds it.

---

# 11. Coordinator API (public, Go, `:8080`)

> Full endpoint reference — including `DELETE /api/jobs/{id}` (cancel), the SSE stream, the error envelope, and limits — is in [API.md](API.md) §2. The shapes below are the summary.

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
All three arrows go through one `langchain_mongodb.MongoDBAtlasVectorSearch` instance (FRD §10.8) so index and query embeddings can never drift.
```text
seed .py files ──► scripts/seed_snippets.py ──► store.add_documents (voyage-code-3) ──► manim_snippets
scene {narration, visual} ──► retriever k=3, pre_filter verified=true ──► Manim Generator `retrieve` node ──► `generate` prompt
`render` node succeeded ──► `ingest` node: store.add_documents (verified=false) ──► PATCH verified=true after human review
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
After `/explain` returns N scenes, the coordinator creates a per-scene `work_dir` and starts N goroutines, each calling the agent's **`POST /scenes/render`** (§10.4). The agent owns the loop — retrieve, generate, lint, render, repair up to 3 times — and renders by calling back into the coordinator's **`POST /internal/render`**, which is where the semaphore (`RENDER_CONCURRENCY`) and P2's `Render()` live. The coordinator waits for all N, then concatenates whatever came back `ok: true`.

```go
// P2 implements; the /internal/render handler calls it.
func Render(ctx context.Context, src string, workDir string, quality string) (clipPath string, err *RenderError)
type RenderError struct { Stage string /* "precheck"|"container"|"timeout" */; Traceback string }
// P2 implements; the coordinator calls it once per job after fan-out.
func Concat(ctx context.Context, clipPaths []string, outKey string) (videoURL string, err error)
```

**`POST /internal/render`** (coordinator, localhost only, called by the agent's `render` node — API.md §2.7): `{source, work_dir, quality}` → `{ok: true, clip_path}` or `{ok: false, stage, traceback}`. Acquires the render semaphore, runs the static pre-check, then `Render()`. Never renders without the semaphore.

A scene whose agent call returns `ok: false` is **dropped, not the job**. Zero surviving scenes → `done` with `video_url: null`.

### 14.2 Static pre-check (before every `docker run`)
Runs inside `/internal/render` on every call — the agent's `lint` node already ran a similar check, but this one is the security gate and it runs regardless. Parses the full AST (`ast.parse` over stdin, walked for `Import`/`ImportFrom`/`Call` nodes — not a per-line regex, so a banned import chained after a `;` or split across a call chain doesn't slip through); rejects any import not `manim`/`numpy`/stdlib; rejects `os.system`, `os.popen`, `subprocess`, `open(` for writing (any mode containing `w`, `a`, or `x`), `__import__`, `eval`, `exec`, `shutil.rmtree`; requires `class GeneratedScene(Scene)`. Cheap, and turns most failures into a fast traceback instead of a slow container.

`numpy` is allowed alongside `manim` and stdlib (amended from "manim/stdlib only") because it ships as a manim dependency and nearly every LLM-written Manim scene imports it directly — banning it wasted the agent's repair budget on a safe, ubiquitous import for no security benefit. The `lint` node's separate literal-coordinate heuristic (`np.array([`, `.move_to([`) still flags hardcoded positions regardless of the import being allowed.

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
Done by the agent's `ingest` node (§10.4), not the coordinator — the agent has the source, the scene, and the render result in its state. The coordinator only reads `snippet_id` from the response for logging.

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
| Spotlight box | `webview.create_window(frameless=True, transparent=True, vibrancy=True, on_top=True, easy_drag=False)`, ~680×96, centered on the display the cursor is on, loads `ui/spotlight/index.html`. **Animates in**: opacity 0→1 and scale 0.96→1 over 180 ms `ease-out`, CSS only. Input focused on open. **Enter** submits (animates out 120 ms, then closes), **Esc** cancels. Thumbnail of the capture on the left. |
| Recents in the box | With an empty field, **↓** or `/` expands the box downward into a list (max 8 visible, scroll for 50): thumbnail · problem text or "Untitled capture" · relative time · a dot if a video exists. **Enter** on a recent with a result → open its result box. **Enter** on a recent without one, or **Tab** on any recent → load that screenshot into the box for a new question. Typing with the list open filters by problem text. |
| Submit | `POST /api/jobs` with `source: "desktop"` and the current guardrails setting. Write the recent immediately (before the response) so a failed submit is still visible in recents. On connection failure: notification "Can't reach the server", spotlight box stays open. |
| Result box | `webview.create_window(frameless=True, transparent=True, vibrancy=True, on_top=True, easy_drag=True)`, ~440×680, opens where the spotlight box was, loads `ui/result/index.html?job=&server=`. Polls every 1 s. **Draggable anywhere**: `easy_drag=True` makes the whole window a drag handle; `<video>`, links, and selectable text opt out with `class="pywebview-drag-region"` *not* applied. **X** top-right → `window.close()`; **Esc** does the same. Each job opens its own box, offset 24 px from the last so several can stay open. Fade-in 150 ms. |
| Result content | Status line in plain words ("Reading the problem…", "Writing explanation…", "Rendering scene 2 of 3…", "Done"). Explanation rendered from markdown (`marked.min.js`, bundled — no CDN). `<video controls autoplay muted>` when `video_url` arrives. `done` with null video → explanation + quiet note. 180 s timeout → message + retry. Every poll that adds `problem_text`, `explanation`, or `video_url` updates the local recent. |
| Reopen from recents | If the local recent has `explanation`/`video_url`, render from local data first, then poll `GET /api/jobs/{id}` once; a `404` is fine — the local copy is the source. |
| Notification | macOS notification when the explanation lands, so the user doesn't have to watch the window. |
| Config | `SERVER_URL` from `~/Library/Application Support/Clarity/config.json`, default `http://localhost:8080`, editable via the **Server…** menu item. |

### 15.2 Build and installer
| Artifact | How | Priority |
|---|---|---|
| `Clarity.app` | `pyinstaller --windowed --name Clarity --icon assets/icon.icns desktop/clarity/main.py`, with `--hidden-import` for `rumps`, `pynput.keyboard._darwin`, `webview`. `LSUIElement` added to `Info.plist` post-build. `ui/` bundled via `--add-data`. | Must |
| Stable identity | Sign with a **free self-signed certificate** (`codesign -s "Clarity Dev"`) so macOS keys the Screen Recording permission to a stable identity and it survives rebuilds. | Must |
| `Clarity.dmg` | `create-dmg` with drag-to-Applications layout and background. | Must |
| `Clarity.pkg` | `pkgbuild` + `productbuild` in `release/build_pkg.sh`; post-install script writes `~/Library/LaunchAgents/com.clarity.app.plist` so it starts at login. Built by P3 from P4's `.app`. | Should |
| Release | `release/publish.sh` attaches the `.dmg` and `.pkg` to a GitHub Release with `release/RELEASE_NOTES.md` (incl. the "Open Anyway" step for unsigned apps on macOS 15). P3. | Must |
| Notarization | Deferred. Requires Apple Developer Program. Everything above works without it; the user does one "Open Anyway" per install. | Out (for now) |

---

# 16. Functional Requirements — End-to-End Flow

## 16.1 Install
1. User downloads `Clarity.dmg`, drags to Applications, opens.
2. macOS 15 blocks the unsigned app → System Settings → Privacy & Security → Open Anyway.
3. App requests Screen Recording; user grants; app asks to restart. App requests Input Monitoring on first hotkey registration; same.
4. Menu bar icon appears. `⌘⇧E` is live.

## 16.2 Capture
1. `⌘⇧E` → crosshair. User **drags a region** around the problem. Esc cancels.
2. Spotlight box **animates in** with the thumbnail. User types context (optional) — or ↓ to pick a recent — and presses **Enter**.
3. Spotlight box animates out. Result box opens: "Reading the problem…".

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
5. User **drags the box** off the problem if it's in the way, watches, and clicks **X** when done.

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
| F46 | The spotlight box animates in (fade + scale, ~180 ms) and out on submit. | Should |
| F47 | The result box is draggable anywhere by its background so it can be moved off the problem. | Must |
| F48 | The result box has an X (and Esc) that closes it. Several result boxes can be open at once, offset. | Must |

### Installer
| # | Requirement | Priority |
|---|---|---|
| F12 | `Clarity.app` builds reproducibly from `scripts/build_app.sh`. | Must |
| F13 | The app is signed with a stable (self-signed) identity so permissions survive rebuilds. | Must |
| F14 | `Clarity.dmg` is produced by `scripts/build_dmg.sh` and attached to a GitHub Release. | Must |
| F15 | `Clarity.pkg` installs a LaunchAgent for start-at-login. | Should |
| F16 | README documents the macOS 15 "Open Anyway" step. | Must |

### Agent service
| # | Requirement | Priority |
|---|---|---|
| F17 | `/vision` returns verbatim problem text, category, confidence via structured outputs. | Must |
| F18 | `/explain` returns markdown explanation and a 2–5 scene storyboard. | Must |
| F19 | `/scenes/render` is a LangGraph agent: retrieve (Atlas vector store) → generate (Sonnet) → lint → render (`/internal/render`) → repair ≤ 3 → ingest. | Must |
| F20 | The `generate` node returns runnable Manim CE source with `class GeneratedScene`, grounded in the retrieved snippets. | Must |
| F21 | The repair path re-retrieves with the traceback as a hint and passes previous source + traceback to `generate`. | Must |
| F21b | `/explain` is a LangGraph agent with a critique node and at most one revision. | Should |
| F21c | Every graph state and node I/O is a Pydantic model; every model call uses `.with_structured_output`. | Must |
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
| F31 | Scenes fan out concurrently as `/scenes/render` calls; `/internal/render` is bounded by `RENDER_CONCURRENCY`. | Must |
| F32 | A scene whose agent call returns `ok: false` is dropped; the job completes with the rest. | Must |
| F33 | `POST /internal/render` exists, is localhost-only, acquires the semaphore and runs the pre-check before `Render()`. | Must |
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
| N6 | Full stack runs on one laptop; no cloud dependency except Gemini, Claude, Voyage, and Atlas. |
| N7 | One command per service to start (§SETUP). |
| N8 | Desktop app cold start < 2 s to menu bar icon. |

---

# 19. Error Handling and Fallbacks

| Failure | Behavior |
|---|---|
| `/vision` → `unknown` | Job `failed`, `error: "no_problem_found"`. Panel: "No problem found in that capture." |
| `/explain` error | Job `failed`, `error` set. Panel: plain error + retry. |
| `retrieve` node error (Atlas) | Agent logs, proceeds with empty snippets. Retrieval is an improvement, not a dependency. |
| `generate` node error / schema violation | Agent counts a lint retry; after 2, counts a render attempt and re-retrieves. |
| `lint` reject | Back to `generate` with the reason as traceback, ≤ 2 per attempt. |
| `/internal/render` pre-check reject or container failure | Agent counts a failed attempt; retrieves with the traceback's first line as hint; repairs. |
| Container timeout | `docker kill`; `/internal/render` returns `stage: "timeout"`; failed attempt. |
| Agent returns `ok: false` (3 attempts) | Coordinator drops the scene. Job continues. |
| Agent unreachable / `/scenes/render` 5xx | Coordinator treats the scene as dropped; job continues. |
| Zero scenes survive | `done`, `video_url: null`. Cache **not** written. |
| Concat or upload error | `done`, `video_url: null`, `error` set. Cache not written. |
| `ingest` node error | Agent logs, returns `snippet_id: null`. Never fatal. |
| Atlas unreachable | Coordinator refuses new jobs with `503`; `/healthz` reports `atlas: false`. |
| Desktop: server unreachable | Notification. Spotlight box stays open for retry. |
| Desktop: poll 404 on a live job | Result box: "This job expired." |
| Desktop: 404 when reopening a recent | Render from the local copy. If none, offer to re-ask. |
| Desktop: 180 s without `done` | Result box: message + retry. |
| Desktop: recent's screenshot file missing | Show the thumbnail, disable re-ask for that entry. |

---

# 20. Security and Credential Handling

- `GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, `VOYAGE_API_KEY`, `MONGODB_URI`, S3 credentials live in `agent/.env` and `server/.env`. Never committed. Never in the desktop app.
- The desktop app holds no secrets. It talks only to the coordinator.
- Generated code runs only inside `--network none` containers, after a static pre-check. Never on the host.
- Server side, screenshots are processed and discarded. The coordinator logs the hash, never the image. Images are not written to Atlas or S3.
- Client side, screenshots are kept **only on the user's machine** in `~/Library/Application Support/Clarity/recents/` for the recents feature. Rolling 50. The menu bar has **Clear Recents**. Nothing in recents is ever uploaded except when the user explicitly re-asks.
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
| `GEMINI_API_KEY` | `AIza…` | Gemini — `/vision`, `/explain` |
| `ANTHROPIC_API_KEY` | `sk-ant-…` | Claude — `/codegen` (Sonnet 5) |
| `VOYAGE_API_KEY` | `pa-…` | Embeddings |
| `MONGODB_URI` | `mongodb+srv://…/clarity` | Atlas |
| `MONGODB_DB` | `clarity` | Database name |
| `EMBED_MODEL` | `voyage-code-3` | Must match the vector index dims |
| `VISION_MODEL` | `gemini-3.8-flash` | |
| `EXPLAIN_MODEL` | `gemini-3.8-flash` | |
| `CODEGEN_MODEL` | `claude-sonnet-5` | |
| `COORDINATOR_URL` | `http://localhost:8080` | The agent's render tool calls `/internal/render` here |
| `LANGSMITH_TRACING` / `LANGSMITH_API_KEY` | unset | Optional graph tracing |

### `server/.env`
| Var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | |
| `AGENT_URL` | `http://localhost:8000` | |
| `MONGODB_URI` | — | Atlas |
| `MONGODB_DB` | `clarity` | |
| `S3_ENDPOINT` | `http://localhost:9000` | MinIO locally |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `minioadmin` / `minioadmin` | |
| `RENDER_BUCKET` | `clarity-renders` | |
| `RENDER_CONCURRENCY` | `NumCPU/2` | |
| `RENDER_TIMEOUT_SEC` | `120` | |
| `MANIM_QUALITY` | `-ql` | `-qm` for release |

### Desktop (`~/Library/Application Support/Clarity/config.json`)
| Key | Default |
|---|---|
| `server_url` | `http://localhost:8080` |
| `guardrails` | `false` |
| `hotkey` | `<cmd>+<shift>+e` |

---

# 23. Key Implementation Rules

### Agent service
1. Every model call goes through LangChain `.with_structured_output(PydanticModel)`. Never parse prose for JSON. Never assistant prefill. Never `temperature` on a Claude model.
2. The `/vision` prompt contains no instruction to interpret, summarize, or contextualize. Verbatim only.
3. Graph state, node inputs and outputs, requests, responses are all Pydantic models. No `dict`, no `TypedDict`.
4. Retrieval filters on `verified: true` in every code path. No debug flag disables it.
5. One `MongoDBAtlasVectorSearch` instance for seed, retrieve, and ingest; index and query use the same `EMBED_MODEL`.
6. Every loop in a graph has a counter in state and a hard cap (Explainer ≤ 1 revision; Manim Generator ≤ 3 render attempts, ≤ 2 lint retries each). Gemini Flash for Intake and Explainer; Sonnet 5 only in `generate`. No Opus-tier models.

### Coordinator
7. `POST /api/jobs` writes the job and returns. Everything else is in a goroutine with `defer recover()`.
8. `explanation` is written to `jobs` before any `/scenes/render` call.
9. Every `jobs` write sets `updated_at = now`.
10. `cache` is written only after upload succeeds. Never on any failure path.
11. Cache hit path never calls `/explain`.
12. The manim quality flag is one job-level value passed to every scene.
13. `PromptVersion` is bumped in the same commit as any prompt change, in any component.

### Render
14. Static pre-check runs inside `/internal/render` before every `docker run`, including the agent's repair attempts.
15. `docker run` always has `--network none`, `--memory`, `--cpus`, and a context timeout. `/internal/render` never runs without the semaphore.
16. A scene failure never propagates to sibling scenes — an `ok: false` from the agent drops that scene only.

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
| Does Gemini at `temperature=0` transcribe the same problem identically across zoom/crop? Run the six-capture experiment. | P1 | End of Sprint 1 |
| Does retrieval measurably reduce repair attempts? Compare 10 scenes with and without snippets. | P1 | End of Sprint 3 |
| Does guardrails mode actually withhold the answer on real problems? | P1 | End of Sprint 4 |
| Is `voyage-code-3` the right embedding model for prose→prose matching, or should it be `voyage-3.5`? | P1 | End of Sprint 2 (before the corpus is large) |
| Does the self-signed cert keep Screen Recording permission stable across rebuilds in practice? | P4 | End of Sprint 4 |
| Does `pywebview` `transparent=True` + `vibrancy=True` render correctly inside a PyInstaller bundle, or does the spotlight box need a solid fallback? | P4 | End of Sprint 2 |

# Clarity — REST API Reference

**Version:** 1.0
**Status:** planning — no code yet

### How to read this

There are two servers. The **coordinator** (Go, port 8080) is what the desktop app talks to — think of it as the public API. The **agent service** (Python, port 8000) is internal: only the coordinator calls it, and it's the only thing that calls Claude. If you're on the desktop app you only care about §2. If you're on the agent service you only care about §3. §4–§7 (errors, lifecycle, sequence, limits) apply to both.

Two HTTP services. This document is the authoritative reference for every endpoint on both: method, path, who calls it, request, response, errors. `docs/FRD.md` §10–§11 summarize the same shapes in context; if the two ever disagree, fix the FRD to match this file.

| Service | Base URL (dev) | Owner | Called by |
|---|---|---|---|
| **Coordinator** (Go) | `http://localhost:8080` | P3 | Desktop app (P4) |
| **Agent service** (Python) | `http://localhost:8000` | P1 | Coordinator only — never the desktop app |

---

## Table of Contents

1. [Conventions](#1-conventions)
2. [Coordinator API](#2-coordinator-api)
3. [Agent Service API](#3-agent-service-api)
4. [Error Codes](#4-error-codes)
5. [Job Lifecycle](#5-job-lifecycle)
6. [Call Sequence for One Job](#6-call-sequence-for-one-job)
7. [Limits and Timeouts](#7-limits-and-timeouts)
8. [curl Cookbook](#8-curl-cookbook)
9. [Endpoint Index](#9-endpoint-index)

---

## 1. Conventions

| Rule | Detail |
|---|---|
| Format | JSON in, JSON out. `Content-Type: application/json; charset=utf-8` on every request and response body. |
| Versioning | None in the path. This is v1. A breaking change moves to `/api/v2/…` and the old path keeps working for one release. |
| IDs | Job IDs are `j_` + 8 hex chars (`j_7f3a9c21`). Snippet IDs are Mongo ObjectId hex strings. Problem hashes are 16 hex chars. |
| Timestamps | RFC 3339 UTC, e.g. `2026-09-12T02:14:03Z`. |
| Errors | One envelope everywhere (§4). Never a bare string, never HTML. |
| Request ID | Every response carries `X-Request-Id`. Send one and it is echoed; omit it and one is generated. Logged on both services. |
| CORS | Coordinator: `Access-Control-Allow-Origin: *` on every response including errors. Agent: no CORS — it is not reachable from a browser context. |
| Auth | **None.** Both services bind to `127.0.0.1` by default. Exposing the coordinator beyond the machine requires a bearer token and rate limiting first — not in scope. |
| Booleans | `guardrails` is a real JSON boolean, never `"true"`. |
| Nulls | Fields that are not yet known are present and `null`, never omitted. Clients can rely on key presence. |
| Unknown fields | Ignored on input. Never emitted on output. |

---

## 2. Coordinator API

Public. The desktop app talks only to this.

### 2.1 `POST /api/jobs` — create a job

Accepts a screenshot and optional context, returns immediately with a job ID. All work happens after the response.

**Request**
```json
{
  "image": "data:image/png;base64,iVBORw0KG...",
  "user_prompt": "why is my binary search not working? visualize where it's messing up",
  "guardrails": false,
  "source": "desktop"
}
```

| Field | Type | Required | Rule |
|---|---|---|---|
| `image` | string | yes | PNG or JPEG data URL. ≤ 8 MB decoded. |
| `user_prompt` | string | no | Default `""`. Part of the cache key. ≤ 2000 chars. |
| `guardrails` | boolean | no | Default `false`. Part of the cache key. |
| `source` | string | no | `"desktop"` (default) \| `"cli"` \| `"test"`. Telemetry only. |

**Response `202 Accepted`**
```json
{ "job_id": "j_7f3a9c21", "poll_url": "/api/jobs/j_7f3a9c21" }
```

**Errors:** `400 bad_request` (malformed JSON, missing `image`, not a data URL) · `413 image_too_large` · `415 unsupported_image_type` · `503 queue_full` (retry after `Retry-After` seconds) · `503 dependency_down` (Atlas or agent unreachable at boot).

### 2.2 `GET /api/jobs/{job_id}` — poll a job

The only endpoint the desktop app polls. Every 1 s. Stop at `done`, `failed`, or `cancelled`, or after 180 s.

**Response `200`**
```json
{
  "job_id": "j_7f3a9c21",
  "status": "rendering",
  "problem_hash": "a3f9c1d2e4b57680",
  "problem_text": "def binary_search(arr, target): ...",
  "category": "algorithm",
  "explanation": "**Step 1.** The loop condition ...",
  "scenes_total": 3,
  "scenes_done": 2,
  "video_url": null,
  "cached": false,
  "guardrails": false,
  "error": null,
  "created_at": "2026-09-12T02:13:51Z",
  "updated_at": "2026-09-12T02:14:03Z"
}
```

| Field | When set |
|---|---|
| `problem_hash`, `problem_text`, `category` | After `/vision`. `problem_text` lets the desktop app label the recent. |
| `explanation` | After `/explain`. **Render it the moment it is non-null**, whatever `status` says. Never cleared afterwards. |
| `scenes_total` | After `/explain`. |
| `scenes_done` | Increments as scenes finish or are dropped. |
| `video_url` | At `done`, or `null` at `done` if zero scenes survived. Direct S3/MinIO URL. |
| `cached` | `true` when the pipeline was skipped on a cache hit. |
| `error` | Error envelope's `code` string on `failed`; otherwise `null`. |

Status values and their guarantees are in §5.

**Errors:** `404 job_not_found` (unknown ID, or expired after 24 h).

### 2.3 `DELETE /api/jobs/{job_id}` — cancel a job

Sent by the desktop app when the user closes the result box before `done`. Stops pending work: queued scenes are skipped, running containers are killed. Nothing is written to the cache.

**Response `200`**
```json
{ "job_id": "j_7f3a9c21", "status": "cancelled" }
```

Idempotent — cancelling a finished or already-cancelled job returns `200` with the current status.

**Errors:** `404 job_not_found`.

### 2.4 `GET /api/jobs/{job_id}/events` — stream a job (stretch)

Server-Sent Events alternative to polling. Emits one `event: status` per state change with the same body as §2.2, then `event: end`. The desktop app may use this instead of polling if implemented; polling stays supported.

```
event: status
data: {"job_id":"j_7f3a9c21","status":"explaining", ...}

event: status
data: {"job_id":"j_7f3a9c21","status":"generating","explanation":"**Step 1.** ...", ...}

event: end
data: {}
```

`Content-Type: text/event-stream`. Heartbeat comment `: ping` every 15 s.

### 2.5 `GET /api/cache/{problem_hash}` — inspect a cache entry

Debugging and pre-warming checks. Not called by the desktop app.

**Response `200`**
```json
{ "problem_hash": "a3f9c1d2e4b57680", "video_url": "http://…/renders/a3f9c1d2e4b57680.mp4", "explanation": "…", "created_at": "…" }
```

**Errors:** `404 cache_miss`.

### 2.6 `GET /healthz` — liveness and dependency health

```json
{ "ok": true, "atlas": true, "docker": true, "s3": true, "agent": true, "version": "0.1.0", "prompt_version": "2026-09-12a" }
```

`200` if `ok`, `503` otherwise. `docker`, `s3`, `agent` are checked at boot and every 60 s, not per request.

### 2.7 `POST /internal/render` — render one source file (internal; called by the agent)

The agent's render tool. Bound to `127.0.0.1` and refused from any other address. Acquires the render semaphore, runs the static pre-check (FRD §14.2), then P2's `Render()`.

**Request**
```json
{ "source": "from manim import *

class GeneratedScene(Scene):
    …", "work_dir": "/tmp/clarity/j_7f3a9c21/scene1", "quality": "-ql" }
```

**Response `200`**
```json
{ "ok": true, "clip_path": "/tmp/clarity/j_7f3a9c21/scene1/scene1.mp4", "duration_seconds": 9.4 }
```
```json
{ "ok": false, "stage": "precheck", "traceback": "banned import: subprocess" }
{ "ok": false, "stage": "container", "traceback": "…last 40 lines of manim stderr…" }
{ "ok": false, "stage": "timeout", "traceback": "render exceeded 120s" }
```
A failed render is a `200` with `ok: false` — that's data for the agent's repair loop, not an error. Non-2xx only for infrastructure: `503 dependency_down` (Docker not running), `429 render_busy` if the semaphore wait exceeds 60 s (agent retries after `Retry-After`).


---

## 3. Agent Service API

Internal. Only the coordinator calls it. Every endpoint runs a **LangGraph** graph (or a single LangChain runnable) and returns a Pydantic-validated JSON body — no prose, no fences. Models: Intake and Explainer → Gemini 3.8 Flash; Manim Generator's `generate` node → Claude Sonnet 5. Every request carries `guardrails`. Architecture in FRD §10.

### 3.1 `POST /vision` — Intake chain: transcribe a screenshot

**Request**
```json
{ "image_b64": "iVBORw0KG...", "media_type": "image/png", "guardrails": false }
```
`image_b64` is raw base64 — the coordinator strips the data-URL prefix.

**Response `200`**
```json
{ "problem_text": "def binary_search(arr, target):
    lo, hi = 0, len(arr)
    ...", "category": "algorithm", "confidence": 0.91 }
```

| Field | Rule |
|---|---|
| `problem_text` | **Verbatim.** This string is hashed by the coordinator. |
| `category` | `"math"` \| `"algorithm"` \| `"unknown"`. |
| `confidence` | 0.0–1.0. |

`category: "unknown"` with `problem_text: ""` is a valid `200` — the coordinator turns it into `failed / no_problem_found`.

**Errors:** `400 bad_request` · `502 model_error` (`details.provider: "gemini"`, retryable) · `504 model_timeout`.

### 3.2 `POST /explain` — Explainer agent: explanation + storyboard

**Request**
```json
{ "problem_text": "…", "category": "algorithm", "user_prompt": "why is my binary search not working?", "guardrails": false }
```

**Response `200`**
```json
{
  "explanation": "**Step 1.** …",
  "storyboard": {
    "title": "Where the Binary Search Goes Wrong",
    "scenes": [
      { "index": 0, "narration": "lo and hi start at the ends.", "visual": "A row of 8 boxes; pointers lo and hi under the first and last.", "duration_seconds": 8 },
      { "index": 1, "narration": "mid rounds down — and hi never moves past it.", "visual": "mid pointer appears; hi jumps to mid instead of mid-1; highlight the box that gets checked twice.", "duration_seconds": 10 }
    ]
  },
  "revisions": 1
}
```

Graph: `draft → critique → (revise → critique)?`, at most one revision. `revisions` reports how many happened. Constraints enforced by the critique rubric and by Python: 2–5 scenes; `narration` ≤ 90 chars; `visual` in relative terms; `duration_seconds` 5–15; with `guardrails`, no final answer.

**Errors:** `400` · `422 schema_violation` (draft failed validation after the graph's own retry) · `502 model_error` · `504 model_timeout`.

### 3.3 `POST /scenes/render` — Manim Generator agent: one scene to one clip

The agent owns the whole loop for one scene: retrieve examples from the Atlas vector store → generate Manim source (Sonnet) → lint → render via the coordinator's `POST /internal/render` (§2.7) → repair on failure, up to 3 render attempts → ingest the successful source back into the corpus.

**Request**
```json
{
  "job_id": "j_7f3a9c21",
  "scene": { "index": 1, "narration": "…", "visual": "…" },
  "storyboard_title": "Where the Binary Search Goes Wrong",
  "category": "algorithm",
  "guardrails": false,
  "work_dir": "/tmp/clarity/j_7f3a9c21/scene1",
  "quality": "-ql"
}
```

| Field | Rule |
|---|---|
| `work_dir` | Per-scene directory the coordinator created. The agent passes it through to `/internal/render`; the clip lands there. |
| `quality` | The **job-level** manim quality flag. Passed through unchanged to every render attempt. |

**Response `200` — success**
```json
{ "ok": true, "clip_path": "/tmp/clarity/j_7f3a9c21/scene1/scene1.mp4", "attempts": 2, "lint_retries": 1, "snippets_used": ["66f1…", "66f2…", "66f3…"], "snippet_id": "66f9…" }
```
`snippet_id` is the newly ingested `origin: "generated", verified: false` snippet, or `null` if ingest failed (never fatal).

**Response `200` — gave up**
```json
{ "ok": false, "attempts": 3, "lint_retries": 4, "stage": "render", "last_traceback": "NameError: name 'RIGHT_ARROW' is not defined …", "snippets_used": ["…"] }
```
A `200` with `ok: false` is the normal "this scene didn't work out" result. The coordinator drops the scene and continues. Only infrastructure failures are non-2xx.

**Errors:** `400` · `502 model_error` · `502 coordinator_unreachable` (couldn't reach `/internal/render`) · `503 atlas_unavailable` · `504 model_timeout`.

**Timeout budget:** 3 attempts × (90 s generate + 120 s render) + retrieval ≈ 10.5 min worst case. The coordinator's client timeout for this call is **11 min**.

### 3.4 `POST /snippets/search` — debug: run the `retrieve` node alone

```json
// request
{ "scene": { "index": 1, "narration": "…", "visual": "…" }, "category": "algorithm", "k": 3, "hint": "NameError: name 'RIGHT_ARROW' is not defined" }
// response
{ "snippets": [ { "id": "66f1…", "title": "Array walk with a moving pointer", "description": "…", "source": "from manim import *
…", "score": 0.86 } ] }
```
Only `verified: true` snippets are ever returned. The coordinator never calls this; it exists so retrieval quality can be tested by hand.

### 3.5 `POST /codegen` — debug: run the `generate` node alone

```json
// request
{ "scene": {…}, "snippets": [ { "title": "…", "source": "…" } ], "previous_source": null, "traceback": null, "guardrails": false }
// response
{ "manim_source": "from manim import *

class GeneratedScene(Scene):
    …", "scene_class": "GeneratedScene" }
```
Same node the agent uses; no lint, no render. For prompt iteration and the retrieval ablation.

### 3.6 `POST /snippets/ingest` — add a snippet to the corpus

Called by the seed script (`verified: true, origin: "seed"`) and by the Manim Generator's `ingest` node (`verified: false, origin: "generated"`) — both through the same `MongoDBAtlasVectorSearch` instance.

```json
// request
{ "title": "…", "description": "…", "category": "algorithm", "tags": ["VGroup", "arrange", "Arrow"], "source": "from manim import *
…", "origin": "generated", "verified": false }
// response 201
{ "id": "66f1a2b3c4d5e6f7a8b9c0d1", "embedded": true }
```

**Errors:** `400` · `409 duplicate_snippet` (`details.id` is the existing one) · `502 embedding_error` · `503 atlas_unavailable`.

### 3.7 `GET /snippets` — list

`?verified=false&origin=generated&limit=50&cursor=…` → `{ "snippets": [ {id, title, category, origin, verified, created_at} ], "next_cursor": null }`. `source` and `embedding` omitted; fetch one for the source.

### 3.8 `GET /snippets/{id}` — fetch one

Full document minus `embedding`. **Errors:** `404 snippet_not_found`.

### 3.9 `PATCH /snippets/{id}` — promote, demote, or edit

`{ "verified": true }` or any subset of `title`, `description`, `category`, `tags`, `verified`. Changing text fields re-embeds. Returns the updated document minus `embedding`.

### 3.10 `DELETE /snippets/{id}` — remove

`204 No Content`.

### 3.11 `GET /healthz`

```json
{ "ok": true, "gemini": true, "anthropic": true, "voyage": true, "atlas": true, "coordinator": true,
  "snippets_verified": 42, "graphs": ["intake", "explainer", "manim_generator"],
  "models": { "vision": "gemini-3.8-flash", "explain": "gemini-3.8-flash", "codegen": "claude-sonnet-5", "embed": "voyage-code-3" },
  "version": "0.1.0" }
```
`coordinator` = can reach `<COORDINATOR_URL>/healthz` (needed for the render tool).

---

## 4. Error Codes

Every non-2xx response has this body:

```json
{ "error": { "code": "image_too_large", "message": "Image is 11.2 MB; the limit is 8 MB.", "details": { "limit_bytes": 8388608, "actual_bytes": 11744051 } }, "request_id": "req_01J…" }
```

| HTTP | `code` | Service | Meaning |
|---|---|---|---|
| 400 | `bad_request` | both | Malformed JSON, missing or mistyped field. `details.field` names it. |
| 404 | `job_not_found` | coordinator | Unknown or expired job. |
| 404 | `cache_miss` | coordinator | No cache entry for that hash. |
| 404 | `snippet_not_found` | agent | |
| 409 | `duplicate_snippet` | agent | Same `source` already ingested. `details.id` is the existing one. |
| 413 | `image_too_large` | coordinator | > 8 MB decoded. |
| 415 | `unsupported_image_type` | coordinator | Not `image/png` or `image/jpeg`. |
| 422 | `schema_violation` | agent | Model output failed schema validation after one retry. Coordinator treats as a failed attempt. |
| 502 | `model_error` | agent | Gemini or Anthropic returned an error. `details.provider` says which. Retryable. |
| 502 | `coordinator_unreachable` | agent | The render tool couldn't reach `/internal/render`. |
| 429 | `render_busy` | coordinator | `/internal/render` semaphore wait exceeded 60 s. `Retry-After` set. |
| 502 | `embedding_error` | agent | Voyage returned an error. Retryable. |
| 503 | `queue_full` | coordinator | Bounded job queue is full. `Retry-After` set. |
| 503 | `dependency_down` | coordinator | Atlas or agent unreachable. |
| 503 | `atlas_unavailable` | agent | |
| 504 | `model_timeout` | agent | Model call exceeded its timeout (§7). |
| 500 | `internal` | both | Anything else. Always has a `request_id` to grep for. |

Job-level failures surface as `status: "failed"` with `error` set to one of: `no_problem_found`, `explain_failed`, `cancelled_by_user`, `internal`. Render failures never fail the job — see §5.

---

## 5. Job Lifecycle

```
queued → transcribing → explaining → generating → rendering → concatenating → uploading → done
                │             │                                                          ↑
                └─ failed     └─ failed                              (zero scenes survived: done, video_url null)
any state ─── DELETE ───► cancelled
```

| Status | Set when | Guarantees at this point |
|---|---|---|
| `queued` | `POST /api/jobs` returned | Job exists in Atlas. |
| `transcribing` | `/vision` in flight | |
| `explaining` | `/vision` returned, cache missed | `problem_hash`, `problem_text`, `category` set. |
| `generating` | `/explain` returned | **`explanation` set. `scenes_total` set.** |
| `rendering` | first scene's container started | `scenes_done` advances. |
| `concatenating` | every scene finished or dropped | `scenes_done == scenes_total`. |
| `uploading` | concat succeeded | |
| `done` | upload succeeded — or zero scenes survived | `video_url` set, or `null` with the explanation intact. `cached: true` if the pipeline was skipped. |
| `failed` | `/vision` said `unknown`, or `/explain` failed | `error` set. `explanation` is `null`. |
| `cancelled` | `DELETE` | Nothing cached. |

The desktop app's contract: render `explanation` on the first non-null poll; treat `done` + `video_url: null` as a quiet note, not an error; stop polling on `done`, `failed`, `cancelled`, or 180 s.

---

## 6. Call Sequence for One Job

```
Desktop                Coordinator                    Agent                       Docker/S3
  │  POST /api/jobs        │                             │                            │
  │───────────────────────►│ 202 {job_id}                │                            │
  │◄───────────────────────│                             │                            │
  │  GET /api/jobs/{id} …  │  POST /vision               │                            │
  │  (every 1 s)           │────────────────────────────►│                            │
  │                        │◄──── {problem_text,…} ──────│                            │
  │                        │  hash → Atlas cache lookup  │                            │
  │                        │  (hit → done, cached:true)  │                            │
  │                        │  POST /explain              │                            │
  │                        │────────────────────────────►│                            │
  │                        │◄── {explanation,storyboard} │                            │
  │  ← explanation visible │  write jobs.explanation     │                            │
  │                        │  per scene, concurrently:   │                            │
  │                        │   POST /scenes/render ─────►│ Manim Generator graph:     │
  │                        │                             │  retrieve (Atlas vectors)  │
  │                        │                             │  generate (Sonnet) → lint  │
  │                        │◄── POST /internal/render ───│  render tool               │
  │                        │   semaphore → precheck → docker run ────────────────────►│
  │                        │─── {ok | traceback} ───────►│  fail? retrieve+repair ≤3  │
  │                        │◄── {ok, clip_path, …} ──────│  ingest → Atlas            │
  │                        │  ffmpeg concat → S3 upload ─────────────────────────────►│
  │                        │  write cache; jobs.done     │                            │
  │  ← video_url           │                             │                            │
```

---

## 7. Limits and Timeouts

| What | Value | Where enforced |
|---|---|---|
| Image size | 8 MB decoded | Coordinator, `413` |
| `user_prompt` length | 2000 chars | Coordinator, `400` |
| Job queue depth | 32 | Coordinator, `503 queue_full` |
| Concurrent renders | `RENDER_CONCURRENCY` (default `NumCPU/2`) | Coordinator semaphore |
| Coordinator → `/vision` | 30 s | Coordinator client |
| Coordinator → `/explain` | 90 s (draft + critique + possible revise) | Coordinator client |
| Coordinator → `/scenes/render` | 11 min (3 × (generate + render) + retrieval) | Coordinator client |
| Agent → `/internal/render` | 130 s per call | Agent render tool |
| Agent → Sonnet (`generate`) | 90 s, 1 retry on 5xx/429 | Agent |
| Agent → Gemini (`/vision`) | 20 s, 1 retry on 5xx/429 | Agent |
| Agent → Gemini (`/explain`) | 40 s, 1 retry on 5xx/429 | Agent |
| Agent → Anthropic (`/codegen`) | 60 s per call, 1 retry on 5xx/429 | Agent |
| Agent → Voyage | 10 s, 1 retry | Agent |
| Per-scene container | 120 s | Coordinator, `docker kill` |
| Render attempts per scene | 3 | Agent (Manim Generator graph) |
| Lint retries per render attempt | 2 | Agent |
| Explainer revisions | 1 | Agent (Explainer graph) |
| Desktop poll | 1 s interval, 180 s total | Desktop |
| Job record TTL | 24 h | Atlas TTL index |
| Cache entry TTL | 7 d | Atlas TTL index |
| Video on S3 | 14 d | Bucket lifecycle |

---

## 8. curl Cookbook

```sh
# Create a job from a PNG on disk
IMG="data:image/png;base64,$(base64 -i problem.png | tr -d '\n')"
curl -s localhost:8080/api/jobs -H 'Content-Type: application/json' \
  -d "{\"image\":\"$IMG\",\"user_prompt\":\"why is my binary search not working?\",\"guardrails\":false}"
# → {"job_id":"j_7f3a9c21","poll_url":"/api/jobs/j_7f3a9c21"}

# Poll
curl -s localhost:8080/api/jobs/j_7f3a9c21 | jq '{status, scenes_done, scenes_total, video_url}'

# Watch until done
watch -n1 'curl -s localhost:8080/api/jobs/j_7f3a9c21 | jq -r .status'

# Cancel
curl -s -X DELETE localhost:8080/api/jobs/j_7f3a9c21

# Check the cache for a hash
curl -s localhost:8080/api/cache/a3f9c1d2e4b57680

# Health, both services
curl -s localhost:8080/healthz; curl -s localhost:8000/healthz

# Agent directly (P1 dev loop)
curl -s localhost:8000/vision -H 'Content-Type: application/json' \
  -d "{\"image_b64\":\"$(base64 -i problem.png | tr -d '\n')\",\"media_type\":\"image/png\",\"guardrails\":false}"

curl -s localhost:8000/snippets/search -H 'Content-Type: application/json' \
  -d '{"scene":{"index":0,"narration":"lo and hi start at the ends.","visual":"A row of boxes with two pointers."},"category":"algorithm","k":3}'

# Corpus management
curl -s 'localhost:8000/snippets?verified=false&origin=generated'
curl -s -X PATCH localhost:8000/snippets/66f1a2b3c4d5e6f7a8b9c0d1 -H 'Content-Type: application/json' -d '{"verified":true}'
curl -s -X DELETE localhost:8000/snippets/66f1a2b3c4d5e6f7a8b9c0d1
```

---

## 9. Endpoint Index

| Method | Path | Service | Called by | Purpose | Priority |
|---|---|---|---|---|---|
| `POST` | `/api/jobs` | Coordinator | Desktop | Create job | Must |
| `GET` | `/api/jobs/{id}` | Coordinator | Desktop | Poll job | Must |
| `DELETE` | `/api/jobs/{id}` | Coordinator | Desktop | Cancel job | Should |
| `GET` | `/api/jobs/{id}/events` | Coordinator | Desktop | SSE stream | Could |
| `GET` | `/api/cache/{hash}` | Coordinator | Dev tools | Inspect cache | Could |
| `GET` | `/healthz` | Coordinator | Desktop, dev | Health | Must |
| `POST` | `/internal/render` | Coordinator | Agent (render tool) | Render one source in Docker | Must |
| `POST` | `/vision` | Agent | Coordinator | Intake chain — transcribe | Must |
| `POST` | `/explain` | Agent | Coordinator | Explainer agent — explanation + storyboard | Must |
| `POST` | `/scenes/render` | Agent | Coordinator | Manim Generator agent — one scene to one clip | Must |
| `POST` | `/snippets/search` | Agent | Dev (debug) | Run the retrieve node alone | Should |
| `POST` | `/codegen` | Agent | Dev (debug) | Run the generate node alone | Should |
| `POST` | `/snippets/ingest` | Agent | Seed script, agent's ingest node | Add snippet | Must |
| `GET` | `/snippets` | Agent | Scripts | List snippets | Should |
| `GET` | `/snippets/{id}` | Agent | Scripts | Fetch snippet | Should |
| `PATCH` | `/snippets/{id}` | Agent | Promote script | Promote / edit | Should |
| `DELETE` | `/snippets/{id}` | Agent | Scripts | Remove snippet | Should |
| `GET` | `/healthz` | Agent | Coordinator, dev | Health | Must |

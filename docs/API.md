# Clarity — REST API Reference

**Version:** 1.0
**Status:** planning — no code yet

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

---

## 3. Agent Service API

Internal. Only the coordinator calls it. Every response body is produced with structured outputs — no prose, no fences. Every request carries `guardrails`.

### 3.1 `POST /vision` — transcribe a screenshot

**Request**
```json
{ "image_b64": "iVBORw0KG...", "media_type": "image/png", "guardrails": false }
```
`image_b64` is raw base64 — the coordinator strips the data-URL prefix.

**Response `200`**
```json
{ "problem_text": "def binary_search(arr, target):\n    lo, hi = 0, len(arr)\n    ...", "category": "algorithm", "confidence": 0.91 }
```

| Field | Rule |
|---|---|
| `problem_text` | **Verbatim.** This string is hashed by the coordinator. |
| `category` | `"math"` \| `"algorithm"` \| `"unknown"`. |
| `confidence` | 0.0–1.0. |

`category: "unknown"` with `problem_text: ""` is a valid `200` — the coordinator turns it into `failed / no_problem_found`.

**Errors:** `400 bad_request` · `502 model_error` (Anthropic error, retryable) · `504 model_timeout`.

### 3.2 `POST /explain` — explanation + storyboard

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
  }
}
```

Constraints: 2–5 scenes; `narration` ≤ 90 chars; `visual` in relative terms; `duration_seconds` 5–15.

**Errors:** `400` · `502 model_error` · `504 model_timeout` · `422 schema_violation` (model output failed validation after one retry).

### 3.3 `POST /snippets/search` — retrieve reference snippets

**Request**
```json
{ "scene": { "index": 1, "narration": "…", "visual": "…" }, "category": "algorithm", "k": 3, "hint": "NameError: name 'RIGHT_ARROW' is not defined" }
```
`hint` is optional — on a repair attempt the coordinator passes the first line of the traceback so retrieval can find a snippet showing the correct API.

**Response `200`**
```json
{ "snippets": [ { "id": "66f1…", "title": "Array walk with a moving pointer", "description": "…", "source": "from manim import *\n…", "score": 0.86 } ] }
```
Only `verified: true` snippets are ever returned. Empty list is a valid `200`.

**Errors:** `400` · `502 embedding_error` · `503 atlas_unavailable`.

### 3.4 `POST /codegen` — generate or repair Manim source

**Request**
```json
{
  "scene": { "index": 1, "narration": "…", "visual": "…" },
  "snippets": [ { "title": "…", "source": "…" } ],
  "previous_source": null,
  "traceback": null,
  "guardrails": false
}
```
Repair: same call with `previous_source` and `traceback` (last 40 lines) set.

**Response `200`**
```json
{ "manim_source": "from manim import *\n\nclass GeneratedScene(Scene):\n    def construct(self):\n        …", "scene_class": "GeneratedScene" }
```
The service asserts `scene_class == "GeneratedScene"` and that the source contains `class GeneratedScene(Scene)`; otherwise `422 schema_violation`.

**Errors:** `400` · `422 schema_violation` · `502 model_error` · `504 model_timeout`.

### 3.5 `POST /snippets/ingest` — add a snippet to the corpus

Called by the seed script (`verified: true, origin: "seed"`) and by the coordinator after a successful render (`verified: false, origin: "generated"`).

**Request**
```json
{ "title": "…", "description": "…", "category": "algorithm", "tags": ["VGroup", "arrange", "Arrow"], "source": "from manim import *\n…", "origin": "generated", "verified": false }
```

**Response `201`**
```json
{ "id": "66f1a2b3c4d5e6f7a8b9c0d1", "embedded": true }
```

**Errors:** `400` · `409 duplicate_snippet` (identical `source` already stored — returns the existing `id` in `details`) · `502 embedding_error` · `503 atlas_unavailable`.

### 3.6 `GET /snippets` — list snippets

Corpus management. `?verified=false&origin=generated&limit=50&cursor=…`.

**Response `200`**
```json
{ "snippets": [ { "id": "…", "title": "…", "category": "…", "origin": "generated", "verified": false, "created_at": "…" } ], "next_cursor": null }
```
`source` and `embedding` are omitted from list responses; fetch one snippet for the source.

### 3.7 `GET /snippets/{id}` — fetch one snippet

Full document minus `embedding`.

**Errors:** `404 snippet_not_found`.

### 3.8 `PATCH /snippets/{id}` — promote, demote, or edit

```json
{ "verified": true }
```
Any subset of `title`, `description`, `category`, `tags`, `verified`. Changing `title`/`description`/`tags` re-embeds the document.

**Response `200`** — the updated document minus `embedding`.

### 3.9 `DELETE /snippets/{id}` — remove a snippet

`204 No Content`. Used when a generated snippet rendered but looked wrong.

### 3.10 `GET /healthz`

```json
{ "ok": true, "anthropic": true, "voyage": true, "atlas": true, "snippets_verified": 42, "embed_model": "voyage-code-3", "version": "0.1.0" }
```

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
| 502 | `model_error` | agent | Anthropic returned an error. Retryable. |
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
  │                        │   POST /snippets/search ───►│                            │
  │                        │   POST /codegen ───────────►│                            │
  │                        │   precheck → docker run ────────────────────────────────►│
  │                        │   (fail → /codegen again with traceback, ≤3)             │
  │                        │  ffmpeg concat → S3 upload ─────────────────────────────►│
  │                        │  write cache; jobs.done     │                            │
  │                        │  POST /snippets/ingest ×N ─►│  (fire-and-forget)         │
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
| Coordinator → `/explain` | 45 s | Coordinator client |
| Coordinator → `/snippets/search` | 10 s | Coordinator client |
| Coordinator → `/codegen` | 90 s | Coordinator client |
| Agent → Anthropic | 60 s per call, 1 retry on 5xx/429 | Agent |
| Agent → Voyage | 10 s, 1 retry | Agent |
| Per-scene container | 120 s | Coordinator, `docker kill` |
| Repair attempts per scene | 3 | Coordinator |
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
| `POST` | `/vision` | Agent | Coordinator | Transcribe | Must |
| `POST` | `/explain` | Agent | Coordinator | Explanation + storyboard | Must |
| `POST` | `/snippets/search` | Agent | Coordinator | Retrieve snippets | Must |
| `POST` | `/codegen` | Agent | Coordinator | Generate / repair source | Must |
| `POST` | `/snippets/ingest` | Agent | Coordinator, seed script | Add snippet | Must |
| `GET` | `/snippets` | Agent | Scripts | List snippets | Should |
| `GET` | `/snippets/{id}` | Agent | Scripts | Fetch snippet | Should |
| `PATCH` | `/snippets/{id}` | Agent | Promote script | Promote / edit | Should |
| `DELETE` | `/snippets/{id}` | Agent | Scripts | Remove snippet | Should |
| `GET` | `/healthz` | Agent | Coordinator, dev | Health | Must |

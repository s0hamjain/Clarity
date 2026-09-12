# Clarity — P3: Backend (Coordinator)

**You are P3. Your job in one sentence:** build the Go server the desktop app talks to — accept a screenshot, create a job, and drive that job through every step by calling P1's endpoints and P2's functions in the right order, recording status in MongoDB the whole way.

You contain no AI logic and no rendering logic. You are a dispatcher. Your hardest problems are ordering, concurrency, and never leaving a job stuck.

---

## Your job in plain words

1. **Accept a job.** `POST /api/jobs` with an image and text → save a job record → return a job ID in under 50 ms. Everything else happens in the background.
2. **Report status.** `GET /api/jobs/{id}` returns the job record. The desktop app polls it every second.
3. **Drive the pipeline.** In a goroutine: ask P1 to transcribe → hash the text → check the cache → (miss) ask P1 to explain → **save the explanation immediately** → for each scene in parallel, ask P1 for snippets and code and P2 to render → ask P2 to stitch and upload → save the video URL → mark done.
4. **Cache.** Same problem twice → skip everything and return the stored result in under a second.
5. **Cancel.** `DELETE /api/jobs/{id}` stops pending work.
6. **Never hang.** Every goroutine recovers from panics. Every job ends in `done`, `failed`, or `cancelled`.

---

## What you own · what you never touch

| You own | You never touch |
|---|---|
| `server/cmd/**` | `server/internal/render/**` — P2's package. You call its three functions; you don't edit them. |
| `server/internal/api/**` — HTTP handlers, CORS | Any prompt or anything in `agent/` (P1) |
| `server/internal/store/**` — MongoDB: `jobs`, `cache`, TTL indexes | The `manim_snippets` collection (P1's) |
| `server/internal/cache/**` — the cache key | The Dockerfile (P2) |
| `server/internal/agent/**` — HTTP client for P1's service | Anything in `desktop/` (P4) |
| `server/internal/jobs/**` — the worker, the fan-out | |
| `server/go.mod` — you own the module; P2 adds dependencies through you or with notice | |

---

## Where your work meets others

| Boundary | Who implements | Who calls | Spec |
|---|---|---|---|
| Public HTTP API (`/api/jobs`, `/healthz`, …) | **You** | P4's desktop app | [API.md §2](API.md#2-coordinator-api) |
| Agent HTTP API (`/vision`, `/explain`, `/snippets/search`, `/codegen`, `/snippets/ingest`) | P1 | **You** | [API.md §3](API.md#3-agent-service-api) |
| `Render`, `RenderWithRepair`, `Concat` | P2 | **You** | [FRD §14.1](FRD.md#14-render-pipeline) |
| `jobs` and `cache` documents | **You** | P4 reads the job shape through your API | [FRD §9.1–9.2](FRD.md#9-database-schema-mongodb-atlas) |

**You start on fakes.** Hardcode P1's responses and copy a sample MP4 instead of rendering. Swap in the real things as they land (Sprint 2 for P1, Sprint 3 for P2).

---

## Before you start (30 minutes)

Follow [SETUP.md](SETUP.md) §1, §5, §7, §9. You need Go 1.22+, the `MONGODB_URI` (create the Atlas cluster if you're first — SETUP §5 — and share the string), MinIO running, Docker running (for `/healthz` and, later, P2's code).

Read once: **API.md §2, §4, §5, §7** (your API, the error envelope, the status lifecycle, the limits), **FRD §9.1–9.2** (your documents), **FRD §11–§12** (API in context, the cache key), **FRD §14.1** (P2's signatures), **FRD §19** (every failure and what you do), **FRD §23 rules 7–13** (yours).

---

## Files you'll create

```
server/
├── .env / .env.example
├── go.mod, go.sum
├── cmd/server/main.go               # env → Atlas → TTL indexes → boot health checks → HTTP
└── internal/
    ├── api/
    │   ├── handlers.go              # POST /api/jobs · GET /api/jobs/{id} · DELETE /api/jobs/{id} · GET /api/cache/{hash} · GET /healthz
    │   ├── errors.go                # the one error envelope (API.md §4)
    │   └── cors.go                  # Access-Control-Allow-Origin: * on every response, including 404/5xx
    ├── store/
    │   ├── mongo.go                 # client, db handle
    │   ├── jobs.go                  # Create / Get / Update — every Update sets updated_at
    │   ├── cache.go                 # Get / Put by problem_hash
    │   └── indexes.go               # TTL: jobs.updated_at 24 h, cache.created_at 7 d — idempotent
    ├── cache/
    │   ├── key.go                   # PromptVersion const · Normalize() · Hash()
    │   └── key_test.go
    ├── agent/
    │   └── client.go                # typed functions for each P1 endpoint; strips data-URL prefix; per-endpoint timeouts
    └── jobs/
        ├── job.go                   # Job struct = FRD §9.1; Status constants
        ├── worker.go                # the pipeline, one job
        └── scenes.go                # bounded parallel fan-out over scenes
```

---

## Sprint 1 — HTTP skeleton, Atlas job store, a fake pipeline, the cache key

**Goal:** the desktop app (or `curl`) can create a job and watch it walk through every status, with a fake explanation and a fake video URL. Jobs live in Atlas and expire.

### Step 1 — HTTP skeleton (1 h)
- `cmd/server/main.go`: read env (FRD §22 defaults), start `net/http` on `PORT`.
- `api/handlers.go`: the routes in **API.md §2**, exactly those paths. Return the error envelope (`api/errors.go`) on every non-2xx — including 404 for unknown routes.
- `api/cors.go`: `Access-Control-Allow-Origin: *` on **every** response.
- `X-Request-Id`: echo if present, else generate.

### Step 2 — Atlas job store (1 h)
- `store/mongo.go`: `mongo.Connect(MONGODB_URI)`, database `MONGODB_DB`.
- `store/jobs.go`: `Create(job)`, `Get(id)`, `Update(id, fields)`. **`Update` always sets `updated_at = now`** — the TTL index deletes jobs whose `updated_at` is 24 h old, so a long render must keep touching it.
- `store/cache.go`: `Get(hash)`, `Put(hash, videoURL, explanation)`.
- `store/indexes.go`: create both TTL indexes at boot (FRD §9.1–9.2). Idempotent — `CreateOne` with the same spec is a no-op.

### Step 3 — Fake worker (45 min)
- `jobs/worker.go`: a goroutine that sleeps 1 s per status: `queued → transcribing → explaining → generating → rendering → concatenating → uploading → done`. At `generating` write a hardcoded markdown explanation and `scenes_total: 3`. At `rendering` increment `scenes_done` once per second. At `done` set `video_url` to a sample MP4 you've put in MinIO by hand.
- `defer recover()` at the top of the goroutine → mark the job `failed` with `error: "internal"`.
- This is now a better stub than P4's own — tell them.

### Step 4 — Cache key (30 min)
- `cache/key.go`: `const PromptVersion = "2026-09-12a"`; `Normalize(s)` = lowercase, trim, collapse every whitespace run (incl. newlines) to one space, nothing else; `Hash(problemText, userPrompt string, guardrails bool)` = `sha256(PromptVersion + "||" + Normalize(problemText) + "||" + Normalize(userPrompt) + "||" + "true"|"false")` hex, first 16 chars. **Exactly FRD §12.**
- `key_test.go`: a multi-line, tab-indented, mixed-case input normalizes to one known string; `guardrails` true vs false produce different hashes; same input twice produces the same hash.

### Done when
- [ ] `curl -X POST localhost:8080/api/jobs -d '{"image":"data:image/png;base64,AAAA"}'` → `202 {"job_id": …}` in < 50 ms.
- [ ] Polling the ID walks through every status; explanation appears at `generating`; `video_url` at `done`.
- [ ] The job is visible in Atlas → `clarity.jobs`; both TTL indexes exist.
- [ ] `go test ./internal/cache/` passes.
- [ ] A 404 returns the error envelope with CORS headers.

---

## Sprint 2 — Real P1 calls, real cache, the fan-out skeleton

**Goal:** a real screenshot produces a real explanation, saved the instant it arrives; a repeated screenshot hits the cache.

### Step 1 — Agent client (1 h)
- `agent/client.go`: one typed function per P1 endpoint, request/response structs matching **API.md §3** exactly. Timeouts: `/vision` 30 s, `/explain` 45 s, `/snippets/search` 10 s, `/codegen` 90 s (API.md §7). Strip the `data:image/png;base64,` prefix before `/vision` — P1 receives raw base64. Map P1's error envelope into a Go error you can log.

### Step 2 — The real pipeline through `/explain` (1.5 h)
`worker.go`, replacing the fake:
```
transcribing → agent.Vision(image)
             → if category == "unknown": failed / no_problem_found; stop
             → hash := cache.Hash(problemText, userPrompt, guardrails)
             → save problem_hash, problem_text, category
             → if hit := store.Cache.Get(hash): status done, cached true, explanation + video_url from cache; stop
explaining   → agent.Explain(problemText, category, userPrompt, guardrails)
             → **save explanation and scenes_total NOW** — before anything else
generating   → (still fake from here in this sprint)
```
That bolded line is the most important line in the server. The desktop app shows the explanation the moment it's non-null; every second between `/explain` returning and that write is a second the user waits for nothing.

### Step 3 — Fan-out skeleton (1 h)
- `jobs/scenes.go`: for N scenes, start N goroutines bounded by P2's semaphore (import `render.Semaphore`; until P2 ships it, a local buffered channel). Each goroutine: `defer recover()` → mark that scene failed, never the job. Each calls a `SceneFunc(ctx, scene) (clipPath string, err error)` and on return increments `scenes_done`. Wait for all N. Collect successful clip paths in scene order.
- `SceneFunc` is fake this sprint: sleep 2 s, copy the sample MP4.

### Step 4 — Cache write (30 min)
After the (fake) concat/upload: `store.Cache.Put(hash, videoURL, explanation)` — **both fields**, so a hit shows text while the video loads. Then `done`. **Never write the cache on any failure path.**

### Done when
- [ ] P4's desktop app (or `curl` with a real PNG) → real explanation in the job record within ~10 s.
- [ ] The same PNG submitted again → `cached: true` on the first or second poll, with explanation and video_url.
- [ ] `category: "unknown"` → `failed`, `error: "no_problem_found"`.
- [ ] Kill the agent service mid-job → job ends `failed`, never stuck.

---

## Sprint 3 — Wire the real render, finish the pipeline, cancel

**Goal:** one capture → real video URL, all real; a closed result box cancels its job.

### Step 1 — Real `SceneFunc` (1 h)
```go
retrieve := func(scene, hint) []Snippet { return agent.SnippetsSearch(scene, category, 3, hint) }
codegen  := func(scene, snippets, prevSrc, tb) string { return agent.Codegen(scene, snippets, prevSrc, tb, guardrails) }
clip, rerr := render.RenderWithRepair(ctx, scene, retrieve, codegen)
```
A `rerr` after 3 attempts means **drop this scene** — log it, increment `scenes_done`, return no clip. Never fail the job for one scene.

### Step 2 — Tail of the pipeline (1 h)
```
concatenating → if len(clips) == 0: done, video_url null, error "all_scenes_failed"; NO cache write; stop
              → url := render.Concat(ctx, clips, "renders/"+hash+".mp4")
uploading     → (Concat does the upload; status is for the UI)
              → store.Cache.Put(hash, url, explanation)
done          → video_url = url
```
Post-render ingest: for every successful scene, `go agent.SnippetsIngest({source, title: storyboard.title+" — scene "+i, description: scene.visual, category, origin: "generated", verified: false})`. Fire-and-forget; log errors, never fail the job.

### Step 3 — Cancel (45 min)
- Each job gets a `context.WithCancel`; keep the cancel func in a map by job ID.
- `DELETE /api/jobs/{id}`: call cancel → pending scenes see `ctx.Done()` and skip; running containers are killed by P2's `Render` on ctx cancellation; status `cancelled`; no cache write. Idempotent: cancelling a finished job returns `200` with the current status.
- `GET /api/cache/{hash}` for pre-warm checks (API.md §2.5).

### Step 4 — Real `/healthz` (30 min)
Boot-time checks, refreshed every 60 s: Atlas ping, `docker info`, S3 `HeadBucket`, `GET <AGENT_URL>/healthz`. Include `version` and `prompt_version` (API.md §2.6). `503` if `ok` is false.

### Done when
- [ ] One real capture → `done` with a `video_url` that plays. All real, no fakes.
- [ ] Inject a bad import into one scene's code → that scene drops, the other scenes' video still plays, `scenes_done == scenes_total`.
- [ ] `DELETE` mid-render → `cancelled` within a few seconds, `docker ps` empty, nothing in `cache`.
- [ ] `/healthz` all `true` from a fresh boot.

---

## Sprint 4 — Prove the cache across machines, break things on purpose

**Goal:** the cache works between two laptops; nothing you can kill leaves a job stuck.

1. **Two-machine cache test (1 h).** Two Macs, same Atlas cluster, same problem captured at different zoom levels. The second must be `cached: true`. If not, log both `Normalize()` outputs and diff them — this is the moment you find out whether the cache design works at all.
2. **Failure injection (1 h).** Stop the agent service mid-job → `failed`, not stuck. Stop Docker mid-render → scenes fail, job ends `done`/no video. Panic inside `SceneFunc` → siblings finish. Kill the coordinator mid-job and restart → the job record in Atlas is intact (it may never finish — that's acceptable; it must not corrupt).
3. **Overload (30 min).** A bounded job queue (depth 32). Over the bound → `503 queue_full` with `Retry-After`.
4. **`updated_at` audit (15 min).** Grep every `store.Jobs.Update` call — every one sets `updated_at`. A job that sits in `rendering` for 3 minutes must not expire.

### Done when
- [ ] Second laptop hits the cache in < 1 s.
- [ ] Every injected failure ends in a terminal status.
- [ ] `503` on overload.

---

## Sprint 5 — Freeze

- All-green `/healthz` from a fresh boot on the shared cluster.
- Pre-warm the cache with P1: 4–5 representative problems run end to end. Verify each with `GET /api/cache/{hash}`.
- No code changes after the freeze.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p3/sprint-N-<what>`.
2. Commit small. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, `go build ./... && go test ./...`, push.
4. **At the sync point:** merge order is **P3 → P1 → P2 → P4**. **You go first** — everyone else's work is built against your shapes. You are also the merge captain in Sprints 1 and 5.
5. After the merge: back to step 1.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

7. `POST /api/jobs` writes the job and returns. Everything else runs in a goroutine with `defer recover()`.
8. `explanation` is written to `jobs` **before** any `/snippets/search` or `/codegen` call.
9. Every `jobs` write sets `updated_at = now`.
10. `cache` is written only after upload succeeds. Never on any failure path.
11. The cache-hit path never calls `/explain`.
12. The manim quality flag is one job-level value passed identically to every scene.
13. `PromptVersion` is bumped in the same commit as any prompt change — P1 will usually do it, but it's your file.

---

## Traps

- A job that never touches `updated_at` during a 3-minute render gets deleted by the TTL index mid-flight. The desktop app sees a 404. Rule 9.
- `defer recover()` in the *parent* goroutine doesn't catch panics in scene goroutines. Each one needs its own.
- A cache hit that returns only `video_url` shows the user a loading video with nothing to read. Return both.
- Killing the `docker` CLI doesn't kill the container — but that's P2's problem to solve inside `Render`. Yours is to cancel the `ctx`.

## If you're blocked

Sprint 1: nothing to wait for. Sprint 2: if P1's `/explain` isn't ready, keep the hardcoded response behind a `FAKE_AGENT=1` env flag. Sprint 3: if P2's functions aren't ready, keep the fake `SceneFunc` behind `FAKE_RENDER=1`. Both flags get deleted in Sprint 4.

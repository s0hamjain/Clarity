# Clarity — P3: Backend (Coordinator)

**You are P3. Your job in one sentence:** build the Go server the desktop app talks to — accept a screenshot, create a job, and drive that job through every step by calling P1's endpoints and P2's functions in the right order, recording status in MongoDB the whole way — and ship the release (PKG, start-at-login, GitHub Release) once P4 hands you the `.app`.

You contain no AI logic and no rendering logic. You are a dispatcher. Your hardest problems are ordering, concurrency, and never leaving a job stuck.

---

## Your job in plain words

1. **Accept a job.** `POST /api/jobs` with an image and text → save a job record → return a job ID in under 50 ms. Everything else happens in the background.
2. **Report status.** `GET /api/jobs/{id}` returns the job record. The desktop app polls it every second.
3. **Drive the pipeline.** In a goroutine: ask P1 to transcribe → hash the text → check the cache → (miss) ask P1 to explain → **save the explanation immediately** → for each scene in parallel, call P1's Manim Generator agent (`POST /scenes/render`) and collect the clips → ask P2 to stitch and upload → save the video URL → mark done.
3b. **Be the agent's render tool.** `POST /internal/render` — localhost only — takes source + `work_dir` + `quality`, acquires P2's semaphore, runs the pre-check, calls P2's `Render()`, returns the clip path or the traceback. The agent decides whether to try again; you just render.
4. **Cache.** Same problem twice → skip everything and return the stored result in under a second.
5. **Cancel.** `DELETE /api/jobs/{id}` stops pending work.
6. **Never hang.** Every goroutine recovers from panics. Every job ends in `done`, `failed`, or `cancelled`.
7. **Fake mode from hour one.** `FAKE_AGENT=1 FAKE_RENDER=1` makes your server walk any job through every status on a timer with sample data — this is what P4 builds the UI against, so it exists at the end of Sprint 1.
8. **Ship it.** `release/` is yours: the `.pkg` installer with a LaunchAgent, the release notes, and the `gh release create`.

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
| `release/**` — `build_pkg.sh`, `postinstall.sh`, `publish.sh`, `RELEASE_NOTES.md` | `desktop/**` — P4 builds the `.app` and `.dmg`; you package and publish what they hand you |

---

## Where your work meets others

| Boundary | Who implements | Who calls | Spec |
|---|---|---|---|
| Public HTTP API (`/api/jobs`, `/healthz`, …) | **You** | P4's desktop app | [API.md §2](API.md#2-coordinator-api) |
| Agent HTTP API (`/vision`, `/explain`, `/snippets/search`, `/codegen`, `/snippets/ingest`) | P1 | **You** | [API.md §3](API.md#3-agent-service-api) |
| `Render`, `Concat`, `Semaphore` | P2 | **You** (from `/internal/render` and the job tail) | [FRD §14.1](FRD.md#14-render-pipeline) |
| `POST /internal/render` | **You** | P1's agent render tool | [API.md §2.7](API.md#27-post-internalrender--render-one-source-file-internal-called-by-the-agent) |
| `jobs` and `cache` documents | **You** | P4 reads the job shape through your API | [FRD §9.1–9.2](FRD.md#9-database-schema-mongodb-atlas) |

**You start on fakes, and your fakes are everyone's stub.** `FAKE_AGENT=1` hardcodes P1's responses; `FAKE_RENDER=1` copies a sample MP4 instead of rendering. Together they make your server walk a job through every status on a timer — P4 builds the whole UI against that. Swap in the real things as they land (Sprint 2 for P1, Sprint 3 for P2). Delete the flags in Sprint 4.

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
│   ├── internal_render.go       # POST /internal/render — localhost only; semaphore → precheck → render.Render()
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

And, from Sprint 4:

```
release/
├── build_pkg.sh                     # pkgbuild + productbuild → dist/Clarity.pkg (takes P4's dist/Clarity.app)
├── publish.sh                       # gh release create v0.1.0 with the .dmg and .pkg
├── RELEASE_NOTES.md                 # install steps, permissions, known issues
└── scripts/
    └── postinstall                  # writes + loads ~/Library/LaunchAgents/com.clarity.app.plist
```

---

## Sprint 1 — HTTP skeleton, Atlas job store, a fake pipeline, the cache key

(Budget: ~3.25 h.)

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

### Step 3 — Fake worker — this is P4's stub server too (1 h)
- Behind `FAKE_AGENT=1 FAKE_RENDER=1` (both default to `1` until Sprint 4). `jobs/worker.go`: a goroutine that sleeps 1 s per status: `queued → transcribing → explaining → generating → rendering → concatenating → uploading → done`. At `generating` write a hardcoded markdown explanation and `scenes_total: 3`. At `rendering` increment `scenes_done` once per second. At `done` set `video_url` to a sample MP4 you've put in MinIO by hand.
- `defer recover()` at the top of the goroutine → mark the job `failed` with `error: "internal"`.
- Tell P4 the moment this runs — it's their server for Sprints 1–2. It must work with **no** Atlas, Docker, or API keys when both flags are on (an in-memory job map is fine in fake mode).

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

(Budget: ~3.5 h.)

**Goal:** a real screenshot produces a real explanation, saved the instant it arrives; a repeated screenshot hits the cache.

### Step 1 — Agent client (1 h)
- `agent/client.go`: one typed function per P1 endpoint you call — `Vision`, `Explain`, `ScenesRender` — request/response structs matching **API.md §3** exactly. Timeouts: `/vision` 30 s, `/explain` 90 s, `/scenes/render` 11 min (API.md §7). Strip the `data:image/png;base64,` prefix before `/vision` — P1 receives raw base64. Map P1's error envelope into a Go error you can log.

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

### Step 3 — Fan-out skeleton + `/internal/render` (1.5 h)
- `jobs/scenes.go`: for N scenes, create `work_dir = <tmp>/<job_id>/scene<i>` and start N goroutines (no semaphore here — that lives in `/internal/render`). Each goroutine: `defer recover()` → mark that scene failed, never the job. Each calls `SceneFunc(ctx, scene, workDir) (clipPath string, ok bool)` and on return increments `scenes_done`. Wait for all N. Collect clip paths in scene order.
- `SceneFunc` is fake this sprint (`FAKE_RENDER=1`): sleep 2 s, copy the sample MP4.
- `api/internal_render.go`: `POST /internal/render` per API.md §2.7. Refuse non-loopback remote addresses with `403`. Acquire `render.Semaphore` (60 s max wait → `429 render_busy`), run `render.Precheck`, call `render.Render(ctx, source, workDir, quality)`, return `{ok, clip_path}` or `{ok: false, stage, traceback}`. Until P2's `Render` lands, `FAKE_RENDER=1` copies the sample MP4 here too. **P1 needs this in Sprint 3** — ship it at the end of Sprint 2.

### Step 4 — Cache write (30 min)
After the (fake) concat/upload: `store.Cache.Put(hash, videoURL, explanation)` — **both fields**, so a hit shows text while the video loads. Then `done`. **Never write the cache on any failure path.**

### Done when
- [ ] P4's desktop app (or `curl` with a real PNG) → real explanation in the job record within ~10 s.
- [ ] The same PNG submitted again → `cached: true` on the first or second poll, with explanation and video_url.
- [ ] `category: "unknown"` → `failed`, `error: "no_problem_found"`.
- [ ] Kill the agent service mid-job → job ends `failed`, never stuck.

---

## Sprint 3 — Wire the real render, finish the pipeline, cancel

(Budget: ~3.25 h.)

**Goal:** one capture → real video URL, all real; a closed result box cancels its job.

### Step 1 — Real `SceneFunc` (45 min)
```go
resp, err := agent.ScenesRender(ctx, agent.SceneRenderRequest{JobID, Scene: scene, StoryboardTitle, Category, Guardrails, WorkDir: workDir, Quality: cfg.ManimQuality})
if err != nil || !resp.OK { log(...); return "", false }   // drop this scene, never the job
return resp.ClipPath, true
```
One HTTP call per scene with an **11-minute** client timeout (API.md §7). The agent owns retrieve → generate → lint → render → repair; while it loops it calls back into your `/internal/render`. Log `resp.Attempts` and `resp.SnippetID`.

### Step 2 — Tail of the pipeline (1 h)
```
concatenating → if len(clips) == 0: done, video_url null, error "all_scenes_failed"; NO cache write; stop
              → url := render.Concat(ctx, clips, "renders/"+hash+".mp4")
uploading     → (Concat does the upload; status is for the UI)
              → store.Cache.Put(hash, url, explanation)
done          → video_url = url
```
Post-render ingest is the agent's job now (its `ingest` node) — nothing to do here except log `snippet_id`.

### Step 3 — Cancel (45 min)
- Each job gets a `context.WithCancel`; keep the cancel func in a map by job ID. Pass the job's ctx into every `/scenes/render` call so cancelling aborts the HTTP request, and have `/internal/render` look up the job's ctx by `work_dir` prefix so a running container is killed too.
- `DELETE /api/jobs/{id}`: call cancel → in-flight agent calls abort, running containers die via ctx, status `cancelled`, no cache write. Idempotent: cancelling a finished job returns `200` with the current status.
- `GET /api/cache/{hash}` for pre-warm checks (API.md §2.5).

### Step 4 — Real `/healthz` (30 min)
Boot-time checks, refreshed every 60 s: Atlas ping, `docker info`, S3 `HeadBucket`, `GET <AGENT_URL>/healthz`. Include `version` and `prompt_version` (API.md §2.6). `503` if `ok` is false.

### Done when
- [ ] One real capture → `done` with a `video_url` that plays. All real, no fakes.
- [ ] Force one scene's agent call to return `ok: false` (P1 has a debug flag) → that scene drops, the other scenes' video still plays, `scenes_done == scenes_total`.
- [ ] `curl -X POST localhost:8080/internal/render` with a sample source → `{ok: true, clip_path}`; from another machine → `403`.
- [ ] `DELETE` mid-render → `cancelled` within a few seconds, `docker ps` empty, nothing in `cache`.
- [ ] `/healthz` all `true` from a fresh boot.

---

## Sprint 4 — Prove the cache across machines, break things on purpose, build the PKG

(Budget: ~4 h.)

**Goal:** the cache works between two laptops; nothing you can kill leaves a job stuck; the `.pkg` installer exists.

1. **Two-machine cache test (1 h).** Two Macs, same Atlas cluster, same problem captured at different zoom levels. The second must be `cached: true`. If not, log both `Normalize()` outputs and diff them — this is the moment you find out whether the cache design works at all.
2. **Failure injection (1 h).** Stop the agent service mid-job → `failed`, not stuck. Stop Docker mid-render → scenes fail, job ends `done`/no video. Panic inside `SceneFunc` → siblings finish. Kill the coordinator mid-job and restart → the job record in Atlas is intact (it may never finish — that's acceptable; it must not corrupt).
3. **Overload (30 min).** A bounded job queue (depth 32). Over the bound → `503 queue_full` with `Retry-After`. `/internal/render` semaphore wait > 60 s → `429 render_busy`.
3b. **Structured logging (30 min).** Every log line carries `request_id`, `job_id`, and `scene` where applicable; JSON to stdout. The agent's `thread_id` is `job_id/scene`, so the two services' logs join on it.
4. **`updated_at` audit (15 min).** Grep every `store.Jobs.Update` call — every one sets `updated_at`. A job that sits in `rendering` for 3 minutes must not expire.
5. **Delete the fake flags (15 min).** `FAKE_AGENT` and `FAKE_RENDER` go away. Everything is real from here.
6. **PKG installer (1 h).** P4 hands you `dist/Clarity.app` at their Sprint 4 Step 2. In `release/`:
   - `build_pkg.sh`: `pkgbuild --component Clarity.app --install-location /Applications --scripts release/scripts --identifier com.clarity.app --version 0.1.0 Clarity-component.pkg` then `productbuild --package Clarity-component.pkg dist/Clarity.pkg`.
   - `scripts/postinstall`: writes `~/Library/LaunchAgents/com.clarity.app.plist` (`RunAtLoad true`, `ProgramArguments /Applications/Clarity.app/Contents/MacOS/Clarity`) and `launchctl load`s it, so Clarity starts at login. Runs as the console user, not root — `pkgbuild` scripts run as root, so use `sudo -u "$USER"` / `launchctl asuser`.
   - Install the `.pkg` on your own Mac. Log out, log in. Menu bar icon is there → it works.
   - You're also the natural second-Mac tester for P4's DMG (their Sprint 4 Step 4). Write down what you hit — it's the seed of `release/RELEASE_NOTES.md`.

### Done when
- [ ] Second laptop hits the cache in < 1 s.
- [ ] Every injected failure ends in a terminal status.
- [ ] `503` on overload.
- [ ] `dist/Clarity.pkg` installs, and Clarity is in the menu bar after a fresh login.
- [ ] Fake flags deleted.

---

## Sprint 5 — Freeze and publish (Budget: ~1 h)

- All-green `/healthz` from a fresh boot on the shared cluster.
- Pre-warm the cache with P1: 4–5 representative problems run end to end. Verify each with `GET /api/cache/{hash}`.
- **Publish.** P4 hands you a final `dist/Clarity.dmg` and `dist/Clarity.app` from clean `main`. Rebuild the `.pkg`. Finish `release/RELEASE_NOTES.md` (install steps incl. macOS 15 "Open Anyway", the two permissions, known issues). Then `release/publish.sh`: `gh release create v0.1.0 dist/Clarity.dmg dist/Clarity.pkg --title "Clarity 0.1.0" --notes-file release/RELEASE_NOTES.md`. P4 verifies the install from the release URL on a second Mac.
- Tag `v0.1.0`. No code changes after.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p3/sprint-N-<what>`.
2. Commit small. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, `go build ./... && go test ./...`, push.
4. **At the sync point:** merge order is **P3 → P1 → P2 → P4**. **You go first** — everyone else's work is built against your shapes. You are also the merge captain in Sprints 1 and 5.
6. **Finished your sprint early?** Take the next item from the Overflow backlog in WORK_SPLIT.md — anyone can, regardless of role.
5. After the merge: back to step 1.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

7. `POST /api/jobs` writes the job and returns. Everything else runs in a goroutine with `defer recover()`.
8. `explanation` is written to `jobs` **before** any `/scenes/render` call.
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
- Killing the `docker` CLI doesn't kill the container — that's P2's problem inside `Render`. Yours is to cancel the `ctx`.
- `/internal/render` without the semaphore = N agents × 3 attempts of concurrent Docker runs. Always acquire.

## If you're blocked

Sprint 1: nothing to wait for. Sprint 2: if P1's `/explain` isn't ready, leave `FAKE_AGENT=1` on. Sprint 3: if P2's functions aren't ready, leave `FAKE_RENDER=1` on. Both flags get deleted in Sprint 4. Sprint 4: if P4's `.app` is late, build the PKG against the SETUP §11.2 command run on your own machine — the script doesn't care who built the bundle.

# AGENTS.md — Clarity

This file is the single point of entry for anyone — person or coding agent — working on this project. Read it in full before writing any code. It says what the project is, where the authoritative details live, what rules must never be broken, and what every sprint requires of every person.

---

## 1. Project Summary

### What Clarity is

A macOS menu-bar app for students. Press `⌘⇧E`, drag a box around any problem on your screen — a PDF, an IDE, a browser, a slide — and a translucent text box slides in. Type what's confusing you if you want (*"why is my binary search not working? visualize where it's messing up"*), or pick a recent screenshot, and press Enter. A floating result window shows a written, step-by-step explanation within seconds. About a minute later, a short animation made for that exact problem plays in the same window. Drag the window anywhere so it doesn't cover the problem; click X when you're done.

The animation is generated, not looked up: a model writes [Manim](https://www.manim.community/) code for the problem, grounded in a library of verified working examples, and Clarity renders it.

If you haven't read [README.md](README.md) yet — especially the glossary — do that first. This file assumes those words.

### What happens when you press the hotkey

1. The desktop app takes the screenshot and sends it (plus your text) to the **coordinator**, a Go server on your Mac. The coordinator creates a **job** and replies instantly with a job ID. The desktop app starts polling that job once a second.
2. The coordinator asks the **agent service** (Python, the only thing that talks to any AI model) to read the problem off the screenshot, word for word — that's **Gemini**, at `temperature=0` so the same problem always transcribes the same way.
3. The coordinator fingerprints that text. If the same problem has been asked before, it returns the stored explanation and video — done in under a second.
4. Otherwise it asks the agent service (**Claude Opus 5**) for a written explanation and a **storyboard**: 2–5 short scenes describing what the animation should show. **The explanation is saved the instant it arrives, and the result box shows it.** This is the moment the user stops waiting.
5. For each scene, in parallel: the agent service finds the 3 most similar verified **snippets** from the example library (vector search in MongoDB Atlas), writes Manim code imitating them (**Claude Sonnet 5**), and the **render pipeline** runs that code inside a locked-down Docker container. If the code crashes, the error goes back to the model and it tries again, up to 3 times. A scene that never works is dropped; the others carry on.
6. Finished scenes are stitched into one video (no re-encoding), uploaded to S3, and the job is marked done. The result box plays it. Successful code is saved back into the library as an unverified example for a human to review.

If rendering fails entirely, the job still finishes: the explanation stays on screen with a quiet "the animation didn't render this time." The explanation never depends on the video.

### The stack

| Part | Tech | Job |
|---|---|---|
| Desktop app | Python — `rumps` (menu bar), `pynput` (hotkey), `pywebview` (the two floating windows), PyInstaller → `.dmg` | Everything the user sees and installs |
| Coordinator | Go, `net/http` | Job lifecycle, cache, calling the other parts in order, fanning scenes out in parallel |
| Agent service | Python, FastAPI, `google-genai` + `anthropic` SDKs | Every model call: transcription (Gemini), explanation (Opus 5), code + repair (Sonnet 5); plus the snippet library |
| Render pipeline | Go + Docker + ffmpeg | Manim code → sandboxed render → repair loop → stitched MP4 on S3 |
| MongoDB Atlas | Free M0 cluster | Jobs, cache, and the snippet library (with a vector-search index) |
| Gemini 3.8 Flash | Google Gemini API | Reads the screenshot (OCR), `temperature=0` |
| Claude Opus 5 · Claude Sonnet 5 | Anthropic API | Explanation · Manim code and repair |
| Voyage AI | `voyage-code-3` | Turns text into embeddings for vector search |
| S3 (MinIO locally) | | Finished videos |

### The team

Four people, four roles, four directories, no overlap:

| Role | Person | Directory | One sentence |
|---|---|---|---|
| AI | P1 | `agent/` | Make the models produce correct JSON: transcription (Gemini), explanation (Opus 5), code + repair (Sonnet 5). |
| Render | P2 | `docker/`, `samples/`, `server/internal/render/` | Turn Manim code into an MP4 on S3, safely; write the verified example scenes. |
| Backend | P3 | `server/` (rest), `release/` | Run the job: API, database, call P1 and P2 in order, report status. Ship the release. |
| Desktop | P4 | `desktop/` | Everything the user sees; the `.app` and `.dmg`. |

Five time-boxed sprints, **15 hours budgeted per person** — the split is even by design, and an overflow backlog in `docs/WORK_SPLIT.md` absorbs anyone who finishes early. Everyone works on their own branch until the end of a sprint, then everyone's work is merged into `main` in a fixed order (P3 → P1 → P2 → P4) and we run a checklist together. Every role fakes the parts it depends on until the real thing lands, so nobody waits on anybody.

---

## 2. Document Reference

There are four authoritative documents plus this file and the file map. Always consult the relevant one before implementing. Never guess at a schema, endpoint, index definition, or configuration value — look it up.

### docs/FRD.md (the technical specification)

The source of truth for every implementation detail. Section map:

- **§1–5 Product** — what it is, goals and non-goals, users, success criteria, the product surface (menu bar app, hotkey, spotlight box, recents, result box, installer).
- **§6 Core Architecture** — the four components and the flow between them, with the two load-bearing decisions.
- **§7 Technology Stack** — every library and service with its role.
- **§8 Data and Storage** — what lives in Atlas vs S3 and for how long, and why the S3 lifetime is longer than the cache TTL.
- **§9 Database Schema** — exact documents for `jobs`, `cache`, `manim_snippets`; TTL indexes; the `snippets_vector` Vector Search index JSON.
- **§10 Agent Service API** — exact request/response for `/vision`, `/explain`, `/snippets/search`, `/codegen`, `/snippets/ingest`, `/healthz`; model configuration; the determinism decision.
- **§11 Coordinator API** — `POST /api/jobs`, `GET /api/jobs/{id}` with the full status table, `/healthz`, video delivery.
- **§12 Cache Key** — the exact hash, `normalize()`, `PromptVersion`.
- **§13 RAG Design** — why retrieval, the corpus (seed / docs / generated), the pipeline, the rules.
- **§14 Render Pipeline** — fan-out, the three Go function signatures, pre-check, Docker invocation, concat, post-render ingest, storage.
- **§15 Desktop App and Installer** — runtime behavior table (spotlight box, recents, result box) and build/installer artifact table.
- **§16 End-to-End Flow** — install, capture, explain, render, re-ask from recents, degrade.
- **§17 Feature Requirements** — F1–F45, by component, with priority.
- **§18 Non-Functional** — latency, concurrency, isolation targets.
- **§19 Error Handling** — every failure and its behavior.
- **§20 Security** — where secrets live and don't, sandboxing, image handling.
- **§21 MVP vs Stretch** — what must ship.
- **§22 Environment Variables** — complete `.env` templates.
- **§23 Key Implementation Rules** — 21 rules that must never be violated. Read them before any code.
- **§24 Open Questions** — each with an owner and a deadline.

### docs/API.md

The REST API reference for both services — every endpoint with method, path, caller, request, response, and errors; the shared error envelope and code table; the job status lifecycle with its guarantees; the call sequence for one job; limits and timeouts; a curl cookbook; an endpoint index with priorities. Authoritative for HTTP — FRD §10–§11 summarize it.

### docs/SETUP.md

Full environment setup: prerequisites, clone and branch strategy, Anthropic and Voyage API keys, MongoDB Atlas cluster and Vector Search index creation, Docker and the `manim-worker` build with a container smoke test, MinIO, the agent service, the coordinator, the desktop app in development mode with the macOS permission dance, building the `.app`/`.dmg`/`.pkg`, environment files, running everything, a 15-point verification checklist, and common problems.

### docs/WORK_SPLIT.md and the four per-person files

`WORK_SPLIT.md` is the team view: the four roles and why they don't overlap, the four contracts where they meet, the sprint calendar, the sync-point checklists we run together, the **Merge Protocol** (branch naming, rebase-before-sync, merge order P3 → P1 → P2 → P4, rotating captain, tagging), and the dependency graph.

**Your tasks are in your own file.** Each is self-contained — open it and you know exactly what to do:

| Person | File | Role |
|---|---|---|
| P1 | [docs/P1_AI.md](docs/P1_AI.md) | The Python service that makes every model call (Gemini for OCR, Claude for explanation and code) |
| P2 | [docs/P2_RENDER.md](docs/P2_RENDER.md) | Docker sandbox, Manim samples, video pipeline |
| P3 | [docs/P3_BACKEND.md](docs/P3_BACKEND.md) | The Go coordinator: API, database, orchestration; the release |
| P4 | [docs/P4_DESKTOP.md](docs/P4_DESKTOP.md) | Menu-bar app, the two windows, the `.app` and `.dmg` |

Each file has: your job in plain words · what you own and never touch · the interfaces you implement or consume · setup · files to create · every sprint's steps with "done when" checklists · your merge steps · your rules · what to do if blocked.

### FILE_STRUCTURE.md

The complete directory tree with every file's purpose annotated, and the one-path-one-directory rule.

---

## 3. Mandatory Rules

From FRD §23. Violations cause silent cache poisoning, hung jobs, broken concat output, or generated code running on the host.

### Agent service (P1)
1. Every response is schema-enforced JSON (Claude `output_config.format`; Gemini `response_schema`). Never parse prose for JSON. Never use assistant prefill; never pass `temperature` to a Claude model.
2. The `/vision` prompt contains no instruction to interpret, summarize, or contextualize. Verbatim only. This string is hashed.
3. `/codegen` asserts `scene_class == "GeneratedScene"` and that the source contains `class GeneratedScene(Scene)` before returning.
4. Retrieval filters on `verified: true` in every code path. No flag disables it.
5. Index and query use the same `EMBED_MODEL`. Changing it means re-running the seed script against a recreated index.
6. `/vision` is Gemini 3.8 Flash at `temperature=0`. `/explain` is Claude Opus 5. `/codegen` is Claude Sonnet 5 via `client.messages.stream()`.

### Coordinator (P3)
7. `POST /api/jobs` writes the job and returns. Everything else runs in a goroutine with `defer recover()`.
8. `explanation` is written to `jobs` before any `/snippets/search` or `/codegen` call. This is the most important line in the server.
9. Every `jobs` write sets `updated_at = now`, or the TTL index deletes the job mid-render.
10. `cache` is written only after upload succeeds. Never on any failure path.
11. The cache-hit path never calls `/explain`.
12. The manim quality flag is one job-level value passed identically to every scene. Concat depends on it.
13. `PromptVersion` is bumped in the same commit as any prompt change, in any component.

### Render pipeline (P2)
14. The static pre-check runs before every `docker run`, including repair attempts.
15. Every `docker run` has `--network none`, `--memory`, `--cpus`, and a context timeout that kills the container.
16. A scene failure never propagates to sibling scenes. A scene that exhausts repair is dropped; the job continues.

### Desktop app (P4)
17. No secrets in the app. Only `server_url` and preferences.
18. Poll every 1 s, give up at 180 s, render `explanation` on the first non-null poll regardless of status.
19. Every network error becomes a notification or a message in the result box. Never an unhandled exception.
20. `ui/` loads nothing from the network except `video_url`. `marked.min.js` is bundled.
21. A recent is written to disk before `POST /api/jobs` and updated on every poll that adds data. Reopening a recent never depends on the server.

---

## 4. Operating Protocol

### Step 1 — Read before you write
Every task in WORK_SPLIT.md carries an FRD reference. Open it and read the section in full. The FRD has the exact JSON, the exact index definition, the exact flags. Do not improvise where it has an answer.

### Step 2 — Plan before you implement
1. **Which files?** List them. Check File Ownership in WORK_SPLIT.md. If a file belongs to another path, coordinate first.
2. **Inputs and outputs?** For an endpoint: request schema, response schema, status codes. For a Go function: signature from FRD §14.1. For the result box: which job fields it reads and which it writes back to the local recent.
3. **Dependencies?** If another role's piece isn't ready, use the fake your own file names (P1_AI.md … P4_DESKTOP.md → "If you're blocked" and the sprint steps). Do not wait.
4. **Failure modes?** FRD §19 lists them. Handle every one that applies.
5. **Verification?** Define the check before writing. The sync-point checklist is the minimum.

### Step 3 — Implement incrementally
Small, testable steps. Verify each before the next. The render pipeline and the permission prompts in particular punish 500-line first drafts.

### Step 4 — Inspect before pushing
- Shapes match the FRD exactly (field names, status strings, index dims).
- No rule in §3 is violated.
- Secrets are in `.env`, gitignored, and not in the desktop app.
- The branch is rebased on `origin/main` and your own tests pass.

### Step 5 — Merge only at the sync point
Follow WORK_SPLIT.md → Merge Protocol. Order P3 → P1 → P2 → P4. The captain tags. Then everyone branches fresh from `main`.

---

## 5. Sprint and Task Reference

Every task is specified in `docs/WORK_SPLIT.md`. This table is the index.

| Sprint | Hours | Goal | P1 — Agent | P2 — Render | P3 — Coordinator | P4 — Desktop |
|---|---|---|---|---|---|---|
| **1 Foundation** | 0–3 | Every path runs on fakes; risky assumptions tested | Skeleton; Gemini + Claude + Voyage + Atlas clients; real `/vision` on Gemini; collision experiment | `manim-worker` image + container smoke test, 5 watched seed scenes, `precheck.go` | HTTP skeleton, Atlas job store with TTL indexes, fake-mode worker (P4's stub), cache key + test | Permissions, `rumps` app, hotkey + `screencapture -i` + downscale, transparent-window spike |
| **2 Vertical slice** | 3–7 | Real capture → real explanation in the real result box | `/explain`, seed script + `snippets_vector`, `/snippets/search`, `/snippets/ingest`, determinism decision | `Render()` against Docker with timeout + validation, semaphore, `render_test.go`, corpus to 20 | Agent client, real worker through `/explain` with immediate explanation write, cache hit path, fan-out skeleton | Spotlight box (translucent, frameless), result box with markdown + progress, real submit + notification, point at real coordinator |
| **3 Real render** | 7–11 | One capture → real video, every codegen prompt grounded in retrieved snippets | `/codegen` with snippets, repair path, retrieval ablation | `RenderWithRepair`, `Concat` with mismatch guard, S3 upload | Real `SceneFunc`, concat → upload → cache → done, post-render ingest, real `/healthz` | Video playback, recents store + recents list in the spotlight box, reopen from local data, all failure states, guardrails toggle wired |
| **4 Cache, guardrails, installer** | 11–15 | Survives real use; someone else can install it | Guardrails pass rate ≥ 8/10, promote/delete generated snippets, prompt tuning + `PromptVersion` | Cleanup on every exit path, timeout kills containers, `-qm` path, seeds for reported error classes | Two-machine cache test, failure injection, `503` on overload, delete fake flags, `.pkg` + LaunchAgent | Self-signed cert, `build_app.sh`, `build_dmg.sh`, install on a second Mac, permission persists across rebuild |
| **5 Freeze + release** | 15–19 | Everything together; tagged release with installer | Freeze prompts, pre-warm cache | `-qm` release, clean-machine image build | All-green `/healthz` from fresh boot, release notes, GitHub Release `v0.1.0` with DMG/PKG | Final `.app`/`.dmg` hand-off, install from the release URL |

Full steps for each cell are in your per-person file (`docs/P1_AI.md` … `docs/P4_DESKTOP.md`). Sync-point checklists are in `docs/WORK_SPLIT.md`.

---

## 6. Quick Navigation

| I need to... | Read... |
|---|---|
| Understand what this is | §1 above; FRD §1–6 |
| Find where a file goes | FILE_STRUCTURE.md |
| Look up any endpoint — method, request, response, errors | API.md §2 (coordinator), §3 (agent) |
| Look up an error code or the error envelope | API.md §4 |
| Look up the job status lifecycle | API.md §5 |
| Look up a limit or timeout | API.md §7 |
| See the shapes in architectural context | FRD §10–§11 |
| Look up an Atlas document or index | FRD §9 |
| Understand the cache key | FRD §12 |
| Understand retrieval and the corpus | FRD §13 |
| Look up the Go render signatures or the Docker command | FRD §14 |
| Look up spotlight box, result box, recents, or installer behavior | FRD §15; FRD §16.5 for re-ask |
| Check a rule before committing | §3 above; FRD §23 |
| Find what happens on a failure | FRD §19 |
| Find an environment variable | FRD §22; SETUP §12 |
| Set up a service or key | SETUP §3–7 |
| Grant macOS permissions or build the installer | SETUP §10–11 |
| Find my tasks this sprint | Your file: `docs/P1_AI.md` / `P2_RENDER.md` / `P3_BACKEND.md` / `P4_DESKTOP.md` → Sprint N |
| Merge at a sync point | WORK_SPLIT.md → Merge Protocol |
| Find who owns a file | WORK_SPLIT.md → Roles; FILE_STRUCTURE.md |
| See what blocks whom | WORK_SPLIT.md → Dependency Graph |
| See an unresolved decision | FRD §24 |

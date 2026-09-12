# Clarity — Work Split

**Engineers:** 4 (P1, P2, P3, P4)
**Structure:** 5 sprints. Everyone works on their own branch until the end of a sprint, then everyone's work is merged together at the sync point.

This file is the **team-level** view: who does what, how the four pieces meet, how we merge, and the checklist we run together at each sync point. **Your own tasks are in your own file** — open it and you'll know exactly what to do:

| Person | Role | Your file |
|---|---|---|
| **P1** | AI — the Python service that makes every model call (Gemini OCR, Claude explanation + code) | **[P1_AI.md](P1_AI.md)** |
| **P2** | Render — Docker sandbox, Manim samples, video pipeline | **[P2_RENDER.md](P2_RENDER.md)** |
| **P3** | Backend — the Go coordinator: API, database, orchestration; plus the release (PKG, GitHub Release) | **[P3_BACKEND.md](P3_BACKEND.md)** |
| **P4** | Desktop — the menu-bar app, the two windows, the `.app` and `.dmg` | **[P4_DESKTOP.md](P4_DESKTOP.md)** |

Each file is self-contained: your job in plain words, what you own and never touch, the interfaces you implement or consume, setup steps, files to create, every sprint's steps with "done when" checklists, your merge steps, your rules, and what to do if you're blocked.

---

## Roles, and why there's no overlap

| Person | Owns | Never touches |
|---|---|---|
| P1 | `agent/**` | Go, Docker, desktop UI |
| P2 | `docker/**`, `samples/**`, `server/internal/render/**` | HTTP handlers, job store, prompts, desktop UI |
| P3 | `server/**` except `internal/render/`, `release/**` | Prompts, Dockerfile, render internals, desktop UI |
| P4 | `desktop/**` | Anything server-side, `release/` |

Five directories, four people. The only shared surfaces are five contracts, each written down once:

| Contract | Written in | Implemented by | Consumed by |
|---|---|---|---|
| Agent HTTP API | [API.md §3](API.md#3-agent-service-api) | P1 | P3 |
| Render Go functions (`Render`, `RenderWithRepair`, `Concat`) | [FRD §14.1](FRD.md#14-render-pipeline) | P2 | P3 |
| Coordinator HTTP API | [API.md §2](API.md#2-coordinator-api) | P3 | P4 |
| `samples/*.py` docstring header | [FRD §13](FRD.md#13-rag-design--the-manim-snippet-corpus) | P2 | P1 |
| `dist/Clarity.app` + `dist/Clarity.dmg` (built artifacts) | [FRD §15.2](FRD.md#15-desktop-app-and-installer) | P4 | P3 packages and publishes |

Changing any of these is a spec change: edit the FRD/API doc first, own commit, straight to `main`, tell the team. Nobody is ever blocked on a contract — every role fakes the other side until it's real (each person's file says exactly what to fake).

---

## Workload balance

Every role is budgeted to the same total. If your sprint comes in under budget, pull from the **Overflow backlog** below — those items are deliberately unassigned so whoever is free takes them.

| Sprint | P1 — AI | P2 — Render | P3 — Backend | P4 — Desktop |
|---|---|---|---|---|
| 1 | 3.0 h | 3.0 h | 3.25 h | 2.5 h |
| 2 | 4.75 h | 4.75 h | 3.5 h | 5.0 h |
| 3 | 3.25 h | 3.75 h | 3.25 h | 4.5 h |
| 4 | 3.0 h | 2.5 h | 4.0 h | 2.5 h |
| 5 | 1.0 h | 1.0 h | 1.0 h | 0.5 h |
| **Total** | **15.0 h** | **15.0 h** | **15.0 h** | **15.0 h** |

How the balance was struck: P4 was heaviest (two windows, recents, installer, release) and P3 lightest, so release engineering — the `.pkg`, the LaunchAgent, release notes, `gh release create` — moved to P3 as a new `release/` directory, and P4's separate stub server was dropped in favour of P3's coordinator in fake mode (which exists anyway). P1 and P2 were already even.

### Overflow backlog

Unassigned. Anyone who finishes a sprint early takes the top item they can do, tells the team, and it's theirs. Each is 1–2 hours and touches only one directory.

| # | Item | Directory | Spec |
|---|---|---|---|
| 1 | `GET /api/jobs/{id}/events` — SSE stream so the result box doesn't poll | `server/` | API.md §2.4 |
| 2 | `ffprobe`-based clip validation beyond the size floor (duration, resolution) | `server/internal/render/` | FRD §17 F40 |
| 3 | 10 more `samples/*.py` covering the error classes seen in repair logs | `samples/` | FRD §13 |
| 4 | Ingest 30 worked examples from the Manim CE docs into the snippet corpus (`origin: "manim_docs"`) | `agent/scripts/` | FRD §13 |
| 5 | Guardrails eval script: 10 problems, pass/fail, pass rate printed | `agent/experiments/` | FRD §24 |
| 6 | Result box: keyboard shortcuts (space play/pause, ← → seek) and a copy-explanation button | `desktop/clarity/ui/result/` | FRD §15.1 |
| 7 | Spotlight box: drag-and-drop an image file to use instead of a capture | `desktop/clarity/ui/spotlight/` | FRD §5 |
| 8 | `docker/README.md` + `samples/README.md` polish; a `make smoke` target that runs the container smoke test | `docker/`, `samples/` | SETUP §6 |
| 9 | Coordinator structured logging with `request_id` and `job_id` on every line | `server/` | API.md §1 |
| 10 | Menu bar icon reflects state (idle / working / done) with the assets in `desktop/assets/` | `desktop/clarity/app.py` | FRD §5 |

---

## Sprint calendar

| Sprint | Hours | Team goal | You know it worked when |
|---|---|---|---|
| **1 — Foundation** | 0–3 | Every role runs end-to-end *on fakes*; the two riskiest assumptions are tested | LaTeX renders inside the container; we know how many of 6 screenshots hash the same |
| **2 — Vertical slice** | 3–7 | A real screenshot produces a real explanation in the real result box | Explanation visible in < 10 s. **This is the product.** |
| **3 — Real render** | 7–11 | One capture → real video, every scene grounded in retrieved snippets | Video plays in the result box, all real |
| **4 — Cache, guardrails, installer** | 11–15 | Survives real use; someone else can install it | Second laptop hits the cache; a stranger installs the `.dmg` |
| **5 — Freeze + release** | 15–19 | Everything together, tagged release with the installer | `v0.1.0` on GitHub; install from the release URL works |

---

## Sync-point checklists

Run together at the end of each sprint, after the merge. Each line names who demonstrates it.

### Sprint 1
1. **P2** — `docker run manim-worker` renders the `MathTex` smoke test to MP4. Five samples rendered and watched.
2. **P1** — `/healthz` shows `gemini`, `anthropic`, `voyage`, `atlas` all `true`. One real `/vision` call returns verbatim text. Collision number: **N of 6**.
3. **P3** — `POST /api/jobs` → job ID; polling walks through every status; the job is visible in Atlas with TTL indexes.
4. **P4** — `python -m clarity --once` → crosshair → PNG under 8 MB. Transparency spike result recorded.
4b. **P3** — `FAKE_AGENT=1 FAKE_RENDER=1` server walks a job through every status with no Atlas/Docker/keys — P4 has a server.
5. **Captain (P3)** merges, tags `sprint-1`.

### Sprint 2
1. **P4 + P3** — `⌘⇧E` → drag → spotlight box → Enter → **real explanation in the result box in under 10 s.**
2. **P1** — `snippets_verified ≥ 20`; `/snippets/search` ranks a plotting query and an array-walk query correctly. Determinism decision recorded.
3. **P2** — `Render()` renders every sample through a real container; `go test ./internal/render/` passes; timeout kills the container.
4. **P3** — Same capture twice → second is `cached: true` with explanation and video.
5. **Captain (P1)** merges, tags `sprint-2`.

### Sprint 3
1. **All** — One capture → explanation → **real video playing in the result box.** No fakes anywhere.
2. **P1 + P3** — Every codegen call in that job included ≥ 1 retrieved snippet (logs). Ablation numbers recorded.
3. **P2 + P3** — Inject a bad import into one scene → that scene drops, the other scenes' video still plays.
4. **P4** — With the server stopped, ↓ in the spotlight box lists a recent and Enter reopens its result from local data. X before `done` → P3 sees `cancelled`.
5. **Captain (P2)** merges, tags `sprint-3`.

### Sprint 4
1. **P3** — Two laptops, same problem, second is `cached: true` in < 1 s.
2. **P1** — Guardrails pass rate ≥ 8/10. No un-reviewed generated snippets.
3. **P3** — Agent killed mid-job → job ends cleanly, result box shows the explanation.
4. **P4** — `Clarity.dmg` installed on a Mac that never ran from source; `⌘⇧E` works there; rebuild did not re-prompt for Screen Recording.
4b. **P3** — `Clarity.pkg` installs; Clarity is in the menu bar after a fresh login. Fake flags deleted.
5. **P2** — `docker ps -a` empty after a batch including a timeout; `-qm` path works.
6. **Captain (P4)** merges, tags `sprint-4`.

### Sprint 5
1. **All** — Fresh clone + `SETUP.md` → full stack running, by someone following the doc.
2. **P3** — GitHub Release `v0.1.0` with `Clarity.dmg` and `Clarity.pkg` attached, release notes complete.
2b. **P4** — Installing from the release URL works on a second Mac.
3. **P1 + P3** — Cache pre-warmed; a cold problem still works end to end.
4. **Captain (P3)** tags `v0.1.0`.

---

## Merge Protocol

Everyone works until the end of the sprint on their own branch, then everyone's work is merged together. The whole procedure:

1. **Branch naming:** `p1/sprint-N-short-description`, `p2/…`, `p3/…`, `p4/…`.
2. **Before the sync point**, each person: `git fetch origin && git rebase origin/main` on their branch, fix conflicts locally, run their own tests, push.
3. **At the sync point**, the merge captain merges into `main` **in this order: P3 → P1 → P2 → P4.** P3 defines the shapes everyone consumes, so it lands first; P4 consumes everything, so it lands last.
4. After each merge the captain runs that person's checklist lines. A failure stops the merge; the owner fixes on their branch; retry.
5. Captain tags `sprint-N` and pushes.
6. Everyone: `git checkout main && git pull && git checkout -b pN/sprint-(N+1)-<what>`.

**Merge captain rotates:** Sprint 1 → P3 · Sprint 2 → P1 · Sprint 3 → P2 · Sprint 4 → P4 · Sprint 5 → P3.

**Between sync points, nothing goes to `main`** except a change to `docs/FRD.md` or `docs/API.md`, which goes there immediately in its own commit and is announced.

---

## Dependency graph

| Sprint | Who needs whom | If it's late |
|---|---|---|
| 1 | Nobody needs anybody. | — |
| 2 | P3 needs P1's real `/vision` + `/explain`. P1 needs P2's first samples to seed. P4 needs P3's server (fake mode is enough until the last step). | P3 keeps `FAKE_AGENT=1`; P1 seeds from the smoke-test scene; P4 stays on fake mode. |
| 3 | P3 needs P2's three functions and P1's `/codegen`. P2 needs P1's `/codegen` to test repair for real. | P3 keeps `FAKE_RENDER=1`; P2 tests repair with a fake codegen (broken sample, then good). |
| 4 | P1's snippet review needs P2 to render clips. P3's two-machine test needs a second laptop. P3's PKG needs P4's `.app` (P4 Sprint 4 Step 2, early in the sprint). | P3 builds the PKG against a `.app` built with the SETUP §11.2 command on their own machine. |
| 5 | Everyone needs `main` green. P3's publish needs P4's final `.dmg`. | Fix forward; nobody branches. |

# Clarity — Work Split

**Engineers:** 4 (P1, P2, P3, P4)
**Structure:** 5 sprints. Everyone works on their own branch until the end of a sprint, then everyone's work is merged together at the sync point.

This file is the **team-level** view: who does what, how the four pieces meet, how we merge, and the checklist we run together at each sync point. **Your own tasks are in your own file** — open it and you'll know exactly what to do:

| Person | Role | Your file |
|---|---|---|
| **P1** | AI — the Python service that makes every model call (Gemini OCR, Claude explanation + code) | **[P1_AI.md](P1_AI.md)** |
| **P2** | Render — Docker sandbox, Manim samples, video pipeline | **[P2_RENDER.md](P2_RENDER.md)** |
| **P3** | Backend — the Go coordinator: API, database, orchestration | **[P3_BACKEND.md](P3_BACKEND.md)** |
| **P4** | Desktop — the menu-bar app, the two windows, the installer | **[P4_DESKTOP.md](P4_DESKTOP.md)** |

Each file is self-contained: your job in plain words, what you own and never touch, the interfaces you implement or consume, setup steps, files to create, every sprint's steps with "done when" checklists, your merge steps, your rules, and what to do if you're blocked.

---

## Roles, and why there's no overlap

| Person | Owns | Never touches |
|---|---|---|
| P1 | `agent/**` | Go, Docker, desktop UI |
| P2 | `docker/**`, `samples/**`, `server/internal/render/**` | HTTP handlers, job store, prompts, desktop UI |
| P3 | `server/**` except `internal/render/` | Prompts, Dockerfile, render internals, desktop UI |
| P4 | `desktop/**` | Anything server-side |

Four directories, four people. The only shared surfaces are four contracts, each written down once:

| Contract | Written in | Implemented by | Consumed by |
|---|---|---|---|
| Agent HTTP API | [API.md §3](API.md#3-agent-service-api) | P1 | P3 |
| Render Go functions (`Render`, `RenderWithRepair`, `Concat`) | [FRD §14.1](FRD.md#14-render-pipeline) | P2 | P3 |
| Coordinator HTTP API | [API.md §2](API.md#2-coordinator-api) | P3 | P4 |
| `samples/*.py` docstring header | [FRD §13](FRD.md#13-rag-design--the-manim-snippet-corpus) | P2 | P1 |

Changing any of these is a spec change: edit the FRD/API doc first, own commit, straight to `main`, tell the team. Nobody is ever blocked on a contract — every role fakes the other side until it's real (each person's file says exactly what to fake).

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
5. **P2** — `docker ps -a` empty after a batch including a timeout; `-qm` path works.
6. **Captain (P4)** merges, tags `sprint-4`.

### Sprint 5
1. **All** — Fresh clone + `SETUP.md` → full stack running, by someone following the doc.
2. **P4** — GitHub Release `v0.1.0` with `Clarity.dmg`; installing from it works.
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
| 2 | P3 needs P1's real `/vision` + `/explain`. P1 needs P2's first samples to seed. P4 needs P3's real server for the last step. | P3 keeps `FAKE_AGENT=1`; P1 seeds from the smoke-test scene; P4 stays on the stub. |
| 3 | P3 needs P2's three functions and P1's `/codegen`. P2 needs P1's `/codegen` to test repair for real. | P3 keeps `FAKE_RENDER=1`; P2 tests repair with a fake codegen (broken sample, then good). |
| 4 | P1's snippet review needs P2 to render clips. P3's two-machine test needs a second laptop. P4 needs nothing. | — |
| 5 | Everyone needs `main` green. | Fix forward; nobody branches. |

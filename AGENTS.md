# Ambient visual learning tool — HackCMU

**Start here.** This is the index and the clock. Everything else is linked from
here.

Four people, four laptops, one repo, demo **Saturday 4:00 PM**.

## What we're building

A student hits a hotkey while looking at a hard math or algorithm problem in
their browser. A side panel opens, they can optionally type a question and
optionally flip on **guardrails mode**, and they get back two things: a written
step-by-step explanation within seconds, and a custom animated video explaining
the same problem about a minute later.

The animations are generated per-problem by Claude writing Manim code — not
pulled from a library of pre-made videos — and rendered as several short scenes
in parallel, then joined into one clip.

There's also an Angular web app where a user can paste a screenshot, for
problems that aren't in a browser — code in an IDE, a PDF on the desktop.

**Guardrails mode** is the answer to "isn't this just a cheating tool?" — with
it on, the explanation and the animation teach the method and stop short of the
final answer, instead of solving the problem for the student.

## The two ideas that shape everything

Rendering an animation is slow and CPU-bound — even split across parallel
scene containers, real seconds per scene versus ~8 seconds for the text
explanation. Two consequences:

**The text is the product; the video is the reward.** The explanation reaches
the user the moment it exists, long before the video. A UI that waits for the
video makes a working system feel broken.

**Rendering is the thing to avoid doing twice.** Students at one university work
the same problem sets. If four people screenshot the same problem, the first
waits and the rest get it instantly. That's what the cache is for, and it's why
Redis is in the stack.

## How it's split

```
Angular extension panel  ──HTTP──►  Go coordinator  ──HTTP──►  Python agent service ──► Claude API
Angular web app (paste)             :8080                      :8000
                                       │
                                       ├──► Redis              (job status + cache)
                                       ├──► Docker render pool (one container per scene)
                                       ├──► ffmpeg             (concat, no re-encode)
                                       └──► S3                 (finished MP4s)
```

- **Python** owns every Claude call — vision, explanation, Manim-doc
  retrieval, code generation.
- **Go** owns orchestration — the job queue, fanning a storyboard's scenes out
  to run concurrently, the cache, Docker dispatch, the repair loop, ffmpeg,
  and S3.
- **Angular** for both the extension side panel and the paste-a-screenshot web
  app, sharing one component/service library so there's one UI to build, not
  two.

## How one request flows

1. Hotkey fires (or a screenshot is pasted). The client sends image + question
   + guardrails flag to Go.
2. Go creates a job, returns a job ID **immediately**, and does everything
   below in a goroutine.
3. **Claude call — vision.** Python reads the screenshot, returns the problem
   text verbatim plus a category.
4. Go hashes the text + question + guardrails flag, checks Redis. **On a
   hit**, returns the stored video *and* the stored explanation, and stops.
   Under a second, zero API calls.
5. **Claude call — explainer.** Returns the written explanation plus a
   storyboard of 2–5 scenes. Go writes the explanation to Redis right here.
   **This is when the user stops waiting.**
6. **Per scene, concurrently:** Python retrieves relevant Manim-doc snippets,
   then turns the scene into Manim source. Go runs it in its own Docker
   container. On failure, the traceback goes back to Claude for repair — up to
   3 attempts, **for that scene only**.
7. Once every scene has finished (rendered or given up), Go concatenates the
   successful clips with ffmpeg — no re-encoding, because every scene was
   rendered at the same resolution and frame rate — and uploads the result to
   S3.
8. The video URL is written to Redis under both job ID and problem hash. The
   client's next poll picks it up.

If every scene fails, the job is `done`-without-a-video — **but the
explanation is preserved and shown**, with a note that the animation didn't
render. **Never fail the explanation because rendering failed.**

## The four lanes

| Lane | Owner | Owns | Brief |
|---|---|---|---|
| `agent/` | TBD | Python: all Claude calls, Manim-doc corpus + retrieval | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#agent--python-all-claude-calls--manim-doc-retrieval) |
| `render/` | TBD | Go: Docker dispatch, per-scene repair, ffmpeg, S3 | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#render--go-docker-dispatch-repair-loop-ffmpeg-s3) |
| `server/` | TBD | Go: HTTP API, Redis, job + scene orchestration | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#server--go-http-api-redis-job--scene-orchestration) |
| `client/` | TBD | Angular: extension panel, web app, shared UI lib | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#client--angular-extension-panel--web-app) |

Claim a lane by putting your name in that table and in
[docs/WORK_SPLIT.md](docs/WORK_SPLIT.md).

The shapes everyone builds against are in [CONTRACTS.md](CONTRACTS.md). Read it
before writing code. If you need it to change, change it there first and tell
everyone.

**The rule that makes four-way parallelism work: every lane fakes its
dependencies from hour zero.** Nobody waits for anybody. Each brief in
[docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) says exactly what that lane fakes and
when it stops faking it.

---

# The clock

Five sprints, Friday 8 PM → Saturday 4 PM. Everything counts backward from the
demo. **This is a bigger build than a single-scene, local-disk MVP** — Docker,
S3, multi-scene concat, and doc retrieval are all new since the first draft.
If any of it is slipping, [see the cut order](#if-you-fall-behind) — it's
ordered so the earliest cuts are the ones this rewrite added, landing you back
on a design that's already known to work in a weekend.

| # | When | Goal | You know it worked when |
|---|---|---|---|
| 1 | Fri 8 PM – 11 PM | **De-risk** | LaTeX renders inside the `manim-worker` Docker image, MinIO is reachable, six-screenshot hash count is known, Angular scaffold loads as an unpacked extension. |
| 2 | Fri 11 PM – 3 AM | **Vertical slice** | Hotkey → real explanation in the panel in < 10s, guardrails flag flows end-to-end, one hardcoded scene renders through a real Docker container. |
| 3 | Sat 3 AM – 7 AM | **Real per-scene pipeline** | One multi-scene problem goes hotkey → explanation → N scenes rendered in parallel → concatenated → uploaded to S3 → played, all real. |
| 4 | Sat 7 AM – 11 AM | **Cache, failure, guardrails, web app** | Two laptops, same problem, second one instant — video *and* explanation. A forced scene failure still plays the rest. |
| 5 | Sat 11 AM – 4 PM | **Freeze and demo** | You've said the demo out loud, on the presenting machine, without it breaking. |

## Sprint 1 — De-risk · Fri 8:00 PM → 11:00 PM

The goal is not features. It's finding out which of our assumptions is false
while there's still time to route around it.

| Lane | Do this |
|---|---|
| `agent/` | `pip install anthropic`, auth working, one vision call against a real screenshot returning real JSON. Start assembling the Manim-doc corpus (official docs' worked examples + `samples/`) — it just needs to exist as text files for now, retrieval comes in Sprint 3. |
| `render/` | **Build the `manim-worker` Docker image first — before anything else.** Python + Manim CE + LaTeX + `dvisvgm` + ffmpeg baked in. Verify `MathTex` renders to MP4 **inside a container**, not on the host — a host-only check tells you nothing about the image. |
| `server/` | Go HTTP server up. `POST /api/jobs` returns a job ID. `GET /api/jobs/{id}` returns a hardcoded `done` job with a fake explanation and a fake video URL. Redis running. MinIO running locally as the S3 stand-in; one `PutObject`/`GetObject` smoke test against it. |
| `client/` | Angular workspace scaffolded: extension-panel project, web-app project, shared library. Extension loads unpacked from the built output, hotkey fires, `captureVisibleTab` produces a PNG, panel opens. |

**Also in this sprint, and it's on the critical path:** somebody writes
`samples/product_rule_scenes.py` — a Manim scene that actually renders inside
`manim-worker`, using relative positioning only. It's the codegen cheatsheet
seed, the seed of the Manim-doc corpus, and the render lane's test fixture. It
does not exist yet. `render/` owns it.

**Also:** the cache-hit experiment. Screenshot one problem six ways (two zoom
levels × three window widths), run each through `/vision`, count distinct
`problem_text` strings. `agent/` owns it. If they're all different, we learn now
that Redis is decoration and we stop building around it.

**Checkpoint 11:00 PM:** every lane runs end-to-end *within itself*, on fakes.
Nothing is integrated. Report: does LaTeX work *inside the container*, how many
distinct hashes out of six, does the Angular extension load and capture, can Go
put/get an object in MinIO.

## Sprint 2 — Vertical slice on fakes · Fri 11:00 PM → 3:00 AM

Real code in every lane, still fake at every boundary. Ends with a real text
explanation reaching a real panel, and one scene making it through a real
container end-to-end.

| Lane | Do this |
|---|---|
| `agent/` | `/vision` and `/explain` real, with structured outputs — `/explain` now returns the multi-scene storyboard shape. `/manim-docs` and `/codegen` can return a hardcoded scene for now. Guardrails flag threaded through `/explain`'s prompt, even if the difference is rough. **Decide the determinism question** (CONTRACTS §1 OPEN) using the Sprint 1 numbers. |
| `render/` | `Render()` runs a hardcoded scene (render/'s own fixture, not yet from `agent/`) inside `manim-worker` via `docker run`, returns a path or a `RenderError`. Static pre-check (AST parse, banned imports) runs before the container starts. Per-scene timeout wired. |
| `server/` | Real job queue + goroutine per job. Calls `/vision` then `/explain`, writes both to Redis. Cache key implemented with `guardrails` in it. Scene fan-out plumbing exists even if `scenes_total` is hardcoded to 1. Still fakes `/manim-docs`, `/codegen`, and hooks `render/`'s container path with its stub scene. |
| `client/` | Polls for real. Renders markdown explanation the instant it's non-null. Guardrails toggle in the UI, sent on `POST /api/jobs`. Progress indicator shows `scenes_done`/`scenes_total` (even if always `0/1` for now). Handles `failed`-with-explanation. |

**Checkpoint 3:00 AM — this is the important one.** Hotkey → screenshot → real
explanation in the panel in under ~10 seconds, guardrails flag visibly
reaching the server, and one scene rendering through a real Docker container.
**If the explanation path works, we have a demo**, and everything after is
upside.

## Sprint 3 — Real per-scene pipeline · Sat 3:00 AM → 7:00 AM

Overnight. Stagger sleep — at least two people awake, and not two from the same
lane if you can help it.

| Lane | Do this |
|---|---|
| `agent/` | `/manim-docs` is real: embed the corpus once at startup, retrieve top-k snippets by similarity to a scene's `visual` text (or keyword match if the embedding model isn't installed in time — worse recall, zero setup risk). `/codegen` is a real per-scene call, grounded in the retrieved snippets, with the hard constraints from CONTRACTS §1. Repair prompt takes a scene's source + traceback. |
| `render/` | `RenderWithRepair` real, 3 attempts, per scene. ffmpeg concat wired: `-f concat -c copy`, guarded by every scene using the same quality flag. Real upload to S3 (MinIO locally). |
| `server/` | Fans a storyboard's scenes out to N goroutines, bounded by `RENDER_CONCURRENCY`, waits for all of them (success or exhausted repair), calls `render/`'s concat + upload, writes `video_url` to Redis under **both** job ID and problem hash, alongside the cached explanation. |
| `client/` | Video appears without a page reload when polling flips to `done`. Progress indicator shows real `scenes_done`/`scenes_total`. |

**Checkpoint 7:00 AM:** one multi-scene problem goes hotkey → explanation → N
scenes rendered in parallel containers → concatenated → uploaded to S3 →
played, all real. Note how often codegen needs repair, and whether repair
actually recovers.

## Sprint 4 — Cache, failure, guardrails, web app · Sat 7:00 AM → 11:00 AM

Everything that makes it survive contact with a judge.

| Lane | Do this |
|---|---|
| `agent/` | Guardrails prompt tuning on real Sprint 3 output — spot-check that it actually withholds the final answer; it's a prompt instruction, not a filter, so it needs eyes on it. Storyboard bias toward what text can't do. **Bump `PromptVersion` every time you change a prompt.** |
| `render/` | Harden: temp dir and container cleanup on both success and failure, concurrency cap tuned so N scenes don't eat every core, partial-success path verified — if one scene of four exhausts repair, concat the other three and note the gap. |
| `server/` | Cache hit path verified end-to-end: second identical request (same problem, same question, same guardrails flag) returns video *and* explanation together in under a second with `cached: true`. Failure path verified: all scenes fail → status `done` with `video_url: null`, explanation **preserved and returned**. S3 lifecycle rule (14d expiry) confirmed configured, sitting behind Redis's 7d cache TTL. |
| `client/` | Angular web app (paste/drop) built and wired to the same API, sharing the panel component from the shared library. Guardrails toggle present on both surfaces. Empty/error/failed states actually look deliberate. |

**Checkpoint 11:00 AM:** two people screenshot the same problem; the second
gets it instantly, with the explanation showing immediately too. Force one
scene to fail and confirm the rest of the video still plays. Turn on
guardrails on a real problem and confirm the final answer is actually withheld.

## Sprint 5 — Freeze, integrate, demo · Sat 11:00 AM → 4:00 PM

| Time | What |
|---|---|
| 11:00 AM – 1:00 PM | Everything running on one machine at once. If presenting with real AWS instead of MinIO, swap `S3_ENDPOINT` now and re-verify. Fix only what's broken end-to-end. |
| 1:00 PM – 2:00 PM | **Pre-warm the cache** with 3–4 problems you'll actually demo — rendered, uploaded, cached, verified playing from S3. This is the single highest-value hour on the clock. |
| **2:00 PM** | **Code freeze.** No new features. Bug fixes only, and only for bugs on the demo path. |
| 2:00 PM – 3:00 PM | Run the demo start to finish, three times, on the machine and network you'll present from. Write down what you say. |
| 3:00 PM – 4:00 PM | README, slides, buffer. Nobody touches code. |

---

# The demo

## Script

1. Open a real problem. Hit the hotkey. Explanation appears in seconds — **say
   out loud that the video is still rendering, in parallel, scene by scene.**
   That's the design, not an apology.
2. Video appears. Play it.
3. Second laptop, same problem, different zoom level → instant, explanation and
   video together. That's the cache, and it's the part that's actually hard.
4. Flip on guardrails, try the same problem. Show that the explanation walks
   the method and stops short of the answer.
5. Have a pre-rendered fallback video on disk. Conference wifi has killed
   better demos than ours.

## Questions the judges will ask

- **"Is this a cheating tool?"** Point at guardrails mode. That's the actual
  answer, not a disclaimer — the storyboard and explanation are built to teach
  the method, and there's a toggle that enforces it.
- **"Why Go and Python both?"** Python has the SDK and the agent ecosystem; Go
  gives explicit concurrency control over the expensive, CPU-bound part —
  including fanning a storyboard's scenes out to render in parallel. The split
  follows the workload.
- **"Why isolate rendering in Docker?"** Generated code runs. A container with
  no network access and a memory/CPU cap means a bad scene can't do anything
  worse than waste its own container's time.
- **"Does the video beat the text?"** Only if it shows what text can't: a
  function and its derivative plotted together, a pointer walking a list, a
  shape transforming. If a scene is just animated algebra, the text already
  did it better. Demo a problem where motion carries meaning.

## If you fall behind

Cut in this order. Each line is safe to lose without the demo dying, and the
list is ordered so the earliest cuts are the complexity this rewrite added —
falling all the way down this list lands you back on the original,
already-proven single-scene design:

1. **Guardrails mode.** Ship answer-mode only.
2. **Manim-doc retrieval.** Fall back to embedding the static
   `samples/product_rule_scenes.py` cheatsheet directly in the codegen prompt —
   no `/manim-docs` call.
3. **Docker isolation.** Fall back to a bare `exec.Command` in `render/`, with
   the static pre-check as the only safety net. Same render logic, no
   container overhead.
4. **S3.** Fall back to local disk under `./renders/`, served directly by Go —
   this was the original design and it works fine for one demo laptop.
5. **Multi-scene concurrent rendering.** Fall back to one scene per problem, no
   ffmpeg concat — the original single-scene design.
6. The Angular paste-a-screenshot web app (extension alone demos fine).
7. The repair loop (fail straight to explanation-only).
8. The cache (slower, still works — but you lose the best 20 seconds of the
   demo).

**Never cut:** hotkey → screenshot → text explanation. That's the product. The
video is the reward.

---

## Still open

Written down so nobody re-decides them at 3 AM. Each one has an owner.

| Question | Owner | By when |
|---|---|---|
| **How do we make transcription deterministic?** `temperature=0` isn't available on Claude Opus 5 (sampling params return a 400). Options in [CONTRACTS §1](CONTRACTS.md#1-python-agent-service-internal). | `agent/` | End of Sprint 2 |
| **Does the cache actually hit?** Unmeasured — that's what the six-screenshot experiment is for. | `agent/` | Sprint 1 |
| **Embeddings model for Manim-doc retrieval.** Local CPU model preferred (zero added latency, zero API cost); keyword match is the fallback if there's no time to install one. | `agent/` | Sprint 3 |
| **Does guardrails mode actually withhold the answer?** It's a prompt instruction, not an enforced filter — needs a human spot-check on real problems before the demo, not just a code review. | `agent/` | Sprint 4 |
| **Public-read S3 vs. presigned URLs.** Defaulting to public-read on the `renders/` prefix for simplicity; presigned URLs are a one-line upgrade if privacy becomes a concern before Saturday. | `render/` | Sprint 4 |
| **Privacy.** The screenshot is the whole viewport — tabs, browser chrome, possibly the student's name — and now it lives on S3, not just local disk. For the demo we say so out loud; anything real needs a crop step and a real retention policy, not just a 14-day lifecycle rule. | everyone | Before Saturday |

## The docs

| File | What it's for | Read it when |
|---|---|---|
| [CONTRACTS.md](CONTRACTS.md) | Every JSON shape and HTTP route at every boundary, plus the render pipeline internals (Docker, ffmpeg, S3). Draft — change it there first, in its own commit | Before writing any code that crosses a lane boundary |
| [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) | Four lanes: what each owns, exposes, and **fakes**; what "done" looks like; traps | When you claim a lane |
| [docs/SETUP.md](docs/SETUP.md) | Install Python, Go, Redis, Docker, Manim, **LaTeX**, MinIO, Angular CLI, load the extension, run everything | First thing, on every laptop |
| [docs/FRD.md](docs/FRD.md) | Functional requirements, numbered, Must/Should/Could; non-functional targets; what's out of scope | When deciding whether something is worth building |
| [FILESTRUCTURE.md](FILESTRUCTURE.md) | Where every file goes, and the one-lane-one-directory rule | Before creating a file |

## Working together

- Everyone pushes to `main`. Pull first. Small commits.
- One lane, one directory. Don't touch another lane's files without saying so.
- `CONTRACTS.md` changes go in their own commit, announced in chat.
- Blocked on someone? You're not faking hard enough. Fake harder, keep moving,
  tell them what shape you assumed.

# Ambient visual learning tool — HackCMU

**Start here.** This is the index and the clock. Everything else is linked from
here.

Four people, four laptops, one repo, demo **Saturday 4:00 PM**.

## What we're building

A student hits a hotkey while looking at a hard math or algorithm problem in
their browser. A side panel opens, they can optionally type a question, and they
get back two things: a written step-by-step explanation within seconds, and a
custom animated video explaining the same problem about a minute later.

The animations are generated per-problem by Claude writing Manim code — not
pulled from a library of pre-made videos.

There's also a plain web page where a user can paste a screenshot, for problems
that aren't in a browser — code in an IDE, a PDF on the desktop.

## The two ideas that shape everything

Rendering an animation is slow and CPU-bound — roughly 60–90 seconds cold,
versus ~8 seconds for the text explanation. Two consequences:

**The text is the product; the video is the reward.** The explanation reaches
the user the moment it exists, long before the video. A UI that waits for the
video makes a working system feel broken.

**Rendering is the thing to avoid doing twice.** Students at one university work
the same problem sets. If four people screenshot the same problem, the first
waits and the rest get it instantly. That's what the cache is for, and it's why
Redis is in the stack.

## How it's split

```
Chrome extension ──HTTP──► Go server ──HTTP──► Python agent service ──► Claude API
  (or paste page)          :8080               :8000
                             ├──► Redis            (job status + cache)
                             ├──► manim subprocess (render, Go-orchestrated)
                             └──► ./renders/*.mp4
```

- **Python** owns every Claude call — vision, explanation, code generation.
- **Go** owns the render pipeline and job orchestration — queue, worker pool,
  cache, repair loop, the `manim` subprocess, serving the MP4.
- **Plain HTML/JS** for the extension and the paste page.

## How one request flows

1. Hotkey fires. Extension screenshots the visible tab, sends image + the user's
   question to Go.
2. Go creates a job, returns a job ID **immediately**, and does everything below
   in a goroutine.
3. **Claude call 1 — vision.** Python reads the screenshot, returns the problem
   text verbatim plus a category.
4. Go hashes the text, checks Redis. **On a hit**, returns the stored video and
   stops. Under a second, one API call instead of three.
5. **Claude call 2 — explainer.** Returns the written explanation plus a
   storyboard. Go writes the explanation to Redis right here. **This is when the
   user stops waiting.**
6. **Claude call 3 — codegen.** Turns the storyboard into Manim source.
7. Go writes it to a `.py`, runs `manim`. On failure the traceback goes back to
   Claude for repair, up to three attempts.
8. MP4 is stored, URL written to Redis under both job ID and problem hash. The
   client's next poll picks it up.

If step 6 or 7 fails all three times, the job is `failed` — **but the
explanation is preserved and shown**, with a note that the animation didn't
render. Five lines of code; the difference between a dead demo and a working one.

## The four lanes

| Lane | Owner | Owns | Brief |
|---|---|---|---|
| `agent/` | TBD | Python, all three Claude calls | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#agent--python-all-three-claude-calls) |
| `render/` | TBD | Go, Manim subprocess, repair loop | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#render--go-manim-subprocess-repair-loop) |
| `server/` | TBD | Go, HTTP API, Redis, job queue | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#server--go-http-api-redis-job-queue) |
| `client/` | TBD | Chrome extension, side panel, paste page | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#client--chrome-extension--paste-page) |

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
demo.

| # | When | Goal | You know it worked when |
|---|---|---|---|
| 1 | Fri 8 PM – 11 PM | **De-risk** | Every lane runs end-to-end *on fakes*. We know if LaTeX is broken and how many of six screenshots hash the same. |
| 2 | Fri 11 PM – 3 AM | **Vertical slice** | Hotkey → real explanation in the side panel in < 10s. **If this works, we have a demo.** |
| 3 | Sat 3 AM – 7 AM | **Real render** | One problem goes hotkey → explanation → video, all real. |
| 4 | Sat 7 AM – 11 AM | **Cache, failure, paste page** | Two laptops, same problem, second one instant. |
| 5 | Sat 11 AM – 4 PM | **Freeze and demo** | You've said the demo out loud, on the presenting machine, without it breaking. |

## Sprint 1 — De-risk · Fri 8:00 PM → 11:00 PM

The goal is not features. It's finding out which of our assumptions is false
while there's still time to route around it.

| Lane | Do this |
|---|---|
| `agent/` | `pip install anthropic`, auth working, one vision call against a real screenshot returning real JSON. |
| `render/` | **Verify LaTeX first — before anything else.** `manim` installed, one hand-written scene with `MathTex` renders to MP4. If TeX/`dvisvgm` is missing, everything math-shaped dies here and we need to know now. |
| `server/` | Go HTTP server up. `POST /api/jobs` returns a job ID. `GET /api/jobs/{id}` returns a hardcoded `done` job with a fake explanation and a fake video URL. Redis running. |
| `client/` | Extension loads unpacked, hotkey fires, `captureVisibleTab` produces a PNG, side panel opens. |

**Also in this sprint, and it's on the critical path:** somebody writes
`samples/product_rule_scenes.py` — a Manim scene that actually renders, using
relative positioning only. It's the codegen cheatsheet seed and the render lane's
test fixture. It does not exist yet. `render/` owns it.

**Also:** the cache-hit experiment. Screenshot one problem six ways (two zoom
levels × three window widths), run each through `/vision`, count distinct
`problem_text` strings. `agent/` owns it. If they're all different, we learn now
that Redis is decoration and we stop building around it.

**Checkpoint 11:00 PM:** every lane runs end-to-end *within itself*, on fakes.
Nothing is integrated. Report: does LaTeX work, and how many distinct hashes out
of six.

## Sprint 2 — Vertical slice on fakes · Fri 11:00 PM → 3:00 AM

Real code in every lane, still fake at every boundary. Ends with a real text
explanation reaching a real side panel.

| Lane | Do this |
|---|---|
| `agent/` | All three endpoints up on `:8000`. `/vision` and `/explain` are real Claude calls with structured outputs. `/codegen` can return a hardcoded scene for now. **Decide the determinism question** (CONTRACTS §1 OPEN) using the Sprint 1 numbers. |
| `render/` | Render function takes a source string, writes `.py`, runs `manim`, returns a path. Static pre-check (AST parse, banned imports). Repair loop scaffolded but can be a stub. |
| `server/` | Real job queue + worker pool. Goroutine calls `/vision` then `/explain`, writes both to Redis. Cache key implemented, cache read/write wired. Still fakes `/codegen` and the render. |
| `client/` | Polls for real. Renders markdown explanation the instant it's non-null. Handles `failed`-with-explanation. Video player element present but can show a placeholder. |

**Checkpoint 3:00 AM — this is the important one.** Hotkey → screenshot → real
explanation in the side panel in under ~10 seconds. No video yet. **If this
works, we have a demo**, and everything after is upside.

## Sprint 3 — Real render path · Sat 3:00 AM → 7:00 AM

Overnight. Stagger sleep — at least two people awake, and not two from the same
lane if you can help it.

| Lane | Do this |
|---|---|
| `agent/` | `/codegen` is a real call. Prompt carries the cheatsheet from `samples/` and hard constraints: relative positioning only (`next_to`, `arrange`, `to_edge`), never literal coordinates, class is always `GeneratedScene`. Repair prompt takes source + traceback. |
| `render/` | Real subprocess, real repair loop, 3 attempts. Per-render timeout (~120s) so one bad scene can't wedge a worker. MP4 lands in `./renders/<hash>.mp4`. |
| `server/` | Wires `/codegen` and render into the goroutine. Writes `video_url` to Redis under **both** job ID and problem hash. Serves `/renders/` with range requests. |
| `client/` | Player works for real. Video appears without a page reload when polling flips to `done`. |

**Checkpoint 7:00 AM:** one problem goes hotkey → explanation → video, all real.
Note how often codegen needs repair, and whether repair actually recovers.

## Sprint 4 — Cache, failure, second surface · Sat 7:00 AM → 11:00 AM

Everything that makes it survive contact with a judge.

| Lane | Do this |
|---|---|
| `agent/` | Prompt tuning on real failures from Sprint 3. Bias storyboards toward what text can't do — a derivative plotted against its function, a pointer walking a list — not animated algebra. **Bump `PromptVersion` every time you change a prompt.** |
| `render/` | Harden: cleanup of temp `.py` and partial output, concurrency cap so N renders don't eat every core, sane behavior when all 3 repairs fail. |
| `server/` | Cache hit path verified end-to-end: second identical request returns in under a second with `cached: true`. Failure path verified: codegen dies 3× → status `failed`, explanation **preserved and returned**. |
| `client/` | Paste-a-screenshot web page (plain HTML + JS, same API). Empty/error/failed states actually look deliberate. |

**Checkpoint 11:00 AM:** two people screenshot the same problem; the second gets
it instantly. Kill the agent service mid-job and confirm the UI degrades to the
explanation instead of hanging.

## Sprint 5 — Freeze, integrate, demo · Sat 11:00 AM → 4:00 PM

| Time | What |
|---|---|
| 11:00 AM – 1:00 PM | Everything running on one machine at once. Fix only what's broken end-to-end. |
| 1:00 PM – 2:00 PM | **Pre-warm the cache** with 3–4 problems you'll actually demo — rendered, cached, verified playing. This is the single highest-value hour on the clock. |
| **2:00 PM** | **Code freeze.** No new features. Bug fixes only, and only for bugs on the demo path. |
| 2:00 PM – 3:00 PM | Run the demo start to finish, three times, on the machine and network you'll present from. Write down what you say. |
| 3:00 PM – 4:00 PM | README, slides, buffer. Nobody touches code. |

---

# The demo

## Script

1. Open a real problem. Hit the hotkey. Explanation appears in seconds — **say
   out loud that the video is still rendering.** That's the design, not an
   apology.
2. Video appears. Play it.
3. Second laptop, same problem, different zoom level → instant. That's the cache,
   and it's the part that's actually hard.
4. Have a pre-rendered fallback video on disk. Conference wifi has killed better
   demos than ours.

## Questions the judges will ask

- **"Is this a cheating tool?"** A judge at a university hackathon will ask.
  Answer with what the storyboard prompt is built to do — teach the method — not
  a disclaimer.
- **"Why Go and Python both?"** Honest answer: Python has the SDK and the agent
  ecosystem; Go gives an explicit concurrency cap on the expensive, CPU-bound
  part. The split follows the workload.
- **"Does the video beat the text?"** Only if it shows what text can't: a
  function and its derivative plotted together, a pointer walking a list, a shape
  transforming. If the animation is just animated algebra, the text already did
  it better. Demo a problem where motion carries meaning, and bias the storyboard
  prompt that way.

## If you fall behind

Cut in this order. Each line is safe to lose without the demo dying:

1. The paste-a-screenshot web page (extension alone demos fine).
2. The repair loop (fail straight to explanation-only).
3. The cache (slower, still works — but you lose the best 20 seconds of the demo).
4. The video.

**Never cut:** hotkey → screenshot → text explanation. That's the product. The
video is the reward.

---

## Still open

Written down so nobody re-decides them at 3 AM. Each one has an owner.

| Question | Owner | By when |
|---|---|---|
| **How do we make transcription deterministic?** `temperature=0` isn't available on Claude Opus 5 (sampling params return a 400). Options in [CONTRACTS §1](CONTRACTS.md#1-python-agent-service-internal). | `agent/` | End of Sprint 2 |
| **Does the cache actually hit?** Unmeasured — that's what the six-screenshot experiment is for. | `agent/` | Sprint 1 |
| **Should a cache hit also return the cached explanation?** Almost certainly yes, under `cache:<hash>:explanation`. | `server/` | Sprint 2 |
| **Privacy.** The screenshot is the whole viewport — tabs, browser chrome, possibly the student's name. For the demo we say so out loud; anything real needs a crop step. | everyone | Before Saturday |

## The docs

| File | What it's for | Read it when |
|---|---|---|
| [CONTRACTS.md](CONTRACTS.md) | Every JSON shape and HTTP route at every boundary. Draft — change it there first, in its own commit | Before writing any code that crosses a lane boundary |
| [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) | Four lanes: what each owns, exposes, and **fakes**; what "done" looks like; traps | When you claim a lane |
| [docs/SETUP.md](docs/SETUP.md) | Install Python, Go, Redis, Manim, **LaTeX**, load the extension, run everything | First thing, on every laptop |
| [docs/FRD.md](docs/FRD.md) | Functional requirements, numbered, Must/Should/Could; non-functional targets; what's out of scope | When deciding whether something is worth building |
| [FILESTRUCTURE.md](FILESTRUCTURE.md) | Where every file goes, and the one-lane-one-directory rule | Before creating a file |

## Working together

- Everyone pushes to `main`. Pull first. Small commits.
- One lane, one directory. Don't touch another lane's files without saying so.
- `CONTRACTS.md` changes go in their own commit, announced in chat.
- Blocked on someone? You're not faking hard enough. Fake harder, keep moving,
  tell them what shape you assumed.

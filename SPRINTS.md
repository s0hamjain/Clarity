# Sprint plan

**Demo: Saturday 4:00 PM.** Everything below counts backward from that.

Four lanes, four people, running in parallel the whole time:

| Lane | Owns | Brief |
|---|---|---|
| `agent/` | Python, all three Claude calls | [docs/work_split.md](docs/work_split.md#agent--python-all-three-claude-calls) |
| `render/` | Go, Manim subprocess, repair loop | [docs/work_split.md](docs/work_split.md#render--go-manim-subprocess-repair-loop) |
| `server/` | Go, HTTP API, Redis, job queue | [docs/work_split.md](docs/work_split.md#server--go-http-api-redis-job-queue) |
| `client/` | Chrome extension, side panel, paste page | [docs/work_split.md](docs/work_split.md#client--chrome-extension--paste-page) |

The shapes everyone builds against are in [CONTRACTS.md](CONTRACTS.md). Read it
before writing code. If you need it to change, change it there first and tell
everyone.

**The rule that makes four-way parallelism work: every lane fakes its
dependencies from hour zero.** Nobody waits for anybody. Each brief says exactly
what that lane fakes and when it stops faking it.

---

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

---

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

---

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

---

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

---

## Sprint 5 — Freeze, integrate, demo · Sat 11:00 AM → 4:00 PM

| Time | What |
|---|---|
| 11:00 AM – 1:00 PM | Everything running on one machine at once. Fix only what's broken end-to-end. |
| 1:00 PM – 2:00 PM | **Pre-warm the cache** with 3–4 problems you'll actually demo — rendered, cached, verified playing. This is the single highest-value hour on the clock. |
| **2:00 PM** | **Code freeze.** No new features. Bug fixes only, and only for bugs on the demo path. |
| 2:00 PM – 3:00 PM | Run the demo start to finish, three times, on the machine and network you'll present from. Write down what you say. |
| 3:00 PM – 4:00 PM | README, slides, buffer. Nobody touches code. |

### Have answers ready

- **"Is this a cheating tool?"** A judge at a university hackathon will ask.
  Answer with what the storyboard prompt is built to do — teach the method — not
  a disclaimer.
- **"Why Go and Python both?"** Honest answer: Python has the SDK and the agent
  ecosystem; Go gives an explicit concurrency cap on the expensive, CPU-bound
  part. The split follows the workload.
- **"Does the video beat the text?"** If the animation only shows animated
  algebra, the text already did it better. Demo a problem where motion actually
  carries meaning.

### Demo script

1. Open a real problem. Hit the hotkey. Explanation appears in seconds — **say
   out loud that the video is still rendering.** That's the design, not an
   apology.
2. Video appears. Play it.
3. Second laptop, same problem, different zoom level → instant. That's the cache,
   and it's the part that's actually hard.
4. Have a pre-rendered fallback video on disk. Conference wifi has killed better
   demos than ours.

---

## If you fall behind

Cut in this order. Each line is safe to lose without the demo dying:

1. The paste-a-screenshot web page (extension alone demos fine).
2. The repair loop (fail straight to explanation-only).
3. The cache (slower, still works — but you lose the best 20 seconds of the demo).
4. The video.

**Never cut:** hotkey → screenshot → text explanation. That's the product. The
video is the reward.

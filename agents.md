# Ambient visual learning tool — HackCMU

**Start here.** This is the index. Everything else is linked from it.

Four people, four laptops, one repo, demo **Saturday 4:00 PM**.

## What we're building

A student hits a hotkey while looking at a hard math or algorithm problem in
their browser. A side panel opens, they can optionally type a question, and
they get back two things: a written step-by-step explanation within seconds,
and a custom animated video explaining the same problem about a minute later.

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

## The steps, in order

Five sprints, Friday 8 PM → Saturday 4 PM. Full detail with per-lane tasks and
checkpoints in [SPRINTS.md](SPRINTS.md).

| # | When | Goal | You know it worked when |
|---|---|---|---|
| 1 | Fri 8 PM – 11 PM | **De-risk.** LaTeX works, one Claude call works, server returns a fake job, extension captures a screenshot. Run the cache-collision experiment. | Every lane runs end-to-end *on fakes*. We know if LaTeX is broken and how many of six screenshots hash the same. |
| 2 | Fri 11 PM – 3 AM | **Vertical slice.** Real vision + explain calls, real queue, real polling. Render still faked. | Hotkey → real explanation in the side panel in < 10s. **If this works, we have a demo.** |
| 3 | Sat 3 AM – 7 AM | **Real render.** Codegen, subprocess, repair loop, video served. | One problem goes hotkey → explanation → video, all real. |
| 4 | Sat 7 AM – 11 AM | **Cache, failure, paste page.** Second identical request is instant. Killing the agent mid-job degrades to explanation-only. | Two laptops, same problem, second one instant. |
| 5 | Sat 11 AM – 4 PM | **Freeze and demo.** Pre-warm the cache with demo problems. Code freeze 2 PM. Rehearse three times. | You've said the demo out loud, on the presenting machine, without it breaking. |

If you fall behind, cut in this order: paste page → repair loop → cache → video.
**Never cut** hotkey → screenshot → text explanation.

## The docs

| File | What it's for | Read it when |
|---|---|---|
| [SPRINTS.md](SPRINTS.md) | The five cycles, per-lane tasks, checkpoints, demo script, what to cut | Now, and at every checkpoint |
| [CONTRACTS.md](CONTRACTS.md) | Every JSON shape and HTTP route at every boundary. Draft — change it there first, in its own commit | Before writing any code that crosses a lane boundary |
| [docs/work_split.md](docs/work_split.md) | Four lanes: what each owns, exposes, and **fakes**; what "done" looks like; traps | When you claim a lane |
| [docs/setup.md](docs/setup.md) | Install Python, Go, Redis, Manim, **LaTeX**, load the extension, run everything | First thing, on every laptop |
| [docs/frd.md](docs/frd.md) | Functional requirements, numbered, Must/Should/Could; non-functional targets; what's out of scope | When deciding whether something is worth building |
| [filestructure.md](filestructure.md) | Where every file goes, and the one-lane-one-directory rule | Before creating a file |

## Things that are still open

Written down so nobody re-decides them at 3 AM. Each one has an owner in the docs.

- **How do we make transcription deterministic?** `temperature=0` isn't
  available on Claude Opus 5 (sampling params return a 400). Options are in
  CONTRACTS §1. `agent/` decides by end of Sprint 2.
- **Does the cache actually hit?** Unmeasured. Sprint 1 experiment.
- **Should a cache hit also return the cached explanation?** Almost certainly
  yes. `server/`, Sprint 2.
- **"Is this a cheating tool?"** A judge will ask. Have an answer about what the
  storyboard prompt is built to do — teach the method — not a disclaimer.
- **Does the video beat the text?** Only if it shows what text can't: a function
  and its derivative plotted together, a pointer walking a list, a shape
  transforming. Bias the storyboard prompt that way.

## Working together

- Everyone pushes to `main`. Pull first. Small commits.
- One lane, one directory. Don't touch another lane's files without saying so.
- `CONTRACTS.md` changes go in their own commit, announced in chat.
- Blocked on someone? You're not faking hard enough. Fake harder, keep moving,
  tell them what shape you assumed.

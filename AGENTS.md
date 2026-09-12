# Ambient visual learning tool

**Start here.** This is the project index — what the system does, how its
pieces fit together, and where to look for detail on any of it.

## The idea

A student is stuck on a hard math or algorithm problem in their browser. They
hit a hotkey. A side panel opens with an optional question field and a
guardrails toggle, and within seconds they get a written, step-by-step
explanation of the problem. About a minute later, a custom animated video
arrives in the same panel, walking through the same problem visually —
generated fresh for that specific problem, not pulled from a library of
pre-made clips.

For problems that aren't in a browser — code in an IDE, a PDF on the desktop —
the same experience is available as a web app: paste or drop a screenshot
instead of using the hotkey.

**Guardrails mode**, when enabled, changes what comes back: instead of
solving the problem, the explanation and the video teach the method and stop
short of the final answer.

Two decisions shape everything downstream:

- **The explanation is the product; the video is a reward that arrives
  after.** Rendering an animation is slow and CPU-bound; producing the
  written explanation is fast. Waiting for the video to show the explanation
  would make a working system feel broken, so the two are delivered
  separately — the explanation the moment it exists, the video once it's
  ready.
- **The same problem is only rendered once.** The system hashes each
  transcribed problem together with the user's question and the guardrails
  setting, and checks a cache before doing any work. A repeat of the same
  request returns the existing video and explanation in under a second
  instead of re-running the pipeline.

## How a request flows, end to end

1. **Capture.** The user hits the hotkey (or pastes a screenshot in the web
   app). The client sends the image, the optional question, and the
   guardrails flag to the coordinator.
2. **Acknowledge.** The coordinator creates a job, returns a job ID
   immediately, and does everything below in the background.
3. **Read the problem.** A model call transcribes the screenshot to verbatim
   problem text and a category (math or algorithm).
4. **Check the cache.** The coordinator hashes the text, the question, and
   the guardrails flag. On a hit, it returns the cached video and
   explanation immediately and stops here.
5. **Explain.** On a miss, a model call produces the written explanation and
   a storyboard of 2–5 short animation scenes. The explanation is written to
   the cache and handed to the client the moment it exists — this is the
   point where the user stops waiting.
6. **Generate and render, per scene, in parallel.** For each scene: a model
   call retrieves relevant reference snippets from a Manim documentation
   corpus, then another turns the scene into Manim source code. The
   coordinator runs that code in its own isolated container. A scene that
   fails is sent back for repair, with the error, up to three times,
   independently of the other scenes.
7. **Join and store.** Once every scene has finished (or given up), the
   coordinator joins the successful clips into one video, uploads it to
   object storage, and writes the URL to the cache and the job record.
8. **Deliver.** The client, still polling the job, picks up the finished
   video and plays it inline.

If every scene fails, the job still completes with the explanation shown and
a note that the animation didn't render — the explanation never depends on
rendering succeeding.

## Architecture

```
Client (browser extension panel, or web app)
        │  HTTP
        ▼
Coordinator ────────────────► Agent service ────────────► Claude API
(job queue, cache,             (every model call:
 per-scene fan-out,              transcription, explanation,
 render dispatch,                doc retrieval, codegen)
 repair loop)
        │
        ├──► Cache / job store        (problem hash → video + explanation)
        ├──► Isolated render workers  (one container per scene)
        ├──► Video join               (no re-encoding — every scene shares
        │                              one resolution and frame rate)
        └──► Object storage           (finished videos, served directly
                                        to the client)
```

- **The agent service** owns every call to the model: transcription,
  explanation and storyboard generation, documentation retrieval, and code
  generation. Nothing else talks to the model API.
- **The coordinator** owns everything else: the job lifecycle, the cache, the
  per-scene fan-out and repair, joining clips, and storage.
- **The client** is the browser extension's side panel and a standalone web
  app, sharing one UI implementation between them.

## Components

| Component | Responsibility | Detail |
|---|---|---|
| `agent/` | Every model call: transcription, explanation, doc retrieval, codegen | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#agent--python-all-claude-calls--manim-doc-retrieval) |
| `render/` | Isolated per-scene rendering, repair, joining clips, upload | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#render--go-docker-dispatch-repair-loop-ffmpeg-s3) |
| `server/` | Public API, job lifecycle, cache, orchestration | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#server--go-http-api-redis-job--scene-orchestration) |
| `client/` | Extension panel and web app | [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md#client--angular-extension-panel--web-app) |

The exact shapes each component builds against — every JSON payload and HTTP
route — are in [CONTRACTS.md](CONTRACTS.md). Read it before writing code that
crosses a component boundary; if it needs to change, change it there first.

## Open questions

- **Transcription determinism.** The cache only works if repeat screenshots
  of the same problem hash the same way. Options in
  [CONTRACTS §1](CONTRACTS.md#1-python-agent-service-internal).
- **Manim-doc retrieval quality.** Depends on the embedding approach chosen —
  see the same section.
- **Does guardrails mode actually withhold the answer?** It's a prompt
  instruction, not an enforced filter, so it needs verification against real
  output, not just a prompt review.
- **Privacy.** The screenshot captures the whole visible tab and is sent to a
  third-party API and stored in object storage. There's no crop step and no
  retention policy beyond the cache TTL.

## The docs

| File | What it's for |
|---|---|
| [CONTRACTS.md](CONTRACTS.md) | Every JSON shape and HTTP route, and the render pipeline's internals |
| [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) | What each component owns, exposes, and mocks; what "done" looks like |
| [docs/SETUP.md](docs/SETUP.md) | Install and run everything locally |
| [docs/FRD.md](docs/FRD.md) | Functional requirements |
| [FILESTRUCTURE.md](FILESTRUCTURE.md) | Directory layout |

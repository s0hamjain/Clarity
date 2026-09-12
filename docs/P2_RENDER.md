# Clarity — P2: Render (Docker, Manim, Video)

**You are P2. Your job in one sentence:** turn a string of Manim Python into a finished MP4 on S3 — safely, inside a locked-down Docker container, retrying when the code crashes, stitching scenes together — and write the verified example scenes the AI learns from.

You never write a prompt, never write an HTTP handler, never touch the desktop app. You own everything between "here is some Python" and "here is a video URL."

---

## Your job in plain words

1. **Build the sandbox.** A Docker image with Python, Manim, LaTeX, and ffmpeg, so every render runs isolated with no network.
2. **Write the examples.** 20+ small, verified Manim scenes in `samples/`. These are what the AI imitates. Each one must render *and look right* — you watch every one.
3. **Check code before running it.** A static pre-check: must parse as Python, may only import `manim`, no `os.system` / `eval` / `subprocess` / file writes, must define `class GeneratedScene(Scene)`.
4. **Render one scene.** Write the code to a temp dir, `docker run` it with a hard timeout, return the MP4 path or the traceback.
5. **Retry.** Up to 3 attempts per scene: on failure, call back to get fixed code (P3 hands you the function), render again.
6. **Stitch and upload.** Join the finished scene clips into one MP4 without re-encoding, upload to S3, return the URL.

---

## What you own · what you never touch

| You own | You never touch |
|---|---|
| `docker/manim-worker/**` — the image | HTTP handlers, the job store, the cache (`server/internal/{api,store,jobs,cache,agent}` — P3) |
| `samples/**` — the example scenes | Any prompt (P1) |
| `server/internal/render/**` — the Go render package | Anything in `desktop/` (P4) |

You and P3 write Go in the same module (`server/`). You own one package inside it. The boundary is **three function signatures** below. Neither of you changes them without the other.

---

## Where your work meets others

### You implement these; P3 calls them (FRD §14.1)

```go
// Render runs src inside a manim-worker container and returns the finished clip path,
// or a RenderError with the container's stderr so the caller can send it back for repair.
// Never blocks longer than the timeout.
func Render(ctx context.Context, src string, workDir string) (clipPath string, err *RenderError)

type RenderError struct {
    Stage     string // "precheck" | "container" | "timeout"
    Traceback string // stderr, trimmed to the last 40 lines
}

// RenderWithRepair tries up to 3 times for one scene. Each attempt: retrieve → codegen → Render.
// retrieve and codegen are functions P3 passes in (they wrap the agent service's HTTP endpoints).
func RenderWithRepair(ctx context.Context, scene Scene, retrieve RetrieveFunc, codegen CodegenFunc) (clipPath string, err *RenderError)

// Concat joins finished clips in order with no re-encode, uploads to S3, returns the public URL.
func Concat(ctx context.Context, clipPaths []string, outKey string) (videoURL string, err error)
```

### You produce this; P1's script reads it (FRD §13)

Every `samples/*.py` starts with this docstring — **exactly this shape**, P1's parser depends on it:

```python
"""
title: Array walk with a moving pointer
description: A row of boxes built with VGroup.arrange; an arrow pointer steps left to right, highlighting each box.
category: algorithm
tags: VGroup, arrange, Arrow, Indicate
"""
from manim import *

class GeneratedScene(Scene):
    def construct(self):
        ...
```

One scene per file. Class name always `GeneratedScene`. Relative positioning only.

**Nobody is waiting on you to start.** Test `Render` by passing a file from `samples/`. Test `RenderWithRepair` by passing a fake `codegen` that returns a broken sample first and a good one second.

---

## Before you start (the Docker build is the long pole — do it first)

Follow [SETUP.md](SETUP.md) §1, §6, §7. You need Docker Desktop running, Go 1.22+, `ffmpeg` on the host (for `ffprobe` and concat), MinIO running with the `clarity-renders` bucket public-read.

Read once: **FRD §14** (the whole render pipeline), **FRD §13** (what the samples are for), **SETUP §6.2** (the smoke test), **FRD §23 rules 14–16** (yours).

---

## Files you'll create

```
docker/manim-worker/
├── Dockerfile
└── README.md                    # build command, smoke test, the 14-day S3 lifecycle note

samples/
├── README.md                    # the docstring format + "watched, not just rendered"
├── 001_mathtex_side_by_side.py
├── 002_axes_plot_moving_dot.py
├── ...                          # 5 by Sprint 1, 20 by Sprint 2

server/internal/render/
├── precheck.go                  # parse + banned list + class check
├── docker.go                    # Render()
├── semaphore.go                 # concurrency cap
├── validate.go                  # clip size floor + ffprobe duration
├── repair.go                    # RenderWithRepair()
├── concat.go                    # Concat(): ffprobe consistency check → ffmpeg concat -c copy
├── s3.go                        # upload, public URL
└── render_test.go               # renders every samples/*.py through a real container
```

---

## Sprint 1 — The image, the smoke test, five samples, the pre-check

**Goal:** LaTeX renders inside the container (this is the single most likely thing to be broken in the whole project — find out now), five samples exist and have been watched, and code can be checked before it runs.

### Step 1 — Dockerfile (1 h, mostly waiting)
```dockerfile
FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    texlive-latex-base texlive-latex-extra texlive-fonts-recommended texlive-science \
    dvisvgm ffmpeg libcairo2-dev libpango1.0-dev pkg-config build-essential \
 && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir manim
WORKDIR /work
```
`docker build -t manim-worker -f docker/manim-worker/Dockerfile docker/manim-worker`. Then run the **SETUP §6.2 smoke test** — a `MathTex` scene inside the container. **Nothing else you do matters until `out.mp4` exists.** If it dies on `latex` or `dvisvgm`, fix the image, not your Mac.

### Step 2 — Five samples (1 h)
Start with these — they cover the shapes the AI will need most:
1. `001_mathtex_side_by_side.py` — two `MathTex`, `next_to`, then `Transform` one into a derivative.
2. `002_axes_plot_moving_dot.py` — `Axes`, `plot` a function, a `Dot` sliding along it via `ValueTracker`.
3. `003_array_walk_pointer.py` — `VGroup` of squares, `arrange(RIGHT)`, an `Arrow` pointer stepping across with `Indicate`.
4. `004_shape_morph.py` — `Circle` → `Square` → `Triangle` with `Transform`.
5. `005_narration_bar.py` — how narration text sits at `to_edge(DOWN)` and gets replaced per beat with `FadeOut`/`FadeIn`.

Rules for every sample: docstring header as above; relative positioning only (`next_to`, `arrange`, `to_edge`, `shift` by fractions of `config.frame_width`) — **never literal coordinates**; 5–15 seconds. Render each in the container. **Watch each one.** Overlapping text or something off-frame means it's not a sample.

### Step 3 — Pre-check (1 h)
`precheck.go`: shell out to `python3 -c "import ast,sys; ast.parse(sys.stdin.read())"` for a parse check; then scan the source for banned patterns — any `import` / `from … import` that isn't `manim` or stdlib; `os.system`, `subprocess`, `__import__`, `eval(`, `exec(`, `open(` with a write mode; require the literal `class GeneratedScene(Scene)`. Return a `RenderError{Stage: "precheck"}` with a one-line reason. Table-driven test: one good sample + six bad inputs.

### Done when
- [ ] `docker run … manim-worker` renders the `MathTex` smoke test to MP4.
- [ ] Five `samples/*.py` with correct docstrings render in the container and you've watched all five.
- [ ] `go test ./internal/render/ -run Precheck` passes.

---

## Sprint 2 — `Render()`, the semaphore, the test, samples to 20

**Goal:** a string of Python becomes an MP4 (or a traceback) through a real container with a real timeout; the library is big enough to be useful.

### Step 1 — `Render()` (2 h)
`docker.go`:
- Make a per-scene temp dir; write `src` to `scene.py`.
- Run the pre-check first. Fail fast.
- `exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--memory", "1g", "--cpus", "1", "-v", tmp+":/work", "manim-worker", "manim", quality, "/work/scene.py", "GeneratedScene", "-o", "out.mp4", "--media_dir", "/work/media")` where `quality` is `-ql` or `-qm` from `MANIM_QUALITY` — **one value per job, never per scene** (concat depends on it).
- Timeout: `ctx` with 120 s. **On expiry, `docker kill` the container** — killing the `docker` CLI process does not stop the container. Name the container (`--name clarity-<jobid>-<scene>`) so you can.
- On non-zero exit: `RenderError{Stage: "container", Traceback: last40Lines(stderr)}`.
- Find the output: manim writes to `<media_dir>/videos/scene/<quality>/out.mp4`. Move it to `<workDir>/scene<N>.mp4`. Don't go hunting.
- `validate.go`: file size > 20 KB and `ffprobe` duration > 1 s. A scene that "succeeded" but is 0.3 s long because every `play` silently failed is a failure.
- Clean up the temp dir on **every** exit path, including timeout.

### Step 2 — Semaphore (20 min)
`semaphore.go`: buffered channel sized from `RENDER_CONCURRENCY`. Acquire around `docker run` only — not around the codegen call. P3 will share one instance across all jobs.

### Step 3 — `render_test.go` (30 min)
Renders every `samples/*.py` through the real container. Skip with `t.Skip` if Docker isn't running. This is also the library's regression test — run it before the sync point.

### Step 4 — Samples to 20 (2 h)
Cover: `Table`; `Code` block with a highlighted line; `NumberLine` with a moving dot; `Graph` (nodes + edges) with BFS-style highlighting; `Brace` with a label; a `ValueTracker`-driven `DecimalNumber`; a recursion tree via nested `VGroup`s; a stack (boxes pushed/popped); a matrix with a row highlighted; two functions plotted with the area between them shaded; a limit approaching a point; an angle sweeping. Docstring on every one. Watched, every one.

### Done when
- [ ] `Render()` renders a sample; returns a real traceback for a broken one; kills the container on a `while True: pass` scene at 120 s (check `docker ps` is empty after).
- [ ] `go test ./internal/render/` passes against Docker.
- [ ] 20 samples in `samples/`, all watched. Tell P1 so they re-run the seed script.

---

## Sprint 3 — `RenderWithRepair()`, `Concat()`, S3

**Goal:** the full path from "scene" to "video URL" works, including repair and stitching.

### Step 1 — `RenderWithRepair()` (1.5 h)
`repair.go`: up to 3 attempts. Each attempt:
```
snippets := retrieve(scene, lastTracebackFirstLine)   // "" on the first attempt
src       := codegen(scene, snippets, prevSrc, prevTraceback)
clip, err := Render(ctx, src, workDir)
```
Success → return. Failure → record `prevSrc`, `prevTraceback`, try again. Third failure → return the error; P3 drops the scene. Log every attempt with the scene index, attempt number, and the traceback's first line — P1 will read these logs in Sprint 4.

Test with a fake `codegen` that returns a broken sample on attempt 1 and a good one on attempt 2.

### Step 2 — `Concat()` (1 h)
`concat.go`:
- Before joining, `ffprobe` every clip's `width,height,r_frame_rate,codec_name`. If they differ, **fail loudly** — `-c copy` on mismatched inputs produces broken output silently.
- Write `concat_list.txt` (`file 'scene0.mp4'` per line), run `ffmpeg -f concat -safe 0 -i concat_list.txt -c copy final.mp4`.
- Then upload (Step 3) and return the URL.

### Step 3 — S3 upload (1 h)
`s3.go`: AWS SDK v2. `S3_ENDPOINT` override with **path-style addressing** (MinIO needs it). `PutObject` to `renders/<hash>.mp4` in `RENDER_BUCKET`, `ContentType: video/mp4`. Return `<S3_ENDPOINT>/<bucket>/renders/<hash>.mp4` (public-read bucket). Test by opening the URL in a browser — it should play.

### Step 4 — Lifecycle note (10 min)
In `docker/README.md`: the real-S3 bucket needs a lifecycle rule expiring `renders/` after 14 days. MinIO locally doesn't. This sits behind the 7-day cache TTL so the cache never points at a deleted video.

### Done when
- [ ] `RenderWithRepair` recovers from a deliberately broken first attempt.
- [ ] `Concat` joins 3 clips into one playable MP4 and refuses mismatched inputs.
- [ ] A URL from `s3.go` plays in a browser.
- [ ] P3 has wired all three into a real job and one capture produced a real video.

---

## Sprint 4 — Harden

**Goal:** nothing leaks, nothing hangs, release quality works.

1. **Cleanup audit:** after a batch of 10 renders including a timeout and a pre-check failure, `docker ps -a` shows nothing and the temp dir is empty.
2. **Timeout actually kills:** a `while True: pass` scene → `docker ps` empty after 120 s, `RenderError{Stage: "timeout"}` returned.
3. **`MANIM_QUALITY=-qm`:** render three scenes at 720p and confirm `Concat` still succeeds.
4. **Seeds for P1's error classes:** P1 will hand you the recurring tracebacks. Each one that's "the model used the API wrong" becomes a new sample showing the right way.
5. **Image rebuild from scratch:** `docker build --no-cache` on someone else's Mac. It should work first time from `docker/README.md`.

### Done when
- [ ] Cleanup audit clean. Timeout kill verified. `-qm` path works. New samples landed. Clean-machine build succeeds.

---

## Sprint 5 — Release

- `MANIM_QUALITY=-qm` for the release configuration.
- Run `render_test.go` one last time. Freeze `samples/`.
- Confirm with P3 that pre-warmed cache videos play.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p2/sprint-N-<what>`.
2. Commit small. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, `go test ./internal/render/`, push.
4. **At the sync point:** merge order is **P3 → P1 → P2 → P4**. You're third. Since you and P3 share `server/go.mod`, rebase carefully — `go mod tidy` conflicts are the usual snag.
5. After the merge: back to step 1.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

14. The static pre-check runs before **every** `docker run`, including repair attempts.
15. Every `docker run` has `--network none`, `--memory`, `--cpus`, and a context timeout that kills the **container**, not just the CLI.
16. A scene failure never propagates to sibling scenes. A scene that exhausts repair is dropped; the job continues.

Plus: the manim quality flag is one job-level value passed identically to every scene (rule 12 — shared with P3).

---

## Traps

- manim's default output path is `media/videos/<scriptname>/<quality>/<SceneName>.mp4`. Always pass `--media_dir` and `-o`, then move the file. Don't search for it.
- `-c copy` requires byte-identical codec parameters. One scene at `-ql` and one at `-qm` = broken video, sometimes with exit code 0. That's why the quality flag is job-level and why `Concat` checks with `ffprobe` first.
- Killing `exec.Cmd` kills the `docker` client. The container keeps running. `docker kill <name>`.
- A sample that renders is not a sample that's *good*. Watch it.

## If you're blocked

You shouldn't be — nothing you build waits on anyone. If you need a real `/codegen` to test repair before P1 has one, fake it with a function that returns a broken sample then a good one.

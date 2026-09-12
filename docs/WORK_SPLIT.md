# Work split

Four components, each with a clear owner and a clear set of things it fakes
so it can be built without waiting on the others.

Shapes at every boundary are in [../CONTRACTS.md](../CONTRACTS.md); read it
before starting.

**Owners: TBD** — fill in names here when you claim a component.

| Component | Owner | Language | Directory |
|---|---|---|---|
| `agent/` | | Python | `agent/` |
| `render/` | | Go | `server/internal/render/` |
| `server/` | | Go | `server/` (everything but `render/`) |
| `client/` | | Angular / TypeScript | `client/` |

`render/` and `server/` are both Go and live in the same module. They are
separate components because they're separate problems: one dispatches
isolated containers and joins their output, the other is an HTTP server with
a queue. They meet at one Go function signature (below).

---

## `agent/` — Python, all Claude calls + Manim-doc retrieval

**Owns:** every line that talks to the Anthropic API — vision, explanation,
Manim-doc retrieval, codegen — plus the Manim-doc corpus and the
determinism decision.

**Exposes:** a FastAPI service on `:8000` with `POST /vision`, `/explain`,
`/manim-docs`, `/codegen`, `GET /healthz`. Request/response shapes in
CONTRACTS §1.

**Fakes:** nothing — it's a leaf. Testable with `curl` and a PNG from day
one. Other components fake it by hardcoding responses until it's ready.

### What "done" looks like

- All four endpoints return schema-valid JSON via structured outputs. No
  markdown fences, no prose.
- `/vision` returns *verbatim* text. If you catch yourself adding "the
  problem asks..." to the prompt, stop — that's the thing that breaks the
  cache.
- `/explain` returns a **multi-scene** storyboard (2–5 scenes). Each scene is
  a full, independent Manim `Scene` — not a moment inside one shared scene.
  That's what lets `render/` fan them out to containers in parallel.
- `guardrails: true` on `/vision`, `/explain`, and `/codegen` changes what
  comes back: the explanation and every scene teach the method and stop
  short of the final answer. This is a prompt instruction, not an enforced
  filter, so it needs verification against real output.
- `/manim-docs` is retrieval, not generation: given one scene's intent,
  return the 2–3 most relevant snippets from the corpus. A static, curated
  corpus of ~50–100 chunks (official Manim CE worked examples + `samples/`),
  embedded once at process startup, retrieved by cosine similarity.
  Keyword/BM25 match against snippet titles is an acceptable fallback if
  embeddings aren't wired up — worse recall, zero setup cost.
- `/codegen` output always has `class GeneratedScene(Scene)`. Always. Assert
  it before returning. It's per-scene — one call per scene, not one call for
  the whole storyboard — because scenes render and repair independently.
- `/codegen` never hardcodes a resolution or frame rate. The coordinator
  controls the manim quality flag identically across every scene in a job;
  generated source must not fight that.
- Repair path: `/codegen` with `previous_source` + `traceback` returns fixed
  source for *that scene*, not a rewrite from scratch.
- `PromptVersion` bumped every time a prompt is touched, even prompts that
  live here — it's a constant in the coordinator (`server/internal/cache/key.go`).

### Constraints to bake into the codegen prompt

- Manim **Community Edition** imports only: `from manim import *`. Nothing
  else.
- Relative positioning only: `next_to`, `arrange`, `to_edge`, `shift` by
  fractions of `config.frame_width`. **Never literal coordinates.** The frame
  is 14.2 units wide and objects that wander off it exit zero — that can't be
  caught programmatically.
- One `Scene` per call, matching one storyboard scene.
- `narration` → `Text(...)` at `to_edge(DOWN)`.
- Include the retrieved `manim_snippets` verbatim in the prompt as the
  reference to imitate.

### Decisions owned here

1. **Determinism** (CONTRACTS §1 OPEN). Run the screenshot experiment, then
   pick: Opus 5 + structured outputs + low effort, or `claude-haiku-4-5` with
   `temperature=0` for `/vision` only.
2. **Embeddings model for retrieval** (CONTRACTS §1 OPEN). A local CPU model
   keeps `/manim-docs` at zero added API latency and zero extra cost. Keyword
   match is the fallback, not the plan.
3. **Storyboard bias.** Each scene should ask: would this beat be better as a
   sentence? If yes, cut it.
4. **Guardrails wording.** What "teach the method, not the answer" actually
   means in the prompt, for both math and code — and verifying it holds on
   real problems.

---

## `render/` — Go, Docker dispatch, repair loop, ffmpeg, S3

**Owns:** turning one scene's Python source into a finished MP4 clip inside
an isolated container, joining the finished clips into one video with no
re-encoding, and getting the result onto S3.

**Exposes** two functions the `server/` component calls:

```go
// Render runs src inside a manim-worker container and returns the path of
// the finished clip, or a RenderError with the container's stderr so the
// caller can send it back for repair. Never blocks longer than timeout.
func Render(ctx context.Context, src string, workDir string) (clipPath string, err *RenderError)

type RenderError struct {
    Stage     string // "precheck" | "container" | "timeout"
    Traceback string // stderr, trimmed
}

// RenderWithRepair wraps Render with up to 3 attempts for one scene, calling
// back into /codegen (via a function the server hands it) with the previous
// source + traceback.
func RenderWithRepair(ctx context.Context, scene Scene, codegen CodegenFunc) (clipPath string, err *RenderError)

// Concat joins finished clips in order (stream copy, no re-encode) and
// uploads the result to S3, returning its public/presigned URL.
func Concat(ctx context.Context, clipPaths []string, outKey string) (videoURL string, err error)
```

`server/` calls `RenderWithRepair` once per scene, concurrently, then calls
`Concat` once all of them have resolved (succeeded or exhausted repair).

**Fakes:** the `/codegen` call, by passing in a function that returns
`samples/product_rule_scenes.py`. `Render` and `Concat` are testable without
Python running.

### What "done" looks like

- The `manim-worker` Docker image is built — Python 3.12 + Manim CE + LaTeX +
  `dvisvgm` + ffmpeg — and a hand-written scene with `MathTex` renders to MP4
  **inside a container run from that image**. A host-only LaTeX check tells
  you nothing about whether the image is right.
- `samples/product_rule_scenes.py` exists, renders inside `manim-worker`, and
  uses relative positioning only. It's the codegen cheatsheet seed, the
  Manim-doc corpus seed, and this component's own test fixture.
- Static pre-check runs **before `docker run`, not instead of container
  isolation** — parses as Python (shell out to
  `python -c "import ast,sys; ast.parse(sys.stdin.read())"`), rejects any
  import that isn't `manim` or stdlib, rejects `os.system`, `subprocess`,
  `open(` for writing, `__import__`, `eval`, `exec`. Cheap, and it turns most
  failures into a fast traceback instead of a slow container spin-up that
  fails anyway.
- Each scene runs in its own container: `--network none`, `--memory 1g
  --cpus 1`, a per-job temp dir mounted at `/work`. Hard per-scene timeout via
  `context.Context`, default 120s, killing the whole container on expiry, not
  just a process.
- Concurrency cap: a buffered channel as a semaphore, sized from
  `RENDER_CONCURRENCY`, shared across all scenes currently rendering
  (possibly across more than one job at once).
- `Concat` uses the ffmpeg concat demuxer with `-c copy` — no re-encoding.
  This only works because every scene renders at the same resolution/fps;
  that's a job-level constant, not a per-scene choice, so concat never
  silently breaks.
- If a scene exhausts all 3 repair attempts, **drop that scene, don't fail
  the job.** `Concat` runs on whatever clips did finish. Zero surviving clips
  means no `Concat` call at all — the job resolves to explanation-only.
- Final MP4 uploads to `s3://<RENDER_BUCKET>/renders/<hash>.mp4`. Local dev
  points `S3_ENDPOINT` at MinIO; the same code talks to real AWS elsewhere.
- Bucket lifecycle rule (14-day expiry on the `renders/` prefix) configured
  once, by hand, not in code. It sits behind Redis's 7-day cache TTL so a
  cache entry never outlives its video.
- `-ql` during development, `-qm` behind a flag for production — passed
  identically to every scene in a job.

### Traps

- manim writes to `media/videos/<scriptname>/<quality>/<SceneName>.mp4` by
  default. Use `--media_dir` to a per-job temp dir inside the container and
  `-o` for the filename, then move the result out via the mounted volume.
- A scene that renders but is 0.5s long because every `play` failed silently
  is a "success" as far as the exit code goes. Check the output file is
  above some minimum size and, if cheap, use `ffprobe` for duration.
- `-c copy` requires byte-identical codec parameters across every input clip.
  One scene rendered at a different quality flag than its siblings breaks
  the whole concat, sometimes silently — the fix is keeping the quality flag
  a single constant, never per-scene.
- Building `manim-worker` takes real minutes. Build it once, early, rather
  than discovering the build time when a render is already blocked on it.

---

## `server/` — Go, HTTP API, Redis, job + scene orchestration

**Owns:** the public API, the job lifecycle, the cache, and fanning a
storyboard's scenes out to render concurrently before joining them back into
one job. Contains no interesting rendering logic — that's `render/`'s job —
it's a dispatcher.

**Exposes:** the HTTP API in CONTRACTS §2, on `:8080`.

**Fakes:** everything at first (`GET /api/jobs/{id}` returns a hardcoded
`done` job, `scenes_total` hardcoded to 1), then swaps in real calls to
`agent/` and `render/` as each becomes available.

### What "done" looks like

- `POST /api/jobs` returns in under 50ms with a job ID. All work happens in a
  goroutine after the response is written.
- Job record in Redis at `job:<id>` updated at every stage transition,
  including `scenes_total` and `scenes_done` as scenes finish. The client
  sees `transcribing` → `explaining` → `generating` → `rendering` →
  `concatenating` → `uploading` → `done`.
- **`explanation` is written to Redis the instant `/explain` returns**,
  before any scene work starts. This is the most important line in the
  server.
- Once `/explain` returns N scenes, fan out N goroutines bounded by a
  semaphore, each calling `render/`'s `RenderWithRepair`. Wait for all N
  (success or exhausted repair) before calling `render/`'s `Concat`.
- Cache key exactly as CONTRACTS §3 — **includes the `guardrails` flag.**
  `normalize()` has a unit test with a multi-line, mixed-case, tab-indented
  input.
- On cache hit: set `status: done`, `cached: true`, `video_url` and
  `explanation` both from `cache:<hash>`, and return. Skip everything after
  `/vision`.
- On successful upload: write `cache:<hash>` (video URL **and** explanation)
  **and** update `job:<id>`. On failure (zero scenes survived repair): update
  `job:<id>` only, `video_url: null`, `explanation` untouched — it was
  already written back in the `/explain` step.
- CORS `*` on every route including 404s and 5xxs.
- Strip the `data:image/png;base64,` prefix before sending to `/vision`.

### Traps

- Redis TTL on `job:<id>` must be *reset* on every update or the job expires
  mid-render, especially with per-scene repair possibly running multiple
  times in parallel.
- If the fan-out goroutine group panics, the job sits in `rendering` forever.
  `defer recover()` on every goroutine → mark that scene failed, don't let
  one panicking scene take down the others' results.
- `PromptVersion` is a `const` in `internal/cache/key.go` — shared
  responsibility to bump, owned file.
- A cache hit must return the explanation too, not just the video — an easy
  thing to half-implement.

---

## `client/` — Angular, extension panel + web app

**Owns:** everything the user sees, across two shells: the Chrome
extension's side panel and the standalone paste-a-screenshot web app.

**Exposes:** nothing. Consumes CONTRACTS §2.

**Fakes:** the server, with a small stub that returns the `GET
/api/jobs/{id}` shapes on a timer — `queued` for 1s, `explaining` for 2s,
`generating` with an explanation and `scenes_total: 3` for 5s, incrementing
`scenes_done` every couple seconds, `done` with a sample MP4 URL. Build the
whole UI against that, then point it at the real server.

### Structure

One Angular workspace, three projects, so there's one UI to build, not two:

- `shared/` — a library: the API client, the panel component (markdown
  render, progress indicator, guardrails toggle, video player), TypeScript
  models matching CONTRACTS.
- `extension-panel/` — thin shell that hosts the shared panel inside the
  MV3 side panel.
- `web-app/` — thin shell that adds a paste/drop zone in front of the same
  shared panel.

Both shells build to static output with `ng build`; nothing is loaded from a
CDN at runtime — extensions can't load remote scripts, and the web app works
offline-first the same way.

### What "done" looks like

- Manifest V3. `commands` for the hotkey, `sidePanel` API for the panel,
  `activeTab` + `tabs` permissions for `captureVisibleTab`.
- Hotkey → capture → panel opens → question field focused, guardrails toggle
  visible → user types or just hits Enter → `POST /api/jobs` (with
  `guardrails` in the body) → poll every 1s.
- **Explanation renders the instant it's non-null.** Markdown → HTML via a
  small library, vendored into the Angular build — no CDN.
- Status line shows the current stage in plain words, and once scenes start:
  "Rendering scene 2 of 3…" using `scenes_done`/`scenes_total`.
- Video `<video controls autoplay muted>` appears when `video_url` is set —
  it's a direct S3 (or MinIO) URL, not proxied through the server. No reload.
- **`done` with `video_url: null`:** show the explanation, plus a quiet line:
  "The animation didn't render this time." Not red. Not an error box.
- `failed`, or a 180s timeout with nothing back: a plain message and a retry
  button.
- `chrome://` / Web Store: "Chrome doesn't allow capture on this page."
  instead of a silent nothing.
- Web app: drop zone or `Ctrl+V`, same question field, same guardrails
  toggle, same shared panel component — not a re-implementation.

### Traps

- `captureVisibleTab` needs the `activeTab` permission *and* a user gesture.
  The hotkey counts as one. A `setTimeout` after it does not.
- Side panel and background script can't share memory. Pass the job ID via
  `chrome.storage.session` or a message.
- Autoplay with sound is blocked. `muted` or it won't start.
- An Angular app's default production bundle is not tiny — keep the
  `extension-panel` project's build lean (no router, no unused Material
  modules) so the side panel opens instantly. Bundle size is a UX bug here,
  not a nitpick.

---

## Integration points, in order of when they go live

| Order | What connects |
|---|---|
| 1 | `client` → `server` (`/vision` + `/explain` real, per-scene render on a fake scene) |
| 2 | `server` → `agent` (vision + explain) |
| 3 | `server` → `render` → real per-scene `/manim-docs` + `/codegen`, fanned out concurrently, concatenated, uploaded to S3 |
| 4 | Cache verified across two independent clients; guardrails mode verified end-to-end |

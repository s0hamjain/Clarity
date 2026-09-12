# Work split

Four people, four lanes, four laptops. Each lane is one directory, one owner,
and one set of things it fakes so it's never blocked on anyone else.

Shapes at every boundary are in [../CONTRACTS.md](../CONTRACTS.md). The clock is
in [../AGENTS.md](../AGENTS.md). Read both before starting.

**Owners: TBD** — fill in names here when you claim a lane.

| Lane | Owner | Language | Directory |
|---|---|---|---|
| `agent/` | | Python | `agent/` |
| `render/` | | Go | `server/internal/render/` |
| `server/` | | Go | `server/` (everything but `render/`) |
| `client/` | | HTML / JS | `client/` |

`render/` and `server/` are both Go and live in the same module. They are
separate lanes because they're separate problems: one is a subprocess wrangler
with a repair loop, the other is an HTTP server with a queue. They meet at one
Go function signature (below).

---

## `agent/` — Python, all three Claude calls

**You own:** every line that talks to the Anthropic API. The three prompts. The
determinism decision. The cache-collision experiment.

**You expose:** a FastAPI service on `:8000` with `POST /vision`, `/explain`,
`/codegen`, `GET /healthz`. Request/response shapes in CONTRACTS §1.

**You fake:** nothing — you're a leaf. You can test with `curl` and a PNG from
day one.

**Others fake you** until Sprint 2 by hardcoding responses in Go.

### What "done" looks like

- All three endpoints return schema-valid JSON via structured outputs. No
  markdown fences, no prose. Go parses your output with zero cleanup.
- `/vision` returns *verbatim* text. If you catch yourself adding "the problem
  asks..." to the prompt, stop — that's the thing that breaks the cache.
- `/codegen` output always has `class GeneratedScene(Scene)`. Always. Assert it
  before returning.
- Repair path: `/codegen` with `previous_source` + `traceback` returns fixed
  source, not a rewrite from scratch.
- `PromptVersion` bumped every time you touch a prompt. It lives in Go — tell the
  `server/` owner, or just make the one-line commit yourself.

### Constraints to bake into the codegen prompt

- Manim **Community Edition** imports only: `from manim import *`. Nothing else.
- Relative positioning only: `next_to`, `arrange`, `to_edge`, `shift` by
  fractions of `config.frame_width`. **Never literal coordinates.** The frame
  is 14.2 units wide and objects that wander off it exit zero — we can't catch
  that programmatically.
- One scene. `beats` are moments in it, not separate scenes.
- `narration` → `Text(...)` at `to_edge(DOWN)`, replaced per beat with
  `Transform` or `FadeOut`/`FadeIn`.
- Include `samples/product_rule_scenes.py` verbatim in the prompt as the
  reference to imitate. It's the cheatsheet.

### The two decisions that are yours

1. **Determinism** (CONTRACTS §1 OPEN). Run the six-screenshot experiment in
   Sprint 1, then pick: Opus 5 + structured outputs + low effort, or
   `claude-haiku-4-5` with `temperature=0` for `/vision` only. Decide by end of
   Sprint 2.
2. **Storyboard bias.** The prompt should push toward things text can't show.
   Ask: would this beat be better as a sentence? If yes, cut it.

---

## `render/` — Go, Manim subprocess, repair loop

**You own:** turning a string of Python into an MP4, or a traceback.

**You expose** one function the `server/` lane calls:

```go
// Render writes src to a temp .py, runs manim, and returns the path of the
// finished MP4. On failure it returns the traceback (stderr) in err so the
// caller can send it back for repair. Never blocks longer than timeout.
func Render(ctx context.Context, src string, outName string) (mp4Path string, err *RenderError)

type RenderError struct {
    Stage     string // "precheck" | "manim" | "timeout"
    Traceback string // stderr, trimmed
}
```

The repair loop itself is **yours too** — `RenderWithRepair(ctx, storyboard, codegen func(...))` that tries up to 3 times, calling back into `/codegen` via a function the server hands you. That way the server just calls one thing.

**You fake:** the `/codegen` call, by passing in a function that returns
`samples/product_rule_scenes.py`. You never need Python running to test.

### What "done" looks like

- **Sprint 1, before anything else:** LaTeX smoke test passes ([SETUP.md](SETUP.md) §5).
- `samples/product_rule_scenes.py` exists, renders, and uses relative positioning
  only. **You write this.** It doesn't exist. It's on the critical path because
  `agent/` needs it for the codegen prompt.
- Static pre-check runs before `manim` does: parses as Python (`go/ast` won't do
  it — shell out to `python -c "import ast,sys; ast.parse(sys.stdin.read())"`),
  rejects any import that isn't `manim` or stdlib, rejects `os.system`,
  `subprocess`, `open(` for writing, `__import__`, `eval`, `exec`. Banned list is
  a slice in one file; make it easy to extend.
- Hard per-render timeout via `exec.CommandContext`. Default 120s. Kill the
  process group, not just the process — manim spawns children.
- Concurrency cap: a buffered channel as a semaphore. Size from
  `RENDER_CONCURRENCY`.
- Output lands at `<RENDER_DIR>/<outName>.mp4`. Temp `.py` and manim's `media/`
  scratch dir are cleaned up on both success and failure.
- `-ql` during dev, `-qm` behind a flag for the demo.

### Traps

- manim writes to `media/videos/<scriptname>/<quality>/<SceneName>.mp4` by
  default. Use `--media_dir` to a per-job temp dir and `-o` for the filename, then
  move the result. Don't go hunting for it.
- A scene that renders but is 0.5s long because every `play` failed silently is
  a "success." Check the output file is > some minimum size and, if cheap, use
  `ffprobe` for duration.

---

## `server/` — Go, HTTP API, Redis, job queue

**You own:** the public API, the job lifecycle, the cache, and calling everything
else in the right order. You contain no interesting logic. You are a dispatcher.

**You expose:** the HTTP API in CONTRACTS §2, on `:8080`.

**You fake:**
- **Sprint 1:** everything. `GET /api/jobs/{id}` returns a hardcoded `done` job.
- **Sprint 2:** `/codegen` and `Render` — hardcoded source, and a sample MP4
  copied to `renders/<hash>.mp4`.
- **Sprint 3:** nothing.

### What "done" looks like

- `POST /api/jobs` returns in under 50ms with a job ID. All work happens in a
  goroutine after the response is written.
- Job record in Redis at `job:<id>` updated at every stage transition. The
  client sees `transcribing` → `explaining` → `generating` → `rendering` → `done`.
- **`explanation` is written to Redis the instant `/explain` returns**, before
  `/codegen` is called. This is the most important line in the server.
- Cache key exactly as CONTRACTS §3. `normalize()` has a unit test with a
  multi-line, mixed-case, tab-indented input.
- On cache hit: set `status: done`, `cached: true`, `video_url` from cache, and
  return. Skip `/explain` and everything after. *(Note: this means a cache hit
  has no `explanation` unless we also cache it — **OPEN**, probably cache it too
  under `cache:<hash>:explanation`. Decide in Sprint 2.)*
- On render success: write `cache:<hash>` **and** update `job:<id>`. On failure:
  update `job:<id>` only, `status: failed`, `explanation` untouched.
- `/renders/` served with `http.ServeFile` (range requests come free).
- CORS `*` on every route including 404s and 5xxs.
- Strip the `data:image/png;base64,` prefix before sending to `/vision`.

### Traps

- Redis TTL on `job:<id>` must be *reset* on every update or the job expires
  mid-render.
- If the goroutine panics, the job sits in `rendering` forever. `defer recover()`
  → mark `failed`.
- `PromptVersion` is a `const` in `internal/cache/key.go`. It is the team's
  shared responsibility to bump it, but it's your file.

---

## `client/` — Chrome extension + paste page

**You own:** everything the user sees.

**You expose:** nothing. You consume CONTRACTS §2.

**You fake:** the server, with a 15-line Node/Python stub that returns the
`GET /api/jobs/{id}` shapes on a timer — `queued` for 1s, `explaining` for 2s,
`generating` with an explanation for 5s, `done` with a sample MP4 URL. Build the
whole UI against that. Point it at the real server in Sprint 2.

### What "done" looks like

- Manifest V3. `commands` for the hotkey, `sidePanel` API for the panel,
  `activeTab` + `tabs` permissions for `captureVisibleTab`.
- Hotkey → capture → panel opens → question field focused → user types or just
  hits Enter → `POST /api/jobs` → poll every 1s.
- **Explanation renders the instant it's non-null.** Markdown → HTML with a small
  library (marked.js is fine, vendored, no CDN — extensions can't load remote
  scripts).
- Status line shows the current stage in plain words: "Reading the problem…" /
  "Writing explanation…" / "Rendering animation…" / "Done".
- Video `<video controls autoplay muted>` appears when `video_url` is set. No
  reload.
- **`failed` with an explanation:** show the explanation, plus a quiet line:
  "The animation didn't render this time." Not red. Not an error box.
- `failed` without an explanation, or 180s timeout: a plain message and a retry
  button.
- `chrome://` / Web Store: "Chrome doesn't allow capture on this page." instead
  of a silent nothing.
- Paste page (`client/web/index.html`): drop zone or `Ctrl+V`, same question
  field, same panel UI. Share the JS with the extension — one `panel.js`, two
  shells around it.

### Traps

- `captureVisibleTab` needs the `activeTab` permission *and* a user gesture. The
  hotkey counts as one. A `setTimeout` after it does not.
- Side panel and background script can't share memory. Pass the job ID via
  `chrome.storage.session` or a message.
- Autoplay with sound is blocked. `muted` or it won't start.

---

## Integration points, in order of when they go live

| When | What connects | Who's involved |
|---|---|---|
| Sprint 2 | `client` → real `server` (`/vision` + `/explain` real, render faked) | client, server, agent |
| Sprint 2 | `server` → real `agent` | server, agent |
| Sprint 3 | `server` → real `render` → real `/codegen` | server, render, agent |
| Sprint 4 | second laptop hits the cache | everyone |

If you're blocked on someone, you're not faking hard enough. Fake harder, keep
moving, and tell them what shape you assumed.

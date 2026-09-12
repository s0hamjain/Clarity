# CONTRACTS — draft

**Status: draft, not frozen.** This is the shape we're building against so four
people on four laptops can work without talking. It is expected to change
tonight. When it does: change it *here first*, in a commit that touches nothing
else, and say so in the group chat.

Anything marked **OPEN** is an unmade decision, not an oversight.

---

## 0. What runs where

```
Angular extension panel  ──HTTP──►  Go coordinator  ──HTTP──►  Python agent service ──► Claude API
Angular web app (paste)             :8080                      :8000
                                       │
                                       ├──► Redis              (job status + cache)
                                       ├──► Docker render pool (one container per scene)
                                       ├──► ffmpeg             (concat, no re-encode)
                                       └──► S3                 (finished MP4s)
```

- **Python owns every Claude call.** Vision, explanation, Manim-doc retrieval,
  codegen. Nothing else talks to the Anthropic API.
- **Go owns orchestration.** The job queue, the per-scene fan-out, the cache,
  Docker dispatch, the repair loop, ffmpeg, S3, Redis.
- **Rendering happens per scene, in parallel, each in its own container.** A
  storyboard is 2–5 scenes; Go dispatches all of them at once (bounded by a
  concurrency cap) and only concatenates once every scene has a finished MP4 or
  has exhausted repair.
- The two services talk over plain HTTP on localhost. Both run on the same
  machine for the demo; Docker containers run alongside them.

---

## 1. Python agent service (internal)

Base URL: `http://localhost:8000`. Not exposed to the browser — only Go calls
it.

Every request below carries a `guardrails: bool` (default `false`). It changes
what the model produces, so it flows through every call in the chain and is
part of the cache key (§3).

### `POST /vision`

```json
// request
{ "image_b64": "iVBORw0KG...", "media_type": "image/png", "guardrails": false }

// response
{
  "problem_text": "Differentiate f(x) = x^2 sin(x) using the product rule.",
  "category": "math",
  "confidence": 0.94
}
```

| Field | Notes |
|---|---|
| `problem_text` | **Verbatim transcription.** No paraphrase, no interpretation, no added context. This string gets hashed — any editorializing destroys the cache. |
| `category` | `"math"` \| `"algorithm"` \| `"unknown"` — picks which explainer prompt and which Manim-doc topics are relevant. |
| `confidence` | 0.0–1.0. Below 0.5 Go still proceeds but flags the job. |

If nothing problem-like is on screen: `category: "unknown"`, `problem_text: ""`.
Go fails the job cleanly rather than explaining a screenshot of someone's inbox.

**Note on `image_b64`:** the client sends a full data URL
(`data:image/png;base64,iVBOR...`). **Go strips the `data:...;base64,` prefix**
before calling Python. Python receives raw base64 only.

**OPEN — how do we make this deterministic?** The cache only works if two
students get byte-identical `problem_text`. `temperature`/`top_p`/`top_k` were
removed on Claude Opus 5 and Sonnet 5 and return a **400**. Three options:

1. **Opus 5 + structured outputs + low effort** (current default here).
   Determinism comes from the schema and a tight prompt, not a sampling param.
2. **`claude-haiku-4-5` for this call only.** Haiku 4.5 still accepts
   `temperature`, so literal `temperature=0` is back on the table — and it's
   faster and cheaper, which matters on the one call that sits in front of
   everything else.
3. Accept jitter and lean harder on `normalize()`.

`agent/` owns this decision and should make it by end of Sprint 2, because it's
the thing the cache-collision test in Sprint 1 is actually testing.

### `POST /explain`

```json
// request
{
  "problem_text": "...",
  "category": "math",
  "user_prompt": "why a cosine?",
  "guardrails": false
}

// response
{
  "explanation": "**Step 1.** The product rule says ...",
  "storyboard": {
    "title": "The Product Rule",
    "scenes": [
      {
        "narration": "Two functions multiplied together.",
        "visual": "Show f(x) = x^2 sin(x). Split it into u = x^2 and v = sin(x), each moving to its own side of the frame.",
        "duration_seconds": 8
      },
      {
        "narration": "The derivative is u'v + uv'.",
        "visual": "Transform the split expression into u'v + uv', highlighting each term as it appears.",
        "duration_seconds": 10
      }
    ]
  }
}
```

| Field | Notes |
|---|---|
| `explanation` | Markdown. **This is the product.** Go writes it to Redis the second it lands and the user sees it immediately — long before any video exists. In `guardrails: true`, this walks through method and reasoning and stops short of the final answer (see below). |
| `storyboard.title` | Opening title card. |
| `storyboard.scenes` | **2–5 items.** Each is now an independent Manim `Scene`, rendered as its own container and its own MP4, concatenated in order. This is the change from the single-scene design: it's what makes per-scene parallel rendering possible. |
| `scenes[].narration` | ≤ 90 chars. Becomes **on-screen text**, not audio. |
| `scenes[].visual` | What the scene shows, in relative terms ("below", "next to", "replacing"). Never coordinates. |
| `scenes[].duration_seconds` | 5–15. A budget, not a promise. |

**Bias every scene toward what text can't do.** A scene that just restates
algebra is wasted — the explanation already did it better, in less time.
Prefer: a function and its derivative plotted together, a pointer walking a
list, a shape transforming, a quantity growing.

**Guardrails mode** (`guardrails: true`): the explanation and every scene teach
the *method*, not the *result*. For math, that means showing the rule and
working the setup, then leaving the final substitution as a step the student
does themselves. For code, that means walking the algorithm's logic and data
flow without emitting a complete, copy-pasteable solution — a skeleton with the
key decision points named but not filled in. This is the honest answer to "is
this a cheating tool?" — a real toggle, not a disclaimer.

### `POST /manim-docs`

Retrieval, not generation. Given one scene's intent, returns the Manim CE
snippets most relevant to building it — grounding codegen in real, working API
usage instead of letting it guess.

```json
// request
{ "scene": { "index": 0, "narration": "...", "visual": "Show f(x) = x^2 sin(x), split into u and v." } }

// response
{
  "snippets": [
    { "topic": "MathTex + relative positioning", "source": "manim docs: Text and Fonts", "code": "eq = MathTex(r\"x^2 \\sin(x)\")\nu = MathTex(\"u\").next_to(eq, LEFT)\n..." }
  ]
}
```

Implementation: a small curated corpus — the official Manim CE docs' worked
examples plus `samples/product_rule_scenes.py` — chunked, embedded once at
startup, retrieved by cosine similarity against the scene's `visual` text.
Top 2–3 snippets. This does not need to be fancy; a static corpus of ~50–100
chunks and an in-memory embedding index is plenty for one weekend. **`agent/`
owns the corpus and the retrieval; it's a library call inside the service, not
a second network hop.**

**OPEN — embeddings model.** Anthropic doesn't currently offer a public
embeddings endpoint; use a small local model (e.g. `sentence-transformers`,
CPU-only, no API key) so retrieval has zero added latency and zero extra cost.
Fall back to keyword/BM25 match against snippet titles if that's not installed
in time — worse recall, zero setup risk.

### `POST /codegen`

Per scene, so a scene can be regenerated or repaired without touching its
siblings.

```json
// request
{
  "scene": { "index": 0, "narration": "...", "visual": "..." },
  "manim_snippets": [ { "topic": "...", "code": "..." } ],
  "previous_source": null,
  "traceback": null
}

// response
{ "manim_source": "from manim import *\n\nclass GeneratedScene(Scene):\n    ...", "scene_class": "GeneratedScene" }
```

On a repair attempt, Go sends back the source that failed for *this scene*
plus its traceback and gets corrected source back. Same endpoint, same shape.

| Field | Notes |
|---|---|
| `manim_source` | Complete runnable Manim CE Python. One file. No imports beyond `manim` and the stdlib. |
| `scene_class` | **Always the literal `"GeneratedScene"`.** Every scene renders in its own container with its own temp directory, so there's no collision risk from reusing the name — Go never needs to parse source to find a class. Anything else in this field = treat as a failure, trigger repair. |

Constraints baked into the prompt:

- Manim **Community Edition** imports only: `from manim import *`. Nothing
  else beyond the stdlib.
- Relative positioning only: `next_to`, `arrange`, `to_edge`, `shift` by
  fractions of `config.frame_width`. **Never literal coordinates.**
- **Every scene must be rendered with the same resolution, frame rate, and
  background** (Go fixes the manim quality flag identically across all scenes
  in a job) — the ffmpeg concat step (§4) depends on this and cannot fix a
  mismatch after the fact.
- Include the retrieved `manim_snippets` verbatim in the prompt as the
  reference to imitate.

### Models

Default `claude-opus-5` for all calls, with adaptive thinking
(`thinking={"type": "adaptive"}` — on by default on Opus 5) and
`output_config={"effort": "low"}` on `/vision`, which is a transcription task
and doesn't need depth.

Use structured outputs (`output_config={"format": ...}`) so every JSON shape
above is schema-enforced rather than requested politely. **Do not** use
assistant prefill to force JSON — it returns a 400 on Opus 5.

`claude-haiku-4-5` is the fallback for `/vision` if we go with determinism
option 2 above.

---

## 2. Public HTTP API (Go)

Base URL in dev: `http://localhost:8080`. `Access-Control-Allow-Origin: *` on
every route — the extension panel and the web app are both cross-origin.

### `POST /api/jobs`

Creates a job. **Returns before any Claude call happens.**

```json
// request
{
  "image": "data:image/png;base64,iVBORw0KG...",
  "user_prompt": "why does the second term have a cosine?",
  "guardrails": false,
  "source": "extension"
}

// 202 Accepted
{ "job_id": "j_7f3a9c21" }
```

`image` is exactly what `chrome.tabs.captureVisibleTab` returns; the web app
sends the same shape from a paste/drop. `user_prompt` may be `""` — it's still
part of the cache key. `guardrails` defaults to `false` and is also part of the
cache key. `source` is `"extension"` or `"web"`, telemetry only.

Errors: `400` bad body, `413` image over 8 MB, `503` queue full.

### `GET /api/jobs/{job_id}`

The only endpoint the client polls. Poll every **1s**, give up at **180s**.

```json
{
  "job_id": "j_7f3a9c21",
  "status": "rendering",
  "problem_hash": "a3f9c1d2e4b57680",
  "explanation": "**Step 1.** ...",
  "scenes_total": 3,
  "scenes_done": 2,
  "video_url": null,
  "cached": false,
  "error": null,
  "updated_at": "2026-09-12T02:14:03Z"
}
```

`404` on unknown or expired job ID.

| Status | Meaning | `explanation` | `scenes_done` | `video_url` |
|---|---|---|---|---|
| `queued` | accepted, not started | null | 0 | null |
| `transcribing` | `/vision` in flight | null | 0 | null |
| `explaining` | `/explain` in flight | null | 0 | null |
| `generating` | per-scene manim-docs + codegen in flight | **set** | 0 | null |
| `rendering` | scenes rendering in Docker, concurrently | set | 0..N | null |
| `concatenating` | ffmpeg joining finished scene clips | set | N | null |
| `uploading` | pushing final MP4 to S3 | set | N | null |
| `done` | video live at `video_url` | set | N | **set** |
| `failed` | ended early | **set if it got that far** | 0..N | null |

**Two rules the client must honor:**

1. **Render `explanation` the instant it is non-null**, whatever the status
   says. Do not wait for `done`. The entire architecture exists to make this
   moment early — a UI that waits for the video makes a working system feel
   broken.
2. **`failed` is not empty.** If `explanation` is non-null on a failed job,
   show it with a note that the animation didn't render. That's the difference
   between a dead demo and a working one.

`scenes_total` / `scenes_done` drive a progress indicator ("rendering scene 2
of 3") — nicer than a spinner, and it's data Go already has.

`cached: true` means the whole pipeline was skipped on a hash hit — usually
lands on the first poll.

### `GET /renders/{hash}.mp4`

**Not served by Go.** `video_url` is a presigned or public S3 URL directly —
the client streams from S3, not through the coordinator. Go's only job here is
putting the right URL in Redis and in the job record.

*(Local dev without AWS creds: point `S3_ENDPOINT` at a local
[MinIO](https://min.io) container — it speaks the S3 API, so nothing in Go or
the client needs to know the difference. See [docs/SETUP.md](docs/SETUP.md).)*

### `GET /healthz`

`{"ok": true, "redis": true, "docker": true, "s3": true, "agent": true}`.
`docker`, `s3`, and `agent` checked once at boot, not per request.

---

## 3. The cache key

```
sha256(PromptVersion + "||" + normalize(problem_text) + "||" + normalize(user_prompt) + "||" + guardrails)[:16]
```

Hex digest, **first 16 chars**. `normalize` = lowercase, trim, collapse every
run of whitespace (including newlines) to one space. Nothing else — no
punctuation stripping, no unicode folding. Implemented in Go; Python doesn't
need it. `guardrails` is serialized as the literal string `"true"` or
`"false"`.

Four details, each load-bearing:

- **The user's prompt is in the key.** Same problem, different question is a
  different video.
- **Guardrails is in the key.** A guided-mode explanation and an answer-mode
  explanation are different artifacts for the same problem — they must not
  collide.
- **`PromptVersion` is a hand-bumped constant in the Go server.** Bump it
  whenever *any* prompt text changes — including prompts that live in Python.
  Without it, a prompt improvement appears to do nothing because Redis keeps
  serving what the old prompt produced. This is the bug that eats an hour at
  3am.
- **Matching is exact.** Hashing has no notion of "close enough." Normalization
  is the only defense against transcription jitter between two students at
  different zoom levels.

### Redis keys

| Key | Value | TTL |
|---|---|---|
| `job:<job_id>` | the full job JSON above | 24h |
| `cache:<problem_hash>` | `{ "video_url": "...", "explanation": "..." }` | 7d |

Checked after `/vision`. **A cache hit returns both the video and the
explanation** — the OPEN question from the earlier draft is resolved: yes,
cache the explanation too, otherwise a cache hit shows a video with nothing to
read while it loads.

Written only after a successful render **and** successful S3 upload. **Never
written on failure** — a failed render must not poison the next student.

---

## 4. The render pipeline

### Per-scene fan-out

Once `/explain` returns a storyboard with N scenes, Go fires N goroutines,
bounded by a semaphore sized from `RENDER_CONCURRENCY`. Each goroutine
independently: calls `/manim-docs`, calls `/codegen`, dispatches to a Docker
worker, and retries up to 3 times through the repair loop **for that scene
only** on failure. Go waits for all N to finish (or exhaust retries) before
moving on.

```go
// Render runs one scene's source in an isolated container and returns the
// path of the finished clip, or a RenderError with the container's stderr so
// the caller can send it back for repair. Never blocks longer than timeout.
func Render(ctx context.Context, src string, workDir string) (clipPath string, err *RenderError)

type RenderError struct {
    Stage     string // "precheck" | "container" | "timeout"
    Traceback string // stderr, trimmed
}

// RenderWithRepair wraps Render with up to 3 attempts, calling back into
// /codegen (via a function Go hands it) with the previous source + traceback.
func RenderWithRepair(ctx context.Context, scene Scene, codegen CodegenFunc) (clipPath string, err *RenderError)
```

If a scene exhausts all 3 repair attempts, **that scene is dropped, not the
whole job** — ffmpeg concatenates whatever scenes did succeed. A 2-of-3-scene
video beats no video. If *zero* scenes succeed, the job still resolves to
`done`-without-video: explanation shown, note that the animation didn't
render. **Never fail the explanation because rendering failed.**

### Docker isolation

Each scene renders in its own container from a prebuilt image
(`manim-worker`, built once in Sprint 1 — Python 3.12 + Manim CE + LaTeX +
ffmpeg baked in, so no per-render install cost).

```sh
docker run --rm \
  --network none \
  --memory 1g --cpus 1 \
  -v <tmpdir>:/work \
  manim-worker \
  manim -qm /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media
```

- `--network none`: generated code has no reason to reach the network, and
  this is defense in depth on top of the static pre-check.
- `--memory` / `--cpus`: one scene's bad loop can't starve the others.
- The static pre-check (AST parse, banned imports) **still runs before
  `docker run`**, even with container isolation — cheap, and it turns most
  failures into a fast, informative traceback instead of a slow container
  spin-up that fails anyway.
- Hard per-scene timeout via `context.Context` + `docker kill` on expiry.
  Default 120s.

### FFmpeg concat — no re-encoding

Because every scene in a job renders at the same resolution/fps (Go passes the
identical manim quality flag to all of them), the finished clips can be joined
with the concat demuxer and a stream copy — no re-encode, effectively free:

```sh
# concat_list.txt:
# file 'scene0.mp4'
# file 'scene1.mp4'
ffmpeg -f concat -safe 0 -i concat_list.txt -c copy final.mp4
```

**Trap:** `-c copy` requires identical codec parameters across inputs. If a
repair attempt or a fallback path ever produces a scene at a different
resolution/fps than its siblings, `-c copy` fails or produces broken output.
Guard this in one place: the manim invocation always takes its quality flag
from a single job-level constant, never per-scene.

### Storage — S3

Final MP4 uploads to `s3://<RENDER_BUCKET>/renders/<hash>.mp4`. `video_url` in
the job record and the cache is either a public URL (bucket configured for
public read on this prefix — simplest for a demo) or a presigned GET URL
(safer, adds one more moving part). **Default to public-read for the demo;
presigned URLs are a one-line upgrade if privacy becomes a concern.**

**Deleting expired videos:** no code needed. A bucket lifecycle rule
(`renders/` prefix, expire after **14 days**) handles it natively. Redis's
`cache:<hash>` TTL is **7 days** — shorter than the S3 lifecycle — so the cache
always stops pointing at a video before S3 removes it. No reaper process, no
keyspace-notification listener, nothing to get wrong at 3am.

---

## 5. Deferred, not deleted

These were cut for time. They are listed so nobody re-litigates them at 3am,
and so we know where they'd slot back in.

| Thing | Why cut | Where it goes back in |
|---|---|---|
| Real vector DB for Manim-doc retrieval (Pinecone/pgvector/etc.) | An in-memory embedding index over ~100 chunks is enough for one corpus that never grows during the hackathon. | Swap the in-memory index in `agent/manim_docs.py` for a real store if the corpus needs to scale. |
| Narration audio (TTS) | Audio forces re-encoding at concat time, which conflicts with the no-re-encode concat design in §4. | `scenes[].narration` is already the right field; add a TTS step before concat and drop `-c copy`. |
| Verification that the explanation or generated code is *correct* | No time, no clear mechanism. | Would need a second model pass or test execution — open research question, not a Sprint 5 task. |
| Accounts, history, anything persistent per user | Out of scope for a demo. | — |

---

## 6. Known risks

**Cache hit rate is unmeasured.** The whole caching story depends on two
students producing byte-identical transcriptions of the same problem at
different zoom levels and window widths. **Test this in Sprint 1:** screenshot
one problem six different ways, count distinct hashes.

**Generated Manim fails two ways.** Code that *crashes* is recoverable — the
per-scene repair loop catches it. Code that *runs but produces a bad video*
(overlapping text, objects off the 14.2-unit frame) exits zero and cannot be
detected programmatically. Mitigation is prompt constraints plus
retrieval-grounded snippets (§1 `/manim-docs`).

**LaTeX, inside the Docker image now, not the host.** `MathTex` needs a TeX
distribution and `dvisvgm` **baked into `manim-worker`**. If the image is
built without them, every math scene fails inside the container and the error
is easy to miss because it's not on the host. **Verify the image, not just the
host, in Sprint 1.**

**Concat codec mismatch.** See §4's trap. One scene rendered at a different
quality setting than its siblings breaks `-c copy` silently or loudly.

**S3 / AWS credentials mid-hackathon.** Not everyone will have (or want to
share) AWS keys at 9pm on a Friday. **Default local dev to MinIO** ([SETUP.md](docs/SETUP.md))
so nobody is blocked on an AWS account; point `S3_ENDPOINT` at real AWS only on
the presenting machine for the demo.

**Docker adds a cold-start tax.** Building `manim-worker` (Python + Manim +
LaTeX) takes real minutes the first time. **Build it once, in Sprint 1,
immediately** — don't discover the build time at 3am when a render is blocked
on it.

**Per-scene parallelism multiplies API calls.** A 4-scene storyboard now means
4× `/manim-docs` + 4× `/codegen` calls instead of 1. Latency to *first* scene
starting render is unaffected (they fire concurrently), but total token spend
per job goes up. Not a correctness risk, but worth knowing before assuming the
old single-call latency numbers still hold.

**Guardrails mode has no automated check.** Nothing stops the model from
leaking the final answer anyway in "guided" mode — it's a prompt instruction,
not a filter. Spot-check it on a few real problems before the demo; don't
trust it on faith.

**Screenshot limits.** `captureVisibleTab` grabs the visible viewport only —
scrolled-off content is lost — and refuses on `chrome://` pages and the Web
Store.

**Nothing checks correctness.** A confidently wrong animated explanation is
worse than none, because the production values make it credible.

**Privacy.** The screenshot captures the whole viewport — tabs, browser
chrome, possibly the student's name — and it goes to a third-party API and
onto S3.

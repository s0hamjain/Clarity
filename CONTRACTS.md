# CONTRACTS — draft

**Status: draft, not frozen.** This is the shape we're building against so four
people on four laptops can work without talking. It is expected to change
tonight. When it does: change it *here first*, in a commit that touches nothing
else, and say so in the group chat.

Anything marked **OPEN** is an unmade decision, not an oversight.

---

## 0. What runs where

```
Chrome extension ──HTTP──► Go server ──HTTP──► Python agent service ──► Claude API
  (or paste page)          :8080               :8000
                             │
                             ├──► Redis            (job status + cache)
                             ├──► manim subprocess (render)
                             └──► ./renders/*.mp4  (static file serving)
```

- **Python owns every Claude call.** All three of them. Nothing else talks to
  the Anthropic API.
- **Go owns the render pipeline and job orchestration.** The queue, the worker
  pool, the cache lookup, the repair loop, the `manim` subprocess, serving the
  MP4.
- The two talk over plain HTTP on localhost. Both run on the same machine for
  the demo.

---

## 1. Python agent service (internal)

Base URL: `http://localhost:8000`. Not exposed to the browser — only Go calls it.

### `POST /vision`

```json
// request
{ "image_b64": "iVBORw0KG...", "media_type": "image/png" }

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
| `category` | `"math"` \| `"algorithm"` \| `"unknown"` — picks which storyboard prompt `/explain` uses. |
| `confidence` | 0.0–1.0. Below 0.5 Go still proceeds but flags the job. |

If nothing problem-like is on screen: `category: "unknown"`, `problem_text: ""`.
Go fails the job cleanly rather than explaining a screenshot of someone's inbox.

**Note on `image_b64`:** the client sends a full data URL
(`data:image/png;base64,iVBOR...`). **Go strips the `data:...;base64,` prefix**
before calling Python. Python receives raw base64 only. Pick one side to do this
and it's Go — decided so nobody does it twice.

**OPEN — how do we make this deterministic?** The cache only works if two
students get byte-identical `problem_text`. The original plan was
`temperature=0`. That is no longer possible: `temperature` / `top_p` / `top_k`
were removed on Claude Opus 5 and Sonnet 5 and return a **400**. Three options:

1. **Opus 5 + structured outputs + low effort** (current default here).
   Determinism comes from the schema and a tight prompt, not a sampling param.
2. **`claude-haiku-4-5` for this call only.** Haiku 4.5 still accepts
   `temperature`, so literal `temperature=0` is back on the table — and it's
   faster and cheaper, which matters on the one call that sits in front of
   everything else.
3. Accept jitter and lean harder on `normalize()`.

The `agent/` lane owns this decision and should make it by end of Sprint 2,
because it's the thing the cache-collision test in Sprint 1 is actually testing.

### `POST /explain`

```json
// request
{ "problem_text": "...", "category": "math", "user_prompt": "why a cosine?" }

// response
{
  "explanation": "**Step 1.** The product rule says ...",
  "storyboard": {
    "title": "The Product Rule",
    "duration_seconds": 30,
    "beats": [
      {
        "narration": "Two functions multiplied together.",
        "visual": "Show f(x) = x^2 sin(x). Split it into u = x^2 and v = sin(x), moving each to its own side of the frame."
      },
      {
        "narration": "The derivative is u'v + uv'.",
        "visual": "Transform the split expression into u'v + uv', highlighting each term as it appears."
      }
    ]
  }
}
```

| Field | Notes |
|---|---|
| `explanation` | Markdown. **This is the product.** Go writes it to Redis the second it lands and the user sees it immediately — long before any video exists. |
| `storyboard.title` | Opening title card. |
| `storyboard.duration_seconds` | 15–45. A budget, not a promise. |
| `storyboard.beats` | **2–5 items.** Beats are moments inside *one* scene, not separate scenes. |
| `beats[].narration` | ≤ 90 chars. Becomes **on-screen text**, not audio. Short enough to read. |
| `beats[].visual` | What the animation does, in relative terms ("below", "next to", "replacing"). Never coordinates. |

**Bias the storyboard toward what text can't do.** A beat that just restates
algebra is wasted — the explanation already did it better, in less time. Prefer:
a function and its derivative plotted together, a pointer walking a list, a shape
transforming, a quantity growing.

### `POST /codegen`

```json
// request
{ "storyboard": { ... }, "previous_source": null, "traceback": null }

// response
{ "manim_source": "from manim import *\n\nclass GeneratedScene(Scene):\n    ...", "scene_class": "GeneratedScene" }
```

On a repair attempt, Go sends back the source that failed plus the traceback and
gets corrected source. Same endpoint, same shape.

| Field | Notes |
|---|---|
| `manim_source` | Complete runnable Manim CE Python. One file. No imports beyond `manim` and the stdlib. |
| `scene_class` | **Always the literal `"GeneratedScene"`.** Frozen so Go never parses source to find a class name. Anything else = treat as a failure, trigger repair. |

### Models

Default `claude-opus-5` for all three calls, with adaptive thinking
(`thinking={"type": "adaptive"}` — it's on by default on Opus 5) and
`output_config={"effort": "low"}` on `/vision`, which is a transcription task and
doesn't need depth.

Use structured outputs (`output_config={"format": ...}`) so the JSON above is
schema-enforced rather than requested politely. **Do not** use assistant prefill
to force JSON — it returns a 400 on Opus 5.

`claude-haiku-4-5` is the fallback for `/vision` if we go with option 2 above.

---

## 2. Public HTTP API (Go)

Base URL in dev: `http://localhost:8080`. `Access-Control-Allow-Origin: *` on
every route — the extension and the paste page are both cross-origin.

### `POST /api/jobs`

Creates a job. **Returns before any Claude call happens.**

```json
// request
{
  "image": "data:image/png;base64,iVBORw0KG...",
  "user_prompt": "why does the second term have a cosine?",
  "source": "extension"
}

// 202 Accepted
{ "job_id": "j_7f3a9c21" }
```

`image` is exactly what `chrome.tabs.captureVisibleTab` returns; the paste page
sends the same shape. `user_prompt` may be `""` — it's still part of the cache
key. `source` is `"extension"` or `"web"`, telemetry only.

Errors: `400` bad body, `413` image over 8 MB, `503` queue full.

### `GET /api/jobs/{job_id}`

The only endpoint the client polls. Poll every **1s**, give up at **180s**.

```json
{
  "job_id": "j_7f3a9c21",
  "status": "rendering",
  "problem_hash": "a3f9c1d2e4b57680",
  "explanation": "**Step 1.** ...",
  "video_url": null,
  "cached": false,
  "error": null,
  "updated_at": "2026-09-12T02:14:03Z"
}
```

`404` on unknown or expired job ID.

| Status | Meaning | `explanation` | `video_url` |
|---|---|---|---|
| `queued` | accepted, not started | null | null |
| `transcribing` | `/vision` in flight | null | null |
| `explaining` | `/explain` in flight | null | null |
| `generating` | `/codegen` in flight | **set** | null |
| `rendering` | manim running | set | null |
| `done` | video ready | set | **set** |
| `failed` | ended early | **set if it got that far** | null |

**Two rules the client must honor:**

1. **Render `explanation` the instant it is non-null**, whatever the status says.
   Do not wait for `done`. The entire architecture exists to make this moment
   early — a UI that waits for the video makes a working system feel broken.
2. **`failed` is not empty.** If `explanation` is non-null on a failed job, show
   it with a note that the animation didn't render. That's ~5 lines of code and
   it's the difference between a dead demo and a working one.

`cached: true` means the whole pipeline was skipped on a hash hit — usually
lands on the first poll.

### `GET /renders/{hash}.mp4`

Static MP4 from a local directory. Range requests supported so `<video>` can
seek. This is what `video_url` points at.

*(If S3 goes back in, layout is already decided: `renders/<hash>.mp4`, same
basename. Only the URL prefix changes.)*

### `GET /healthz`

`{"ok": true, "redis": true, "manim": true, "agent": true}`. `manim` and `agent`
checked once at boot, not per request.

---

## 3. The cache key

```
sha256(PromptVersion + "||" + normalize(problem_text) + "||" + normalize(user_prompt))[:16]
```

Hex digest, **first 16 chars**. `normalize` = lowercase, trim, collapse every run
of whitespace (including newlines) to one space. Nothing else — no punctuation
stripping, no unicode folding. Implemented in Go; Python doesn't need it.

Three details, each load-bearing:

- **The user's prompt is in the key.** Same problem, different question is a
  different video.
- **`PromptVersion` is a hand-bumped constant in the Go server.** Bump it
  whenever *any* prompt text changes — including prompts that live in Python.
  Without it, a prompt improvement appears to do nothing because Redis keeps
  serving what the old prompt produced. This is the bug that eats an hour at 3am.
- **Matching is exact.** Hashing has no notion of "close enough." Normalization
  is the only defense against transcription jitter between two students at
  different zoom levels. See the OPEN question in §1.

### Redis keys

| Key | Value | TTL |
|---|---|---|
| `job:<job_id>` | the full job JSON above | 24h |
| `cache:<problem_hash>` | the video URL string | 7d |

Checked after `/vision`, written after a successful render. **Never written on
failure** — a failed render must not poison the next student.

---

## 4. Deferred, not deleted

These were cut for time. They are listed so nobody re-litigates them at 3am, and
so we know where they'd slot back in.

| Thing | Why cut | Where it goes back in |
|---|---|---|
| Docker isolation of render workers | Time. Static pre-check (AST parse, banned imports) **stays**. | `render/` worker, swap `exec.Command` for a container run. |
| Multi-scene + FFmpeg concat | One scene per problem kills a whole class of failure and the clips-don't-flow problem. | `storyboard.beats` would become `storyboard.scenes`. |
| Angular | Plain HTML + JS. | — |
| S3 | Go serves `./renders/`. | Key layout already decided. |
| Narration audio (TTS) | Audio forces re-encoding at concat time. `narration` becomes on-screen text. | `beats[].narration` is already the right field. |

---

## 5. Known risks

**Cache hit rate is unmeasured.** The whole caching story depends on two students
producing byte-identical transcriptions of the same problem at different zoom
levels and window widths. If the rate is near zero, Redis is decoration. **Test
this in Sprint 1:** screenshot one problem six different ways, count distinct
hashes.

**Generated Manim fails two ways.** Code that *crashes* is recoverable — the
repair loop catches it. Code that *runs but produces a bad video* (overlapping
text, objects off the 14.2-unit frame) exits zero and cannot be detected
programmatically. Mitigation is prompt constraints: relative positioning only
(`next_to`, `arrange`, `to_edge`), never literal coordinates, plus a cheatsheet
of verified snippets to imitate.

**LaTeX.** Manim's `MathTex` needs a TeX distribution and `dvisvgm`. If it's
missing, every math animation fails. **Verify before anything else.**

**Screenshot limits.** `captureVisibleTab` grabs the visible viewport only —
scrolled-off content is lost — and refuses on `chrome://` pages and the Web
Store.

**Nothing checks correctness.** A confidently wrong animated explanation is worse
than none, because the production values make it credible.

**Privacy.** The screenshot captures the whole viewport — tabs, browser chrome,
possibly the student's name — and it goes to a third-party API and sits on disk.

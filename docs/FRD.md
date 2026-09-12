# Functional requirements — draft

Rough. This is what the thing has to do, written so we can tell whether we're
done. Shapes and API live in [../CONTRACTS.md](../CONTRACTS.md); this file is
the *what*, that one is the *how it's wired*.

## The one-sentence version

A student hits a hotkey while looking at a hard math or algorithm problem in
their browser, and gets a written step-by-step explanation within seconds and a
custom animated video — rendered as several scenes in parallel and joined into
one clip — explaining the same problem about a minute later.

## Users

- **Primary:** a student stuck on a problem set, in a browser.
- **Secondary:** the same student with the problem in an IDE or a PDF — they
  paste a screenshot into the web app instead.

## Functional requirements

Numbered so we can point at them. **Must** = the system doesn't work without
it. **Should** = noticeably worse without it. **Could** = if there's time.

### Capture

| # | Req | Priority |
|---|---|---|
| F1 | A global hotkey in Chrome captures the visible tab as a PNG. | Must |
| F2 | A side panel opens on capture with an optional free-text question field and a guardrails toggle. | Must |
| F3 | The capture, the question, and the guardrails flag are sent to the server in one request. | Must |
| F4 | An Angular web app accepts a pasted or dropped image and sends the same request. | Should |
| F5 | Capture on pages Chrome refuses (`chrome://`, Web Store) shows a clear message rather than failing silently. | Should |

### Understanding the problem

| # | Req | Priority |
|---|---|---|
| F6 | The screenshot is transcribed to verbatim problem text plus a category (`math` / `algorithm` / `unknown`). | Must |
| F7 | A screenshot with no recognizable problem fails cleanly with a message, rather than producing an explanation of nothing. | Should |
| F8 | Transcription is deterministic enough that the same problem at different zoom levels produces the same normalized text most of the time. | Should — unmeasured |

### Explanation

| # | Req | Priority |
|---|---|---|
| F9 | A written step-by-step explanation in markdown is produced for the problem. | Must |
| F10 | The explanation reaches the user **as soon as it exists**, before any video work begins. Target: under 10s from hotkey. | Must |
| F11 | The user's typed question shapes the explanation. | Must |
| F12 | The explanation is preserved and shown even if every later step fails. | Must |
| F13 | With guardrails on, the explanation teaches the method and reasoning but stops short of the final answer. | Should |

### Animation

| # | Req | Priority |
|---|---|---|
| F14 | A storyboard of 2–5 independent scenes is produced, each describing what one animation segment shows. | Must |
| F15 | Each scene is grounded in a relevant snippet retrieved from a Manim-doc corpus before it is generated. | Should |
| F16 | Each scene is turned into its own Manim CE source file and rendered to its own MP4 clip, **in its own isolated container**, concurrently with the other scenes in the same job. | Must |
| F17 | Generated code passes a static pre-check (parses, no banned imports) before it is run, in addition to running in an isolated, network-disabled container. | Must |
| F18 | A scene that fails to render is fed back to the model for repair, up to 3 attempts, without affecting the other scenes in the job. | Should |
| F19 | A scene that exhausts repair is dropped; the job still completes with whatever scenes succeeded. | Should |
| F20 | Finished scene clips are joined into one video without re-encoding. | Must |
| F21 | The finished video uploads to S3 and the client streams it directly from there. | Must |
| F22 | The video plays inline in the panel when ready, without a reload. | Must |
| F23 | Narration text from the storyboard appears on screen in the video. | Should |
| F24 | Each scene favors things text can't show — plotted functions, pointers walking structures, shapes transforming — over animated algebra. | Should |
| F25 | With guardrails on, the animation shows the technique without resolving to the literal final answer. | Should |

### Caching

| # | Req | Priority |
|---|---|---|
| F26 | The same problem + same question + same guardrails setting, seen again, returns the existing video and explanation without re-running the pipeline. | Must |
| F27 | A cache hit returns in under one second. | Should |
| F28 | Changing a prompt invalidates the cache (via `PromptVersion`). | Must |
| F29 | A job with zero surviving scenes is never cached. | Must |
| F30 | Cached videos are removed from S3 automatically after a fixed retention period, without a custom cleanup process. | Should |

### Status and feedback

| # | Req | Priority |
|---|---|---|
| F31 | The client shows which stage the job is in, including a per-scene progress count ("rendering scene 2 of 3"). | Should |
| F32 | A job that finishes with at least one explanation but no video shows the explanation plus a note that the animation did not fully render. | Must |
| F33 | A job that fails before the explanation shows a plain error. | Must |

## Non-functional

| # | Req |
|---|---|
| N1 | Text explanation latency: **< 10s** p50 on a cold request. |
| N2 | Video latency: **< 120s** p50 on a cold request, including up to 3 repair attempts per scene, run concurrently across scenes. |
| N3 | Concurrent scene renders are capped (the CPU-bound part). Number is a config knob; start at `runtime.NumCPU() / 2`. |
| N4 | One bad scene cannot wedge a worker or the rest of the job — every render has a hard per-container timeout, and one scene's failure never blocks the others. |
| N5 | Render isolation runs in Docker; a local S3-compatible store (MinIO) means no machine needs real AWS credentials to develop against. Real AWS is a config swap, not a code change. |
| N6 | The whole system starts with one command per service (see [SETUP.md](SETUP.md)). |

## Out of scope — for now

Deferred, not deleted. See CONTRACTS §5 for where each slots back in.

- A real vector database for Manim-doc retrieval (an in-memory index over a
  fixed corpus is enough for now)
- Narration audio / TTS (conflicts with the no-re-encode concat design)
- Any verification that the explanation or generated code is *correct*
- Accounts, history, anything persistent per user
- Mobile, Firefox, Safari
- Automated enforcement of guardrails mode (it's a prompt instruction,
  verified by hand, not a filter)

## Open questions

See [../AGENTS.md](../AGENTS.md#open-questions) and CONTRACTS for the current
list and where each decision lives.

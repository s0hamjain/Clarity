# Functional requirements — draft

Rough. This is what the thing has to do, written so we can tell whether we're
done. Shapes and API live in [../CONTRACTS.md](../CONTRACTS.md); this file is
the *what*, that one is the *how it's wired*.

## The one-sentence version

A student hits a hotkey while looking at a hard math or algorithm problem in
their browser, and gets a written step-by-step explanation within seconds and a
custom animated video explaining the same problem about a minute later.

## Users

- **Primary:** a CMU student stuck on a problem set, in a browser, at 1am.
- **Secondary:** the same student with the problem in an IDE or a PDF — they
  paste a screenshot into a web page instead.
- **Tertiary:** the hackathon judge watching the demo. Optimize for the first
  two and the third takes care of itself.

## Functional requirements

Numbered so we can point at them. **Must** = demo dies without it. **Should** =
demo is noticeably better with it. **Could** = if there's time.

### Capture

| # | Req | Priority |
|---|---|---|
| F1 | A global hotkey in Chrome captures the visible tab as a PNG. | Must |
| F2 | A side panel opens on capture with an optional free-text question field. | Must |
| F3 | The capture plus the question are sent to the server in one request. | Must |
| F4 | A plain web page accepts a pasted or dropped image and sends the same request. | Should |
| F5 | Capture on pages Chrome refuses (`chrome://`, Web Store) shows a clear message rather than failing silently. | Should |

### Understanding the problem

| # | Req | Priority |
|---|---|---|
| F6 | The screenshot is transcribed to verbatim problem text plus a category (`math` / `algorithm` / `unknown`). | Must |
| F7 | A screenshot with no recognizable problem fails cleanly with a message, rather than producing an explanation of nothing. | Should |
| F8 | Transcription is deterministic enough that the same problem at different zoom levels produces the same normalized text most of the time. | Should — **unmeasured, test in Sprint 1** |

### Explanation

| # | Req | Priority |
|---|---|---|
| F9 | A written step-by-step explanation in markdown is produced for the problem. | Must |
| F10 | The explanation reaches the user **as soon as it exists**, before any video work begins. Target: under 10s from hotkey. | Must |
| F11 | The user's typed question shapes the explanation. | Must |
| F12 | The explanation is preserved and shown even if every later step fails. | Must |

### Animation

| # | Req | Priority |
|---|---|---|
| F13 | A storyboard (2–5 beats, one scene) is produced describing what the animation shows. | Must |
| F14 | The storyboard is turned into Manim CE source and rendered to an MP4. | Must |
| F15 | Rendering failures are fed back to the model for repair, up to 3 attempts. | Should |
| F16 | Generated code passes a static pre-check (parses, no banned imports) before it is run. | Must |
| F17 | The video plays inline in the side panel when ready, without a reload. | Must |
| F18 | Narration text from the storyboard appears on screen in the video. | Should |
| F19 | The storyboard favors things text can't show — plotted functions, pointers walking structures, shapes transforming — over animated algebra. | Should |

### Caching

| # | Req | Priority |
|---|---|---|
| F20 | The same problem + same question, seen again, returns the existing video without re-running the pipeline. | Must |
| F21 | A cache hit returns in under one second. | Should |
| F22 | Changing a prompt invalidates the cache (via `PromptVersion`). | Must |
| F23 | Failed renders are never cached. | Must |

### Status and feedback

| # | Req | Priority |
|---|---|---|
| F24 | The client shows which stage the job is in (transcribing / explaining / rendering). | Should |
| F25 | A job that fails after the explanation shows the explanation plus a note that the animation did not render. | Must |
| F26 | A job that fails before the explanation shows a plain error. | Must |

## Non-functional

| # | Req |
|---|---|
| N1 | Text explanation latency: **< 10s** p50 on a cold request. |
| N2 | Video latency: **< 120s** p50 on a cold request, including up to 3 repair attempts. |
| N3 | Concurrent renders are capped (the CPU-bound part). Number is a config knob; start at `runtime.NumCPU() / 2`. |
| N4 | One bad scene cannot wedge a worker — every render has a hard timeout. |
| N5 | Everything runs on one laptop for the demo. No cloud dependency except the Claude API. |
| N6 | The whole system starts with one command per service (see [setup.md](setup.md)). |

## Out of scope — for now

Deferred, not deleted. See CONTRACTS §4 for where each slots back in.

- Docker isolation of render workers
- Multi-scene animations and FFmpeg concat
- S3 storage
- Narration audio / TTS
- Any verification that the explanation is *correct*
- Accounts, history, anything persistent per user
- Mobile, Firefox, Safari

## Open questions

- **Determinism of transcription** without `temperature=0` — CONTRACTS §1.
- **Cache hit rate in practice** — Sprint 1 experiment.
- **Is this a cheating tool?** Needs a real answer, not a disclaimer, before
  Saturday. The honest version: the storyboard is built to teach the method,
  not produce the answer, and the prompt should be written that way.
- **Privacy.** The screenshot is the whole viewport. For the demo we say so out
  loud; for anything real it needs a crop step.

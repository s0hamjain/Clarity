# Clarity

**Press a hotkey on any problem on your screen. Get it explained — in words within seconds, and as a custom animated video about a minute later.**

Clarity is a macOS menu-bar app for students. You're stuck on a derivative in a PDF, a recurrence on a slide, a binary search in your editor that returns the wrong index. You press `⌘⇧E`, drag a box around the problem, optionally type what's confusing you, and press Enter. A small floating window shows a step-by-step written explanation almost immediately. Then a short animation — made specifically for *your* problem — plays in the same window.

The animation isn't pulled from a library. A model writes [Manim](https://www.manim.community/) code (the animation engine behind 3Blue1Brown's videos) for that exact problem, and Clarity renders it.

---

## How it works, in 60 seconds

```
   You                          Clarity desktop app              Server (on the same Mac)
    │                                  │                                  │
    │  ⌘⇧E, drag a box                 │                                  │
    │─────────────────────────────────►│                                  │
    │  floating box slides in;         │                                  │
    │  you type context, press Enter   │                                  │
    │─────────────────────────────────►│  POST the screenshot + text      │
    │                                  │─────────────────────────────────►│  1. read the problem off the image
    │                                  │                                  │  2. seen it before? → return cached video
    │                                  │                                  │  3. write the explanation  ◄── you see this in ~8 s
    │  result box shows explanation    │◄────────────────────────────────│
    │◄─────────────────────────────────│                                  │  4. plan 2–5 short animation scenes
    │                                  │                                  │  5. for each scene: look up example code,
    │                                  │                                  │     write Manim code, render it in a sandbox,
    │                                  │                                  │     fix it if it crashes (up to 3 tries)
    │                                  │                                  │  6. stitch scenes into one video, store it
    │  video plays in the result box   │◄────────────────────────────────│
    │◄─────────────────────────────────│                                  │
```

Two ideas drive the whole design:

- **The written explanation is the product. The video is a bonus that arrives later.** Rendering takes 60–90 seconds; the explanation takes about 8. So they're delivered separately — you never wait for the video to read the explanation.
- **Never render the same problem twice.** Every problem is fingerprinted (hashed). If anyone has asked about this exact problem before, you get the existing video in under a second.

---

## The four parts

| Part | What it is | Language | Who owns it |
|---|---|---|---|
| **Desktop app** | The menu-bar app: hotkey, screenshot, the floating input box, the result window, the `.app`/`.dmg` build | Python | P4 |
| **Coordinator** | The server the desktop app talks to. Tracks each request ("job") through its steps, checks the cache, calls the other two parts in order. Also owns the release (`.pkg`, GitHub Release) | Go | P3 |
| **Agent service** | The only part that talks to AI models. Reads the screenshot and writes the explanation (Google Gemini Flash), writes the Manim code and fixes it when it breaks (Claude Sonnet 5) | Python | P1 |
| **Render pipeline** | Turns Manim code into an MP4 safely: runs it inside a locked-down Docker container, retries on failure, stitches scenes, uploads the video | Go + Docker | P2 |

Supporting services: **MongoDB Atlas** (stores jobs, the cache, and a library of verified Manim examples the AI learns from), **S3 / MinIO** (stores finished videos), **Google Gemini** (reads the screenshot, writes the explanation), **Claude Sonnet** (writes the Manim code), **Voyage AI** (turns text into vectors so we can search the example library by meaning).

---

## New here? Read in this order

1. **This page** — you're doing it.
2. **[AGENTS.md](AGENTS.md)** — the project in one page: what each part does, the rules everyone follows, what each person builds each sprint. *15 minutes.*
3. **[docs/SETUP.md](docs/SETUP.md)** — get everything running on your Mac. *An hour, mostly waiting on Docker.*
4. **Your own file** — [P1_AI.md](docs/P1_AI.md), [P2_RENDER.md](docs/P2_RENDER.md), [P3_BACKEND.md](docs/P3_BACKEND.md), or [P4_DESKTOP.md](docs/P4_DESKTOP.md). Everything you personally build, sprint by sprint. [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) is the team view: roles, merge protocol, sync checklists.
5. **[docs/FRD.md](docs/FRD.md)** and **[docs/API.md](docs/API.md)** — the detailed spec. Don't read these cover to cover; open the section your task points to.

| Doc | Read it when you need… |
|---|---|
| [AGENTS.md](AGENTS.md) | The overview, the rules, the sprint plan at a glance |
| [docs/SETUP.md](docs/SETUP.md) | To install anything or run the system |
| [docs/P1_AI.md](docs/P1_AI.md) · [P2_RENDER.md](docs/P2_RENDER.md) · [P3_BACKEND.md](docs/P3_BACKEND.md) · [P4_DESKTOP.md](docs/P4_DESKTOP.md) | Your tasks — one file per person, self-contained |
| [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) | Team view: roles, contracts, sprint calendar, sync checklists, how we merge |
| [docs/FRD.md](docs/FRD.md) | The exact behavior of anything — the spec |
| [docs/API.md](docs/API.md) | Any HTTP endpoint: request, response, errors |
| [FILE_STRUCTURE.md](FILE_STRUCTURE.md) | Where a file goes and who owns it |

---

## Glossary

Words the docs use as if you already know them.

| Term | Meaning |
|---|---|
| **Job** | One request, from screenshot to video. Has an ID like `j_7f3a9c21` and a **status** that moves through `queued → transcribing → explaining → generating → rendering → concatenating → uploading → done`. |
| **Spotlight box** | The translucent floating text box that slides in after you take a screenshot, where you type context. Named after macOS Spotlight, which it resembles. |
| **Result box** | The translucent floating window that shows the explanation and then the video. You can drag it anywhere and close it with X. |
| **Recents** | The last 50 screenshots you've taken, stored on your Mac. The spotlight box can list them so you can ask a new question about an old screenshot. |
| **Coordinator** | The Go server. Called that because it doesn't do any AI or rendering itself — it coordinates the parts that do. |
| **Agent service** | The Python server that makes every AI call — Gemini to read the screenshot and explain it, Claude Sonnet to write the Manim code. |
| **Storyboard** | The plan for the animation: 2–5 **scenes**, each with a line of on-screen text (**narration**) and a description of what to show (**visual**). Written by the model before any code. |
| **Scene** | One self-contained Manim animation, 5–15 seconds. Each scene is rendered in its own container, in parallel with the others, then they're stitched together. |
| **Snippet** / **snippet corpus** | A short, verified, working Manim example stored in MongoDB. Before writing code for a scene, the system finds the 3 most similar snippets and shows them to the model as examples to imitate. This is **RAG** — retrieval-augmented generation. |
| **Vector search** | Searching by meaning instead of keywords. Text is turned into a list of numbers (an **embedding**); similar meanings produce similar numbers. MongoDB Atlas does the search. |
| **Repair loop** | If generated code crashes, the error message is sent back to the model with the code, and it tries again. Up to 3 attempts per scene. |
| **Pre-check** | A safety scan of generated code *before* it runs: must parse as Python, may only import `manim`, may not call `os.system`, `eval`, etc. |
| **`manim-worker`** | The Docker image (Python + Manim + LaTeX + ffmpeg) every scene renders inside. No network, capped memory and CPU. |
| **Cache** / **cache key** | Fingerprint of a problem: `hash(prompt version + problem text + your question + guardrails flag)`. Same fingerprint → reuse the existing video. |
| **`PromptVersion`** | A constant that's part of every cache key. Bump it whenever a prompt changes, or the cache keeps serving results from the old prompt. |
| **Guardrails mode** | A toggle. When on, the explanation and video teach the *method* and stop short of the final answer. |
| **Sprint** / **sync point** | We work in five time-boxed sprints. At the end of each, everyone's branch is merged into `main` in a fixed order and we run a checklist together. That meeting is the sync point. |
| **P1–P4** | The four roles. P1 agent service, P2 render pipeline, P3 coordinator, P4 desktop app. |
| **FRD** | Functional Requirements Document — `docs/FRD.md`, the spec. "FRD §10.3" means section 10.3 of it. |

---

## Install (once a release exists)

1. Download `Clarity.dmg` from the latest [GitHub Release](https://github.com/s0hamjain/Clarity/releases).
2. Drag **Clarity** to Applications and open it. On macOS 15: **System Settings → Privacy & Security → Open Anyway**.
3. Grant **Screen Recording** when asked, relaunch. Press `⌘⇧E`; grant **Input Monitoring**, relaunch.
4. `⌘⇧E`, drag a box around a problem, type any context, press Enter. Press ↓ in the box to pick a recent screenshot instead.

The backend has to be running — see [docs/SETUP.md](docs/SETUP.md).

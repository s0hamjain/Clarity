# Clarity

**Press a hotkey on any problem on your screen. Get it explained in words within seconds, and as a custom animated video about a minute later.**

Clarity is a macOS menu-bar app. You're stuck on a derivative in a PDF, a recurrence on a slide, a graph you need to run an algorithm on, a binary search in your editor that returns the wrong index. You press `⌘⇧E`, drag a box around it, optionally type what you want explained or visualized, and press Enter. A floating window shows a step-by-step written explanation almost immediately. A short animation made specifically for *your* problem plays in the same window shortly after.

The animation isn't pulled from a library. A model writes [Manim](https://www.manim.community/) code — the animation engine behind 3Blue1Brown's videos — for that exact problem, and Clarity renders it.

---

## User flow

1. `⌘⇧E` — a translucent box slides in. Drag to select the region of the screen with the problem.
2. Type any context ("explain guardrails only", "model topological sort on this graph") and press Enter, or press ↓ to reuse a recent screenshot instead.
3. A result window shows the written explanation in a few seconds.
4. The same window plays the generated video roughly 60–90 seconds later.

Two things shape this: the **explanation always arrives first**, since rendering takes far longer than writing it, and **identical problems never render twice** — a hash of the problem and your question is checked against a cache before any model is called.

---

## How it works

```
   You                     Desktop app                    Coordinator (Go)              Agent service (Python)
    │                           │                                  │                              │
    │ ⌘⇧E, drag a box            │                                  │                              │
    │──────────────────────────►│                                  │                              │
    │ type context, Enter        │  POST screenshot + text          │                              │
    │──────────────────────────►│─────────────────────────────────►│                              │
    │                           │                                  │  /vision: read the image     │
    │                           │                                  │─────────────────────────────►│
    │                           │                                  │◄─────────────────────────────│
    │                           │                                  │  seen this before? → cached video, done
    │                           │                                  │  /explain: write the explanation
    │                           │                                  │─────────────────────────────►│
    │ explanation appears        │◄─────────────────────────────────│◄─────────────────────────────│
    │◄──────────────────────────│                                  │  /render: one continuous Manim
    │                           │                                  │  script for the whole storyboard,
    │                           │                                  │  rendered + repaired in a sandbox
    │                           │                                  │─────────────────────────────►│
    │ video plays                │◄─────────────────────────────────│◄─────────────────────────────│
    │◄──────────────────────────│                                  │                              │
```

A job moves through statuses: `queued → transcribing → explaining → generating → rendering → uploading → done` (or `failed` / `cancelled`). The desktop app polls the coordinator for these and renders whatever is non-null as soon as it arrives.

**Why one continuous script, not several short clips stitched together:** an earlier design generated and rendered each storyboard scene independently and concatenated the results. Each scene's code had no knowledge of the previous scene's final frame, so consecutive scenes routinely redrew the same objects from scratch — the video looked like it kept restarting. The agent now writes a single `construct()` method that plays every beat of the storyboard as one uninterrupted animation, with objects introduced early on transformed or moved later rather than recreated.

---

## Infrastructure

| Part | What it does | Stack |
|---|---|---|
| **Desktop app** | Menu-bar app: global hotkey, screenshot capture, the floating input and result windows, recent-screenshot history. | Python, PyWebView |
| **Coordinator** | The server the desktop app talks to. Owns job lifecycle and status, the cache, and orchestration — never calls a model itself. | Go |
| **Agent service** | The only part that calls AI models, as three LangGraph agents: **Intake** reads the screenshot and your typed prompt; **Explainer** drafts and critiques the explanation + storyboard; **Manim Generator** retrieves example code from a vector store, writes and lints the Manim script, renders it in a sandbox, and repairs it on failure (up to 3 attempts). | Python, FastAPI, LangGraph/LangChain |
| **Render sandbox** | Runs generated Manim code inside a locked-down, network-isolated Docker container and produces the MP4. | Docker |

Supporting services:
- **MongoDB Atlas** — job records, the response cache, and a vector-searchable library of verified Manim code examples (hand-written seeds, real Manim CE documentation examples, and past successful generations) that the code-generation model retrieves from before writing anything new.
- **S3 / MinIO** — stores finished videos.
- **Google Gemini** — reads the screenshot, writes the explanation.
- **Claude (Sonnet)** — writes and repairs the Manim code.
- **Voyage AI** — embeds text so the example library can be searched by meaning, not keywords.

---

## Install

1. Download `Clarity.dmg` from the latest [GitHub Release](https://github.com/s0hamjain/Clarity/releases).
2. Drag **Clarity** to Applications and open it. On recent macOS: **System Settings → Privacy & Security → Open Anyway**.
3. Grant **Screen Recording** when asked, relaunch. Press `⌘⇧E`; grant **Input Monitoring**, relaunch.
4. `⌘⇧E`, drag a box around a problem, type any context, press Enter.

The desktop app needs the coordinator and agent service running somewhere it can reach (`http://localhost:8080` by default). For running those yourself — dependencies, API keys, MongoDB Atlas, Docker — see [docs/SETUP.md](docs/SETUP.md).

For the full technical spec, see [docs/FRD.md](docs/FRD.md) (behavior) and [docs/API.md](docs/API.md) (HTTP contracts).

# File structure

Proposed. Nothing under `agent/`, `server/`, `client/`, `render/` (the Docker
build context), or `samples/` exists yet — each component creates its own
directory. Keep to this layout so paths in the docs stay true.

```
HackCMU/
├── AGENTS.md                  ← start here: the idea, the flow, the architecture
├── CONTRACTS.md                ← shapes at every boundary (draft)
├── FILESTRUCTURE.md            ← this file
├── README.md
├── .gitignore
│
├── docs/
│   ├── FRD.md                  ← functional requirements
│   ├── SETUP.md                ← install + run everything
│   └── WORK_SPLIT.md           ← four components, who owns what, what each fakes
│
├── samples/
│   └── product_rule_scenes.py  ← verified Manim reference; codegen cheatsheet
│                                  seed AND the first entry in the Manim-doc
│                                  corpus (does not exist yet — render/ owns it)
│
├── docker/
│   └── manim-worker/           ← render/ component
│       ├── Dockerfile          ← Python 3.12 + Manim CE + LaTeX + dvisvgm + ffmpeg
│       └── entrypoint.sh       ← runs manim on /work/scene.py, writes /work/out.mp4
│
├── agent/                      ← Python · owns all Claude calls
│   ├── main.py                 ← FastAPI app: /vision /explain /manim-docs /codegen /healthz
│   ├── claude.py                ← one client, four call functions
│   ├── schemas.py               ← pydantic models = CONTRACTS §1 shapes
│   ├── manim_docs/
│   │   ├── corpus/               ← curated chunks: Manim CE docs + samples/
│   │   ├── index.py               ← embeds the corpus once at startup, cosine search
│   │   └── retrieve.py            ← top-k snippets for one scene's `visual` text
│   ├── prompts/
│   │   ├── vision.md
│   │   ├── explain_math.md
│   │   ├── explain_algorithm.md
│   │   ├── explain_guardrails.md ← the "teach the method, not the answer" variant
│   │   ├── codegen.md             ← embeds retrieved manim_docs snippets
│   │   └── repair.md
│   ├── experiments/
│   │   └── cache_collision.py   ← six screenshots → count distinct hashes
│   ├── requirements.txt
│   └── .env.example
│
├── server/                     ← Go · one module, two components
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          ← wires everything, reads env, starts HTTP
│   ├── internal/
│   │   ├── api/                 ← server/ component
│   │   │   ├── handlers.go       ← POST /api/jobs, GET /api/jobs/{id}, /healthz
│   │   │   └── cors.go
│   │   ├── jobs/                ← server/ component
│   │   │   ├── job.go             ← Job struct = CONTRACTS §2 shape, status enum, scenes_total/done
│   │   │   ├── store.go           ← Redis read/write, TTL reset on every update
│   │   │   └── worker.go          ← the goroutine: vision → cache? → explain → fan out scenes → concat → upload
│   │   ├── cache/                ← server/ component
│   │   │   ├── key.go             ← PromptVersion const, normalize(), Hash() — includes guardrails
│   │   │   └── key_test.go
│   │   ├── agent/                ← server/ component
│   │   │   └── client.go          ← HTTP client for the Python service
│   │   └── render/               ← render/ component
│   │       ├── docker.go          ← Render(): temp dir + `docker run` manim-worker → mp4 path
│   │       ├── precheck.go        ← AST parse via python, banned imports
│   │       ├── repair.go          ← RenderWithRepair(), 3 attempts, per scene
│   │       ├── semaphore.go       ← concurrency cap across concurrently-rendering scenes
│   │       ├── concat.go          ← Concat(): ffmpeg concat demuxer, -c copy, no re-encode
│   │       ├── s3.go              ← upload finished video, build the URL (public or presigned)
│   │       └── render_test.go     ← renders samples/ fixture through a real container
│   └── renders/                  ← per-job temp working dirs, gitignored (final MP4s live in S3, not here)
│
├── client/                     ← Angular workspace · everything the user sees
│   ├── angular.json
│   ├── package.json
│   ├── projects/
│   │   ├── shared/                ← Angular library, used by both shells below
│   │   │   └── src/lib/
│   │   │       ├── api.service.ts   ← POST /api/jobs, poll GET /api/jobs/{id}
│   │   │       ├── models.ts         ← TypeScript types matching CONTRACTS
│   │   │       └── panel/            ← the panel component: markdown render,
│   │   │                                progress ("scene 2 of 3"), guardrails
│   │   │                                toggle, video player, error/failed states
│   │   ├── extension-panel/       ← MV3 side panel shell around shared/panel
│   │   │   └── src/
│   │   │       ├── manifest.json    ← MV3, commands, sidePanel, activeTab, tabs
│   │   │       ├── background.ts     ← hotkey → captureVisibleTab → POST → open panel
│   │   │       ├── config.ts          ← SERVER_URL
│   │   │       └── icons/
│   │   └── web-app/                ← paste/drop shell around shared/panel
│   │       └── src/
│   │           └── app/
│   │               └── drop-zone/    ← paste / drag-drop image capture
│   └── stub/
│       └── stub_server.py         ← fake GET /api/jobs/{id} on a timer, for UI dev
│
└── media/                       ← manim scratch inside containers, gitignored
```

## Rules

- **One component, one directory.** Don't edit another component's directory
  without saying so — you'll both be pushing to `main`.
- **`server/internal/render/` is the `render/` component** even though it's
  inside `server/`. The boundary is the `Render()` / `RenderWithRepair()` /
  `Concat()` function signatures in [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md).
- **`docker/manim-worker/` is also the `render/` component.** It's the image
  every container in `Render()` runs from — build it before anything that
  depends on it.
- **`agent/manim_docs/` is the `agent/` component**, even though it never
  calls Claude directly — it's the retrieval half of the codegen pipeline,
  owned by whoever writes the codegen prompt.
- **`client/projects/shared/` is shared by two shells, not two components.**
  It's all `client/`.
- Generated output (`server/renders/` temp dirs, `media/`, `.venv/`, `.env`,
  Angular's `dist/`) is gitignored. Finished videos live in S3, not in the
  repo or on disk long-term.
- Docs at the root are shared. Change `CONTRACTS.md` in its own commit.

## Git

Push to `main`, pulling first. If two people touch the same file, the second
push resolves it — keep commits small so that's cheap. Branches are fine if
you want them; nothing requires going through a PR.

# File structure

Proposed. Nothing under `agent/`, `server/`, `client/`, `render/` (the Docker
build context), or `samples/` exists yet — each lane creates its own
directory. Keep to this layout so paths in the docs stay true.

```
HackCMU/
├── AGENTS.md                  ← start here; also the clock, five cycles, demo Sat 4 PM
├── CONTRACTS.md                ← shapes at every boundary (draft)
├── FILESTRUCTURE.md            ← this file
├── README.md
├── .gitignore
│
├── docs/
│   ├── FRD.md                  ← functional requirements
│   ├── SETUP.md                ← install + run everything
│   └── WORK_SPLIT.md           ← four lanes, who owns what, what each fakes
│
├── samples/
│   └── product_rule_scenes.py  ← verified Manim reference; codegen cheatsheet
│                                  seed AND the first entry in the Manim-doc
│                                  corpus (does not exist yet — render/ lane,
│                                  Sprint 1)
│
├── docker/
│   └── manim-worker/           ← render/ lane, built once in Sprint 1
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
├── server/                     ← Go · one module, two lanes
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          ← wires everything, reads env, starts HTTP
│   ├── internal/
│   │   ├── api/                 ← server/ lane
│   │   │   ├── handlers.go       ← POST /api/jobs, GET /api/jobs/{id}, /healthz
│   │   │   └── cors.go
│   │   ├── jobs/                ← server/ lane
│   │   │   ├── job.go             ← Job struct = CONTRACTS §2 shape, status enum, scenes_total/done
│   │   │   ├── store.go           ← Redis read/write, TTL reset on every update
│   │   │   └── worker.go          ← the goroutine: vision → cache? → explain → fan out scenes → concat → upload
│   │   ├── cache/                ← server/ lane
│   │   │   ├── key.go             ← PromptVersion const, normalize(), Hash() — includes guardrails
│   │   │   └── key_test.go
│   │   ├── agent/                ← server/ lane
│   │   │   └── client.go          ← HTTP client for the Python service
│   │   └── render/               ← render/ lane
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

- **One lane, one directory.** Don't edit another lane's directory without
  telling them; you'll both be pushing to `main`.
- **`server/internal/render/` is the `render/` lane** even though it's inside
  `server/`. The boundary is the `Render()` / `RenderWithRepair()` / `Concat()`
  function signatures in [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md).
- **`docker/manim-worker/` is also the `render/` lane.** It's the image every
  container in `Render()` runs from — build it first, in Sprint 1.
- **`agent/manim_docs/` is the `agent/` lane**, even though it never calls
  Claude directly — it's the retrieval half of the codegen pipeline, owned by
  the same people who write the codegen prompt.
- **`client/projects/shared/` is shared by two shells, not two lanes.** It's
  all `client/`.
- Generated output (`server/renders/` temp dirs, `media/`, `.venv/`, `.env`,
  Angular's `dist/`) is gitignored. Finished videos live in S3, not in the
  repo or on disk long-term.
- Docs at the root are everyone's. Change `CONTRACTS.md` in its own commit.

## Git

Everyone pushes to `main`. Pull before you push. If two people touch the same
file, the second one to push resolves it. Keep commits small so that's cheap.

Branches are fine if you want them, but nothing is required to go through a PR.
There's no time.

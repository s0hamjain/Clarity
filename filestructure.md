# File structure

Proposed. Nothing under `agent/`, `server/`, `client/`, or `samples/` exists yet
— each lane creates its own directory. Keep to this layout so paths in the docs
stay true.

```
HackCMU/
├── agents.md                  ← start here
├── CONTRACTS.md               ← shapes at every boundary (draft)
├── SPRINTS.md                 ← the clock, five cycles, demo Sat 4 PM
├── filestructure.md           ← this file
├── README.md
├── .gitignore
│
├── docs/
│   ├── frd.md                 ← functional requirements
│   ├── setup.md               ← install + run everything
│   └── work_split.md          ← four lanes, who owns what, what each fakes
│
├── samples/
│   └── product_rule_scenes.py ← verified Manim reference; codegen cheatsheet seed
│                                 (does not exist yet — render/ lane, Sprint 1)
│
├── agent/                     ← Python · owns all Claude calls
│   ├── main.py                ← FastAPI app: /vision /explain /codegen /healthz
│   ├── claude.py              ← one client, three call functions
│   ├── schemas.py             ← pydantic models = CONTRACTS §1 shapes
│   ├── prompts/
│   │   ├── vision.md
│   │   ├── explain_math.md
│   │   ├── explain_algorithm.md
│   │   ├── codegen.md         ← embeds samples/product_rule_scenes.py
│   │   └── repair.md
│   ├── experiments/
│   │   └── cache_collision.py ← six screenshots → count distinct hashes
│   ├── requirements.txt
│   └── .env.example
│
├── server/                    ← Go · one module, two lanes
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go        ← wires everything, reads env, starts HTTP
│   ├── internal/
│   │   ├── api/               ← server/ lane
│   │   │   ├── handlers.go    ← POST /api/jobs, GET /api/jobs/{id}, /healthz
│   │   │   ├── cors.go
│   │   │   └── static.go      ← /renders/ file serving
│   │   ├── jobs/              ← server/ lane
│   │   │   ├── job.go         ← Job struct = CONTRACTS §2 shape, status enum
│   │   │   ├── store.go       ← Redis read/write, TTL reset on every update
│   │   │   └── worker.go      ← the goroutine: vision → cache? → explain → codegen → render
│   │   ├── cache/             ← server/ lane
│   │   │   ├── key.go         ← PromptVersion const, normalize(), Hash()
│   │   │   └── key_test.go
│   │   ├── agent/             ← server/ lane
│   │   │   └── client.go      ← HTTP client for the Python service
│   │   └── render/            ← render/ lane
│   │       ├── render.go      ← Render(): temp .py → manim → mp4 path
│   │       ├── precheck.go    ← AST parse via python, banned imports
│   │       ├── repair.go      ← RenderWithRepair(), 3 attempts
│   │       ├── semaphore.go   ← concurrency cap
│   │       └── render_test.go ← renders samples/ fixture
│   └── renders/               ← output MP4s, gitignored
│
├── client/                    ← HTML / JS · everything the user sees
│   ├── shared/
│   │   ├── panel.js           ← poll loop, render explanation, show video
│   │   ├── panel.css
│   │   └── marked.min.js      ← vendored; extensions can't load remote scripts
│   ├── extension/
│   │   ├── manifest.json      ← MV3, commands, sidePanel, activeTab, tabs
│   │   ├── background.js      ← hotkey → captureVisibleTab → POST → open panel
│   │   ├── sidepanel.html
│   │   ├── sidepanel.js       ← thin shell around shared/panel.js
│   │   ├── config.js          ← SERVER_URL
│   │   └── icons/
│   ├── web/
│   │   ├── index.html         ← paste / drop zone + same panel
│   │   └── web.js
│   └── stub/
│       └── stub_server.py     ← fake GET /api/jobs/{id} on a timer, for UI dev
│
└── media/                     ← manim scratch, gitignored
```

## Rules

- **One lane, one directory.** Don't edit another lane's directory without
  telling them; you'll both be pushing to `main`.
- **`server/internal/render/` is the `render/` lane** even though it's inside
  `server/`. The boundary is the `Render()` function signature in
  [docs/work_split.md](docs/work_split.md).
- **`shared/` in `client/` is shared by two shells, not two lanes.** It's all
  `client/`.
- Generated output (`renders/`, `media/`, `.venv/`, `.env`) is gitignored. Don't
  force-add MP4s.
- Docs at the root are everyone's. Change `CONTRACTS.md` in its own commit.

## Git

Everyone pushes to `main`. Pull before you push. If two people touch the same
file, the second one to push resolves it. Keep commits small so that's cheap.

Branches are fine if you want them, but nothing is required to go through a PR.
There's no time.

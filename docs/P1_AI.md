# Clarity — P1: AI (Agents)

**You are P1. Your job in one sentence:** build the Python service where every model call happens — as **LangGraph agents**, with **LangChain** for prompts and structured output, **Pydantic** for every data shape, and a **MongoDB Atlas vector store** that the Manim generator retrieves from and feeds back into.

You are the only person who talks to any AI model. Nobody else writes a prompt. Nobody else imports `langchain`, `langgraph`, `google.genai`, or `anthropic`.

| Graph | Endpoint | What it does | Model |
|---|---|---|---|
| **Intake** (chain) | `/vision` | Reads the problem off the screenshot, verbatim | Gemini 3.8 Flash, `temperature=0` |
| **Explainer** (agent) | `/explain` | Drafts the explanation + storyboard, critiques it against the rules, revises once if needed | Gemini 3.8 Flash |
| **Manim Generator** (agent) | `/scenes/render` | For one scene: retrieve examples from Atlas → write Manim (Sonnet) → lint → render via the coordinator → repair up to 3× → ingest the working source back into Atlas | Claude Sonnet 5 for code; Gemini nowhere near code |

**Cost policy: no Opus-tier models, anywhere.** Gemini Flash by default. Sonnet only in the `generate` node.

---

## Your job in plain words

1. **Transcribe.** Screenshot in → problem text *word for word* + category (`math` / `algorithm` / `unknown`). One chain, no loop.
2. **Explain and plan, then check your own work.** Problem + question in → markdown explanation + a 2–5 scene **storyboard**. A second model call grades the draft against the rules (visual not algebraic, lengths, guardrails); if it fails, one revision.
3. **Generate the animation code — and own the loop.** For one scene: find the 3 most similar verified Manim examples in Atlas (vector search), write code imitating them, lint it, hand it to the coordinator to render in Docker, and if it crashes, retrieve again with the error as a hint, fix, re-render. Three tries. On success, store the code in Atlas as an unverified example so the corpus grows.
4. **Keep the library.** Seed it from P2's `samples/*.py`; expose list/get/promote/delete so a human can review what the agent added.

Everything in and out of every node is a Pydantic model. Nothing is a `dict`.

---

## What you own · what you never touch

| You own | You never touch |
|---|---|
| `agent/**` — the whole directory | Any Go (`server/`) — you *call* `POST /internal/render`, you don't implement it |
| Every prompt, every graph, every node | The Dockerfile, `samples/*.py` content (P2 writes those — you only *read* them) |
| The Gemini, Claude, Voyage, and Atlas clients; the `MongoDBAtlasVectorSearch` instance | Anything in `desktop/` |
| The `manim_snippets` collection and its vector index | The `jobs` and `cache` collections (P3's) |
| One exception: the one-line `PromptVersion` bump in `server/internal/cache/key.go` whenever you change a prompt | |

---

## Where your work meets others

You **implement** these; P3's coordinator **calls** them. Exact shapes in **[API.md §3](API.md#3-agent-service-api)** — copy them, don't invent fields.

| Endpoint | P3 sends | You return |
|---|---|---|
| `POST /vision` | base64 image | `{problem_text, category, confidence}` |
| `POST /explain` | problem text, question, guardrails | `{explanation, storyboard, revisions}` |
| `POST /scenes/render` | one scene, `work_dir`, `quality`, `job_id` | `{ok: true, clip_path, attempts, …}` or `{ok: false, attempts, stage, last_traceback}` |
| `POST /snippets/ingest`, `GET/PATCH/DELETE /snippets…` | corpus management | see API.md §3.6–3.10 |
| `GET /healthz` | — | `{ok, gemini, anthropic, voyage, atlas, coordinator, snippets_verified, graphs, …}` |

You **call** one thing P3 implements: **`POST <COORDINATOR_URL>/internal/render`** — `{source, work_dir, quality}` → `{ok, clip_path}` or `{ok: false, stage, traceback}` (API.md §2.7). That's your agent's render tool.

You **read** one thing P2 produces: `samples/*.py` files with a docstring header (FRD §13). Your seed script parses it.

**You are not blocked on anyone.** `/vision` and `/explain` are leaves. `/scenes/render` needs `/internal/render` — until P3 has it (Sprint 2), point `COORDINATOR_URL` at a 15-line stub that returns `{ok: true, clip_path: "<work_dir>/fake.mp4"}` on the first call and a fake traceback on demand so you can exercise the repair edge.

---

## Before you start (30 minutes)

Follow [SETUP.md](SETUP.md) §1, §3, §4, §5, §8. You need:

- `GEMINI_API_KEY` — SETUP §3.1
- `ANTHROPIC_API_KEY` — SETUP §3.2
- `VOYAGE_API_KEY` — SETUP §4
- `MONGODB_URI` — SETUP §5 (one person creates the cluster; get the string from them, or create it if you're first)
- Python 3.12 venv in `agent/` with:
  `langgraph langchain-core langchain-google-genai langchain-anthropic langchain-mongodb langchain-voyageai fastapi uvicorn pydantic pydantic-settings pymongo python-dotenv`

Verify: all four sanity checks in SETUP §3–§5 print what they should.

Read once: **FRD §10** (your architecture — the whole section), **FRD §13** (corpus design), **FRD §9.3** (snippet document + vector index), **API.md §3** and **§2.7**, **FRD §23 rules 1–6**.

If LangGraph is new to you: the 20-minute version is *a graph has a state model, nodes are functions `(state) -> partial update`, edges say which node runs next, conditional edges are functions `(state) -> node name`.* That's all you use here.

---

## Files you'll create

```
agent/
├── .env / .env.example
├── requirements.txt
├── app/
│   ├── main.py                        # FastAPI; one route per graph; /healthz
│   ├── config.py                      # pydantic-settings: keys, MONGODB_*, COORDINATOR_URL, model IDs, LANGSMITH_*
│   ├── schemas.py                     # API request/response models = API.md §3, exactly
│   ├── llm.py                         # gemini_flash(), gemini_flash_deterministic(), sonnet() — LangChain chat models
│   ├── prompts.py                     # load_prompt(name) → ChatPromptTemplate from prompts/*.md
│   ├── vectorstore.py                 # the ONE MongoDBAtlasVectorSearch instance + VoyageAIEmbeddings; retriever factory
│   ├── graphs/
│   │   ├── intake/
│   │   │   └── chain.py               # vision_prompt | gemini_flash_deterministic.with_structured_output(VisionResponse)
│   │   ├── explainer/
│   │   │   ├── state.py               # ExplainerState(BaseModel)
│   │   │   ├── nodes.py               # draft, critique, revise
│   │   │   └── graph.py               # StateGraph wiring + conditional edge
│   │   └── manim_generator/
│   │       ├── state.py               # SceneState(BaseModel)
│   │       ├── nodes.py               # retrieve, generate, lint, render, ingest
│   │       ├── lint.py                # ast checks, banned imports, class name, literal-coordinate heuristics
│   │       ├── tools.py               # render_tool(): POST /internal/render
│   │       └── graph.py               # StateGraph wiring; two bounded loops
│   ├── routers/
│   │   ├── vision.py                  # POST /vision      → intake chain
│   │   ├── explain.py                 # POST /explain     → explainer graph
│   │   ├── scenes.py                  # POST /scenes/render → manim_generator graph
│   │   ├── snippets.py                # ingest / list / get / patch / delete + debug /snippets/search
│   │   └── debug.py                   # POST /codegen — runs the generate node alone
│   └── prompts/
│       ├── vision.md
│       ├── explain_math.md
│       ├── explain_algorithm.md
│       ├── explain_guardrails.md
│       ├── critique.md
│       ├── codegen.md
│       └── repair.md
├── scripts/
│   ├── seed_snippets.py               # samples/*.py → vectorstore.add_documents; --create-index
│   └── promote_snippet.py             # PATCH /snippets/{id} {"verified": true}
└── experiments/
    ├── cache_collision.py
    └── retrieval_ablation.py
```

---

## Sprint 1 — Skeleton, clients, Intake chain, collision experiment

(Budget: ~3 h.)

**Goal:** the service runs; `/healthz` proves Gemini, Anthropic, Voyage, Atlas are reachable; `/vision` works for real as a LangChain chain; you know how often the same problem transcribes identically.

### Step 1 — Skeleton + models (45 min)
- `app/config.py`: `GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, `VOYAGE_API_KEY`, `MONGODB_URI`, `MONGODB_DB="clarity"`, `COORDINATOR_URL="http://localhost:8080"`, `VISION_MODEL="gemini-3.8-flash"`, `EXPLAIN_MODEL="gemini-3.8-flash"`, `CODEGEN_MODEL="claude-sonnet-5"`, `EMBED_MODEL="voyage-code-3"`, optional `LANGSMITH_TRACING`, `LANGSMITH_API_KEY`.
- `app/schemas.py`: a Pydantic model for every request and response in API.md §3. Field names exact — these *are* the contract.
- `app/llm.py`:
  ```python
  from langchain_google_genai import ChatGoogleGenerativeAI
  from langchain_anthropic import ChatAnthropic
  def gemini_flash(thinking="medium"):       return ChatGoogleGenerativeAI(model=cfg.EXPLAIN_MODEL, thinking_level=thinking)
  def gemini_flash_deterministic():          return ChatGoogleGenerativeAI(model=cfg.VISION_MODEL, temperature=0, thinking_level="low")
  def sonnet():                              return ChatAnthropic(model=cfg.CODEGEN_MODEL, max_tokens=16000, streaming=True)
  ```
  Never pass `temperature` to `ChatAnthropic`. Check the exact kwarg names against the installed `langchain-google-genai` version — thinking-level plumbing has moved between releases.
- `app/prompts.py`: `load_prompt("vision")` reads `prompts/vision.md` and returns `ChatPromptTemplate.from_messages([("system", text)])` (or system + human when the file has a `---` separator). Cache the templates.
- `app/main.py`: routes stubbed to return valid hardcoded responses so P3 can hit you today. `/healthz` pings all four services.

### Step 2 — Vector store (30 min)
- `app/vectorstore.py`: **one** `MongoDBAtlasVectorSearch(collection=db.manim_snippets, embedding=VoyageAIEmbeddings(model=cfg.EMBED_MODEL), index_name="snippets_vector", text_key="page_content", embedding_key="embedding")`. The `page_content` you embed is `title + "\n" + description + "\n" + " ".join(tags)` — **not** the source; queries are prose, so index prose. Source, title, category, tags, origin, verified live in metadata.
- `retriever(category)` → `store.as_retriever(search_kwargs={"k": 3, "pre_filter": {"verified": True, "category": {"$in": [category, "general"]}}})`.
- `--create-index` in the seed script creates `snippets_vector` exactly as FRD §9.3 (1024 dims, cosine, filter fields `verified`, `category`) via `store.create_vector_search_index(dimensions=1024, filters=["verified","category"])` (or the raw `create_search_index` if the helper isn't available in your version).

### Step 3 — Intake chain, for real (45 min)
- `graphs/intake/chain.py`: `chain = load_prompt("vision") | gemini_flash_deterministic().with_structured_output(VisionResponse)`. The image goes in as an inline image content block on the human message (`{"type": "image", "base64": ..., "mime_type": ...}` in LangChain's content-block format).
- `prompts/vision.md`: transcribe the problem **verbatim**. No interpretation, no "the problem asks…", no summary. Preserve line breaks as shown. No problem on screen → `category: "unknown"`, `problem_text: ""`.
- Test with a real screenshot via `curl` (API.md §8). Compare to the image character by character.

### Step 4 — Collision experiment (45 min)
- `experiments/cache_collision.py`: six screenshots of one problem (two zoom levels × three crop widths — take them by hand with `screencapture -i`), each through `/vision`, then `normalize()` reimplemented exactly as FRD §12, count distinct strings. Put the number in your PR description.

### Done when
- [ ] `uvicorn app.main:app --port 8000` runs; `/healthz` → `gemini`, `anthropic`, `voyage`, `atlas` all `true`.
- [ ] `/vision` returns verbatim text and the right category for a real screenshot.
- [ ] `snippets_vector` exists in Atlas (Active).
- [ ] Every other endpoint returns a schema-valid hardcoded response.
- [ ] Collision number recorded: **N distinct out of 6**.

---

## Sprint 2 — Explainer agent, seeded corpus, corpus endpoints

(Budget: ~4.75 h.)

**Goal:** `/explain` is a real LangGraph agent with a critique loop; the corpus is seeded from P2's samples and searchable; the corpus endpoints work.

### Step 1 — Explainer graph (2 h)
- `graphs/explainer/state.py`:
  ```python
  class ExplainerState(BaseModel):
      problem_text: str; category: str; user_prompt: str; guardrails: bool
      draft: ExplainDraft | None = None
      critique: Critique | None = None
      revisions: int = 0
  ```
- `nodes.py`:
  - `draft`: prompt = `explain_math.md` or `explain_algorithm.md` by category, + `explain_guardrails.md` appended when `guardrails`. `gemini_flash().with_structured_output(ExplainDraft)`. The storyboard rule goes in the prompt as a rule **and** as a bad/good scene pair: *every scene shows something text can't* — a plotted function, a pointer walking an array, a shape transforming. Algebra restated is cut.
  - `critique`: first, Python checks the hard rules (2–5 scenes, `narration` ≤ 90, `duration_seconds` 5–15) — if any fail, build a `Critique(passed=False, issues=[...])` without a model call. Otherwise `gemini_flash("low").with_structured_output(Critique)` with `critique.md`: does each scene describe motion or a plot rather than algebra? relative positioning language only? if guardrails, is the final answer truly absent?
  - `revise`: same as `draft` with the issues appended; `revisions += 1`.
- `graph.py`: `draft → critique →` conditional: `passed` → END; `not passed and revisions < 1` → `revise → critique`; else → END (log `explainer.gave_up`). Compile once at import.
- Router builds `ExplainerState`, runs `graph.invoke(state, config={"configurable": {"thread_id": job_id}})`, returns `ExplainResponse(explanation=..., storyboard=..., revisions=...)`.

### Step 2 — Seed script (45 min)
- `scripts/seed_snippets.py`: parse each `samples/*.py` docstring header (`title:`, `description:`, `category:`, `tags:` — FRD §13), read the source, build a LangChain `Document(page_content=title+"\n"+description+"\n"+tags, metadata={title, description, category, tags, source, origin:"seed", verified:True, created_at})`, `store.add_documents(docs, ids=[slug(title)])` — ids make it idempotent. `--create-index` as in Sprint 1.
- P2 has 5 samples by end of Sprint 1, 20 by end of Sprint 2. Seed whatever exists; re-run as more land.

### Step 3 — Corpus endpoints + debug retrieve (1 h)
- `POST /snippets/ingest`: build a `Document`, `add_documents`; `verified` defaults `false`; `409` if identical `source` exists (check first).
- `GET /snippets` (filter `verified`, `origin`; paginate), `GET /snippets/{id}`, `PATCH /snippets/{id}` (re-embed if title/description/tags change — delete + add), `DELETE /snippets/{id}`. These use `pymongo` directly on the same collection for the non-vector operations.
- `POST /snippets/search` (debug): runs `retriever(category).invoke(query)`; returns snippets with `score` from `similarity_search_with_score`.
- `scripts/promote_snippet.py <id>` = `PATCH {"verified": true}`.
- Test: a query about *plotting a derivative* returns a plotting seed above an array-walk seed; swap the query, the ranking flips.

### Step 4 — Determinism check (15 min)
- Sprint 1 number **≥ 4 of 6 identical** → Gemini at `temperature=0` is doing its job. Done.
- If not: the fix is the prompt or `normalize()`, not the model. Tighten (e.g. "preserve line breaks exactly" vs "join wrapped lines"), re-run, record.

### Step 5 — Coordinator stub for Sprint 3 (15 min)
- `agent/dev/fake_coordinator.py`: `POST /internal/render` → first call `{"ok": false, "stage": "render", "traceback": "NameError: name 'RIGHT_ARROW' is not defined"}`, second call `{"ok": true, "clip_path": "<work_dir>/fake.mp4"}`. So you can watch the repair edge fire before P3's real endpoint exists.

### Done when
- [ ] `/explain` returns a real explanation and storyboard; force a bad draft (e.g. a prompt that produces 7 scenes) and watch `revisions: 1` and the fix.
- [ ] Guardrails variant withholds the final answer on one test problem.
- [ ] `/healthz` shows `snippets_verified ≥ 20` (or as many as P2 has shipped).
- [ ] `/snippets/search` ranks correctly for a plotting query and an array-walk query.
- [ ] Ingest, list, get, patch, delete all work via `curl`.
- [ ] Determinism decision written down.

---

## Sprint 3 — Manim Generator agent

(Budget: ~3.25 h.)

**Goal:** `/scenes/render` runs the full retrieve → generate → lint → render → repair → ingest loop against P3's real `/internal/render`, and you can show retrieval helps.

### Step 1 — State, lint, render tool (45 min)
- `state.py`:
  ```python
  class SceneState(BaseModel):
      job_id: str; scene: Scene; storyboard_title: str; category: str; guardrails: bool; work_dir: str; quality: str
      snippets: list[Snippet] = []; hint: str = ""
      source: str | None = None; traceback: str | None = None
      attempts: int = 0; lint_retries: int = 0
      clip_path: str | None = None; stage: str | None = None; snippet_id: str | None = None
  ```
- `lint.py`: `ast.parse`; only `manim`/stdlib imports; ban `os.system`, `subprocess`, `__import__`, `eval(`, `exec(`, `open(` with write modes; require `class GeneratedScene(Scene)`; flag literal-coordinate patterns (`np.array([`, `.move_to([`, `.shift([`). Returns `None` or a one-line reason.
- `tools.py`: `render_tool(source, work_dir, quality) -> RenderResult` = `httpx.post(f"{COORDINATOR_URL}/internal/render", json=..., timeout=130)`. Map connection errors to `502 coordinator_unreachable`.

### Step 2 — Nodes and graph (1.5 h)
- `retrieve`: `retriever(state.category).invoke(narration + " " + visual + (" " + hint if hint else ""))` → `snippets`.
- `generate`: `sonnet().with_structured_output(ManimSource)`. Prompt = `codegen.md` (constraints from FRD §10.4 + "Reference — imitate these" with snippets verbatim + the scene) or `repair.md` when `traceback` is set (previous source + last 40 traceback lines; *fix the minimal thing, do not rewrite*). Assert `scene_class == "GeneratedScene"` — otherwise treat as a lint failure.
- `lint`: run `lint.py`. Fail → `traceback = reason`, `lint_retries += 1`.
- `render`: `render_tool(...)`; `attempts += 1`; success → `clip_path`; fail → `traceback`, `hint = first line`, `stage`.
- `ingest`: `add_documents([Document(page_content=title+desc, metadata={source, origin:"generated", verified:False, ...})])` → `snippet_id`. Wrap in try/except; log on failure.
- `graph.py`:
  ```
  retrieve → generate → lint →(ok)→ render →(ok)→ ingest → END
                  ▲       └(fail, lint_retries<2)┘   └(fail, attempts<3)→ retrieve
                  └────────(fail, lint_retries≥2 → count as an attempt, go to retrieve)
                                                       (fail, attempts≥3)→ END
  ```
  Two counters, two caps. Log every transition with `job_id`, `scene.index`, `attempts`, `lint_retries`, and the traceback's first line — P2 and you will read these logs in Sprint 4.
- Router builds `SceneState`, runs the graph with `thread_id=f"{job_id}/{scene.index}"`, returns `SceneRenderResponse` from the final state.

### Step 3 — Debug `/codegen` + retrieval ablation (1 h)
- `routers/debug.py`: `POST /codegen` runs the `generate` node alone on a caller-supplied scene + snippets. No lint, no render.
- `experiments/retrieval_ablation.py`: 10 real storyboard scenes. For each, `/codegen` with retrieved snippets and with `snippets: []`; render both through P3's `/internal/render`; count first-attempt successes per arm. Record both numbers in the PR. If "without" ≈ "with", the corpus needs better coverage — tell P2 which scene types failed.

### Done when
- [ ] `/scenes/render` against P3's real `/internal/render` returns `ok: true` with a `clip_path` for a real scene.
- [ ] Inject a bad import into a generated source (a debug flag in `generate`) → lint catches it, `lint_retries` increments, the graph recovers.
- [ ] Point at the fake coordinator → the render-fail edge fires, `retrieve` runs with a hint, second attempt succeeds, `attempts: 2`.
- [ ] A new `origin: "generated", verified: false` snippet appears in Atlas after a success.
- [ ] Ablation numbers recorded: with **X/10**, without **Y/10**.

---

## Sprint 4 — Guardrails verification, corpus review, prompt tuning

(Budget: ~3 h.)

**Goal:** guardrails mode actually withholds answers; the corpus only contains examples worth imitating; recurring failures become prompt rules or new samples.

### Step 1 — Guardrails check (1 h)
- 5 math + 5 algorithm problems with `guardrails: true` through `/explain`. Does the explanation reveal the final answer? Does any scene? Record pass/fail. The `critique` node is your first line of defense — if it's passing drafts that leak, fix `critique.md` before `explain_guardrails.md`. Target **≥ 8/10**.

### Step 2 — Review generated snippets (1 h)
- `GET /snippets?verified=false&origin=generated`. P2 renders each (or you do via `/internal/render`); **watch the clip**. Good → `promote_snippet.py`. Bad → `DELETE`. Write down *why* the bad ones were bad — overlapping text, off-frame, literal coordinates. Each recurring cause becomes a `codegen.md` rule or a `lint.py` check.

### Step 3 — Prompt tuning from logs (1 h)
- Pull the Sprint 3 logs: group tracebacks by error class. Each class → a `codegen.md` constraint, a `lint.py` heuristic, or a request to P2 for a seed sample showing the right API.
- **Every prompt change → bump `PromptVersion`** in `server/internal/cache/key.go`. One-line commit, or tell P3.

### Done when
- [ ] Guardrails pass rate recorded, ≥ 8/10.
- [ ] Zero un-reviewed generated snippets.
- [ ] `PromptVersion` bumped; `codegen.md` / `lint.py` carry rules from real failures.

---

## Sprint 5 — Freeze (Budget: ~1 h)

- Final `PromptVersion` bump. **No prompt or graph changes after this.**
- Pre-warm the cache with P3: 4–5 representative problems end to end.
- `/healthz` all-green on a fresh boot, `graphs` lists all three.
- If you finish early, pull from the **Overflow backlog** in WORK_SPLIT.md.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p1/sprint-N-<what>`.
2. Commit small and often. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, run your own tests, push.
4. **At the sync point:** merge order is **P3 → P1 → P2 → P4**. You're second. You're the merge captain in Sprint 2.
5. After the merge: back to step 1.
6. **Finished early?** Take the next Overflow item in WORK_SPLIT.md.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

1. Every model call goes through `.with_structured_output(PydanticModel)`. Never parse prose for JSON. Never assistant prefill. Never `temperature` on a Claude model.
2. The `/vision` prompt contains **no** instruction to interpret, summarize, or contextualize. Verbatim only. That string is hashed.
3. Graph state, node inputs, node outputs, requests, responses: all Pydantic models. No `dict`, no `TypedDict`.
4. Retrieval filters on `verified: true` in every code path. No debug flag disables it.
5. One `MongoDBAtlasVectorSearch` instance; seed, retrieve, and ingest all go through it. Index and query use the same `EMBED_MODEL`.
6. Every loop in a graph has a counter in state and a hard cap. Intake: no loop. Explainer: ≤ 1 revision. Manim Generator: ≤ 3 render attempts, ≤ 2 lint retries per attempt. No Opus-tier model anywhere; Sonnet only in `generate`.

---

## Decisions that are yours

- **Whether Sonnet stays in `generate`** — if the Sprint 3 ablation shows Flash renders first-try as often, switch and drop the Anthropic dependency.
- **The critique rubric** — what "shows something text can't" means concretely, and how strict guardrails checking is.
- **Lint heuristics** — how aggressively to flag literal coordinates without false positives.
- **Embedding model** — `voyage-code-3` default; if prose→prose matching looks weak in Sprint 2, try `voyage-3.5` *before* the corpus is large.

## If you're blocked

Sprints 1–2: nothing to wait for. Sprint 3: if P3's `/internal/render` isn't up, use `agent/dev/fake_coordinator.py` — the graph doesn't know the difference. If `samples/` is empty when you want to seed, seed from the SETUP §6.2 smoke-test scene and re-run when P2 lands more.

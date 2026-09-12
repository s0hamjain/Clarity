# Clarity — P1: AI (Agent Service)

**You are P1. Your job in one sentence:** build the Python service that makes every call to an AI model and returns exact JSON — read the problem off a screenshot (Gemini), write the explanation and animation plan (Claude Opus 5), find example code, write the Manim code and fix it when it crashes (Claude Sonnet 5).

You are the only person who talks to any AI model. Nobody else writes a prompt. Nobody else imports `google.genai` or `anthropic`.

| Task | Model | Why |
|---|---|---|
| `/vision` — OCR the screenshot | **Gemini 3.8 Flash** `gemini-3.8-flash` | Strong vision, fast, cheap, and accepts `temperature=0` — which the cache needs |
| `/explain` — explanation + storyboard | **Claude Opus 5** `claude-opus-5` | Best reasoning for the part the user reads |
| `/codegen` — Manim code + repair | **Claude Sonnet 5** `claude-sonnet-5` | Strong at code, faster and cheaper than Opus for 2–5 calls per job plus repairs |

---

## Your job in plain words

1. **Transcribe.** Given a screenshot, return the problem text *word for word* plus a category (`math` / `algorithm` / `unknown`).
2. **Explain and plan.** Given problem text and the user's question, return a step-by-step markdown explanation and a **storyboard**: 2–5 short scenes describing what an animation should show.
3. **Find examples.** Given one scene, find the 3 most similar verified Manim examples from a library in MongoDB Atlas (vector search). This is the RAG part.
4. **Write code.** Given one scene plus those examples, return complete runnable Manim code with `class GeneratedScene(Scene)`.
5. **Fix code.** Given the code that crashed plus its traceback, return fixed code.
6. **Keep the library.** Seed it from `samples/*.py`, ingest new examples from successful renders as *unverified*, let a human promote them.

Everything you return is schema-enforced JSON — Claude's **structured outputs** (`output_config.format`) and Gemini's **`response_schema`** — never prose you have to parse.

---

## What you own · what you never touch

| You own | You never touch |
|---|---|
| `agent/**` — the whole directory | Any Go (`server/`) |
| Every prompt | The Dockerfile, `samples/*.py` content (P2 writes those — you only *read* them) |
| The Gemini, Claude, Voyage, and Atlas clients | Anything in `desktop/` |
| The `manim_snippets` collection and its vector index | The `jobs` and `cache` collections (P3's) |
| `agent/scripts/` and `agent/experiments/` | |
| One exception: the one-line `PromptVersion` bump in `server/internal/cache/key.go` whenever you change a prompt | |

---

## Where your work meets others

You **implement** these; P3's coordinator **calls** them. The exact request/response for each is in **[API.md §3](API.md#3-agent-service-api)**. Copy the shapes; don't invent fields.

| Endpoint | What P3 sends you | What you return |
|---|---|---|
| `POST /vision` | base64 image | `{problem_text, category, confidence}` |
| `POST /explain` | problem text, question, guardrails flag | `{explanation, storyboard{title, scenes[]}}` |
| `POST /snippets/search` | one scene, category, k, optional traceback hint | `{snippets[]}` |
| `POST /codegen` | one scene, snippets, optional previous source + traceback | `{manim_source, scene_class}` |
| `POST /snippets/ingest` | a new snippet | `{id, embedded}` |
| `GET/PATCH/DELETE /snippets…` | corpus management | see API.md §3.6–3.9 |
| `GET /healthz` | — | `{ok, anthropic, voyage, atlas, snippets_verified, …}` |

You **read** one thing P2 produces: `samples/*.py` files with a docstring header (FRD §13). Your seed script parses that header.

**Nobody is waiting on you to start.** You're a leaf: test everything with `curl` and a PNG.

---

## Before you start (30 minutes)

Follow [SETUP.md](SETUP.md) §1, §3, §4, §5, §8. You need:

- `GEMINI_API_KEY` — SETUP §3.1
- `ANTHROPIC_API_KEY` — SETUP §3.2
- `VOYAGE_API_KEY` — SETUP §4
- `MONGODB_URI` — SETUP §5 (one person creates the cluster; get the string from them, or create it yourself if you're first)
- Python 3.12 venv in `agent/` with `google-genai anthropic fastapi uvicorn pydantic pydantic-settings voyageai pymongo python-dotenv`

Verify: all four sanity checks in SETUP §3–§5 print what they should.

Read once: **FRD §10** (your endpoints in context), **FRD §13** (RAG design), **FRD §9.3** (the snippet document and vector index), **API.md §3**, and **FRD §23 rules 1–6** (yours).

---

## Files you'll create

```
agent/
├── .env / .env.example
├── requirements.txt
├── app/
│   ├── main.py                 # FastAPI app, routers, /healthz
│   ├── config.py               # pydantic-settings reading .env
│   ├── schemas.py              # Pydantic models = API.md §3 shapes, exactly
│   ├── clients/gemini.py       # google-genai client; temperature=0 + response_schema helper for /vision
│   ├── clients/claude.py       # anthropic.Anthropic(); structured helper (Opus 5); stream helper (Sonnet 5)
│   ├── clients/embed.py        # voyageai embed(texts, model, input_type)
│   ├── clients/atlas.py        # pymongo client, collection handles, $vectorSearch helper
│   ├── routers/vision.py
│   ├── routers/explain.py
│   ├── routers/snippets.py     # search, ingest, list, get, patch, delete
│   ├── routers/codegen.py
│   └── prompts/
│       ├── vision.md
│       ├── explain_math.md
│       ├── explain_algorithm.md
│       ├── explain_guardrails.md
│       ├── codegen.md
│       └── repair.md
├── scripts/
│   ├── seed_snippets.py        # samples/*.py → embed → upsert; --create-index
│   └── promote_snippet.py      # PATCH /snippets/{id} {"verified": true}
└── experiments/
    ├── cache_collision.py
    └── retrieval_ablation.py
```

---

## Sprint 1 — Skeleton, clients, real `/vision`, collision experiment

(Budget: ~3 h.)

**Goal:** the service runs, `/healthz` proves all three external services are reachable, one real transcription works, and you know how often the same problem transcribes identically.

### Step 1 — Skeleton (45 min)
- `app/main.py`: FastAPI app. Mount routers. No CORS (only the coordinator calls you).
- `app/config.py`: `pydantic_settings.BaseSettings` with `ANTHROPIC_API_KEY`, `VOYAGE_API_KEY`, `MONGODB_URI`, `MONGODB_DB="clarity"`, `EMBED_MODEL="voyage-code-3"`, `VISION_MODEL="gemini-3.8-flash"`, `EXPLAIN_MODEL="claude-opus-5"`, `CODEGEN_MODEL="claude-sonnet-5"`; plus `GEMINI_API_KEY`.
- `app/schemas.py`: one Pydantic model per request and response in **API.md §3**. Field names exact. These *are* the contract.
- Every endpoint returns a valid hardcoded response so P3 can hit you today.

### Step 2 — Clients (1 h)
- `clients/gemini.py`: one `genai.Client()` (reads `GEMINI_API_KEY`). A helper `vision(image_bytes, mime_type, prompt, schema_model)` that calls `client.models.generate_content(model=VISION_MODEL, contents=[types.Part.from_bytes(data=image_bytes, mime_type=mime_type), prompt], config=types.GenerateContentConfig(temperature=0, response_mime_type="application/json", response_schema=schema_model, thinking_config=types.ThinkingConfig(thinking_level="low")))` and returns `schema_model.model_validate_json(response.text)`. **Always `temperature=0`** — that's the whole reason Gemini has this job.
- `clients/claude.py`: one `anthropic.Anthropic()`. `structured(model, prompt, schema_model)` calls `messages.create` with `output_config={"format": ...}` built from the Pydantic model's JSON schema and returns the parsed model — used with `EXPLAIN_MODEL` (Opus 5). `structured_stream(model, ...)` uses `messages.stream()` + `get_final_message()` for long outputs — used with `CODEGEN_MODEL` (Sonnet 5). **Never** pass `temperature`; **never** use assistant prefill — both are 400s on Opus 5 and Sonnet 5.
- `clients/embed.py`: `voyageai.Client().embed(texts, model=EMBED_MODEL, input_type="document"|"query").embeddings`.
- `clients/atlas.py`: `MongoClient(MONGODB_URI)[MONGODB_DB]`; handles for `manim_snippets`.
- `/healthz`: ping all four (Gemini, Anthropic, Voyage, Atlas); report `snippets_verified` = count of `{verified: true}` and the four model IDs.

### Step 3 — Real `/vision` on Gemini (45 min)
- `prompts/vision.md`: transcribe the problem **verbatim**. No interpretation, no "the problem asks…", no summary. If there is no problem on screen, `category: "unknown"`, `problem_text: ""`.
- Gemini 3.8 Flash, `temperature=0`, thinking `low`, `response_schema=VisionResponse` — this is transcription, not reasoning. Decode the base64 P3 sends you into bytes for the image `Part`.
- Test: `curl` a real screenshot (SETUP §… curl cookbook in API.md §8). Compare the output to the image by eye. It should be character-for-character.

### Step 4 — Collision experiment (45 min)
- `experiments/cache_collision.py`: take **six** screenshots of one problem (two zoom levels × three crop widths — do this by hand with `screencapture -i`), run each through `/vision`, apply `normalize()` (lowercase, trim, collapse whitespace — reimplement exactly as FRD §12), print the number of **distinct** strings.
- Write the number in your PR description. It decides Sprint 2 Step 5.

### Done when
- [ ] `uvicorn app.main:app --port 8000` starts; `curl localhost:8000/healthz` → `gemini`, `anthropic`, `voyage`, `atlas` all `true`.
- [ ] `/vision` on a real screenshot returns verbatim text and the right category.
- [ ] Every other endpoint returns a schema-valid hardcoded response.
- [ ] Collision number recorded: **N distinct out of 6**.

---

## Sprint 2 — `/explain`, the snippet library, `/snippets/search`, determinism decision

(Budget: ~4.75 h.)

**Goal:** a real explanation and storyboard come back; the library is seeded from P2's samples and searchable by meaning.

### Step 1 — `/explain` on Claude Opus 5 (1.5 h)
- Prompts: `explain_math.md`, `explain_algorithm.md`, chosen by `category`. When `guardrails: true`, append `explain_guardrails.md`: teach the method, work the setup, **stop before the final answer** (math: leave the final substitution; code: give a skeleton with decision points named, never a complete solution).
- Structured output = `ExplainResponse`. 2–5 scenes. `narration` ≤ 90 chars. `visual` in relative terms ("below", "next to"), never coordinates.
- The storyboard rule, in the prompt as a rule *and* as a bad/good example pair: **every scene must show something text can't** — a function and its derivative plotted together, a pointer walking an array, a shape transforming. A scene that just restates algebra gets cut.

### Step 2 — Seed script + vector index (1 h)
- `scripts/seed_snippets.py`: for each `samples/*.py`, parse the docstring header (`title:`, `description:`, `category:`, `tags:` — FRD §13), read the source, embed **`title + "\n" + description + "\n" + " ".join(tags)`** (not the code — queries are prose, so index prose) with `input_type="document"`, upsert by `title` into `manim_snippets` with `verified: true, origin: "seed"`.
- `--create-index` creates the Atlas Vector Search index `snippets_vector` exactly as **FRD §9.3** (1024 dims, cosine, filters on `verified` and `category`) via `create_search_index`. Idempotent — safe to re-run.
- P2 will have 5 samples by end of Sprint 1 and 20 by end of Sprint 2. Seed whatever exists; re-run as more land.

### Step 3 — `/snippets/search` (1 h)
- Embed `narration + " " + visual` (+ `" " + hint` if present) with `input_type="query"`.
- `$vectorSearch` on `snippets_vector`: `numCandidates: 50`, `limit: k`, filter `{verified: true, category: {$in: [category, "general"]}}`. Project `score: {$meta: "vectorSearchScore"}`.
- Test: a query about *plotting a derivative* returns a plotting seed above an array-walk seed. Swap the query; the ranking flips.

### Step 4 — `/snippets/ingest` + corpus management (1 h)
- `POST /snippets/ingest`: embed, insert. `verified` defaults `false`. Return `409` if the same `source` exists.
- `GET /snippets` (filter `verified`, `origin`; paginate), `GET /snippets/{id}`, `PATCH /snippets/{id}` (re-embed if title/description/tags change), `DELETE /snippets/{id}`. API.md §3.6–3.9.
- `scripts/promote_snippet.py <id>` = `PATCH {"verified": true}`.

### Step 5 — Check determinism (15 min)
- If Sprint 1's number was **≥ 4 of 6 identical**: Gemini at `temperature=0` is doing its job. Done.
- If not: the fix is in the prompt or in `normalize()`, not the model — tighten the transcription instructions (e.g. "preserve line breaks exactly as shown" vs "join wrapped lines") and re-run. Record the new number. If it's still bad, raise it at the sync point.

### Done when
- [ ] `/explain` returns a real explanation and a 2–5 scene storyboard for a real problem; guardrails variant withholds the answer on one test problem.
- [ ] `/healthz` shows `snippets_verified ≥ 20` (or however many samples P2 has shipped).
- [ ] `/snippets/search` ranks correctly for a plotting query and an array-walk query.
- [ ] Ingest, list, get, patch, delete all work via `curl`.
- [ ] Determinism decision written down.

---

## Sprint 3 — `/codegen` with retrieved examples, repair, measure it

(Budget: ~3.25 h.)

**Goal:** real Manim code comes back for every scene, grounded in retrieved snippets; crashes get repaired; you can show retrieval helps.

### Step 1 — `/codegen` on Claude Sonnet 5 (1.5 h)
- Model is `CODEGEN_MODEL` = `claude-sonnet-5`, via the streaming helper. Same for repair.
- `prompts/codegen.md` contains, in this order: the hard constraints (FRD §10.4 — `from manim import *` only, relative positioning only, never literal coordinates, never set resolution/fps, `narration` → `Text(...).to_edge(DOWN)`, class name is always `GeneratedScene`); a section **"Reference — imitate these"** with the retrieved snippets pasted verbatim; the scene's `narration` and `visual`; the output schema.
- **Assert before returning:** `scene_class == "GeneratedScene"` and `"class GeneratedScene(Scene)" in manim_source`. Otherwise raise → `422 schema_violation`. P3 counts that as a failed attempt.

### Step 2 — Repair (45 min)
- When `previous_source` and `traceback` are present, use `prompts/repair.md`: *fix the minimal thing that caused this error; do not rewrite the scene.* Include the previous source and the **last 40 lines** of the traceback only.

### Step 3 — Retrieval ablation (1 h)
- `experiments/retrieval_ablation.py`: 10 storyboard scenes (save them from real `/explain` calls). For each: `/codegen` with retrieved snippets and `/codegen` with `snippets: []`. Render both through P2's container (ask P2 for the command, or use `docker run manim-worker …` from SETUP §6.2). Count first-attempt successes in each arm. Write both numbers in the PR.

### Done when
- [ ] `/codegen` returns runnable code for a real scene; the class name assertion fires on a deliberately bad prompt.
- [ ] Repair path returns fixed code for a real traceback (get one from P2).
- [ ] Ablation numbers recorded: with snippets **X/10**, without **Y/10**.
- [ ] P3 confirms every codegen prompt in a real job contained ≥ 1 snippet (they'll check your logs — log the snippet titles per call).

---

## Sprint 4 — Guardrails check, promote/delete generated snippets, tune prompts

(Budget: ~3 h.)

**Goal:** guardrails mode actually withholds answers; the library only contains examples worth imitating; recurring failures become prompt rules.

### Step 1 — Guardrails verification (1 h)
- 5 math + 5 algorithm problems with `guardrails: true`. For each: does the explanation reveal the final answer? Does any scene? Record pass/fail. Tune `explain_guardrails.md` until **≥ 8/10** pass.

### Step 2 — Review generated snippets (1 h)
- `GET /snippets?verified=false&origin=generated`. For each, ask P2 to render it (or run the container yourself) and **watch the clip**. Good → `promote_snippet.py`. Bad → `DELETE`. Write down *what made the bad ones bad* — overlapping text, off-frame objects, literal coordinates. Each recurring cause becomes a line in `codegen.md`.

### Step 3 — Prompt tuning from real tracebacks (1 h)
- Ask P3 for the repair tracebacks from Sprint 3. Group by error class. Each class becomes either a codegen constraint or a request to P2 for a seed snippet showing the right usage.
- **Every prompt change → bump `PromptVersion`** in `server/internal/cache/key.go`. One-line commit, or tell P3.

### Done when
- [ ] Guardrails pass rate recorded, ≥ 8/10.
- [ ] Zero `verified: false, origin: generated` snippets left un-reviewed.
- [ ] `PromptVersion` bumped; `codegen.md` has new rules from real failures.

---

## Sprint 5 — Freeze (Budget: ~1 h)

- Final `PromptVersion` bump. **No prompt changes after this.**
- Pre-warm the cache: run 4–5 representative problems end to end with P3 so they're instant.
- Confirm `/healthz` all-green on a fresh boot.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p1/sprint-N-<what>`.
2. Commit small and often. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, run your own tests, push.
4. **At the sync point:** the merge captain merges in order **P3 → P1 → P2 → P4**. You're second. Be ready to fix anything the checklist catches on your path.
5. After the merge: back to step 1 for the next sprint.
6. **Finished your sprint early?** Take the next item from the Overflow backlog in WORK_SPLIT.md — anyone can, regardless of role.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

1. Every response is schema-enforced JSON (Claude `output_config.format`; Gemini `response_schema`). Never parse prose for JSON. Never use assistant prefill; never pass `temperature` to a Claude model.
2. The `/vision` prompt contains **no** instruction to interpret, summarize, or contextualize. Verbatim only. That string is hashed.
3. `/codegen` asserts `scene_class == "GeneratedScene"` and that the source contains `class GeneratedScene(Scene)`.
4. Retrieval filters on `verified: true` in every code path. No debug flag disables it.
5. Index and query use the same `EMBED_MODEL`. Changing it means recreating the index and re-seeding.
6. `/vision` is Gemini at `temperature=0`. `/explain` is Opus 5. `/codegen` is Sonnet 5 via streaming. Model IDs come from config, never hardcoded in a router.

---

## Decisions that are yours

- **Transcription prompt** — how verbatim is verbatim (line breaks, LaTeX vs Unicode math). Sprint 2 Step 5.
- **Embedding model** — `voyage-code-3` is the default; if prose-to-prose matching looks weak in Sprint 2, try `voyage-3.5` *before* the corpus is large (re-embedding is cheap now, expensive later).
- **What "teach the method, not the answer" means** in the guardrails prompt.
- **Storyboard bias** — the bad/good example pairs in `explain_*.md` that push toward motion over algebra.

## If you're blocked

You shouldn't be — you have no upstream dependencies. If `samples/` is empty when you want to seed, seed from the single smoke-test scene in SETUP §6.2 and re-run when P2 lands more.

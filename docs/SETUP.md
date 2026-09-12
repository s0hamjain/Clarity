# Clarity — Setup Guide

**Version:** 3.0

### What you're setting up

Clarity is four programs plus a few services, all running on your Mac:

| You'll run… | Which is… | From |
|---|---|---|
| The **agent service** | Python server that calls Claude | §8 |
| The **coordinator** | Go server the desktop app talks to | §9 |
| The **desktop app** | The menu-bar app itself | §10 |
| **Docker** with the `manim-worker` image | Sandbox every animation renders inside | §6 |
| **MinIO** | A local stand-in for S3, holds finished videos | §7 |

…and you'll need accounts for **Google AI Studio** (Gemini — reads the screenshot), **Anthropic** (Claude — explanation and code), **Voyage AI** (embeddings), and **MongoDB Atlas** (free; database + vector search). Sections 3–5 walk through each.

This document walks every engineer through the full environment setup required to develop and run the whole system on one Mac. Complete **all sections** before starting sprint work. The two steps most likely to burn time if left for later are the `manim-worker` Docker build (§6) and the macOS permission prompts for the desktop app (§10) — do those first.

Assumes macOS 14+ on Apple Silicon with Homebrew. Linux works for everything except the desktop app and installer.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Repository Setup](#2-repository-setup)
3. [Model API Keys — Google Gemini + Anthropic](#3-model-api-keys--google-gemini--anthropic)
4. [Voyage AI API Key (Embeddings)](#4-voyage-ai-api-key-embeddings)
5. [MongoDB Atlas (Jobs, Cache, Snippet Corpus)](#5-mongodb-atlas-jobs-cache-snippet-corpus)
6. [Docker + `manim-worker` Image](#6-docker--manim-worker-image)
7. [MinIO (Local S3)](#7-minio-local-s3)
8. [Agent Service — Python + FastAPI](#8-agent-service--python--fastapi)
9. [Coordinator — Go](#9-coordinator--go)
10. [Desktop App — Development Mode](#10-desktop-app--development-mode)
11. [Building the Installer](#11-building-the-installer)
12. [Environment Files](#12-environment-files)
13. [Running Everything](#13-running-everything)
14. [Verification Checklist](#14-verification-checklist)
15. [Common Problems](#15-common-problems)

---

## 1. Prerequisites

### System Requirements
- macOS 14 Sonoma or 15 Sequoia, Apple Silicon
- 16 GB RAM recommended (Docker + several concurrent Manim renders)
- ~10 GB free disk (Docker image is ~3 GB)

### Core Tools

```sh
xcode-select --install                       # compilers, git
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
brew install git python@3.12 go ffmpeg create-dmg
brew install --cask docker
```

### Verify installations

```sh
git --version          # 2.40+
python3.12 --version   # 3.12.x
go version             # go1.22+
ffmpeg -version        # any recent
docker --version       # 27+
open -a Docker         # start Docker Desktop; wait for "Docker Desktop is running"
```

---

## 2. Repository Setup

### 2.1 Clone

```sh
git clone https://github.com/s0hamjain/Clarity.git
cd Clarity
```

### 2.2 Directory skeleton

Each person creates their own top-level directory in Sprint 1 (see `FILE_STRUCTURE.md`). Nothing under `agent/`, `server/`, `desktop/`, `docker/`, or `samples/` exists at the start.

### 2.3 Git Branch Strategy

Each person works on their own branch per sprint:

```
p1/sprint-N-short-description
p2/sprint-N-short-description
p3/sprint-N-short-description
p4/sprint-N-short-description
```

Merge to `main` **only at sprint sync points**, in the order and with the protocol in `docs/WORK_SPLIT.md → Merge Protocol`. Your own tasks are in `docs/P1_AI.md` … `docs/P4_DESKTOP.md`. The one exception: a change to `docs/FRD.md` goes to `main` immediately, in its own commit, and is announced — everyone is building against it.

---

## 3. Model API Keys — Google Gemini + Anthropic

Two model providers. Everyone needs both keys — you'll run the full stack locally.

| Provider | Used for | Model |
|---|---|---|
| **Google Gemini** | Reading the problem off the screenshot (`/vision`) | `gemini-3.8-flash` |
| **Anthropic Claude** | Explanation (`/explain`) and Manim code (`/codegen`) | `claude-opus-5`, `claude-sonnet-5` |

### 3.1 Gemini
1. Go to https://aistudio.google.com → **Get API key** → Create.
2. Put it in `agent/.env` as `GEMINI_API_KEY=AIza...` (§12). Never commit it.

```sh
pip install google-genai
GEMINI_API_KEY=AIza... python3 -c "from google import genai; c=genai.Client(); print(c.models.get(model='gemini-3.8-flash').display_name)"
```

### 3.2 Anthropic
1. Go to https://console.anthropic.com → API Keys → Create Key.
2. Put it in `agent/.env` as `ANTHROPIC_API_KEY=sk-ant-...`.

```sh
pip install anthropic
ANTHROPIC_API_KEY=sk-ant-... python3 -c "import anthropic; c=anthropic.Anthropic(); print(c.models.retrieve('claude-sonnet-5').display_name)"
```

---

## 4. Voyage AI API Key (Embeddings)

Voyage AI provides the embedding model (`voyage-code-3`) used to index the Manim snippet corpus and to embed scene queries. It is owned by MongoDB and pairs with Atlas Vector Search.

1. Go to https://dash.voyageai.com → sign up → API Keys → Create.
2. Put it in `agent/.env` as `VOYAGE_API_KEY=pa-...`.

Sanity check:

```sh
pip install voyageai
VOYAGE_API_KEY=pa-... python3 -c "import voyageai; r=voyageai.Client().embed(['hello'], model='voyage-code-3', input_type='query'); print(len(r.embeddings[0]))"
# → 1024
```

That number (1024) must match `numDimensions` in the vector index (§5.4).

---

## 5. MongoDB Atlas (Jobs, Cache, Snippet Corpus)

One free-tier cluster holds everything persistent: `jobs`, `cache`, and `manim_snippets` (with its vector index). One person creates the cluster and shares the connection string; everyone points at the same cluster.

### 5.1 Create the cluster
1. https://cloud.mongodb.com → sign up → **Create** → **M0 Free** → region closest to you → name it `clarity`.
2. Wait for deployment (~2 min).

### 5.2 Database user
1. **Security → Database Access → Add New Database User.**
2. Username `clarity`, autogenerate a password, role **Read and write to any database**. Save the password.

### 5.3 Network access
1. **Security → Network Access → Add IP Address → Allow Access from Anywhere** (`0.0.0.0/0`).
   Fine for development on a free cluster. Tighten later if it ever matters.

### 5.4 Connection string
1. **Database → Connect → Drivers → Python**. Copy the `mongodb+srv://...` string.
2. Replace `<password>`, append `/clarity`. Put it in **both** `agent/.env` and `server/.env` as `MONGODB_URI`.

### 5.5 Vector Search index

The `manim_snippets` collection needs an Atlas Vector Search index named `snippets_vector`. Create it via the seed script (preferred) or the UI.

**Via script** (P1 owns this; run once after §8):
```sh
cd agent && python scripts/seed_snippets.py --create-index
```

**Via UI:** Database → Browse Collections → `clarity.manim_snippets` → **Search Indexes → Create → Atlas Vector Search → JSON Editor**, name `snippets_vector`:

```json
{
  "fields": [
    { "type": "vector", "path": "embedding", "numDimensions": 1024, "similarity": "cosine" },
    { "type": "filter", "path": "verified" },
    { "type": "filter", "path": "category" }
  ]
}
```

Wait for status **Active** (~1 min). M0 supports up to 3 search indexes; we use one.

### 5.6 TTL indexes

The coordinator creates these at startup (`server/internal/store/indexes.go`). No manual step. If you want to confirm:

```js
db.jobs.getIndexes()    // expect { updated_at: 1 } with expireAfterSeconds: 86400
db.cache.getIndexes()   // expect { created_at: 1 } with expireAfterSeconds: 604800
```

### 5.7 Verify

```sh
pip install pymongo
MONGODB_URI='mongodb+srv://...' python3 -c "import os,pymongo; print(pymongo.MongoClient(os.environ['MONGODB_URI']).admin.command('ping'))"
# → {'ok': 1.0}
```

---

## 6. Docker + `manim-worker` Image

Every scene renders inside a container from this image. Build it before writing any render code — it takes real minutes.

### 6.1 Build

```sh
docker build -t manim-worker -f docker/manim-worker/Dockerfile docker/manim-worker
```

The Dockerfile (P2 owns it) installs Python 3.12, Manim CE, a TeX distribution with `dvisvgm`, and ffmpeg.

### 6.2 Smoke test — against the image, not the host

```sh
mkdir -p /tmp/manim-smoke
cat > /tmp/manim-smoke/scene.py <<'PY'
from manim import *
class GeneratedScene(Scene):
    def construct(self):
        t = MathTex(r"\frac{d}{dx}\left[x^2 \sin x\right] = 2x\sin x + x^2\cos x")
        self.play(Write(t))
        self.wait(1)
PY
docker run --rm --network none -v /tmp/manim-smoke:/work manim-worker \
    manim -ql /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media
ls /tmp/manim-smoke/media/videos/scene/*/out.mp4
```

If the MP4 exists, the image is right. If it dies on `latex` or `dvisvgm`, the Dockerfile is missing a TeX package — fix the image and rebuild. There is no host-side fix.

Flags: `-ql` 480p (dev) · `-qm` 720p (release) · `-o` filename · `--media_dir` keeps scratch inside the mounted dir.

---

## 7. MinIO (Local S3)

MinIO speaks the S3 API. Locally nobody needs AWS credentials; the same Go code talks to real S3 by changing `S3_ENDPOINT`.

```sh
docker run -d --name minio -p 9000:9000 -p 9001:9001 \
    -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
    -v minio-data:/data minio/minio server /data --console-address ":9001"

brew install minio/stable/mc
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/clarity-renders
mc anonymous set download local/clarity-renders     # public-read, matches FRD §14.6
```

Console: http://localhost:9001 (minioadmin / minioadmin).

---

## 8. Agent Service — Python + FastAPI

```sh
cd agent
python3.12 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt     # google-genai anthropic fastapi uvicorn pydantic voyageai pymongo python-dotenv
cp .env.example .env                 # fill in §12 values
```

### 8.1 Seed the snippet corpus

```sh
python scripts/seed_snippets.py --create-index      # first time: creates snippets_vector
python scripts/seed_snippets.py                     # embeds and upserts every samples/*.py
```

### 8.2 Run

```sh
uvicorn app.main:app --port 8000 --reload
curl localhost:8000/healthz
# → {"ok":true,"gemini":true,"anthropic":true,"voyage":true,"atlas":true,"snippets_verified":N,...}
```

---

## 9. Coordinator — Go

```sh
cd server
cp .env.example .env                 # fill in §12 values
go mod download
go run ./cmd/server
curl localhost:8080/healthz
# → {"ok":true,"atlas":true,"docker":true,"s3":true,"agent":true}
```

`docker`, `s3`, and `agent` are checked once at boot. If any is `false`, fix that service and restart.

---

## 10. Desktop App — Development Mode

Run from source before building the installer. **Use Terminal.app for all of this, consistently** — macOS grants Screen Recording and Input Monitoring permissions to the *host application* that launched Python (Terminal, iTerm, or VS Code), and switching terminals means re-granting.

### 10.1 Install

```sh
cd desktop
python3.12 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt     # rumps pynput pywebview pillow requests pyinstaller
```

### 10.2 Grant permissions — once

```sh
screencapture -i /tmp/x.png          # → macOS asks for Screen Recording. Grant it.
```
**Quit and reopen Terminal.** Run it again; a crosshair should appear.

```sh
python -c "from pynput import keyboard; print('ok')"
python -m clarity --once                # → macOS asks for Input Monitoring. Grant it.
```
**Quit and reopen Terminal** again.

### 10.3 Run

```sh
python -m clarity                       # menu bar icon appears; ⌘⇧E is live
python -m clarity --once                # one capture without the hotkey — for testing the pipeline
```

Server URL defaults to `http://localhost:8080`. Change it via the menu bar **Server…** item or edit `~/Library/Application Support/Clarity/config.json`.

### 10.4 UI development without the backend

```sh
cd server && FAKE_AGENT=1 FAKE_RENDER=1 go run ./cmd/server   # walks a job through every status on a timer; needs no Atlas, Docker, or keys
```

---

## 11. Building the Installer

P4 owns `build_app.sh` and `build_dmg.sh`; P3 owns `release/build_pkg.sh` and `release/publish.sh`. Anyone can run any of them.

### 11.1 Self-signed certificate — once per machine

macOS keys the Screen Recording permission to the app's code signature. An unsigned build changes identity every rebuild and the permission resets. A free self-signed certificate gives a stable identity.

1. **Keychain Access → Certificate Assistant → Create a Certificate.**
2. Name `Clarity Dev`, Identity Type **Self Signed Root**, Certificate Type **Code Signing**. Create.

### 11.2 Build the `.app`

```sh
cd desktop && source .venv/bin/activate
./scripts/build_app.sh               # pyinstaller → dist/Clarity.app, patches Info.plist (LSUIElement), codesigns with "Clarity Dev"
open dist/Clarity.app                   # menu bar icon should appear
```

### 11.3 Build the `.dmg`

```sh
./scripts/build_dmg.sh               # create-dmg → dist/Clarity.dmg
```

### 11.4 Build the `.pkg` (adds start-at-login)

```sh
cd ../release && ./build_pkg.sh      # pkgbuild + productbuild → ../desktop/dist/Clarity.pkg; postinstall writes the LaunchAgent
```

### 11.5 Install like a user would

Mount the DMG, drag to Applications, open. On macOS 15 the unsigned app is blocked: **System Settings → Privacy & Security → scroll down → Open Anyway**. Grant Screen Recording, relaunch, grant Input Monitoring, relaunch. `⌘⇧E`.

### 11.6 Release

```sh
./release/publish.sh                 # gh release create v0.1.0 desktop/dist/Clarity.dmg desktop/dist/Clarity.pkg --notes-file release/RELEASE_NOTES.md
```

---

## 12. Environment Files

### 12.1 `agent/.env`

```sh
GEMINI_API_KEY=AIza...
ANTHROPIC_API_KEY=sk-ant-...
VOYAGE_API_KEY=pa-...
MONGODB_URI=mongodb+srv://clarity:<password>@clarity.xxxxx.mongodb.net/clarity
MONGODB_DB=clarity
EMBED_MODEL=voyage-code-3
VISION_MODEL=gemini-3.8-flash
EXPLAIN_MODEL=claude-opus-5
CODEGEN_MODEL=claude-sonnet-5
```

### 12.2 `server/.env`

```sh
PORT=8080
AGENT_URL=http://localhost:8000
MONGODB_URI=mongodb+srv://clarity:<password>@clarity.xxxxx.mongodb.net/clarity
MONGODB_DB=clarity
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
RENDER_BUCKET=clarity-renders
RENDER_CONCURRENCY=4
RENDER_TIMEOUT_SEC=120
MANIM_QUALITY=-ql
```

### 12.3 Desktop — `~/Library/Application Support/Clarity/config.json`

Created on first run. No secrets.

```json
{ "server_url": "http://localhost:8080", "guardrails": false, "hotkey": "<cmd>+<shift>+e" }
```

### 12.4 Security rules
- `.env` files are gitignored. Never commit one. Never paste a key into chat.
- The desktop app never holds an API key. If you find yourself adding one, stop.
- `MONGODB_URI` contains a password. Same rules.

---

## 13. Running Everything

Four processes plus Docker:

```sh
open -a Docker && docker start minio
cd agent   && source .venv/bin/activate && uvicorn app.main:app --port 8000
cd server  && go run ./cmd/server
cd desktop && source .venv/bin/activate && python -m clarity
```

Then `⌘⇧E`, drag a box around a problem, press Enter.

---

## 14. Verification Checklist

| # | Check | Command / Action | Expected |
|---|---|---|---|
| 1 | Python | `python3.12 --version` | 3.12.x |
| 2 | Go | `go version` | 1.22+ |
| 3 | Docker running | `docker info` | no error |
| 4 | `manim-worker` built | `docker images manim-worker` | one row |
| 5 | Container renders LaTeX | §6.2 smoke test | `out.mp4` exists |
| 6 | MinIO up + bucket public | `mc anonymous get local/clarity-renders` | `download` |
| 7 | Atlas reachable | §5.7 ping | `{'ok': 1.0}` |
| 8 | Vector index active | Atlas UI → Search Indexes | `snippets_vector` **Active** |
| 9 | Voyage key works | §4 sanity check | `1024` |
| 10 | Anthropic key works | §3.2 sanity check | `Claude Sonnet 5` |
| 10b | Gemini key works | §3.1 sanity check | `Gemini 3.8 Flash` |
| 11 | Corpus seeded | `curl localhost:8000/healthz` | `snippets_verified` ≥ 20 |
| 12 | Coordinator healthy | `curl localhost:8080/healthz` | all `true` |
| 13 | Screen Recording granted | `screencapture -i /tmp/x.png` from Terminal | crosshair appears |
| 14 | Input Monitoring granted | `python -m clarity` then `⌘⇧E` | crosshair appears |
| 15 | `.env` ignored | `git check-ignore agent/.env server/.env` | both printed |

---

## 15. Common Problems

- **`latex: command not found` inside `docker run`** — Dockerfile lacks TeX or `dvisvgm`. Fix the image, rebuild, re-run §6.2.
- **Docker daemon not running** — Docker Desktop must be open, not just installed.
- **`$vectorSearch` returns nothing** — index not yet **Active**, or `numDimensions` ≠ 1024, or every document is `verified: false`. Check `snippets_verified` in `/healthz`.
- **Embeddings dimension mismatch on insert** — `EMBED_MODEL` changed. Drop and recreate the index, re-run the seed script.
- **MinIO uploads succeed but the panel can't play the video** — bucket isn't public-read. `mc anonymous set download local/clarity-renders`.
- **`ffmpeg concat` fails** — codec mismatch between scenes. Confirm every scene in the job used the same `MANIM_QUALITY`.
- **Hotkey does nothing** — Input Monitoring not granted to *this* terminal app, or you didn't restart it after granting. System Settings → Privacy & Security → Input Monitoring.
- **`screencapture` produces no file and no crosshair** — Screen Recording not granted to this terminal app. Same fix.
- **"Python is accessing your screen" prompt keeps appearing** — macOS 15 re-prompts periodically for apps not using ScreenCaptureKit. Click Allow. The built `.app` with a stable signature prompts less.
- **Built `.app` asks for permissions again after every rebuild** — it isn't being signed with the `Clarity Dev` certificate. Check `build_app.sh`'s `codesign` step.
- **Cache ignores prompt changes** — `PromptVersion` not bumped. `server/internal/cache/key.go`.
- **Job stuck in `rendering`** — a goroutine panicked without `recover()`, or `updated_at` wasn't refreshed and the TTL index deleted the job. Check coordinator logs.

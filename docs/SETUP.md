# Setup

Everything runs on one laptop, using Docker for render isolation and a local
S3-compatible store so nobody needs real AWS credentials to develop. Get this
working before writing any code in your lane — especially the Docker image
build, which is the thing most likely to eat an hour if left for later.

Assumes macOS with Homebrew. Adjust for Linux; Windows is on your own.

## 0. Clone

```sh
git clone https://github.com/s0hamjain/HackCMU.git
cd HackCMU
```

## 1. Anthropic API key

Everyone needs one, even if you're not on the `agent/` lane — you'll be running
the full stack locally.

```sh
export ANTHROPIC_API_KEY=sk-ant-...
```

Put it in your shell profile. Never commit it. `.env` files are gitignored.

Sanity check:

```sh
pip install anthropic
python -c "import anthropic; c=anthropic.Anthropic(); print(c.models.retrieve('claude-opus-5').display_name)"
```

## 2. Docker — do this early, it's the critical path

```sh
brew install --cask docker
open -a Docker   # start Docker Desktop, wait for it to say "running"
docker --version
```

### Build `manim-worker` — before writing any render/ code

This image is Python + Manim CE + LaTeX + `dvisvgm` + ffmpeg, baked in once so
no render pays an install cost.

```sh
docker build -t manim-worker -f docker/manim-worker/Dockerfile docker/manim-worker
```

### The smoke test — run it against the image, not the host

A LaTeX check on your host tells you nothing about whether the *image* has
LaTeX. Run the real thing:

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
docker run --rm -v /tmp/manim-smoke:/work manim-worker \
    manim -ql /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media
ls /tmp/manim-smoke/media/videos/scene/*/out.mp4
```

If the MP4 exists, you're good. If it dies on `latex` or `dvisvgm`, the
`Dockerfile` is missing a TeX package — fix the image, not your host, and
rebuild. Nothing downstream works until this passes.

Flags you'll use: `-ql` (480p, fast, for dev) · `-qm` (720p, for the demo) ·
`-o <name>` (output filename) · `--media_dir` (keep scratch output inside the
per-job temp dir, not wherever manim feels like).

## 3. MinIO — local S3, no AWS account needed

```sh
docker run -d --name minio -p 9000:9000 -p 9001:9001 \
    -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
    -v minio-data:/data \
    minio/minio server /data --console-address ":9001"
```

Console at `http://localhost:9001` (minioadmin / minioadmin). Create a bucket
named `hackcmu-renders` there, or via the client:

```sh
brew install minio/stable/mc
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/hackcmu-renders
mc anonymous set download local/hackcmu-renders   # public-read, matches CONTRACTS default
```

Go's S3 client (AWS SDK v2) talks to MinIO the same way it talks to real AWS —
only `S3_ENDPOINT` changes. On the presenting machine for the demo, point
`S3_ENDPOINT` at real AWS instead and re-verify before code freeze.

## 4. Python (agent service)

```sh
brew install python@3.12
cd agent
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt   # anthropic, fastapi, uvicorn, pydantic, sentence-transformers
```

`sentence-transformers` is for local, no-API-key embeddings used by
`/manim-docs` retrieval — CPU-only, downloads a small model on first run. If
it's not installed in time, retrieval falls back to keyword matching; the
service still runs either way.

Run:

```sh
uvicorn main:app --port 8000 --reload
```

Check: `curl localhost:8000/healthz`

## 5. Go (server + render)

```sh
brew install go        # 1.22+
cd server
go mod download
go run ./cmd/server
```

Check: `curl localhost:8080/healthz`

## 6. Redis

```sh
brew install redis
brew services start redis     # or: redis-server, in its own terminal
redis-cli ping                # → PONG
```

Default `localhost:6379`, no auth. The server reads `REDIS_ADDR` if you need to
change it.

## 7. Angular (client)

```sh
brew install node             # 20+
npm install -g @angular/cli
cd client
npm install
```

Three projects in one workspace — build each separately:

```sh
ng build extension-panel      # → client/dist/extension-panel
ng build web-app --watch      # → client/dist/web-app, rebuilds on save
```

### Load the extension

1. `chrome://extensions` → toggle **Developer mode** (top right).
2. **Load unpacked** → select `client/dist/extension-panel/`.
3. Pin it. The hotkey is set in `manifest.json` under `commands`; you can
   rebind at `chrome://extensions/shortcuts`.
4. Reload the extension after every change to `manifest.json` or the
   background script. Panel-only changes usually just need a rebuild
   (`ng build extension-panel`) and the panel reopened — the extension itself
   doesn't need reloading for those.

Server URL is set in `client/projects/extension-panel/src/config.ts`.

### Web app

```sh
cd client/dist/web-app && python3 -m http.server 3000
```

Open `http://localhost:3000`.

## 8. Everything at once

Six things running at once (Docker Desktop + `docker run`, or one `tmux`):

```sh
docker ps            # confirm minio is up (started in step 3)
redis-server
cd agent  && source .venv/bin/activate && uvicorn main:app --port 8000
cd server && go run ./cmd/server
cd client && ng build web-app --watch
cd client/dist/web-app && python3 -m http.server 3000
```

Then `curl localhost:8080/healthz` should return
`{"ok":true,"redis":true,"docker":true,"s3":true,"agent":true}`.

## Environment variables

| Var | Default | Used by |
|---|---|---|
| `ANTHROPIC_API_KEY` | — | agent |
| `AGENT_URL` | `http://localhost:8000` | server |
| `REDIS_ADDR` | `localhost:6379` | server |
| `S3_ENDPOINT` | `http://localhost:9000` (MinIO) | server (render lane) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `minioadmin` / `minioadmin` | server (render lane) |
| `RENDER_BUCKET` | `hackcmu-renders` | server (render lane) |
| `RENDER_CONCURRENCY` | `NumCPU/2` | server |
| `RENDER_TIMEOUT_SEC` | `120` | server |
| `PORT` | `8080` | server |

## Common problems

- **`latex: command not found` inside a `docker run`** — the `Dockerfile`
  doesn't install a TeX distribution or `dvisvgm`. Fix the image, rebuild,
  re-run the smoke test. There is no host-side fix for this.
- **`docker: command not found` / daemon not running** — Docker Desktop has to
  actually be open, not just installed. Check the whale icon in the menu bar.
- **MinIO bucket exists but uploads 403** — the bucket needs
  `mc anonymous set download` (or an equivalent policy) for public-read URLs
  to work, matching the CONTRACTS default. Switch to presigned URLs if you'd
  rather not make it public.
- **`ffmpeg concat` fails or produces a broken file** — almost always a codec
  mismatch between scenes. Confirm every scene in the job used the same manim
  quality flag (`-ql` vs `-qm`) — see CONTRACTS §4.
- **Extension hotkey does nothing** — another extension owns it. Rebind at
  `chrome://extensions/shortcuts`.
- **`captureVisibleTab` returns nothing** — you're on a `chrome://` page or the
  Web Store. Chrome refuses. Try a normal page.
- **Cache seems to ignore prompt changes** — you didn't bump `PromptVersion`.
  It's in `server/internal/cache/key.go`.
- **Cache seems to ignore the guardrails toggle** — check `guardrails` is
  actually part of the hash input, not just stored alongside the job. See
  CONTRACTS §3.
- **CORS error in the extension** — server must send
  `Access-Control-Allow-Origin: *`. Check it's on *every* route, including
  404s and 5xxs.

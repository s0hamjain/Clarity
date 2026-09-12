# Setup

Everything runs on one laptop. Get this working before writing any code in your
lane — especially the LaTeX check, which is the thing most likely to be broken.

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

## 2. Python (agent service)

```sh
brew install python@3.12
cd agent
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt   # anthropic, fastapi, uvicorn, pydantic
```

Run:

```sh
uvicorn main:app --port 8000 --reload
```

Check: `curl localhost:8000/healthz`

## 3. Go (server + render)

```sh
brew install go        # 1.22+
cd server
go mod download
go run ./cmd/server
```

Check: `curl localhost:8080/healthz`

## 4. Redis

```sh
brew install redis
brew services start redis     # or: redis-server, in its own terminal
redis-cli ping                # → PONG
```

Default `localhost:6379`, no auth. The server reads `REDIS_ADDR` if you need to
change it.

## 5. Manim — do this first if you're on `render/`

Manim Community Edition, **not** the 3b1b repo. Python 3.9+.

```sh
brew install py3cairo ffmpeg pango pkg-config scipy
pip install manim
manim --version
```

### LaTeX — verify this before anything else

`MathTex` needs a TeX distribution *and* `dvisvgm`. Without it every math
animation fails, and the failure message is not obvious.

```sh
brew install --cask basictex
# restart your terminal so tlmgr is on PATH, then:
sudo tlmgr update --self
sudo tlmgr install standalone preview dvisvgm doublestroke relsize fundus-calligra \
    wasysym physics dvipng rsfs wasy cm-super babel-english gnu-freefont \
    mathastext cbfonts-fd
which latex dvisvgm
```

(Full MacTeX also works — it's 5 GB but has everything.)

### The smoke test

```sh
cat > /tmp/smoke.py <<'PY'
from manim import *
class GeneratedScene(Scene):
    def construct(self):
        t = MathTex(r"\frac{d}{dx}\left[x^2 \sin x\right] = 2x\sin x + x^2\cos x")
        self.play(Write(t))
        self.wait(1)
PY
manim -ql /tmp/smoke.py GeneratedScene
```

If an MP4 appears under `media/videos/smoke/480p15/`, you're good. If it dies on
`latex` or `dvisvgm`, fix that now — nothing downstream works until it does.

Flags you'll use: `-ql` (480p, fast, for dev) · `-qm` (720p, for the demo) ·
`-o <name>` (output filename).

## 6. Chrome extension

1. `chrome://extensions` → toggle **Developer mode** (top right).
2. **Load unpacked** → select the `client/extension/` folder.
3. Pin it. The hotkey is set in `manifest.json` under `commands`; you can rebind
   at `chrome://extensions/shortcuts`.
4. Reload the extension after every change to `manifest.json` or the background
   script. Content/panel changes usually just need the panel reopened.

Server URL is hardcoded to `http://localhost:8080` in `client/extension/config.js`.

## 7. Paste page

Static. Any file server works:

```sh
cd client/web && python3 -m http.server 3000
```

Open `http://localhost:3000`.

## 8. Everything at once

Four terminals (or one `tmux`):

```sh
redis-server
cd agent  && source .venv/bin/activate && uvicorn main:app --port 8000
cd server && go run ./cmd/server
cd client/web && python3 -m http.server 3000
```

Then `curl localhost:8080/healthz` should return
`{"ok":true,"redis":true,"manim":true,"agent":true}`.

## Environment variables

| Var | Default | Used by |
|---|---|---|
| `ANTHROPIC_API_KEY` | — | agent |
| `AGENT_URL` | `http://localhost:8000` | server |
| `REDIS_ADDR` | `localhost:6379` | server |
| `RENDER_DIR` | `./renders` | server |
| `RENDER_CONCURRENCY` | `NumCPU/2` | server |
| `RENDER_TIMEOUT_SEC` | `120` | server |
| `PORT` | `8080` | server |

## Common problems

- **`latex: command not found` inside manim** — terminal opened before basictex
  install finished. Restart the terminal. Check `echo $PATH` includes
  `/Library/TeX/texbin`.
- **`dvisvgm` missing** — `sudo tlmgr install dvisvgm`.
- **Extension hotkey does nothing** — another extension owns it. Rebind at
  `chrome://extensions/shortcuts`.
- **`captureVisibleTab` returns nothing** — you're on a `chrome://` page or the
  Web Store. Chrome refuses. Try a normal page.
- **Cache seems to ignore prompt changes** — you didn't bump `PromptVersion`.
  It's in `server/internal/cache/key.go`.
- **CORS error in the extension** — server must send
  `Access-Control-Allow-Origin: *`. Check it's on *every* route, including 404s.

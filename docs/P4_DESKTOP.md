# Clarity — P4: Desktop App and Installer

**You are P4. Your job in one sentence:** build everything the user sees and installs — the menu-bar app, the hotkey, the region screenshot, the translucent input box, the floating result window, and the `.dmg` that puts it on someone else's Mac.

You never touch a server. You talk to one thing: P3's HTTP API on `localhost:8080`. Until that exists, you talk to a stub you write in 40 lines.

---

## Your job in plain words

The user's experience, in order — this is what you're building:

1. **`⌘⇧E`** anywhere → crosshair → the user **drags a region** around the problem → screenshot taken. (Esc cancels.)
2. A translucent **spotlight box animates in** (fade + scale, ~180 ms) with a thumbnail of the capture and one text field: *"Add context… (why is my binary search not working? visualize where it's messing up)"*. Typing is optional. **↓** shows recent screenshots.
3. **Enter** → the box animates out → the screenshot and text go to the server.
4. A translucent **result box appears**. It shows "Reading the problem…", then the written explanation the moment it exists (seconds), then the video (about a minute). The user can **drag it anywhere** so it doesn't cover the problem.
5. **X** (or Esc) closes it. Closing early cancels the job.

Plus: **recents** — the last 50 screenshots live on the user's Mac, and the spotlight box can list them so the user can re-ask about an old one. And the **installer** — a `.dmg` anyone can download.

---

## What you own · what you never touch

| You own | You never touch |
|---|---|
| `desktop/**` — the whole directory | `server/` (P2, P3) |
| The two windows' HTML/CSS/JS | `agent/` (P1) |
| The build scripts and release | Anything about how a job is *processed* — you only display what the API returns |

---

## Where your work meets others

One boundary. You **call** P3's public API; the exact requests and responses are in **[API.md §2](API.md#2-coordinator-api)**. Read it in full — it's short. Specifically:

| You call | When | You use from the response |
|---|---|---|
| `POST /api/jobs` | User presses Enter in the spotlight box | `job_id` |
| `GET /api/jobs/{id}` | Every 1 s while the result box is open | `status`, `explanation`, `scenes_done`/`scenes_total`, `video_url`, `problem_text`, `error` |
| `DELETE /api/jobs/{id}` | User closes the result box before `done` | nothing — fire and forget |
| `GET /healthz` | On launch and from the **Server…** menu | `ok` |

**The two rules you must honor** (API.md §5): render `explanation` the moment it is non-null, whatever `status` says; treat `done` with `video_url: null` as a quiet note ("The animation didn't render this time"), not an error.

**Nobody is waiting on you.** Build against your stub until Sprint 2.

---

## Before you start (45 minutes — the permission prompts are the long pole)

Follow [SETUP.md](SETUP.md) §1 and **§10 in full**. Use Terminal.app and keep using it — macOS grants Screen Recording and Input Monitoring to whichever app launched Python, and switching to iTerm or VS Code's terminal means granting again.

Read once: **FRD §5** (the flow and the two windows), **FRD §15** (exact behavior and the installer table), **FRD §16** (end-to-end flows incl. re-ask), **API.md §2, §4, §5**, **FRD §23 rules 17–21** (yours).

---

## Files you'll create

```
desktop/
├── requirements.txt              # rumps pynput pywebview pillow requests pyinstaller
├── README.md                     # user-facing install notes: Open Anyway, the two permissions + restarts
├── RELEASE_NOTES.md
├── clarity/
│   ├── __main__.py               # python -m clarity ; --once = one capture, no hotkey
│   ├── app.py                    # rumps.App: Capture · Recents · Guardrails ☐ · Server… · Clear Recents · Quit
│   ├── config.py                 # ~/Library/Application Support/Clarity/config.json
│   ├── hotkey.py                 # pynput GlobalHotKeys, 2 s debounce, runs capture on a worker thread
│   ├── capture.py                # screencapture -i -x → Pillow ≤1568 px → PNG data URL ; Esc → None
│   ├── recents.py                # recents.json + recents/<id>.png + <id>_thumb.png ; rolling 50
│   ├── client.py                 # POST / GET / DELETE against the coordinator
│   ├── spotlight_window.py       # the input box (pywebview, frameless, transparent, vibrancy)
│   ├── result_window.py          # the output box (pywebview, frameless, on_top, draggable)
│   └── ui/
│       ├── shared/marked.min.js  # vendored markdown renderer — nothing loads from the network
│       ├── shared/base.css
│       ├── spotlight/{index.html, spotlight.js, spotlight.css}
│       └── result/{index.html, result.js, result.css}
├── stub/stub_server.py           # fake coordinator on :8080
├── assets/{icon.icns, menubar_idle.png, menubar_working.png, dmg_background.png}
└── scripts/{build_app.sh, build_dmg.sh, build_pkg.sh, postinstall.sh}
```

---

## Sprint 1 — Permissions, menu bar app, hotkey + capture, the transparency spike, the stub

**Goal:** the app lives in the menu bar; `⌘⇧E` produces a region screenshot on disk; you've proven the translucent-window trick works on your Mac; you have a fake server to build the UI against.

### Step 1 — Permissions first (30 min)
SETUP §10.2, exactly. Write down what you saw — the prompts, the restarts — in `desktop/README.md`. That paragraph ships to users.

### Step 2 — App skeleton (45 min)
- `clarity/app.py`: `rumps.App("Clarity", icon="assets/menubar_idle.png", template=True)`. Menu items: **Capture**, **Recents**, **Guardrails** (checkbox, persisted), **Server…** (text prompt for the URL), **Clear Recents**, **Quit**. No dock icon (`LSUIElement` — set in `Info.plist` at build time; in dev you'll see a dock icon, that's fine).
- `clarity/config.py`: read/write `~/Library/Application Support/Clarity/config.json` — `server_url` (default `http://localhost:8080`), `guardrails` (false), `hotkey` (`<cmd>+<shift>+e`). Create on first run.
- `__main__.py`: `--once` runs one capture and exits — your test hook for the whole pipeline without the hotkey.

### Step 3 — Hotkey + capture (1 h)
- `hotkey.py`: `pynput.keyboard.GlobalHotKeys({config.hotkey: on_fire})`. 2 s debounce. `on_fire` hands off to a worker thread — never block the listener.
- `capture.py`: `subprocess.run(["screencapture", "-i", "-x", tmp])`. Exit code 1 / no file = user pressed Esc → return `None`. Else `Pillow` `thumbnail((1568, 1568))` (keeps retina captures under the 8 MB API limit and at Claude's sweet spot), save PNG, return `data:image/png;base64,…`.

### Step 4 — The transparency spike (20 min) — do not skip
```python
import webview
webview.create_window("x", html="<body style='background:transparent;color:white'>hello</body>",
                      frameless=True, transparent=True, vibrancy=True, on_top=True, width=400, height=100)
webview.start()
```
Is it translucent and blurred over your desktop? **Yes** → the design works as specified. **No** → note it in your PR; Sprint 2 uses a solid dark box at 92 % opacity instead. Either way, answer FRD §24's question.

### Step 5 — Stub server (30 min)
`stub/stub_server.py`: `http.server`, ~40 lines. `POST /api/jobs` → `{"job_id": "j_stub"}`. `GET /api/jobs/j_stub` → walks through every status from API.md §5 on a timer (1 s each), with a sample markdown explanation from `generating` on, `scenes_total: 3` and `scenes_done` incrementing during `rendering`, and a `video_url` pointing at any MP4 on disk at `done`. `DELETE` → `{"status": "cancelled"}`. This is your server until Sprint 2.

### Done when
- [ ] `python -m clarity` → menu bar icon; all menu items present; Quit works.
- [ ] `⌘⇧E` → crosshair → drag → a PNG under 8 MB on disk. Esc → nothing happens, no error.
- [ ] Spike result recorded (translucent: yes/no).
- [ ] `python stub/stub_server.py` + `curl` walks a job through every status.

---

## Sprint 2 — Spotlight box, result box, real submit

**Goal:** the full visual flow works against the stub, then against P3's real server: capture → box animates in → type → Enter → result box shows a real explanation.

### Step 1 — Spotlight box (2 h)
`spotlight_window.py` + `ui/spotlight/`:
- `webview.create_window(url=…/spotlight/index.html, frameless=True, transparent=True, vibrancy=True, on_top=True, width=680, height=96, x=…, y=…)` centered on the display the cursor is on. Pass the thumbnail data URL and a `js_api` object with `submit(text)`, `cancel()`, `list_recents()`, `pick_recent(id)`.
- **Animate in**: CSS on `body` — `opacity: 0; transform: scale(0.96)` → `opacity: 1; transform: scale(1)`, 180 ms `ease-out`, triggered on load. Animate out on submit (120 ms) then `window.destroy()`.
- Layout: thumbnail (64×48, rounded) left; input right, placeholder *"Add context… (why is my binary search not working? visualize where it's messing up)"*; input focused on open.
- **Enter** → `js_api.submit(text)`. **Esc** → `js_api.cancel()`.
- If the spike said "no": solid `rgba(20,20,24,0.92)` background, same everything else.

### Step 2 — Result box (2 h)
`result_window.py` + `ui/result/`:
- `webview.create_window(url=…/result/index.html?job=<id>&server=<url>, frameless=True, transparent=True, on_top=True, vibrancy=True, easy_drag=True, width=440, height=680)`, positioned where the spotlight box was. `easy_drag=True` makes the **whole window a drag handle**; exclude the `<video>` and selectable text with `-webkit-app-region: no-drag` (pywebview honors it).
- Header: title "Clarity" left, **X** right. X and **Esc** → `window.close()` → also `DELETE /api/jobs/{id}` if not yet `done`.
- `result.js`: poll `GET /api/jobs/{id}` every 1 s. Status line in plain words: `queued`/`transcribing` → "Reading the problem…"; `explaining` → "Writing explanation…"; `generating` → "Planning the animation…"; `rendering` → "Rendering scene {done+1} of {total}…"; `concatenating` → "Joining scenes…"; `uploading` → "Almost there…"; `done` → "Done". **Render `explanation` via `marked` the first time it is non-null, whatever the status.** `<video controls autoplay muted src=video_url>` when `video_url` arrives. `done` + null `video_url` → keep the explanation, add a quiet line "The animation didn't render this time." `failed` + `no_problem_found` → "No problem found in that capture." Other `failed` → plain message + Retry. 404 → "This job expired." 180 s without a terminal status → message + Retry.
- Each job opens a **new** result box offset 24 px from the last; old ones stay until closed. Fade in 150 ms.

### Step 3 — Submit + notification (45 min)
`client.py`: `POST /api/jobs` with `{image, user_prompt, guardrails: config.guardrails, source: "desktop"}`. `requests.ConnectionError` → `rumps.notification("Clarity", "Can't reach the server", server_url)` and **leave the spotlight box open** so the user can retry. `rumps.notification` when the explanation first appears, so the user doesn't have to stare at the box.

### Step 4 — Point at the real server (15 min)
Stop the stub. `config.server_url = http://localhost:8080` (P3's). A real capture → a real explanation in the result box.

### Done when
- [ ] `⌘⇧E` → drag → spotlight box animates in, translucent (or the fallback) → type → Enter → animates out → result box appears.
- [ ] Against P3's server: real explanation visible in **< 10 s**. This is the product.
- [ ] Result box drags anywhere by its background; X and Esc close it.
- [ ] Server down → notification, spotlight box still open.

---

## Sprint 3 — Video, recents, every failure state, guardrails, cancel

**Goal:** the video plays; old screenshots come back; nothing the server does can make the UI look broken.

### Step 1 — Video (30 min)
`<video>` appears when `video_url` is set — no reload, `autoplay muted controls`. Progress text during `rendering` uses `scenes_done`/`scenes_total`.

### Step 2 — Recents store (1.5 h)
`recents.py`, in `~/Library/Application Support/Clarity/`:
- `recents.json` — a list of `{id, created_at, question, job_id, problem_hash, problem_text, explanation, video_url, screenshot_path, thumb_path}`.
- `add(image_data_url, question) -> id` writes the PNG and a 128-px thumbnail, appends the entry. **Call it before `POST /api/jobs`** so a failed submit is still visible.
- `update(id, **fields)` — the result box calls this (via `js_api`) whenever a poll adds `job_id`, `problem_text`, `explanation`, or `video_url`.
- Rolling cap 50: drop the oldest entry and delete its two files. `clear()` for the menu item.

### Step 3 — Recents in the spotlight box (2 h)
- Empty field + **↓** or `/` → the window grows downward (resize via `js_api`) into a list: up to 8 rows visible, scroll for more. Each row: thumbnail · `problem_text` or "Untitled capture" · relative time ("2 h ago") · a dot if `video_url` exists. Typing filters by `problem_text`.
- **Enter** on a row **with** a result → close spotlight; open a result box **from local data** (render `explanation` and `video_url` immediately, then one `GET /api/jobs/{id}` — a 404 is fine, the local copy wins).
- **Enter** on a row **without** a result, or **Tab** on any row → load that screenshot as the current capture; the thumbnail swaps; the user types and submits a new job.
- Menu bar **Recents** → opens the spotlight box with no capture and the list already expanded. `⌘⇧E` then Esc at the crosshair does the same.

### Step 4 — Every failure state (45 min)
Walk API.md §5 and FRD §19's desktop rows and make each one look deliberate: `done`+null video (quiet note), `no_problem_found`, generic `failed` (+ Retry), 404 on a live job ("expired"), 404 on a reopened recent (local copy wins, silently), 180 s (+ Retry), recent's screenshot file missing (thumbnail shown, re-ask disabled).

### Step 5 — Guardrails + cancel (30 min)
The **Guardrails** checkbox writes `config.json`; every `POST` sends the current value. Closing the result box before `done` → `DELETE /api/jobs/{id}`, fire-and-forget.

### Done when
- [ ] One capture → explanation → **video playing in the result box.** All real.
- [ ] Stop the server. `⌘⇧E`, Esc, ↓ → yesterday's capture listed; Enter → its result box opens from local data.
- [ ] Tab on a recent → new question about an old screenshot → new job.
- [ ] Every failure state in the list above has been triggered on purpose and looks intentional.
- [ ] X before `done` → P3 sees `cancelled`.

---

## Sprint 4 — The installer

**Goal:** someone who has never run this from source installs it from a `.dmg` and it works.

### Step 1 — Self-signed certificate (10 min)
Keychain Access → Certificate Assistant → Create a Certificate → name `Clarity Dev`, Identity Type **Self Signed Root**, Certificate Type **Code Signing**. This gives the app a *stable* identity so macOS remembers the Screen Recording permission across rebuilds. Unsigned builds change identity every time and the permission resets.

### Step 2 — `build_app.sh` (1.5 h)
```sh
pyinstaller --windowed --name Clarity --icon assets/icon.icns \
  --add-data "clarity/ui:clarity/ui" \
  --hidden-import rumps --hidden-import pynput.keyboard._darwin --hidden-import webview \
  clarity/__main__.py
/usr/libexec/PlistBuddy -c "Add :LSUIElement bool true" dist/Clarity.app/Contents/Info.plist
codesign --force --deep --sign "Clarity Dev" dist/Clarity.app
```
`open dist/Clarity.app` → menu bar icon, no dock icon. **Confirm the spotlight box is still translucent inside the bundle** — a bundled `.app` can behave differently from `python -m clarity`.

### Step 3 — `build_dmg.sh` (30 min)
`create-dmg --volname Clarity --background assets/dmg_background.png --window-size 600 400 --icon Clarity.app 150 200 --app-drop-link 450 200 dist/Clarity.dmg dist/Clarity.app`.

### Step 4 — Install on a second Mac (1 h)
A teammate who hasn't run from source. Mount, drag, open → macOS 15 blocks it → **System Settings → Privacy & Security → Open Anyway** → grant Screen Recording → relaunch → `⌘⇧E` → grant Input Monitoring → relaunch → `⌘⇧E` → works. Write `RELEASE_NOTES.md` from whatever they hit.

### Step 5 — Rebuild, reinstall, confirm (15 min)
Rebuild the `.app`, replace it in Applications, launch. It must **not** ask for Screen Recording again. If it does, the `codesign` step isn't running.

### Done when
- [ ] `Clarity.dmg` installs and works on a Mac that never ran from source.
- [ ] Permission survives a rebuild.
- [ ] Spotlight box translucent inside the bundle (or fallback in place).

---

## Sprint 5 — Release

- `build_pkg.sh` (if time): `pkgbuild` + `productbuild`; `postinstall.sh` writes `~/Library/LaunchAgents/com.clarity.app.plist` so it starts at login.
- `gh release create v0.1.0 dist/Clarity.dmg [dist/Clarity.pkg] --notes-file RELEASE_NOTES.md`.
- Install **from the release URL**, not from `dist/`. It works → done.

---

## Branch and merge — your steps

1. Start each sprint: `git checkout main && git pull && git checkout -b p4/sprint-N-<what>`.
2. Commit small. Push whenever.
3. **Before the sync point:** `git fetch origin && git rebase origin/main`, fix conflicts, run `python -m clarity --once` end to end, push.
4. **At the sync point:** merge order is **P3 → P1 → P2 → P4**. You're last — you consume everything. You're the merge captain in Sprint 4.
5. After the merge: back to step 1.

Full protocol: [WORK_SPLIT.md → Merge Protocol](WORK_SPLIT.md#merge-protocol).

---

## Your rules (never break these — FRD §23)

17. No secrets in the app. Only `server_url` and preferences.
18. Poll every 1 s, give up at 180 s, render `explanation` on the first non-null poll regardless of status.
19. Every network error becomes a notification or a message in the result box. Never an unhandled exception.
20. `ui/` loads nothing from the network except `video_url`. `marked.min.js` is bundled.
21. A recent is written to disk before `POST /api/jobs` and updated on every poll that adds data. Reopening a recent never depends on the server.

---

## Traps

- Permissions belong to the **host app** — Terminal, iTerm, VS Code — not to Python. Grant, then **restart that app**. "It worked yesterday" usually means you launched from a different terminal.
- `⌘⇧3/4/5` are taken by macOS screenshots. `⌘⇧E` is free.
- `screencapture` with Esc returns exit code 1 and no file. Handle it or you'll crash on the most common user action.
- Retina full-screen PNGs are 5–10 MB. The 1568-px thumbnail is what keeps you under the 8 MB limit.
- `easy_drag=True` makes *everything* a drag handle including the video's scrubber. Opt the video out.
- The cache key includes the question. If you pre-warm a demo problem with no question, typing a question makes it a cold request.

## If you're blocked

You shouldn't be. Sprint 1 is all local. Sprint 2 onward: if P3's server is down, switch `server_url` back to the stub and keep building UI.

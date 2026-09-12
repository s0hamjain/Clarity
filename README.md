# Clarity

Press a hotkey on any math or algorithm problem on your screen — a PDF, an IDE, a browser, a slide. Drag a box around it. Add context in a translucent Spotlight-style box (*"why is my binary search not working? visualize where it's messing up"*), or pull up a recent screenshot and ask again. Get a step-by-step written explanation in seconds, and a custom animated walkthrough of that exact problem about a minute later.

A macOS menu-bar app backed by a Go coordinator, a Python agent service calling Claude, retrieval over MongoDB Atlas Vector Search, and per-scene Manim rendering in isolated containers.

**Start with [AGENTS.md](AGENTS.md).** It's the index for everything.

| Doc | Purpose |
|---|---|
| [AGENTS.md](AGENTS.md) | Project summary, document map, mandatory rules, operating protocol, sprint index |
| [docs/FRD.md](docs/FRD.md) | Technical specification — every shape, schema, route, and rule |
| [docs/API.md](docs/API.md) | REST API reference for both services — endpoints, errors, lifecycle, limits |
| [docs/SETUP.md](docs/SETUP.md) | Environment setup and verification checklist |
| [docs/WORK_SPLIT.md](docs/WORK_SPLIT.md) | P1–P4 across five sprints, sync points, merge protocol |
| [FILE_STRUCTURE.md](FILE_STRUCTURE.md) | Directory layout and ownership |

## Install (once a release exists)

1. Download `Clarity.dmg` from the latest [GitHub Release](https://github.com/s0hamjain/Clarity/releases).
2. Drag **Clarity** to Applications and open it. On macOS 15: **System Settings → Privacy & Security → Open Anyway**.
3. Grant **Screen Recording** when asked, relaunch. Press `⌘⇧E`; grant **Input Monitoring**, relaunch.
4. `⌘⇧E`, drag a box around a problem, type any context, press Enter. Press ↓ in the box to pick a recent screenshot instead.

Requires the backend running — see [docs/SETUP.md](docs/SETUP.md).

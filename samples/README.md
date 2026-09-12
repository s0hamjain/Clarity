# `samples/` — the RAG corpus seed

These are verified, working Manim CE scenes. They are what the Manim Generator agent
imitates (FRD §13), so quality here directly controls how often generated code renders
on the first try. `agent/scripts/seed_snippets.py` (P1) parses every file in this
directory and embeds it into the `manim_snippets` collection.

## Docstring format — exact shape, the seed script depends on it

Every file starts with a module docstring in this shape:

```python
"""
title: Array walk with a moving pointer
description: A row of boxes built with VGroup.arrange; an arrow pointer steps left to right, highlighting each box.
category: algorithm
tags: VGroup, arrange, Arrow, Indicate
"""
from manim import *

class GeneratedScene(Scene):
    def construct(self):
        ...
```

- `title:` — short, specific.
- `description:` — one sentence, plain language, describes *what the viewer sees*, not
  the API calls. This (title + description + tags) is what gets embedded — not the
  source code — so it has to read like the natural-language scene descriptions the
  agent will query with.
- `category:` — `math` | `algorithm` | `general`.
- `tags:` — comma-separated Manim API names used, for humans skimming the corpus.

One scene per file. The class is always named `GeneratedScene` — this is also what the
static pre-check requires of every generated file. Relative positioning only:
`next_to`, `arrange`, `to_edge`, `shift` by fractions of `config.frame_width` /
`config.frame_height`. **Never literal coordinates** (`np.array([...])`,
`.move_to([x, y, z])`) — the generated code that imitates these samples has to stay
off-frame-safe without hardcoded numbers tuned to one aspect ratio.

Every scene is 5–15 seconds.

## "Watched, not just rendered" — the rule that matters most

A scene that exits 0 is not the same as a scene that looks right. Overlapping text,
objects that drift off the 14.2-unit frame, or a transform that happens too fast to
read all render successfully and are still bad samples. **Every file in this directory
must be rendered in `manim-worker` and watched, by a person, before it's committed.**
If you didn't watch it, don't add it.

## Rendering one to check it

```sh
docker run --rm --network none -v "$(pwd)/samples:/work" manim-worker \
    manim -ql /work/001_mathtex_side_by_side.py GeneratedScene -o out.mp4 --media_dir /work/media
open samples/media/videos/001_mathtex_side_by_side/480p15/out.mp4
```

(`media/` is gitignored — delete it after watching, or leave it; it never gets
committed.)

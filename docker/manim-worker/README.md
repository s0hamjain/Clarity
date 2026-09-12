# `manim-worker`

The sandbox every scene renders inside. Python 3.12 + Manim CE + a TeX distribution
(`dvisvgm` included, needed for `MathTex`/`Tex`) + ffmpeg. No network at run time
(`--network none`), capped memory/CPU per container.

## Build

```sh
docker build -t manim-worker -f docker/manim-worker/Dockerfile docker/manim-worker
```

Takes real minutes the first time (TeX packages are large). Rebuilds are cached unless
the Dockerfile changes.

## Smoke test

Do this before writing or trusting anything else in the render pipeline — it is the
single most likely thing to be broken in the whole project.

```sh
make smoke
```

From the repo root. Builds the image if it doesn't exist yet, renders
`docker/manim-worker/smoke_scene.py` (the same `MathTex` scene from SETUP.md §6.2)
through it, and prints `PASS` or `FAIL`. If `out.mp4` exists, the image is right. If it
dies on `latex` or `dvisvgm`, the Dockerfile is missing a TeX package — fix the image
and rebuild. There is no host-side fix; the host's own `manim`/`latex` install (if any)
is irrelevant.

To run the exact same check by hand instead:

```sh
mkdir -p /tmp/manim-smoke
cp docker/manim-worker/smoke_scene.py /tmp/manim-smoke/scene.py
docker run --rm --network none -v /tmp/manim-smoke:/work manim-worker \
    manim -ql /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media
ls /tmp/manim-smoke/media/videos/scene/*/out.mp4
```

## Flags

- `-ql` — 854×480 @ 15fps, dev quality.
- `-qm` — 1280×720 @ 30fps, release quality (`MANIM_QUALITY` env in `server/.env`).
- `--media_dir` — keeps every scratch file inside the mounted work directory so nothing
  leaks onto the container's own filesystem.
- `-o` — output filename; the render package moves the result out of Manim's own
  `media/videos/<script>/<quality>/` layout rather than searching for it.

These exact resolutions and frame rates are what `validate.go` checks every clip
against — confirmed empirically, not assumed from the manim docs.

## Render package regression test

```sh
make render-test
```

Renders every `samples/*.py` through a real container via the actual `Render()`
function (not just `docker run` by hand), plus the precheck, timeout-kill, clip
validation, `Concat`, and S3 upload tests. Tests that need Docker or a running MinIO
skip themselves (don't fail) when those aren't available — see `server/internal/render/`.

## S3 lifecycle note

The real S3 bucket (`RENDER_BUCKET`) needs a lifecycle rule expiring objects under
`renders/` after **14 days**. This is longer than the cache's 7-day TTL on purpose —
the cache must stop pointing at a video before S3 deletes it. MinIO, used locally, has
no such rule and doesn't need one.

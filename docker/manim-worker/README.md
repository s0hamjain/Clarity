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

If `out.mp4` exists, the image is right. If it dies on `latex` or `dvisvgm`, the
Dockerfile is missing a TeX package — fix the image and rebuild. There is no host-side
fix; the host's own `manim`/`latex` install (if any) is irrelevant.

Once the Go render package's `make smoke` target exists (Sprint 3), prefer that — it
builds the image if missing and runs this same scene automatically.

## Flags

- `-ql` — 480p @ 15fps, dev quality.
- `-qm` — 720p @ 30fps, release quality (`MANIM_QUALITY` env in `server/.env`).
- `--media_dir` — keeps every scratch file inside the mounted work directory so nothing
  leaks onto the container's own filesystem.
- `-o` — output filename; the render package moves the result out of Manim's own
  `media/videos/<script>/<quality>/` layout rather than searching for it.

## S3 lifecycle note

The real S3 bucket (`RENDER_BUCKET`) needs a lifecycle rule expiring objects under
`renders/` after **14 days**. This is longer than the cache's 7-day TTL on purpose —
the cache must stop pointing at a video before S3 deletes it. MinIO, used locally, has
no such rule and doesn't need one.

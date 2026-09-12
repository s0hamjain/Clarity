SHELL := /bin/bash
IMAGE := manim-worker

.PHONY: smoke render-test

# Builds manim-worker if it doesn't exist, runs the SETUP.md §6.2 scene
# through it, and prints PASS/FAIL. The single command anyone — not just P2 —
# runs to know the sandbox itself is not the thing that's broken.
smoke:
	@if ! docker image inspect $(IMAGE) >/dev/null 2>&1; then \
		echo "Building $(IMAGE)..."; \
		docker build -t $(IMAGE) -f docker/manim-worker/Dockerfile docker/manim-worker || exit 1; \
	fi
	@tmp=$$(mktemp -d); \
	cp docker/manim-worker/smoke_scene.py $$tmp/scene.py; \
	docker run --rm --network none -v $$tmp:/work $(IMAGE) \
		manim -ql /work/scene.py GeneratedScene -o out.mp4 --media_dir /work/media \
		> $$tmp/log.txt 2>&1; \
	if [ -f "$$tmp/media/videos/scene/480p15/out.mp4" ]; then \
		echo "PASS"; \
	else \
		echo "FAIL"; \
		cat $$tmp/log.txt; \
		rm -rf $$tmp; \
		exit 1; \
	fi; \
	rm -rf $$tmp

# The render package's regression suite — every samples/*.py through a real
# container, plus the precheck/timeout/validation/concat/S3 tests. Skips
# Docker- or MinIO-dependent cases gracefully when those aren't running.
render-test:
	go -C server test ./internal/render/

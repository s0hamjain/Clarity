package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dockerAvailable skips the test (not fails it) when Docker or the
// manim-worker image isn't present — this suite doubles as the render
// package's regression test and shouldn't block anyone without Docker running.
func dockerAvailable(t *testing.T) bool {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker is not running; skipping render tests")
		return false
	}
	if err := exec.Command("docker", "image", "inspect", "manim-worker").Run(); err != nil {
		t.Skip("manim-worker image not built; skipping render tests")
		return false
	}
	return true
}

// TestRenderSamples renders every samples/*.py through the real container —
// the library's regression test. Run before every sync point.
func TestRenderSamples(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	samplesDir := "../../../samples"
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		t.Fatalf("read samples dir: %v", err)
	}

	found := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".py" {
			continue
		}
		found++
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(samplesDir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}

			workDir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			clipPath, rerr := Render(ctx, string(src), workDir, "-ql")
			if rerr != nil {
				t.Fatalf("render failed: stage=%s traceback=%s", rerr.Stage, rerr.Traceback)
			}
			info, err := os.Stat(clipPath)
			if err != nil {
				t.Fatalf("clip missing at %s: %v", clipPath, err)
			}
			if info.Size() < minClipBytes {
				t.Fatalf("clip suspiciously small: %d bytes", info.Size())
			}
		})
	}
	if found == 0 {
		t.Fatal("no sample files found under samples/")
	}
}

// TestRenderBrokenScene confirms a scene that fails inside the container
// returns a real traceback, not a silent failure or a panic.
func TestRenderBrokenScene(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	src := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        self.play(ThisMobjectDoesNotExist())
`
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, rerr := Render(ctx, src, workDir, "-ql")
	if rerr == nil {
		t.Fatal("expected a render error for a broken scene, got success")
	}
	if rerr.Stage != "container" {
		t.Fatalf("expected stage %q, got %q", "container", rerr.Stage)
	}
	if !strings.Contains(rerr.Traceback, "ThisMobjectDoesNotExist") {
		t.Fatalf("expected the traceback to name the real error, got: %s", rerr.Traceback)
	}
}

// TestRenderRejectsSilentlyEmptyScene confirms Sprint 3's core promise: a
// scene whose play() calls all silently fail — here, a construct with no
// animation at all, just a brief wait — exits 0 and produces a real MP4 file,
// but validateClip must still reject it before it ever reaches Concat.
func TestRenderRejectsSilentlyEmptyScene(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	src := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        self.wait(0.1)
`
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, rerr := Render(ctx, src, workDir, "-ql")
	if rerr == nil {
		t.Fatal("expected a render error for a silently-empty scene, got success")
	}
	if rerr.Stage != "container" {
		t.Fatalf("expected stage %q, got %q", "container", rerr.Stage)
	}
	if !strings.Contains(rerr.Traceback, "clip invalid") {
		t.Fatalf("expected validateClip's rejection, got: %s", rerr.Traceback)
	}
}

// TestRenderAcceptsSparseButRealScene is the regression test for a real false
// positive: a correctly-rendered MathTex("x^2") comes out at 8,818 bytes —
// under the old 20 KB floor, which rejected it even though it's a completely
// valid render (confirmed against a real render: 854x480@15fps, 2s duration,
// 2 real animations played). Storyboard scenes are often one short formula,
// so this used to drop good scenes in production.
func TestRenderAcceptsSparseButRealScene(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	src := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        t = MathTex(r"x^2")
        self.play(Write(t))
        self.wait(1)
`
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clipPath, rerr := Render(ctx, src, workDir, "-ql")
	if rerr != nil {
		t.Fatalf("expected a sparse-but-real scene to be accepted, got: stage=%s traceback=%s", rerr.Stage, rerr.Traceback)
	}
	info, err := os.Stat(clipPath)
	if err != nil {
		t.Fatalf("clip missing at %s: %v", clipPath, err)
	}
	t.Logf("accepted a %d-byte clip", info.Size())
}

// TestRenderTimeoutKillsContainer proves the container is actually killed on
// timeout, not just the docker CLI process (FRD §23 rule 15). Passes a short
// deadline instead of waiting the real 120s — Render's internal timeout
// combines with whatever the caller's ctx already carries, so this exercises
// the identical code path.
func TestRenderTimeoutKillsContainer(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	src := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        while True:
            pass
`
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, rerr := Render(ctx, src, workDir, "-ql")
	if rerr == nil {
		t.Fatal("expected a timeout error, got success")
	}
	if rerr.Stage != "timeout" {
		t.Fatalf("expected stage %q, got %q: %s", "timeout", rerr.Stage, rerr.Traceback)
	}

	// Give the daemon a moment to finish tearing down the (--rm) container
	// after `docker kill`.
	time.Sleep(2 * time.Second)
	name := containerName(workDir)
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name="+name, "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("container %s still present after timeout: %q", name, out)
	}
}

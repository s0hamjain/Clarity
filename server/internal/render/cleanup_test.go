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

// TestCleanupAudit is Sprint 4's batch audit: ten renders spanning every
// failure mode the pipeline has to handle — precheck rejection, a container
// runtime error, a silently-empty clip, a timeout, and plain successes — and
// confirms none of them leaves a container running or scratch behind.
//
// A precheck rejection never reaches Render at all (FRD's architecture: the
// caller runs Precheck first), so it has nothing to clean up by construction;
// it's included here to document that explicitly, not because Render needs
// to guard against it.
func TestCleanupAudit(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}

	samplesDir := "../../../samples"
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		t.Fatalf("read samples dir: %v", err)
	}
	var sampleFiles []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".py" {
			sampleFiles = append(sampleFiles, filepath.Join(samplesDir, e.Name()))
		}
	}
	if len(sampleFiles) < 6 {
		t.Fatalf("need at least 6 samples for the audit, found %d", len(sampleFiles))
	}

	type job struct {
		name       string
		src        string
		precheck   bool // whether this job's source should pass Precheck
		wantRender bool // whether Render should report success
	}

	brokenScene := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        self.play(ThisMobjectDoesNotExist())
`
	emptyScene := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        self.wait(0.1)
`
	hangingScene := `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        while True:
            pass
`
	bannedScene := "import os\nos.system('ls')\n\n\nclass GeneratedScene(Scene):\n    def construct(self):\n        pass\n"

	jobs := []job{}
	for i, path := range sampleFiles[:6] {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		jobs = append(jobs, job{name: filepath.Base(path), src: string(src), precheck: true, wantRender: true})
		_ = i
	}
	jobs = append(jobs,
		job{name: "banned-pattern", src: bannedScene, precheck: false},
		job{name: "broken-scene", src: brokenScene, precheck: true, wantRender: false},
		job{name: "silently-empty", src: emptyScene, precheck: true, wantRender: false},
		job{name: "hanging-scene", src: hangingScene, precheck: true, wantRender: false},
	)

	if len(jobs) != 10 {
		t.Fatalf("expected a batch of 10, built %d", len(jobs))
	}

	var workDirs []string
	for _, j := range jobs {
		t.Run(j.name, func(t *testing.T) {
			if perr := Precheck(j.src); perr != nil {
				if j.precheck {
					t.Fatalf("expected precheck to pass, got: %v", perr)
				}
				return // rejected before Render is ever called — nothing to clean up
			}
			if !j.precheck {
				t.Fatal("expected precheck to reject this source, but it passed")
			}

			workDir := t.TempDir()
			workDirs = append(workDirs, workDir)

			timeout := 60 * time.Second
			if j.name == "hanging-scene" {
				timeout = 5 * time.Second // exercise the real timeout path quickly
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			_, rerr := Render(ctx, j.src, workDir, "-ql")
			gotSuccess := rerr == nil
			if gotSuccess != j.wantRender {
				t.Fatalf("job %s: expected success=%v, got success=%v (err=%v)", j.name, j.wantRender, gotSuccess, rerr)
			}

			// The scratch media dir must be gone immediately after Render
			// returns, on every exit path — not just eventually via t.TempDir().
			if _, err := os.Stat(filepath.Join(workDir, "media")); !os.IsNotExist(err) {
				t.Fatalf("job %s: media scratch dir was not cleaned up", j.name)
			}
		})
	}

	// Give the daemon a moment to finish removing any --rm containers.
	time.Sleep(2 * time.Second)

	out, err := exec.Command("docker", "ps", "-a", "--filter", "name=clarity-", "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("docker ps: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("containers still present after the batch: %q", out)
	}

	for _, wd := range workDirs {
		if _, err := os.Stat(filepath.Join(wd, "media")); !os.IsNotExist(err) {
			t.Fatalf("work dir %s still has scratch media after the full batch", wd)
		}
	}
}

// docker.go implements Render, the boundary FRD §14.1 defines between "here
// is some Manim Python" and "here is a finished clip or a RenderError".
//
// Callers must run Precheck on src before calling Render — per FRD's
// architecture, the /internal/render handler (P3) runs the static pre-check
// and holds the render semaphore around this call; Render itself trusts its
// caller and only turns source into a container run.
package render

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// containerTimeout is the hard per-scene cap (FRD §14.3). Render combines
// this with whatever deadline the caller's ctx already carries — context.
// WithTimeout always honors the earlier of the two, so a caller (a test, or
// the coordinator cancelling a job) can shorten this, never lengthen it.
const containerTimeout = 120 * time.Second

// qualityDirs maps the job-level MANIM_QUALITY flag to the directory manim
// writes its output under (`<media_dir>/videos/scene/<dir>/out.mp4`) — always
// "scene" because the script is always written as scene.py.
var qualityDirs = map[string]string{
	"-ql": "480p15",
	"-qm": "720p30",
}

// Render writes src into workDir, runs it inside a locked-down manim-worker
// container, and returns the finished clip's path — or a RenderError with
// enough detail (Stage, Traceback) for the agent's repair loop to act on.
// Never blocks longer than containerTimeout past the caller's own deadline.
func Render(ctx context.Context, src string, workDir string, quality string) (string, *RenderError) {
	if _, ok := qualityDirs[quality]; !ok {
		return "", &RenderError{Stage: "container", Traceback: fmt.Sprintf("invalid quality %q, want -ql or -qm", quality)}
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", &RenderError{Stage: "container", Traceback: "failed to create work dir: " + err.Error()}
	}
	scenePath := filepath.Join(workDir, "scene.py")
	if err := os.WriteFile(scenePath, []byte(src), 0o644); err != nil {
		return "", &RenderError{Stage: "container", Traceback: "failed to write scene.py: " + err.Error()}
	}

	// manim's own scratch (partial movies, TeX/svg cache) — never the final
	// artifact — cleaned up on every exit path, success or failure.
	mediaDir := filepath.Join(workDir, "media")
	defer os.RemoveAll(mediaDir)

	renderCtx, cancel := context.WithTimeout(ctx, containerTimeout)
	defer cancel()

	name := containerName(workDir)
	cmd := exec.CommandContext(renderCtx, "docker", "run", "--rm",
		"--name", name,
		"--network", "none",
		"--memory", "1g",
		"--cpus", "1",
		"-v", workDir+":/work",
		"manim-worker",
		"manim", quality, "/work/scene.py", "GeneratedScene",
		"-o", "out.mp4",
		"--media_dir", "/work/media",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if renderCtx.Err() != nil {
		// Killing the `docker` CLI process (what CommandContext just did) does
		// not stop the container — it has to be killed by name explicitly.
		killContainer(name)
		return "", &RenderError{
			Stage:     "timeout",
			Traceback: fmt.Sprintf("render did not finish within %s: %s", containerTimeout, renderCtx.Err()),
		}
	}

	if runErr != nil {
		return "", &RenderError{Stage: "container", Traceback: last40Lines(stderr.String())}
	}

	outPath, ferr := findOutput(mediaDir, quality)
	if ferr != nil {
		return "", &RenderError{Stage: "container", Traceback: ferr.Error()}
	}

	clipPath := filepath.Join(workDir, "scene.mp4")
	if err := os.Rename(outPath, clipPath); err != nil {
		return "", &RenderError{Stage: "container", Traceback: "failed to move rendered clip: " + err.Error()}
	}

	if verr := validateClip(clipPath, quality); verr != nil {
		return "", verr
	}

	return clipPath, nil
}

// containerName derives a stable, docker-safe name from workDir so a timed-
// out render can be killed by name — CommandContext only kills the CLI, not
// the container it started.
func containerName(workDir string) string {
	h := sha1.Sum([]byte(workDir))
	return fmt.Sprintf("clarity-%x", h[:8])
}

func killContainer(name string) {
	_ = exec.Command("docker", "kill", name).Run()
}

// findOutput locates manim's output without hunting: the script is always
// scene.py, so the output always lands at videos/scene/<qualityDir>/out.mp4.
func findOutput(mediaDir, quality string) (string, error) {
	dir := qualityDirs[quality]
	p := filepath.Join(mediaDir, "videos", "scene", dir, "out.mp4")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("expected rendered output at %s: %w", p, err)
	}
	return p, nil
}

// last40Lines trims a container's stderr to what the agent's repair prompt
// actually uses (FRD §10.4: "last 40 traceback lines").
func last40Lines(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(lines, "\n")
}

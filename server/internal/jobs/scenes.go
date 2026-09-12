package jobs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/s0hamjain/Clarity/server/internal/agent"
)

// RenderFunc renders the whole storyboard to one clip in one call. It returns
// ok:false when the render did not work out — there is no partial-success
// path any more (see FRD §10.4): one continuous script either renders or it
// doesn't.
type RenderFunc func(ctx context.Context, scenes []agent.Scene, workDir string) (clipPath string, ok bool)

// WorkRoot is where a job's render work directory is created. The agent
// renders into it and hands the path back, so both services must see the same
// filesystem — true today, since they run on the same Mac.
func WorkRoot(jobID string) string {
	return filepath.Join(os.TempDir(), "clarity", jobID)
}

// IDFromWorkDir recovers the job ID from a path under WorkRoot. The agent
// passes work_dir to /internal/render but not the job ID, and the coordinator
// needs the job to bind the render to its cancellation. Returns "" for a path
// that is not under the work root.
func IDFromWorkDir(workDir string) string {
	root := filepath.Clean(filepath.Join(os.TempDir(), "clarity"))
	rel, err := filepath.Rel(root, filepath.Clean(workDir))
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" || parts[0] == ".." || parts[0] == "." {
		return ""
	}
	return parts[0]
}

// renderStoryboard hands the whole storyboard to the agent as one call, with
// its own panic recovery so a panic in the render never takes down the
// pipeline goroutine that called this.
func (w *Worker) renderStoryboard(ctx context.Context, jobID string, scenes []agent.Scene, render RenderFunc) (clipPath string, ok bool) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("render panicked",
				"job_id", jobID, "request_id", agent.RequestIDFrom(ctx), "panic", p)
			clipPath, ok = "", false
		}
	}()

	workDir := WorkRoot(jobID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		slog.Error("create job work dir",
			"job_id", jobID, "request_id", agent.RequestIDFrom(ctx), "error", err)
		return "", false
	}

	return render(ctx, scenes, workDir)
}

// cleanupWorkDir removes a job's work directory. The clip only needs to
// survive until the upload has read it.
func cleanupWorkDir(jobID string) {
	if err := os.RemoveAll(WorkRoot(jobID)); err != nil {
		slog.Warn("remove job work dir", "job_id", jobID, "error", err)
	}
}

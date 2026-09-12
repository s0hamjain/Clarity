package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/s0hamjain/Clarity/server/internal/agent"
)

// SceneFunc renders one scene to one clip. It returns ok:false for a scene that
// could not be rendered — that drops the scene, never the job (FRD §23 rule 16).
type SceneFunc func(ctx context.Context, scene agent.Scene, workDir string) (clipPath string, ok bool)

// WorkRoot is where per-job scene directories are created. The agent renders
// into these and hands the paths back, so both services must see the same
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

// fanOut renders every scene concurrently and returns the clips of the ones
// that worked, in scene order.
//
// There is deliberately no semaphore here. Concurrency is bounded inside
// /internal/render, around `docker run` — the expensive part — so that the
// agent's codegen calls, which are just waiting on a model, are not serialized
// behind renders.
//
// onSceneDone is called once per scene, finished or dropped, with the running
// total. It is how scenes_done advances in the job record.
func (w *Worker) fanOut(
	ctx context.Context,
	jobID string,
	scenes []agent.Scene,
	render SceneFunc,
	onSceneDone func(done int),
) []string {
	type result struct {
		clipPath string
		ok       bool
	}
	results := make([]result, len(scenes))

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		done int
	)

	for i, scene := range scenes {
		wg.Add(1)
		go func(i int, scene agent.Scene) {
			defer wg.Done()

			// Each scene goroutine needs its own recover: a deferred recover in
			// the parent does not catch a panic in a child. A panic here fails
			// this scene only.
			defer func() {
				if p := recover(); p != nil {
					slog.Error("scene panicked", "job_id", jobID, "scene", i, "panic", p)
				}
				// The write stays inside the lock. Incrementing under the lock
				// and then writing outside it lets a goroutine holding 3 reach
				// the job record before the one holding 2, and scenes_done
				// finishes at 2 of 3 — a progress counter stuck one short.
				mu.Lock()
				defer mu.Unlock()
				done++
				onSceneDone(done)
			}()

			workDir := filepath.Join(WorkRoot(jobID), fmt.Sprintf("scene%d", scene.Index))
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				slog.Error("create scene work dir", "job_id", jobID, "scene", i, "error", err)
				return
			}

			clipPath, ok := render(ctx, scene, workDir)
			results[i] = result{clipPath: clipPath, ok: ok}
		}(i, scene)
	}

	wg.Wait()

	// Scene order is the storyboard's order, and concat depends on it.
	clips := make([]string, 0, len(scenes))
	for _, r := range results {
		if r.ok && r.clipPath != "" {
			clips = append(clips, r.clipPath)
		}
	}
	return clips
}

// cleanupWorkDir removes a job's scene directories. Clips only need to survive
// until concat has read them.
func cleanupWorkDir(jobID string) {
	if err := os.RemoveAll(WorkRoot(jobID)); err != nil {
		slog.Warn("remove job work dir", "job_id", jobID, "error", err)
	}
}

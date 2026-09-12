package jobs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/s0hamjain/Clarity/server/internal/agent"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

// AgentRenderFunc renders the whole storyboard by handing it to the Manim
// Generator agent in one call.
//
// The agent owns the whole loop — retrieve, generate, lint, render, repair,
// up to three attempts — over one continuous script covering every beat, and
// calls back into the coordinator's /internal/render to do the rendering. One
// HTTP call per job, with the 11-minute budget from API.md §7 taken from the
// job's context, so cancelling the job aborts the call.
func AgentRenderFunc(
	client *agent.Client,
	cfg *config.Config,
	jobID string,
	storyboardTitle, category string,
	guardrails bool,
) RenderFunc {
	return func(ctx context.Context, scenes []agent.Scene, workDir string) (string, bool) {
		// job_id is the agent's thread_id, so its graph logs and these join on
		// the same key.
		log := slog.With("job_id", jobID, "request_id", agent.RequestIDFrom(ctx))

		resp, err := client.Render(ctx, agent.RenderRequest{
			JobID:           jobID,
			Scenes:          scenes,
			StoryboardTitle: storyboardTitle,
			Category:        category,
			Guardrails:      guardrails,
			WorkDir:         workDir,
			Quality:         cfg.ManimQuality,
		})
		if err != nil {
			log.Error("render call failed", "error", err)
			return "", false
		}
		if !resp.OK {
			log.Warn("agent gave up on the render",
				"attempts", resp.Attempts, "lint_retries", resp.LintRetries,
				"stage", resp.Stage, "traceback", firstLine(resp.LastTraceback))
			return "", false
		}

		clipPath, ok := validClip(resp.ClipPath, workDir)
		if !ok {
			log.Error("agent reported success with an unusable clip_path", "clip_path", resp.ClipPath)
			return "", false
		}

		log.Info("render finished",
			"attempts", resp.Attempts, "lint_retries", resp.LintRetries,
			"snippets_used", len(resp.SnippetsUsed), "snippet_id", resp.SnippetID)
		return clipPath, true
	}
}

// validClip checks the path the agent handed back before anything reads it.
// The clip must sit inside the job's own work dir and actually exist: the
// path is fed to the uploader, so it gets the same containment treatment
// work_dir gets in /internal/render, and a success pointing at a missing file
// has to be caught here rather than by a confusing upload failure later.
func validClip(clipPath, workDir string) (string, bool) {
	if clipPath == "" {
		return "", false
	}
	clean := filepath.Clean(clipPath)
	if !filepath.IsAbs(clean) {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(workDir), clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(clean)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return "", false
	}
	return clean, true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

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

// AgentSceneFunc renders one scene by handing it to the Manim Generator agent.
//
// The agent owns the whole loop for that scene — retrieve, generate, lint,
// render, repair, up to three attempts — and calls back into the coordinator's
// /internal/render to do the rendering. One HTTP call per scene, with the
// 11-minute budget from API.md §7 taken from the job's context, so cancelling
// the job aborts the call.
//
// A scene that does not work out is dropped, never the job (FRD §23 rule 16),
// so every failure path here returns ok:false rather than an error.
func AgentSceneFunc(
	client *agent.Client,
	cfg *config.Config,
	jobID string,
	storyboardTitle, category string,
	guardrails bool,
) SceneFunc {
	return func(ctx context.Context, scene agent.Scene, workDir string) (string, bool) {
		// job_id + scene is the agent's thread_id, so its graph logs and these
		// join on the same key.
		log := slog.With("job_id", jobID, "scene", scene.Index,
			"request_id", agent.RequestIDFrom(ctx))

		resp, err := client.ScenesRender(ctx, agent.SceneRenderRequest{
			JobID:           jobID,
			Scene:           scene,
			StoryboardTitle: storyboardTitle,
			Category:        category,
			Guardrails:      guardrails,
			WorkDir:         workDir,
			// One job-level quality value for every scene (rule 12); concat
			// with -c copy depends on every clip matching.
			Quality: cfg.ManimQuality,
		})
		if err != nil {
			log.Error("scene render call failed", "error", err)
			return "", false
		}
		if !resp.OK {
			log.Warn("agent gave up on this scene",
				"attempts", resp.Attempts, "lint_retries", resp.LintRetries,
				"stage", resp.Stage, "traceback", firstLine(resp.LastTraceback))
			return "", false
		}

		clipPath, ok := validClip(resp.ClipPath, workDir)
		if !ok {
			log.Error("agent reported success with an unusable clip_path", "clip_path", resp.ClipPath)
			return "", false
		}

		log.Info("scene rendered",
			"attempts", resp.Attempts, "lint_retries", resp.LintRetries,
			"snippets_used", len(resp.SnippetsUsed), "snippet_id", resp.SnippetID)
		return clipPath, true
	}
}

// validClip checks the path the agent handed back before anything reads it.
// The clip must sit inside the scene's own work dir and actually exist: the
// path is fed to ffmpeg, so it gets the same containment treatment work_dir
// gets in /internal/render, and a success pointing at a missing file has to be
// caught here rather than by a confusing concat failure later.
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

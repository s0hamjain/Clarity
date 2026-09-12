package api

import (
	"context"
	"path/filepath"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
)

// FakeRender stands in for P2's render.Render until it lands. It writes a real
// clip to workDir so everything downstream — concat, upload, the desktop
// player — exercises actual media rather than a placeholder. The pre-check has
// already run by the time this is called, exactly as it will for the real
// thing. Deleted in Sprint 4 along with FAKE_RENDER.
func FakeRender(ctx context.Context, src, workDir, quality string) (string, *render.RenderError) {
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return "", &render.RenderError{Stage: "timeout", Traceback: "render cancelled"}
	}

	clipPath := filepath.Join(workDir, filepath.Base(workDir)+".mp4")
	if err := jobs.WriteFakeClip(ctx, clipPath); err != nil {
		return "", &render.RenderError{
			Stage:     "container",
			Traceback: "fake render could not produce a clip: " + err.Error(),
		}
	}
	return clipPath, nil
}

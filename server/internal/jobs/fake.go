package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
)

// Canned answers standing in for P1's two model calls while FAKE_AGENT is on.
// They are the same shapes the real endpoints return, so the pipeline around
// them is the real pipeline. Deleted in Sprint 4 with the flags.

// fakeVision stands in for POST /vision. The text is hashed like the real
// thing, so two fake jobs with the same user_prompt and guardrails flag share a
// cache entry — which is what makes the cache path testable before P1 is wired.
func fakeVision() *agent.VisionResponse {
	return &agent.VisionResponse{
		ProblemText: fakeProblemText,
		Category:    agent.CategoryAlgorithm,
		Confidence:  0.93,
	}
}

// fakeExplain stands in for POST /explain. Three scenes, because the fan-out
// and the desktop app's progress bar are more interesting with more than one.
func fakeExplain() *agent.ExplainResponse {
	return &agent.ExplainResponse{
		Explanation: fakeExplanation,
		Storyboard: agent.Storyboard{
			Title: "Where the Binary Search Goes Wrong",
			Scenes: []agent.Scene{
				{Index: 0, Narration: "lo and hi start at the ends.",
					Visual: "A row of 8 boxes; pointers lo and hi under the first and last.", DurationSeconds: 8},
				{Index: 1, Narration: "mid rounds down — and lo never moves past it.",
					Visual: "mid pointer appears; lo jumps to mid instead of mid+1; highlight the box checked twice.", DurationSeconds: 10},
				{Index: 2, Narration: "One character fixes it.",
					Visual: "The assignment changes to lo = mid + 1; the window shrinks to empty.", DurationSeconds: 7},
			},
		},
		Revisions: 0,
	}
}

const fakeProblemText = `def binary_search(arr, target):
    lo, hi = 0, len(arr)
    while lo < hi:
        mid = (lo + hi) // 2
        if arr[mid] == target:
            return mid
        elif arr[mid] < target:
            lo = mid
        else:
            hi = mid
    return -1`

// Markdown, because the result box renders it through marked.js — this is what
// P4 styles against.
const fakeExplanation = `**Step 1 — What the loop is supposed to do.**
Each pass should shrink the window ` + "`[lo, hi)`" + ` strictly. As long as the
window gets smaller every time, the loop has to end.

**Step 2 — Where it stops shrinking.**
` + "`mid = (lo + hi) // 2`" + ` rounds *down*. When ` + "`hi - lo == 1`" + `,
that makes ` + "`mid == lo`" + `. The branch ` + "`lo = mid`" + ` then assigns
` + "`lo`" + ` to itself and the window never changes.

**Step 3 — The fix.**
Move past the midpoint you already checked:

` + "```python" + `
lo = mid + 1     # instead of lo = mid
` + "```" + `

**Step 4 — Why the ` + "`hi`" + ` side is already fine.**
` + "`hi = mid`" + ` is correct with a half-open window, because ` + "`arr[mid]`" + `
has been ruled out and ` + "`hi`" + ` is exclusive.

> _This is sample data from the coordinator's fake pipeline —
> ` + "`FAKE_AGENT=1`" + `. The real explanation arrives when P1's Explainer graph lands._`

// FakeConcat stands in for P2's render.Concat until it lands. It checks the
// clips are really there — the one thing the real Concat would also do first —
// and then reports the sample URL instead of stitching and uploading.
func FakeConcat(fakeVideoURL string) ConcatFunc {
	return func(ctx context.Context, clipPaths []string, outKey string) (string, error) {
		for _, p := range clipPaths {
			if _, err := os.Stat(p); err != nil {
				return "", fmt.Errorf("clip is missing: %w", err)
			}
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		slog.Info("fake concat", "clips", len(clipPaths), "out_key", outKey)
		return fakeVideoURL, nil
	}
}

// FakeSceneFunc stands in for one POST /scenes/render call while FAKE_AGENT is
// on: it takes a beat, then drops a clip in the scene's work dir.
func FakeSceneFunc(ctx context.Context, scene agent.Scene, workDir string) (string, bool) {
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return "", false
	}

	clipPath := filepath.Join(workDir, fmt.Sprintf("scene%d.mp4", scene.Index))
	if err := WriteFakeClip(ctx, clipPath); err != nil {
		slog.Error("fake scene render", "scene", scene.Index, "error", err)
		return "", false
	}
	return clipPath, true
}

// WriteFakeClip produces a two-second black MP4 with ffmpeg, which SETUP §1
// installs anyway, so concat and the desktop player get real media to work
// with. Without ffmpeg it writes a minimal MP4 container instead: enough for a
// file to exist and be moved around, but it will not play and ffprobe will not
// like it — which is honest, because there is nothing real to render yet.
//
// Shared with /internal/render's fake. Deleted in Sprint 4 with the flags.
func WriteFakeClip(ctx context.Context, clipPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "color=c=black:s=480x270:d=2:r=30",
			"-pix_fmt", "yuv420p", clipPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ffmpeg: %v: %s", err, out)
		}
		return nil
	}
	return os.WriteFile(clipPath, minimalMP4, 0o644)
}

// minimalMP4 is an ftyp box plus an empty moov box — structurally a valid MP4
// file that no player will show anything for. Only used when ffmpeg is absent.
var minimalMP4 = []byte{
	0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm',
	0x00, 0x00, 0x02, 0x00, 'i', 's', 'o', 'm', 'm', 'p', '4', '2',
	0x00, 0x00, 0x00, 0x08, 'm', 'o', 'o', 'v',
}

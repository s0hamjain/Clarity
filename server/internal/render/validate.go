package render

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// minClipBytes and minClipSeconds are the Sprint 2 floor: a clip that exists
// but is empty of real content — every play() silently failing is the usual
// cause — must not be reported as a success. Sprint 3 adds resolution/fps
// checks against the quality flag; this is deliberately the cheap half.
const (
	minClipBytes   = 20 * 1024
	minClipSeconds = 1.0
)

// validateClip rejects a clip that rendered (exit 0) but isn't real content.
func validateClip(path string) *RenderError {
	info, err := os.Stat(path)
	if err != nil {
		return &RenderError{Stage: "container", Traceback: "rendered clip missing: " + err.Error()}
	}
	if info.Size() < minClipBytes {
		return &RenderError{
			Stage:     "container",
			Traceback: fmt.Sprintf("clip invalid: only %d bytes — every play() likely failed silently", info.Size()),
		}
	}

	dur, err := clipDuration(path)
	if err != nil {
		return &RenderError{Stage: "container", Traceback: "ffprobe failed: " + err.Error()}
	}
	if dur < minClipSeconds {
		return &RenderError{
			Stage:     "container",
			Traceback: fmt.Sprintf("clip invalid: duration %.2fs — every play() likely failed silently", dur),
		}
	}
	return nil
}

func clipDuration(path string) (float64, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

package render

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// minClipBytes is the floor below which a clip is almost certainly empty of
// real content, whatever its reported duration or resolution say.
const minClipBytes = 20 * 1024

// qualitySpec is what a quality flag's output must measure as. Confirmed
// against real manim-worker renders, not assumed from the docs.
type qualitySpec struct {
	Width, Height, FPS int
}

var qualitySpecs = map[string]qualitySpec{
	"-ql": {854, 480, 15},
	"-qm": {1280, 720, 30},
}

type ffprobeStream struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
	Duration   string `json:"duration"`
}

type ffprobeStreamsOutput struct {
	Streams []ffprobeStream `json:"streams"`
}

// validateClip rejects a clip that exited 0 but isn't real content: too
// small, too short, or the wrong resolution/fps for its quality flag — the
// signature of every play() silently failing.
func validateClip(path string, quality string) *RenderError {
	info, err := os.Stat(path)
	if err != nil {
		return &RenderError{Stage: "container", Traceback: "clip invalid: rendered clip missing: " + err.Error()}
	}
	if info.Size() < minClipBytes {
		return &RenderError{
			Stage:     "container",
			Traceback: fmt.Sprintf("clip invalid: only %d bytes — every play() likely failed silently", info.Size()),
		}
	}

	spec, ok := qualitySpecs[quality]
	if !ok {
		return &RenderError{Stage: "container", Traceback: fmt.Sprintf("clip invalid: unknown quality %q", quality)}
	}

	stream, err := probeStream(path)
	if err != nil {
		return &RenderError{Stage: "container", Traceback: "clip invalid: ffprobe failed: " + err.Error()}
	}

	dur, durErr := parseDuration(stream.Duration)
	if durErr != nil {
		// Some containers don't tag stream-level duration; fall back to the
		// format-level one rather than fail a real clip on a metadata gap.
		dur, durErr = clipDuration(path)
	}
	if durErr != nil {
		return &RenderError{Stage: "container", Traceback: "clip invalid: could not read duration: " + durErr.Error()}
	}
	if dur < 1.0 {
		return &RenderError{
			Stage:     "container",
			Traceback: fmt.Sprintf("clip invalid: duration %.2fs — every play() likely failed silently", dur),
		}
	}

	if stream.Width != spec.Width || stream.Height != spec.Height {
		return &RenderError{
			Stage: "container",
			Traceback: fmt.Sprintf(
				"clip invalid: %dx%d, expected %dx%d for %s",
				stream.Width, stream.Height, spec.Width, spec.Height, quality,
			),
		}
	}

	fps, err := parseFrameRate(stream.RFrameRate)
	if err != nil {
		return &RenderError{Stage: "container", Traceback: "clip invalid: could not read frame rate: " + err.Error()}
	}
	if math.Round(fps) != float64(spec.FPS) {
		return &RenderError{
			Stage:     "container",
			Traceback: fmt.Sprintf("clip invalid: %.2f fps, expected %d fps for %s", fps, spec.FPS, quality),
		}
	}

	return nil
}

func probeStream(path string) (ffprobeStream, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,duration",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return ffprobeStream{}, err
	}
	var parsed ffprobeStreamsOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return ffprobeStream{}, err
	}
	if len(parsed.Streams) == 0 {
		return ffprobeStream{}, fmt.Errorf("no video stream found")
	}
	return parsed.Streams[0], nil
}

func parseDuration(s string) (float64, error) {
	if s == "" || s == "N/A" {
		return 0, fmt.Errorf("no duration reported")
	}
	return strconv.ParseFloat(s, 64)
}

// parseFrameRate turns ffprobe's "15/1" (or a non-integer ratio like
// "30000/1001") into a float.
func parseFrameRate(s string) (float64, error) {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return strconv.ParseFloat(s, 64)
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d == 0 {
		return 0, fmt.Errorf("invalid frame rate denominator in %q", s)
	}
	return n / d, nil
}

// clipDuration is the format-level fallback when the stream doesn't carry
// its own duration tag.
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

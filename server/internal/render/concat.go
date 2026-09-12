package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// concatProbe is the subset of a clip's stream metadata that -c copy needs
// identical across every input. Comparable as a plain struct so two probes
// can be checked with !=.
type concatProbe struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
	CodecName  string `json:"codec_name"`
}

// Concat joins finished clips in order with no re-encode, uploads the result
// to S3, and returns its public URL (FRD §14.1, §14.4). Every input's
// resolution/fps/codec is checked first — the job-level MANIM_QUALITY
// constant is supposed to guarantee they match, but -c copy on mismatched
// inputs produces broken output at exit code 0, so this fails loudly instead
// of trusting that guarantee.
func Concat(ctx context.Context, clipPaths []string, outKey string) (string, error) {
	if len(clipPaths) == 0 {
		return "", fmt.Errorf("concat: no clips to join")
	}

	first, err := probeForConcat(clipPaths[0])
	if err != nil {
		return "", fmt.Errorf("concat: probing %s: %w", clipPaths[0], err)
	}
	for _, p := range clipPaths[1:] {
		s, err := probeForConcat(p)
		if err != nil {
			return "", fmt.Errorf("concat: probing %s: %w", p, err)
		}
		if s != first {
			return "", fmt.Errorf(
				"concat: %s (%+v) does not match %s (%+v) — every scene must share MANIM_QUALITY",
				p, s, clipPaths[0], first,
			)
		}
	}

	workDir, err := os.MkdirTemp("", "clarity-concat-")
	if err != nil {
		return "", fmt.Errorf("concat: temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	listPath := filepath.Join(workDir, "concat_list.txt")
	var list strings.Builder
	for _, p := range clipPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", fmt.Errorf("concat: resolving %s: %w", p, err)
		}
		// ffmpeg's concat demuxer needs single quotes escaped as '\''.
		list.WriteString("file '" + strings.ReplaceAll(abs, "'", `'\''`) + "'\n")
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return "", fmt.Errorf("concat: writing list: %w", err)
	}

	finalPath := filepath.Join(workDir, "final.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "concat", "-safe", "0", "-i", listPath,
		"-c", "copy", finalPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("concat: ffmpeg failed: %s", last40Lines(stderr.String()))
	}

	url, err := uploadToS3(ctx, finalPath, outKey)
	if err != nil {
		return "", fmt.Errorf("concat: %w", err)
	}
	return url, nil
}

func probeForConcat(path string) (concatProbe, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,codec_name",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return concatProbe{}, err
	}
	var parsed struct {
		Streams []concatProbe `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return concatProbe{}, err
	}
	if len(parsed.Streams) == 0 {
		return concatProbe{}, fmt.Errorf("no video stream found")
	}
	return parsed.Streams[0], nil
}

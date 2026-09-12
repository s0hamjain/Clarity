package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/s0hamjain/Clarity/server/internal/config"
	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
)

// stubPipeline records what the handler handed off, without doing any work.
type stubPipeline struct {
	mu          sync.Mutex
	started     []string
	cancelled   []string
	requestIDs  []string
	contexts    map[string]context.Context
	rejectStart error
}

func (p *stubPipeline) Start(j *jobs.Job, imageB64, mediaType, requestID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rejectStart != nil {
		return p.rejectStart
	}
	p.started = append(p.started, j.ID)
	p.requestIDs = append(p.requestIDs, requestID)
	return nil
}

// JobContext reports no running job by default, which is the state
// /internal/render must handle anyway: the agent can call it for a job that
// has already finished or been cancelled.
func (p *stubPipeline) JobContext(id string) (context.Context, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, ok := p.contexts[id]
	return ctx, ok
}

func (p *stubPipeline) Cancel(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelled = append(p.cancelled, id)
	return nil
}

func newTestServer(t *testing.T) (http.Handler, jobs.Store, jobs.CacheStore, *stubPipeline) {
	srv, jobStore, cacheStore, pipe := newTestServerFull(t)
	return srv.Routes(), jobStore, cacheStore, pipe
}

// newTestServerFull also hands back the *Server, for tests that need to reach
// past the router — the render function and the semaphore wait.
func newTestServerFull(t *testing.T) (*Server, jobs.Store, jobs.CacheStore, *stubPipeline) {
	t.Helper()
	t.Setenv("MONGODB_URI", "mongodb://test.invalid/clarity") // never dialled; the stores are injected

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	jobStore, cacheStore := newMemJobs(), newMemCache()
	pipe := &stubPipeline{}
	health := NewHealth(cfg, nil)
	// The renderer is a stand-in, so report Docker up: these tests are about
	// the handler, not about whether this machine has a daemon running.
	health.set(HealthReport{Docker: true})
	srv := NewServer(cfg, jobStore, cacheStore, pipe, health, stubRender)
	return srv, jobStore, cacheStore, pipe
}

func pngDataURL(nBytes int) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, nBytes))
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the error envelope: %v (body: %s)", err, w.Body.String())
	}
	if env.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
	return env
}

func TestCreateJobAccepts(t *testing.T) {
	h, jobStore, _, pipe := newTestServer(t)

	w := do(t, h, "POST", "/api/jobs", `{"image":"`+pngDataURL(64)+`","user_prompt":"why?","guardrails":true}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body: %s)", w.Code, w.Body.String())
	}

	var got struct {
		JobID   string `json:"job_id"`
		PollURL string `json:"poll_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.JobID, "j_") || len(got.JobID) != 10 {
		t.Errorf("job_id = %q, want j_ + 8 hex chars", got.JobID)
	}
	if got.PollURL != "/api/jobs/"+got.JobID {
		t.Errorf("poll_url = %q, want /api/jobs/%s", got.PollURL, got.JobID)
	}

	// The job must exist before the response is written, and the pipeline must
	// have been handed it (FRD §23 rule 7).
	j, err := jobStore.Get(context.Background(), got.JobID)
	if err != nil {
		t.Fatalf("job was not persisted: %v", err)
	}
	if j.Status != jobs.StatusQueued {
		t.Errorf("status = %q, want queued", j.Status)
	}
	if !j.Guardrails {
		t.Error("guardrails was not carried onto the job")
	}
	if j.Source != "desktop" {
		t.Errorf("source = %q, want the default \"desktop\"", j.Source)
	}
	if len(pipe.started) != 1 || pipe.started[0] != got.JobID {
		t.Errorf("pipeline.Start not called with the job: %v", pipe.started)
	}
}

func TestCreateJobRejects(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"malformed json", `{nope`, http.StatusBadRequest, CodeBadRequest},
		{"missing image", `{}`, http.StatusBadRequest, CodeBadRequest},
		{"not a data url", `{"image":"problem.png"}`, http.StatusBadRequest, CodeBadRequest},
		{"no comma", `{"image":"data:image/png;base64"}`, http.StatusBadRequest, CodeBadRequest},
		{"not base64-flagged", `{"image":"data:image/png,abc"}`, http.StatusBadRequest, CodeBadRequest},
		{"bad base64", `{"image":"data:image/png;base64,!!!!"}`, http.StatusBadRequest, CodeBadRequest},
		{"gif", `{"image":"data:image/gif;base64,AAAA"}`, http.StatusUnsupportedMediaType, CodeUnsupportedImageType},
		{"9 MB png", `{"image":"` + pngDataURL(9<<20) + `"}`, http.StatusRequestEntityTooLarge, CodeImageTooLarge},
		{"long prompt", `{"image":"` + pngDataURL(8) + `","user_prompt":"` + strings.Repeat("x", 2001) + `"}`, http.StatusBadRequest, CodeBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _, _ := newTestServer(t)
			w := do(t, h, "POST", "/api/jobs", tc.body)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.wantCode, w.Body.String())
			}
			if got := decodeError(t, w).Error.Code; got != tc.wantErr {
				t.Errorf("code = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

// An 8 MB image is exactly at the limit and must be accepted.
func TestCreateJobAcceptsImageAtTheLimit(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	w := do(t, h, "POST", "/api/jobs", `{"image":"`+pngDataURL(8<<20)+`"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body: %s)", w.Code, w.Body.String())
	}
}

// Every field the desktop app reads must be present, null rather than omitted
// when unknown (API.md §1, §2.2).
func TestGetJobShape(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	w := do(t, h, "POST", "/api/jobs", `{"image":"`+pngDataURL(8)+`"}`)
	var created struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = do(t, h, "GET", "/api/jobs/"+created.JobID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{
		"job_id", "status", "problem_hash", "problem_text", "category", "explanation",
		"scenes_total", "scenes_done", "video_url", "cached", "guardrails", "error",
		"created_at", "updated_at",
	} {
		if _, ok := body[k]; !ok {
			t.Errorf("field %q is missing; unknown fields must be null, never omitted", k)
		}
	}
	for _, k := range []string{"problem_hash", "explanation", "video_url", "error"} {
		if string(body[k]) != "null" {
			t.Errorf("field %q = %s on a queued job, want null", k, body[k])
		}
	}
	// The screenshot and the user's prompt are never returned (FRD §20).
	for _, k := range []string{"image", "user_prompt"} {
		if _, ok := body[k]; ok {
			t.Errorf("field %q must not be in the response", k)
		}
	}
}

func TestJobNotFound(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	for _, method := range []string{"GET", "DELETE"} {
		w := do(t, h, method, "/api/jobs/j_deadbeef", "")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", method, w.Code)
		}
		if got := decodeError(t, w).Error.Code; got != CodeJobNotFound {
			t.Errorf("%s code = %q, want %q", method, got, CodeJobNotFound)
		}
	}
}

func TestCacheEndpoint(t *testing.T) {
	h, _, cacheStore, _ := newTestServer(t)

	w := do(t, h, "GET", "/api/cache/0000000000000000", "")
	if w.Code != http.StatusNotFound || decodeError(t, w).Error.Code != CodeCacheMiss {
		t.Fatalf("miss: status = %d, body = %s", w.Code, w.Body.String())
	}

	if err := cacheStore.Put(context.Background(), "a3f9c1d2e4b57680", "http://minio/x.mp4", "**Step 1.**"); err != nil {
		t.Fatalf("cache put: %v", err)
	}
	w = do(t, h, "GET", "/api/cache/a3f9c1d2e4b57680", "")
	if w.Code != http.StatusOK {
		t.Fatalf("hit: status = %d", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	// A hit returns both fields — a video with nothing to read defeats the point.
	for _, k := range []string{"problem_hash", "video_url", "explanation", "created_at"} {
		if got[k] == nil || got[k] == "" {
			t.Errorf("cache hit is missing %q: %v", k, got)
		}
	}
}

// Every response carries CORS and a request ID, including 404 and 5xx.
func TestCORSAndRequestIDOnEveryResponse(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/healthz"},
		{"GET", "/api/jobs/j_deadbeef"},
		{"GET", "/nope"},
		{"POST", "/api/jobs"},
	} {
		w := do(t, h, tc.method, tc.path, "")
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s %s: Access-Control-Allow-Origin = %q, want *", tc.method, tc.path, got)
		}
		if w.Header().Get("X-Request-Id") == "" {
			t.Errorf("%s %s: no X-Request-Id", tc.method, tc.path)
		}
	}
}

func TestRequestIDIsEchoed(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	r := httptest.NewRequest("GET", "/healthz", nil)
	r.Header.Set("X-Request-Id", "req_from_the_client")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("X-Request-Id"); got != "req_from_the_client" {
		t.Errorf("X-Request-Id = %q, want it echoed", got)
	}
}

// An unknown route is the error envelope, never net/http's bare "404 page not found".
func TestUnknownRouteUsesTheEnvelope(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	w := do(t, h, "GET", "/nope", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "page not found") {
		t.Fatalf("bare net/http 404 leaked: %s", w.Body.String())
	}
	decodeError(t, w)
}

// A method that no route claims is still the envelope, not a bare 405.
func TestWrongMethodUsesTheEnvelope(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	w := do(t, h, "PUT", "/api/jobs", "")
	if w.Code == http.StatusOK {
		t.Fatal("PUT /api/jobs should not succeed")
	}
	if strings.Contains(w.Body.String(), "Method Not Allowed") {
		t.Fatalf("bare net/http 405 leaked: %s", w.Body.String())
	}
}

func TestHealthzShape(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	w := do(t, h, "GET", "/healthz", "")
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"ok", "atlas", "docker", "s3", "agent", "version", "prompt_version"} {
		if _, present := body[k]; !present {
			t.Errorf("/healthz is missing %q", k)
		}
	}
	// Checks have not run in this test, so nothing is healthy and the status
	// must be 503 rather than a cheerful 200.
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when ok is false", w.Code)
	}
}

// stubRender stands in for P2's render.Render so the API tests need no Docker.
// It writes a small file where a clip would be, because /internal/render's
// caller is entitled to find something at clip_path.
func stubRender(ctx context.Context, src, workDir, quality string) (string, *render.RenderError) {
	if ctx.Err() != nil {
		return "", &render.RenderError{Stage: "timeout", Traceback: "cancelled"}
	}
	clipPath := filepath.Join(workDir, "scene.mp4")
	if err := os.WriteFile(clipPath, []byte("not really an mp4"), 0o644); err != nil {
		return "", &render.RenderError{Stage: "container", Traceback: err.Error()}
	}
	return clipPath, nil
}

// Over the bound, new captures are refused rather than queued forever, and the
// desktop app is told when to come back (API.md §7).
func TestCreateJobQueueFull(t *testing.T) {
	h, jobStore, _, pipe := newTestServer(t)
	pipe.rejectStart = jobs.ErrQueueFull

	w := do(t, h, "POST", "/api/jobs", `{"image":"`+pngDataURL(8)+`"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := decodeError(t, w).Error.Code; got != CodeQueueFull {
		t.Errorf("code = %q, want %q", got, CodeQueueFull)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("503 queue_full without Retry-After leaves the client guessing")
	}

	// The job row was already written, so it has to be closed out rather than
	// left sitting in `queued` for a pipeline that will never run.
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	jobStore.(*memJobs).mu.RLock()
	defer jobStore.(*memJobs).mu.RUnlock()
	for _, j := range jobStore.(*memJobs).m {
		if j.Status != jobs.StatusFailed {
			t.Errorf("rejected job left in %q, want failed", j.Status)
		}
		if j.Error == nil || *j.Error != CodeQueueFull {
			t.Errorf("rejected job error = %v, want %q", j.Error, CodeQueueFull)
		}
	}
}

// The request ID reaches the pipeline, so a capture can be followed from the
// HTTP log line all the way through the agent's graph logs.
func TestRequestIDReachesThePipeline(t *testing.T) {
	h, _, _, pipe := newTestServer(t)

	r := httptest.NewRequest("POST", "/api/jobs", strings.NewReader(`{"image":"`+pngDataURL(8)+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-Id", "req_traceme")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if len(pipe.requestIDs) != 1 || pipe.requestIDs[0] != "req_traceme" {
		t.Errorf("pipeline got request IDs %v, want [req_traceme]", pipe.requestIDs)
	}
}

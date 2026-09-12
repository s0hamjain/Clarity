package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
	"github.com/s0hamjain/Clarity/server/internal/cache"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

// These exercise the real pipeline — FAKE_AGENT off — against a stand-in for
// P1's service, so the Sprint 2 "done when" list runs on every `go test`
// instead of needing the agent, Atlas, Docker and a Gemini key.

type fakeAgentService struct {
	server *httptest.Server

	visionCalls  atomic.Int32
	explainCalls atomic.Int32

	visionResponse  agent.VisionResponse
	visionStatus    int
	explainStatus   int
	explainResponse agent.ExplainResponse

	// delay stands in for how long a real model call takes, so tests that need
	// a job to still be running — cancellation, overload — have a window.
	delay time.Duration
}

func newFakeAgent(t *testing.T) *fakeAgentService {
	t.Helper()
	f := &fakeAgentService{
		visionResponse: agent.VisionResponse{
			ProblemText: "def binary_search(arr, target):\n    lo, hi = 0, len(arr)",
			Category:    agent.CategoryAlgorithm,
			Confidence:  0.91,
		},
		explainResponse: agent.ExplainResponse{
			Explanation: "**Step 1.** The loop condition never shrinks.",
			Storyboard: agent.Storyboard{Title: "Binary Search", Scenes: []agent.Scene{
				{Index: 0, Narration: "a", Visual: "b", DurationSeconds: 8},
				{Index: 1, Narration: "c", Visual: "d", DurationSeconds: 9},
			}},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /vision", func(w http.ResponseWriter, r *http.Request) {
		f.visionCalls.Add(1)
		if f.visionStatus != 0 {
			w.WriteHeader(f.visionStatus)
			_, _ = w.Write([]byte(`{"error":{"code":"model_error","message":"gemini down"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(f.visionResponse)
	})
	mux.HandleFunc("POST /explain", func(w http.ResponseWriter, r *http.Request) {
		f.explainCalls.Add(1)
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		if f.explainStatus != 0 {
			w.WriteHeader(f.explainStatus)
			_, _ = w.Write([]byte(`{"error":{"code":"model_error","message":"gemini down"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(f.explainResponse)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func newRealPipelineWorker(t *testing.T, agentURL string) (*Worker, *recordingStore, *recordingCache) {
	t.Helper()
	t.Setenv("MONGODB_URI", "mongodb://test.invalid/clarity") // never dialled; the store is injected
	t.Setenv("AGENT_URL", agentURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	js, cs := newRecordingStore(), newRecordingCache()
	w := NewWorker(cfg, js, cs, agent.New(agentURL), stubConcat)
	w.renderFunc = stubRenderFunc
	return w, js, cs
}

func TestRealPipelineProducesAnExplanationAndAVideo(t *testing.T) {
	f := newFakeAgent(t)
	w, js, cs := newRealPipelineWorker(t, f.server.URL)

	final := runToTerminal(t, w, js, New("why?", false, "test"))
	if final.Status != StatusDone {
		t.Fatalf("status = %q, want done (error %v)", final.Status, final.Error)
	}
	if final.Explanation == nil || *final.Explanation != f.explainResponse.Explanation {
		t.Errorf("explanation not taken from /explain: %v", final.Explanation)
	}
	if final.ProblemText == nil || *final.ProblemText != f.visionResponse.ProblemText {
		t.Errorf("problem_text not taken from /vision: %v", final.ProblemText)
	}
	if final.Category == nil || *final.Category != agent.CategoryAlgorithm {
		t.Errorf("category = %v", final.Category)
	}
	if final.ScenesTotal != len(f.explainResponse.Storyboard.Scenes) {
		t.Errorf("scenes_total = %d, want %d", final.ScenesTotal, len(f.explainResponse.Storyboard.Scenes))
	}
	if final.VideoURL == nil {
		t.Error("done without a video_url")
	}

	// The hash must come from what /vision actually returned.
	want := cache.Hash(f.visionResponse.ProblemText, "why?", false)
	if final.ProblemHash == nil || *final.ProblemHash != want {
		t.Errorf("problem_hash = %v, want %q", final.ProblemHash, want)
	}
	if _, err := cs.Get(context.Background(), want); err != nil {
		t.Errorf("nothing cached after a successful job: %v", err)
	}
}

// "category: unknown -> failed, error: no_problem_found" — and /explain is
// never reached, because there is nothing to explain.
func TestUnknownCategoryFailsTheJob(t *testing.T) {
	f := newFakeAgent(t)
	f.visionResponse = agent.VisionResponse{ProblemText: "", Category: agent.CategoryUnknown, Confidence: 0.1}
	w, js, cs := newRealPipelineWorker(t, f.server.URL)

	final := runToTerminal(t, w, js, New("", false, "test"))
	if final.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", final.Status)
	}
	if final.Error == nil || *final.Error != ErrNoProblemFound {
		t.Errorf("error = %v, want %q", final.Error, ErrNoProblemFound)
	}
	if final.Explanation != nil {
		t.Error("a failed job must not carry an explanation")
	}
	if n := f.explainCalls.Load(); n != 0 {
		t.Errorf("/explain called %d times on an unreadable capture", n)
	}
	if cs.len() != 0 {
		t.Error("a failed job wrote to the cache")
	}
}

// Rule 11: the cache-hit path never calls /explain.
func TestCacheHitSkipsExplain(t *testing.T) {
	f := newFakeAgent(t)
	w, js, cs := newRealPipelineWorker(t, f.server.URL)

	first := runToTerminal(t, w, js, New("why?", false, "test"))
	if first.Cached {
		t.Fatal("the first run of a problem cannot be a cache hit")
	}
	explainAfterFirst := f.explainCalls.Load()
	if explainAfterFirst != 1 {
		t.Fatalf("/explain called %d times on a miss, want 1", explainAfterFirst)
	}

	second := runToTerminal(t, w, js, New("why?", false, "test"))
	if !second.Cached {
		t.Fatal("the same problem twice must be a cache hit")
	}
	if got := f.explainCalls.Load(); got != explainAfterFirst {
		t.Errorf("/explain called again on a cache hit (%d -> %d); rule 11 says never", explainAfterFirst, got)
	}
	// A hit returns both fields — a video with nothing to read defeats the point.
	if second.Explanation == nil || *second.Explanation == "" {
		t.Error("cache hit returned no explanation")
	}
	if second.VideoURL == nil || *second.VideoURL == "" {
		t.Error("cache hit returned no video_url")
	}
	if second.Status != StatusDone {
		t.Errorf("status = %q, want done", second.Status)
	}
	_ = cs
}

// Guardrails split the cache key, so the same problem asked both ways is
// explained twice.
func TestGuardrailsMissesTheCache(t *testing.T) {
	f := newFakeAgent(t)
	w, js, _ := newRealPipelineWorker(t, f.server.URL)

	runToTerminal(t, w, js, New("why?", false, "test"))
	runToTerminal(t, w, js, New("why?", true, "test"))
	if got := f.explainCalls.Load(); got != 2 {
		t.Errorf("/explain called %d times, want 2 — guardrails must not share a cache entry", got)
	}
}

// "Kill the agent service mid-job -> job ends failed, never stuck."
func TestAgentFailuresEndTheJob(t *testing.T) {
	cases := []struct {
		name                        string
		visionStatus, explainStatus int
		wantError                   string
	}{
		{"vision 502", http.StatusBadGateway, 0, ErrInternal},
		{"explain 502", 0, http.StatusBadGateway, ErrExplainFailed},
		{"explain 504", 0, http.StatusGatewayTimeout, ErrExplainFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAgent(t)
			f.visionStatus, f.explainStatus = tc.visionStatus, tc.explainStatus
			w, js, cs := newRealPipelineWorker(t, f.server.URL)

			final := runToTerminal(t, w, js, New("", false, "test"))
			if final.Status != StatusFailed {
				t.Fatalf("status = %q, want failed", final.Status)
			}
			if final.Error == nil || *final.Error != tc.wantError {
				t.Errorf("error = %v, want %q", final.Error, tc.wantError)
			}
			if cs.len() != 0 {
				t.Error("a failed job wrote to the cache")
			}
		})
	}
}

// An agent that is not running at all is the same story: failed, not stuck.
func TestUnreachableAgentEndsTheJob(t *testing.T) {
	w, js, _ := newRealPipelineWorker(t, "http://127.0.0.1:1")
	final := runToTerminal(t, w, js, New("", false, "test"))
	if final.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", final.Status)
	}
}

// Rule 8 on the real path: the explanation is in the job record before any
// render work starts.
func TestExplanationIsWrittenBeforeSceneWorkOnTheRealPath(t *testing.T) {
	f := newFakeAgent(t)
	w, js, _ := newRealPipelineWorker(t, f.server.URL)
	runToTerminal(t, w, js, New("", false, "test"))

	explainedAt, renderingAt := -1, -1
	for i, fields := range js.snapshot() {
		if _, ok := fields["explanation"]; ok && explainedAt < 0 {
			explainedAt = i
		}
		if s, ok := fields["status"].(Status); ok && s == StatusRendering && renderingAt < 0 {
			renderingAt = i
		}
	}
	if explainedAt < 0 {
		t.Fatal("the explanation was never written")
	}
	if renderingAt >= 0 && explainedAt > renderingAt {
		t.Fatalf("explanation written at write %d, after render work began at %d", explainedAt, renderingAt)
	}
}

// A panic during the render must not take the pipeline goroutine down with it.
func TestPanickingRenderIsContained(t *testing.T) {
	f := newFakeAgent(t)
	w, js, _ := newRealPipelineWorker(t, f.server.URL)
	w.renderFunc = func(ctx context.Context, scenes []agent.Scene, workDir string) (string, bool) {
		panic("render exploded")
	}

	final := runToTerminal(t, w, js, New("", false, "test"))
	if final.Status != StatusDone {
		t.Fatalf("status = %q, want done — a panicking render must not fail the job", final.Status)
	}
	if final.VideoURL != nil {
		t.Errorf("video_url = %v, want null after a panicking render", *final.VideoURL)
	}
}

// A render that does not work out is `done` with no video and nothing
// cached, not a job failure — the explanation is still the product.
func TestRenderFailureStillCompletes(t *testing.T) {
	f := newFakeAgent(t)
	w, js, cs := newRealPipelineWorker(t, f.server.URL)
	w.renderFunc = func(ctx context.Context, scenes []agent.Scene, workDir string) (string, bool) {
		return "", false
	}

	final := runToTerminal(t, w, js, New("", false, "test"))
	if final.Status != StatusDone {
		t.Fatalf("status = %q, want done", final.Status)
	}
	if final.VideoURL != nil {
		t.Errorf("video_url = %v, want null", *final.VideoURL)
	}
	if final.Explanation == nil {
		t.Error("the explanation must survive a total render failure")
	}
	if final.Error == nil || *final.Error != ErrRenderFailed {
		t.Errorf("error = %v, want %q", final.Error, ErrRenderFailed)
	}
	if cs.len() != 0 {
		t.Error("nothing may be cached when the render did not work out")
	}
}

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStripDataURL(t *testing.T) {
	cases := []struct {
		in, wantB64, wantType string
	}{
		{"data:image/png;base64,AAAA", "AAAA", "image/png"},
		{"data:image/jpeg;base64,BBBB", "BBBB", "image/jpeg"},
		// Already raw base64 passes through untouched.
		{"AAAA", "AAAA", ""},
		{"data:image/png;base64", "data:image/png;base64", ""},
	}
	for _, tc := range cases {
		b64, mt := StripDataURL(tc.in)
		if b64 != tc.wantB64 || mt != tc.wantType {
			t.Errorf("StripDataURL(%q) = (%q, %q), want (%q, %q)", tc.in, b64, mt, tc.wantB64, tc.wantType)
		}
	}
}

// P1 receives raw base64 — the coordinator strips the prefix (API.md §3.1).
func TestVisionSendsRawBase64(t *testing.T) {
	var got VisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vision" {
			t.Errorf("path = %q, want /vision", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q", ct)
		}
		if id := r.Header.Get("X-Request-Id"); id != "req_abc" {
			t.Errorf("X-Request-Id = %q, want it propagated", id)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(VisionResponse{
			ProblemText: "d/dx [x^2 sin x]", Category: CategoryMath, Confidence: 0.9,
		})
	}))
	defer srv.Close()

	ctx := WithRequestID(context.Background(), "req_abc")
	resp, err := New(srv.URL).Vision(ctx, "data:image/png;base64,AAAA", "", false)
	if err != nil {
		t.Fatalf("Vision: %v", err)
	}
	if got.ImageB64 != "AAAA" {
		t.Errorf("image_b64 = %q, want the prefix stripped", got.ImageB64)
	}
	if got.MediaType != "image/png" {
		t.Errorf("media_type = %q, want it taken from the data URL", got.MediaType)
	}
	if resp.ProblemText != "d/dx [x^2 sin x]" || resp.Category != CategoryMath {
		t.Errorf("response not decoded: %+v", resp)
	}
}

// category "unknown" with empty text is a valid 200, not an error.
func TestVisionUnknownIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(VisionResponse{ProblemText: "", Category: CategoryUnknown, Confidence: 0.1})
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Vision(context.Background(), "AAAA", "image/png", false)
	if err != nil {
		t.Fatalf("unknown must not be an error: %v", err)
	}
	if resp.Category != CategoryUnknown {
		t.Errorf("category = %q", resp.Category)
	}
}

func TestExplainRoundTrip(t *testing.T) {
	var got ExplainRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(ExplainResponse{
			Explanation: "**Step 1.**",
			Storyboard: Storyboard{Title: "T", Scenes: []Scene{
				{Index: 0, Narration: "n", Visual: "v", DurationSeconds: 8},
			}},
			Revisions: 1,
		})
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Explain(context.Background(), ExplainRequest{
		ProblemText: "p", Category: CategoryAlgorithm, UserPrompt: "why?", Guardrails: true,
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if got.ProblemText != "p" || got.Category != CategoryAlgorithm || got.UserPrompt != "why?" || !got.Guardrails {
		t.Errorf("request not sent faithfully: %+v", got)
	}
	if resp.Revisions != 1 || len(resp.Storyboard.Scenes) != 1 || resp.Storyboard.Scenes[0].DurationSeconds != 8 {
		t.Errorf("response not decoded: %+v", resp)
	}
}

// ok:false is data, not an error — the caller drops that scene.
func TestScenesRenderOKFalseIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(SceneRenderResponse{
			OK: false, Attempts: 3, LintRetries: 4, Stage: "render", LastTraceback: "NameError",
		})
	}))
	defer srv.Close()

	resp, err := New(srv.URL).ScenesRender(context.Background(), SceneRenderRequest{JobID: "j_1"})
	if err != nil {
		t.Fatalf("ok:false must not be a transport error: %v", err)
	}
	if resp.OK || resp.Attempts != 3 || resp.Stage != "render" {
		t.Errorf("response not decoded: %+v", resp)
	}
}

// The quality flag is job-level and must reach the agent unchanged (rule 12).
func TestScenesRenderPassesQualityThrough(t *testing.T) {
	var got SceneRenderRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(SceneRenderResponse{OK: true, ClipPath: "/tmp/x.mp4"})
	}))
	defer srv.Close()

	_, err := New(srv.URL).ScenesRender(context.Background(), SceneRenderRequest{
		JobID: "j_1", Quality: "-qm", WorkDir: "/tmp/clarity/j_1/scene0",
		Scene: Scene{Index: 0, Narration: "n", Visual: "v"},
	})
	if err != nil {
		t.Fatalf("ScenesRender: %v", err)
	}
	if got.Quality != "-qm" || got.WorkDir != "/tmp/clarity/j_1/scene0" {
		t.Errorf("request not sent faithfully: %+v", got)
	}
}

// Both envelope shapes: API.md §4's flat one, and the {"detail": {...}} that
// FastAPI's HTTPException currently produces on the agent side.
func TestErrorEnvelopeShapes(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		wantCode   string
		retryable  bool
	}{
		{"flat envelope", `{"error":{"code":"model_error","message":"gemini blew up"},"request_id":"req_1"}`,
			502, "model_error", true},
		{"fastapi detail wrapper", `{"detail":{"error":{"code":"model_error","message":"gemini blew up"}}}`,
			502, "model_error", true},
		{"schema violation is not retryable", `{"error":{"code":"schema_violation","message":"bad draft"}}`,
			422, "schema_violation", false},
		{"unparseable body", `<html>502 Bad Gateway</html>`, 502, "internal", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := New(srv.URL).Explain(context.Background(), ExplainRequest{})
			if err == nil {
				t.Fatal("want an error")
			}
			var agentErr *Error
			if !asAgentError(err, &agentErr) {
				t.Fatalf("err = %T (%v), want *agent.Error", err, err)
			}
			if agentErr.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", agentErr.Code, tc.wantCode)
			}
			if agentErr.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", agentErr.StatusCode, tc.status)
			}
			if agentErr.Retryable() != tc.retryable {
				t.Errorf("Retryable() = %v, want %v", agentErr.Retryable(), tc.retryable)
			}
		})
	}
}

// Cancelling a job must abort its in-flight agent calls.
func TestCancelAbortsTheRequest(t *testing.T) {
	started := make(chan struct{})
	// The handler needs a release the test controls: a handler parked on
	// r.Context() alone can outlive the client's cancel, and httptest.Close
	// then blocks on it forever.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := New(srv.URL).Explain(ctx, ExplainRequest{})
		done <- err
	}()

	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error after cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Explain did not return after the context was cancelled")
	}
}

// An unreachable agent is a transport error, never a panic or a hang.
func TestUnreachableAgent(t *testing.T) {
	// Port 1 is reserved and refuses connections immediately.
	_, err := New("http://127.0.0.1:1").Vision(context.Background(), "AAAA", "image/png", false)
	if err == nil {
		t.Fatal("want an error from an unreachable agent")
	}
}

func asAgentError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

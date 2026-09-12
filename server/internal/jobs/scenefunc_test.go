package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

func sceneTestConfig(t *testing.T, agentURL string) *config.Config {
	t.Helper()
	t.Setenv("MONGODB_URI", "mongodb://test.invalid/clarity")
	t.Setenv("AGENT_URL", agentURL)
	t.Setenv("MANIM_QUALITY", "-qm")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// writeClip puts a real file where the agent claims a clip is, since the
// coordinator refuses a clip_path it cannot stat.
func writeClip(t *testing.T, workDir, name string) string {
	t.Helper()
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(workDir, name)
	if err := os.WriteFile(p, []byte("clip bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAgentSceneFuncSuccess(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "scene0")
	clip := writeClip(t, workDir, "scene0.mp4")

	var got agent.SceneRenderRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scenes/render" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(agent.SceneRenderResponse{
			OK: true, ClipPath: clip, Attempts: 2, LintRetries: 1,
			SnippetsUsed: []string{"a", "b"}, SnippetID: "66f9",
		})
	}))
	defer srv.Close()

	fn := AgentSceneFunc(agent.New(srv.URL), sceneTestConfig(t, srv.URL),
		"j_abc", "Binary Search", agent.CategoryAlgorithm, true)

	scene := agent.Scene{Index: 0, Narration: "n", Visual: "v", DurationSeconds: 8}
	path, ok := fn(context.Background(), scene, workDir)
	if !ok {
		t.Fatal("a successful scene was reported as dropped")
	}
	if path != clip {
		t.Errorf("clip path = %q, want %q", path, clip)
	}

	// Everything the agent needs must be carried faithfully, especially the
	// job-level quality flag (rule 12).
	if got.JobID != "j_abc" || got.StoryboardTitle != "Binary Search" ||
		got.Category != agent.CategoryAlgorithm || !got.Guardrails {
		t.Errorf("request not sent faithfully: %+v", got)
	}
	if got.Quality != "-qm" {
		t.Errorf("quality = %q, want the job-level -qm", got.Quality)
	}
	if got.WorkDir != workDir || got.Scene.Index != 0 {
		t.Errorf("work_dir/scene not sent faithfully: %+v", got)
	}
}

// Every way a scene can fail drops that scene and nothing else.
func TestAgentSceneFuncDropsBadScenes(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "scene0")
	realClip := writeClip(t, workDir, "scene0.mp4")
	outside := filepath.Join(t.TempDir(), "elsewhere.mp4")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := writeClip(t, workDir, "empty.mp4")
	if err := os.Truncate(empty, 0); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		status  int
		body    any
		wantOK  bool
		comment string
	}{
		{name: "agent gave up", status: 200,
			body: agent.SceneRenderResponse{OK: false, Attempts: 3, Stage: "render", LastTraceback: "NameError: x\nmore"}},
		{name: "agent 502", status: 502, body: map[string]any{"error": map[string]string{"code": "model_error"}}},
		{name: "empty clip_path", status: 200, body: agent.SceneRenderResponse{OK: true, ClipPath: ""}},
		{name: "clip that does not exist", status: 200,
			body: agent.SceneRenderResponse{OK: true, ClipPath: filepath.Join(workDir, "nope.mp4")}},
		{name: "zero-byte clip", status: 200, body: agent.SceneRenderResponse{OK: true, ClipPath: empty}},
		{name: "clip outside the work dir", status: 200,
			body: agent.SceneRenderResponse{OK: true, ClipPath: outside}},
		{name: "relative clip path", status: 200,
			body: agent.SceneRenderResponse{OK: true, ClipPath: "scene0.mp4"}},
		{name: "traversal out of the work dir", status: 200,
			body: agent.SceneRenderResponse{OK: true, ClipPath: filepath.Join(workDir, "..", "..", "etc", "passwd")}},
		// The one that must succeed, so the table is not vacuously passing.
		{name: "a real clip in the work dir", status: 200,
			body: agent.SceneRenderResponse{OK: true, ClipPath: realClip}, wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()

			fn := AgentSceneFunc(agent.New(srv.URL), sceneTestConfig(t, srv.URL),
				"j_abc", "T", agent.CategoryAlgorithm, false)
			_, ok := fn(context.Background(), agent.Scene{Index: 0}, workDir)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
		})
	}
}

// Cancelling the job must abort the scene's agent call rather than waiting out
// the 11-minute budget.
func TestAgentSceneFuncHonoursCancellation(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Defers run last-in-first-out, so the release must be registered after the
	// server: Close waits for outstanding handlers, and a handler still parked
	// on `release` would deadlock the test.
	defer srv.Close()
	defer close(release)

	fn := AgentSceneFunc(agent.New(srv.URL), sceneTestConfig(t, srv.URL),
		"j_abc", "T", agent.CategoryAlgorithm, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		_, ok := fn(ctx, agent.Scene{Index: 0}, t.TempDir())
		done <- ok
	}()

	<-started
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Error("a cancelled scene must not report success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scene did not return after the job was cancelled")
	}
}

func TestIDFromWorkDir(t *testing.T) {
	root := WorkRoot("j_7f3a9c21")
	cases := []struct{ in, want string }{
		{filepath.Join(root, "scene0"), "j_7f3a9c21"},
		{filepath.Join(root, "scene12", "media"), "j_7f3a9c21"},
		{root, "j_7f3a9c21"},
		{"/etc/passwd", ""},
		{filepath.Dir(root), ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := IDFromWorkDir(tc.in); got != tc.want {
			t.Errorf("IDFromWorkDir(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

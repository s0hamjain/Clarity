package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
)

// POST /internal/render is the agent's render tool (API.md §2.7). The agent
// owns the repair loop; this endpoint just renders one source file once and
// reports what happened. It is the security gate: the static pre-check runs
// here before every `docker run`, including the agent's repair attempts
// (FRD §23 rule 14), and nothing renders without the semaphore (rule 15).

// RenderFunc is P2's render.Render, FRD §14.1. It is a field rather than a
// direct call so the fake can stand in until P2's Render lands in Sprint 3.
type RenderFunc func(ctx context.Context, src, workDir, quality string) (clipPath string, err *render.RenderError)

// semaphoreWait is how long a render may queue before the agent is told to come
// back later (API.md §7).
const semaphoreWait = 60 * time.Second

type internalRenderRequest struct {
	Source  string `json:"source"`
	WorkDir string `json:"work_dir"`
	Quality string `json:"quality"`
}

type internalRenderResponse struct {
	OK              bool     `json:"ok"`
	ClipPath        string   `json:"clip_path,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	Stage           string   `json:"stage,omitempty"`
	Traceback       string   `json:"traceback,omitempty"`
}

func (s *Server) internalRender(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		slog.Warn("rejected non-loopback /internal/render",
			"request_id", RequestIDFrom(r.Context()), "remote_addr", r.RemoteAddr)
		writeError(w, r, http.StatusForbidden, CodeForbidden,
			"/internal/render is reachable from localhost only.", nil)
		return
	}

	var req internalRenderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, "Body is not valid JSON.", nil)
		return
	}
	for field, value := range map[string]string{"source": req.Source, "work_dir": req.WorkDir} {
		if strings.TrimSpace(value) == "" {
			writeError(w, r, http.StatusBadRequest, CodeBadRequest,
				"Field `"+field+"` is required.", map[string]any{"field": field})
			return
		}
	}
	if req.Quality == "" {
		req.Quality = s.cfg.ManimQuality
	}

	// The agent supplies work_dir, so confirm it is one of ours before anything
	// writes there. Generated code is untrusted; the path that carries it is too.
	workDir, err := safeWorkDir(req.WorkDir)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, err.Error(),
			map[string]any{"field": "work_dir"})
		return
	}

	// A container cannot start without Docker, and that is infrastructure, not
	// something the agent's repair loop can fix — so it is a non-2xx.
	if !s.health.Report().Docker {
		writeError(w, r, http.StatusServiceUnavailable, CodeDependencyDown,
			"Docker is not running; no scene can be rendered.", nil)
		return
	}

	// Cancelling the job must kill the container this call is about to start,
	// so the render runs under the job's own context, found from work_dir.
	parent := r.Context()
	if jobCtx, ok := s.pipeline.JobContext(jobs.IDFromWorkDir(workDir)); ok {
		parent = mergeCancel(r.Context(), jobCtx)
	}

	release, ok := s.acquireRender(parent)
	if !ok {
		w.Header().Set("Retry-After", "30")
		writeError(w, r, http.StatusTooManyRequests, CodeRenderBusy,
			"Every render slot is busy. Retry after the suggested delay.",
			map[string]any{"waited_seconds": int(s.renderWait.Seconds())})
		return
	}
	defer release()

	// Rule 14: the pre-check runs on every call, before any container starts.
	if rerr := render.Precheck(req.Source); rerr != nil {
		slog.Info("precheck rejected source",
			"request_id", RequestIDFrom(r.Context()), "work_dir", workDir, "traceback", rerr.Traceback)
		writeJSON(w, http.StatusOK, internalRenderResponse{
			OK: false, Stage: rerr.Stage, Traceback: rerr.Traceback,
		})
		return
	}

	ctx, cancel := context.WithTimeout(parent, time.Duration(s.cfg.RenderTimeoutSec)*time.Second)
	defer cancel()

	clipPath, rerr := s.render(ctx, req.Source, workDir, req.Quality)
	if rerr != nil {
		// A failed render is a 200 with ok:false — that is data for the agent's
		// repair loop, not an error.
		slog.Info("render failed",
			"request_id", RequestIDFrom(r.Context()), "work_dir", workDir, "stage", rerr.Stage)
		writeJSON(w, http.StatusOK, internalRenderResponse{
			OK: false, Stage: rerr.Stage, Traceback: rerr.Traceback,
		})
		return
	}

	slog.Info("render succeeded",
		"request_id", RequestIDFrom(r.Context()), "work_dir", workDir, "clip_path", clipPath)
	// duration_seconds stays null until P2's ffprobe validation lands in Sprint 3.
	writeJSON(w, http.StatusOK, internalRenderResponse{OK: true, ClipPath: clipPath})
}

// acquireRender takes a render slot, waiting at most semaphoreWait. The
// returned func gives the slot back.
func (s *Server) acquireRender(ctx context.Context) (release func(), ok bool) {
	timer := time.NewTimer(s.renderWait)
	defer timer.Stop()

	select {
	case s.renderSem <- struct{}{}:
		return func() { <-s.renderSem }, true
	case <-timer.C:
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
}

// safeWorkDir keeps renders inside the coordinator's own work root, so a
// work_dir of "/etc" or "../../somewhere" cannot be written to.
func safeWorkDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	if !filepath.IsAbs(clean) {
		return "", errors.New("`work_dir` must be an absolute path")
	}
	root := filepath.Clean(jobs.WorkRoot(""))
	rel, err := filepath.Rel(root, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("`work_dir` must be inside " + root)
	}
	return clean, nil
}

// mergeCancel returns a context that is cancelled when either input is. The
// request's own context alone is not enough: a DELETE cancels the job, not this
// HTTP call, and the container has to die with it.
func mergeCancel(a, b context.Context) context.Context {
	merged, cancel := context.WithCancel(a)
	go func() {
		defer cancel()
		select {
		case <-b.Done():
		case <-merged.Done():
		}
	}()
	return merged
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

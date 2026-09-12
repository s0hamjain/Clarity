// Package api is the coordinator's HTTP surface: the public API the desktop app
// talks to (API.md §2) plus the middleware every response passes through.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/config"
	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
)

// Limits from API.md §7.
const (
	maxImageBytes      = 8 << 20 // 8 MB decoded
	maxUserPromptChars = 2000
	// Base64 inflates by 4/3; leave room for that plus the JSON around it.
	maxRequestBytes = 16 << 20
)

// Pipeline is the job worker. POST /api/jobs hands the job over and returns
// immediately (FRD §23 rule 7) — everything after that is the pipeline's.
type Pipeline interface {
	// Start drives one job to a terminal status in its own goroutine.
	// imageB64 is raw base64 with the data-URL prefix already stripped; it is
	// never persisted (FRD §20). Returns jobs.ErrQueueFull when the
	// coordinator is already running as many jobs as it will accept.
	Start(j *jobs.Job, imageB64, mediaType, requestID string) error
	// Cancel stops pending work for a job. Idempotent.
	Cancel(ctx context.Context, id string) error
	// JobContext returns a running job's context so /internal/render can bind
	// a container's lifetime to it. False once the job is no longer running.
	JobContext(id string) (context.Context, bool)
}

// Server wires the stores, the pipeline and the health checker into a router.
type Server struct {
	cfg      *config.Config
	jobs     jobs.Store
	cache    jobs.CacheStore
	pipeline Pipeline
	health   *Health

	// render is P2's render.Render (FRD §14.1). A field, not a direct call, so
	// tests can stand in for Docker.
	render RenderFunc
	// renderSem is P2's semaphore, one instance shared by every job, held
	// around `docker run` and nothing else (rule 15).
	renderSem render.Semaphore
	// renderWait is how long a render may queue for a slot. Tests shorten it.
	renderWait time.Duration
}

func NewServer(cfg *config.Config, j jobs.Store, c jobs.CacheStore, p Pipeline, h *Health, renderFn RenderFunc) *Server {
	return &Server{
		cfg:        cfg,
		jobs:       j,
		cache:      c,
		pipeline:   p,
		health:     h,
		render:     renderFn,
		renderSem:  render.NewSemaphore(cfg.RenderConcurrency),
		renderWait: semaphoreWait,
	}
}

// Routes returns the handler for the whole service — the paths of API.md §2,
// exactly those, wrapped in the middleware every response needs.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/jobs", s.createJob)
	mux.HandleFunc("GET /api/jobs/{job_id}", s.getJob)
	mux.HandleFunc("DELETE /api/jobs/{job_id}", s.cancelJob)
	mux.HandleFunc("GET /api/cache/{problem_hash}", s.getCache)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("POST /internal/render", s.internalRender)

	// Anything else is a 404 in the error envelope, not net/http's bare text.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, CodeBadRequest,
			fmt.Sprintf("No route for %s %s.", r.Method, r.URL.Path), nil)
	})

	return RequestID(CORS(mux))
}

// --- POST /api/jobs (API.md §2.1) ----------------------------------------

type createJobRequest struct {
	Image      string `json:"image"`
	UserPrompt string `json:"user_prompt"`
	Guardrails bool   `json:"guardrails"`
	Source     string `json:"source"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)

	var req createJobRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, CodeImageTooLarge,
				fmt.Sprintf("Request body exceeds %d bytes; the image limit is %d bytes decoded.", maxRequestBytes, maxImageBytes),
				map[string]any{"limit_bytes": maxImageBytes})
			return
		}
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, "Body is not valid JSON.", nil)
		return
	}

	if req.Image == "" {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, "Field `image` is required.",
			map[string]any{"field": "image"})
		return
	}
	if len(req.UserPrompt) > maxUserPromptChars {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("`user_prompt` is %d chars; the limit is %d.", len(req.UserPrompt), maxUserPromptChars),
			map[string]any{"field": "user_prompt", "limit_chars": maxUserPromptChars})
		return
	}

	imageB64, mediaType, apiErr := parseImageDataURL(req.Image)
	if apiErr != nil {
		writeError(w, r, apiErr.status, apiErr.code, apiErr.message, apiErr.details)
		return
	}

	source := req.Source
	if source == "" {
		source = "desktop"
	}

	requestID := RequestIDFrom(r.Context())
	job := jobs.New(req.UserPrompt, req.Guardrails, source)
	if err := s.jobs.Create(r.Context(), job); err != nil {
		slog.Error("create job", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusServiceUnavailable, CodeDependencyDown,
			"Could not record the job. Is Atlas reachable?", nil)
		return
	}

	// Hand off and return. Nothing that can block runs before this response.
	if err := s.pipeline.Start(job, imageB64, mediaType, requestID); err != nil {
		// The job record exists but nothing will drive it, so close it out
		// rather than leaving a row that never moves.
		if uerr := s.jobs.Update(r.Context(), job.ID, jobs.Fields{
			"status": jobs.StatusFailed, "error": CodeQueueFull,
		}); uerr != nil {
			slog.Error("mark queue-rejected job failed", "request_id", requestID, "job_id", job.ID, "error", uerr)
		}
		slog.Warn("job rejected: queue full", "request_id", requestID, "job_id", job.ID, "depth", jobs.QueueDepth)
		w.Header().Set("Retry-After", "30")
		writeError(w, r, http.StatusServiceUnavailable, CodeQueueFull,
			"Too many captures are already being processed. Retry after the suggested delay.",
			map[string]any{"queue_depth": jobs.QueueDepth})
		return
	}

	slog.Info("job created",
		"request_id", requestID,
		"job_id", job.ID,
		"source", source,
		"guardrails", job.Guardrails,
	)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id":   job.ID,
		"poll_url": "/api/jobs/" + job.ID,
	})
}

type apiError struct {
	status  int
	code    string
	message string
	details map[string]any
}

// parseImageDataURL validates the data URL and returns the raw base64 payload
// and its media type. The coordinator strips the prefix so the agent receives
// raw base64 (API.md §3.1).
func parseImageDataURL(s string) (b64, mediaType string, err *apiError) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", "", &apiError{http.StatusBadRequest, CodeBadRequest,
			"`image` must be a data URL, e.g. data:image/png;base64,…", map[string]any{"field": "image"}}
	}
	comma := strings.Index(s, ",")
	if comma < 0 {
		return "", "", &apiError{http.StatusBadRequest, CodeBadRequest,
			"`image` is not a well-formed data URL: no comma separating the payload.", map[string]any{"field": "image"}}
	}
	header := s[len(prefix):comma]
	payload := s[comma+1:]

	if !strings.Contains(header, ";base64") {
		return "", "", &apiError{http.StatusBadRequest, CodeBadRequest,
			"`image` must be base64-encoded.", map[string]any{"field": "image"}}
	}
	mediaType = strings.TrimSpace(strings.SplitN(header, ";", 2)[0])
	if mediaType != "image/png" && mediaType != "image/jpeg" {
		return "", "", &apiError{http.StatusUnsupportedMediaType, CodeUnsupportedImageType,
			fmt.Sprintf("Image type %q is not supported; use image/png or image/jpeg.", mediaType),
			map[string]any{"media_type": mediaType}}
	}

	decoded, decErr := base64.StdEncoding.DecodeString(payload)
	if decErr != nil {
		return "", "", &apiError{http.StatusBadRequest, CodeBadRequest,
			"`image` payload is not valid base64.", map[string]any{"field": "image"}}
	}
	if len(decoded) > maxImageBytes {
		return "", "", &apiError{http.StatusRequestEntityTooLarge, CodeImageTooLarge,
			fmt.Sprintf("Image is %.1f MB; the limit is %d MB.", float64(len(decoded))/(1<<20), maxImageBytes>>20),
			map[string]any{"limit_bytes": maxImageBytes, "actual_bytes": len(decoded)}}
	}
	return payload, mediaType, nil
}

// --- GET /api/jobs/{id} (API.md §2.2) ------------------------------------

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobs.Get(r.Context(), r.PathValue("job_id"))
	if errors.Is(err, jobs.ErrNotFound) {
		s.jobNotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("get job", "request_id", RequestIDFrom(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read the job.", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// --- DELETE /api/jobs/{id} (API.md §2.3) ---------------------------------

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("job_id")
	if _, err := s.jobs.Get(r.Context(), id); errors.Is(err, jobs.ErrNotFound) {
		s.jobNotFound(w, r)
		return
	} else if err != nil {
		slog.Error("get job for cancel", "request_id", RequestIDFrom(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read the job.", nil)
		return
	}

	if err := s.pipeline.Cancel(r.Context(), id); err != nil {
		slog.Error("cancel job", "request_id", RequestIDFrom(r.Context()), "job_id", id, "error", err)
		writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not cancel the job.", nil)
		return
	}

	// Report whatever status the job actually settled on — cancelling a job that
	// already finished is a 200 with its current status, not an error.
	job, err := s.jobs.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"job_id": id, "status": string(jobs.StatusCancelled)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"job_id": id, "status": string(job.Status)})
}

func (s *Server) jobNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, CodeJobNotFound,
		"No such job. Unknown ID, or the job expired after 24 h.", nil)
}

// --- GET /api/cache/{hash} (API.md §2.5) ---------------------------------

func (s *Server) getCache(w http.ResponseWriter, r *http.Request) {
	entry, err := s.cache.Get(r.Context(), r.PathValue("problem_hash"))
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, CodeCacheMiss, "No cache entry for that hash.", nil)
		return
	}
	if err != nil {
		slog.Error("get cache", "request_id", RequestIDFrom(r.Context()), "error", err)
		writeError(w, r, http.StatusInternalServerError, CodeInternal, "Could not read the cache.", nil)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// --- GET /healthz (API.md §2.6) ------------------------------------------

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	report := s.health.Report()
	status := http.StatusOK
	if !report.OK {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}

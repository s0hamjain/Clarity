package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
	"github.com/s0hamjain/Clarity/server/internal/cache"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

// Worker drives one job from a screenshot to a video: transcribe, fingerprint,
// check the cache, explain, render the whole storyboard as one continuous
// script, upload, done.
type Worker struct {
	cfg   *config.Config
	jobs  Store
	cache CacheStore
	agent *agent.Client

	// concat is P2's render.Concat (FRD §14.1): a no-op join for the one clip
	// this job produced, then upload, returning the public URL. Kept under
	// this name so the upload path doesn't special-case the common case of
	// exactly one clip.
	concat ConcatFunc

	// renderFunc, when set, overrides how the storyboard becomes a clip. Only
	// tests set it; a real job gets its RenderFunc from renderFuncFor below.
	renderFunc RenderFunc

	mu      sync.Mutex
	running map[string]*jobCtl

	// slots bounds how many jobs may be in flight at once (API.md §7). It is
	// the back pressure behind 503 queue_full: without it, a burst of captures
	// becomes an unbounded pile of goroutines all waiting on model calls.
	slots chan struct{}
}

// jobCtl is the handle on one in-flight job. The mutex serializes the two
// writers to a job record — the pipeline goroutine and a DELETE — so a status
// can never land on top of a terminal one. Without it, a DELETE that arrives
// while a status write is already in flight leaves the job `cancelled` and then
// back at `explaining`, and the desktop app polls a job that rose from the dead.
type jobCtl struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// ConcatFunc is P2's render.Concat (FRD §14.1). A field rather than a direct
// call so the fake can stand in until P2's Concat lands.
type ConcatFunc func(ctx context.Context, clipPaths []string, outKey string) (videoURL string, err error)

// QueueDepth is the most jobs that may be in flight at once (API.md §7).
const QueueDepth = 32

func NewWorker(cfg *config.Config, j Store, c CacheStore, a *agent.Client, concat ConcatFunc) *Worker {
	return &Worker{
		cfg:     cfg,
		jobs:    j,
		cache:   c,
		agent:   a,
		concat:  concat,
		running: make(map[string]*jobCtl),
		slots:   make(chan struct{}, QueueDepth),
	}
}

// ErrQueueFull is returned by Start when the coordinator is already running as
// many jobs as it is willing to. The caller turns it into 503 queue_full.
var ErrQueueFull = errors.New("job queue is full")

// renderFuncFor decides how this job's storyboard becomes a clip.
func (w *Worker) renderFuncFor(j *Job, storyboardTitle, category string) RenderFunc {
	if w.renderFunc != nil {
		return w.renderFunc // test override
	}
	return AgentRenderFunc(w.agent, w.cfg, j.ID, storyboardTitle, category, j.Guardrails)
}

// Start runs the pipeline for one job in its own goroutine and returns at once.
// POST /api/jobs must not block on anything (FRD §23 rule 7). It returns
// ErrQueueFull when the coordinator is already at QueueDepth.
func (w *Worker) Start(j *Job, imageB64, mediaType, requestID string) error {
	select {
	case w.slots <- struct{}{}:
	default:
		return ErrQueueFull
	}

	// The pipeline outlives the HTTP request, so it gets its own context — but
	// it carries the request ID forward, so every line this job ever logs, in
	// either service, can be grepped back to the capture that started it.
	ctx, cancel := context.WithCancel(agent.WithRequestID(context.Background(), requestID))
	ctl := &jobCtl{ctx: ctx, cancel: cancel}
	w.mu.Lock()
	w.running[j.ID] = ctl
	w.mu.Unlock()

	go func() {
		defer func() { <-w.slots }()
		defer w.forget(j.ID)
		defer cleanupWorkDir(j.ID)
		// Rule 7: a panic anywhere in the pipeline ends the job as failed
		// rather than taking the process down or leaving the job stuck.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("pipeline panicked", "job_id", j.ID, "panic", p)
				w.fail(j.ID, ErrInternal)
			}
		}()
		w.run(ctx, j, imageB64, mediaType)
	}()
	return nil
}

// InFlight is how many jobs are running right now. Used by /healthz and the
// overload tests.
func (w *Worker) InFlight() int { return len(w.slots) }

// Cancel stops pending work for a job. Idempotent — cancelling a job that has
// already finished leaves its terminal status alone.
func (w *Worker) Cancel(ctx context.Context, id string) error {
	ctl := w.ctl(id)
	if ctl != nil {
		// Stop the pipeline first, then wait for any write it is in the middle
		// of, so `cancelled` is the last word on this job.
		ctl.cancel()
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
	}

	j, err := w.jobs.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.Status.IsTerminal() {
		return nil
	}
	// Nothing is written to the cache on this path (FRD §23 rule 10).
	return w.jobs.Update(ctx, id, Fields{"status": StatusCancelled})
}

// JobContext returns the context of a running job, so /internal/render can
// bind a container's lifetime to the job that asked for it. Reports false once
// the job has finished or was never here.
func (w *Worker) JobContext(id string) (context.Context, bool) {
	ctl := w.ctl(id)
	if ctl == nil {
		return nil, false
	}
	return ctl.ctx, true
}

func (w *Worker) ctl(id string) *jobCtl {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running[id]
}

func (w *Worker) forget(id string) {
	w.mu.Lock()
	delete(w.running, id)
	w.mu.Unlock()
}

// Job-level failure codes (API.md §4).
const (
	ErrNoProblemFound = "no_problem_found"
	ErrExplainFailed  = "explain_failed"
	ErrInternal       = "internal"
	ErrRenderFailed   = "render_failed"
)

// run is the pipeline for one job.
func (w *Worker) run(ctx context.Context, j *Job, imageB64, mediaType string) {
	// Every line this pipeline logs carries both IDs, so a capture can be
	// followed across the coordinator and the agent (whose thread_id is
	// job_id/scene).
	log := slog.With("job_id", j.ID, "request_id", agent.RequestIDFrom(ctx))
	log.Info("pipeline started", "guardrails", j.Guardrails, "quality", w.cfg.ManimQuality)

	// --- transcribing: read the problem off the screenshot ---
	if !w.setStatus(ctx, j.ID, StatusTranscribing) {
		return
	}
	vision, err := w.transcribe(ctx, imageB64, mediaType, j.UserPrompt, j.Guardrails)
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled; Cancel owns the status
		}
		log.Error("vision failed", "error", err)
		w.fail(j.ID, ErrInternal)
		return
	}

	// An unreadable capture is a finished job, not a broken one.
	if vision.Category == agent.CategoryUnknown {
		log.Info("no problem found in the capture", "confidence", vision.Confidence)
		w.fail(j.ID, ErrNoProblemFound)
		return
	}

	// --- fingerprint, and short-circuit on a cache hit ---
	hash := cache.Hash(vision.ProblemText, j.UserPrompt, j.Guardrails)
	if !w.update(ctx, j.ID, Fields{
		"problem_hash": hash,
		"problem_text": vision.ProblemText,
		"category":     vision.Category,
	}) {
		return
	}

	if entry, err := w.cache.Get(ctx, hash); err == nil {
		// Rule 11: the cache-hit path never calls /explain. Both fields are
		// returned, so there is something to read while the video loads.
		log.Info("cache hit", "problem_hash", hash)
		w.update(ctx, j.ID, Fields{
			"status":      StatusDone,
			"cached":      true,
			"explanation": entry.Explanation,
			"video_url":   entry.VideoURL,
		})
		return
	} else if !errors.Is(err, ErrNotFound) {
		// A cache lookup that fails for any other reason is a miss, not a
		// failure — the job can still be done the slow way.
		log.Warn("cache lookup failed; treating as a miss", "error", err)
	}

	// --- explaining: the explanation and the storyboard ---
	if !w.setStatus(ctx, j.ID, StatusExplaining) {
		return
	}
	explained, err := w.explain(ctx, agent.ExplainRequest{
		ProblemText: vision.ProblemText,
		Category:    vision.Category,
		UserPrompt:  j.UserPrompt,
		Guardrails:  j.Guardrails,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("explain failed", "error", err)
		w.fail(j.ID, ErrExplainFailed)
		return
	}

	// Rule 8, and the most important write in the server: the explanation goes
	// in before any scene work starts. Every moment between /explain returning
	// and this line is a moment the user waits for nothing.
	scenes := explained.Storyboard.Scenes
	if !w.update(ctx, j.ID, Fields{
		"status":       StatusGenerating,
		"explanation":  explained.Explanation,
		"scenes_total": len(scenes),
	}) {
		return
	}
	log.Info("explanation written",
		"problem_hash", hash, "scenes_total", len(scenes), "revisions", explained.Revisions)

	// --- rendering: one continuous script for the whole storyboard ---
	if !w.setStatus(ctx, j.ID, StatusRendering) {
		return
	}
	clipPath, ok := w.renderStoryboard(ctx, j.ID, scenes, w.renderFuncFor(j, explained.Storyboard.Title, vision.Category))
	if ctx.Err() != nil {
		return
	}
	if !ok {
		// The explanation survives; the animation quietly did not. Not a job
		// failure, and nothing is cached.
		log.Warn("render did not work out; finishing without a video")
		w.update(ctx, j.ID, Fields{"status": StatusDone, "video_url": nil, "error": ErrRenderFailed})
		return
	}
	log.Info("render finished")

	// --- uploading ---
	videoURL, err := w.upload(ctx, clipPath, hash)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("upload failed", "error", err)
		w.update(ctx, j.ID, Fields{"status": StatusDone, "video_url": nil, "error": ErrInternal})
		return
	}
	if !w.setStatus(ctx, j.ID, StatusUploading) {
		return
	}

	// Rule 10: the cache is written only now, after the upload succeeded, and
	// never on any path above.
	if err := w.cache.Put(ctx, hash, videoURL, explained.Explanation); err != nil {
		log.Error("cache put", "error", err) // not fatal; the job still completes
	}

	if !w.update(ctx, j.ID, Fields{"status": StatusDone, "video_url": videoURL}) {
		return
	}
	log.Info("pipeline finished", "status", StatusDone, "problem_hash", hash, "video_url", videoURL)
}

// --- the three swappable steps -------------------------------------------

func (w *Worker) transcribe(ctx context.Context, imageB64, mediaType, userPrompt string, guardrails bool) (*agent.VisionResponse, error) {
	return w.agent.Vision(ctx, imageB64, mediaType, userPrompt, guardrails)
}

func (w *Worker) explain(ctx context.Context, req agent.ExplainRequest) (*agent.ExplainResponse, error) {
	return w.agent.Explain(ctx, req)
}

// upload hands the one clip to P2's render.Concat, which joins it (a no-op
// for a single input) and uploads it, returning the public URL. The key is
// the problem hash, so the same problem always lands at the same object.
func (w *Worker) upload(ctx context.Context, clipPath, hash string) (string, error) {
	return w.concat(ctx, []string{clipPath}, "renders/"+hash+".mp4")
}

// --- job record helpers ---------------------------------------------------

// setStatus advances the job.
func (w *Worker) setStatus(ctx context.Context, id string, status Status) bool {
	return w.update(ctx, id, Fields{"status": status})
}

// update writes to a job under its control lock, refusing once the job's
// context is cancelled. Every write the pipeline makes goes through here.
// Returns false when the caller must stop.
func (w *Worker) update(ctx context.Context, id string, fields Fields) bool {
	if ctl := w.ctl(id); ctl != nil {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
	}
	if ctx.Err() != nil {
		return false // cancelled while we waited for the lock
	}
	if err := w.jobs.Update(ctx, id, fields); err != nil {
		slog.Error("update job", "job_id", id, "fields", len(fields), "error", err)
		w.failLocked(id, ErrInternal)
		return false
	}
	return true
}

// fail ends a job with a code from API.md §4. It uses a fresh context, because
// the job's own context is often the reason we got here, and refuses to
// overwrite a terminal status — a job the user cancelled stays `cancelled`.
func (w *Worker) fail(id, code string) {
	if ctl := w.ctl(id); ctl != nil {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
	}
	w.failLocked(id, code)
}

// failLocked is fail for callers that already hold the job's control lock.
func (w *Worker) failLocked(id, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if j, err := w.jobs.Get(ctx, id); err == nil && j.Status.IsTerminal() {
		return
	}
	if err := w.jobs.Update(ctx, id, Fields{"status": StatusFailed, "error": code}); err != nil {
		slog.Error("mark failed", "job_id", id, "error", err)
	}
}

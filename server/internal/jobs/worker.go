package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/cache"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

// Worker drives one job from a screenshot to a video.
//
// Sprint 1 is the fake pipeline: with FAKE_AGENT=1 and FAKE_RENDER=1 it walks
// any job through every status on a one-second timer with hardcoded data, using
// no Atlas, no Docker and no API keys. That is the server P4 builds the desktop
// UI against in Sprints 1–2. Sprint 2 replaces the fake /vision and /explain
// steps with real agent calls; Sprint 3 replaces the fake render.
type Worker struct {
	cfg   *config.Config
	jobs  Store
	cache CacheStore

	mu      sync.Mutex
	running map[string]*jobCtl

	// stepInterval is how long the fake pipeline holds each status. One
	// second matches the desktop app's poll interval, so P4 sees every
	// status exactly once. Tests shorten it.
	stepInterval time.Duration
}

// jobCtl is the handle on one in-flight job. The mutex serializes the two
// writers to a job record — the pipeline goroutine and a DELETE — so a status
// can never land on top of a terminal one. Without it, a DELETE that arrives
// while a status write is already in flight leaves the job `cancelled` and then
// back at `explaining`, and the desktop app polls a job that rose from the dead.
type jobCtl struct {
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewWorker(cfg *config.Config, j Store, c CacheStore) *Worker {
	return &Worker{cfg: cfg, jobs: j, cache: c, running: make(map[string]*jobCtl), stepInterval: time.Second}
}

// Start runs the pipeline for one job in its own goroutine and returns at once.
// POST /api/jobs must not block on anything (FRD §23 rule 7).
func (w *Worker) Start(j *Job, imageB64, mediaType string) {
	ctx, cancel := context.WithCancel(context.Background())
	ctl := &jobCtl{cancel: cancel}
	w.mu.Lock()
	w.running[j.ID] = ctl
	w.mu.Unlock()

	go func() {
		defer w.forget(j.ID)
		// Rule 7: a panic anywhere in the pipeline ends the job as failed
		// rather than taking the process down or leaving the job stuck.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("pipeline panicked", "job_id", j.ID, "panic", p)
				w.fail(j.ID, "internal")
			}
		}()
		w.run(ctx, j, imageB64, mediaType)
	}()
}

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

// update writes to a job under its control lock, refusing once the job's
// context is cancelled. Every write the pipeline makes goes through here.
func (w *Worker) update(ctx context.Context, id string, fields Fields) error {
	if ctl := w.ctl(id); ctl != nil {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return err // cancelled while we waited for the lock
	}
	return w.jobs.Update(ctx, id, fields)
}

// run is the fake pipeline. Every step checks for cancellation first, so a
// DELETE lands within a second.
func (w *Worker) run(ctx context.Context, j *Job, imageB64, mediaType string) {
	log := slog.With("job_id", j.ID)
	log.Info("pipeline started",
		"fake_agent", w.cfg.FakeAgent,
		"fake_render", w.cfg.FakeRender,
		"image_bytes_b64", len(imageB64),
		"media_type", mediaType,
		"quality", w.cfg.ManimQuality,
	)

	// transcribing — stands in for POST /vision (Sprint 2).
	if !w.step(ctx, j.ID, StatusTranscribing, nil) {
		return
	}

	problemText := fakeProblemText
	category := "algorithm"
	hash := cache.Hash(problemText, j.UserPrompt, j.Guardrails)

	// explaining — problem_hash, problem_text and category are set by now
	// (API.md §5). The real cache lookup goes in right here in Sprint 2.
	if !w.step(ctx, j.ID, StatusExplaining, Fields{
		"problem_hash": hash,
		"problem_text": problemText,
		"category":     category,
	}) {
		return
	}

	// generating — the explanation is written the instant it exists
	// (FRD §23 rule 8). This is the moment the user stops waiting.
	if !w.step(ctx, j.ID, StatusGenerating, Fields{
		"explanation":  fakeExplanation,
		"scenes_total": fakeScenesTotal,
	}) {
		return
	}
	log.Info("explanation written", "problem_hash", hash, "scenes_total", fakeScenesTotal)

	// rendering — one scene finishes per second.
	if !w.step(ctx, j.ID, StatusRendering, nil) {
		return
	}
	for done := 1; done <= fakeScenesTotal; done++ {
		if !w.sleep(ctx) {
			return
		}
		if err := w.update(ctx, j.ID, Fields{"scenes_done": done}); err != nil {
			log.Error("update scenes_done", "error", err)
			w.fail(j.ID, "internal")
			return
		}
	}

	if !w.step(ctx, j.ID, StatusConcatenating, nil) {
		return
	}
	if !w.step(ctx, j.ID, StatusUploading, nil) {
		return
	}
	// Hold `uploading` for a poll interval like every other status, so P4 can
	// see it. Without this the upload and the done write land in the same
	// millisecond and the status is never observable.
	if !w.sleep(ctx) {
		return
	}

	// The cache is written only now, after the (fake) upload succeeded
	// (FRD §23 rule 10), and carries both fields so a hit has something to read
	// while the video loads.
	if err := w.cache.Put(ctx, hash, w.cfg.FakeVideoURL, fakeExplanation); err != nil {
		log.Error("cache put", "error", err) // not fatal; the job still completes
	}

	if err := w.update(ctx, j.ID, Fields{
		"status":    StatusDone,
		"video_url": w.cfg.FakeVideoURL,
	}); err != nil {
		log.Error("mark done", "error", err)
		return
	}
	log.Info("pipeline finished", "status", StatusDone, "problem_hash", hash)
}

// step sleeps one second, then moves the job to status with any extra fields.
// It returns false when the job was cancelled or could not be written, in which
// case the caller must stop.
func (w *Worker) step(ctx context.Context, id string, status Status, fields Fields) bool {
	if !w.sleep(ctx) {
		return false
	}
	if fields == nil {
		fields = Fields{}
	}
	fields["status"] = status
	if err := w.update(ctx, id, fields); err != nil {
		slog.Error("advance status", "job_id", id, "status", status, "error", err)
		w.fail(id, "internal")
		return false
	}
	return true
}

// sleep waits one second unless the job is cancelled first.
func (w *Worker) sleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(w.stepInterval):
		return true
	}
}

// fail ends a job with an error code from API.md §4. It uses a fresh context,
// because the job's own context is usually the reason we got here, but it still
// takes the control lock and refuses to overwrite a terminal status — a job the
// user cancelled stays `cancelled`.
func (w *Worker) fail(id, code string) {
	if ctl := w.ctl(id); ctl != nil {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if j, err := w.jobs.Get(ctx, id); err == nil && j.Status.IsTerminal() {
		return
	}
	if err := w.jobs.Update(ctx, id, Fields{"status": StatusFailed, "error": code}); err != nil {
		slog.Error("mark failed", "job_id", id, "error", err)
	}
}

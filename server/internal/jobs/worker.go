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
// check the cache, explain, fan the scenes out, stitch, upload, done.
//
// Fake mode is not a separate pipeline. FAKE_AGENT swaps only the two model
// calls for canned answers and FAKE_RENDER swaps only the render, so the server
// P4 develops against walks exactly the code path a real job walks. Both flags
// default on and are deleted in Sprint 4.
type Worker struct {
	cfg   *config.Config
	jobs  Store
	cache CacheStore
	agent *agent.Client

	// concat is P2's render.Concat (FRD §14.1): it stitches the clips and
	// uploads the result, returning the public URL.
	concat ConcatFunc

	// renderScene, when set, overrides how a scene becomes a clip. Only tests
	// set it; a real job gets its SceneFunc from sceneFunc below.
	renderScene SceneFunc

	mu      sync.Mutex
	running map[string]*jobCtl

	// stepInterval is how long fake mode holds each status. One second matches
	// the desktop app's poll interval, so P4 sees every status exactly once.
	// Tests shorten it.
	stepInterval time.Duration
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

func NewWorker(cfg *config.Config, j Store, c CacheStore, a *agent.Client, concat ConcatFunc) *Worker {
	return &Worker{
		cfg:          cfg,
		jobs:         j,
		cache:        c,
		agent:        a,
		concat:       concat,
		running:      make(map[string]*jobCtl),
		stepInterval: time.Second,
	}
}

// sceneFunc decides how this job's scenes become clips. FAKE_AGENT gates it,
// not FAKE_RENDER: /scenes/render is an agent endpoint, and the rendering it
// triggers happens back inside /internal/render, which is always real.
func (w *Worker) sceneFunc(j *Job, storyboardTitle, category string) SceneFunc {
	if w.renderScene != nil {
		return w.renderScene // test override
	}
	if w.cfg.FakeAgent {
		return FakeSceneFunc
	}
	return AgentSceneFunc(w.agent, w.cfg, j.ID, storyboardTitle, category, j.Guardrails)
}

// Start runs the pipeline for one job in its own goroutine and returns at once.
// POST /api/jobs must not block on anything (FRD §23 rule 7).
func (w *Worker) Start(j *Job, imageB64, mediaType string) {
	ctx, cancel := context.WithCancel(context.Background())
	ctl := &jobCtl{ctx: ctx, cancel: cancel}
	w.mu.Lock()
	w.running[j.ID] = ctl
	w.mu.Unlock()

	go func() {
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
	ErrNoProblemFound  = "no_problem_found"
	ErrExplainFailed   = "explain_failed"
	ErrInternal        = "internal"
	ErrAllScenesFailed = "all_scenes_failed"
)

// run is the pipeline for one job.
func (w *Worker) run(ctx context.Context, j *Job, imageB64, mediaType string) {
	log := slog.With("job_id", j.ID)
	log.Info("pipeline started",
		"fake_agent", w.cfg.FakeAgent,
		"fake_render", w.cfg.FakeRender,
		"guardrails", j.Guardrails,
		"quality", w.cfg.ManimQuality,
	)

	// --- transcribing: read the problem off the screenshot ---
	if !w.setStatus(ctx, j.ID, StatusTranscribing) {
		return
	}
	vision, err := w.transcribe(ctx, imageB64, mediaType, j.Guardrails)
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

	// --- rendering: every scene at once ---
	if !w.setStatus(ctx, j.ID, StatusRendering) {
		return
	}
	clips := w.fanOut(ctx, j.ID, scenes, w.sceneFunc(j, explained.Storyboard.Title, vision.Category), func(done int) {
		w.update(ctx, j.ID, Fields{"scenes_done": done})
	})
	if ctx.Err() != nil {
		return
	}
	log.Info("scenes finished", "surviving", len(clips), "total", len(scenes))

	// --- concatenating, uploading ---
	if !w.setStatus(ctx, j.ID, StatusConcatenating) {
		return
	}
	if len(clips) == 0 {
		// The explanation survives; the animation quietly did not. Not a
		// failure, and nothing is cached.
		log.Warn("every scene was dropped; finishing without a video")
		w.update(ctx, j.ID, Fields{"status": StatusDone, "video_url": nil, "error": ErrAllScenesFailed})
		return
	}

	// Concat does the upload too, so it runs under `concatenating`; `uploading`
	// is set afterwards, and exists for the UI.
	videoURL, err := w.concatAndUpload(ctx, clips, hash)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("concat or upload failed", "error", err)
		w.update(ctx, j.ID, Fields{"status": StatusDone, "video_url": nil, "error": ErrInternal})
		return
	}
	if !w.setStatus(ctx, j.ID, StatusUploading) {
		return
	}
	// Only the cache write separates `uploading` from `done`, so in fake mode
	// hold it for a poll interval — otherwise P4 has a status they can never
	// see on screen. In a real run it is genuinely near-instantaneous, because
	// Concat has already done the uploading; see the note in P3_BACKEND
	// Sprint 3 Step 2 ("status is for the UI").
	if w.cfg.FakeAgent && !w.sleep(ctx) {
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

func (w *Worker) transcribe(ctx context.Context, imageB64, mediaType string, guardrails bool) (*agent.VisionResponse, error) {
	if w.cfg.FakeAgent {
		if !w.sleep(ctx) {
			return nil, ctx.Err()
		}
		return fakeVision(), nil
	}
	return w.agent.Vision(ctx, imageB64, mediaType, guardrails)
}

func (w *Worker) explain(ctx context.Context, req agent.ExplainRequest) (*agent.ExplainResponse, error) {
	if w.cfg.FakeAgent {
		if !w.sleep(ctx) {
			return nil, ctx.Err()
		}
		return fakeExplain(), nil
	}
	return w.agent.Explain(ctx, req)
}

// concatAndUpload stitches the surviving clips into one video and uploads it,
// returning the public URL. Both halves are P2's render.Concat; the key is the
// problem hash, so the same problem always lands at the same object.
func (w *Worker) concatAndUpload(ctx context.Context, clips []string, hash string) (string, error) {
	return w.concat(ctx, clips, "renders/"+hash+".mp4")
}

// --- job record helpers ---------------------------------------------------

// setStatus advances the job, pausing first in fake mode so each status stays
// visible for a poll interval.
func (w *Worker) setStatus(ctx context.Context, id string, status Status) bool {
	if w.cfg.FakeAgent && !w.sleep(ctx) {
		return false
	}
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

// sleep waits one step unless the job is cancelled first.
func (w *Worker) sleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(w.stepInterval):
		return true
	}
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

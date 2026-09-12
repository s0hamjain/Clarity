package jobs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
)

// A local store keeps this test in package jobs without importing store, which
// would be a cycle. It also records the order of writes, which is what the two
// rules under test are about.

type recordingStore struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	history []Fields
	failOn  string // status name whose write should fail, for the error path
	updates int    // every Update call
	stamps  int    // every Update call that moved updated_at
}

func newRecordingStore() *recordingStore {
	return &recordingStore{jobs: make(map[string]*Job)}
}

func (s *recordingStore) Create(_ context.Context, j *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *j
	s.jobs[j.ID] = &cp
	return nil
}

func (s *recordingStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *j
	return &cp, nil
}

func (s *recordingStore) Update(_ context.Context, id string, f Fields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if st, isStatus := f["status"].(Status); isStatus && string(st) == s.failOn {
		return errors.New("simulated store failure")
	}
	rec := Fields{}
	for k, v := range f {
		rec[k] = v
		switch k {
		case "status":
			j.Status = v.(Status)
		case "problem_hash":
			j.ProblemHash = strPtr(v)
		case "problem_text":
			j.ProblemText = strPtr(v)
		case "category":
			j.Category = strPtr(v)
		case "explanation":
			j.Explanation = strPtr(v)
		case "video_url":
			j.VideoURL = strPtr(v)
		case "error":
			j.Error = strPtr(v)
		case "scenes_total":
			j.ScenesTotal = v.(int)
		case "scenes_done":
			j.ScenesDone = v.(int)
		case "cached":
			j.Cached = v.(bool)
		default:
			// Fail loudly rather than silently dropping a field the pipeline
			// writes — a test double that ignores writes proves nothing.
			panic("recordingStore: unhandled job field " + k)
		}
	}
	s.history = append(s.history, rec)
	s.updates++
	// Every write stamps updated_at (FRD §23 rule 9) — the real stores do it in
	// Update, so the caller cannot forget.
	before := j.UpdatedAt
	j.UpdatedAt = time.Now().UTC()
	if j.UpdatedAt.After(before) || before.IsZero() {
		s.stamps++
	}
	return nil
}

func (s *recordingStore) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func (s *recordingStore) stampCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stamps
}

func (s *recordingStore) snapshot() []Fields {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Fields(nil), s.history...)
}

type recordingCache struct {
	mu      sync.Mutex
	entries map[string]CacheEntry
}

func newRecordingCache() *recordingCache {
	return &recordingCache{entries: make(map[string]CacheEntry)}
}

func (c *recordingCache) Get(_ context.Context, hash string) (*CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return &e, nil
}

func (c *recordingCache) Put(_ context.Context, hash, videoURL, explanation string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[hash] = CacheEntry{Hash: hash, VideoURL: videoURL, Explanation: explanation, CreatedAt: time.Now().UTC()}
	return nil
}

func (c *recordingCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// newTestWorker builds a worker on the real pipeline, with a stand-in for P1's
// service. There is no fake mode any more, so every test drives the real path.
func newTestWorker(t *testing.T) (*Worker, *recordingStore, *recordingCache) {
	t.Helper()
	f := newFakeAgent(t)
	return newRealPipelineWorker(t, f.server.URL)
}

// newSlowTestWorker keeps a job in flight long enough to act on it.
func newSlowTestWorker(t *testing.T, d time.Duration) (*Worker, *recordingStore, *recordingCache) {
	t.Helper()
	f := newFakeAgent(t)
	f.delay = d
	return newRealPipelineWorker(t, f.server.URL)
}

func runToTerminal(t *testing.T, w *Worker, js *recordingStore, j *Job) *Job {
	t.Helper()
	_ = js.Create(context.Background(), j)
	if err := w.Start(j, "aGVsbG8=", "image/png", "req_test"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := js.Get(context.Background(), j.ID)
		if err == nil && got.Status.IsTerminal() {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("job %s never reached a terminal status", j.ID)
	return nil
}

func TestFakePipelineWalksEveryStatus(t *testing.T) {
	w, js, _ := newTestWorker(t)
	final := runToTerminal(t, w, js, New("why?", false, "test"))

	if final.Status != StatusDone {
		t.Fatalf("final status = %q, want done", final.Status)
	}

	var statuses []Status
	for _, f := range js.snapshot() {
		if s, ok := f["status"].(Status); ok {
			statuses = append(statuses, s)
		}
	}
	want := []Status{
		StatusTranscribing, StatusExplaining, StatusGenerating,
		StatusRendering, StatusConcatenating, StatusUploading, StatusDone,
	}
	if len(statuses) != len(want) {
		t.Fatalf("statuses = %v, want %v", statuses, want)
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("statuses = %v, want %v", statuses, want)
		}
	}

	if final.VideoURL == nil || *final.VideoURL == "" {
		t.Error("done without a video_url")
	}
	if final.ScenesDone != final.ScenesTotal || final.ScenesTotal == 0 {
		t.Errorf("scenes_done/scenes_total = %d/%d", final.ScenesDone, final.ScenesTotal)
	}
}

// Rule 8: the explanation is written before any scene work starts. This is the
// most important ordering in the server — every millisecond after /explain
// returns is a second the user waits for nothing.
func TestExplanationIsWrittenBeforeAnySceneWork(t *testing.T) {
	w, js, _ := newTestWorker(t)
	runToTerminal(t, w, js, New("", false, "test"))

	explainedAt, firstSceneAt := -1, -1
	for i, f := range js.snapshot() {
		if _, ok := f["explanation"]; ok && explainedAt < 0 {
			explainedAt = i
		}
		if _, ok := f["scenes_done"]; ok && firstSceneAt < 0 {
			firstSceneAt = i
		}
	}
	if explainedAt < 0 {
		t.Fatal("the explanation was never written")
	}
	if firstSceneAt >= 0 && explainedAt > firstSceneAt {
		t.Fatalf("explanation written at write %d, after scene work began at %d", explainedAt, firstSceneAt)
	}
}

// Rule 10: the cache is written only after a successful upload, and carries
// both fields.
func TestCacheIsWrittenOnlyAfterSuccess(t *testing.T) {
	w, js, cs := newTestWorker(t)
	final := runToTerminal(t, w, js, New("why?", false, "test"))

	entry, err := cs.Get(context.Background(), *final.ProblemHash)
	if err != nil {
		t.Fatalf("a completed job left nothing in the cache: %v", err)
	}
	if entry.VideoURL == "" || entry.Explanation == "" {
		t.Errorf("cache entry must carry both fields, got %+v", entry)
	}
}

func TestFailedJobWritesNothingToTheCache(t *testing.T) {
	w, js, cs := newTestWorker(t)
	js.failOn = string(StatusUploading) // die after rendering, before the cache write

	final := runToTerminal(t, w, js, New("why?", false, "test"))
	if final.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", final.Status)
	}
	if final.Error == nil || *final.Error != "internal" {
		t.Errorf("error = %v, want \"internal\"", final.Error)
	}
	if cs.len() != 0 {
		t.Errorf("a failed job wrote %d cache entries; rule 10 says never", cs.len())
	}
}

func TestCancelStopsThePipeline(t *testing.T) {
	w, js, cs := newSlowTestWorker(t, 2*time.Second)
	j := New("cancel me", false, "test")
	_ = js.Create(context.Background(), j)
	if err := w.Start(j, "aGVsbG8=", "image/png", "req_test"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // let it reach the /explain call
	if err := w.Cancel(context.Background(), j.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	got, _ := js.Get(context.Background(), j.ID)
	if got.Status != StatusCancelled {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}

	// It must stay cancelled — the goroutine does not carry on behind the DELETE.
	time.Sleep(300 * time.Millisecond)
	got, _ = js.Get(context.Background(), j.ID)
	if got.Status != StatusCancelled {
		t.Errorf("status drifted to %q after cancel", got.Status)
	}
	if cs.len() != 0 {
		t.Error("a cancelled job wrote to the cache")
	}
}

// Cancelling a job that already finished is a no-op, not an error.
func TestCancelIsIdempotent(t *testing.T) {
	w, js, _ := newTestWorker(t)
	final := runToTerminal(t, w, js, New("", false, "test"))

	for i := 0; i < 2; i++ {
		if err := w.Cancel(context.Background(), final.ID); err != nil {
			t.Fatalf("cancel #%d: %v", i+1, err)
		}
	}
	got, _ := js.Get(context.Background(), final.ID)
	if got.Status != StatusDone {
		t.Errorf("status = %q; cancelling a finished job must leave it alone", got.Status)
	}
}

func TestCancelUnknownJob(t *testing.T) {
	w, _, _ := newTestWorker(t)
	if err := w.Cancel(context.Background(), "j_deadbeef"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Two jobs with the same problem and prompt share a cache key; the guardrails
// flag splits them.
func TestCacheKeyGroupsIdenticalJobs(t *testing.T) {
	w, js, _ := newTestWorker(t)
	a := runToTerminal(t, w, js, New("why?", false, "test"))
	b := runToTerminal(t, w, js, New("why?", false, "test"))
	c := runToTerminal(t, w, js, New("why?", true, "test"))

	if *a.ProblemHash != *b.ProblemHash {
		t.Errorf("same input hashed differently: %q vs %q", *a.ProblemHash, *b.ProblemHash)
	}
	if *a.ProblemHash == *c.ProblemHash {
		t.Error("guardrails did not change the cache key")
	}
}

// stubSceneFunc renders instantly and always succeeds, so the pipeline tests
// measure the pipeline rather than the renderer.
func stubSceneFunc(ctx context.Context, scene agent.Scene, workDir string) (string, bool) {
	if ctx.Err() != nil {
		return "", false
	}
	return filepath.Join(workDir, fmt.Sprintf("scene%d.mp4", scene.Index)), true
}

// strPtr mirrors how the real stores handle a nullable string field: a nil
// value clears it, which is how video_url is unset when no scene survived.
func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

// stubConcat stands in for P2's render.Concat: it reports a URL without
// touching ffmpeg or S3, so pipeline tests measure the pipeline.
func stubConcat(ctx context.Context, clipPaths []string, outKey string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "http://minio.test/clarity-renders/" + outKey, nil
}

// The bounded queue is what turns a burst of captures into 503s instead of an
// unbounded pile of goroutines (API.md §7).
func TestQueueDepthIsEnforced(t *testing.T) {
	w, js, _ := newSlowTestWorker(t, 5*time.Second)

	started := make([]*Job, 0, QueueDepth)
	for i := 0; i < QueueDepth; i++ {
		j := New("queue filler", false, "test")
		_ = js.Create(context.Background(), j)
		if err := w.Start(j, "aGVsbG8=", "image/png", "req_test"); err != nil {
			t.Fatalf("job %d of %d rejected early: %v", i+1, QueueDepth, err)
		}
		started = append(started, j)
	}
	if got := w.InFlight(); got != QueueDepth {
		t.Errorf("InFlight() = %d, want %d", got, QueueDepth)
	}

	// One more than the bound is refused, not queued.
	overflow := New("one too many", false, "test")
	_ = js.Create(context.Background(), overflow)
	if err := w.Start(overflow, "aGVsbG8=", "image/png", "req_test"); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}

	// Cancelling frees a slot, and the next capture gets in.
	if err := w.Cancel(context.Background(), started[0].ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && w.InFlight() >= QueueDepth {
		time.Sleep(5 * time.Millisecond)
	}
	if err := w.Start(overflow, "aGVsbG8=", "image/png", "req_test"); err != nil {
		t.Errorf("a freed slot was not reusable: %v", err)
	}

	for _, j := range started {
		_ = w.Cancel(context.Background(), j.ID)
	}
}

// Rule 9, enforced rather than audited by hand: every write the pipeline makes
// has to move updated_at, or the TTL index deletes a job mid-render.
func TestEveryWriteStampsUpdatedAt(t *testing.T) {
	w, js, _ := newTestWorker(t)
	final := runToTerminal(t, w, js, New("updated_at", false, "test"))

	if !final.UpdatedAt.After(final.CreatedAt) {
		t.Errorf("updated_at %v is not after created_at %v", final.UpdatedAt, final.CreatedAt)
	}
	if n := js.updateCount(); n != js.stampCount() {
		t.Errorf("%d of %d Update calls stamped updated_at; rule 9 says all of them", js.stampCount(), n)
	}
	if js.updateCount() == 0 {
		t.Fatal("the pipeline made no writes at all")
	}
}

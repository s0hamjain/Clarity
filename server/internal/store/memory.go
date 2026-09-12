package store

import (
	"context"
	"sync"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
)

// The in-memory store exists for one reason: `FAKE_AGENT=1 FAKE_RENDER=1` has
// to walk a job through every status with no Atlas, no Docker and no API keys,
// because that is the server P4 builds the desktop UI against in Sprints 1–2.
// It is not a general fallback — anything real is backed by Atlas.

type memJobs struct {
	mu sync.RWMutex
	m  map[string]jobs.Job
}

func NewMemoryJobs() jobs.Store { return &memJobs{m: make(map[string]jobs.Job)} }

func (s *memJobs) Create(_ context.Context, j *jobs.Job) error {
	now := time.Now().UTC()
	j.CreatedAt, j.UpdatedAt = now, now
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[j.ID] = *j
	return nil
}

func (s *memJobs) Get(_ context.Context, id string) (*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.m[id]
	if !ok {
		return nil, jobs.ErrNotFound
	}
	return &j, nil
}

// Update mirrors the Mongo implementation, including the updated_at stamp, so
// the two behave identically to everything above them.
func (s *memJobs) Update(_ context.Context, id string, fields jobs.Fields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[id]
	if !ok {
		return jobs.ErrNotFound
	}
	for k, v := range fields {
		applyField(&j, k, v)
	}
	j.UpdatedAt = time.Now().UTC()
	s.m[id] = j
	return nil
}

// applyField maps a bson field name onto the struct. The mongo driver does this
// by reflection over the bson tags; here the set of writable fields is small and
// fixed, so an explicit switch is clearer and fails loudly on a typo.
func applyField(j *jobs.Job, key string, v any) {
	switch key {
	case "status":
		j.Status = v.(jobs.Status)
	case "problem_hash":
		j.ProblemHash = toStrPtr(v)
	case "problem_text":
		j.ProblemText = toStrPtr(v)
	case "category":
		j.Category = toStrPtr(v)
	case "explanation":
		j.Explanation = toStrPtr(v)
	case "video_url":
		j.VideoURL = toStrPtr(v)
	case "error":
		j.Error = toStrPtr(v)
	case "scenes_total":
		j.ScenesTotal = v.(int)
	case "scenes_done":
		j.ScenesDone = v.(int)
	case "cached":
		j.Cached = v.(bool)
	case "updated_at":
		// owned by Update, never by the caller
	default:
		panic("store: unknown job field " + key)
	}
}

func toStrPtr(v any) *string {
	switch s := v.(type) {
	case nil:
		return nil
	case *string:
		return s
	case string:
		return &s
	}
	panic("store: field is not a string")
}

type memCache struct {
	mu sync.RWMutex
	m  map[string]jobs.CacheEntry
}

func NewMemoryCache() jobs.CacheStore { return &memCache{m: make(map[string]jobs.CacheEntry)} }

func (s *memCache) Get(_ context.Context, hash string) (*jobs.CacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[hash]
	if !ok {
		return nil, jobs.ErrNotFound
	}
	return &e, nil
}

func (s *memCache) Put(_ context.Context, hash, videoURL, explanation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[hash] = jobs.CacheEntry{Hash: hash, VideoURL: videoURL, Explanation: explanation, CreatedAt: time.Now().UTC()}
	return nil
}

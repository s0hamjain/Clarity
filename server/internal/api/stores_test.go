package api

import (
	"context"
	"sync"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
)

// In-memory stores for the handler tests. These live here rather than in the
// store package because production has one backend now — Atlas — and a
// memory implementation shipping alongside it is an invitation to run on it
// by accident.

type memJobs struct {
	mu sync.RWMutex
	m  map[string]jobs.Job
}

func newMemJobs() *memJobs { return &memJobs{m: make(map[string]jobs.Job)} }

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

func (s *memJobs) Update(_ context.Context, id string, fields jobs.Fields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[id]
	if !ok {
		return jobs.ErrNotFound
	}
	for k, v := range fields {
		switch k {
		case "status":
			j.Status = v.(jobs.Status)
		case "error":
			j.Error = strPtr(v)
		case "explanation":
			j.Explanation = strPtr(v)
		case "video_url":
			j.VideoURL = strPtr(v)
		}
	}
	// The real store stamps this inside Update so no caller can forget it
	// (FRD §23 rule 9); the double has to behave the same way.
	j.UpdatedAt = time.Now().UTC()
	s.m[id] = j
	return nil
}

func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

type memCache struct {
	mu sync.RWMutex
	m  map[string]jobs.CacheEntry
}

func newMemCache() *memCache { return &memCache{m: make(map[string]jobs.CacheEntry)} }

func (c *memCache) Get(_ context.Context, hash string) (*jobs.CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.m[hash]
	if !ok {
		return nil, jobs.ErrNotFound
	}
	return &e, nil
}

func (c *memCache) Put(_ context.Context, hash, videoURL, explanation string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[hash] = jobs.CacheEntry{Hash: hash, VideoURL: videoURL, Explanation: explanation, CreatedAt: time.Now().UTC()}
	return nil
}

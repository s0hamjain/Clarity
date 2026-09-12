package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// The persistence interfaces live here, next to their only consumer, and the
// store package implements them. Declaring them the other way round would make
// `store` and `jobs` import each other.

// ErrNotFound is returned by every Get when the document does not exist — an
// unknown job, a job the TTL index already expired, or a cache miss.
var ErrNotFound = errors.New("store: not found")

// Store is the `jobs` collection (FRD §9.1).
type Store interface {
	Create(ctx context.Context, j *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	// Update applies fields to one job and always sets updated_at = now
	// (FRD §23 rule 9). A job that stops being touched during a long render is
	// deleted by the TTL index mid-flight, and the desktop app sees a 404.
	Update(ctx context.Context, id string, fields Fields) error
}

// CacheStore is the `cache` collection (FRD §9.2), keyed by problem_hash.
type CacheStore interface {
	Get(ctx context.Context, hash string) (*CacheEntry, error)
	// Put is called only after a render AND an upload have both succeeded
	// (FRD §23 rule 10). Never on any failure path.
	Put(ctx context.Context, hash, videoURL, explanation string) error
}

// Fields is a partial update to a job document. Keys are bson field names.
type Fields map[string]any

// CacheEntry is the `cache` document. A hit returns both fields — a video with
// nothing to read while it loads defeats the point.
type CacheEntry struct {
	Hash        string    `bson:"_id"         json:"problem_hash"`
	VideoURL    string    `bson:"video_url"   json:"video_url"`
	Explanation string    `bson:"explanation" json:"explanation"`
	CreatedAt   time.Time `bson:"created_at"  json:"-"`
}

// MarshalJSON renders the entry as API.md §2.5 specifies.
func (e CacheEntry) MarshalJSON() ([]byte, error) {
	type entry CacheEntry
	return json.Marshal(struct {
		entry
		CreatedAt string `json:"created_at"`
	}{entry: entry(e), CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339)})
}

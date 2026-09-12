// Package jobs owns the job record and the pipeline that drives one job from a
// screenshot to a video.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Status values and their guarantees are API.md §5 / FRD §11.2.
type Status string

const (
	StatusQueued        Status = "queued"
	StatusTranscribing  Status = "transcribing"
	StatusExplaining    Status = "explaining"
	StatusGenerating Status = "generating"
	StatusRendering  Status = "rendering"
	StatusUploading  Status = "uploading"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// IsTerminal reports whether the desktop app should stop polling.
func (s Status) IsTerminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusCancelled
}

// Job is the `jobs` document, FRD §9.1. Fields that are not yet known are
// pointers so they serialize as JSON null rather than being omitted — clients
// rely on key presence (API.md §1).
type Job struct {
	ID          string  `bson:"_id"          json:"job_id"`
	Status      Status  `bson:"status"       json:"status"`
	ProblemHash *string `bson:"problem_hash" json:"problem_hash"`
	ProblemText *string `bson:"problem_text" json:"problem_text"`
	Category    *string `bson:"category"     json:"category"`
	Explanation *string `bson:"explanation"  json:"explanation"`
	// ScenesTotal is the number of beats in the storyboard; informational
	// only — the whole storyboard renders as one continuous script, not
	// scene by scene, so there is no "done so far" count to report.
	ScenesTotal int     `bson:"scenes_total" json:"scenes_total"`
	VideoURL    *string `bson:"video_url"    json:"video_url"`
	Cached      bool    `bson:"cached"       json:"cached"`
	Guardrails  bool    `bson:"guardrails"   json:"guardrails"`
	Error       *string `bson:"error"        json:"error"`
	Source      string  `bson:"source"       json:"source"`

	CreatedAt time.Time `bson:"created_at" json:"-"`
	UpdatedAt time.Time `bson:"updated_at" json:"-"`

	// UserPrompt is part of the cache key but is never returned to the client.
	UserPrompt string `bson:"user_prompt" json:"-"`
}

// job is a distinct type with no MarshalJSON of its own, so embedding it below
// does not re-enter Job.MarshalJSON and recurse forever.
type job Job

// jobJSON adds the RFC 3339 UTC timestamps (API.md §1) to the wire shape.
type jobJSON struct {
	job
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MarshalJSON renders the job exactly as API.md §2.2 specifies.
func (j Job) MarshalJSON() ([]byte, error) {
	return json.Marshal(jobJSON{
		job:       job(j),
		CreatedAt: j.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: j.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

// New creates a queued job. Nothing else about it is known yet.
func New(userPrompt string, guardrails bool, source string) *Job {
	now := time.Now().UTC()
	return &Job{
		ID:         NewID(),
		Status:     StatusQueued,
		Guardrails: guardrails,
		Source:     source,
		UserPrompt: userPrompt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// NewID returns a job ID: "j_" + 8 hex chars (API.md §1).
func NewID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on any platform we run on; fall back to the
		// clock rather than panicking inside a request.
		return "j_" + hex.EncodeToString([]byte(time.Now().UTC().Format("150405.0")))[:8]
	}
	return "j_" + hex.EncodeToString(b)
}

// Str is a helper for the pointer fields.
func Str(s string) *string { return &s }

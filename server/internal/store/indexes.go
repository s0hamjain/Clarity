package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Retention, FRD §9.1–9.2. A job outlives one render many times over; a cache
// entry outlives a job so a problem asked yesterday still skips the pipeline.
const (
	JobTTL   = 24 * time.Hour
	CacheTTL = 7 * 24 * time.Hour
)

// EnsureIndexes creates the two TTL indexes from FRD §9.1–9.2:
//
//	jobs.updated_at   expireAfterSeconds 86400   (24 h)
//	cache.created_at  expireAfterSeconds 604800  (7 d)
//
// Idempotent: CreateOne with the same spec is a no-op, so this runs at every
// boot. Called before the HTTP server starts listening.
func EnsureIndexes(ctx context.Context, m *Mongo) error {
	specs := []struct {
		coll    string
		field   string
		seconds int32
	}{
		{"jobs", "updated_at", int32(JobTTL.Seconds())},
		{"cache", "created_at", int32(CacheTTL.Seconds())},
	}
	for _, s := range specs {
		model := mongo.IndexModel{
			Keys:    bson.D{{Key: s.field, Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(s.seconds),
		}
		if _, err := m.DB.Collection(s.coll).Indexes().CreateOne(ctx, model); err != nil {
			return fmt.Errorf("create TTL index on %s.%s: %w", s.coll, s.field, err)
		}
	}
	return nil
}

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/s0hamjain/Clarity/server/internal/jobs"
)

type mongoJobs struct{ coll *mongo.Collection }

// NewJobs returns the Atlas-backed `jobs` collection.
func NewJobs(m *Mongo) jobs.Store { return &mongoJobs{coll: m.DB.Collection("jobs")} }

func (s *mongoJobs) Create(ctx context.Context, j *jobs.Job) error {
	now := time.Now().UTC()
	j.CreatedAt, j.UpdatedAt = now, now
	if _, err := s.coll.InsertOne(ctx, j); err != nil {
		return fmt.Errorf("insert job %s: %w", j.ID, err)
	}
	return nil
}

func (s *mongoJobs) Get(ctx context.Context, id string) (*jobs.Job, error) {
	var j jobs.Job
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&j)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, jobs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", id, err)
	}
	return &j, nil
}

// Update always stamps updated_at (FRD §23 rule 9) — the caller cannot forget.
func (s *mongoJobs) Update(ctx context.Context, id string, fields jobs.Fields) error {
	set := bson.D{{Key: "updated_at", Value: time.Now().UTC()}}
	for k, v := range fields {
		if k == "updated_at" {
			continue // owned here, never by the caller
		}
		set = append(set, bson.E{Key: k, Value: v})
	}
	res, err := s.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: set}},
	)
	if err != nil {
		return fmt.Errorf("update job %s: %w", id, err)
	}
	if res.MatchedCount == 0 {
		return jobs.ErrNotFound
	}
	return nil
}

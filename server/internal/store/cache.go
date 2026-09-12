package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/jobs"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoCache struct{ coll *mongo.Collection }

// NewCache returns the Atlas-backed `cache` collection.
func NewCache(m *Mongo) jobs.CacheStore { return &mongoCache{coll: m.DB.Collection("cache")} }

func (s *mongoCache) Get(ctx context.Context, hash string) (*jobs.CacheEntry, error) {
	var e jobs.CacheEntry
	err := s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: hash}}).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, jobs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cache %s: %w", hash, err)
	}
	return &e, nil
}

func (s *mongoCache) Put(ctx context.Context, hash, videoURL, explanation string) error {
	e := jobs.CacheEntry{Hash: hash, VideoURL: videoURL, Explanation: explanation, CreatedAt: time.Now().UTC()}
	_, err := s.coll.ReplaceOne(ctx,
		bson.D{{Key: "_id", Value: hash}}, e,
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("put cache %s: %w", hash, err)
	}
	return nil
}

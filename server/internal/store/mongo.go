package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Mongo holds the Atlas client and the database handle everything else uses.
type Mongo struct {
	Client *mongo.Client
	DB     *mongo.Database
}

// Connect dials Atlas and verifies the connection with a ping.
func Connect(ctx context.Context, uri, dbName string) (*Mongo, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to atlas: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping atlas: %w", err)
	}
	return &Mongo{Client: client, DB: client.Database(dbName)}, nil
}

// Ping reports whether Atlas is reachable right now. Used by /healthz.
func (m *Mongo) Ping(ctx context.Context) error {
	return m.Client.Ping(ctx, nil)
}

func (m *Mongo) Close(ctx context.Context) error {
	return m.Client.Disconnect(ctx)
}

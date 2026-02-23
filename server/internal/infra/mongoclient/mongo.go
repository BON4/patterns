package mongoclient

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Client struct {
	Client *mongo.Client
	DB     *mongo.Database
}

func New(ctx context.Context, uri string, dbName string) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	cli, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, err
	}

	if err := cli.Ping(ctx, nil); err != nil {
		_ = cli.Disconnect(ctx)
		return nil, err
	}

	return &Client{Client: cli, DB: cli.Database(dbName)}, nil
}

func (c *Client) Close(ctx context.Context) error {
	return c.Client.Disconnect(ctx)
}

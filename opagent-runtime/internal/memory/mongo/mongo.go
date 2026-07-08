package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	defaultURI    = ""
	defaultDBName = "opagent"

	defaultTimeout = 5 * time.Second
	client         *mongo.Client
	db             *mongo.Database
)

type MongoOptions struct {
	URI    string
	DBName string
	// if true, will only initialize the client, not the database, default is false
	ClientOnly bool
}

func NewMongo(opts *MongoOptions) error {
	if opts.URI == "" {
		opts.URI = defaultURI
	}
	if opts.DBName == "" {
		opts.DBName = defaultDBName
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	if err != nil {
		fmt.Printf("NEW_MONGO_ERROR %s\n", err.Error())
		return err
	}

	err = mongoClient.Ping(ctx, readpref.Primary())
	if err != nil {
		fmt.Printf("NEW_MONGO_ERROR %s\n", err.Error())
		return err
	}

	if opts.ClientOnly {
		client = mongoClient
		return nil
	}

	mongoDatabase := mongoClient.Database(opts.DBName)
	db = mongoDatabase

	if err := ensureIndexes(ctx, db); err != nil {
		return err
	}

	return nil

}

// CloseMongoDB closes the MongoDB connection
func CloseMongoDB() error {
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.Disconnect(ctx)
	}
	return nil
}

// GetCollection returns a collection from the database
func GetCollection(collectionName string) *mongo.Collection {
	return db.Collection(collectionName)
}

func GetClient() *mongo.Client {
	return client
}

func GetDB() *mongo.Database {
	return db
}

func ensureIndexes(ctx context.Context, database *mongo.Database) error {
	indexes := []struct {
		Collection string
		Keys       bson.D
	}{
		{Collection: "agent_config", Keys: bson.D{{Key: "id", Value: 1}}},
		{Collection: "skill", Keys: bson.D{{Key: "id", Value: 1}}},
		{Collection: "tool_server", Keys: bson.D{{Key: "id", Value: 1}}},
		{Collection: "tool", Keys: bson.D{{Key: "id", Value: 1}}},
		// message history (cloud MessageStore)
		{Collection: "thread_message", Keys: bson.D{{Key: "threadID", Value: 1}, {Key: "seq", Value: 1}}},
		{Collection: "thread_message_counter", Keys: bson.D{{Key: "threadID", Value: 1}}},
	}

	for _, idx := range indexes {
		_, err := database.Collection(idx.Collection).Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    idx.Keys,
			Options: options.Index().SetUnique(true),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

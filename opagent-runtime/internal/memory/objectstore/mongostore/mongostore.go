package mongostore

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type Store struct {
	client     *mongo.Client
	db         *mongo.Database
	bucket     *gridfs.Bucket
	filesColl  *mongo.Collection
	bucketName string
}

func New(ctx context.Context, cfg *op.Config) (*Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	uri := strings.TrimSpace(cfg.ObjectStore.MongoDB.URI)
	dbName := strings.TrimSpace(cfg.ObjectStore.MongoDB.Database)
	bucketName := strings.TrimSpace(cfg.ObjectStore.MongoDB.GridFSBucket)

	if uri == "" {
		uri = strings.TrimSpace(cfg.MongoDB.URI)
	}
	if dbName == "" {
		dbName = strings.TrimSpace(cfg.MongoDB.Database)
	}
	if bucketName == "" {
		bucketName = "images"
	}
	if uri == "" {
		return nil, fmt.Errorf("objectStore.mongodb.uri (or top-level mongodb.uri) is required")
	}
	if dbName == "" {
		dbName = "opagent"
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	db := client.Database(dbName)
	bucket, err := gridfs.NewBucket(db, options.GridFSBucket().SetName(bucketName))
	if err != nil {
		return nil, fmt.Errorf("new gridfs bucket: %w", err)
	}

	filesColl := db.Collection(bucketName + ".files")
	return &Store{
		client:     client,
		db:         db,
		bucket:     bucket,
		filesColl:  filesColl,
		bucketName: bucketName,
	}, nil
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts *objectstore.PutOptions) (*objectstore.ObjectInfo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("empty key")
	}

	// Ensure uniqueness by filename: delete existing versions first.
	if err := s.deleteAllByFilename(ctx, key); err != nil {
		return nil, err
	}

	meta := bson.M{}
	if opts != nil {
		if opts.ContentType != "" {
			meta["contentType"] = opts.ContentType
		}
		if len(opts.Metadata) > 0 {
			meta["metadata"] = opts.Metadata
		}
	}

	uploadOpts := options.GridFSUpload().SetMetadata(meta)
	stream, err := s.bucket.OpenUploadStream(key, uploadOpts)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	cr := &countingReader{r: r}
	if _, err := io.Copy(stream, cr); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}

	info := &objectstore.ObjectInfo{
		Key:  key,
		Size: cr.n,
	}
	if opts != nil {
		info.ContentType = opts.ContentType
		info.Metadata = opts.Metadata
	}
	return info, nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, *objectstore.ObjectInfo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil, fmt.Errorf("empty key")
	}

	meta, size, err := s.latestMeta(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	stream, err := s.bucket.OpenDownloadStreamByName(key)
	if err != nil {
		return nil, nil, err
	}

	info := &objectstore.ObjectInfo{
		Key:      key,
		Size:     size,
		Metadata: meta.metadata,
	}
	info.ContentType = meta.contentType
	return stream, info, nil
}

func (s *Store) Stat(ctx context.Context, key string) (*objectstore.ObjectInfo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("empty key")
	}
	meta, size, err := s.latestMeta(ctx, key)
	if err != nil {
		return nil, err
	}
	return &objectstore.ObjectInfo{
		Key:         key,
		Size:        size,
		ContentType: meta.contentType,
		Metadata:    meta.metadata,
	}, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("empty key")
	}
	return s.deleteAllByFilename(ctx, key)
}

type fileMeta struct {
	contentType string
	metadata    map[string]string
}

func (s *Store) latestMeta(ctx context.Context, key string) (*fileMeta, int64, error) {
	var doc struct {
		Length   int64  `bson:"length"`
		Metadata bson.M `bson:"metadata"`
	}
	err := s.filesColl.FindOne(
		ctx,
		bson.M{"filename": key},
		options.FindOne().SetSort(bson.D{{Key: "uploadDate", Value: -1}}).SetProjection(bson.M{"length": 1, "metadata": 1}),
	).Decode(&doc)
	if err != nil {
		return nil, 0, err
	}

	m := &fileMeta{metadata: map[string]string{}}
	if doc.Metadata != nil {
		if ct, ok := doc.Metadata["contentType"].(string); ok {
			m.contentType = ct
		}
		if mm, ok := doc.Metadata["metadata"].(map[string]string); ok {
			m.metadata = mm
		} else if mm2, ok := doc.Metadata["metadata"].(bson.M); ok {
			for k, v := range mm2 {
				if vs, ok := v.(string); ok {
					m.metadata[k] = vs
				}
			}
		}
	}
	return m, doc.Length, nil
}

func (s *Store) deleteAllByFilename(ctx context.Context, key string) error {
	cur, err := s.filesColl.Find(ctx, bson.M{"filename": key}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return err
		}
		_ = s.bucket.Delete(doc.ID)
	}
	return cur.Err()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

package objectstore

import (
	"context"
	"io"
)

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	Metadata    map[string]string
}

type PutOptions struct {
	ContentType string
	Metadata    map[string]string
}

type Store interface {
	Put(ctx context.Context, key string, r io.Reader, opts *PutOptions) (*ObjectInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, *ObjectInfo, error)
	Stat(ctx context.Context, key string) (*ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

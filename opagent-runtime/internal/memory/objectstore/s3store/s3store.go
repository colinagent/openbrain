package s3store

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"
)

type Store struct {
	client *s3.Client
	bucket string
	prefix string
}

func New(ctx context.Context, cfg *op.S3ObjectStoreConfig) (*Store, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("objectStore.s3.bucket is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" || cfg.SessionToken != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken),
		))
	}

	if strings.TrimSpace(cfg.Endpoint) != "" {
		endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
		loadOpts = append(loadOpts, awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == s3.ServiceID {
					return aws.Endpoint{
						URL:               endpoint,
						HostnameImmutable: true,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
	})

	return &Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts *objectstore.PutOptions) (*objectstore.ObjectInfo, error) {
	k := s.fullKey(key)
	cr := &countingReader{r: r}

	in := &s3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    &k,
		Body:   cr,
	}
	if opts != nil {
		if opts.ContentType != "" {
			in.ContentType = aws.String(opts.ContentType)
		}
		if len(opts.Metadata) > 0 {
			in.Metadata = opts.Metadata
		}
	}

	out, err := s.client.PutObject(ctx, in)
	if err != nil {
		return nil, err
	}

	info := &objectstore.ObjectInfo{
		Key:  key,
		Size: cr.n,
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, "\"")
	}
	if opts != nil {
		info.ContentType = opts.ContentType
		info.Metadata = opts.Metadata
	}
	return info, nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, *objectstore.ObjectInfo, error) {
	k := s.fullKey(key)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &k,
	})
	if err != nil {
		return nil, nil, err
	}

	info := &objectstore.ObjectInfo{
		Key:      key,
		Metadata: out.Metadata,
	}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, "\"")
	}
	return out.Body, info, nil
}

func (s *Store) Stat(ctx context.Context, key string) (*objectstore.ObjectInfo, error) {
	k := s.fullKey(key)
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    &k,
	})
	if err != nil {
		return nil, err
	}
	info := &objectstore.ObjectInfo{
		Key:      key,
		Metadata: out.Metadata,
	}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, "\"")
	}
	return info, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	k := s.fullKey(key)
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &k,
	})
	return err
}

func (s *Store) fullKey(key string) string {
	key = strings.TrimLeft(key, "/")
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
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

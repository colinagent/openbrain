package fsstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"
)

type Store struct {
	baseDir string
}

func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

type metaFile struct {
	ContentType string            `json:"contentType,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts *objectstore.PutOptions) (*objectstore.ObjectInfo, error) {
	_ = ctx

	dst, metaPath, err := s.pathsForKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".openbrain-object-*")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	n, err := io.Copy(tmp, r)
	if err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}

	info := &objectstore.ObjectInfo{
		Key:  key,
		Size: n,
	}
	if opts != nil {
		info.ContentType = opts.ContentType
		info.Metadata = opts.Metadata
	}

	if opts != nil && (opts.ContentType != "" || len(opts.Metadata) > 0) {
		if err := writeMetaAtomic(metaPath, &metaFile{
			ContentType: opts.ContentType,
			Metadata:    opts.Metadata,
		}); err != nil {
			return nil, err
		}
	}

	return info, nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, *objectstore.ObjectInfo, error) {
	_ = ctx

	dst, metaPath, err := s.pathsForKey(key)
	if err != nil {
		return nil, nil, err
	}

	f, err := os.Open(dst)
	if err != nil {
		return nil, nil, err
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}

	meta, _ := readMeta(metaPath)
	info := &objectstore.ObjectInfo{
		Key:         key,
		Size:        st.Size(),
		ContentType: meta.ContentType,
		Metadata:    meta.Metadata,
	}
	return f, info, nil
}

func (s *Store) Stat(ctx context.Context, key string) (*objectstore.ObjectInfo, error) {
	_ = ctx

	dst, metaPath, err := s.pathsForKey(key)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}
	meta, _ := readMeta(metaPath)
	return &objectstore.ObjectInfo{
		Key:         key,
		Size:        st.Size(),
		ContentType: meta.ContentType,
		Metadata:    meta.Metadata,
	}, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_ = ctx

	dst, metaPath, err := s.pathsForKey(key)
	if err != nil {
		return err
	}
	_ = os.Remove(dst)
	_ = os.Remove(metaPath)
	return nil
}

func (s *Store) pathsForKey(key string) (filePath string, metaPath string, err error) {
	if s.baseDir == "" {
		return "", "", fmt.Errorf("fs object store baseDir is empty")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}

	clean := path.Clean("/" + key)
	if strings.HasPrefix(clean, "/..") {
		return "", "", fmt.Errorf("invalid key: %q", key)
	}
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return "", "", fmt.Errorf("invalid key: %q", key)
	}

	filePath = filepath.Join(s.baseDir, filepath.FromSlash(clean))
	metaPath = filePath + ".meta.json"
	return filePath, metaPath, nil
}

func writeMetaAtomic(metaPath string, meta *metaFile) error {
	if metaPath == "" {
		return fmt.Errorf("empty metaPath")
	}
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return fmt.Errorf("mkdir meta dir: %w", err)
	}

	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(metaPath), ".openbrain-meta-*")
	if err != nil {
		return fmt.Errorf("create temp meta: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write meta: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close meta: %w", err)
	}
	if err := os.Rename(tmpName, metaPath); err != nil {
		return fmt.Errorf("rename meta: %w", err)
	}
	return nil
}

func readMeta(metaPath string) (*metaFile, error) {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return &metaFile{}, err
	}
	var m metaFile
	if err := json.Unmarshal(b, &m); err != nil {
		return &metaFile{}, err
	}
	if m.Metadata == nil {
		m.Metadata = map[string]string{}
	}
	return &m, nil
}

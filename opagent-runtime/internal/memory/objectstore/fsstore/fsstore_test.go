package fsstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"
)

func TestFSStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	ctx := context.Background()
	key := "a/b/c.txt"
	data := []byte("hello world")

	putInfo, err := s.Put(ctx, key, bytes.NewReader(data), &objectstore.PutOptions{
		ContentType: "text/plain",
		Metadata:    map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if putInfo.Size != int64(len(data)) {
		t.Fatalf("Put size=%d want=%d", putInfo.Size, len(data))
	}

	statInfo, err := s.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statInfo.Size != int64(len(data)) {
		t.Fatalf("Stat size=%d want=%d", statInfo.Size, len(data))
	}
	if statInfo.ContentType != "text/plain" {
		t.Fatalf("Stat contentType=%q", statInfo.ContentType)
	}
	if statInfo.Metadata["k"] != "v" {
		t.Fatalf("Stat metadata=%v", statInfo.Metadata)
	}

	rc, getInfo, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Get data=%q want=%q", got, data)
	}
	if getInfo.ContentType != "text/plain" {
		t.Fatalf("Get contentType=%q", getInfo.ContentType)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(dir + "/a/b/c.txt"); err == nil {
		t.Fatalf("file still exists after Delete")
	}
}

package nodeindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const indexVersion = 1

type File struct {
	Version int      `json:"version"`
	Nodes   []Record `json:"nodes"`
}

type Record struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	URI  string `json:"uri"`
}

type Index struct {
	baseDir string
	path    string
	mu      sync.Mutex
	records map[string]Record
}

func Open(baseDir string) (*Index, error) {
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	if baseDir == "" || baseDir == "." {
		return &Index{records: map[string]Record{}}, nil
	}
	idx := &Index{
		baseDir: baseDir,
		path:    filepath.Join(baseDir, "index", "nodes.json"),
		records: map[string]Record{},
	}
	if err := idx.load(); err != nil {
		return nil, err
	}
	return idx, nil
}

func (idx *Index) Assign(node *op.OpNode) error {
	if idx == nil || node == nil {
		return nil
	}
	kind := op.NormalizeNodeKind(node.Kind)
	if kind == "" {
		return fmt.Errorf("node kind is required")
	}
	uri := strings.TrimSpace(node.URI)
	if uri == "" {
		return fmt.Errorf("node uri is required")
	}
	id := strings.TrimSpace(node.ID)
	if id == "" || !strings.HasPrefix(id, kind+"-") {
		return fmt.Errorf("deterministic node id is required")
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if existing, exists := idx.records[id]; exists && existing.URI != uri {
		return fmt.Errorf("id conflict: %s maps to multiple URIs: %s and %s", id, existing.URI, uri)
	}
	idx.records[id] = Record{ID: id, Kind: kind, URI: uri}
	if idx.path != "" {
		if err := idx.saveLocked(); err != nil {
			return err
		}
	}

	node.ID = id
	node.Kind = kind
	node.URI = uri
	return nil
}

func (idx *Index) Resolve(id string) (Record, bool) {
	if idx == nil {
		return Record{}, false
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	rec, ok := idx.records[strings.TrimSpace(id)]
	return rec, ok
}

func (idx *Index) load() error {
	raw, err := os.ReadFile(idx.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("decode node index %s: %w", idx.path, err)
	}
	for _, rec := range file.Nodes {
		rec = normalizeRecord(rec)
		if rec.ID == "" || rec.Kind == "" || rec.URI == "" {
			continue
		}
		idx.records[rec.ID] = rec
	}
	return nil
}

func (idx *Index) saveLocked() error {
	nodes := make([]Record, 0, len(idx.records))
	for _, rec := range idx.records {
		rec = normalizeRecord(rec)
		if rec.ID == "" || rec.Kind == "" || rec.URI == "" {
			continue
		}
		nodes = append(nodes, rec)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	raw, err := json.MarshalIndent(File{Version: indexVersion, Nodes: nodes}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(idx.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(idx.path), filepath.Base(idx.path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, idx.path)
}

func normalizeRecord(rec Record) Record {
	rec.ID = strings.TrimSpace(rec.ID)
	rec.Kind = op.NormalizeNodeKind(rec.Kind)
	rec.URI = strings.TrimSpace(rec.URI)
	return rec
}

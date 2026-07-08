package corecfg

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

// Host wraps the OpAgent ServerSession for calling back to the host.
type Host struct {
	Session *op.ServerSession
}

var (
	instance *Host
	once     sync.Once
	readyCh  = make(chan struct{})
)

// Init initialises the global Host and unblocks WaitReady.
func Init(session *op.ServerSession) {
	instance = &Host{Session: session}
	once.Do(func() { close(readyCh) })
}

// Get returns the global Host (may be nil before Init).
func Get() *Host { return instance }

// WaitReady blocks until Init has been called or ctx is cancelled.
func WaitReady(ctx context.Context) error {
	select {
	case <-readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetConfig fetches the full host config from the OpAgent host via config/get.
func (h *Host) GetConfig() (*op.Config, error) {
	if h == nil || h.Session == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	res, err := h.Session.OpNode(context.Background(), &op.OpNodeParams{
		OpCode: op.ConfigGet,
	})
	if err != nil {
		return nil, fmt.Errorf("host config get: %w", err)
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		return nil, fmt.Errorf("content is not JsonContent")
	}

	// config/get returns the full op.Config, including the hot-reloaded user section.
	var cfg op.Config
	if err := json.Unmarshal(jsonContent.Raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

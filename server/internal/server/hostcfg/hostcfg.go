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
	return h.GetConfigContext(context.Background())
}

func (h *Host) GetConfigContext(ctx context.Context) (*op.Config, error) {
	if h == nil || h.Session == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	res, err := h.Session.OpNode(ctx, &op.OpNodeParams{
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

func (h *Host) GetSystemConfig(ctx context.Context) (*op.SystemConfigResult, error) {
	if h == nil || h.Session == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	res, err := h.Session.OpNode(ctx, &op.OpNodeParams{OpCode: op.ConfigSystemGet})
	if err != nil {
		return nil, fmt.Errorf("host system config get: %w", err)
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		return nil, fmt.Errorf("content is not JsonContent")
	}
	var cfg op.SystemConfigResult
	if err := json.Unmarshal(jsonContent.Raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal system config: %w", err)
	}
	return &cfg, nil
}

func (h *Host) ListNodes(ctx context.Context) ([]*op.OpNode, error) {
	if h == nil || h.Session == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	res, err := h.Session.OpNode(ctx, &op.OpNodeParams{OpCode: op.OpNodeList})
	if err != nil {
		return nil, fmt.Errorf("host node list: %w", err)
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok {
		return nil, fmt.Errorf("content is not JsonContent")
	}
	var nodes []*op.OpNode
	if err := json.Unmarshal(jsonContent.Raw, &nodes); err != nil {
		return nil, fmt.Errorf("unmarshal node list: %w", err)
	}
	return nodes, nil
}

func (h *Host) CallAgent(
	ctx context.Context,
	opcode op.OpCode,
	meta op.Meta,
	content op.Content,
) (*op.OpAgentResult, error) {
	if h == nil || h.Session == nil {
		return nil, fmt.Errorf("host session is nil")
	}
	return h.Session.OpAgent(ctx, &op.OpAgentParams{
		OpCode:  opcode,
		Meta:    meta,
		Content: content,
	})
}

func (h *Host) ListActiveThreads(ctx context.Context) ([]op.ThreadRuntimeInfo, error) {
	result, err := h.CallAgent(ctx, op.OpThreadActiveList, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("host active thread list: %w", err)
	}
	jsonContent, ok := result.Content.(*op.JsonContent)
	if !ok {
		return nil, fmt.Errorf("content is not JsonContent")
	}
	var payload op.ThreadActiveList
	if err := json.Unmarshal(jsonContent.Raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal active thread list: %w", err)
	}
	return append([]op.ThreadRuntimeInfo(nil), payload.Threads...), nil
}

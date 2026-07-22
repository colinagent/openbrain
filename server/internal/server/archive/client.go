package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type CoreClient interface {
	GetSystemConfig(ctx context.Context) (*op.SystemConfigResult, error)
	ListActiveThreads(ctx context.Context) ([]op.ThreadRuntimeInfo, error)
	UpdateThreadMeta(ctx context.Context, params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error)
}

type SessionProvider interface {
	GetHostSession() *op.ServerSession
}

type hostCoreClient struct {
	sessions SessionProvider
}

func NewHostCoreClient(sessions SessionProvider) CoreClient {
	return &hostCoreClient{sessions: sessions}
}

func (c *hostCoreClient) session() (*op.ServerSession, error) {
	if c == nil || c.sessions == nil {
		return nil, fmt.Errorf("host session provider is nil")
	}
	session := c.sessions.GetHostSession()
	if session == nil {
		return nil, fmt.Errorf("host session is not initialized")
	}
	return session, nil
}

func (c *hostCoreClient) GetSystemConfig(ctx context.Context) (*op.SystemConfigResult, error) {
	session, err := c.session()
	if err != nil {
		return nil, err
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{OpCode: op.ConfigSystemGet})
	if err != nil {
		return nil, err
	}
	var cfg op.SystemConfigResult
	if err := decodeHostJSONContent(res.Content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *hostCoreClient) ListActiveThreads(ctx context.Context) ([]op.ThreadRuntimeInfo, error) {
	session, err := c.session()
	if err != nil {
		return nil, err
	}
	res, err := session.OpAgent(ctx, &op.OpAgentParams{OpCode: op.OpThreadActiveList})
	if err != nil {
		return nil, err
	}
	var payload op.ThreadActiveList
	if err := decodeHostJSONContent(res.Content, &payload); err != nil {
		return nil, err
	}
	return append([]op.ThreadRuntimeInfo(nil), payload.Threads...), nil
}

func (c *hostCoreClient) UpdateThreadMeta(ctx context.Context, params op.ThreadMetaUpdateParams) (*op.ThreadMeta, error) {
	session, err := c.session()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	res, err := session.OpNode(ctx, &op.OpNodeParams{
		OpCode:  op.OpThreadMetaUpdate,
		Content: &op.JsonContent{Raw: raw},
	})
	if err != nil {
		return nil, err
	}
	var meta op.ThreadMeta
	if err := decodeHostJSONContent(res.Content, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func decodeHostJSONContent(content op.Content, out any) error {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return fmt.Errorf("expected json content")
	}
	if err := json.Unmarshal(jsonContent.Raw, out); err != nil {
		return err
	}
	return nil
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

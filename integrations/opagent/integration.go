// Package opagent owns the OpenBrain product policy supplied to OpAgent
// Runtime. Runtime mechanisms remain in the upstream OpAgent module.
package opagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
	agentruntime "github.com/colinagent/opagent/opagent-runtime/runtime"
)

const (
	SessionPlaceholder = "{openbrain_session}"
	gbrainCloudNodeID  = "tools-gbrain-cloud"
	gbrainCloudURL     = "https://api.op-agent.com/brain/mcp"
)

// Dependencies are trusted product implementations. They must be assembled by
// the OpenBrain host, never from workspace or user-provided manifests.
type Dependencies struct {
	BaseDir string

	ConfigLoader            agentruntime.ConfigLoader
	ApprovalReviewer        agentruntime.ApprovalReviewer
	SessionToken            func(context.Context) (string, error)
	SystemServiceAuthorizer func(context.Context, *protocol.OpNode) error
	PromptAugmenter         func(context.Context, string, string, protocol.Meta) (string, error)
	ToolScope               func(context.Context, protocol.Meta, string, string, any, any) (any, error)
	CronPolicy              func(context.Context, protocol.OpCode, protocol.Meta, protocol.Content) error
	BackgroundServices      []agentruntime.BackgroundService
}

// RuntimeOptions compiles OpenBrain product policy into public Runtime hooks.
func RuntimeOptions(dependencies Dependencies) (agentruntime.Options, error) {
	baseDir, err := resolveBaseDir(dependencies.BaseDir)
	if err != nil {
		return agentruntime.Options{}, err
	}
	return agentruntime.Options{
		BaseDir:                 baseDir,
		ConfigLoader:            dependencies.ConfigLoader,
		ApprovalReviewer:        dependencies.ApprovalReviewer,
		HeaderResolver:          sessionHeaderResolver(dependencies.SessionToken),
		SystemServiceAuthorizer: failClosedSystemAuthorizer(dependencies.SystemServiceAuthorizer),
		RemoteNodeAuthorizer:    gbrainCloudAuthorizer(baseDir),
		PromptAugmenter:         dependencies.PromptAugmenter,
		ToolScope:               dependencies.ToolScope,
		CronPolicy:              dependencies.CronPolicy,
		BackgroundServices:      dependencies.BackgroundServices,
	}, nil
}

func resolveBaseDir(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("OPAGENT_BASE_DIR"))
	}
	if value == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", errors.New("resolve OpenBrain product base directory")
		}
		value = filepath.Join(home, ".opagent", "openbrain")
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("OpenBrain product base directory must be absolute")
	}
	return filepath.Clean(value), nil
}

func sessionHeaderResolver(token func(context.Context) (string, error)) func(context.Context, map[string]string) (map[string]string, error) {
	return func(ctx context.Context, headers map[string]string) (map[string]string, error) {
		resolved := make(map[string]string, len(headers))
		for name, value := range headers {
			if !strings.Contains(value, SessionPlaceholder) {
				resolved[name] = value
				continue
			}
			if token == nil {
				return nil, errors.New("OpenBrain session resolver is unavailable")
			}
			session, err := token(ctx)
			if err != nil {
				return nil, fmt.Errorf("resolve OpenBrain session: %w", err)
			}
			session = strings.TrimSpace(session)
			if session == "" {
				return nil, errors.New("OpenBrain session is empty")
			}
			resolved[name] = strings.ReplaceAll(value, SessionPlaceholder, session)
		}
		return resolved, nil
	}
}

func failClosedSystemAuthorizer(authorize func(context.Context, *protocol.OpNode) error) func(context.Context, *protocol.OpNode) error {
	return func(ctx context.Context, node *protocol.OpNode) error {
		if authorize == nil {
			return errors.New("OpenBrain system service is not authorized")
		}
		return authorize(ctx, node)
	}
}

func gbrainCloudAuthorizer(baseDir string) func(context.Context, *protocol.OpNode) error {
	expectedManifest := filepath.Join(baseDir, "tools", "gbrain-cloud", "TOOL.md")
	return func(_ context.Context, node *protocol.OpNode) error {
		if node == nil || strings.TrimSpace(node.ID) != gbrainCloudNodeID || strings.TrimSpace(node.Kind) != string(protocol.NodeKindTools) {
			return errors.New("remote node is not an OpenBrain-managed GBrain tool")
		}
		if len(node.Run.Command) != 0 || !node.Run.Daemon || strings.TrimSpace(node.Run.URL) != gbrainCloudURL {
			return errors.New("GBrain remote endpoint does not match product policy")
		}
		if len(node.Run.Header) != 1 || strings.TrimSpace(node.Run.Header["Authorization"]) != "Bearer "+SessionPlaceholder {
			return errors.New("GBrain remote endpoint headers do not match product policy")
		}
		manifest := filepath.Clean(protocol.URIToPath(strings.TrimSpace(node.URI)))
		if manifest != expectedManifest {
			return errors.New("GBrain remote endpoint is not installed at the managed product path")
		}
		if err := requireRegularFile(expectedManifest, false); err != nil {
			return fmt.Errorf("validate GBrain manifest: %w", err)
		}
		return nil
	}
}

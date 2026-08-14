package opagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	protocol "github.com/colinagent/opagent/opagent-protocol/go-sdk/op"
	agentruntime "github.com/colinagent/opagent/opagent-runtime/runtime"
	"github.com/colinagent/openbrain/integrations/opagent/guardian"
)

const managedServerAgentID = "agent-openbrain-server"

type sessionAuth struct {
	Version int    `json:"version"`
	Token   string `json:"token"`
}

// ProductRuntimeOptions assembles the OpenBrain-owned policies used by the
// product Runtime binary. Workspace configuration cannot grant these trusts.
func ProductRuntimeOptions(baseDir string) (agentruntime.Options, error) {
	resolved, err := resolveBaseDir(baseDir)
	if err != nil {
		return agentruntime.Options{}, err
	}
	return RuntimeOptions(Dependencies{
		BaseDir:                 resolved,
		ApprovalReviewer:        guardian.NewClient(resolved),
		SessionToken:            diskSessionToken(resolved),
		SystemServiceAuthorizer: managedServerAuthorizer(resolved),
		PromptAugmenter:         gbrainPromptAugmenter,
		ToolScope:               gbrainToolScope,
		CronPolicy:              openBrainCronPolicy,
		BackgroundServices:      []agentruntime.BackgroundService{NewHeartbeatService(resolved)},
	})
}

func diskSessionToken(baseDir string) func(context.Context) (string, error) {
	path := filepath.Join(baseDir, "configs", "user", "auth.json")
	return func(context.Context) (string, error) {
		if err := requireRegularFile(path, false); err != nil {
			return "", fmt.Errorf("validate OpenBrain session: %w", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read OpenBrain session: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		var auth sessionAuth
		if err := decoder.Decode(&auth); err != nil {
			return "", errors.New("OpenBrain auth config is invalid")
		}
		token := strings.TrimSpace(auth.Token)
		if auth.Version != 2 || token == "" {
			return "", errors.New("OpenBrain login is required")
		}
		return token, nil
	}
}

func managedServerAuthorizer(baseDir string) func(context.Context, *protocol.OpNode) error {
	return func(_ context.Context, node *protocol.OpNode) error {
		if node == nil || !slices.Contains(node.OpCodes, protocol.SystemStarted) {
			return errors.New("OpenBrain system service requires system/started")
		}
		if node.ID != managedServerAgentID || node.Kind != string(protocol.NodeKindAgent) {
			return errors.New("system/started is reserved for the product-managed server")
		}
		serviceRoot := filepath.Join(baseDir, "agents", "opagent-server")
		manifestPath := filepath.Join(serviceRoot, ".agent", "AGENT.md")
		if !sameSystemPath(protocol.URIToPath(node.URI), manifestPath) || !sameSystemPath(node.Cwd, serviceRoot) {
			return errors.New("system service manifest is outside the product-managed directory")
		}
		if err := requireRegularFile(manifestPath, false); err != nil {
			return fmt.Errorf("validate system service manifest: %w", err)
		}
		if !node.Run.Daemon || len(node.Run.Command) != 5 || node.Run.Command[1] != "--host" || node.Run.Command[2] != "127.0.0.1" || node.Run.Command[3] != "--port" {
			return errors.New("system service command does not match the OpenBrain control-plane shape")
		}
		port, err := strconv.Atoi(strings.TrimSpace(node.Run.Command[4]))
		if err != nil || port < 1 || port > 65535 {
			return errors.New("system service port is invalid")
		}
		executable := "openbrain-server"
		if runtime.GOOS == "windows" {
			executable += ".exe"
		}
		executablePath := filepath.Join(serviceRoot, ".agent", "bin", executable)
		if !sameSystemPath(node.Run.Command[0], executablePath) {
			return errors.New("system service executable is outside the product-managed directory")
		}
		if err := requireRegularFile(executablePath, runtime.GOOS != "windows"); err != nil {
			return fmt.Errorf("validate system service executable: %w", err)
		}
		return nil
	}
}

func sameSystemPath(left, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func requireRegularFile(path string, executable bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("path must be a regular file, not a symlink or special file")
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return errors.New("file is not executable")
	}
	return nil
}

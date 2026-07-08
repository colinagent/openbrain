package scan

import (
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

// ParseRun 从 yaml map 解析完整的 Run 结构
func ParseRun(raw map[string]any) (op.Run, error) {
	runRaw, ok := raw["run"]
	if !ok {
		return op.Run{}, nil
	}

	runMap, ok := runRaw.(map[string]any)
	if !ok {
		return op.Run{}, fmt.Errorf("invalid run config: run must be an object")
	}
	if _, ok := runMap["lifecycle"]; ok {
		return op.Run{}, fmt.Errorf("invalid run config: run.lifecycle has been removed; use run.daemon: true")
	}
	if _, ok := runMap["schedule"]; ok {
		return op.Run{}, fmt.Errorf("invalid run config: run.schedule has been removed; use tasks/tasks.json")
	}
	if _, ok := runMap["auth"]; ok {
		return op.Run{}, fmt.Errorf("invalid run config: run.auth has been removed; use run.header")
	}

	daemon, err := parseRunDaemon(runMap)
	if err != nil {
		return op.Run{}, fmt.Errorf("invalid run config: %w", err)
	}
	header, err := parseRunHeader(runMap)
	if err != nil {
		return op.Run{}, fmt.Errorf("invalid run config: %w", err)
	}

	run := op.Run{
		Command: getStringSlice(runMap, "command"),
		URL:     getString(runMap, "url"),
		Header:  header,
		Daemon:  daemon,
	}
	if err := run.Validate(); err != nil {
		return op.Run{}, fmt.Errorf("invalid run config: %w", err)
	}
	return run, nil
}

func parseRunDaemon(runMap map[string]any) (bool, error) {
	raw, ok := runMap["daemon"]
	if !ok {
		return false, nil
	}
	daemon, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("run.daemon must be a boolean")
	}
	return daemon, nil
}

func parseRunHeader(runMap map[string]any) (map[string]string, error) {
	raw, ok := runMap["header"]
	if !ok {
		return nil, nil
	}
	headerMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("run.header must be an object")
	}
	headers := make(map[string]string, len(headerMap))
	for key, value := range headerMap {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, fmt.Errorf("run.header contains an empty header name")
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("run.header.%s must be a string", name)
		}
		headers[name] = text
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}

// ResolveRunPaths 解析 run.command 中的相对路径
func ResolveRunPaths(baseDir string, run *op.Run) {
	if run == nil || len(run.Command) == 0 {
		return
	}
	for i, cmd := range run.Command {
		if !common.LooksLikePath(cmd) {
			continue
		}
		resolved, err := common.ResolveAbsolutePath(baseDir, cmd)
		if err != nil {
			continue
		}
		run.Command[i] = resolved
	}
}

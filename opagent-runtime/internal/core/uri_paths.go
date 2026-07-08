package core

import (
	"path/filepath"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func agentRootFromURI(uri string) string {
	configPath := op.URIToPath(uri)
	if configPath == "" {
		return ""
	}
	configDir := filepath.Dir(configPath)
	switch filepath.Base(configDir) {
	case ".agent", ".agents":
		return filepath.Dir(configDir)
	default:
		return configDir
	}
}

func agentHomeFromURI(uri string) string {
	configPath := op.URIToPath(uri)
	if configPath == "" {
		return ""
	}
	configDir := filepath.Dir(configPath)
	switch filepath.Base(configDir) {
	case ".agent", ".agents":
		return configDir
	default:
		return filepath.Join(configDir, ".agent")
	}
}

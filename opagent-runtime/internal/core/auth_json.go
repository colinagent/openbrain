package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func AuthJSONPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", "user", "auth.json")
}

type authJSON struct {
	BaseURL   string `json:"baseUrl,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
	AIGateway string `json:"aiGateway,omitempty"`
	Token     string `json:"token"`
	UID       string `json:"uid,omitempty"`
	Email     string `json:"email,omitempty"`
	Version   int    `json:"version,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

func readAuthJSON(path string) (*authJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a authJSON
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func readAuthJSONToken(path string) (string, error) {
	a, err := readAuthJSON(path)
	if err != nil || a == nil {
		return "", err
	}
	return a.Token, nil
}

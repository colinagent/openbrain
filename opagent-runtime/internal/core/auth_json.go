package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func AuthJSONPath(baseDir string) string {
	return filepath.Join(baseDir, "configs", "user", "auth.json")
}

type authJSON struct {
	Version      int    `json:"version"`
	BaseURL      string `json:"baseUrl,omitempty"`
	Gateway      string `json:"gateway,omitempty"`
	AIGateway    string `json:"aiGateway,omitempty"`
	Token        string `json:"token"`
	UID          string `json:"uid"`
	Email        string `json:"email,omitempty"`
	DeploymentID string `json:"deploymentID"`
	OrgID        string `json:"orgID"`
	IdentityID   string `json:"identityID"`
	ConnectionID string `json:"connectionID"`
	AuthMethod   string `json:"authMethod"`
	AuthTime     string `json:"authTime"`
	ExpiresAt    string `json:"expiresAt"`
	UpdatedAt    int64  `json:"updatedAt,omitempty"`
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
	if a.Version != 2 || a.Token == "" || a.UID == "" || a.DeploymentID == "" ||
		a.OrgID == "" || a.IdentityID == "" || a.ConnectionID == "" ||
		a.AuthMethod == "" || a.AuthTime == "" || a.ExpiresAt == "" {
		return nil, errors.New("tenant-bound auth config version 2 is required")
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

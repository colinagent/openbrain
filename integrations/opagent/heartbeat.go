package opagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatTimeout  = 5 * time.Second
)

type HeartbeatService struct {
	baseDir string
	client  *http.Client
	now     func() time.Time
}

type heartbeatAuth struct {
	Version int    `json:"version"`
	Gateway string `json:"gateway"`
	Token   string `json:"token"`
}

type heartbeatEnvelope struct {
	SchemaVersion string            `json:"schemaVersion"`
	SentAt        time.Time         `json:"sentAt"`
	Instance      heartbeatInstance `json:"instance"`
}

type heartbeatInstance struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname,omitempty"`
	Env      string `json:"env,omitempty"`
	BaseDir  string `json:"baseDir"`
	PID      int    `json:"pid,omitempty"`
}

func NewHeartbeatService(baseDir string) *HeartbeatService {
	return &HeartbeatService{
		baseDir: filepath.Clean(strings.TrimSpace(baseDir)),
		client:  &http.Client{Timeout: defaultHeartbeatTimeout},
		now:     time.Now,
	}
}

func (service *HeartbeatService) Run(ctx context.Context) error {
	if service == nil || service.baseDir == "" || service.baseDir == "." {
		return fmt.Errorf("OpenBrain heartbeat base directory is required")
	}
	service.report(ctx)
	ticker := time.NewTicker(defaultHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			service.report(ctx)
		}
	}
}

func (service *HeartbeatService) report(ctx context.Context) {
	if err := service.reportOnce(ctx); err != nil {
		slog.Warn("OpenBrain heartbeat failed", "error", err)
	}
}

func (service *HeartbeatService) reportOnce(ctx context.Context) error {
	authPath := filepath.Join(service.baseDir, "configs", "user", "auth.json")
	auth, err := readHeartbeatAuth(authPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	hostIDPath := filepath.Join(service.baseDir, "configs", "system", "host_id")
	if err := requireRegularFile(hostIDPath, false); err != nil {
		return fmt.Errorf("validate Runtime host ID: %w", err)
	}
	hostID, err := os.ReadFile(hostIDPath)
	if err != nil {
		return fmt.Errorf("read Runtime host ID: %w", err)
	}
	hostname, _ := os.Hostname()
	payload := heartbeatEnvelope{
		SchemaVersion: "v1",
		SentAt:        service.now().UTC(),
		Instance: heartbeatInstance{
			ID: strings.TrimSpace(string(hostID)), Hostname: strings.TrimSpace(hostname),
			Env: "local", BaseDir: service.baseDir, PID: os.Getpid(),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, auth.Gateway+"/api/v1/heartbeat/heartbeats", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+auth.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := service.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("heartbeat response %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func readHeartbeatAuth(path string) (heartbeatAuth, error) {
	if err := requireRegularFile(path, false); err != nil {
		return heartbeatAuth{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return heartbeatAuth{}, err
	}
	var auth heartbeatAuth
	if err := json.Unmarshal(raw, &auth); err != nil {
		return heartbeatAuth{}, fmt.Errorf("decode OpenBrain heartbeat auth: %w", err)
	}
	auth.Gateway = strings.TrimRight(strings.TrimSpace(auth.Gateway), "/")
	auth.Token = strings.TrimSpace(auth.Token)
	parsed, parseErr := url.Parse(auth.Gateway)
	if auth.Version != 2 || auth.Token == "" || parseErr != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return heartbeatAuth{}, fmt.Errorf("OpenBrain heartbeat auth is invalid")
	}
	if parsed.Scheme != "https" {
		address := net.ParseIP(parsed.Hostname())
		if parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && (address == nil || !address.IsLoopback())) {
			return heartbeatAuth{}, fmt.Errorf("OpenBrain heartbeat gateway must use HTTPS")
		}
	}
	return auth, nil
}

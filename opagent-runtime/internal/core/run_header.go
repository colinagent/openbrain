package core

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
)

const openbrainSessionHeaderPlaceholder = "{openbrain_session}"

type runHeaderRoundTripper struct {
	base   http.RoundTripper
	header map[string]string
}

func (t runHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	headers, err := resolveRunHeaders(t.header)
	if err != nil {
		return nil, err
	}
	if len(headers) > 0 {
		req = req.Clone(req.Context())
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func newRunHeaderHTTPClient(headers map[string]string) *http.Client {
	return newRunHeaderHTTPClientWithTimeout(headers, 0)
}

func newRunHeaderHTTPClientWithTimeout(headers map[string]string, timeout time.Duration) *http.Client {
	if len(headers) == 0 {
		if timeout <= 0 {
			return nil
		}
		return &http.Client{Timeout: timeout}
	}
	client := &http.Client{Transport: runHeaderRoundTripper{header: headers}}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}

func resolveRunHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	resolved := make(map[string]string, len(headers))
	for key, value := range headers {
		headerName := strings.TrimSpace(key)
		if headerName == "" {
			continue
		}
		nextValue, err := resolveRunHeaderValue(value)
		if err != nil {
			return nil, err
		}
		resolved[headerName] = nextValue
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	return resolved, nil
}

func resolveRunHeaderValue(value string) (string, error) {
	if !strings.Contains(value, openbrainSessionHeaderPlaceholder) {
		return value, nil
	}
	token, err := loadOpenBrainSessionToken()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(value, openbrainSessionHeaderPlaceholder, token), nil
}

func loadOpenBrainSessionToken() (string, error) {
	baseDir := ""
	if sys := config.GetSystem(); sys != nil {
		baseDir = strings.TrimSpace(sys.BaseDir)
	}
	if baseDir == "" {
		return "", errors.New("OpenBrain baseDir is not configured")
	}
	token, err := readAuthJSONToken(AuthJSONPath(baseDir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("OpenBrain login is required")
		}
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("OpenBrain login is required")
	}
	return token, nil
}

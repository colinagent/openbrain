package op

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type staticOAuthHandler struct {
	token string
}

func (h staticOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: h.token, TokenType: "Bearer"}), nil
}

func (h staticOAuthHandler) Authorize(context.Context, *http.Request, *http.Response) error {
	return nil
}

func TestStreamableClientTransportSendsOAuthBearer(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			ID any `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"capabilities":    map[string]any{"logging": map[string]any{}},
				"protocolVersion": latestProtocolVersion,
				"serverInfo":      map[string]any{"name": "server", "version": "v0.0.1"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(&Implementation{Name: "client", Version: "v0.0.1"}, nil)
	transport := &StreamableClientTransport{
		Endpoint:     server.URL,
		OAuthHandler: staticOAuthHandler{token: "session-token"},
	}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if gotAuth != "Bearer session-token" {
		t.Fatalf("Authorization = %q, want bearer session token", gotAuth)
	}
}

func TestStreamableClientTransportSendsConfiguredHeadersOnAllRequests(t *testing.T) {
	type headerLog struct {
		method string
		value  string
	}
	var (
		mu     sync.Mutex
		logs   []headerLog
		gotGET = make(chan struct{}, 1)
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		logs = append(logs, headerLog{method: r.Method, value: r.Header.Get("X-Tool-Token")})
		mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			select {
			case gotGET <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
			return
		case http.MethodPost:
			var req struct {
				ID any `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.ID == nil {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"capabilities":    map[string]any{"logging": map[string]any{}},
					"protocolVersion": latestProtocolVersion,
					"serverInfo":      map[string]any{"name": "server", "version": "v0.0.1"},
				},
			})
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}))
	defer server.Close()

	client := NewClient(&Implementation{Name: "client", Version: "v0.0.1"}, nil)
	transport := &StreamableClientTransport{
		Endpoint: server.URL,
		Header: map[string]string{
			"X-Tool-Token": "tool-token",
		},
	}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-gotGET:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for standalone GET")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(logs) < 3 {
		t.Fatalf("requests = %v, want at least POST/GET/DELETE", logs)
	}
	var sawPost, sawGet, sawDelete bool
	for _, log := range logs {
		if log.value != "tool-token" {
			t.Fatalf("%s header = %q, want tool-token", log.method, log.value)
		}
		switch log.method {
		case http.MethodPost:
			sawPost = true
		case http.MethodGet:
			sawGet = true
		case http.MethodDelete:
			sawDelete = true
		}
	}
	if !sawPost || !sawGet || !sawDelete {
		t.Fatalf("methods = %v, want POST/GET/DELETE", logs)
	}
}

func TestStreamableClientTransportExposesSessionMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
			return
		case http.MethodPost:
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		switch req.Method {
		case methodInitialize:
			w.Header().Set(sessionIDHeader, "gone-session")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"capabilities":    map[string]any{"logging": map[string]any{}},
					"protocolVersion": latestProtocolVersion,
					"serverInfo":      map[string]any{"name": "server", "version": "v0.0.1"},
				},
			})
		case notificationInitialized:
			w.WriteHeader(http.StatusAccepted)
		case methodListTools:
			if got := r.Header.Get(sessionIDHeader); got != "gone-session" {
				t.Fatalf("%s = %q, want gone-session", sessionIDHeader, got)
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewClient(&Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &StreamableClientTransport{Endpoint: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	_, err = session.ListTools(context.Background(), &ListToolsParams{})
	if !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("ListTools error = %v, want ErrSessionMissing", err)
	}
}

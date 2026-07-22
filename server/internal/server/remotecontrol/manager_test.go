package remotecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/gorilla/websocket"
)

func TestManagerEnrollmentRelayRegionSwitchPairingAndDisable(t *testing.T) {
	var mu sync.Mutex
	region := "us"
	generation := int64(1)
	revoked := false
	disabled := false
	responses := make(chan protocol.Envelope, 4)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/remote-control/regions", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{"regions": []map[string]any{{"regionID": "us", "displayName": "United States", "enabled": true}, {"regionID": "jp", "displayName": "Japan", "enabled": true}}})
	})
	mux.HandleFunc("POST /v1/remote-control/regions/us/environments/enroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Errorf("enroll authorization = %q", r.Header.Get("Authorization"))
		}
		writeTestJSON(w, map[string]any{"environment": map[string]any{"environmentID": "environment-a", "name": "Test Mac", "regionID": "us", "routingGeneration": 1}, "serverCredential": "rc_server.credential.secret"})
	})
	mux.HandleFunc("POST /v1/remote-control/regions/{regionID}/environments/environment-a/switch", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		region = r.PathValue("regionID")
		generation++
		currentRegion, currentGeneration := region, generation
		mu.Unlock()
		writeTestJSON(w, map[string]any{"environment": map[string]any{"environmentID": "environment-a", "name": "Test Mac", "regionID": currentRegion, "routingGeneration": currentGeneration}})
	})
	mux.HandleFunc("POST /v1/remote-control/regions/{regionID}/connect-tokens/server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]string{"connectToken": "connect-token"})
	})
	mux.HandleFunc("GET /v1/remote-control/regions/{regionID}/relay/server", func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		request := protocol.Envelope{ProtocolVersion: 1, Type: protocol.EnvelopeTypeRequest, ClientID: "client-a", StreamID: "stream-a", SeqID: 1, RequestID: "request-a", Operation: protocol.OperationEnvironmentStatus, Payload: json.RawMessage(`{}`)}
		if err := connection.WriteJSON(request); err != nil {
			return
		}
		var response protocol.Envelope
		if err := connection.ReadJSON(&response); err == nil {
			responses <- response
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("POST /v1/remote-control/regions/{regionID}/environments/environment-a/pairings", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{"pairingID": "pairing-a", "code": "ABCD-2345", "expiresAt": time.Now().Add(time.Minute)})
	})
	mux.HandleFunc("GET /v1/remote-control/regions/{regionID}/environments/environment-a/clients", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{"clients": []map[string]any{{"clientID": "client-a", "environmentID": "environment-a", "name": "iPhone", "platform": "iOS", "createdAt": time.Now()}}})
	})
	mux.HandleFunc("DELETE /v1/remote-control/regions/{regionID}/clients/client-a", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		revoked = true
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /v1/remote-control/regions/{regionID}/environments/environment-a", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		disabled = true
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runtimeView := &fakeRuntimeView{
		config: &op.Config{System: &op.SystemConfig{HostID: "host-a", HostName: "Test Mac"}, User: &op.UserConfig{Auth: &op.AuthConfig{UID: "user-alice", Token: "session-token"}}},
		system: &op.SystemConfigResult{SystemConfig: op.SystemConfig{HostID: "host-a"}, DefaultWorkspace: "/tmp/conversations"},
	}
	config := Config{Enabled: true, KillSwitch: false, APIURL: server.URL + "/v1/remote-control"}
	dispatcher := NewDispatcher(config)
	manager, err := NewManager(config, runtimeView, dispatcher, t.TempDir(), "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterMinimumHandlers(dispatcher, runtimeView, manager); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Enable(ctx, EnableInput{RegionID: "us"}); err == nil {
		t.Fatal("enable without confirmation succeeded")
	}
	status, err := manager.Enable(ctx, EnableInput{Confirmed: true, RegionID: "us"})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.EnvironmentID != "environment-a" {
		t.Fatalf("enabled status = %#v", status)
	}
	select {
	case response := <-responses:
		if response.Error != nil || !strings.Contains(string(response.Payload), `"regionID":"us"`) {
			t.Fatalf("relay response = %#v", response)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("host connector did not answer relay request")
	}
	pairing, err := manager.StartPairing(ctx)
	if err != nil || pairing.Code != "ABCD-2345" {
		t.Fatalf("pairing = %#v, %v", pairing, err)
	}
	clients, err := manager.Clients(ctx)
	if err != nil || len(clients) != 1 {
		t.Fatalf("clients = %#v, %v", clients, err)
	}
	if err := manager.RevokeClient(ctx, "client-a"); err != nil {
		t.Fatal(err)
	}
	status, err = manager.SwitchRegion(ctx, "jp")
	if err != nil || status.RegionID != "jp" || status.RoutingGeneration != 2 {
		t.Fatalf("switched status = %#v, %v", status, err)
	}
	if err := manager.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !revoked || !disabled || region != "jp" {
		t.Fatalf("cloud lifecycle revoked=%t disabled=%t region=%s", revoked, disabled, region)
	}
	statePath := filepath.Join(manager.stateStore.path)
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions = %o", info.Mode().Perm())
	}
}

func TestManagerDispatchesInterruptWhileSubmitIsRunning(t *testing.T) {
	dispatcher := NewDispatcher(Config{Enabled: true, KillSwitch: false})
	submitStarted := make(chan struct{})
	releaseSubmit := make(chan struct{})
	if err := dispatcher.Register(
		protocol.OperationThreadSubmit,
		protocol.CapabilityThreadExecute,
		func(ctx context.Context, _ Principal, _ json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			close(submitStarted)
			select {
			case <-releaseSubmit:
				return json.RawMessage(`{"ok":true}`), nil
			case <-ctx.Done():
				return nil, remoteError(protocol.ErrorInternal, "submit was cancelled")
			}
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Register(
		protocol.OperationThreadInterrupt,
		protocol.CapabilityThreadExecute,
		func(context.Context, Principal, json.RawMessage) (json.RawMessage, *protocol.RemoteError) {
			return json.RawMessage(`{"interrupted":true}`), nil
		},
	); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{dispatcher: dispatcher, rootContext: context.Background()}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = manager.serveConnection(
			r.Context(), connection,
			connectorState{EnvironmentID: "environment-a"},
			sessionSnapshot{UID: "user-a"},
		)
	}))
	defer server.Close()

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	writeRequest := func(sequence uint64, requestID string, operation protocol.Operation) {
		t.Helper()
		if err := connection.WriteJSON(protocol.Envelope{
			ProtocolVersion: protocol.CurrentProtocolVersion,
			Type:            protocol.EnvelopeTypeRequest, ClientID: "client-a", StreamID: "conversation",
			SeqID: sequence, RequestID: requestID, Operation: operation, Payload: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	writeRequest(1, "submit-a", protocol.OperationThreadSubmit)
	select {
	case <-submitStarted:
	case <-time.After(time.Second):
		t.Fatal("submit handler did not start")
	}
	writeRequest(2, "interrupt-a", protocol.OperationThreadInterrupt)
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	var interrupt protocol.Envelope
	if err := connection.ReadJSON(&interrupt); err != nil {
		t.Fatal(err)
	}
	if interrupt.RequestID != "interrupt-a" || interrupt.Error != nil || !strings.Contains(string(interrupt.Payload), `"interrupted":true`) {
		t.Fatalf("interrupt response = %+v", interrupt)
	}
	close(releaseSubmit)
	var submit protocol.Envelope
	if err := connection.ReadJSON(&submit); err != nil {
		t.Fatal(err)
	}
	if submit.RequestID != "submit-a" || submit.Error != nil {
		t.Fatalf("submit response = %+v", submit)
	}
}

func writeTestJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

type mutableSessionRuntime struct {
	mu     sync.RWMutex
	config *op.Config
}

func (r *mutableSessionRuntime) GetConfigContext(context.Context) (*op.Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := *r.config
	if r.config.User != nil {
		user := *r.config.User
		copy.User = &user
		if r.config.User.Auth != nil {
			auth := *r.config.User.Auth
			copy.User.Auth = &auth
		}
	}
	return &copy, nil
}
func (r *mutableSessionRuntime) GetSystemConfig(context.Context) (*op.SystemConfigResult, error) {
	return &op.SystemConfigResult{}, nil
}
func (r *mutableSessionRuntime) ListNodes(context.Context) ([]*op.OpNode, error) { return nil, nil }

func TestManagerStopsAndClearsCredentialsAfterLogout(t *testing.T) {
	closed := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/remote-control/regions/us/connect-tokens/server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]string{"connectToken": "connect-token"})
	})
	mux.HandleFunc("GET /v1/remote-control/regions/us/relay/server", func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				closed <- struct{}{}
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	runtimeView := &mutableSessionRuntime{config: &op.Config{User: &op.UserConfig{Auth: &op.AuthConfig{UID: "user-alice", Token: "session-token"}}}}
	config := Config{Enabled: true, KillSwitch: false, APIURL: server.URL + "/v1/remote-control"}
	dispatcher := NewDispatcher(config)
	baseDir := t.TempDir()
	store := newStateStore(baseDir)
	if err := store.Save(connectorState{Enabled: true, EnvironmentID: "environment-a", EnvironmentName: "Mac", ServerCredential: "server-secret", RegionID: "us", RoutingGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(config, runtimeView, dispatcher, baseDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.Status().ConnectionState != "online" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	runtimeView.mu.Lock()
	runtimeView.config.User.Auth = nil
	runtimeView.mu.Unlock()
	deadline = time.Now().Add(4 * time.Second)
	for manager.Status().Enabled && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	status := manager.Status()
	if status.Enabled || status.EnvironmentID != "" || status.LastError != "signed_out" {
		t.Fatalf("logout status = %#v", status)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("logout did not close relay connection")
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.ServerCredential != "" {
		t.Fatal("logout left the server credential on disk")
	}
}

func TestManagerStopsAfterEnvironmentIsRevoked(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/remote-control/regions/us/connect-tokens/server", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeTestJSON(w, map[string]string{"code": "not_authenticated"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runtimeView := &mutableSessionRuntime{config: &op.Config{User: &op.UserConfig{Auth: &op.AuthConfig{UID: "user-alice", Token: "session-token"}}}}
	config := Config{Enabled: true, KillSwitch: false, APIURL: server.URL + "/v1/remote-control"}
	baseDir := t.TempDir()
	store := newStateStore(baseDir)
	if err := store.Save(connectorState{Enabled: true, EnvironmentID: "environment-a", ServerCredential: "server-secret", RegionID: "us", RoutingGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(config, runtimeView, NewDispatcher(config), baseDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for manager.Status().Enabled && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	status := manager.Status()
	if status.Enabled || status.EnvironmentID != "" || status.LastError != "remote_access_revoked" {
		t.Fatalf("revoked status = %#v", status)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.ServerCredential != "" {
		t.Fatal("revoked environment left the server credential on disk")
	}
}

func TestManagerFollowsAccountRegionSwitch(t *testing.T) {
	connected := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/remote-control/regions/us/connect-tokens/server", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		writeTestJSON(w, map[string]any{"code": "region_changed", "regionID": "jp", "routingGeneration": 2})
	})
	mux.HandleFunc("POST /v1/remote-control/regions/jp/connect-tokens/server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]string{"connectToken": "connect-token"})
	})
	mux.HandleFunc("GET /v1/remote-control/regions/jp/relay/server", func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		connected <- struct{}{}
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runtimeView := &mutableSessionRuntime{config: &op.Config{User: &op.UserConfig{Auth: &op.AuthConfig{UID: "user-alice", Token: "session-token"}}}}
	config := Config{Enabled: true, KillSwitch: false, APIURL: server.URL + "/v1/remote-control"}
	baseDir := t.TempDir()
	store := newStateStore(baseDir)
	if err := store.Save(connectorState{Enabled: true, EnvironmentID: "environment-a", ServerCredential: "server-secret", RegionID: "us", RoutingGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(config, runtimeView, NewDispatcher(config), baseDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("host did not follow the new relay region")
	}
	deadline := time.Now().Add(time.Second)
	for manager.Status().ConnectionState != "online" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	status := manager.Status()
	if status.RegionID != "jp" || status.RoutingGeneration != 2 || status.ConnectionState != "online" {
		t.Fatalf("redirected status = %#v", status)
	}
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RegionID != "jp" || persisted.RoutingGeneration != 2 {
		t.Fatalf("persisted redirect = %#v", persisted)
	}
}

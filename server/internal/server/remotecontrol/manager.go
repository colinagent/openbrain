package remotecontrol

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	protocol "github.com/colinagent/openbrain/opagent-protocol/go-sdk/remotecontrol"
	"github.com/gorilla/websocket"
)

type sessionSnapshot struct {
	UID   string
	Token string
}

var errSignedOut = errors.New("OpenBrain sign-in is required")

type ManagementStatus struct {
	Available         bool   `json:"available"`
	Enabled           bool   `json:"enabled"`
	ConnectionState   string `json:"connectionState"`
	EnvironmentID     string `json:"environmentID,omitempty"`
	EnvironmentName   string `json:"environmentName,omitempty"`
	RegionID          string `json:"regionID,omitempty"`
	RoutingGeneration int64  `json:"routingGeneration,omitempty"`
	LastError         string `json:"lastError,omitempty"`
}

type EnableInput struct {
	Confirmed bool   `json:"confirmed"`
	Name      string `json:"name"`
	RegionID  string `json:"regionID"`
}

type Manager struct {
	config      Config
	runtime     RuntimeView
	dispatcher  *Dispatcher
	cloud       *cloudClient
	stateStore  stateStore
	version     string
	rootContext context.Context

	mu              sync.RWMutex
	state           connectorState
	session         sessionSnapshot
	connectionState string
	lastError       string
	active          *websocket.Conn
	runCancel       context.CancelFunc
	runID           uint64
}

func NewManager(config Config, runtimeView RuntimeView, dispatcher *Dispatcher, baseDir, version string) (*Manager, error) {
	if runtimeView == nil || dispatcher == nil {
		return nil, errors.New("remote-control runtime and dispatcher are required")
	}
	cloud, err := newCloudClient(config.APIURL)
	if err != nil {
		return nil, err
	}
	return &Manager{
		config: config, runtime: runtimeView, dispatcher: dispatcher, cloud: cloud,
		stateStore: newStateStore(baseDir), version: version, connectionState: "off",
	}, nil
}

func (m *Manager) Start(ctx context.Context) error {
	state, err := m.stateStore.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.rootContext = ctx
	m.state = state
	m.mu.Unlock()
	if err := m.refreshSession(ctx); err != nil {
		if errors.Is(err, errSignedOut) && state.Enabled {
			m.suspend("signed_out")
		} else if !errors.Is(err, errSignedOut) {
			return err
		}
	}
	if state.Enabled && m.config.AllowsRemoteControl() {
		m.restartConnector()
	}
	go m.watchSession(ctx)
	return nil
}

func (m *Manager) Status() ManagementStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ManagementStatus{
		Available: m.config.AllowsRemoteControl(), Enabled: m.state.Enabled && m.config.AllowsRemoteControl(),
		ConnectionState: m.connectionState, EnvironmentID: m.state.EnvironmentID,
		EnvironmentName: m.state.EnvironmentName, RegionID: m.state.RegionID,
		RoutingGeneration: m.state.RoutingGeneration, LastError: m.lastError,
	}
}

func (m *Manager) EnvironmentSnapshot() EnvironmentSnapshot {
	status := m.Status()
	return EnvironmentSnapshot{
		EnvironmentID: status.EnvironmentID, Name: status.EnvironmentName,
		RegionID: status.RegionID, State: status.ConnectionState, ServerVersion: m.version,
		Platform: platformName(),
	}
}

func (m *Manager) Regions(ctx context.Context) ([]Region, error) {
	if !m.config.AllowsRemoteControl() {
		return nil, errors.New("remote control is unavailable")
	}
	return m.cloud.regions(ctx)
}

func (m *Manager) Enable(ctx context.Context, input EnableInput) (ManagementStatus, error) {
	if !m.config.AllowsRemoteControl() {
		return m.Status(), errors.New("remote control is unavailable")
	}
	if !input.Confirmed {
		return m.Status(), errors.New("explicit confirmation is required")
	}
	if err := m.refreshSession(ctx); err != nil {
		return m.Status(), err
	}
	cfg, err := m.runtime.GetConfigContext(ctx)
	if err != nil || cfg.System == nil {
		return m.Status(), errors.New("runtime configuration is unavailable")
	}
	regions, err := m.cloud.regions(ctx)
	if err != nil {
		return m.Status(), err
	}
	regionID := strings.TrimSpace(input.RegionID)
	if regionID == "" && len(regions) > 0 {
		regionID = regions[0].ID
	}
	if !containsEnabledRegion(regions, regionID) {
		return m.Status(), errors.New("selected region is unavailable")
	}
	m.mu.RLock()
	existing := m.state
	m.mu.RUnlock()
	if existing.EnvironmentID != "" && existing.ServerCredential != "" {
		if existing.RegionID != regionID {
			environment, err := m.cloud.switchRegion(ctx, existing.ServerCredential, regionID, existing.EnvironmentID)
			if err != nil {
				return m.Status(), err
			}
			existing.RegionID = environment.RegionID
			existing.RoutingGeneration = environment.RoutingGeneration
			existing.EnvironmentName = environment.Name
		}
		existing.Enabled = true
		m.mu.Lock()
		m.state = existing
		m.connectionState = "connecting"
		m.lastError = ""
		err := m.stateStore.Save(m.state)
		m.mu.Unlock()
		if err != nil {
			return m.Status(), err
		}
		m.restartConnector()
		return m.Status(), nil
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = strings.TrimSpace(cfg.System.HostName)
	}
	if name == "" {
		name = "OpenBrain on " + runtime.GOOS
	}
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()
	environment, credential, err := m.cloud.enroll(ctx, session.Token, regionID, map[string]any{
		"name": name, "installationID": cfg.System.HostID, "platform": runtime.GOOS, "serverVersion": m.version,
	})
	if err != nil {
		return m.Status(), err
	}
	state := connectorState{
		Enabled: true, EnvironmentID: environment.ID, EnvironmentName: environment.Name,
		ServerCredential: credential, RegionID: environment.RegionID, RoutingGeneration: environment.RoutingGeneration,
	}
	m.mu.Lock()
	m.state = state
	m.connectionState = "connecting"
	m.lastError = ""
	err = m.stateStore.Save(state)
	m.mu.Unlock()
	if err != nil {
		return m.Status(), err
	}
	m.restartConnector()
	return m.Status(), nil
}

func (m *Manager) Disable(ctx context.Context) error {
	m.stopConnector()
	m.mu.Lock()
	state := m.state
	m.state.Enabled = false
	m.connectionState = "off"
	m.lastError = ""
	if err := m.stateStore.Save(m.state); err != nil {
		m.mu.Unlock()
		return err
	}
	session := m.session
	m.mu.Unlock()
	if state.EnvironmentID == "" || state.RegionID == "" || session.Token == "" {
		return nil
	}
	if err := m.cloud.disable(ctx, session.Token, state.RegionID, state.EnvironmentID); err != nil {
		return err
	}
	m.mu.Lock()
	m.state = connectorState{}
	err := m.stateStore.Save(m.state)
	m.mu.Unlock()
	return err
}

func (m *Manager) SwitchRegion(ctx context.Context, regionID string) (ManagementStatus, error) {
	regionID = strings.TrimSpace(regionID)
	if regionID == "" {
		return m.Status(), errors.New("region is required")
	}
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	if !state.Enabled || state.ServerCredential == "" {
		return m.Status(), errors.New("remote control is disabled")
	}
	environment, err := m.cloud.switchRegion(ctx, state.ServerCredential, regionID, state.EnvironmentID)
	if err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.state.RegionID = environment.RegionID
	m.state.RoutingGeneration = environment.RoutingGeneration
	m.state.EnvironmentName = environment.Name
	m.connectionState = "connecting"
	m.lastError = ""
	err = m.stateStore.Save(m.state)
	m.mu.Unlock()
	if err != nil {
		return m.Status(), err
	}
	m.restartConnector()
	return m.Status(), nil
}

func (m *Manager) StartPairing(ctx context.Context) (Pairing, error) {
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	if !state.Enabled || state.ServerCredential == "" {
		return Pairing{}, errors.New("remote control is disabled")
	}
	return m.cloud.startPairing(ctx, state.ServerCredential, state.RegionID, state.EnvironmentID)
}

func (m *Manager) PairingStatus(ctx context.Context, pairingID string) (Pairing, error) {
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	if !state.Enabled || state.ServerCredential == "" || strings.TrimSpace(pairingID) == "" {
		return Pairing{}, errors.New("remote control is disabled")
	}
	return m.cloud.pairingStatus(ctx, state.ServerCredential, state.RegionID, state.EnvironmentID, pairingID)
}

func (m *Manager) Clients(ctx context.Context) ([]RemoteClient, error) {
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	if state.RegionID == "" {
		return []RemoteClient{}, nil
	}
	return m.cloud.clients(ctx, state.ServerCredential, state.RegionID, state.EnvironmentID)
}

func (m *Manager) RevokeClient(ctx context.Context, clientID string) error {
	if err := m.refreshSession(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	state, session := m.state, m.session
	m.mu.RUnlock()
	return m.cloud.revokeClient(ctx, session.Token, state.RegionID, clientID)
}

func (m *Manager) watchSession(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopConnector()
			return
		case <-ticker.C:
			if err := m.refreshSession(ctx); errors.Is(err, errSignedOut) {
				m.mu.RLock()
				enabled := m.state.Enabled
				m.mu.RUnlock()
				if enabled {
					m.suspend("signed_out")
				}
			}
		}
	}
}

func (m *Manager) refreshSession(ctx context.Context) error {
	cfg, err := m.runtime.GetConfigContext(ctx)
	if err != nil {
		return fmt.Errorf("refresh OpenBrain session: %w", err)
	}
	if cfg.User == nil || cfg.User.Auth == nil {
		return errSignedOut
	}
	next := sessionSnapshot{UID: strings.TrimSpace(cfg.User.Auth.UID), Token: strings.TrimSpace(cfg.User.Auth.Token)}
	if next.UID == "" || next.Token == "" {
		return errSignedOut
	}
	m.mu.Lock()
	previous := m.session
	m.session = next
	enabled := m.state.Enabled
	m.mu.Unlock()
	if enabled && previous.UID != "" && previous.UID != next.UID {
		m.suspend("account_changed")
		return errors.New("OpenBrain account changed")
	}
	return nil
}

func (m *Manager) suspend(reason string) {
	m.stopConnector()
	m.mu.Lock()
	m.state = connectorState{}
	m.connectionState = "off"
	m.lastError = reason
	_ = m.stateStore.Save(m.state)
	m.mu.Unlock()
}

func (m *Manager) restartConnector() {
	m.stopConnector()
	m.mu.Lock()
	if !m.state.Enabled || !m.config.AllowsRemoteControl() || m.rootContext == nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.rootContext)
	m.runCancel = cancel
	m.runID++
	runID := m.runID
	m.connectionState = "connecting"
	m.mu.Unlock()
	go m.connectionLoop(ctx, runID)
}

func (m *Manager) stopConnector() {
	m.mu.Lock()
	m.runID++
	if m.runCancel != nil {
		m.runCancel()
		m.runCancel = nil
	}
	if m.active != nil {
		_ = m.active.Close()
		m.active = nil
	}
	m.mu.Unlock()
}

func (m *Manager) connectionLoop(ctx context.Context, runID uint64) {
	backoff := time.Second
	for ctx.Err() == nil {
		m.mu.RLock()
		state, session := m.state, m.session
		m.mu.RUnlock()
		if !state.Enabled || session.UID == "" {
			return
		}
		token, err := m.cloud.connectToken(ctx, state.ServerCredential, state.RegionID)
		if regionID, generation, ok := connectorRegionRedirect(err); ok {
			if redirectErr := m.followRegionRedirect(runID, state.EnvironmentID, regionID, generation); redirectErr == nil {
				backoff = time.Second
				continue
			} else {
				err = redirectErr
			}
		}
		if isTerminalConnectorError(err) {
			m.suspend("remote_access_revoked")
			return
		}
		if err == nil {
			header := http.Header{}
			header.Set("Authorization", "Bearer "+token)
			connection, _, dialErr := websocket.DefaultDialer.DialContext(ctx, m.cloud.relayURL(state.RegionID), header)
			if dialErr == nil {
				if !m.setActive(connection, runID) {
					return
				}
				backoff = time.Second
				err = m.serveConnection(ctx, connection, state, session)
				m.clearActive(connection)
			} else {
				err = dialErr
			}
		}
		if ctx.Err() != nil {
			return
		}
		m.setConnectionError(err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (m *Manager) followRegionRedirect(runID uint64, environmentID, regionID string, generation int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runID != runID || !m.state.Enabled || m.state.EnvironmentID != environmentID {
		return context.Canceled
	}
	m.state.RegionID = regionID
	m.state.RoutingGeneration = generation
	m.connectionState = "connecting"
	m.lastError = ""
	return m.stateStore.Save(m.state)
}

func (m *Manager) serveConnection(ctx context.Context, connection *websocket.Conn, state connectorState, session sessionSnapshot) error {
	connectionCtx, cancelConnection := context.WithCancel(ctx)
	defer cancelConnection()

	var writeMu sync.Mutex
	var requestMu sync.Mutex
	requestCancels := make(map[string]map[uint64]context.CancelFunc)
	var nextCancelID uint64
	inFlight := make(chan struct{}, protocol.MaxInFlightRequests)

	requestKey := func(frame protocol.Envelope) string {
		return frame.ClientID + "\x00" + frame.StreamID + "\x00" + frame.RequestID
	}
	registerCancel := func(key string, cancel context.CancelFunc) uint64 {
		requestMu.Lock()
		defer requestMu.Unlock()
		nextCancelID++
		if requestCancels[key] == nil {
			requestCancels[key] = make(map[uint64]context.CancelFunc)
		}
		requestCancels[key][nextCancelID] = cancel
		return nextCancelID
	}
	removeCancel := func(key string, cancelID uint64) {
		requestMu.Lock()
		defer requestMu.Unlock()
		cancels := requestCancels[key]
		delete(cancels, cancelID)
		if len(cancels) == 0 {
			delete(requestCancels, key)
		}
	}
	cancelRequest := func(frame protocol.Envelope) {
		key := requestKey(frame)
		requestMu.Lock()
		registered := requestCancels[key]
		cancels := make([]context.CancelFunc, 0, len(registered))
		for _, cancel := range registered {
			cancels = append(cancels, cancel)
		}
		delete(requestCancels, key)
		requestMu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
	}

	for {
		messageType, data, err := connection.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage {
			return errors.New("relay sent a non-text frame")
		}
		request, err := protocol.DecodeEnvelope(data)
		if err != nil {
			return errors.New("relay sent an invalid request")
		}
		if request.Type == protocol.EnvelopeTypeClose {
			cancelRequest(request)
			continue
		}
		if request.Type == protocol.EnvelopeTypePing {
			if err := writeRemoteEnvelope(connection, &writeMu, protocol.Envelope{
				ProtocolVersion: protocol.CurrentProtocolVersion,
				Type:            protocol.EnvelopeTypePong,
				ClientID:        request.ClientID,
			}); err != nil {
				return err
			}
			continue
		}
		if request.Type != protocol.EnvelopeTypeRequest {
			return errors.New("relay sent an unsupported client frame")
		}
		principal, err := NewPrincipal(session.UID, state.EnvironmentID, request.ClientID,
			protocol.CapabilityEnvironmentRead, protocol.CapabilityWorkspaceList,
			protocol.CapabilityAgentList, protocol.CapabilityModelList,
			protocol.CapabilityThreadRead, protocol.CapabilityThreadExecute,
			protocol.CapabilityMessageReply, protocol.CapabilityFileRead)
		if err != nil {
			return err
		}
		select {
		case inFlight <- struct{}{}:
		default:
			response := protocol.Envelope{
				ProtocolVersion: protocol.CurrentProtocolVersion,
				Type:            protocol.EnvelopeTypeResponse,
				ClientID:        request.ClientID,
				StreamID:        request.StreamID,
				SeqID:           request.SeqID,
				RequestID:       request.RequestID,
				Error:           remoteError(protocol.ErrorRateLimited, "host in-flight request limit exceeded"),
			}
			if err := writeRemoteEnvelope(connection, &writeMu, response); err != nil {
				return err
			}
			continue
		}

		requestContext := connectionCtx
		if request.Operation == protocol.OperationThreadSubmit || request.Operation == protocol.OperationThreadContinue {
			m.mu.RLock()
			requestContext = m.rootContext
			m.mu.RUnlock()
			if requestContext == nil {
				requestContext = ctx
			}
		}
		requestCtx, cancel := context.WithCancel(requestContext)
		key := requestKey(request)
		var cancelID uint64
		if request.Operation != protocol.OperationThreadSubmit && request.Operation != protocol.OperationThreadContinue {
			cancelID = registerCancel(key, cancel)
		}
		go func(request protocol.Envelope, principal Principal, key string, cancelID uint64) {
			defer func() { <-inFlight }()
			defer cancel()
			if cancelID != 0 {
				defer removeCancel(key, cancelID)
			}
			response := m.dispatcher.Dispatch(requestCtx, principal, request)
			_ = writeRemoteEnvelope(connection, &writeMu, response)
		}(request, principal, key, cancelID)
	}
}

const remoteChunkPayloadBytes = 160 * 1024

func writeRemoteEnvelope(connection *websocket.Conn, mu *sync.Mutex, envelope protocol.Envelope) error {
	frames, err := encodeRemoteEnvelopeFrames(envelope)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	for _, frame := range frames {
		if err := connection.WriteMessage(websocket.TextMessage, frame); err != nil {
			return err
		}
	}
	return nil
}

func encodeRemoteEnvelopeFrames(envelope protocol.Envelope) ([][]byte, error) {
	message, err := protocol.EncodeMessage(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode remote response: %w", err)
	}
	if len(message) <= protocol.MaxFrameBytes {
		return [][]byte{message}, nil
	}
	segmentCount := (len(message) + remoteChunkPayloadBytes - 1) / remoteChunkPayloadBytes
	if segmentCount > protocol.MaxChunkCount {
		return nil, errors.New("remote response exceeds chunk count limit")
	}
	frames := make([][]byte, 0, segmentCount)
	for segmentID := 0; segmentID < segmentCount; segmentID++ {
		start := segmentID * remoteChunkPayloadBytes
		end := start + remoteChunkPayloadBytes
		if end > len(message) {
			end = len(message)
		}
		chunk := protocol.Envelope{
			ProtocolVersion: protocol.CurrentProtocolVersion,
			Type:            protocol.EnvelopeTypeChunk,
			ClientID:        envelope.ClientID,
			StreamID:        envelope.StreamID,
			SeqID:           envelope.SeqID,
			Chunk: &protocol.Chunk{
				SegmentID:          uint32(segmentID),
				SegmentCount:       uint32(segmentCount),
				MessageSizeBytes:   uint64(len(message)),
				MessageChunkBase64: base64.StdEncoding.EncodeToString(message[start:end]),
			},
		}
		encoded, err := protocol.EncodeEnvelope(chunk)
		if err != nil {
			return nil, fmt.Errorf("encode remote response chunk: %w", err)
		}
		frames = append(frames, encoded)
	}
	return frames, nil
}

func (m *Manager) setActive(connection *websocket.Conn, runID uint64) bool {
	m.mu.Lock()
	if m.runID != runID || !m.state.Enabled {
		m.mu.Unlock()
		_ = connection.Close()
		return false
	}
	m.active = connection
	m.connectionState = "online"
	m.lastError = ""
	m.mu.Unlock()
	return true
}

func (m *Manager) clearActive(connection *websocket.Conn) {
	m.mu.Lock()
	if m.active == connection {
		m.active = nil
	}
	m.mu.Unlock()
}

func (m *Manager) setConnectionError(err error) {
	m.mu.Lock()
	m.connectionState = "reconnecting"
	if err != nil {
		m.lastError = err.Error()
	}
	m.mu.Unlock()
}

func containsEnabledRegion(regions []Region, regionID string) bool {
	for _, region := range regions {
		if region.ID == regionID && region.Enabled {
			return true
		}
	}
	return false
}

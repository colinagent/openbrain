package notify

import (
	"encoding/json"
	"log/slog"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/sse"
)

// Service forwards host notifications to SSE. No file I/O; persistence is handled by the chat service.
type Service struct {
	sseManager         *sse.Manager
	messengerBroadcast func(op.MessageRecord)
}

func NewService(sseManager *sse.Manager) *Service {
	return &Service{sseManager: sseManager}
}

func (s *Service) SetMessengerBroadcast(fn func(op.MessageRecord)) {
	if s == nil {
		return
	}
	s.messengerBroadcast = fn
}

// HandleHostNotification handles notifications from OpAgent host.
func (s *Service) HandleHostNotification(req *op.InfoNotificationServerRequest) {
	if req == nil {
		slog.Error("host notify: empty request")
		return
	}
	slog.Info("host notify: request", "request", req)

	params := req.Params
	if params == nil {
		slog.Error("host notify: empty params")
		return
	}

	meta := params.Meta.Clone()
	typ, ok := meta["type"].(string)

	if !ok || typ == "" || typ == "stream" {
		s.NotifyStream(meta, params.Content)
		if !ok || typ == "" {
			return
		}
	}

	if typ == "ignore" {
		return
	}

	if typ == "message" {
		s.Notify(meta, params.Content)
		s.NotifyMessenger(meta, params.Content)
		return
	}

	s.Notify(meta, params.Content)
}

// Notify sends a normal event.
func (s *Service) Notify(meta op.Meta, content op.Content) {
	newMeta := meta.Clone()
	s.sendEvent(newMeta, content)
}

func (s *Service) NotifyMessenger(meta op.Meta, content op.Content) {
	if s == nil || s.sseManager == nil {
		return
	}
	record, ok := decodeMessengerRecord(content)
	if !ok {
		return
	}
	if s.messengerBroadcast != nil {
		s.messengerBroadcast(record)
	}
}

// NotifyStream sends a stream event.
func (s *Service) NotifyStream(meta op.Meta, content op.Content) {
	newMeta := meta.Add(op.Meta{
		"type": "stream",
	})
	s.sendEvent(newMeta, content)
}

// NotifyError sends an error event. Meta must contain threadID for the event to be delivered.
func (s *Service) NotifyError(meta op.Meta, content op.Content) {
	newMeta := meta.Clone()
	if newMeta == nil {
		newMeta = op.Meta{}
	}
	newMeta["type"] = "error"
	s.sendEvent(newMeta, content)
}

// NotifyEnd sends an end event. Meta must contain threadID for the event to be delivered.
func (s *Service) NotifyEnd(meta op.Meta, content op.Content) {
	newMeta := meta.Clone()
	if newMeta == nil {
		newMeta = op.Meta{}
	}
	newMeta["type"] = "end"
	s.sendEvent(newMeta, content)
}

// NotifyModelOutput sends a model output event.
func (s *Service) NotifyModelOutput(meta op.Meta, content op.Content) {
	newMeta := meta.Add(op.Meta{
		"type": "modelOutput",
	})
	s.sendEvent(newMeta, content)
}

func (s *Service) sendEvent(meta op.Meta, content op.Content) {
	threadID, ok := meta["threadID"].(string)
	if !ok || threadID == "" {
		slog.Error("threadID not found in event meta", "meta", meta)
		return
	}
	s.sseManager.Publish(meta, content)
}

func decodeMessengerRecord(content op.Content) (op.MessageRecord, bool) {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok || jsonContent == nil || len(jsonContent.Raw) == 0 {
		return op.MessageRecord{}, false
	}
	var record op.MessageRecord
	if err := json.Unmarshal(jsonContent.Raw, &record); err != nil {
		return op.MessageRecord{}, false
	}
	return record, record.ID != "" && record.ChannelID != ""
}

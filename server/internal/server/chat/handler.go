package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/sse"
	"github.com/gin-gonic/gin"
)

const chatSSEKeepAliveInterval = 5 * time.Second

// Handler handles chat HTTP endpoints.
type Handler struct {
	sseManager        *sse.Manager
	chatSvc           *Service
	keepAliveInterval time.Duration
}

func NewHandler(sseManager *sse.Manager, chatSvc *Service) *Handler {
	return &Handler{
		sseManager:        sseManager,
		chatSvc:           chatSvc,
		keepAliveInterval: chatSSEKeepAliveInterval,
	}
}

func (h *Handler) Stream(c *gin.Context) {
	var req *op.GeneralContent
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}
	if req == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request is required"})
		return
	}
	if req.Meta == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "meta is required"})
		return
	}
	meta := req.Meta.Clone()
	delete(meta, "model")

	slog.Info("chat request", "meta", meta)
	modelKey := strings.TrimSpace(projectionMetaString(meta, "modelKey"))
	if modelKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelKey is required"})
		return
	}
	meta["modelKey"] = modelKey

	fileID, _ := meta["fileID"].(string)
	fileID = strings.TrimSpace(fileID)
	agentID := normalizeThreadAgentID(projectionMetaString(meta, "agentID"))
	if agentID == "" {
		slog.Error("chat request: agentID is required")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agentID is required",
		})
		return
	}
	meta["agentID"] = agentID
	threadID, _ := meta["threadID"].(string)
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		slog.Error("chat request: threadID is required")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "threadID is required",
		})
		return
	}
	turnRequestID, _ := meta["turnRequestID"].(string)
	turnRequestID = strings.TrimSpace(turnRequestID)
	if turnRequestID == "" {
		slog.Error("chat request: turnRequestID is required")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "turnRequestID is required",
		})
		return
	}

	threadMeta, err := h.chatSvc.validateThreadMeta(c.Request.Context(), meta)
	if err != nil {
		slog.Error("chat request: failed to validate thread metadata", "error", err)
		status := http.StatusBadRequest
		if isThreadNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{
			"error": "failed to load chat context: " + err.Error(),
		})
		return
	}
	meta["threadID"] = threadMeta.ThreadID
	meta["fileID"] = threadMeta.FileID
	meta["chatPath"] = threadMeta.ChatPath
	meta["path"] = threadMeta.Path
	meta["title"] = threadMeta.Title
	threadID = threadMeta.ThreadID
	meta["turnRequestID"] = turnRequestID

	content := req.Content
	content, err = normalizeUserMessageForThread(content, threadMeta.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid user content: " + err.Error(),
		})
		return
	}
	lastEventID := sse.ParseLastEventID(c.Request.Header.Get("Last-Event-ID"))

	turnCtx, turnCancel := context.WithCancel(context.Background())
	conn, replay, shouldStart, err := h.sseManager.BeginOrReattachTurn(threadID, turnRequestID, lastEventID, turnCancel)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, sse.ErrTurnConflict):
			status = http.StatusConflict
		case errors.Is(err, sse.ErrTurnNotFound):
			status = http.StatusGone
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	for _, event := range replay {
		if !h.writeEvent(c.Writer, event) {
			h.sseManager.Unregister(threadID, conn)
			return
		}
	}

	if h.sseManager.IsTurnComplete(threadID, turnRequestID) {
		h.sseManager.Unregister(threadID, conn)
		return
	}

	if shouldStart {
		go h.runChatStream(turnCtx, meta, content, threadMeta)
	}

	h.eventLoop(c.Writer, c.Request, conn)
	h.sseManager.Unregister(threadID, conn)
}

func (h *Handler) runChatStream(ctx context.Context, meta op.Meta, content op.Content, threadMeta *op.ThreadMeta) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("chat stream panic: %v", recovered)
			slog.Error("chat stream panic",
				"error", err,
				"path", streamThreadPath(threadMeta),
				"fileID", streamThreadFileID(threadMeta),
			)
			h.finishStreamTurnIfIncomplete(meta, err)
		}
	}()
	if h == nil || h.chatSvc == nil {
		return
	}
	if err := h.chatSvc.Stream(ctx, meta, content); err != nil {
		slog.Error("chat stream failed",
			"error", err,
			"path", streamThreadPath(threadMeta),
			"fileID", streamThreadFileID(threadMeta),
		)
		h.finishStreamTurnIfIncomplete(meta, err)
	}
}

func (h *Handler) finishStreamTurnIfIncomplete(meta op.Meta, err error) {
	if h == nil || h.chatSvc == nil || h.sseManager == nil || meta == nil || err == nil {
		return
	}
	threadID, _ := meta["threadID"].(string)
	turnRequestID, _ := meta["turnRequestID"].(string)
	threadID = strings.TrimSpace(threadID)
	turnRequestID = strings.TrimSpace(turnRequestID)
	if threadID == "" || turnRequestID == "" {
		return
	}
	if h.sseManager.IsTurnComplete(threadID, turnRequestID) {
		return
	}
	h.chatSvc.notifyError(meta, err.Error())
	if h.sseManager.IsTurnComplete(threadID, turnRequestID) {
		return
	}
	h.chatSvc.notifyEnd(meta)
}

func streamThreadPath(threadMeta *op.ThreadMeta) string {
	if threadMeta == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(threadMeta.Path); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(threadMeta.ChatPath)
}

func streamThreadFileID(threadMeta *op.ThreadMeta) string {
	if threadMeta == nil {
		return ""
	}
	return strings.TrimSpace(threadMeta.FileID)
}

func (h *Handler) Control(c *gin.Context) {
	var req *op.GeneralContent
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}
	if req == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request is required"})
		return
	}
	if req.Meta == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "meta is required"})
		return
	}
	meta := req.Meta.Clone()
	delete(meta, "model")
	codeStr, _ := meta["opcode"].(string)
	threadID, _ := meta["threadID"].(string)
	if strings.TrimSpace(threadID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "threadID is required"})
		return
	}
	content := req.Content
	threadMeta, err := h.chatSvc.validateThreadMeta(c.Request.Context(), meta)
	if err != nil {
		status := http.StatusBadRequest
		if isThreadNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	meta["threadID"] = threadMeta.ThreadID
	meta["fileID"] = threadMeta.FileID
	meta["chatPath"] = threadMeta.ChatPath
	meta["path"] = threadMeta.Path
	meta["title"] = threadMeta.Title
	if agentID := normalizeThreadAgentID(projectionMetaString(meta, "agentID")); agentID != "" {
		meta["agentID"] = agentID
	} else if agentID := normalizeThreadAgentID(threadMeta.AgentID); agentID != "" {
		meta["agentID"] = agentID
	}
	switch op.OpCode(strings.TrimSpace(codeStr)) {
	case op.OpThreadSteer, op.OpThreadFollowUp:
		modelKey := strings.TrimSpace(projectionMetaString(meta, "modelKey"))
		if modelKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "modelKey is required"})
			return
		}
		meta["modelKey"] = modelKey
		content, err = normalizeUserMessageForThread(content, threadMeta.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user content: " + err.Error()})
			return
		}
	}
	ack, err := h.chatSvc.Control(c.Request.Context(), meta, content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if op.OpCode(strings.TrimSpace(codeStr)) == op.OpThreadInterrupted {
		h.sseManager.CancelTurn(strings.TrimSpace(threadID))
	}
	c.JSON(http.StatusOK, ack)
}

func (h *Handler) eventLoop(w http.ResponseWriter, r *http.Request, conn *sse.Connection) {
	interval := h.keepAliveInterval
	if interval <= 0 {
		interval = chatSSEKeepAliveInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-conn.SSEChan:
			if !ok {
				return
			}
			if !h.writeEvent(w, event) {
				return
			}

		case <-ticker.C:
			if err := h.writeKeepAlive(w); err != nil {
				return
			}
			if h.sseManager != nil && conn != nil {
				h.sseManager.UpdateLastActive(conn.ThreadID)
			}

		case <-conn.Ctx.Done():
			// connection replaced or canceled
			return

		case <-r.Context().Done():
			// HTTP connection closed
			return
		}
	}
}

func (h *Handler) writeEvent(w http.ResponseWriter, event *sse.Event) bool {
	if event == nil || event.Message == nil {
		return true
	}
	data, err := json.Marshal(event.Message)
	if err != nil {
		slog.Error("failed to marshal SSE message", "error", err)
		return false
	}
	if event.ID > 0 {
		if _, err := w.Write([]byte("id: " + strconv.FormatInt(event.ID, 10) + "\n")); err != nil {
			return false
		}
	}
	if _, err := w.Write([]byte("event: message\n")); err != nil {
		return false
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := w.Write(data); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	typ, ok := event.Message.Meta["type"].(string)
	if ok && typ == "end" {
		return false
	}
	return true
}

func (h *Handler) writeKeepAlive(w http.ResponseWriter) error {
	if _, err := w.Write([]byte(": keep-alive\n\n")); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (h *Handler) writeUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": unauthorizedMessage})
}

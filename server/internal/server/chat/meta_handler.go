package chat

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
	"github.com/gin-gonic/gin"
)

type MetaHandler struct {
	service *Service
}

func NewMetaHandler(service *Service) *MetaHandler {
	return &MetaHandler{service: service}
}

func (h *MetaHandler) Get(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "chat service is not initialized"})
		return
	}
	query := op.ThreadMetaQuery{
		ThreadID: strings.TrimSpace(c.Query("threadID")),
		FileID:   strings.TrimSpace(c.Query("fileID")),
		ChatPath: strings.TrimSpace(c.Query("chatPath")),
		AgentID:  strings.TrimSpace(c.Query("agentID")),
	}
	meta, err := h.service.GetThreadMeta(context.Background(), query)
	if err != nil {
		status := http.StatusBadRequest
		message := err.Error()
		if isThreadNotFound(err) {
			status = http.StatusNotFound
			message = "thread not found"
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (h *MetaHandler) Update(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "chat service is not initialized"})
		return
	}
	var params op.ThreadMetaUpdateParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	meta, err := h.service.UpdateThreadMeta(context.Background(), params)
	if err != nil {
		status := http.StatusBadRequest
		message := err.Error()
		if isThreadNotFound(err) {
			status = http.StatusNotFound
			message = "thread not found"
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (h *MetaHandler) Snapshot(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "chat service is not initialized"})
		return
	}
	query := op.ThreadMetaQuery{
		ThreadID: strings.TrimSpace(c.Query("threadID")),
		FileID:   strings.TrimSpace(c.Query("fileID")),
		ChatPath: strings.TrimSpace(c.Query("chatPath")),
		AgentID:  strings.TrimSpace(c.Query("agentID")),
	}
	if entryWindow := parseThreadEntryWindowQuery(c); entryWindow != nil {
		query.EntryWindow = entryWindow
	}
	snapshot, err := h.service.GetThreadSnapshotWithOptions(context.Background(), query, ThreadSnapshotOptions{
		ModelKey: strings.TrimSpace(c.Query("modelKey")),
	})
	if err != nil {
		status := http.StatusBadRequest
		message := err.Error()
		if isThreadNotFound(err) {
			status = http.StatusNotFound
			message = "thread not found"
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	if snapshot == nil {
		snapshot = &ai.ThreadSnapshot{}
	}
	c.JSON(http.StatusOK, snapshot)
}

func parseThreadEntryWindowQuery(c *gin.Context) *op.ThreadEntryWindowQuery {
	mode := strings.TrimSpace(c.Query("entryWindow"))
	anchorID := strings.TrimSpace(c.Query("entryAnchorId"))
	limitRaw := strings.TrimSpace(c.Query("entryLimit"))
	if mode == "" && anchorID == "" && limitRaw == "" {
		return nil
	}
	query := &op.ThreadEntryWindowQuery{
		Mode:     mode,
		AnchorID: anchorID,
	}
	if limitRaw != "" {
		if limit, err := strconv.Atoi(limitRaw); err == nil {
			query.Limit = limit
		}
	}
	return query
}

package chat

import (
	"context"
	"net/http"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
	"github.com/gin-gonic/gin"
)

type ChatForkParams struct {
	SourceThreadID    string `json:"sourceThreadID,omitempty"`
	SourceChatPath    string `json:"sourceChatPath,omitempty"`
	CWD               string `json:"cwd,omitempty"`
	AgentID           string `json:"agentID,omitempty"`
	Title             string `json:"title"`
	ChatBaseDir       string `json:"chatBaseDir,omitempty"`
	ChatFileName      string `json:"chatFileName,omitempty"`
	ExecutionPlanPath string `json:"executionPlanPath,omitempty"`
}

type ForkHandler struct {
	service *Service
}

var forkThread = func(ctx context.Context, service *Service, params op.ThreadForkParams) (*op.ThreadMeta, error) {
	if service == nil {
		return nil, http.ErrServerClosed
	}
	return service.ForkThread(ctx, params)
}

func NewForkHandler(service *Service) *ForkHandler {
	return &ForkHandler{service: service}
}

func buildForkChatPath(cwd, chatBaseDir, chatFileName, title string) (string, error) {
	baseDir := strings.TrimSpace(chatBaseDir)
	if baseDir == "" {
		baseDir = strings.TrimSpace(cwd)
	}
	if strings.TrimSpace(chatFileName) != "" {
		return buildUniqueChatPathForFileName(baseDir, chatFileName)
	}
	return buildUniqueChatPath(baseDir, title)
}

func (h *ForkHandler) Fork(c *gin.Context) {
	if h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "chat service is not initialized"})
		return
	}

	var params ChatForkParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	params.SourceThreadID = strings.TrimSpace(params.SourceThreadID)
	params.SourceChatPath = strings.TrimSpace(params.SourceChatPath)
	params.CWD = strings.TrimSpace(params.CWD)
	params.AgentID = normalizeThreadAgentID(params.AgentID)
	params.Title = strings.TrimSpace(params.Title)
	params.ChatBaseDir = strings.TrimSpace(params.ChatBaseDir)
	params.ChatFileName = strings.TrimSpace(params.ChatFileName)
	params.ExecutionPlanPath = strings.TrimSpace(params.ExecutionPlanPath)
	if params.SourceThreadID == "" && params.SourceChatPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourceThreadID or sourceChatPath is required"})
		return
	}
	if params.CWD == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cwd is required"})
		return
	}
	if params.AgentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agentID is required"})
		return
	}
	if params.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	fileID := chatindex.GenerateFileID()
	chatPath, err := buildForkChatPath(params.CWD, params.ChatBaseDir, params.ChatFileName, params.Title)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to allocate chat path: " + err.Error()})
		return
	}

	meta, err := forkThread(context.Background(), h.service, op.ThreadForkParams{
		SourceThreadID:    params.SourceThreadID,
		SourceChatPath:    params.SourceChatPath,
		AgentID:           params.AgentID,
		CWD:               params.CWD,
		FileID:            fileID,
		ChatPath:          chatPath,
		Title:             params.Title,
		ExecutionPlanPath: params.ExecutionPlanPath,
	})
	if err != nil {
		status := http.StatusBadRequest
		if isThreadNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	path := strings.TrimSpace(meta.Path)
	if path == "" {
		path = strings.TrimSpace(meta.ChatPath)
	}
	if err := createProjectionFile(meta.ThreadID, fileID, meta.Title, path, meta.ParentThreadID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create chat markdown: " + err.Error()})
		return
	}
	if err := chatindex.UpsertFileRecord(params.CWD, chatindex.FileRecord{
		FileID:   fileID,
		AgentID:  params.AgentID,
		ThreadID: meta.ThreadID,
		CWD:      params.CWD,
		Path:     path,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update chat file index: " + err.Error()})
		return
	}
	if threadFilePath := strings.TrimSpace(meta.ThreadFilePath); threadFilePath != "" {
		if err := chatindex.UpsertThreadRecordForThreadFile(threadFilePath, chatindex.ThreadRecord{
			ThreadID: meta.ThreadID,
			AgentID:  params.AgentID,
			FileID:   fileID,
			CWD:      params.CWD,
			ChatPath: path,
			Path:     threadFilePath,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update thread index: " + err.Error()})
			return
		}
	}
	meta.FileID = fileID
	meta.Path = path
	meta.ChatPath = path

	c.JSON(http.StatusOK, meta)
}

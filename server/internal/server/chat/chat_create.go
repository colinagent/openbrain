package chat

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	mdutil "github.com/colinagent/openbrain/opagent-runtime/packages/agentctx/md"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
	"github.com/gin-gonic/gin"
)

type ChatCreateParams struct {
	CWD       string `json:"cwd,omitempty"`
	UserInput string `json:"userInput"`
	AgentID   string `json:"agentID"`
}

type ChatCreateResult struct {
	ThreadID       string `json:"threadID"`
	FileID         string `json:"fileID"`
	Title          string `json:"title"`
	CWD            string `json:"cwd,omitempty"`
	Path           string `json:"path"`
	ChatPath       string `json:"chatPath,omitempty"`
	InitialContent string `json:"initialContent,omitempty"`
}

var createThread = func(ctx context.Context, service *Service, params op.ThreadCreateParams) (*op.ThreadCreateResult, error) {
	if service == nil {
		return nil, fmt.Errorf("chat service is not initialized")
	}
	return service.CreateThread(ctx, params)
}

// CreateHandler handles POST /v1/thread/create and returns threadID/fileID/title/path.
type CreateHandler struct {
	service *Service
}

func NewCreateHandler(service *Service) *CreateHandler {
	return &CreateHandler{service: service}
}

func (h *CreateHandler) Create(c *gin.Context) {
	var params ChatCreateParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	params.CWD = strings.TrimSpace(params.CWD)
	params.AgentID = strings.TrimSpace(params.AgentID)
	if params.AgentID == "" {
		c.JSON(400, gin.H{"error": "agentID is required"})
		return
	}

	title := mdutil.DeriveChatTitle(params.UserInput)
	initResult, err := createThread(context.Background(), h.service, op.ThreadCreateParams{
		AgentID: params.AgentID,
		CWD:     params.CWD,
		Title:   title,
	})
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to init chat: " + err.Error()})
		return
	}
	cwd := strings.TrimSpace(initResult.CWD)
	if cwd == "" {
		cwd = params.CWD
	}
	if cwd == "" {
		c.JSON(500, gin.H{"error": "runtime did not return cwd"})
		return
	}
	fileID := strings.TrimSpace(initResult.FileID)
	if fileID == "" {
		c.JSON(500, gin.H{"error": "runtime did not return fileID"})
		return
	}
	path := strings.TrimSpace(initResult.Path)
	if path == "" {
		path = strings.TrimSpace(initResult.ChatPath)
	}
	if path == "" {
		c.JSON(500, gin.H{"error": "runtime did not return chatPath"})
		return
	}
	if err := createProjectionFile(initResult.ThreadID, fileID, initResult.Title, path, ""); err != nil {
		c.JSON(500, gin.H{"error": "failed to create chat markdown: " + err.Error()})
		return
	}
	initialContentBytes, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to read chat markdown: " + err.Error()})
		return
	}
	if err := chatindex.UpsertFileRecord(cwd, chatindex.FileRecord{
		FileID:   fileID,
		AgentID:  params.AgentID,
		ThreadID: initResult.ThreadID,
		CWD:      cwd,
		Path:     path,
	}); err != nil {
		c.JSON(500, gin.H{"error": "failed to update chat file index: " + err.Error()})
		return
	}
	if threadFilePath := strings.TrimSpace(initResult.ThreadFilePath); threadFilePath != "" {
		if err := chatindex.UpsertThreadRecordForThreadFile(threadFilePath, chatindex.ThreadRecord{
			ThreadID: initResult.ThreadID,
			AgentID:  params.AgentID,
			FileID:   fileID,
			CWD:      cwd,
			ChatPath: path,
			Path:     threadFilePath,
		}); err != nil {
			c.JSON(500, gin.H{"error": "failed to update thread index: " + err.Error()})
			return
		}
	}

	res := ChatCreateResult{
		ThreadID:       initResult.ThreadID,
		FileID:         fileID,
		Title:          initResult.Title,
		CWD:            cwd,
		Path:           path,
		ChatPath:       path,
		InitialContent: string(initialContentBytes),
	}
	c.JSON(200, res)

}

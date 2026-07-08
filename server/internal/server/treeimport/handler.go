package treeimport

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateSession(c *gin.Context) {
	var req CreateTreeImportSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	res, err := h.service.CreateSession(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) UploadFile(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("sessionId"))
	relativePath := strings.TrimPrefix(strings.TrimSpace(c.Param("relativePath")), "/")
	err := h.service.UploadFile(sessionID, relativePath, c.Request.Body, c.Request.ContentLength)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrSessionNotFound), errors.Is(err, os.ErrNotExist):
			status = http.StatusNotFound
		case strings.Contains(err.Error(), "mismatch"):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) CommitSession(c *gin.Context) {
	var req CommitTreeImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	res, err := h.service.CommitSession(strings.TrimSpace(c.Param("sessionId")), req.Overwrite)
	if err != nil {
		status := http.StatusBadRequest
		var conflictErr *ConflictError
		switch {
		case errors.Is(err, ErrSessionNotFound):
			status = http.StatusNotFound
		case errors.As(err, &conflictErr):
			status = http.StatusConflict
			c.JSON(status, gin.H{"error": err.Error(), "conflicts": conflictErr.Paths})
			return
		case errors.Is(err, ErrPathConflict):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) CancelSession(c *gin.Context) {
	err := h.service.CancelSession(strings.TrimSpace(c.Param("sessionId")))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CancelTreeImportResponse{Success: true})
}

package transfer

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	res, err := h.service.CreateWithBasePath(req, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) PutContent(c *gin.Context) {
	record, err := h.service.PutContent(c.Param("id"), c.Request.Body, c.Request.ContentLength)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "not pending") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) GetMeta(c *gin.Context) {
	record, ok := h.service.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "transfer not found"})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) GetContent(c *gin.Context) {
	record, ok := h.service.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "transfer not found"})
		return
	}
	if record.Status != StatusCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "transfer is not completed"})
		return
	}
	f, err := os.Open(record.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open transfer content"})
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stat transfer content"})
		return
	}
	contentType := strings.TrimSpace(record.MIMEType)
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(record.FileName)))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	c.Header("Content-Type", contentType)
	if strings.HasPrefix(contentType, "image/") {
		c.Header("Content-Disposition", "inline; filename=\""+record.FileName+"\"")
	} else {
		c.Header("Content-Disposition", "attachment; filename=\""+record.FileName+"\"")
	}
	http.ServeContent(c.Writer, c.Request, record.FileName, stat.ModTime(), f)
}

package resources

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

func (h *Handler) CreateGrant(c *gin.Context) {
	var req CreateGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	res, err := h.service.CreateGrant(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Inspect(c *gin.Context) {
	var req InspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	meta, err := h.service.Inspect(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, meta)
}

func (h *Handler) CreateHandle(c *gin.Context) {
	var req InspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	handle, err := h.service.CreateHandle(req.Target, req.Intent, req.GrantToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, handle)
}

func (h *Handler) GetContent(c *gin.Context) {
	h.serveContent(c, "")
}

func (h *Handler) GetPackageContent(c *gin.Context) {
	h.serveContent(c, c.Param("resourcePath"))
}

func (h *Handler) serveContent(c *gin.Context, resourcePath string) {
	record, ok := h.service.GetHandle(c.Param("handleId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "handle not found"})
		return
	}
	path := record.Path
	mimeType := record.MIMEType
	if record.EntryType == "directory" {
		if strings.TrimSpace(resourcePath) == "" || resourcePath == "/" {
			resourcePath = record.EpubPackagePath
		}
		var err error
		path, err = resolvePackageChildPath(record.Path, resourcePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		mimeType = detectMIME(path)
	} else if strings.TrimSpace(resourcePath) != "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource path not found"})
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open resource"})
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stat resource"})
		return
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	c.Header("Content-Type", mimeType)
	if record.Inline {
		c.Header("Content-Disposition", "inline")
		cacheControl := "private, no-cache"
		if strings.TrimSpace(c.Query("v")) != "" {
			cacheControl = "private, max-age=31536000, immutable"
		}
		c.Header("Cache-Control", cacheControl)
	} else {
		c.Header("Content-Disposition", "attachment")
	}
	http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), file)
}

func (h *Handler) CreateImportSession(c *gin.Context) {
	var req CreateImportSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	res, err := h.service.CreateImportSession(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) UploadImportSession(c *gin.Context) {
	res, err := h.service.UploadImportSession(c.Param("sessionId"), c.Request.Body, c.Request.ContentLength)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "mismatch") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

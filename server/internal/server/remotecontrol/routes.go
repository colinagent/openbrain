package remotecontrol

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"
)

type ManagementHandler struct {
	manager *Manager
}

func NewManagementHandler(manager *Manager) *ManagementHandler {
	return &ManagementHandler{manager: manager}
}

func (h *ManagementHandler) Register(router gin.IRoutes) {
	router.GET("/v1/remote-control/status", h.status)
	router.GET("/v1/remote-control/regions", h.regions)
	router.POST("/v1/remote-control/enable", h.enable)
	router.POST("/v1/remote-control/disable", h.disable)
	router.POST("/v1/remote-control/region", h.switchRegion)
	router.POST("/v1/remote-control/pairings", h.startPairing)
	router.GET("/v1/remote-control/pairings/:pairingID", h.pairingStatus)
	router.GET("/v1/remote-control/clients", h.clients)
	router.DELETE("/v1/remote-control/clients/:clientID", h.revokeClient)
}

func (h *ManagementHandler) status(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	ctx.JSON(http.StatusOK, h.manager.Status())
}

func (h *ManagementHandler) regions(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	regions, err := h.manager.Regions(ctx.Request.Context())
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"regions": regions})
}

func (h *ManagementHandler) enable(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	var input EnableInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}
	status, err := h.manager.Enable(ctx.Request.Context(), input)
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (h *ManagementHandler) disable(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	if err := h.manager.Disable(ctx.Request.Context()); err != nil {
		managementError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *ManagementHandler) switchRegion(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	var input struct {
		RegionID string `json:"regionID"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}
	status, err := h.manager.SwitchRegion(ctx.Request.Context(), input.RegionID)
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (h *ManagementHandler) startPairing(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	pairing, err := h.manager.StartPairing(ctx.Request.Context())
	if err != nil {
		managementError(ctx, err)
		return
	}
	status := h.manager.Status()
	payload := "openbrain://pair?code=" + url.QueryEscape(pairing.Code) + "&region=" + url.QueryEscape(status.RegionID)
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{
		"pairingID": pairing.ID, "code": pairing.Code, "expiresAt": pairing.ExpiresAt,
		"qrPayload": payload, "qrDataURL": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

func (h *ManagementHandler) pairingStatus(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	pairing, err := h.manager.PairingStatus(ctx.Request.Context(), ctx.Param("pairingID"))
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, pairing)
}

func (h *ManagementHandler) clients(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	clients, err := h.manager.Clients(ctx.Request.Context())
	if err != nil {
		managementError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"clients": clients})
}

func (h *ManagementHandler) revokeClient(ctx *gin.Context) {
	if !requireLocalManagementRequest(ctx) {
		return
	}
	if err := h.manager.RevokeClient(ctx.Request.Context(), ctx.Param("clientID")); err != nil {
		managementError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func requireLocalManagementRequest(ctx *gin.Context) bool {
	host, _, err := net.SplitHostPort(ctx.Request.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() || !allowedManagementOrigin(ctx.Request.Header.Get("Origin")) {
		ctx.JSON(http.StatusForbidden, gin.H{"code": "local_management_only"})
		return false
	}
	return true
}

func allowedManagementOrigin(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

func managementError(ctx *gin.Context, err error) {
	code := "remote_control_failed"
	status := http.StatusBadGateway
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "confirmation") || strings.Contains(message, "required") || strings.Contains(message, "disabled"):
		status = http.StatusBadRequest
	case strings.Contains(message, "unavailable"):
		status = http.StatusServiceUnavailable
	}
	ctx.JSON(status, gin.H{"code": code, "message": err.Error()})
}

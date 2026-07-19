package gbrain

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListSources(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListSources(c.Request.Context()))
}

func (h *Handler) Query(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
			Results: []QueryResult{},
		})
		return
	}
	res := h.service.Query(c.Request.Context(), req)
	status := http.StatusOK
	if res.Code == "invalid_request" {
		status = http.StatusBadRequest
	}
	c.JSON(status, res)
}

func (h *Handler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Status(c.Request.Context()))
}

func (h *Handler) CloudListSources(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListOpenBrainSources(c.Request.Context()))
}

func (h *Handler) CachedListSources(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListCachedOpenBrainSources(c.Request.Context()))
}

func (h *Handler) CloudQuery(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Success:  false,
			Code:     "invalid_request",
			Error:    "invalid JSON: " + err.Error(),
			Provider: "cloud",
			Results:  []QueryResult{},
		})
		return
	}
	res := h.service.QueryOpenBrain(c.Request.Context(), req)
	status := http.StatusOK
	if res.Code == "invalid_request" {
		status = http.StatusBadRequest
	}
	c.JSON(status, res)
}

func (h *Handler) CloudCreateSource(c *gin.Context) {
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CreateSourceResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}
	res := h.service.CreateOpenBrainSource(c.Request.Context(), req)
	status := http.StatusOK
	if res.Code == "invalid_request" || res.Code == "invalid_repository" {
		status = http.StatusBadRequest
	} else if res.Code == "workspace_path_conflict" || res.Code == "path_owned_by_other_account" || res.Code == "workspace_repo_mismatch" || res.Code == "workspace_repo_ambiguous" {
		status = http.StatusConflict
	}
	c.JSON(status, res)
}

func (h *Handler) CloudVerifySource(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, VerifySourceResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}
	res := h.service.VerifyOpenBrainSource(c.Request.Context(), req)
	c.JSON(http.StatusOK, res)
}

func (h *Handler) CloudSourceRecoveryCandidates(c *gin.Context) {
	var req recoveryCandidatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, RecoveryCandidatesResponse{
			Success:    false,
			Code:       "invalid_request",
			Error:      "invalid JSON: " + err.Error(),
			Provider:   "cloud",
			Candidates: []RecoveryCandidate{},
		})
		return
	}
	res := h.service.ListOpenBrainSourceRecoveryCandidates(c.Request.Context(), req)
	status := http.StatusOK
	if res.Code == "invalid_request" {
		status = http.StatusBadRequest
	}
	c.JSON(status, res)
}

func (h *Handler) CloudRemoveSourceFromDevice(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MutationResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, h.service.RemoveOpenBrainSourceFromDevice(c.Request.Context(), req))
}

func (h *Handler) CloudArchiveSource(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MutationResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, h.service.ArchiveOpenBrainSource(c.Request.Context(), req))
}

func (h *Handler) CloudSourceAction(c *gin.Context) {
	var req mutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, MutationResponse{
			Success: false,
			Code:    "invalid_request",
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, h.service.ApplyOpenBrainSourceAction(c.Request.Context(), req))
}

func (h *Handler) CloudGetSourceShare(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodGet, cloudSourceShareEndpoint(c))
}

func (h *Handler) CloudShareSourceWithUser(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodPut, cloudSourceShareEndpoint(c)+"/users")
}

func (h *Handler) CloudRevokeSourceUserShare(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodDelete, cloudSourceShareEndpoint(c)+"/users/"+url.PathEscape(c.Param("uid")))
}

func (h *Handler) CloudSetSourcePublic(c *gin.Context) {
	status, payload := h.proxyCloudAPIRaw(c, http.MethodPut, cloudSourceShareEndpoint(c)+"/public")
	if status >= 200 && status < 300 {
		h.service.updateCurrentCloudSourceSnapshotPublicAccess(c.Param("orgID"), c.Param("resourceID"), true)
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

func (h *Handler) CloudRevokeSourcePublic(c *gin.Context) {
	status, payload := h.proxyCloudAPIRaw(c, http.MethodDelete, cloudSourceShareEndpoint(c)+"/public")
	if status >= 200 && status < 300 {
		h.service.updateCurrentCloudSourceSnapshotPublicAccess(c.Param("orgID"), c.Param("resourceID"), false)
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

func (h *Handler) CloudGetPublicBrainProfile(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodGet, "/v1/me/brain/public-profile")
}

func (h *Handler) CloudUpdatePublicBrainProfile(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodPut, "/v1/me/brain/public-profile")
}

func (h *Handler) CloudListPublicBrains(c *gin.Context) {
	endpoint := "/v1/me/brain/public-brains"
	params := url.Values{}
	if query := c.Query("query"); query != "" {
		params.Set("query", query)
	}
	if includeSelf := c.Query("includeSelf"); includeSelf != "" {
		params.Set("includeSelf", includeSelf)
	}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	h.proxyCloudAPI(c, http.MethodGet, endpoint)
}

func (h *Handler) CloudFollowPublicBrain(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodPut, "/v1/me/brain/public-brains/"+url.PathEscape(c.Param("ownerUID"))+"/follow")
}

func (h *Handler) CloudUnfollowPublicBrain(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodDelete, "/v1/me/brain/public-brains/"+url.PathEscape(c.Param("ownerUID"))+"/follow")
}

func (h *Handler) CloudCreatePublicBrainConversation(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodPost, "/v1/public-brains/"+url.PathEscape(c.Param("brainID"))+"/conversations")
}

func (h *Handler) CloudQuotePublicBrainTurn(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodPost, "/v1/public-brains/"+url.PathEscape(c.Param("brainID"))+"/conversations/"+url.PathEscape(c.Param("conversationID"))+"/turn-quotes")
}

func (h *Handler) CloudRunPublicBrainTurn(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "error": err.Error()})
		return
	}
	endpoint := "/v1/public-brains/" + url.PathEscape(c.Param("brainID")) + "/conversations/" + url.PathEscape(c.Param("conversationID")) + "/turns"
	response, err := h.service.OpenCloudAPIStream(c.Request.Context(), http.MethodPost, endpoint, raw)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "cloud_unavailable", "error": err.Error()})
		return
	}
	defer response.Body.Close()
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json; charset=utf-8"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(response.StatusCode)
	flusher, _ := c.Writer.(http.Flusher)
	buffer := make([]byte, 16<<10)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if _, writeErr := c.Writer.Write(buffer[:count]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (h *Handler) proxyCloudAPI(c *gin.Context, method string, endpoint string) {
	status, payload := h.proxyCloudAPIRaw(c, method, endpoint)
	c.Data(status, "application/json; charset=utf-8", payload)
}

func (h *Handler) proxyCloudAPIRaw(c *gin.Context, method string, endpoint string) (int, []byte) {
	var raw []byte
	if c.Request.Body != nil && method != http.MethodGet {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			return http.StatusBadRequest, []byte(`{"success":false,"code":"invalid_request","error":` + strconv.Quote(err.Error()) + `}`)
		}
		raw = body
	}
	return h.service.ProxyCloudAPI(c.Request.Context(), method, endpoint, raw)
}

func cloudSourceShareEndpoint(c *gin.Context) string {
	return "/v1/orgs/" + url.PathEscape(c.Param("orgID")) +
		"/resources/" + url.PathEscape(c.Param("resourceID")) +
		"/source-share"
}

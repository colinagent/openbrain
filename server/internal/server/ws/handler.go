package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	sharedmarketplace "github.com/colinagent/openbrain/opagent-runtime/marketplace"
	serverarchive "github.com/colinagent/openbrain/server/internal/server/archive"
	"github.com/colinagent/openbrain/server/internal/server/fs"
	gitservice "github.com/colinagent/openbrain/server/internal/server/git"
	"github.com/colinagent/openbrain/server/internal/server/protocol"
	storageservice "github.com/colinagent/openbrain/server/internal/server/storage"
	"github.com/rs/xid"
)

// Handler handles JSON-RPC requests
type Handler struct {
	server      *Server
	verbose     bool
	fs          *fs.FileService
	git         *gitservice.Service
	storage     *storageservice.Service
	cmd         *commandManager
	archive     cleanupRunner
	marketplace marketplaceService
}

type cleanupRunner interface {
	Run(context.Context, protocol.ArchiveCleanupParams) (*protocol.ArchiveCleanupResult, error)
}

type marketplaceService interface {
	ListItems(context.Context, bool, sharedmarketplace.UsageReport) (*sharedmarketplace.ListResult, error)
	ListItemsForOrg(context.Context, string, bool, sharedmarketplace.UsageReport) (*sharedmarketplace.ListResult, error)
	Refresh(context.Context, sharedmarketplace.UsageReport) (*sharedmarketplace.ListResult, error)
	RefreshForOrg(context.Context, string, sharedmarketplace.UsageReport) (*sharedmarketplace.ListResult, error)
	GetState(context.Context) (*sharedmarketplace.StateResult, error)
	ListOrgs(context.Context) (*sharedmarketplace.OrgListResult, error)
	InstallItem(context.Context, sharedmarketplace.Kind, string, sharedmarketplace.UsageReport) (*sharedmarketplace.ActionResult, error)
	InstallOrgItem(context.Context, string, sharedmarketplace.Kind, string, sharedmarketplace.UsageReport) (*sharedmarketplace.ActionResult, error)
	UpdateItem(context.Context, sharedmarketplace.Kind, string, sharedmarketplace.UsageReport) (*sharedmarketplace.ActionResult, error)
	UpdateOrgItem(context.Context, string, sharedmarketplace.Kind, string, sharedmarketplace.UsageReport) (*sharedmarketplace.ActionResult, error)
}

// NewHandler creates a new request handler
func NewHandler(server *Server, verbose bool) *Handler {
	return &Handler{
		server:  server,
		verbose: verbose,
		fs:      fs.NewFileService(verbose),
		git:     gitservice.NewService(),
		storage: storageservice.NewService(),
		cmd:     newCommandManager(),
		archive: serverarchive.NewService(serverarchive.NewHostCoreClient(server)),
	}
}

func (h *Handler) SetMarketplaceService(service marketplaceService) {
	h.marketplace = service
}

// HandleMessage processes an incoming JSON-RPC message
func (h *Handler) HandleMessage(client *Client, message []byte) []byte {
	var req protocol.Request
	if err := json.Unmarshal(message, &req); err != nil {
		if h.verbose {
			log.Printf("JSON parse error: %v", err)
		}
		return h.errorResponse(nil, protocol.ErrCodeParse, "Parse error", nil)
	}

	if req.JSONRPC != "2.0" {
		return h.errorResponse(req.ID, protocol.ErrCodeInvalidRequest, "Invalid Request: jsonrpc must be 2.0", nil)
	}

	if h.verbose {
		log.Printf("Request: method=%s id=%v", req.Method, req.ID)
	}

	// Route to appropriate handler
	result, rpcErr := h.dispatch(client, req.Method, req.Params)

	// If this is a notification (no ID), don't send response
	if req.ID == nil {
		return nil
	}

	if rpcErr != nil {
		return h.errorResponse(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
	}

	return h.successResponse(req.ID, result)
}

// dispatch routes request to the appropriate handler
func (h *Handler) dispatch(client *Client, method string, params json.RawMessage) (interface{}, *protocol.RPCError) {
	switch method {
	// File system methods
	case protocol.MethodFSStat:
		return h.handleStat(params)
	case protocol.MethodFSReadFile:
		return h.handleReadFile(params)
	case protocol.MethodFSWriteFile:
		return h.handleWriteFile(params)
	case protocol.MethodFSReaddir:
		return h.handleReaddir(params)
	case protocol.MethodFSSearch:
		return h.handleSearch(params)
	case protocol.MethodFSMkdir:
		return h.handleMkdir(params)
	case protocol.MethodFSDelete:
		return h.handleDelete(params)
	case protocol.MethodFSRename:
		return h.handleRename(params)
	case protocol.MethodFSCopy:
		return h.handleCopy(params)
	case protocol.MethodFSWatch:
		return h.handleWatch(client, params)
	case protocol.MethodFSUnwatch:
		return h.handleUnwatch(client, params)
	// Agent methods
	// case protocol.MethodAgentGet:
	// 	return h.handleAgentGet(params)
	case protocol.MethodAgentScan:
		return h.handleAgentScan(params)
	// case protocol.MethodNodes:
	// 	return h.handleNodes(params)
	case protocol.MethodNodeList:
		return h.handleNodeList(params)

	case protocol.MethodConfigSystemGet:
		return h.handleConfigSystemGet(params)
	case protocol.MethodConfigPush:
		return h.handleConfigPush(params)
	case protocol.MethodGitBranches:
		return h.handleGitBranches(params)
	case protocol.MethodGitCheckout:
		return h.handleGitCheckout(params)
	case protocol.MethodThreadReviewList:
		return h.handleThreadReviewList(params)
	case protocol.MethodThreadReviewResolve:
		return h.handleThreadReviewResolve(params)
	case protocol.MethodThreadReviewRollback:
		return h.handleThreadReviewRollback(params)
	case protocol.MethodEditorCompletion:
		return h.handleEditorCompletion(params)
	case protocol.MethodEditorCompletionCancel:
		return h.handleEditorCompletionCancel(params)
	case protocol.MethodEditorRandomID:
		return h.handleEditorRandomID()
	case protocol.MethodCommandExec:
		return h.handleCommandExec(client, params)
	case protocol.MethodCommandStop:
		return h.handleCommandStop(params)
	case protocol.MethodArchiveCleanupRun:
		return h.handleArchiveCleanupRun(params)
	case protocol.MethodCronList:
		return h.handleCronList(params)
	case protocol.MethodCronGet:
		return h.handleCronGet(params)
	case protocol.MethodCronAdd:
		return h.handleCronAdd(params)
	case protocol.MethodCronUpsert:
		return h.handleCronUpsert(params)
	case protocol.MethodCronUpdate:
		return h.handleCronUpdate(params)
	case protocol.MethodCronRemove:
		return h.handleCronRemove(params)
	case protocol.MethodCronRun:
		return h.handleCronRun(params)
	case protocol.MethodCronHistory:
		return h.handleCronHistory(params)
	case protocol.MethodMarketplaceList:
		return h.handleMarketplaceList(params)
	case protocol.MethodMarketplaceRefresh:
		return h.handleMarketplaceRefresh(params)
	case protocol.MethodMarketplaceState:
		return h.handleMarketplaceState(params)
	case protocol.MethodMarketplaceInstall:
		return h.handleMarketplaceInstall(params)
	case protocol.MethodMarketplaceUpdate:
		return h.handleMarketplaceUpdate(params)
	case protocol.MethodMarketplaceOrgs:
		return h.handleMarketplaceOrgs(params)
	case protocol.MethodStorageStatus:
		return h.handleStorageStatus(params)
	case protocol.MethodStorageSyncNow:
		return h.handleStorageSyncNow(params)
	case protocol.MethodStorageUpdatePolicy:
		return h.handleStorageUpdatePolicy(params)
	case protocol.MethodMessengerList:
		return h.handleMessengerList(params)
	case protocol.MethodMessengerChannel:
		return h.handleMessengerChannel(params)
	case protocol.MethodMessengerReply:
		return h.handleMessengerReply(params)
	case protocol.MethodMessengerMarkRead:
		return h.handleMessengerMarkRead(params)
	case protocol.MethodMessengerArchive:
		return h.handleMessengerArchive(params)
	// case protocol.MethodAgentsRoots:
	// 	return h.handleAgentsRoots(params)
	// Session methods
	// case protocol.MethodSessionSet:
	// 	return h.handleSessionSet(params)
	// case protocol.MethodSessionClear:
	// 	return h.handleSessionClear(params)
	// case protocol.MethodSessionGet:
	// 	return h.handleSessionGet(params)
	default:
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeMethodNotFound,
			Message: "Method not found: " + method,
		}
	}
}

// File system handlers

func (h *Handler) handleStat(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.StatParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.fs.Stat(&p)
}

func (h *Handler) handleReadFile(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.ReadFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.fs.ReadFile(&p)
}

func (h *Handler) handleWriteFile(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.WriteFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.fs.WriteFile(&p)
}

func (h *Handler) handleReaddir(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.ReaddirParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.fs.Readdir(&p)
}

func (h *Handler) handleSearch(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.SearchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.fs.Search(context.Background(), &p)
}

func (h *Handler) handleMkdir(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.MkdirParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	if rpcErr := h.fs.Mkdir(&p); rpcErr != nil {
		return nil, rpcErr
	}
	return map[string]bool{"success": true}, nil
}

func (h *Handler) handleDelete(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.DeleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	if rpcErr := h.fs.Delete(&p); rpcErr != nil {
		return nil, rpcErr
	}
	return map[string]bool{"success": true}, nil
}

func (h *Handler) handleRename(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.RenameParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	if rpcErr := h.fs.Rename(&p); rpcErr != nil {
		return nil, rpcErr
	}
	return map[string]bool{"success": true}, nil
}

func (h *Handler) handleCopy(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.CopyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	if rpcErr := h.fs.Copy(&p); rpcErr != nil {
		return nil, rpcErr
	}
	return map[string]bool{"success": true}, nil
}

func (h *Handler) handleThreadReviewList(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.ThreadReviewListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpThreadReviewList, p)
}

func (h *Handler) handleThreadReviewResolve(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.ThreadReviewResolveParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpThreadReviewResolve, p)
}

func (h *Handler) handleThreadReviewRollback(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.ThreadReviewRollbackParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpThreadReviewRollback, p)
}

func (h *Handler) handleEditorCompletion(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.EditorCompletionRequest
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpEditorCompletion, p)
}

func (h *Handler) handleEditorCompletionCancel(params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p op.EditorCompletionCancelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}
	return h.callHostNode(op.OpEditorCompletionCancel, p)
}

func (h *Handler) handleEditorRandomID() (interface{}, *protocol.RPCError) {
	return protocol.EditorRandomIDResult{ID: xid.New().String()}, nil
}

func (h *Handler) callHostNode(opCode op.OpCode, payload any) (interface{}, *protocol.RPCError) {
	session := h.server.GetHostSession()
	if session == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Host session not initialized",
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to encode request: " + err.Error(),
		}
	}
	res, err := session.OpNode(context.Background(), &op.OpNodeParams{
		OpCode:  opCode,
		Content: &op.JsonContent{Raw: raw},
	})
	if err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}
	}
	if res == nil {
		return nil, nil
	}
	jsonContent, ok := res.Content.(*op.JsonContent)
	if !ok || jsonContent == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Host returned non-json content",
		}
	}
	var result any
	if err := json.Unmarshal(jsonContent.Raw, &result); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to decode host result: " + err.Error(),
		}
	}
	return result, nil
}

func (h *Handler) handleWatch(client *Client, params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.WatchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}

	watcher := h.server.GetWatcher()
	if watcher == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "File watcher not available",
		}
	}

	// Generate watch ID
	watchID := generateWatchID()

	// Create subscription
	sub := &fs.WatchSubscription{
		ID:        watchID,
		Path:      p.Path,
		Recursive: p.Recursive,
		Excludes:  p.Excludes,
		Client:    client,
	}

	if err := watcher.Watch(sub); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to watch: " + err.Error(),
		}
	}

	// Track watch in client
	client.AddWatch(watchID)

	return &protocol.WatchResult{WatchID: watchID}, nil
}

func (h *Handler) handleUnwatch(client *Client, params json.RawMessage) (interface{}, *protocol.RPCError) {
	var p protocol.UnwatchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInvalidParams,
			Message: "Invalid params: " + err.Error(),
		}
	}

	watcher := h.server.GetWatcher()
	if watcher == nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "File watcher not available",
		}
	}

	// Remove watch from client
	client.RemoveWatch(p.WatchID)

	// Remove subscription
	if err := watcher.Unwatch(p.WatchID); err != nil {
		return nil, &protocol.RPCError{
			Code:    protocol.ErrCodeInternal,
			Message: "Failed to unwatch: " + err.Error(),
		}
	}

	return map[string]bool{"success": true}, nil
}

func (h *Handler) handleMarketplaceList(params json.RawMessage) (interface{}, *protocol.RPCError) {
	service := h.marketplace
	if service == nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Marketplace service not available"}
	}
	var p protocol.MarketplaceListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	var result *sharedmarketplace.ListResult
	var err error
	if strings.TrimSpace(p.OrgID) != "" {
		result, err = service.ListItemsForOrg(context.Background(), strings.TrimSpace(p.OrgID), p.Force, toMarketplaceUsageReport(p.Usage))
	} else {
		result, err = service.ListItems(context.Background(), p.Force, toMarketplaceUsageReport(p.Usage))
	}
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
	}
	return result, nil
}

func (h *Handler) handleMarketplaceRefresh(params json.RawMessage) (interface{}, *protocol.RPCError) {
	service := h.marketplace
	if service == nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Marketplace service not available"}
	}
	var p protocol.MarketplaceListParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
		}
	}
	var result *sharedmarketplace.ListResult
	var err error
	if strings.TrimSpace(p.OrgID) != "" {
		result, err = service.RefreshForOrg(context.Background(), strings.TrimSpace(p.OrgID), toMarketplaceUsageReport(p.Usage))
	} else {
		result, err = service.Refresh(context.Background(), toMarketplaceUsageReport(p.Usage))
	}
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
	}
	return result, nil
}

func (h *Handler) handleMarketplaceOrgs(params json.RawMessage) (interface{}, *protocol.RPCError) {
	service := h.marketplace
	if service == nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Marketplace service not available"}
	}
	if len(params) > 0 && strings.TrimSpace(string(params)) != "{}" {
		var raw map[string]any
		if err := json.Unmarshal(params, &raw); err != nil {
			return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
		}
	}
	result, err := service.ListOrgs(context.Background())
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
	}
	return result, nil
}

func (h *Handler) handleMarketplaceState(params json.RawMessage) (interface{}, *protocol.RPCError) {
	service := h.marketplace
	if service == nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Marketplace service not available"}
	}
	if len(params) > 0 && strings.TrimSpace(string(params)) != "{}" {
		var raw map[string]any
		if err := json.Unmarshal(params, &raw); err != nil {
			return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
		}
	}
	result, err := service.GetState(context.Background())
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
	}
	return result, nil
}

func (h *Handler) handleMarketplaceInstall(params json.RawMessage) (interface{}, *protocol.RPCError) {
	return h.handleMarketplaceAction(params, true)
}

func (h *Handler) handleMarketplaceUpdate(params json.RawMessage) (interface{}, *protocol.RPCError) {
	return h.handleMarketplaceAction(params, false)
}

func (h *Handler) handleMarketplaceAction(params json.RawMessage, install bool) (interface{}, *protocol.RPCError) {
	service := h.marketplace
	if service == nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: "Marketplace service not available"}
	}
	var p protocol.MarketplaceItemParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	kind := sharedmarketplace.Kind(strings.TrimSpace(p.Kind))
	if !isMarketplaceKind(kind) || strings.TrimSpace(p.ID) == "" {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInvalidParams, Message: "Marketplace kind and id are required"}
	}
	var (
		result *sharedmarketplace.ActionResult
		err    error
	)
	if install {
		if strings.TrimSpace(p.OrgID) != "" {
			result, err = service.InstallOrgItem(context.Background(), strings.TrimSpace(p.OrgID), kind, p.ID, toMarketplaceUsageReport(p.Usage))
		} else {
			result, err = service.InstallItem(context.Background(), kind, p.ID, toMarketplaceUsageReport(p.Usage))
		}
	} else {
		if strings.TrimSpace(p.OrgID) != "" {
			result, err = service.UpdateOrgItem(context.Background(), strings.TrimSpace(p.OrgID), kind, p.ID, toMarketplaceUsageReport(p.Usage))
		} else {
			result, err = service.UpdateItem(context.Background(), kind, p.ID, toMarketplaceUsageReport(p.Usage))
		}
	}
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.ErrCodeInternal, Message: err.Error()}
	}
	return result, nil
}

func toMarketplaceUsageReport(raw protocol.MarketplaceUsageReport) sharedmarketplace.UsageReport {
	return sharedmarketplace.UsageReport{Agents: raw.Agents, Skills: raw.Skills, Tools: raw.Tools}
}

func isMarketplaceKind(kind sharedmarketplace.Kind) bool {
	switch kind {
	case sharedmarketplace.KindAgent, sharedmarketplace.KindSkill, sharedmarketplace.KindTool:
		return true
	default:
		return false
	}
}

// JSON-RPC response helpers

func (h *Handler) successResponse(id interface{}, result interface{}) []byte {
	resp := protocol.NewResponse(id, result)
	data, _ := json.Marshal(resp)
	return data
}

func (h *Handler) errorResponse(id interface{}, code int, message string, data interface{}) []byte {
	resp := protocol.NewErrorResponse(id, code, message, data)
	result, _ := json.Marshal(resp)
	return result
}

// generateWatchID creates a unique ID for watch subscriptions
func generateWatchID() string {
	return fmt.Sprintf("watch_%d", time.Now().UnixNano())
}

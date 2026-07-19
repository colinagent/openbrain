package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
)

const unauthorizedError = "unauthorized: please sign in first"

// logCachedNodeIDsForDiagnostics logs existing node IDs when a requested node is not found,
// so operators can verify whether the agent exists in the current scan (BaseDir/UID).
// func logCachedNodeIDsForDiagnostics(requestedID, handler string) {
// 	nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
// 	ids := make([]string, 0, len(nodes))
// 	for _, n := range nodes {
// 		name := ""
// 		if n.Kind == string(op.NodeKindAgent) {
// 			if m, ok := n.Meta.(*op.AgentMeta); ok && m != nil {
// 				name = m.Name
// 			}
// 		}
// 		if name != "" {
// 			ids = append(ids, n.ID+" ("+name+")")
// 		} else {
// 			ids = append(ids, n.ID)
// 		}
// 	}
// 	slog.Warn("node not found; current cached node IDs",
// 		"requestedID", requestedID,
// 		"handler", handler,
// 		"cachedCount", len(ids),
// 		"cachedIDs", ids,
// 	)
// }

func OpNodeHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	switch req.Params.OpCode {

	// // agent
	// case op.OpAgentUpsert:
	// 	return OpAgentUpsertHandler(ctx, req)
	// case op.OpAgentList:
	// 	return OpAgentListHandler(ctx, req)
	// case op.OpAgentScan:
	// 	return OpAgentScanHandler(ctx, req)
	// case op.OpNodeScan:
	// 	return OpNodeScanHandler(ctx, req)
	// case op.OpNodeList:
	// 	return OpNodeListHandler(ctx, req)
	// case op.OpNodeCached:
	// 	return OpNodeCachedHandler(ctx, req)
	// case op.OpAgentRoots:
	// 	return OpAgentRootsHandler(ctx, req)
	// case op.OpAgentGet:
	// 	return OpAgentGetHandler(ctx, req)

	case op.OpNodeList:
		return OpNodeListHandler(ctx, req)
	case op.OpAgentScan:
		return OpAgentScanHandler(ctx, req)
		// legacy chat edge adapters
	case op.OpAgentLoopCreate:
		return OpAgentLoopCreateHandler(ctx, req)
	case op.OpThreadCreate:
		return OpThreadCreateHandler(req)
	case op.OpThreadFork:
		return OpThreadForkHandler(req)
	case op.OpThreadMetaGet:
		return OpThreadMetaGetHandler(req)
	case op.OpThreadMetaUpdate:
		return OpThreadMetaUpdateHandler(req)
	case op.OpThreadSnapshotGet:
		return OpThreadSnapshotGetHandler(req)
	case op.OpThreadReviewList:
		return OpThreadReviewListHandler(req)
	case op.OpThreadReviewResolve:
		return OpThreadReviewResolveHandler(req)
	case op.OpThreadReviewRollback:
		return OpThreadReviewRollbackHandler(req)
	case op.OpEditorCompletion:
		return OpEditorCompletionHandler(ctx, req)
	case op.OpEditorCompletionCancel:
		return OpEditorCompletionCancelHandler(req)
	case op.OpRuntimeEvidenceAnswer:
		return OpRuntimeEvidenceAnswerHandler(ctx, req)
	case op.OpThreadSubmit:
		return OpThreadSubmitHandler(ctx, req)
	case cronOpList:
		return CronListHandler(ctx, req)
	case cronOpGet:
		return CronGetHandler(ctx, req)
	case cronOpAdd:
		return CronAddHandler(ctx, req)
	case cronOpUpsert:
		return CronUpsertHandler(ctx, req)
	case cronOpUpdate:
		return CronUpdateHandler(ctx, req)
	case cronOpRemove:
		return CronRemoveHandler(ctx, req)
	case cronOpRun:
		return CronRunHandler(ctx, req)
	case cronOpHistory:
		return CronHistoryHandler(ctx, req)

	// case op.OpThreadUpsert:
	// 	return OpThreadUpsertHandler(ctx, req)

	// //user
	// case op.OpUserProfileGet:
	// 	return OpUserProfileGetHandler(ctx, req)
	// case op.OpUserProfileUpsert:
	// 	return OpUserProfileUpsertHandler(ctx, req)
	// case op.OpUserAgentList:
	// 	return OpUserAgentListHandler(ctx, req)

	// //tool server
	// case op.OpToolCall:
	// 	return OpToolCallHandler(ctx, req)
	case op.ConfigSystemGet:
		return ConfigSystemGetHandler(ctx, req)
	case op.ConfigGet:
		return ConfigGetHandler(ctx, req)
	// case op.HostSecretGet:
	// 	return HostSecretGetHandler(ctx, req)

	default:
		return nil, fmt.Errorf("unknown op code: %s", req.Params.OpCode)
	}

}

func OpNodeListHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	if config.GetSystem().Env == op.EnvLocal {
		if req != nil && req.Params.Meta != nil {
			if refresh, ok := req.Params.Meta["refresh"].(bool); ok && refresh {
				if err := RefreshNodeCache(ctx, scan.ScanOptions{
					UID:     op.LocalUser,
					BaseDir: config.GetSystem().BaseDir,
				}); err != nil {
					return nil, fmt.Errorf("refresh node cache: %w", err)
				}
			}
		}
		nodes := cache.ListValuesByPrefix[op.OpNode](cache.PrefixNode)
		raw, err := json.Marshal(nodes)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal nodes: %w", err)
		}
		return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
	}
	return nil, fmt.Errorf("env is not supported: %s", config.GetSystem().Env)
}

func OpAgentScanHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	if req.Params.Meta == nil {
		return nil, fmt.Errorf("meta is required")
	}
	dir, _ := req.Params.Meta["dir"].(string)
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}

	if config.GetSystem().Env == op.EnvLocal {
		s := scan.NewScanner(op.LocalUser, dir).
			WithPathAwareAgentDedup().
			WithRefBaseDir(config.GetSystem().BaseDir).
			WithNodeIndexBaseDir(config.GetSystem().BaseDir)
		nodes := s.ScanAgents(dir, 0)
		if err := s.Err(); err != nil {
			return nil, fmt.Errorf("scan agents: %w", err)
		}
		raw, err := json.Marshal(nodes)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal nodes: %w", err)
		}
		return &op.OpNodeResult{Content: &op.JsonContent{Raw: raw}}, nil
	}
	return nil, fmt.Errorf("env is not supported: %s", config.GetSystem().Env)

}

// func OpNodeCallHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
// 	ctx, cancel := context.WithCancel(ctx)
// 	defer cancel()
// 	nodeID, _ := req.Params.Meta["nodeID"].(string)
// 	if nodeID == "" {
// 		return nil, fmt.Errorf("nodeID is required")
// 	}
// 	node := cache.Get[op.OpNode](nodeID, cache.PrefixNode)
// 	if node == nil {
// 		return nil, fmt.Errorf("node not found: %s", nodeID)
// 	}

// 	if node.Run.HasEndpoint() {
// 		conn, err := EnsureConnection(ctx, node)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get connection: %w", err)
// 		}
// 		if conn == nil {
// 			return nil, fmt.Errorf("connection is nil")
// 		}
// 		callRes, err := conn.CallNode(ctx, req.Params.Meta, req.Params.Content)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to call node: %w", err)
// 		}
// 		return &op.OpNodeResult{
// 			Content: callRes.Content,
// 			Meta:    callRes.Meta,
// 		}, nil
// 	}

// 	// if is agent, run loop
// 	return OpAgentLoopCreateHandler(ctx, req)

// }

func ConfigGetHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {

	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func ConfigSystemGetHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {

	cfg := config.GetSystem()
	if cfg == nil {
		return nil, fmt.Errorf("system config is nil")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal host config: %w", err)
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

// func OpThreadUpsertHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	var thread *op.ThreadStorage
// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not a json content")
// 	}
// 	if err := json.Unmarshal(jsonContent.Raw, &thread); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
// 	}
// 	if thread.ThreadID == "" {
// 		return nil, fmt.Errorf("threadID is required")
// 	}
// 	err := GetStorage().UpsertThreadStorage(ctx, thread)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to upsert thread: %w", err)
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.TextContent{Text: "thread added: " + thread.ThreadID},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// // OpAgentUpsertHandler upserts an agent node into nodestore.
// func OpAgentUpsertHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not a json content")
// 	}
// 	var node *op.OpNode
// 	if err := json.Unmarshal(jsonContent.Raw, &node); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal agent node: %w", err)
// 	}
// 	store := nodestore.GetDefault()
// 	if store == nil {
// 		return nil, fmt.Errorf("nodestore not initialized")
// 	}
// 	if err := store.Upsert(ctx, node); err != nil {
// 		return nil, fmt.Errorf("failed to upsert agent node: %w", err)
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.TextContent{Text: "agent node upserted: " + node.ID},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// // OpAgentGetHandler retrieves an agent node by ID.
// func OpAgentGetHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	agentID := ""
// 	if req.Params.Meta != nil {
// 		if v, ok := req.Params.Meta["agentID"].(string); ok && v != "" {
// 			agentID = v
// 		}
// 	}
// 	if agentID == "" {
// 		return nil, fmt.Errorf("agentID is required")
// 	}

// 	node, _, err := getAgentNode(ctx, agentID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get agent node: %w", err)
// 	}
// 	jsonBytes, err := json.Marshal(node)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to marshal agent node: %w", err)
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: jsonBytes},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpUserProfileGetHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not json content")
// 	}
// 	var settingsReq op.UserSettings
// 	if err := json.Unmarshal(jsonContent.Raw, &settingsReq); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal user settings: %w", err)
// 	}
// 	settingsReq.UID = strings.TrimSpace(settingsReq.UID)
// 	if settingsReq.UID == "" || strings.EqualFold(settingsReq.UID, "anonymous") {
// 		return nil, fmt.Errorf(unauthorizedError)
// 	}
// 	settings, err := GetStorage().GetUserSettings(ctx, settingsReq.UID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	raw, err := json.Marshal(settings)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: raw},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpUserProfileUpsertHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not json content")
// 	}
// 	var settings op.UserSettings
// 	if err := json.Unmarshal(jsonContent.Raw, &settings); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal user settings: %w", err)
// 	}
// 	settings.UID = strings.TrimSpace(settings.UID)
// 	if settings.UID == "" || strings.EqualFold(settings.UID, "anonymous") {
// 		return nil, fmt.Errorf(unauthorizedError)
// 	}
// 	if err := GetStorage().UpsertUserSettings(ctx, &settings); err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.TextContent{Text: "user settings upserted: " + settings.UID},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// // OpUserAgentListHandler lists agent nodes for a specific uid.
// func OpUserAgentListHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	uid, err := resolveRequestUID(req.Params.Meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	nodes, err := listAgentNodesByUID(ctx, uid)
// 	if err != nil {
// 		return nil, err
// 	}
// 	raw, err := json.Marshal(nodes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: raw},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func upsertThreadUID(ctx context.Context, uid, threadID string) error {
// 	now := time.Now().UnixMilli()
// 	thread, err := GetStorage().GetThreadStorage(ctx, threadID)
// 	if err != nil {
// 		thread = &op.ThreadStorage{
// 			ThreadID:  threadID,
// 			UID:       uid,
// 			CreatedAt: now,
// 			UpdatedAt: now,
// 		}
// 		return GetStorage().UpsertThreadStorage(ctx, thread)
// 	}
// 	thread.UID = uid
// 	thread.UpdatedAt = now
// 	return GetStorage().UpsertThreadStorage(ctx, thread)
// }

// func resolveDefaultAgentID(ctx context.Context, meta op.Meta) string {
// 	uid := ""
// 	if meta != nil {
// 		if v, ok := meta["uid"].(string); ok && v != "" {
// 			uid = v
// 		}
// 	}
// 	if uid == "" {
// 		return ""
// 	}
// 	settings, err := GetStorage().GetUserSettings(ctx, uid)
// 	if err == nil && settings != nil && settings.DefaultAgentID != "" {
// 		return settings.DefaultAgentID
// 	}
// 	if settings == nil {
// 		cfg := config.Get()
// 		if cfg != nil {
// 			settings = &op.UserSettings{
// 				UID:     uid,
// 				BaseDir: cfg.BaseDir,
// 			}
// 		}
// 	}
// 	if settings == nil || settings.BaseDir == "" {
// 		return ""
// 	}

// 	// Find default agent by path or name from nodestore
// 	defaultPath := filepath.Clean(filepath.Join(settings.BaseDir, "agents", "li"))
// 	nodes, err := listAgentNodes(ctx)
// 	if err != nil {
// 		return ""
// 	}
// 	for _, node := range nodes {
// 		agentRoot := agentRootFromNode(node)
// 		if agentRoot != "" && filepath.Clean(agentRoot) == defaultPath {
// 			return node.ID
// 		}
// 	}
// 	// Fallback: find by name
// 	for _, node := range nodes {
// 		agentMeta, ok := nodestore.NodeMeta[op.AgentMeta](node)
// 		if !ok {
// 			continue
// 		}
// 		if agentMeta.Name == "li" {
// 			return node.ID
// 		}
// 	}
// 	return ""
// }

// func updateDefaultAgent(ctx context.Context, meta op.Meta, agentID string) {
// 	if agentID == "" {
// 		return
// 	}
// 	uid := ""
// 	if meta != nil {
// 		if v, ok := meta["uid"].(string); ok && v != "" {
// 			uid = v
// 		}
// 	}
// 	if uid == "" {
// 		return
// 	}
// 	settings, err := GetStorage().GetUserSettings(ctx, uid)
// 	if err != nil || settings == nil {
// 		cfg := config.Get()
// 		settings = &op.UserSettings{
// 			UID:     uid,
// 			BaseDir: cfg.BaseDir,
// 		}
// 	}
// 	settings.DefaultAgentID = agentID
// 	if err := GetStorage().UpsertUserSettings(ctx, settings); err != nil {
// 		slog.Warn("failed to update default agent", "error", err, "uid", uid)
// 	}
// }

// func OpThreadQueryHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not a json content")
// 	}
// 	var query *op.ThreadStorageQuery
// 	if err := json.Unmarshal(jsonContent.Raw, &query); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
// 	}
// 	threads, total, err := GetStorage().QueryThreadStorage(ctx, query)
// 	if err != nil {
// 		return nil, err
// 	}
// 	result := &op.ThreadStorageQueryResult{
// 		Threads: threads,
// 		Total:   total,
// 		Limit:   query.Limit,
// 		Offset:  query.Offset,
// 	}
// 	resultBytes, err := json.Marshal(result)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: resultBytes},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpToolCallHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	serverID, _ := req.Params.Meta["toolServerID"].(string)
// 	if serverID == "" {
// 		serverName, _ := req.Params.Meta["toolServerName"].(string)
// 		if serverName == "" {
// 			return nil, fmt.Errorf("toolServerID is required")
// 		}
// 		node, _, err := findToolsNodeByName(ctx, serverName)
// 		if err != nil {
// 			return nil, err
// 		}
// 		serverID = node.ID
// 	}
// 	conn, err := GetConnection(serverID)
// 	if err != nil {
// 		slog.Error("failed to get tool server conn", "error", err, "serverID", serverID)
// 		return nil, err
// 	}
// 	if conn.Session == nil {
// 		slog.Error("tool server conn session is nil", "serverID", serverID)
// 		return nil, fmt.Errorf("tool server conn session is nil")
// 	}

// 	jsonContent, ok := req.Params.Content.(*op.JsonContent)
// 	if !ok {
// 		return nil, fmt.Errorf("content is not a json raw content")
// 	}
// 	var callToolParams op.CallToolParams
// 	if err := json.Unmarshal(jsonContent.Raw, &callToolParams); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal call tool: %w", err)
// 	}

// 	res, err := conn.Session.CallTool(ctx, &op.CallToolParams{
// 		Meta:      req.Params.Meta,
// 		Name:      callToolParams.Name,
// 		Arguments: callToolParams.Arguments,
// 	})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to call tool: %w", err)
// 	}

// 	if len(res.Content) == 0 {
// 		res.Content = []op.Content{&op.TextContent{Text: "no content"}}
// 	}
// 	return &op.OpAgentResult{
// 		Content: res.Content[0],
// 		Meta:    res.Meta,
// 	}, nil
// }

func OpAgentLoopCreateHandler(ctx context.Context, req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if req.Params.Meta == nil {
		slog.Error("meta is required")
		return nil, fmt.Errorf("meta is required")
	}

	nodeID, _ := req.Params.Meta["agentID"].(string)
	if nodeID == "" {
		slog.Error("agentID is required")
		return nil, fmt.Errorf("agentID is required")
	}

	nodeVal, ok := cache.GetValue[op.OpNode](nodeID, cache.PrefixNode)
	if !ok {
		slog.Error("node not found", "agentID", nodeID)
		return nil, fmt.Errorf("node not found")
	}
	nodeVal = refreshFileBackedAgentNode(nodeVal)
	node := &nodeVal
	if node.Kind != string(op.NodeKindAgent) {
		slog.Error("node is not an agent", "agentID", nodeID)
		return nil, fmt.Errorf("node is not an agent")
	}
	agentLoop, err := NewAgentLoop(ctx, node, req.Params.Meta, req.Params.Content)
	if err != nil {
		slog.Error("failed to create agent loop", "error", err, "agentID", nodeID)
		return nil, fmt.Errorf("failed to create agent loop: %w", err)
	}

	return agentLoop.run()
}

// // OpAgentListHandler lists all agent nodes from nodestore.
// func OpAgentListHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	nodes, err := listAgentNodes(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	jsonBytes, err := json.Marshal(nodes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: jsonBytes},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// // OpAgentScanHandler triggers a rescan of agents/tools/skills.
// func OpAgentScanHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	settings, err := resolveUserSettingsForMeta(ctx, req.Params.Meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if _, err := agentload.RefreshNodes(ctx, agentload.ScanOptions{
// 		UID:     settings.UID,
// 		BaseDir: settings.BaseDir,
// 	}); err != nil {
// 		return nil, err
// 	}
// 	result, err := json.Marshal(map[string]any{
// 		"ok":      true,
// 		"uid":     settings.UID,
// 		"baseDir": settings.BaseDir,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: result},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpNodeListHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	settings, err := resolveUserSettingsForMeta(ctx, req.Params.Meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	uid := settings.UID
// 	if req.Params.Meta != nil {
// 		if v, ok := req.Params.Meta["uid"].(string); ok && v != "" {
// 			uid = v
// 		}
// 	}
// 	refresh := true
// 	if req.Params.Meta != nil {
// 		if v, ok := req.Params.Meta["refresh"].(bool); ok {
// 			refresh = v
// 		}
// 	}
// 	if refresh {
// 		if _, err := agentload.RefreshNodes(ctx, agentload.ScanOptions{
// 			UID:     uid,
// 			BaseDir: settings.BaseDir,
// 		}); err != nil {
// 			return nil, err
// 		}
// 	}
// 	nodes, err := listNodesByUID(ctx, uid)
// 	if err != nil {
// 		return nil, err
// 	}
// 	raw, err := json.Marshal(nodes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: raw},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpNodeCachedHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	settings, err := resolveUserSettingsForMeta(ctx, req.Params.Meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	uid := settings.UID
// 	if req.Params.Meta != nil {
// 		if v, ok := req.Params.Meta["uid"].(string); ok && v != "" {
// 			uid = v
// 		}
// 	}
// 	nodes, err := listNodesByUID(ctx, uid)
// 	if err != nil {
// 		return nil, err
// 	}
// 	raw, err := json.Marshal(nodes)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: raw},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func OpNodeScanHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	settings, err := resolveUserSettingsForMeta(ctx, req.Params.Meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	uid := settings.UID
// 	if req.Params.Meta != nil {
// 		if v, ok := req.Params.Meta["uid"].(string); ok && v != "" {
// 			uid = v
// 		}
// 	}
// 	if req.Params.Meta == nil {
// 		return nil, fmt.Errorf("meta is required")
// 	}
// 	dir, _ := req.Params.Meta["dir"].(string)
// 	if dir == "" {
// 		return nil, fmt.Errorf("dir is required")
// 	}
// 	includeSelf := false
// 	if v, ok := req.Params.Meta["includeSelf"].(bool); ok {
// 		includeSelf = v
// 	}
// 	childDirs := readMetaStringSlice(req.Params.Meta, "childDirs")
// 	targets := make([]string, 0, len(childDirs)+1)
// 	if includeSelf {
// 		targets = append(targets, dir)
// 	}
// 	for _, child := range childDirs {
// 		child = filepath.Clean(child)
// 		if child == "." || child == "" || child == ".." {
// 			continue
// 		}
// 		targets = append(targets, filepath.Join(dir, child))
// 	}
// 	if len(targets) == 0 {
// 		targets = append(targets, dir)
// 	}
// 	items := make([]map[string]any, 0)
// 	seenPath := make(map[string]struct{})
// 	for _, target := range targets {
// 		nodes, err := agentload.ScanAndCacheWithBaseDir(ctx, target, nodestore.KindAgent, uid, settings.BaseDir)
// 		if err != nil {
// 			return nil, err
// 		}
// 		for _, node := range nodes {
// 			if node == nil {
// 				continue
// 			}
// 			path := workdirFromAgentURI(node.URI)
// 			if path == "" {
// 				continue
// 			}
// 			if _, ok := seenPath[path]; ok {
// 				continue
// 			}
// 			meta, ok := nodestore.NodeMeta[op.AgentMeta](node)
// 			if !ok {
// 				continue
// 			}
// 			agentID, agentName, ok := resolveNodeScanAgent(meta, node.ID, func(bindID string) (string, bool) {
// 				_, boundMeta, err := getAgentNode(ctx, bindID)
// 				if err != nil || boundMeta == nil {
// 					return "", false
// 				}
// 				resolvedName := strings.TrimSpace(boundMeta.Name)
// 				if resolvedName == "" {
// 					return "", false
// 				}
// 				return resolvedName, true
// 			})
// 			if !ok {
// 				continue
// 			}
// 			seenPath[path] = struct{}{}
// 			items = append(items, map[string]any{
// 				"path":      path,
// 				"agentId":   agentID,
// 				"agentName": agentName,
// 			})
// 		}
// 	}
// 	raw, err := json.Marshal(items)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: raw},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func resolveNodeScanAgent(
// 	meta *op.AgentMeta,
// 	nodeID string,
// 	resolveBoundName func(bindID string) (string, bool),
// ) (agentID string, agentName string, ok bool) {
// 	if meta == nil {
// 		return "", "", false
// 	}
// 	agentID = strings.TrimSpace(nodeID)
// 	agentName = strings.TrimSpace(meta.Name)
// 	bindID := strings.TrimSpace(meta.BindAgentID)
// 	if bindID == "" {
// 		if agentID == "" {
// 			return "", "", false
// 		}
// 		return agentID, agentName, true
// 	}
// 	if resolveBoundName == nil {
// 		return "", "", false
// 	}
// 	resolvedName, resolved := resolveBoundName(bindID)
// 	if !resolved {
// 		return "", "", false
// 	}
// 	resolvedName = strings.TrimSpace(resolvedName)
// 	if resolvedName == "" {
// 		return "", "", false
// 	}
// 	return bindID, resolvedName, true
// }

// func OpAgentRootsHandler(ctx context.Context, req *op.OpAgentRequest) (*op.OpAgentResult, error) {
// 	cfg := config.Get()
// 	if cfg == nil {
// 		return nil, fmt.Errorf("config not initialized")
// 	}

// 	roots := cfg.AgentsDir
// 	if len(roots) == 0 {
// 		if cfg.BaseDir != "" {
// 			roots = []string{filepath.Join(cfg.BaseDir, "agents")}
// 		}
// 	}

// 	result := map[string][]string{"roots": roots}
// 	jsonBytes, err := json.Marshal(result)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to marshal roots: %w", err)
// 	}

// 	return &op.OpAgentResult{
// 		Content: &op.JsonContent{Raw: jsonBytes},
// 		Meta:    req.Params.Meta,
// 	}, nil
// }

// func resolveUserSettingsForMeta(ctx context.Context, meta op.Meta) (*op.UserSettings, error) {
// 	uid, err := resolveRequestUID(meta)
// 	if err != nil {
// 		return nil, err
// 	}
// 	settings, err := GetStorage().GetUserSettings(ctx, uid)
// 	if err == nil && settings != nil && settings.BaseDir != "" {
// 		return settings, nil
// 	}
// 	cfg := config.Get()
// 	if cfg == nil {
// 		if err != nil {
// 			return nil, err
// 		}
// 		return nil, fmt.Errorf("config not initialized")
// 	}
// 	if settings == nil {
// 		settings = &op.UserSettings{UID: uid}
// 	}
// 	if settings.UID == "" {
// 		settings.UID = uid
// 	}
// 	if settings.BaseDir == "" {
// 		settings.BaseDir = cfg.BaseDir
// 	}
// 	return settings, nil
// }

// func resolveRequestUID(meta op.Meta) (string, error) {
// 	if meta == nil {
// 		return "", fmt.Errorf(unauthorizedError)
// 	}
// 	uid, _ := meta["uid"].(string)
// 	uid = strings.TrimSpace(uid)
// 	if uid == "" || strings.EqualFold(uid, "anonymous") {
// 		return "", fmt.Errorf(unauthorizedError)
// 	}
// 	return uid, nil
// }

// func readMetaStringSlice(meta op.Meta, key string) []string {
// 	if meta == nil {
// 		return nil
// 	}
// 	raw, ok := meta[key]
// 	if !ok || raw == nil {
// 		return nil
// 	}
// 	switch typed := raw.(type) {
// 	case []string:
// 		return typed
// 	case []interface{}:
// 		out := make([]string, 0, len(typed))
// 		for _, item := range typed {
// 			str, ok := item.(string)
// 			if !ok || str == "" {
// 				continue
// 			}
// 			out = append(out, str)
// 		}
// 		return out
// 	default:
// 		return nil
// 	}
// }

// func workdirFromAgentURI(uri string) string {
// 	path := op.URIToPath(uri)
// 	if path == "" {
// 		return ""
// 	}
// 	if filepath.Base(path) == "AGENT.md" && filepath.Base(filepath.Dir(path)) == ".agent" {
// 		return filepath.Dir(filepath.Dir(path))
// 	}
// 	if filepath.Base(path) == "AGENTS.md" && filepath.Base(filepath.Dir(path)) == ".agents" {
// 		return filepath.Dir(filepath.Dir(path))
// 	}
// 	return ""
// }

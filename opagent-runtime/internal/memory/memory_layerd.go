package memory

// // ---------------------------- user task ----------------------------
// func (m *LayeredMemory) GetUserTask(ctx context.Context, taskID string) (*op.UserTask, error) {
// 	return m.storage.GetUserTask(ctx, taskID)
// }

// func (m *LayeredMemory) ListUserTasks(ctx context.Context, uid string) ([]*op.UserTask, error) {
// 	return m.storage.ListUserTasks(ctx, uid)
// }

// func (m *LayeredMemory) UpsertUserTask(ctx context.Context, task *op.UserTask) error {
// 	return m.storage.UpsertUserTask(ctx, task)
// }

// func (m *LayeredMemory) BindThreadIDToUserTaskID(ctx context.Context, taskID, threadID string) error {
// 	return m.storage.BindThreadIDToUserTaskID(ctx, taskID, threadID)
// }

// // ---------------------------- agent config ----------------------------
// func (m *LayeredMemory) UpsertAgentConfig(ctx context.Context, agentConfig *op.AgentConfig) error {
// 	return m.storage.UpsertAgentConfig(ctx, agentConfig)
// }

// func (m *LayeredMemory) GetAgentConfig(ctx context.Context, agentID string) (*op.AgentConfig, error) {
// 	return m.storage.GetAgentConfig(ctx, agentID)
// }

// func (m *LayeredMemory) ListAgentConfigs(ctx context.Context) ([]*op.AgentConfig, error) {
// 	return m.storage.ListAgentConfigs(ctx)
// }

// // ---------------------------- agent server config ----------------------------
// func (m *LayeredMemory) UpsertAgentServerConfig(ctx context.Context, agentServerConfig *op.AgentServerConfig) error {
// 	return m.storage.UpsertAgentServerConfig(ctx, agentServerConfig)
// }

// func (m *LayeredMemory) GetAgentServerConfig(ctx context.Context, serverID string) (*op.AgentServerConfig, error) {
// 	return m.storage.GetAgentServerConfig(ctx, serverID)
// }

// func (m *LayeredMemory) ListAgentServerConfigs(ctx context.Context) ([]*op.AgentServerConfig, error) {
// 	return m.storage.ListAgentServerConfigs(ctx)
// }

// // GetAgentServerConn fetches an existing AgentServer connection from cache.
// func (m *LayeredMemory) GetAgentServerConn(ctx context.Context, serverID string) (*host.AgentServerConn, error) {
// 	agentServer, err := cache.Get[host.AgentServerConn](serverID)
// 	if err != nil {
// 		return nil, err
// 	}

// 	agentServer.Status = op.Status_Running
// 	agentServer.LastActiveAt = time.Now()
// 	// Use agentServer.Ctx (the original background context for daemon connections)
// 	// instead of the request ctx to prevent the cache entry from being deleted
// 	// when the request context is canceled.
// 	cache.Set(serverID, agentServer, cache.NoExpiration)
// 	return agentServer, nil
// }

// // GetToolServerConn returns a cached tool server connection if it exists.
// func (m *LayeredMemory) GetToolServerConn(ctx context.Context, name string) (*host.ToolServerConn, error) {
// 	toolConn, err := cache.Get[host.ToolServerConn](name)
// 	if err != nil {
// 		return nil, err
// 	}

// 	toolConn.Status = op.Status_Running
// 	toolConn.LastActiveAt = time.Now()
// 	// Use toolConn.Ctx (the original background context for daemon connections)
// 	// instead of the request ctx to prevent the cache entry from being deleted
// 	// when the request context is canceled.
// 	cache.Set(name, toolConn, cache.NoExpiration)
// 	return toolConn, nil
// }

// // UpsertToolServerConn stores or updates a tool server connection in the cache.
// func (m *LayeredMemory) UpsertToolServerConn(ctx context.Context, serverConn *host.ToolServerConn) error {
// 	cache.Set(serverConn.Name, serverConn, cache.NoExpiration)
// 	return nil
// }

// // ---------------------------- tool server ----------------------------
// func (m *LayeredMemory) UpsertToolServerConfig(ctx context.Context, toolServer *op.ToolServerConfig) error {
// 	return m.storage.UpsertToolServerConfig(ctx, toolServer)
// }

// func (m *LayeredMemory) ListToolServerConfigs(ctx context.Context) ([]*op.ToolServerConfig, error) {
// 	return m.storage.ListToolServerConfigs(ctx)
// }

// // ---------------------------- agent task ----------------------------
// func (m *LayeredMemory) UpsertAgentTaskMemory(ctx context.Context, task *op.AgentTaskStorage) error {

// 	task.UpdatedAt = time.Now().UnixMilli()

// 	// upsert task memory
// 	return m.storage.UpsertAgentTaskMemory(ctx, task)
// }

// func (m *LayeredMemory) GetAgentTaskMemory(ctx context.Context, taskID string) (*op.AgentTaskStorage, error) {
// 	return m.storage.GetAgentTask(ctx, taskID)
// }

// func (m *LayeredMemory) ListAgentTaskMemory(ctx context.Context, threadID string) ([]*op.AgentTaskStorage, error) {
// 	return m.storage.ListAgentTaskMemory(ctx, threadID)
// }

// func (m *LayeredMemory) GetLatestTaskMemory(threadID string) (*op.AgentTaskStorage, error) {
// 	return m.storage.GetLatestAgentTaskMemory(context.Background(), threadID)
// }

// // ---------------------------- thread ----------------------------
// func (m *LayeredMemory) GetThreadMemory(ctx context.Context, threadID string) (*op.ThreadStorage, error) {
// 	return m.storage.GetThread(ctx, threadID)
// }

// func (m *LayeredMemory) GetThread(threadID string) (*host.Thread, error) {
// 	thread, err := cache.Get[host.Thread](threadID)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return thread, nil
// }

// func (m *LayeredMemory) ClearThread(threadID string) error {
// 	cache.Delete(threadID)
// 	return nil
// }

// func (m *LayeredMemory) UpsertThread(ctx context.Context, thread *host.Thread) error {

// 	// set cache
// 	cache.Set(thread.ThreadID, thread, cache.DefaultExpiration)

// 	// 通过 JSON 标签做类型间映射，自动拷贝同名字段
// 	var threadMemory op.ThreadStorage
// 	bytes, err := json.Marshal(thread)
// 	if err != nil {
// 		return fmt.Errorf("marshal thread: %w", err)
// 	}
// 	if err := json.Unmarshal(bytes, &threadMemory); err != nil {
// 		return fmt.Errorf("unmarshal thread into ThreadMemory: %w", err)
// 	}
// 	threadMemory.UpdatedAt = time.Now().UnixMilli()

// 	// upsert thread memory
// 	return m.storage.UpsertThread(ctx, &threadMemory)
// }

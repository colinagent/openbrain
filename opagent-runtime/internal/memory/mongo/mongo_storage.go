package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const limit = 1000
const modelCollection = "model"

// MongoStorage 实现 Storage 接口的 MongoDB 版本
type MongoStorage struct{}

// NewMongoStorage 创建 MongoDB 存储实例
func NewMongoStorage() *MongoStorage {
	cfg := config.GetConfig()
	if cfg == nil {
		slog.Error("config is nil, cannot initialize MongoDB storage")
		os.Exit(1)
	}

	if err := NewMongo(&MongoOptions{
		URI:    cfg.MongoDB.URI,
		DBName: cfg.MongoDB.Database,
	}); err != nil {
		slog.Error("failed to initialize MongoDB storage", "error", err)
		os.Exit(1)
	}
	return &MongoStorage{}
}

// -------------------------------- thread --------------------------------
// thread
// func (s *MongoStorage) GetThreadStorage(ctx context.Context, threadID string) (*op.ThreadStorage, error) {
// 	var thread op.ThreadStorage
// 	err := GetCollection("thread").FindOne(ctx, bson.M{"threadID": threadID}).Decode(&thread)
// 	if err != nil {
// 		if err == mongo.ErrNoDocuments {
// 			return nil, fmt.Errorf("thread not found")
// 		}
// 		return nil, fmt.Errorf("failed to get thread: %w", err)
// 	}
// 	return &thread, nil
// }

// func (s *MongoStorage) UpsertThreadStorage(ctx context.Context, thread *op.ThreadStorage) error {
// 	_, err := GetCollection("thread").UpdateOne(
// 		ctx,
// 		bson.M{"threadID": thread.ThreadID},
// 		bson.M{"$set": thread},
// 		options.Update().SetUpsert(true),
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to upsert thread: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) DeleteThreadStorage(ctx context.Context, threadID string) error {
// 	//删除thread
// 	_, err := GetCollection("thread").DeleteMany(
// 		ctx,
// 		bson.M{"threadID": threadID},
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to delete thread: %w", err)
// 	}
// 	//删除thread对应的agent task
// 	return nil
// }

// func (s *MongoStorage) QueryThreadStorage(ctx context.Context, query *op.ThreadStorageQuery) ([]*op.ThreadStorage, int64, error) {
// 	// 空指针检查
// 	if query == nil {
// 		query = &op.ThreadStorageQuery{}
// 	}

// 	filter := bson.M{}

// 	// 基础字段过滤
// 	if query.ThreadID != "" {
// 		filter["threadID"] = query.ThreadID
// 	}
// 	if query.UID != "" {
// 		filter["uid"] = query.UID
// 	}

// 	// Meta 自定义字段过滤（使用点号表示法）
// 	// 注意：如果 meta 字段不存在或值类型不匹配，MongoDB 会正常处理（不返回该文档）
// 	for key, value := range query.MetaFilter {
// 		if key == "" {
// 			continue // 跳过空 key
// 		}
// 		filter["meta."+key] = value
// 	}

// 	// 时间范围
// 	if query.CreatedAtFrom > 0 || query.CreatedAtTo > 0 {
// 		timeFilter := bson.M{}
// 		if query.CreatedAtFrom > 0 {
// 			timeFilter["$gte"] = query.CreatedAtFrom
// 		}
// 		if query.CreatedAtTo > 0 {
// 			timeFilter["$lte"] = query.CreatedAtTo
// 		}
// 		filter["createdAt"] = timeFilter
// 	}

// 	// 获取总数
// 	total, err := GetCollection("thread").CountDocuments(ctx, filter)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to count threads: %w", err)
// 	}

// 	// 分页设置
// 	queryLimit := query.Limit
// 	if queryLimit <= 0 {
// 		queryLimit = limit // 使用默认 limit
// 	}

// 	// 排序设置
// 	sortField := query.SortBy
// 	if sortField == "" {
// 		sortField = "createdAt"
// 	}
// 	sortOrder := 1
// 	if query.Desc {
// 		sortOrder = -1
// 	}

// 	opts := options.Find().
// 		SetLimit(queryLimit).
// 		SetSkip(query.Offset).
// 		SetSort(bson.D{{Key: sortField, Value: sortOrder}})

// 	cursor, err := GetCollection("thread").Find(ctx, filter, opts)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to query threads: %w", err)
// 	}
// 	defer cursor.Close(ctx)

// 	var threads []*op.ThreadStorage
// 	if err = cursor.All(ctx, &threads); err != nil {
// 		return nil, 0, fmt.Errorf("failed to decode threads: %w", err)
// 	}

// 	// 确保返回空切片而非 nil
// 	if threads == nil {
// 		threads = []*op.ThreadStorage{}
// 	}

// 	return threads, total, nil
// }

// // --------------user-----------------
// func (s *MongoStorage) DeleteThreadIDFromUserStorage(ctx context.Context, threadID string) error {
// 	_, err := GetCollection("user").DeleteMany(
// 		ctx,
// 		bson.M{"threadID": threadID},
// 	)
// 	return err
// }

// func (s *MongoStorage) ListUserStorage(ctx context.Context, uid string) ([]*op.UserStorage, error) {
// 	opts := options.Find().
// 		SetLimit(100).
// 		SetSort(bson.M{"createdAt": -1})

// 	cursor, err := GetCollection("user").Find(ctx, bson.M{"uid": uid}, opts)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer cursor.Close(ctx)

// 	var users []*op.UserStorage
// 	if err := cursor.All(ctx, &users); err != nil {
// 		return nil, err
// 	}
// 	return users, nil
// }

// func (s *MongoStorage) UpsertUserStorage(ctx context.Context, user *op.UserStorage) error {
// 	_, err := GetCollection("user").UpdateOne(
// 		ctx,
// 		bson.M{"uid": user.UID, "threadID": user.ThreadID},
// 		bson.M{"$set": user},
// 		options.Update().SetUpsert(true),
// 	)
// 	return err
// }

func (s *MongoStorage) ListUIDs(ctx context.Context) ([]string, error) {
	results, err := GetCollection("thread").Distinct(ctx, "uid", bson.M{"uid": bson.M{"$ne": ""}})
	if err != nil {
		return nil, err
	}

	uids := make([]string, 0, len(results))
	for _, r := range results {
		if id, ok := r.(string); ok {
			uids = append(uids, id)
		}
	}
	return uids, nil
}

// -------------------------------- user settings --------------------------------
func (s *MongoStorage) UpsertUserSettings(ctx context.Context, settings *op.UserSettings) error {
	if settings == nil || settings.UID == "" {
		return fmt.Errorf("uid is required")
	}
	now := time.Now().UnixMilli()
	if settings.CreatedAt == 0 {
		createdAt, err := resolveCreatedAt(ctx, GetCollection("user_settings"), bson.M{"uid": settings.UID}, now)
		if err != nil {
			return fmt.Errorf("resolve user settings createdAt: %w", err)
		}
		settings.CreatedAt = createdAt
	}
	settings.UpdatedAt = now
	_, err := GetCollection("user_settings").UpdateOne(
		ctx,
		bson.M{"uid": settings.UID},
		bson.M{"$set": settings},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert user settings: %w", err)
	}
	return nil
}

func (s *MongoStorage) GetUserSettings(ctx context.Context, uid string) (*op.UserSettings, error) {
	if uid == "" {
		return nil, fmt.Errorf("uid is required")
	}
	var settings op.UserSettings
	err := GetCollection("user_settings").FindOne(ctx, bson.M{"uid": uid}).Decode(&settings)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user settings not found")
		}
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}
	return &settings, nil
}

// -------------------------------- agent records by uid --------------------------------
// func (s *MongoStorage) ListAgentRecordsByUID(ctx context.Context, uid string) ([]*op.AgentRecord, error) {
// 	if uid == "" {
// 		return nil, fmt.Errorf("uid is required")
// 	}
// 	findOpts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	cursor, err := GetCollection("agent_record").Find(ctx, bson.M{"uid": uid}, findOpts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list agent records: %w", err)
// 	}
// 	defer cursor.Close(ctx)
// 	var records []*op.AgentRecord
// 	if err = cursor.All(ctx, &records); err != nil {
// 		return nil, fmt.Errorf("failed to decode agent records: %w", err)
// 	}
// 	return records, nil
// }

// // -------------------------------- tool server --------------------------------
// func (s *MongoStorage) GetToolServer(ctx context.Context, id string) (*op.ToolServerRecord, error) {
// 	var record op.ToolServerRecord
// 	err := GetCollection("tool_server").FindOne(ctx, bson.M{"id": id}).Decode(&record)
// 	if err != nil {
// 		if err == mongo.ErrNoDocuments {
// 			return nil, fmt.Errorf("tool server not found: %s", id)
// 		}
// 		return nil, fmt.Errorf("failed to get tool server: %w", err)
// 	}
// 	return &record, nil
// }

// func (s *MongoStorage) UpsertToolServer(ctx context.Context, record *op.ToolServerRecord) error {
// 	if record == nil || record.Name == "" || record.URI == "" {
// 		return fmt.Errorf("tool server name and URI are required")
// 	}
// 	if record.ID == "" {
// 		return fmt.Errorf("tool server id is required; build it with op.BuildNodeID(...) before upsert")
// 	}
// 	now := time.Now().UnixMilli()
// 	if record.CreatedAt == 0 {
// 		createdAt, err := resolveCreatedAt(ctx, GetCollection("tool_server"), bson.M{"id": record.ID}, now)
// 		if err != nil {
// 			return fmt.Errorf("resolve tool server createdAt: %w", err)
// 		}
// 		record.CreatedAt = createdAt
// 	}
// 	record.UpdatedAt = now
// 	_, err := GetCollection("tool_server").UpdateOne(
// 		ctx,
// 		bson.M{"id": record.ID},
// 		bson.M{"$set": record},
// 		options.Update().SetUpsert(true),
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to upsert tool server: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) ListToolServers(ctx context.Context) ([]*op.ToolServerRecord, error) {
// 	findOpts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	cursor, err := GetCollection("tool_server").Find(ctx, bson.M{}, findOpts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get tool servers: %w", err)
// 	}
// 	defer cursor.Close(ctx)
// 	var records []*op.ToolServerRecord
// 	if err = cursor.All(ctx, &records); err != nil {
// 		return nil, fmt.Errorf("failed to decode tool servers: %w", err)
// 	}
// 	return records, nil
// }

// func (s *MongoStorage) DeleteToolServer(ctx context.Context, id string) error {
// 	if id == "" {
// 		return fmt.Errorf("tool server id is required")
// 	}
// 	if _, err := GetCollection("tool").DeleteMany(ctx, bson.M{"serverID": id}); err != nil {
// 		return fmt.Errorf("failed to delete tools by server id: %w", err)
// 	}
// 	_, err := GetCollection("tool_server").DeleteMany(ctx, bson.M{"id": id})
// 	if err != nil {
// 		return fmt.Errorf("failed to delete tool server: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) ReplaceToolsByServerID(ctx context.Context, serverID string, tools []*op.ToolRecord) error {
// 	if serverID == "" {
// 		return fmt.Errorf("serverID is required")
// 	}
// 	server, err := s.GetToolServer(ctx, serverID)
// 	if err != nil {
// 		return err
// 	}
// 	if _, err := GetCollection("tool").DeleteMany(ctx, bson.M{"serverID": serverID}); err != nil {
// 		return fmt.Errorf("failed to clear tools by server id: %w", err)
// 	}
// 	now := time.Now().UnixMilli()
// 	docs := make([]interface{}, 0, len(tools))
// 	for _, tool := range tools {
// 		if tool == nil || tool.Name == "" {
// 			continue
// 		}
// 		if tool.ServerID == "" {
// 			tool.ServerID = serverID
// 		}
// 		if tool.ID == "" {
// 			tool.ID = op.ToolIDFromURI(server.URI, tool.Name)
// 		}
// 		if tool.CreatedAt == 0 {
// 			tool.CreatedAt = now
// 		}
// 		tool.UpdatedAt = now
// 		docs = append(docs, tool)
// 	}
// 	if len(docs) == 0 {
// 		return nil
// 	}
// 	if _, err := GetCollection("tool").InsertMany(ctx, docs); err != nil {
// 		return fmt.Errorf("failed to insert tool docs: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) ListToolsByServerID(ctx context.Context, serverID string) ([]*op.ToolRecord, error) {
// 	findOpts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
// 	cursor, err := GetCollection("tool").Find(ctx, bson.M{"serverID": serverID}, findOpts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list tools by server id: %w", err)
// 	}
// 	defer cursor.Close(ctx)
// 	var records []*op.ToolRecord
// 	if err = cursor.All(ctx, &records); err != nil {
// 		return nil, fmt.Errorf("failed to decode tools: %w", err)
// 	}
// 	return records, nil
// }

// -------------------------------- user task --------------------------------
func (s *MongoStorage) GetUserTask(ctx context.Context, taskID string) (*op.UserTask, error) {
	var task op.UserTask
	err := GetCollection("user_task").FindOne(ctx, bson.M{"taskID": taskID}).Decode(&task)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user task not found")
		}
		return nil, fmt.Errorf("failed to get user task: %w", err)
	}
	return &task, nil
}

func (s *MongoStorage) ListUserTasks(ctx context.Context, uid string) ([]*op.UserTask, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.M{"createdAt": -1})

	cursor, err := GetCollection("user_task").Find(ctx, bson.M{"uid": uid}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list user tasks: %w", err)
	}
	defer cursor.Close(ctx)

	var tasks []*op.UserTask
	if err = cursor.All(ctx, &tasks); err != nil {
		return nil, fmt.Errorf("failed to decode user tasks: %w", err)
	}
	return tasks, nil
}

func (s *MongoStorage) UpsertUserTask(ctx context.Context, task *op.UserTask) error {
	_, err := GetCollection("user_task").UpdateOne(
		ctx,
		bson.M{"taskID": task.TaskID},
		bson.M{"$set": task},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert user task: %w", err)
	}
	return nil
}

// BindThreadIDToTaskID 将 threadID 绑定到 taskID
func (s *MongoStorage) BindThreadIDToUserTaskID(ctx context.Context, taskID, threadID string) error {
	_, err := GetCollection("user_task").UpdateOne(
		ctx,
		bson.M{"taskID": taskID},
		bson.M{"$addToSet": bson.M{"threadIDs": threadID}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to bind threadID to taskID: %w", err)
	}
	return nil
}

func (s *MongoStorage) DeleteThreadIDFromUserTask(ctx context.Context, taskID, threadID string) error {
	_, err := GetCollection("user_task").UpdateOne(
		ctx,
		bson.M{"taskID": taskID},
		bson.M{"$pull": bson.M{"threadIDs": threadID}},
	)
	if err != nil {
		return fmt.Errorf("failed to delete threadID from user task: %w", err)
	}
	return nil
}

func (s *MongoStorage) DeleteUserTask(ctx context.Context, taskID string) error {
	_, err := GetCollection("user_task").DeleteMany(
		ctx,
		bson.M{"taskID": taskID},
	)
	if err != nil {
		return fmt.Errorf("failed to delete user task: %w", err)
	}
	return nil
}

// // -------------------------------- skill --------------------------------
// func (s *MongoStorage) UpsertSkill(ctx context.Context, skill *op.SkillRecord) error {
// 	if skill == nil || skill.Spec.Name == "" || skill.URI == "" {
// 		return fmt.Errorf("skill name and URI are required")
// 	}
// 	if skill.ID == "" {
// 		return fmt.Errorf("skill id is required; build it with op.BuildNodeID(...) before upsert")
// 	}
// 	now := time.Now().UnixMilli()
// 	if skill.CreatedAt == 0 {
// 		createdAt, err := resolveCreatedAt(ctx, GetCollection("skill"), bson.M{"id": skill.ID}, now)
// 		if err != nil {
// 			return fmt.Errorf("resolve skill createdAt: %w", err)
// 		}
// 		skill.CreatedAt = createdAt
// 	}
// 	skill.UpdatedAt = now
// 	_, err := GetCollection("skill").UpdateOne(
// 		ctx,
// 		bson.M{"id": skill.ID},
// 		bson.M{
// 			"$set": bson.M{
// 				"id":        skill.ID,
// 				"uri":       skill.URI,
// 				"uid":       skill.UID,
// 				"spec":      skill.Spec,
// 				"createdAt": skill.CreatedAt,
// 				"updatedAt": skill.UpdatedAt,
// 			},
// 		},
// 		options.Update().SetUpsert(true),
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to upsert skill: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) ListSkills(ctx context.Context) ([]*op.SkillRecord, error) {
// 	findOpts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	cursor, err := GetCollection("skill").Find(ctx, bson.M{}, findOpts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list skills: %w", err)
// 	}
// 	defer cursor.Close(ctx)

// 	var rawSkills []*op.SkillRecord
// 	if err = cursor.All(ctx, &rawSkills); err != nil {
// 		return nil, fmt.Errorf("failed to decode skills: %w", err)
// 	}
// 	// 去重，只保留同 id 最新配置
// 	seen := make(map[string]struct{})
// 	var skills []*op.SkillRecord
// 	for _, skill := range rawSkills {
// 		if skill == nil || skill.ID == "" {
// 			continue
// 		}
// 		if _, ok := seen[skill.ID]; ok {
// 			continue
// 		}
// 		seen[skill.ID] = struct{}{}
// 		skills = append(skills, skill)
// 	}
// 	return skills, nil
// }

// func (s *MongoStorage) GetSkill(ctx context.Context, id string) (*op.SkillRecord, error) {
// 	if id == "" {
// 		return nil, fmt.Errorf("skill id is required")
// 	}
// 	var skill op.SkillRecord
// 	findOpts := options.FindOne().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	err := GetCollection("skill").FindOne(ctx, bson.M{"id": id}, findOpts).Decode(&skill)
// 	if err != nil {
// 		if err == mongo.ErrNoDocuments {
// 			return nil, fmt.Errorf("skill not found: %s", id)
// 		}
// 		return nil, fmt.Errorf("failed to get skill: %w", err)
// 	}
// 	return &skill, nil
// }

// func (s *MongoStorage) DeleteSkill(ctx context.Context, id string) error {
// 	if id == "" {
// 		return fmt.Errorf("skill id is required")
// 	}
// 	_, err := GetCollection("skill").DeleteMany(ctx, bson.M{"id": id})
// 	if err != nil {
// 		return fmt.Errorf("failed to delete skill: %w", err)
// 	}
// 	return nil
// }

// // -------------------------------- agent config --------------------------------
// func (s *MongoStorage) UpsertAgentRecord(ctx context.Context, agentRecord *op.AgentRecord) error {
// 	if agentRecord == nil || agentRecord.ID == "" {
// 		return fmt.Errorf("agent id is required")
// 	}
// 	now := time.Now().UnixMilli()
// 	if agentRecord.CreatedAt == 0 {
// 		createdAt, err := resolveCreatedAt(ctx, GetCollection("agent_record"), bson.M{"id": agentRecord.ID}, now)
// 		if err != nil {
// 			return fmt.Errorf("resolve agent record createdAt: %w", err)
// 		}
// 		agentRecord.CreatedAt = createdAt
// 	}
// 	agentRecord.UpdatedAt = now
// 	_, err := GetCollection("agent_record").UpdateOne(
// 		ctx,
// 		bson.M{"id": agentRecord.ID},
// 		bson.M{"$set": agentRecord},
// 		options.Update().SetUpsert(true),
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to upsert agent record: %w", err)
// 	}
// 	return nil
// }

// func (s *MongoStorage) GetAgentRecord(ctx context.Context, agentID string) (*op.AgentRecord, error) {
// 	var agentRecord op.AgentRecord
// 	findOpts := options.FindOne().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	err := GetCollection("agent_record").FindOne(ctx, bson.M{"id": agentID}, findOpts).Decode(&agentRecord)

// 	// 先检查"不存在"的情况
// 	if err == mongo.ErrNoDocuments {
// 		return nil, op.ErrAgentRecordNotFound
// 	}

// 	// 再检查其他错误
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get agent record: %w", err)
// 	}
// 	return &agentRecord, nil
// }

// func (s *MongoStorage) ListAgentRecords(ctx context.Context) ([]*op.AgentRecord, error) {
// 	findOpts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
// 	cursor, err := GetCollection("agent_record").Find(ctx, bson.M{}, findOpts)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get agent records: %w", err)
// 	}
// 	defer cursor.Close(ctx)

// 	var rawRecords []*op.AgentRecord
// 	if err = cursor.All(ctx, &rawRecords); err != nil {
// 		return nil, fmt.Errorf("failed to decode agent records: %w", err)
// 	}
// 	seen := make(map[string]struct{})
// 	var records []*op.AgentRecord
// 	for _, record := range rawRecords {
// 		if record == nil || record.ID == "" {
// 			continue
// 		}
// 		if _, ok := seen[record.ID]; ok {
// 			continue
// 		}
// 		seen[record.ID] = struct{}{}
// 		records = append(records, record)
// 	}
// 	return records, nil
// }

// -------------------------------- models --------------------------------
func (s *MongoStorage) ListModels(ctx context.Context) ([]*op.ModelConfig, error) {
	findOpts := options.Find().SetSort(bson.D{{Key: "id", Value: 1}})
	cursor, err := GetCollection(modelCollection).Find(ctx, bson.M{}, findOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer cursor.Close(ctx)

	var models []*op.ModelConfig
	if err := cursor.All(ctx, &models); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}
	return models, nil
}

// ListModelIDs returns a list of distinct model IDs, represented as ModelConfig objects with only ID populated.
func (s *MongoStorage) ListModelIDs(ctx context.Context) ([]string, error) {
	results, err := GetCollection(modelCollection).Distinct(ctx, "id", bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to distinct model ids: %w", err)
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		if id, ok := r.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *MongoStorage) UpsertModel(ctx context.Context, model *op.ModelConfig) error {
	if model == nil || model.ID == "" {
		return fmt.Errorf("model id is required")
	}
	_, err := GetCollection(modelCollection).UpdateOne(
		ctx,
		bson.M{"id": model.ID},
		bson.M{"$set": model},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert model: %w", err)
	}
	return nil
}

func (s *MongoStorage) GetModel(ctx context.Context, id string) (*op.ModelConfig, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	var model op.ModelConfig
	err := GetCollection(modelCollection).FindOne(ctx, bson.M{"id": id}).Decode(&model)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("model not found")
		}
		return nil, fmt.Errorf("failed to get model: %w", err)
	}
	return &model, nil
}

func (s *MongoStorage) DeleteModel(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	_, err := GetCollection(modelCollection).DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}
	return nil
}

func resolveCreatedAt(ctx context.Context, coll *mongo.Collection, filter interface{}, fallback int64) (int64, error) {
	opts := options.FindOne().SetProjection(bson.M{"createdAt": 1})
	var existing struct {
		CreatedAt int64 `bson:"createdAt"`
	}
	err := coll.FindOne(ctx, filter, opts).Decode(&existing)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fallback, nil
		}
		return 0, err
	}
	if existing.CreatedAt == 0 {
		return fallback, nil
	}
	return existing.CreatedAt, nil
}

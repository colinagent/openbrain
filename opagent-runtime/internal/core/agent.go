package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/builtintools"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
	"github.com/colinagent/openbrain/opagent-runtime/internal/scan"
	"github.com/colinagent/openbrain/opagent-runtime/packages/agentprompt"
)

type Agent struct {
	AgentID            string
	AgentMeta          *op.AgentMeta
	AvailableSkills    []op.OpNode
	AvailableSubagents []op.OpNode
	Conn               *Connection
	ToolSpecs          map[string]*op.ToolSpec
	Meta               op.Meta
	Sysprompt          string
	PromptIsFinal      bool
}

func NewAgent(ctx context.Context, node *op.OpNode, meta op.Meta) (*Agent, error) {
	if node == nil {
		return nil, fmt.Errorf("node is required")
	}
	agentMeta, ok := node.Meta.(*op.AgentMeta)
	if !ok {
		return nil, fmt.Errorf("node %s is not an agent", node.ID)
	}
	agentMeta = cloneAgentMeta(agentMeta)

	loadedSkills, resolvedSkillIDs := resolveSkillNodes(agentMeta.Skills)
	agentMeta.Skills = resolvedSkillIDs
	availableSubagents, resolvedSubagentIDs := resolveSubagentNodes(agentMeta.SubAgents)
	agentMeta.SubAgents = resolvedSubagentIDs

	conn := &Connection{}
	sysprompt := ""
	promptIsFinal := false
	if node.Run.HasEndpoint() {
		var err error
		conn, err = EnsureConnection(ctx, node)
		if err != nil {
			return nil, err
		}
		sysprompt, promptIsFinal = loadPromptViaEndpoint(ctx, conn, node, meta)
	}
	if !promptIsFinal {
		var err error
		sysprompt, err = scan.LoadPromptByURI(node.URI)
		if err != nil {
			return nil, err
		}
	}
	sysprompt, err := expandAgentPromptVariables(sysprompt, node, meta)
	if err != nil {
		return nil, err
	}

	toolSpecs := assembleTools(ctx, agentMeta)
	if isThreadSubmitCapableAgent(node) {
		addMessageToolSpecs(toolSpecs, agentMeta)
	}
	if shouldExposeBuiltinSystool(agentMeta, agentTaskToolName) {
		addAgentTaskToolSpec(toolSpecs, availableSubagents)
	}
	sysprompt = appendMessageToolGuidance(sysprompt, toolSpecs)
	sysprompt = appendSubagentsAppendix(sysprompt, availableSubagents)

	return &Agent{
		AgentID:            node.ID,
		AgentMeta:          agentMeta,
		AvailableSkills:    loadedSkills,
		AvailableSubagents: availableSubagents,
		Conn:               conn,
		ToolSpecs:          toolSpecs,
		Meta:               meta,
		Sysprompt:          sysprompt,
		PromptIsFinal:      promptIsFinal,
	}, nil
}

func resolveSubagentNodes(agentIDs []string) ([]op.OpNode, []string) {
	subagents := make([]op.OpNode, 0, len(agentIDs))
	resolved := make([]string, 0, len(agentIDs))
	seen := make(map[string]struct{})

	for _, agentID := range agentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, exists := seen[agentID]; exists {
			continue
		}
		nodeValue, ok := cache.GetValue[op.OpNode](agentID, cache.PrefixNode)
		if !ok {
			slog.Warn("configured subagent missing from node cache", "agentID", agentID)
			continue
		}
		nodeValue = refreshFileBackedAgentNode(nodeValue)
		if !isThreadSubmitCapableAgent(&nodeValue) {
			slog.Warn("configured subagent is not thread-submit capable", "agentID", agentID, "kind", nodeValue.Kind, "opcodes", nodeValue.OpCodes)
			continue
		}
		seen[agentID] = struct{}{}
		resolved = append(resolved, agentID)
		subagents = append(subagents, nodeValue)
	}

	sort.SliceStable(subagents, func(i, j int) bool {
		left := subagentDisplayName(&subagents[i])
		right := subagentDisplayName(&subagents[j])
		if left != right {
			return left < right
		}
		return strings.TrimSpace(subagents[i].ID) < strings.TrimSpace(subagents[j].ID)
	})
	return subagents, resolved
}

func isThreadSubmitCapableAgent(node *op.OpNode) bool {
	if node == nil || strings.TrimSpace(node.Kind) != string(op.NodeKindAgent) {
		return false
	}
	for _, code := range node.OpCodes {
		if strings.TrimSpace(string(code)) == string(op.OpThreadSubmit) {
			return true
		}
	}
	return false
}

func subagentDisplayName(node *op.OpNode) string {
	if node == nil {
		return ""
	}
	if meta, ok := node.Meta.(*op.AgentMeta); ok && meta != nil {
		if name := strings.TrimSpace(meta.Name); name != "" {
			return name
		}
	}
	return strings.TrimSpace(node.ID)
}

func addAgentTaskToolSpec(toolSpecs map[string]*op.ToolSpec, subagents []op.OpNode) {
	if len(subagents) == 0 {
		return
	}
	if toolSpecs == nil {
		return
	}
	toolSpecs[agentTaskToolName] = &op.ToolSpec{
		ServerID:    systoolServerID,
		Name:        agentTaskToolName,
		Description: "Delegate a bounded task to one mounted subagent and wait for its result. Use only for tasks that match an available subagent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"subagent_id", "task"},
			"properties": map[string]any{
				"subagent_id": map[string]any{
					"type":        "string",
					"description": "Mounted subagent id from the Available Subagents list.",
				},
				"task": map[string]any{
					"type":        "string",
					"description": "The bounded task for the subagent to perform.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional additional context for the task.",
				},
			},
		},
	}
}

func appendSubagentsAppendix(basePrompt string, subagents []op.OpNode) string {
	if len(subagents) == 0 {
		return basePrompt
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(basePrompt))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("## Available Subagents\n\n")
	b.WriteString("Use the `agent_task` tool when a task should be delegated to one of these mounted subagents. Do not claim to have completed specialized subagent work yourself.\n\n")
	for _, node := range subagents {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		meta, _ := node.Meta.(*op.AgentMeta)
		name := id
		description := ""
		if meta != nil {
			if value := strings.TrimSpace(meta.Name); value != "" {
				name = value
			}
			description = strings.TrimSpace(meta.Description)
		}
		agentFile := strings.TrimSpace(op.URIToPath(node.URI))
		agentRoot := agentRootFromURI(node.URI)
		agentHome := agentHomeFromURI(node.URI)
		b.WriteString("- id: `")
		b.WriteString(id)
		b.WriteString("`; name: ")
		b.WriteString(name)
		if description != "" {
			b.WriteString("; description: ")
			b.WriteString(description)
		}
		if agentFile != "" {
			b.WriteString("; agent file: ")
			b.WriteString(agentFile)
		}
		if agentRoot != "" {
			b.WriteString("; agentRoot: ")
			b.WriteString(agentRoot)
		}
		if agentHome != "" {
			b.WriteString("; agentHome: ")
			b.WriteString(agentHome)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func expandAgentPromptVariables(prompt string, node *op.OpNode, meta op.Meta) (string, error) {
	cwd := strings.TrimSpace(metaString(meta, "cwd"))
	if strings.Contains(prompt, "${cwd}") && cwd == "" {
		return "", fmt.Errorf("prompt requires meta.cwd")
	}
	return agentprompt.ExpandVariables(prompt, agentPromptVariables(node, cwd)), nil
}

func agentPromptVariables(node *op.OpNode, cwd string) agentprompt.Variables {
	if node == nil {
		return agentprompt.Variables{CWD: strings.TrimSpace(cwd)}
	}
	return agentprompt.Variables{
		AgentRoot: agentRootFromURI(node.URI),
		AgentHome: agentHomeFromURI(node.URI),
		CWD:       strings.TrimSpace(cwd),
	}
}

func marshalToolResultJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}

func cloneAgentMeta(src *op.AgentMeta) *op.AgentMeta {
	if src == nil {
		return nil
	}
	clone := *src
	clone.ToolServers = append([]string(nil), src.ToolServers...)
	clone.SysTools = append([]string(nil), src.SysTools...)
	clone.Skills = append([]string(nil), src.Skills...)
	clone.SubAgents = append([]string(nil), src.SubAgents...)
	return &clone
}

func resolveSkillNodes(skillIDs []string) ([]op.OpNode, []string) {
	skills := make([]op.OpNode, 0, len(skillIDs))
	resolved := make([]string, 0, len(skillIDs))
	seen := make(map[string]struct{})

	for _, skillID := range skillIDs {
		skillID = strings.TrimSpace(skillID)
		if skillID == "" {
			continue
		}
		if _, exists := seen[skillID]; exists {
			continue
		}
		nodeValue, ok := cache.GetValue[op.OpNode](skillID, cache.PrefixNode)
		if !ok {
			slog.Warn("configured skill missing from node cache", "skillID", skillID)
			continue
		}
		if nodeValue.Kind != string(op.NodeKindSkill) {
			slog.Warn("configured skill ID does not point to a skill node", "skillID", skillID, "kind", nodeValue.Kind)
			continue
		}
		meta, ok := nodeValue.Meta.(*op.SkillMeta)
		if !ok || meta == nil {
			slog.Warn("configured skill meta is invalid", "skillID", skillID)
			continue
		}
		if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Description) == "" {
			slog.Warn("configured skill meta is incomplete", "skillID", skillID)
			continue
		}
		seen[skillID] = struct{}{}
		resolved = append(resolved, skillID)
		skills = append(skills, nodeValue)
	}

	return skills, resolved
}

func selectedSkillIDsFromMeta(meta op.Meta) []string {
	if meta == nil {
		return nil
	}
	raw := meta["selectedSkillIDs"]
	switch typed := raw.(type) {
	case []string:
		next := make([]string, 0, len(typed))
		for _, value := range typed {
			value = strings.TrimSpace(value)
			if value != "" {
				next = append(next, value)
			}
		}
		return next
	case []any:
		next := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					next = append(next, text)
				}
			}
		}
		return next
	default:
		return nil
	}
}

func selectedSkillContextFromMeta(meta op.Meta) op.Meta {
	if meta == nil {
		return nil
	}
	raw := meta["selectedSkillContext"]
	switch typed := raw.(type) {
	case op.Meta:
		return typed.Clone()
	case map[string]any:
		return op.Meta(typed).Clone()
	default:
		return nil
	}
}

func appendSkillsAppendix(basePrompt string, skills []op.OpNode) string {
	return agentprompt.BuildSystemPrompt(basePrompt, "", skillsToPromptContexts(skills), nil, nil)
}

func appendSelectedSkillsAppendix(basePrompt string, skills []op.OpNode, selectedSkillContext op.Meta) string {
	return agentprompt.BuildSystemPrompt(basePrompt, "", nil, skillsToPromptContexts(skills), selectedSkillContext)
}

func buildAgentSystemPrompt(basePrompt string, availableSkills []op.OpNode, selectedSkills []op.OpNode, selectedSkillContext op.Meta) string {
	return agentprompt.BuildSystemPrompt(
		basePrompt,
		"",
		skillsToPromptContexts(availableSkills),
		skillsToPromptContexts(selectedSkills),
		selectedSkillContext,
	)
}

func skillsToPromptContexts(skills []op.OpNode) []agentprompt.SkillContext {
	if len(skills) == 0 {
		return nil
	}
	contexts := make([]agentprompt.SkillContext, 0, len(skills))
	for _, skillNode := range skills {
		meta, ok := skillNode.Meta.(*op.SkillMeta)
		if !ok || meta == nil {
			continue
		}
		slug := strings.TrimSpace(meta.Slug)
		if slug == "" {
			slug = strings.TrimSpace(filepath.Base(strings.TrimSpace(skillNode.Cwd)))
		}
		contexts = append(contexts, agentprompt.SkillContext{
			ID:          strings.TrimSpace(skillNode.ID),
			Slug:        slug,
			Name:        strings.TrimSpace(meta.Name),
			Description: strings.TrimSpace(meta.Description),
			SkillFile:   strings.TrimSpace(op.URIToPath(skillNode.URI)),
			SkillDir:    strings.TrimSpace(skillNode.Cwd),
		})
	}
	return contexts
}

func loadPromptViaEndpoint(ctx context.Context, conn *Connection, node *op.OpNode, meta op.Meta) (string, bool) {
	if conn == nil {
		return "", false
	}
	requestMeta := op.Meta{}
	if meta != nil {
		requestMeta = meta.Clone()
	}
	if strings.TrimSpace(metaString(requestMeta, "agentID")) == "" {
		requestMeta["agentID"] = strings.TrimSpace(node.ID)
	}

	result, err := conn.OpNode(ctx, &op.OpNodeParams{
		OpCode: op.OpPromptGet,
		Meta:   requestMeta,
	})
	if err != nil {
		return "", false
	}
	text, ok := result.Content.(*op.TextContent)
	if !ok {
		return "", false
	}
	prompt := strings.TrimSpace(text.Text)
	if prompt == "" {
		return "", false
	}
	return prompt, true
}

func assembleTools(ctx context.Context, agentMeta *op.AgentMeta) map[string]*op.ToolSpec {

	toolSpecs := make(map[string]*op.ToolSpec, 0)
	if agentMeta == nil {
		return toolSpecs
	}

	for _, toolName := range resolvedSystoolNames(agentMeta) {
		if !builtintools.IsOSToolName(toolName) {
			continue
		}
		spec, ok := builtintools.OSToolSpec(toolName)
		if !ok {
			continue
		}
		normalized := normalizeToolName(spec.Name)
		if _, exists := toolSpecs[normalized]; !exists {
			toolSpecs[normalized] = spec
		}
	}

	for _, id := range agentMeta.ToolServers {
		nodeVal, ok := cache.GetValue[op.OpNode](id, cache.PrefixNode)
		if !ok {
			slog.Warn("configured tool server missing from node cache", "toolServerID", id)
			continue
		}
		node := &nodeVal
		toolsMeta, ok := node.Meta.(*op.ToolsMeta)
		if !ok {
			continue
		}
		for _, tool := range toolsMeta.Tools {
			if tool == nil {
				continue
			}
			toolName := normalizeToolName(tool.Name)
			if toolName == "" {
				continue
			}
			if _, ok := toolSpecs[toolName]; !ok {
				toolSpecs[toolName] = tool
			}
		}
	}
	return toolSpecs
}

func shouldExposeBuiltinSystool(agentMeta *op.AgentMeta, toolName string) bool {
	if agentMeta == nil {
		return false
	}
	toolName = normalizeToolName(toolName)
	if toolName == "" {
		return false
	}
	switch strings.TrimSpace(agentMeta.SysToolMode) {
	case op.SystoolModeDisabled:
		return false
	case op.SystoolModeAllowlist:
		for _, name := range agentMeta.SysTools {
			if normalizeToolName(name) == toolName {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func resolvedSystoolNames(agentMeta *op.AgentMeta) []string {
	if agentMeta == nil {
		return nil
	}
	switch strings.TrimSpace(agentMeta.SysToolMode) {
	case op.SystoolModeDisabled:
		return nil
	case op.SystoolModeAllowlist:
		return uniqueSystoolNames(agentMeta.SysTools)
	default:
		return append([]string(nil), op.SystoolNames...)
	}
}

func uniqueSystoolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = normalizeToolName(name)
		if !builtintools.IsBuiltinName(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// func createModelClient(ctx context.Context, modelID string, meta op.Meta) (*ModelClient, error) {

// 	if meta != nil {
// 		if model, ok := meta["model"].(string); ok && model != "" {
// 			modelID = model
// 		}
// 	}

// 	if modelID == "" {
// 		modelID = autoModelID
// 	}

// 	modelclient, err := NewModelClient(ctx, modelID)
// 	if err != nil {
// 		slog.Error("failed to create model client", "error", err)
// 		return nil, err
// 	}
// 	return modelclient, nil
// }

// // agentURIFromNode extracts the URI from an agent node.
// func agentURIFromNode(node *op.OpNode) string {
// 	if node == nil {
// 		return ""
// 	}
// 	return strings.TrimSpace(node.URI)
// }

// // agentRootFromNode extracts the agent root directory from a node.
// func agentRootFromNode(node *op.OpNode) string {
// 	uri := agentURIFromNode(node)
// 	if uri == "" {
// 		return ""
// 	}
// 	return op.URIToDir(uri)
// }

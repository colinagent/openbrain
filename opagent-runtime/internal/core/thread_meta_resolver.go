package core

import (
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func threadQueryFromMeta(meta op.Meta) op.ThreadMetaQuery {
	return op.ThreadMetaQuery{
		ThreadID: metaString(meta, "threadID"),
	}
}

func resolveThreadMetaFromMeta(meta op.Meta) (*op.ThreadMeta, error) {
	query := threadQueryFromMeta(meta)
	if strings.TrimSpace(query.ThreadID) == "" {
		return nil, fmt.Errorf("threadID is required")
	}
	return getThreadMeta(query.ThreadID, query.AgentID)
}

func threadMetaQuery(meta op.ThreadMeta) op.ThreadMetaQuery {
	return op.ThreadMetaQuery{
		ThreadID: strings.TrimSpace(meta.ThreadID),
	}
}

func applyResolvedThreadMetaToMeta(meta op.Meta, threadMeta op.ThreadMeta) op.Meta {
	next := meta.Clone()
	if next == nil {
		next = op.Meta{}
	}
	if threadID := strings.TrimSpace(threadMeta.ThreadID); threadID != "" {
		next["threadID"] = threadID
	}
	if fileID := strings.TrimSpace(threadMeta.FileID); fileID != "" {
		next["fileID"] = fileID
	}
	path := strings.TrimSpace(threadMeta.Path)
	if path == "" {
		path = strings.TrimSpace(threadMeta.ChatPath)
	}
	if path != "" {
		next["path"] = path
		next["chatPath"] = path
	}
	if cwd := strings.TrimSpace(threadMeta.CWD); cwd != "" {
		next["cwd"] = cwd
	}
	if title := strings.TrimSpace(threadMeta.Title); title != "" {
		next["title"] = title
	}
	return next
}

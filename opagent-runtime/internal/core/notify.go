package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/memory/cache"
)

var notifyChan = make(chan *op.InfoNotificationParams, 1000)

// NotifyProgress sends a notification with metadata to the thread.
func NotifyProgress(opCode op.OpCode, meta op.Meta, content op.Content) {
	params := &op.InfoNotificationParams{
		OpCode:  opCode,
		Meta:    meta.Clone(),
		Content: content,
	}
	// Use select so a full channel does not block and deadlock the process.
	select {
	case notifyChan <- params:
		// Sent successfully
	default:
		// When channel is full, log a warning and drop instead of blocking.
		slog.Warn("⚠️ notifyChan is full, dropping notification")
	}
}

func NotifyInfo(params *op.InfoNotificationParams) error {
	conns := cache.ListByPrefix[Connection](cache.PrefixConnection)
	if len(conns) == 0 {
		slog.Warn("no connections found")
		return nil
	}
	for _, conn := range conns {
		if conn.OpCodes == nil || len(conn.OpCodes) == 0 {
			continue
		}
		if slices.Contains(conn.OpCodes, params.OpCode) {
			err := conn.NotifyInfo(context.Background(), &op.InfoNotificationParams{
				OpCode:  params.OpCode,
				Meta:    params.Meta.Clone(),
				Content: params.Content,
			})
			if err != nil {
				slog.Error("❌ failed to send info notification", "error", err, "key", conn.NodeID)
				continue
			}
		}
	}

	return nil

}

// logContentDebug formats notify payloads for debug logs only. Writing these
// payloads to stdout is unsafe because stdout may be reserved for stdio
// protocol transports.
func logContentDebug(content op.Content) {
	switch c := content.(type) {
	case *op.TextContent:
		slog.Debug("notify text content", "text", c.Text)
	case *op.ImageContent:
		slog.Debug("notify image content", "mimeType", c.MIMEType)
	case *op.AudioContent:
		slog.Debug("notify audio content", "mimeType", c.MIMEType)
	default:
		if data, err := json.MarshalIndent(content, "", "  "); err == nil {
			slog.Debug("notify content", "payload", string(data))
		} else {
			slog.Debug("notify content", "payload", fmt.Sprintf("%+v", content))
		}
	}
}

func StartNotify(ctx context.Context) {
	// notifyChan is initialized at package level; no need to create it again.
	slog.Info("🚀 StartNotify started, waiting for messages...")
	for {
		select {
		case <-ctx.Done():
			slog.Info("StartNotify context done, exiting")
			return
		case params := <-notifyChan:
			if params == nil {
				continue
			}
			logContentDebug(params.Content)
			NotifyInfo(params)
		}
	}
}

package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

func InitLogger(debug bool, logFile string) {
	logFile, err := common.ExpandHome(logFile)
	if err != nil {
		slog.Error("Error expanding log file: %v", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		slog.Error("Failed to create log directory", "error", err)
		return
	}
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Failed to open log file", "error", err)
		return
	}
	// Runtime may host stdio-based child protocol sessions. Keep diagnostic logs
	// off stdout so they never contaminate JSON-RPC transport streams.
	writer := io.MultiWriter(os.Stderr, file)

	replace := func(groups []string, a slog.Attr) slog.Attr {
		// Remove time.
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			if source, ok := a.Value.Any().(*slog.Source); ok && source != nil {
				source.File = filepath.Base(source.File)
			}
		}
		return a
	}
	if debug {
		logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level:       slog.LevelDebug,
			AddSource:   true,
			ReplaceAttr: replace,
		}))
		slog.SetDefault(logger)
		return
	}

	// r := &lumberjack.Logger{
	// 	Filename:   logFile,
	// 	MaxSize:    10,
	// 	LocalTime:  true,
	// 	MaxAge:     7,
	// 	MaxBackups: 10,
	// 	Compress:   true,
	// }
	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		AddSource:   true,
		ReplaceAttr: replace,
	}))
	slog.SetDefault(logger)
}

package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hanamilabs/opencode-telegram-bridge/go-bridge/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg config.Config) (*slog.Logger, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.LogFilePath), 0o755); err != nil {
		return nil, err
	}

	rotatingWriter := &lumberjack.Logger{
		Filename:   cfg.LogFilePath,
		MaxSize:    cfg.LogMaxSizeMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAge:     cfg.LogMaxAgeDays,
		Compress:   true,
	}

	writer := io.MultiWriter(os.Stdout, rotatingWriter)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)})
	return slog.New(handler), nil
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

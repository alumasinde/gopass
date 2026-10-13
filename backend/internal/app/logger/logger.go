package logger

import (
	"log/slog"
	"os"
	"strings"
)

func New(v string) *slog.Logger {
	var l slog.Level
	switch strings.ToUpper(v) {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l, AddSource: true}))
}

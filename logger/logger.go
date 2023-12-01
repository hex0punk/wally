package logger

import (
	"log/slog"
	"os"
)

func NewLogger(level int) *slog.Logger {
	verbosity := parseVerbosity(level)
	opts := &slog.HandlerOptions{Level: verbosity}

	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	return logger
}

func parseVerbosity(verbosityFlag int) slog.Level {
	switch verbosityFlag {
	case 2:
		return slog.LevelInfo
	case 3:
		return slog.LevelDebug
	default:
		return slog.LevelError
	}
}

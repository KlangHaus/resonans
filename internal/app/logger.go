package app

import (
	"log/slog"
	"os"
	"strings"
)

// parseLevel maps the LOG_LEVEL env value to a slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "fatal", "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "debug", "trace":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// newStdoutLogger builds the base JSON logger written to stdout. The
// observability layer may tee this to OTLP when enabled.
func newStdoutLogger(level, serviceName, env string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(h).With("service", serviceName, "env", env)
}

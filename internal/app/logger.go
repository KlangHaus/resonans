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
// logNodeEnv says whether NODE_ENV was DECLARED or DEFAULTED, on a logger the
// caller has already built. It takes the two values rather than a *config.Config
// so this package keeps no dependency on config and the line stays testable.
//
// Every entry point builds its own logger and so has to say this itself.
//
// 🔴 The first version of this line (#102) was in Run() alone — the server —
// while the observation that motivated it came from the `resonans-migrate`
// container, which logged "env":"development" on a production box (measured
// 2026-10-06) and never calls Run(). A line absent from the one surface where
// the question gets asked is not a measurement. The one-shots now carry it too.
//
// Warn on the defaulted path: locally it is correct and costs one line per run,
// while on a deployed box it is the only thing that says so. Nothing branches on
// this; IsProduction() remains the only gate.
func logNodeEnv(l *slog.Logger, nodeEnv string, defaulted bool) {
	if defaulted {
		l.Warn("node_env", "value", nodeEnv, "source", "defaulted",
			"effect", "NODE_ENV was not set, so this process is NOT production: IsProduction() is false and every check behind it takes the non-production branch")
		return
	}
	l.Info("node_env", "value", nodeEnv, "source", "declared")
}

func newStdoutLogger(level, serviceName, env string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(h).With("service", serviceName, "env", env)
}

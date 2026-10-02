// Package logging builds the logger every Rulemart command writes with: JSON lines, at the level LOG_LEVEL names,
// each naming the release that wrote it. Lambda sends a function's standard output to CloudWatch Logs, which bills
// by the byte, so commands log one line per event worth keeping and nothing at debug level by default.
package logging

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"time"
)

const (
	// LevelEnv names the variable that holds the level to log at: debug, info, warn, or error. Unset means info.
	LevelEnv = "LOG_LEVEL"
	// ReleaseEnv names the variable that holds the release, such as v0.0.3, that every line names. Unset means
	// dev, a local build.
	ReleaseEnv = "RULEMART_RELEASE"
)

// New returns a logger that writes JSON lines to w at the level getenv(LevelEnv) names, with a release attribute
// from getenv(ReleaseEnv). When the level isn't one New knows, it returns an error and a logger at info level, so
// the caller can report the error in the same format before it stops.
func New(w io.Writer, getenv func(string) string) (*slog.Logger, error) {
	level, err := parseLevel(getenv(LevelEnv))
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	return logger.With("release", cmp.Or(getenv(ReleaseEnv), "dev")), err
}

// parseLevel returns the level text names, in any case, and info for empty text. It accepts slog's offsets too,
// such as info+2. It returns info and an error for any other text.
func parseLevel(text string) (slog.Level, error) {
	if text == "" {
		return slog.LevelInfo, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(text)); err != nil {
		return slog.LevelInfo, fmt.Errorf("read %s=%q: want debug, info, warn, or error", LevelEnv, text)
	}
	return level, nil
}

// Ready logs that a long-lived command has started: the schema version its release needs, and how long starting
// took.
func Ready(logger *slog.Logger, schemaVersion int64, took time.Duration) {
	logger.Info("ready", "schema", schemaVersion, "init_ms", Milliseconds(took))
}

// StartupFailed logs why a command couldn't start. The caller exits non-zero after it.
func StartupFailed(logger *slog.Logger, err error) {
	logger.Error("startup failed", "error", err.Error())
}

// Milliseconds returns d in milliseconds, to the microsecond, which keeps durations short in log lines.
func Milliseconds(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

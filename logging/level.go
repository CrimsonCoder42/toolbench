package logging

import (
	"log/slog"
	"strings"
)

// ParseLevel maps a level name to a slog level: "debug", "info", "warn" or "warning", "error", ignoring case
// and surrounding space. Anything else, "" included, is slog.LevelInfo; it never fails, so a mistyped
// setting still logs. Typical use: logging.New(os.Stdout, logging.ParseLevel(cfgValue)).
//
// ID:          TB-LOG-020
// Pattern:     level parsing (configuration to slog.Level)
// Authority:   pkg.go.dev log/slog, "type Level" (LevelDebug, LevelInfo, LevelWarn, LevelError)
// Tags:        logging, slog, level, config
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

package logging_test

import (
	"log/slog"
	"testing"

	"github.com/CrimsonCoder42/toolbench/logging"
)

// S5: the accepted names, any case and surrounding space; everything else is Info.
func TestParseLevel_S5_Table(t *testing.T) {
	for _, c := range []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{" Warning\n", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"trace", slog.LevelInfo},
		{"fatal", slog.LevelInfo},
		{"err", slog.LevelInfo},
		{"-4", slog.LevelInfo},
	} {
		if got := logging.ParseLevel(c.in); got != c.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

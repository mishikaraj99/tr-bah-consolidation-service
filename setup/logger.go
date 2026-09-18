package setup

import (
	"log/slog"
	"os"
)

// NewLogger returns a JSON slog logger; DEBUG outside production.
func NewLogger(env string) *slog.Logger {
	level := slog.LevelDebug
	if env == "production" {
		level = slog.LevelInfo
	}
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(l)
	return l
}

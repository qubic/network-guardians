package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/app"
	"github.com/qubic/network-guardians/internal/config"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "configs/config.json", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup logger based on config
	logger := setupLogger(cfg.Log)
	slog.SetDefault(logger)

	logger.Info("starting qubic guardians",
		"config", *configPath,
		"logLevel", cfg.Log.Level,
	)

	logger.Info("configuration loaded",
		"server", cfg.Server,
		"database.host", cfg.Database.Host,
	)

	// run application
	application := app.New(cfg, logger)

	ctx := context.Background()
	if err := application.Run(ctx); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}

	logger.Info("application exited")
}

// setupLogger creates a logger based on configuration
func setupLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	return slog.New(&prettyHandler{
		out:   os.Stdout,
		level: level,
	})
}

// Logs and formatting
type prettyHandler struct {
	out   io.Writer
	level slog.Level
	mu    sync.Mutex
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format timestamp
	timeStr := r.Time.Format(time.DateTime)

	// Format log level
	var levelStr string
	switch r.Level {
	case slog.LevelDebug:
		levelStr = colorGray + "DEBUG" + colorReset
	case slog.LevelInfo:
		levelStr = colorGreen + "INFO " + colorReset
	case slog.LevelWarn:
		levelStr = colorYellow + "WARN " + colorReset
	case slog.LevelError:
		levelStr = colorRed + "ERROR" + colorReset
	default:
		levelStr = "     "
	}

	// Build attributes string
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s%s%s=%v", colorCyan, a.Key, colorReset, a.Value.Any()))
		return true
	})

	// Format output
	attrStr := ""
	if len(attrs) > 0 {
		attrStr = " " + colorGray + "|" + colorReset + " " + strings.Join(attrs, " ")
	}

	fmt.Fprintf(h.out, "%s%s%s %s %s %s%s%s%s\n",
		colorGray, timeStr, colorReset,
		colorGray+"|"+colorReset,
		levelStr,
		colorGray+"|"+colorReset,
		" "+r.Message,
		attrStr,
		"",
	)

	return nil
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return h
}

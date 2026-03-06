package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/checker"
	"github.com/qubic/network-guardians/internal/service/reference"
)

// monitorEpochTransitions watches for epoch transitions and pauses the checker
// Wednesday 12:00 UTC + grace period config
func monitorEpochTransitions(ctx context.Context, checkerSvc *checker.DistributedService, gracePeriodMinutes int, logger *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	var inGracePeriod bool
	var gracePeriodStarted time.Time
	var lastPausedEpoch int16

	// Calculate current epoch for tracking
	now := time.Now().UTC()
	currentEpoch := epochForTime(now)

	// check if we're starting during a grace period (Wednesday 12:xx UTC)
	if now.Weekday() == time.Wednesday && now.Hour() == 12 {
		logger.Info("starting during epoch grace period, pausing checker")
		checkerSvc.Pause()
		inGracePeriod = true
		gracePeriodStarted = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
		lastPausedEpoch = currentEpoch
	} else {
		lastPausedEpoch = currentEpoch
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()

			if inGracePeriod {
				// Check if grace period is complete
				gracePeriodEnd := gracePeriodStarted.Add(time.Duration(gracePeriodMinutes) * time.Minute)
				if now.After(gracePeriodEnd) {
					logger.Info("grace period complete, resuming checker")
					checkerSvc.Resume()
					inGracePeriod = false
				}
			} else {
				newEpoch := epochForTime(now)
				if newEpoch > lastPausedEpoch && now.Weekday() == time.Wednesday && now.Hour() >= 12 {
					logger.Info("epoch transition detected, pausing checker for grace period",
						"epoch", newEpoch,
						"gracePeriodMinutes", gracePeriodMinutes,
					)
					checkerSvc.Pause()
					inGracePeriod = true
					gracePeriodStarted = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
					lastPausedEpoch = newEpoch
				}
			}
		}
	}
}

func epochForTime(t time.Time) int16 {
	// Epoch 198 started Wednesday, January 28, 2026 at 12:00 PM UTC - indexer point
	referenceDate := time.Date(2026, time.January, 28, 12, 0, 0, 0, time.UTC)
	referenceEpoch := 198
	duration := t.UTC().Sub(referenceDate)
	weeks := int(duration.Hours() / (24 * 7))
	return int16(referenceEpoch + weeks)
}

func main() {
	configPath := flag.String("config", "configs/config.json", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Validate checker settings
	if cfg.Checker.CheckerID == "" {
		fmt.Fprintf(os.Stderr, "Error: checker.checkerID is required\n")
		os.Exit(1)
	}

	// Setup logger
	logLevel := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	logger.Info("starting distributed checker",
		"checkerID", cfg.Checker.CheckerID,
		"region", cfg.Checker.Region,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to database
	pool, err := postgres.NewPool(ctx, &cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connected")

	// Create repositories
	nodeRepo := postgres.NewNodeRepository(pool)

	// Create reference service (needed for validation)
	referenceSvc := reference.NewService(&cfg.Reference, logger)
	referenceSvc.Start(ctx)
	defer referenceSvc.Stop()

	// Wait for reference data
	logger.Info("waiting for reference data")
	var refData *domain.ReferenceData
	for i := 0; i < 100; i++ {
		refData = referenceSvc.GetReference()
		if refData.GetTick() > 0 {
			logger.Info("reference data ready", "tick", refData.GetTick(), "epoch", refData.GetEpoch())
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if refData.GetTick() == 0 {
		logger.Error("failed to get reference data")
		os.Exit(1)
	}

	// Create and start distributed checker service
	checkerSvc := checker.NewDistributedService(
		&cfg.Checker,
		&cfg.Scoring,
		nodeRepo,
		refData,
		logger,
	)
	checkerSvc.Start(ctx)

	logger.Info("distributed checker running",
		"checkerID", cfg.Checker.CheckerID,
		"region", cfg.Checker.Region,
		"workers", cfg.Checker.WorkerCount,
		"claimBatch", cfg.Checker.ClaimBatch,
	)

	// Start epoch monitor goroutine to pause checker during epoch transitions (Grace period)
	go monitorEpochTransitions(ctx, checkerSvc, cfg.Epoch.GracePeriodMinutes, logger)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutdown signal received", "signal", sig)

	// Graceful shutdown
	cancel()
	checkerSvc.Stop()
	logger.Info("distributed checker stopped")
}

package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qubic/network-guardians/internal/api"
	"github.com/qubic/network-guardians/internal/cache"
	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/discovery"
	"github.com/qubic/network-guardians/internal/service/epoch"
	"github.com/qubic/network-guardians/internal/service/flagging"
	"github.com/qubic/network-guardians/internal/service/geoip"
	"github.com/qubic/network-guardians/internal/service/reference"
	"github.com/qubic/network-guardians/internal/service/scoring"
)

type App struct {
	cfg    *config.Config
	logger *slog.Logger

	// Database
	pool *pgxpool.Pool

	// Repos
	nodeRepo  *postgres.NodeRepository
	epochRepo *postgres.EpochRepository

	// Services
	referenceSvc *reference.Service
	geoipSvc     *geoip.Service
	discoverySvc *discovery.Service
	scoringSvc   *scoring.Service
	epochSvc     *epoch.Service
	flaggingSvc  *flagging.Service

	// HTTP
	httpServer *http.Server
	apiCache   *cache.Cache
}

// New creates a new app instance
func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts the application and blocks until shutdown
func (a *App) Run(ctx context.Context) error {
	// Initialize components
	if err := a.initialize(ctx); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	// Start services
	a.start(ctx)

	a.waitForShutdown(ctx)

	a.shutdown()

	return nil
}

// initialize sets up all components
func (a *App) initialize(ctx context.Context) error {
	a.logger.Info("initializing application")

	// db connection
	pool, err := postgres.NewPool(ctx, &a.cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	a.pool = pool
	a.logger.Info("database connected")

	// Repositories
	a.nodeRepo = postgres.NewNodeRepository(pool)
	a.epochRepo = postgres.NewEpochRepository(pool)

	a.apiCache = cache.New(time.Duration(a.cfg.Cache.TTL) * time.Second)

	// Services
	a.referenceSvc = reference.NewService(&a.cfg.Reference, a.logger)
	a.geoipSvc = geoip.NewService(a.logger)
	a.discoverySvc = discovery.NewService(&a.cfg.Discovery, a.nodeRepo, a.geoipSvc, a.logger)
	a.scoringSvc = scoring.NewService(&a.cfg.Scoring)
	a.epochSvc = epoch.NewService(
		&a.cfg.Epoch,
		a.nodeRepo,
		a.epochRepo,
		a.scoringSvc,
		a.logger,
	)

	// Flagging service
	a.flaggingSvc = flagging.NewService(&a.cfg.Flagging, a.nodeRepo, a.logger)

	// HTTP server
	router := api.NewRouter(a.nodeRepo, a.epochRepo, a.referenceSvc, a.epochSvc, &a.cfg.Epoch, &a.cfg.Scoring, a.apiCache)
	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	a.logger.Info("initialization complete")
	return nil
}

// start begins all services
func (a *App) start(ctx context.Context) {
	a.logger.Info("starting services")

	// Start in dependency order
	a.referenceSvc.Start(ctx)

	// Wait for valid reference data (up to 10 secs)
	a.logger.Info("waiting for reference data")
	for i := 0; i < 100; i++ {
		if ref := a.referenceSvc.GetReference(); ref.GetTick() > 0 {
			a.logger.Info("reference data ready", "tick", ref.GetTick())
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	_, currentEpoch, _, _ := a.referenceSvc.GetReference().Get()
	a.epochSvc.SetCurrentEpoch(currentEpoch)
	a.logger.Info("epoch service initialized", "epoch", currentEpoch)

	go a.syncEpochLoop(ctx)

	// Register epoch service callbacks for pause/resume control
	a.epochSvc.SetCallbacks(
		a.pauseServices,
		a.resumeServices,
		a.deleteAllNodes,
	)

	a.discoverySvc.Start(ctx)
	a.flaggingSvc.Start(ctx)
	a.epochSvc.Start(ctx)

	// Start HTTP server
	go func() {
		a.logger.Info("starting HTTP server", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	a.logger.Info("all services started")
}

// epoch transition handling

func (a *App) pauseServices() {
	a.logger.Info("pausing services for epoch transition")
	a.discoverySvc.Pause()
	a.flaggingSvc.Pause()
}

// resumeServices resumes discovery and flagging services after epoch transition
func (a *App) resumeServices() {
	a.logger.Info("resuming services after epoch transition")
	a.discoverySvc.Resume()
	a.flaggingSvc.Resume()
}

// deleteAllNodes deletes all nodes from the database for new epoch
func (a *App) deleteAllNodes(ctx context.Context) error {
	deleted, err := a.nodeRepo.DeleteAll(ctx)
	if err != nil {
		a.logger.Error("failed to delete all nodes", "error", err)
		return err
	}
	a.logger.Info("deleted all nodes for new epoch", "count", deleted)
	return nil
}

// syncEpochLoop periodically syncs the epoch from reference service to epoch service
func (a *App) syncEpochLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, newEpoch, _, _ := a.referenceSvc.GetReference().Get()
			if newEpoch > 0 {
				a.epochSvc.SetCurrentEpoch(newEpoch)
			}
		}
	}
}

func (a *App) waitForShutdown(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		a.logger.Info("shutdown signal received", "signal", sig)
	case <-ctx.Done():
		a.logger.Info("context cancelled")
	}
}

// shutdown gracefully stops all components
func (a *App) shutdown() {
	a.logger.Info("shutting down")

	// Stop HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.logger.Error("HTTP server shutdown error", "error", err)
	}

	// Stop services in reverse order
	a.epochSvc.Stop()
	a.flaggingSvc.Stop()
	a.discoverySvc.Stop()
	a.referenceSvc.Stop()

	// Close db
	a.pool.Close()

	a.logger.Info("shutdown complete")
}

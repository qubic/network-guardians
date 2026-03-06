package flagging

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/repository/postgres"
)

// Auto-flagging: Flags duplicate nodes (same IP or same operator)
// Rule: Latest first_seen_at wins -> newest registration keeps accumulating points -> old one ignored
type Service struct {
	cfg      *config.FlaggingConfig
	nodeRepo *postgres.NodeRepository
	logger   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Pause control for epoch transitions
	paused   bool
	pausedMu sync.RWMutex
}

// NewService creates a new flagging service
func NewService(
	cfg *config.FlaggingConfig,
	nodeRepo *postgres.NodeRepository,
	logger *slog.Logger,
) *Service {
	return &Service{
		cfg:      cfg,
		nodeRepo: nodeRepo,
		logger:   logger,
	}
}

// Start begins the auto-flagging loop
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		s.logger.Info("flagging service disabled")
		return
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.run()
	s.logger.Info("flagging service started", "pollInterval", s.cfg.PollInterval)
}

// Stop gracefully stops the auto-flagging loop
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("flagging service stopped")
}

// pauses the flagging service (for epoch transitions)
func (s *Service) Pause() {
	s.pausedMu.Lock()
	s.paused = true
	s.pausedMu.Unlock()
	s.logger.Info("flagging service paused")
}

// resumes the flagging service after pause
func (s *Service) Resume() {
	s.pausedMu.Lock()
	s.paused = false
	s.pausedMu.Unlock()
	s.logger.Info("flagging service resumed")
}

// returns whether the service is paused
func (s *Service) IsPaused() bool {
	s.pausedMu.RLock()
	defer s.pausedMu.RUnlock()
	return s.paused
}

// run is the main auto-flagging loop
func (s *Service) run() {
	defer s.wg.Done()

	// Run immediately on start
	if !s.IsPaused() {
		s.runFlaggingCycle()
	}

	ticker := time.NewTicker(time.Duration(s.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Skip cycle if paused
			if s.IsPaused() {
				continue
			}
			s.runFlaggingCycle()
		}
	}
}

// performs one cycle of auto-flagging
func (s *Service) runFlaggingCycle() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// First clear flags for nodes that are no longer duplicates
	clearedIP, err := s.nodeRepo.ClearDuplicateIPFlags(ctx)
	if err != nil {
		s.logger.Error("failed to clear duplicate IP flags", "error", err)
	} else if clearedIP > 0 {
		s.logger.Info("cleared duplicate IP flags", "count", clearedIP)
	}

	clearedOp, err := s.nodeRepo.ClearDuplicateOperatorFlags(ctx)
	if err != nil {
		s.logger.Error("failed to clear duplicate operator flags", "error", err)
	} else if clearedOp > 0 {
		s.logger.Info("cleared duplicate operator flags", "count", clearedOp)
	}

	// Then, flag new duplicates
	flaggedIP, err := s.nodeRepo.FlagDuplicateIPs(ctx)
	if err != nil {
		s.logger.Error("failed to flag duplicate IP nodes", "error", err)
	} else if flaggedIP > 0 {
		s.logger.Info("flagged duplicate IP nodes", "count", flaggedIP)
	}

	flaggedOp, err := s.nodeRepo.FlagDuplicateOperators(ctx)
	if err != nil {
		s.logger.Error("failed to flag duplicate operator nodes", "error", err)
	} else if flaggedOp > 0 {
		s.logger.Info("flagged duplicate operator nodes", "count", flaggedOp)
	}
}

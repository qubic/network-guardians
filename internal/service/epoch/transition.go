package epoch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/scoring"
)

// Qubic epoch reference point - epoch 198 started Wednesday, January 28, 2026 at 12:00 PM UTC
var (
	referenceEpoch = 198
	referenceDate  = time.Date(2026, time.January, 28, 12, 0, 0, 0, time.UTC)
)

// EpochPhase represents the current phase of epoch transition
type EpochPhase string

const (
	// PhaseActive is normal operations
	PhaseActive EpochPhase = "active"
	// PhaseTransitioning is when saving epoch results
	PhaseTransitioning EpochPhase = "transitioning"
	// PhaseGracePeriod is when services are paused, waiting
	PhaseGracePeriod EpochPhase = "grace_period"
	// PhaseStarting is when deleting nodes and resuming
	PhaseStarting EpochPhase = "starting"
)

// handles epoch transitions
type Service struct {
	cfg          *config.EpochConfig
	nodeRepo     *postgres.NodeRepository
	epochRepo    *postgres.EpochRepository
	scoringSvc   *scoring.Service
	currentEpoch uint16
	logger       *slog.Logger

	// Phase state machine
	phase              EpochPhase
	gracePeriodStarted time.Time
	transitionEpoch    int16

	// Tracks the last epoch we saved results for, so we never miss or double-fire
	lastSavedEpoch int16

	// Callbacks for controlling other services
	onPauseServices  func()
	onResumeServices func()
	onDeleteAllNodes func(ctx context.Context) error

	stopCh chan struct{}
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// creates a new epoch service
func NewService(
	cfg *config.EpochConfig,
	nodeRepo *postgres.NodeRepository,
	epochRepo *postgres.EpochRepository,
	scoringSvc *scoring.Service,
	logger *slog.Logger,
) *Service {
	return &Service{
		cfg:        cfg,
		nodeRepo:   nodeRepo,
		epochRepo:  epochRepo,
		scoringSvc: scoringSvc,
		logger:     logger,
		phase:      PhaseActive,
		stopCh:     make(chan struct{}),
	}
}

// registers callbacks for controlling other services
func (s *Service) SetCallbacks(
	onPauseServices func(),
	onResumeServices func(),
	onDeleteAllNodes func(ctx context.Context) error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPauseServices = onPauseServices
	s.onResumeServices = onResumeServices
	s.onDeleteAllNodes = onDeleteAllNodes
}

// Start begins the epoch monitoring
func (s *Service) Start(ctx context.Context) {
	s.checkStartupState()

	s.wg.Add(1)
	go s.monitorLoop(ctx)
	s.logger.Info("epoch service started", "phase", s.phase)
}

// detects if we should be in grace period on startup
func (s *Service) checkStartupState() {
	now := time.Now().UTC()

	// Check if it's Wednesday between 12:00 and 13:00 UTC (grace period window)
	if now.Weekday() != time.Wednesday || now.Hour() != 12 {
		currentEpoch := CalculateExpectedEpoch(now)
		s.mu.Lock()
		s.lastSavedEpoch = currentEpoch - 1
		s.mu.Unlock()
		return
	}

	transitionTime := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	justBeforeTransition := transitionTime.Add(-1 * time.Minute)
	epoch := CalculateExpectedEpoch(justBeforeTransition)

	// Check if epoch results exist to determine if transition already happened
	existing, err := s.epochRepo.GetByEpoch(context.Background(), epoch)
	if err != nil {
		s.logger.Error("failed to check existing epoch results on startup", "error", err)
		return
	}

	if len(existing) > 0 {
		// Transition already happened, we should be in grace period
		s.logger.Info("detected restart during grace period, entering grace period phase",
			"epoch", epoch)
		s.mu.Lock()
		s.phase = PhaseGracePeriod
		s.gracePeriodStarted = transitionTime
		s.transitionEpoch = epoch
		s.lastSavedEpoch = epoch
		s.mu.Unlock()

		// Pause services
		if s.onPauseServices != nil {
			s.onPauseServices()
		}
	} else {
		s.mu.Lock()
		s.lastSavedEpoch = epoch - 1
		s.mu.Unlock()
	}
}

// Stop gracefully stops the service
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("epoch service stopped")
}

// updates the current epoch from reference service
func (s *Service) SetCurrentEpoch(epoch uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentEpoch = epoch
}

// calculates the expected epoch for a given time
func CalculateExpectedEpoch(t time.Time) int16 {
	duration := t.UTC().Sub(referenceDate)
	weeks := int(duration.Hours() / (24 * 7))

	return int16(referenceEpoch + weeks)
}

// returns the current epoch
func (s *Service) GetCurrentEpoch() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentEpoch
}

// returns the current epoch phase
func (s *Service) GetPhase() EpochPhase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase
}

// returns current phase information for API
func (s *Service) GetPhaseInfo() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := map[string]interface{}{
		"phase": string(s.phase),
	}

	if s.phase == PhaseGracePeriod {
		gracePeriodEnd := s.gracePeriodStarted.Add(time.Duration(s.cfg.GracePeriodMinutes) * time.Minute)
		remaining := time.Until(gracePeriodEnd)
		if remaining < 0 {
			remaining = 0
		}
		info["gracePeriodStarted"] = s.gracePeriodStarted
		info["gracePeriodEnds"] = gracePeriodEnd
		info["gracePeriodRemainingSeconds"] = int64(remaining.Seconds())
		info["transitionEpoch"] = s.transitionEpoch
	}

	return info
}

// checks for epoch transitions (Wednesday 12:00 UTC)
func (s *Service) monitorLoop(ctx context.Context) {
	defer s.wg.Done()

	// Check every minute
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processPhase(ctx)
		}
	}
}

// handles the current phase logic
func (s *Service) processPhase(ctx context.Context) {
	now := time.Now().UTC()

	switch s.GetPhase() {
	case PhaseActive:
		s.checkTransition(ctx, now)
	case PhaseGracePeriod:
		s.checkGracePeriodComplete(ctx, now)
	}
}

// checks if it's time for epoch transition (Wednesday 12:00 UTC)
func (s *Service) checkTransition(ctx context.Context, now time.Time) {
	if now.Weekday() != time.Wednesday || now.Hour() < 12 {
		return
	}

	transitionTime := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	justBeforeTransition := transitionTime.Add(-1 * time.Minute)
	epoch := CalculateExpectedEpoch(justBeforeTransition)

	if epoch <= 0 {
		s.logger.Warn("invalid epoch calculated, skipping transition", "epoch", epoch)
		return
	}

	s.mu.RLock()
	alreadySaved := epoch <= s.lastSavedEpoch
	s.mu.RUnlock()

	if alreadySaved {
		return
	}

	s.logger.Info("epoch transition detected",
		"calculatedEpoch", epoch,
		"rpcEpoch", s.GetCurrentEpoch(),
	)

	// Double-check against database in case of process restart
	existing, err := s.epochRepo.GetByEpoch(ctx, epoch)
	if err != nil {
		s.logger.Error("failed to check existing epoch results", "error", err)
		return
	}

	if len(existing) > 0 {
		s.logger.Info("epoch already processed in database, updating tracker", "epoch", epoch)
		s.mu.Lock()
		s.lastSavedEpoch = epoch
		s.mu.Unlock()
		return
	}

	// Begin transition
	if err := s.beginTransition(ctx, epoch); err != nil {
		s.logger.Error("failed to begin epoch transition", "epoch", epoch, "error", err)
		return
	}

	// Mark as saved
	s.mu.Lock()
	s.lastSavedEpoch = epoch
	s.mu.Unlock()

	// Enter grace period
	s.enterGracePeriod(now, epoch)
}

// pauses services first, then saves epoch results to ensure a consistent snapshot
func (s *Service) beginTransition(ctx context.Context, epoch int16) error {
	s.mu.Lock()
	s.phase = PhaseTransitioning
	s.mu.Unlock()

	s.logger.Info("beginning epoch transition", "epoch", epoch)

	// Pause services BEFORE snapshotting so discovery, flagging, and checkers
	if s.onPauseServices != nil {
		s.onPauseServices()
		s.logger.Info("services paused, waiting for in-flight operations to settle")
		// Give in-flight operations time to complete
		time.Sleep(10 * time.Second)
	}

	if err := s.saveEpochResults(ctx, epoch); err != nil {
		// Resume services on failure so the system doesn't stay paused
		if s.onResumeServices != nil {
			s.onResumeServices()
		}
		s.mu.Lock()
		s.phase = PhaseActive
		s.mu.Unlock()
		return err
	}

	return nil
}

// sets up the grace period state
func (s *Service) enterGracePeriod(now time.Time, epoch int16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.phase = PhaseGracePeriod
	s.gracePeriodStarted = now
	s.transitionEpoch = epoch

	s.logger.Info("entered grace period",
		"epoch", epoch,
		"gracePeriodMinutes", s.cfg.GracePeriodMinutes,
		"endsAt", now.Add(time.Duration(s.cfg.GracePeriodMinutes)*time.Minute),
	)
}

// checks if the grace period is complete (1:00 PM UTC Wednesday)
func (s *Service) checkGracePeriodComplete(ctx context.Context, now time.Time) {
	s.mu.RLock()
	gracePeriodEnd := s.gracePeriodStarted.Add(time.Duration(s.cfg.GracePeriodMinutes) * time.Minute)
	maxTimeout := s.gracePeriodStarted.Add(2 * time.Hour) // Max 2 hour timeout
	s.mu.RUnlock()

	if now.After(maxTimeout) {
		s.logger.Warn("grace period max timeout exceeded, forcing completion")
		s.completeTransition(ctx)
		return
	}

	if now.Before(gracePeriodEnd) {
		return
	}

	s.logger.Info("grace period complete, starting new epoch")
	s.completeTransition(ctx)
}

// deletes all nodes and resumes services
func (s *Service) completeTransition(ctx context.Context) {
	s.mu.Lock()
	s.phase = PhaseStarting
	epoch := s.transitionEpoch
	s.mu.Unlock()

	s.logger.Info("completing epoch transition", "epoch", epoch)

	// 1. Delete all nodes
	if s.onDeleteAllNodes != nil {
		if err := s.onDeleteAllNodes(ctx); err != nil {
			s.logger.Error("failed to delete all nodes", "error", err)
		}
	}

	// 2. Resume services
	if s.onResumeServices != nil {
		s.onResumeServices()
		s.logger.Info("services resumed for new epoch")
	}

	// 3. Return to active phase
	s.mu.Lock()
	s.phase = PhaseActive
	s.transitionEpoch = 0
	s.gracePeriodStarted = time.Time{}
	s.mu.Unlock()

	// Calculate the new epoch we're starting
	newEpoch := CalculateExpectedEpoch(time.Now().UTC())
	s.logger.Info("epoch transition completed",
		"completedEpoch", epoch,
		"newEpoch", newEpoch,
	)
}

func (s *Service) saveEpochResults(ctx context.Context, epoch int16) error {
	s.logger.Info("saving epoch results", "epoch", epoch)

	// 1. Get nodes eligible for rewards deduplicated by operator AND IP
	nodes, err := s.nodeRepo.GetEligibleForRewards(ctx)
	if err != nil {
		return err
	}

	// Log stats
	totalCount, _ := s.nodeRepo.Count(ctx)
	flaggedCount, _ := s.nodeRepo.GetFlaggedCount(ctx)
	activeCount := totalCount - flaggedCount

	s.logger.Info("epoch transition node stats",
		"totalNodes", totalCount,
		"flaggedNodes", flaggedCount,
		"activeNodes", activeCount,
		"eligibleForRewards", len(nodes),
		"excludedDuplicates", activeCount-len(nodes),
	)

	// 2. Calculate scores and create epoch results
	var results []*domain.EpochResult
	eligibilityCfg := domain.EligibilityConfig{
		UptimeEnabled:    s.cfg.UptimeThreshold.Enabled,
		MinUptimePercent: s.cfg.UptimeThreshold.Value,
		SyncEnabled:      s.cfg.SyncThreshold.Enabled,
		MinSyncPercent:   s.cfg.SyncThreshold.Value,
		MinChecksEnabled: s.cfg.MinChecksThreshold.Enabled,
		MinChecks:        int(s.cfg.MinChecksThreshold.Value),
	}

	for _, node := range nodes {
		uptimeScore, syncScore, finalScore, rewardPoints := s.scoringSvc.CalculateScores(node)

		eligible, disqualifyReason := domain.CheckEligibility(
			uptimeScore, syncScore, node.TotalChecks, eligibilityCfg,
		)

		var disqualifyReasonPtr *string
		if disqualifyReason != "" {
			disqualifyReasonPtr = &disqualifyReason
		}

		result := &domain.EpochResult{
			Operator:         node.Operator,
			Epoch:            epoch,
			Type:             node.Type,
			Alias:            node.Alias,
			TotalChecks:      node.TotalChecks,
			SuccessfulChecks: node.SuccessfulChecks,
			UptimeScore:      uptimeScore,
			SyncScore:        syncScore,
			FinalScore:       finalScore,
			Eligible:         eligible,
			DisqualifyReason: disqualifyReasonPtr,
			RewardPoints:     rewardPoints,
			CalculatedAt:     time.Now(),
		}

		results = append(results, result)
	}

	// 3. Calculate rewards before persisting
	rewards := s.buildRewardsMap(results)

	// 4. Insert epoch results and update rewards atomically
	if err := s.epochRepo.InsertBatchWithRewards(ctx, results, epoch, rewards); err != nil {
		return fmt.Errorf("failed to save epoch results: %w", err)
	}

	s.logger.Info("epoch results and rewards saved",
		"epoch", epoch,
		"nodes", len(results),
		"rewardedNodes", len(rewards),
	)

	return nil
}

// buildRewardsMap computes the reward distribution without touching the database
func (s *Service) buildRewardsMap(results []*domain.EpochResult) map[string]int64 {
	// Separate results by type
	var liteResults, bobResults []*domain.EpochResult

	for _, r := range results {
		if r.Type == domain.NodeTypeLite {
			liteResults = append(liteResults, r)
		} else {
			bobResults = append(bobResults, r)
		}
	}

	// Use configured pool amount
	totalPool := s.cfg.TotalPoolAmount

	litePool := int64(float64(totalPool) * s.cfg.LitePoolPercent / 100)
	bobPool := int64(float64(totalPool) * s.cfg.BobPoolPercent / 100)

	// Calculate rewards for each pool
	liteRewards := s.scoringSvc.CalculateRewardDistribution(liteResults, litePool)
	bobRewards := s.scoringSvc.CalculateRewardDistribution(bobResults, bobPool)

	// Merge rewards
	allRewards := make(map[string]int64)
	for k, v := range liteRewards {
		allRewards[k] = v
	}
	for k, v := range bobRewards {
		allRewards[k] = v
	}

	return allRewards
}

// executes the epoch transition
func (s *Service) PerformTransition(ctx context.Context, epoch int16) error {
	// Save epoch results
	if err := s.saveEpochResults(ctx, epoch); err != nil {
		return err
	}

	if err := s.nodeRepo.ResetCounters(ctx); err != nil {
		return err
	}

	s.logger.Info("epoch transition completed (legacy)", "epoch", epoch)
	return nil
}

// allows manual triggering of epoch transition
func (s *Service) ForceTransition(ctx context.Context, epoch int16) error {
	return s.PerformTransition(ctx, epoch)
}

// allows manual triggering of the new grace period transition
func (s *Service) ForceGracePeriodTransition(ctx context.Context, epoch int16) error {
	now := time.Now().UTC()

	// Begin transition
	if err := s.beginTransition(ctx, epoch); err != nil {
		return err
	}

	// Track that this epoch has been saved (beginTransition already persisted results)
	s.lastSavedEpoch = epoch

	// Enter grace period
	s.enterGracePeriod(now, epoch)

	return nil
}

// allows manual completion of grace period
func (s *Service) ForceCompleteGracePeriod(ctx context.Context) {
	if s.GetPhase() != PhaseGracePeriod {
		s.logger.Warn("not in grace period, cannot force complete")
		return
	}
	s.completeTransition(ctx)
}

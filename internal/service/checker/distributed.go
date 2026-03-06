package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
)

// maxResponseSize (1 MB)
const maxResponseSize = 1 << 20

type DistributedService struct {
	cfg       *config.CheckerConfig
	nodeRepo  *postgres.NodeRepository
	validator *Validator
	client    *http.Client
	logger    *slog.Logger

	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex

	// Pause control for epoch transitions
	paused   bool
	pausedMu sync.RWMutex
}

// NewDistributedService creates a new distributed checker service
func NewDistributedService(
	cfg *config.CheckerConfig,
	scoringCfg *config.ScoringConfig,
	nodeRepo *postgres.NodeRepository,
	reference *domain.ReferenceData,
	logger *slog.Logger,
) *DistributedService {
	return &DistributedService{
		cfg:       cfg,
		nodeRepo:  nodeRepo,
		validator: NewValidator(scoringCfg, reference),
		client: &http.Client{
			Timeout: time.Duration(cfg.CheckTimeout) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start begins the distributed check cycle
func (s *DistributedService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Start worker pool - each worker claims and processes nodes independently
	for i := 0; i < s.cfg.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}

	s.logger.Info("distributed checker service started",
		"checkerID", s.cfg.CheckerID,
		"region", s.cfg.Region,
		"workers", s.cfg.WorkerCount,
		"claimBatch", s.cfg.ClaimBatch,
		"claimTTL", s.cfg.ClaimTTL,
	)
}

// Stop gracefully stops the service
func (s *DistributedService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("distributed checker service stopped", "checkerID", s.cfg.CheckerID)
}

// epoch transition handling
func (s *DistributedService) Pause() {
	s.pausedMu.Lock()
	s.paused = true
	s.pausedMu.Unlock()
	s.logger.Info("distributed checker service paused", "checkerID", s.cfg.CheckerID)
}

// Resume the checker service after grace
func (s *DistributedService) Resume() {
	s.pausedMu.Lock()
	s.paused = false
	s.pausedMu.Unlock()
	s.logger.Info("distributed checker service resumed", "checkerID", s.cfg.CheckerID)
}

// IsPaused returns whether the service is paused
func (s *DistributedService) IsPaused() bool {
	s.pausedMu.RLock()
	defer s.pausedMu.RUnlock()
	return s.paused
}

// worker claims and processes nodes
func (s *DistributedService) worker(ctx context.Context, id int) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			// Skip if paused
			if s.IsPaused() {
				time.Sleep(time.Second)
				continue
			}

			// Claim a batch of nodes
			nodes, err := s.nodeRepo.ClaimNodes(
				ctx,
				s.cfg.CheckerID,
				s.cfg.ClaimBatch,
				s.cfg.ClaimTTL,
			)
			if err != nil {
				s.logger.Error("failed to claim nodes",
					"worker", id,
					"error", err,
				)
				time.Sleep(time.Second)
				continue
			}

			if len(nodes) == 0 {
				time.Sleep(2 * time.Second)
				continue
			}

			s.logger.Debug("claimed nodes",
				"worker", id,
				"count", len(nodes),
			)

			// Process each claimed node
			for _, node := range nodes {
				select {
				case <-ctx.Done():
					return
				case <-s.stopCh:
					return
				default:
					s.checkNode(ctx, node)
				}
			}
		}
	}
}

// checkNode performs a health check on a single node
func (s *DistributedService) checkNode(ctx context.Context, node *domain.Node) {
	result := &domain.CheckResult{
		Operator:  node.Operator,
		NodeType:  node.Type,
		Timestamp: time.Now(),
	}

	var syncScore float64
	var nodeTick uint32
	var err error

	switch node.Type {
	case domain.NodeTypeLite:
		syncScore, nodeTick, err = s.checkLiteNode(ctx, node)
	case domain.NodeTypeBob:
		syncScore, nodeTick, err = s.checkBobNode(ctx, node)
	default:
		err = fmt.Errorf("unknown node type: %s", node.Type)
	}

	if err != nil {
		result.Success = false
		if valErr, ok := err.(*ValidationError); ok {
			result.FailureReason = valErr.Reason
		} else {
			result.FailureReason = "check_failed"
		}
		s.logger.Debug("node check failed",
			"operator", node.Operator,
			"type", node.Type,
			"reason", result.FailureReason,
			"checkerID", s.cfg.CheckerID,
		)
	} else {
		result.Success = true
		result.SyncScore = syncScore
		result.NodeTick = nodeTick
		result.ReferenceTick = s.validator.reference.GetTick()
		s.logger.Debug("node check succeeded",
			"operator", node.Operator,
			"type", node.Type,
			"syncScore", syncScore,
			"nodeTick", nodeTick,
			"referenceTick", result.ReferenceTick,
			"checkerID", s.cfg.CheckerID,
		)
	}

	// Update node counters, set next check time with jitter and release claim
	if err := s.nodeRepo.UpdateCheckResultAndRelease(ctx, s.cfg.CheckerID, result, s.cfg.BaseInterval, s.cfg.JitterMax); err != nil {
		s.logger.Error("failed to update check result",
			"operator", node.Operator,
			"error", err,
		)
	}
}

// checkLiteNode checks a lite node at :41841/tick-info
func (s *DistributedService) checkLiteNode(ctx context.Context, node *domain.Node) (float64, uint32, error) {
	if err := validateNodeIP(node.CurrentIP); err != nil {
		return 0, 0, err
	}

	url := fmt.Sprintf("http://%s:%d/tick-info", node.CurrentIP, s.cfg.LitePort)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, &ValidationError{Reason: "connection_failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, &ValidationError{Reason: fmt.Sprintf("http_%d", resp.StatusCode)}
	}

	var nodeResp LiteNodeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&nodeResp); err != nil {
		return 0, 0, &ValidationError{Reason: "invalid_response"}
	}

	syncScore, err := s.validator.ValidateLiteNode(node.Operator, &nodeResp)
	return syncScore, nodeResp.Tick, err
}

// checkBobNode checks a bob node at :40420/status
func (s *DistributedService) checkBobNode(ctx context.Context, node *domain.Node) (float64, uint32, error) {
	if err := validateNodeIP(node.CurrentIP); err != nil {
		return 0, 0, err
	}

	url := fmt.Sprintf("http://%s:%d/status", node.CurrentIP, s.cfg.BobPort)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, &ValidationError{Reason: "connection_failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, &ValidationError{Reason: fmt.Sprintf("http_%d", resp.StatusCode)}
	}

	var nodeResp BobNodeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&nodeResp); err != nil {
		return 0, 0, &ValidationError{Reason: "invalid_response"}
	}

	syncScore, err := s.validator.ValidateBobNode(node.Operator, &nodeResp)
	return syncScore, nodeResp.CurrentFetchingTick, err
}

package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
)

// Service runs health checks on nodes using a worker pool
type Service struct {
	cfg       *config.CheckerConfig
	nodeRepo  *postgres.NodeRepository
	validator *Validator
	client    *http.Client
	logger    *slog.Logger

	jobCh       chan *domain.Node
	stopCh      chan struct{}
	wg          sync.WaitGroup
	running     bool
	mu          sync.Mutex
	nextCheckAt map[string]time.Time // in-memory
	scheduleMu  sync.RWMutex

	// Pause control for epoch transitions
	paused   bool
	pausedMu sync.RWMutex
}

// NewService creates a new checker service
func NewService(
	cfg *config.CheckerConfig,
	scoringCfg *config.ScoringConfig,
	nodeRepo *postgres.NodeRepository,
	reference *domain.ReferenceData,
	logger *slog.Logger,
) *Service {
	return &Service{
		cfg:       cfg,
		nodeRepo:  nodeRepo,
		validator: NewValidator(scoringCfg, reference),
		client: &http.Client{
			Timeout: time.Duration(cfg.CheckTimeout) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger:      logger,
		jobCh:       make(chan *domain.Node, cfg.WorkerCount*2),
		stopCh:      make(chan struct{}),
		nextCheckAt: make(map[string]time.Time),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Start worker pool
	for i := 0; i < s.cfg.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}

	// Start scheduler
	s.wg.Add(1)
	go s.scheduler(ctx)

	s.logger.Info("checker service started",
		"workers", s.cfg.WorkerCount,
		"baseInterval", s.cfg.BaseInterval,
		"jitterMax", s.cfg.JitterMax,
	)
}

// Stop the service
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("checker service stopped")
}

// epoch transitions handling
func (s *Service) Pause() {
	s.pausedMu.Lock()
	s.paused = true
	s.pausedMu.Unlock()
	s.logger.Info("checker service paused")
}

// Resume checker
func (s *Service) Resume() {
	s.pausedMu.Lock()
	s.paused = false
	s.pausedMu.Unlock()

	// Clear the next check schedule to check immediately
	s.scheduleMu.Lock()
	s.nextCheckAt = make(map[string]time.Time)
	s.scheduleMu.Unlock()

	s.logger.Info("checker service resumed")
}

// IsPaused returns whether the service is paused
func (s *Service) IsPaused() bool {
	s.pausedMu.RLock()
	defer s.pausedMu.RUnlock()
	return s.paused
}

// scheduler polls for nodes due for checking
func (s *Service) scheduler(ctx context.Context) {
	defer s.wg.Done()

	// Poll frequently to pick up nodes as they become due
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.scheduleDueNodes(ctx)
		}
	}
}

// nodeKey returns a unique key for operator+type combination
func nodeKey(operator string, nodeType domain.NodeType) string {
	return operator + ":" + string(nodeType)
}

// scheduleDueNodes fetches active nodes and queues those due for checking
func (s *Service) scheduleDueNodes(ctx context.Context) {
	if s.IsPaused() {
		return
	}

	// Get active nodes from database (all non-flagged nodes)
	nodes, err := s.nodeRepo.GetActiveNodes(ctx)
	if err != nil {
		s.logger.Error("failed to get active nodes", "error", err)
		return
	}

	now := time.Now()
	var scheduled int

	for _, node := range nodes {
		key := nodeKey(node.Operator, node.Type)

		// Check if node is due for checking
		s.scheduleMu.RLock()
		nextCheck, exists := s.nextCheckAt[key]
		s.scheduleMu.RUnlock()

		// New nodes or nodes past their next check time are due
		if !exists || now.After(nextCheck) {
			s.scheduleMu.Lock()
			s.nextCheckAt[key] = now.Add(time.Hour)
			s.scheduleMu.Unlock()

			select {
			case s.jobCh <- node:
				scheduled++
			default:
				// Channel full -> reset -> try next cycle
				s.scheduleMu.Lock()
				s.nextCheckAt[key] = now
				s.scheduleMu.Unlock()
				s.logger.Debug("job channel full", "scheduled", scheduled)
				return
			}
		}
	}

	if scheduled > 0 {
		s.logger.Debug("scheduled nodes for checking", "count", scheduled)
	}
}

// worker processes check jobs
func (s *Service) worker(ctx context.Context, id int) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case node, ok := <-s.jobCh:
			if !ok {
				return
			}
			s.checkNode(ctx, node)
		}
	}
}

// checkNode performs a health check on a single node
func (s *Service) checkNode(ctx context.Context, node *domain.Node) {
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
			"reason", result.FailureReason,
		)
	} else {
		result.Success = true
		result.SyncScore = syncScore
		result.NodeTick = nodeTick
		result.ReferenceTick = s.validator.reference.GetTick()
		s.logger.Debug("node check succeeded",
			"operator", node.Operator,
			"syncScore", syncScore,
			"nodeTick", nodeTick,
			"referenceTick", result.ReferenceTick,
		)
	}

	// Update node counters in database
	if err := s.nodeRepo.UpdateCheckResult(ctx, result); err != nil {
		s.logger.Error("failed to update check result",
			"operator", node.Operator,
			"error", err,
		)
	}

	// Schedule next check with per-node jitter (in-memory)
	jitter := time.Duration(rand.Intn(s.cfg.JitterMax+1)) * time.Second
	nextCheckTime := time.Now().Add(time.Duration(s.cfg.BaseInterval)*time.Second + jitter)

	s.scheduleMu.Lock()
	s.nextCheckAt[nodeKey(node.Operator, node.Type)] = nextCheckTime
	s.scheduleMu.Unlock()
}

// checkLiteNode checks a lite node at :41841/tick-info
func (s *Service) checkLiteNode(ctx context.Context, node *domain.Node) (float64, uint32, error) {
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

	s.logger.Debug("lite node response",
		"operator", node.Operator,
		"tick", nodeResp.Tick,
		"epoch", nodeResp.Epoch,
		"timestamp", nodeResp.ExtraInfo.Timestamp,
	)

	syncScore, err := s.validator.ValidateLiteNode(node.Operator, &nodeResp)
	return syncScore, nodeResp.Tick, err
}

// checkBobNode checks a bob node at :40420/status
func (s *Service) checkBobNode(ctx context.Context, node *domain.Node) (float64, uint32, error) {
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

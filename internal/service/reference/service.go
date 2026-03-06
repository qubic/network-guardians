package reference

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
)

// maxReferenceResponseSize (1 MB)
const maxReferenceResponseSize = 1 << 20

// represents the response {"tickInfo":{"tick":...,"epoch":...}}
type TickInfoResponse struct {
	TickInfo struct {
		Tick  uint32 `json:"tick"`
		Epoch uint16 `json:"epoch"`
	} `json:"tickInfo"`
}

// represents the response from direct format: {"tick":...,"epoch":...}
type DirectResponse struct {
	Tick  uint32 `json:"tick"`
	Epoch uint16 `json:"epoch"`
}

// polls multiple APIs to get the reference tick
type Service struct {
	cfg       *config.ReferenceConfig
	client    *http.Client
	reference *domain.ReferenceData
	logger    *slog.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// creates a new reference tick service
func NewService(cfg *config.ReferenceConfig, logger *slog.Logger) *Service {
	return &Service{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		reference: domain.NewReferenceData(),
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start begins polling tick APIs
func (s *Service) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.pollLoop(ctx)
	s.logger.Info("reference tick service started",
		"apis", s.cfg.GetEnabledAPIs(),
		"interval", s.cfg.PollInterval,
	)
}

// Stop gracefully stops the service
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("reference tick service stopped")
}

// returns the current reference data
func (s *Service) GetReference() *domain.ReferenceData {
	return s.reference
}

// pollLoop continuously polls tick APIs
func (s *Service) pollLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	// Initial poll
	s.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll fetches tick from all APIs and takes the highest
func (s *Service) poll(ctx context.Context) {
	type result struct {
		tick   uint32
		epoch  uint16
		source string
		err    error
	}

	enabledAPIs := s.cfg.GetEnabledAPIs()
	if len(enabledAPIs) == 0 {
		s.logger.Warn("no enabled reference APIs configured")
		return
	}

	results := make(chan result, len(enabledAPIs))
	var wg sync.WaitGroup

	for _, api := range enabledAPIs {
		wg.Add(1)
		go func(apiURL string) {
			defer wg.Done()

			tick, epoch, err := s.fetchTick(ctx, apiURL)
			results <- result{
				tick:   tick,
				epoch:  epoch,
				source: apiURL,
				err:    err,
			}
		}(api)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var highestTick uint32
	var winningEpoch uint16
	var winningSource string

	for r := range results {
		if r.err != nil {
			s.logger.Warn("failed to fetch tick",
				"source", r.source,
				"error", r.err,
			)
			continue
		}

		if r.tick > highestTick {
			highestTick = r.tick
			winningEpoch = r.epoch
			winningSource = r.source
		}
	}

	if highestTick > 0 {
		s.reference.Update(highestTick, winningEpoch, winningSource)
		s.logger.Debug("reference tick updated",
			"tick", highestTick,
			"epoch", winningEpoch,
			"source", winningSource,
		)
	}
}

// fetchTick fetches tick info from a single API
// Supports multiple response formats based on the API endpoint
func (s *Service) fetchTick(ctx context.Context, apiURL string) (uint32, uint16, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse based on API format
	limitedBody := io.LimitReader(resp.Body, maxReferenceResponseSize)
	if strings.Contains(apiURL, "rpc.qubic.org") {
		var tickResp TickInfoResponse
		if err := json.NewDecoder(limitedBody).Decode(&tickResp); err != nil {
			return 0, 0, fmt.Errorf("failed to decode tickInfo response: %w", err)
		}
		return tickResp.TickInfo.Tick, tickResp.TickInfo.Epoch, nil
	}

	var directResp DirectResponse
	if err := json.NewDecoder(limitedBody).Decode(&directResp); err != nil {
		return 0, 0, fmt.Errorf("failed to decode direct response: %w", err)
	}
	return directResp.Tick, directResp.Epoch, nil
}

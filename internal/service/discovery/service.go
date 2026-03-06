package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/geoip"
)

// maxDiscoveryResponseSize (20 MB)
const maxDiscoveryResponseSize = 20 << 20

// CheckinsAPIResponse represents the wrapper response from the API
type CheckinsAPIResponse struct {
	Checkins []*CheckinResponse `json:"checkins"`
}

// CheckinResponse represents a node checkin from the API
type CheckinResponse struct {
	Operator  string `json:"operator"`
	IP        string `json:"ip"`
	Alias     string `json:"alias,omitempty"`
	Type      string `json:"type"`      // "lite" or "bob"
	Timestamp int64  `json:"timestamp"` // Unix timestamp in milliseconds
}

// Service discovers and registers nodes
type Service struct {
	cfg      *config.DiscoveryConfig
	client   *http.Client
	nodeRepo *postgres.NodeRepository
	geoSvc   *geoip.Service
	logger   *slog.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup

	// Pause control for epoch transitions
	paused   bool
	pausedMu sync.RWMutex
}

// creates a new discovery service
func NewService(cfg *config.DiscoveryConfig, nodeRepo *postgres.NodeRepository, geoSvc *geoip.Service, logger *slog.Logger) *Service {
	return &Service{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		nodeRepo: nodeRepo,
		geoSvc:   geoSvc,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the discovery polling loop
func (s *Service) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.pollLoop(ctx)
	s.logger.Info("discovery service started",
		"endpoint", s.cfg.Endpoint,
		"interval", s.cfg.PollInterval,
	)
}

func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("discovery service stopped")
}

// epoch transitions handling
func (s *Service) Pause() {
	s.pausedMu.Lock()
	s.paused = true
	s.pausedMu.Unlock()
	s.logger.Info("discovery service paused")
}

// resumes the discovery service after pause
func (s *Service) Resume() {
	s.pausedMu.Lock()
	s.paused = false
	s.pausedMu.Unlock()
	s.logger.Info("discovery service resumed")
}

// returns whether the service is paused
func (s *Service) IsPaused() bool {
	s.pausedMu.RLock()
	defer s.pausedMu.RUnlock()
	return s.paused
}

// continuously polls the checkins endpoint
func (s *Service) pollLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	// Initial poll (only if not paused)
	if !s.IsPaused() {
		s.poll(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.IsPaused() {
				continue
			}
			s.poll(ctx)
		}
	}
}

// poll fetches and processes checkins
func (s *Service) poll(ctx context.Context) {
	checkins, err := s.fetchCheckins(ctx)
	if err != nil {
		s.logger.Error("failed to fetch checkins", "error", err)
		return
	}

	// Group checkins by operator+type use latest timestamp per combination
	operatorMap := make(map[string]*CheckinResponse)
	for _, checkin := range checkins {
		key := checkin.Operator + ":" + checkin.Type
		existing, ok := operatorMap[key]
		if !ok || checkin.Timestamp > existing.Timestamp {
			operatorMap[key] = checkin
		}
	}

	var upserted, failed, skipped int
	for _, checkin := range operatorMap {
		// Validate IP before accepting into the system
		if !isValidPublicIP(checkin.IP) {
			s.logger.Warn("rejected node with non-public IP",
				"operator", checkin.Operator,
				"ip", checkin.IP,
			)
			skipped++
			continue
		}

		nodeType := s.classifyNodeType(checkin.Type)

		var alias *string
		if checkin.Alias != "" {
			alias = &checkin.Alias
		}

		// Lookup country info for the IP
		var country, countryCode *string
		var latitude, longitude *float64
		if s.geoSvc != nil {
			geoInfo, err := s.geoSvc.Lookup(ctx, checkin.IP)
			if err != nil {
				s.logger.Debug("failed to lookup geo info",
					"ip", checkin.IP,
					"error", err,
				)
			} else if geoInfo != nil {
				country = &geoInfo.Country
				countryCode = &geoInfo.CountryCode
				latitude = &geoInfo.Latitude
				longitude = &geoInfo.Longitude
			}
		}

		// Convert API timestamp (milliseconds) to time.Time for accurate last_seen_at
		lastSeenAt := time.UnixMilli(checkin.Timestamp)

		if err := s.nodeRepo.UpsertFromDiscovery(ctx, checkin.Operator, nodeType, alias, checkin.IP, country, countryCode, latitude, longitude, lastSeenAt); err != nil {
			s.logger.Error("failed to upsert node",
				"operator", checkin.Operator,
				"error", err,
			)
			failed++
			continue
		}
		upserted++
	}

	s.logger.Info("discovery poll completed",
		"total", len(checkins),
		"unique", len(operatorMap),
		"upserted", upserted,
		"failed", failed,
		"skippedInvalidIP", skipped,
	)
}

// fetches checkins from the API
func (s *Service) fetchCheckins(ctx context.Context) ([]*CheckinResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.Endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp CheckinsAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDiscoveryResponseSize)).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return apiResp.Checkins, nil
}

// isValidPublicIP returns true if the IP is a valid
func isValidPublicIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() ||
		parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
		return false
	}
	return true
}

// determines the node type
func (s *Service) classifyNodeType(typeStr string) domain.NodeType {
	typeStr = strings.ToLower(typeStr)
	if strings.Contains(typeStr, "bob") {
		return domain.NodeTypeBob
	}
	return domain.NodeTypeLite
}

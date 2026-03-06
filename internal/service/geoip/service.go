package geoip

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// maxGeoIPResponseSize (1 MB)
const maxGeoIPResponseSize = 1 << 20

// represents geographic information for an IP
type GeoInfo struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
}

// represents the response from ip-api.com
type ipAPIResponse struct {
	Status      string  `json:"status"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Message     string  `json:"message,omitempty"`
}

// handles GeoIP lookups with caching
type Service struct {
	client *http.Client
	logger *slog.Logger
	cache  map[string]*GeoInfo
	mu     sync.RWMutex
}

// creates a new GeoIP service
func NewService(logger *slog.Logger) *Service {
	return &Service{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
		cache:  make(map[string]*GeoInfo),
	}
}

// Lookup returns geographic info for an IP address
func (s *Service) Lookup(ctx context.Context, ip string) (*GeoInfo, error) {
	// Check cache first
	s.mu.RLock()
	if info, ok := s.cache[ip]; ok {
		s.mu.RUnlock()
		return info, nil
	}
	s.mu.RUnlock()

	// Fetch from API
	info, err := s.fetchFromAPI(ctx, ip)
	if err != nil {
		return nil, err
	}

	// Cache the result
	s.mu.Lock()
	s.cache[ip] = info
	s.mu.Unlock()

	return info, nil
}

// fetchFromAPI fetches geo info from ip-api.com
func (s *Service) fetchFromAPI(ctx context.Context, ip string) (*GeoInfo, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,lat,lon", ip)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	var apiResp ipAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGeoIPResponseSize)).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	return &GeoInfo{
		Country:     apiResp.Country,
		CountryCode: apiResp.CountryCode,
		Latitude:    apiResp.Lat,
		Longitude:   apiResp.Lon,
	}, nil
}

// LookupBatch looks up multiple IPs with rate limiting
func (s *Service) LookupBatch(ctx context.Context, ips []string) map[string]*GeoInfo {
	results := make(map[string]*GeoInfo)

	// ip-api.com allows 45 requests per minute for free tier
	ticker := time.NewTicker(1500 * time.Millisecond) // ~40 per minute
	defer ticker.Stop()

	for i, ip := range ips {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		// Check if already cached
		s.mu.RLock()
		if info, ok := s.cache[ip]; ok {
			s.mu.RUnlock()
			results[ip] = info
			continue
		}
		s.mu.RUnlock()

		// Wait for rate limit (skip for first request)
		if i > 0 {
			<-ticker.C
		}

		info, err := s.Lookup(ctx, ip)
		if err != nil {
			s.logger.Warn("failed to lookup IP",
				"ip", ip,
				"error", err,
			)
			continue
		}
		results[ip] = info
	}

	return results
}

// CacheSize returns the current cache size
func (s *Service) CacheSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

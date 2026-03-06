package domain

import (
	"time"
)

type EpochResult struct {
	Operator         string    `json:"operator"`
	Epoch            int16     `json:"epoch"`
	Type             NodeType  `json:"type"`
	Alias            *string   `json:"alias,omitempty"`
	TotalChecks      int       `json:"totalChecks"`
	SuccessfulChecks int       `json:"successfulChecks"`
	UptimeScore      float64   `json:"uptimeScore"`
	SyncScore        float64   `json:"syncScore"`
	FinalScore       float64   `json:"finalScore"`
	Eligible         bool      `json:"eligible"`
	DisqualifyReason *string   `json:"disqualifyReason,omitempty"`
	RewardPoints     float64   `json:"rewardPoints"`
	RewardAmount     *int64    `json:"rewardAmount,omitempty"`
	CalculatedAt     time.Time `json:"calculatedAt"`
}

type EpochStats struct {
	Epoch           int16   `json:"epoch"`
	TotalNodes      int     `json:"totalNodes"`
	EligibleNodes   int     `json:"eligibleNodes"`
	TotalLiteNodes  int     `json:"totalLiteNodes"`
	TotalBobNodes   int     `json:"totalBobNodes"`
	AvgUptimeScore  float64 `json:"avgUptimeScore"`
	AvgSyncScore    float64 `json:"avgSyncScore"`
	TotalRewardPool int64   `json:"totalRewardPool"`
}

type EligibilityConfig struct {
	UptimeEnabled    bool
	MinUptimePercent float64 // Default 70%
	SyncEnabled      bool
	MinSyncPercent   float64 // Default 40%
	MinChecksEnabled bool
	MinChecks        int // Default 720 (~12 hours)
}

func DefaultEligibilityConfig() EligibilityConfig {
	return EligibilityConfig{
		UptimeEnabled:    true,
		MinUptimePercent: 70.0,
		SyncEnabled:      true,
		MinSyncPercent:   40.0,
		MinChecksEnabled: true,
		MinChecks:        720,
	}
}

// CheckEligibility determines if a node is eligible for rewards
// Only checks thresholds that are enabled
func CheckEligibility(uptimeScore, syncScore float64, totalChecks int, cfg EligibilityConfig) (bool, string) {
	if cfg.MinChecksEnabled && totalChecks < cfg.MinChecks {
		return false, "insufficient_checks"
	}
	if cfg.UptimeEnabled && uptimeScore < cfg.MinUptimePercent {
		return false, "low_uptime"
	}
	if cfg.SyncEnabled && syncScore < cfg.MinSyncPercent {
		return false, "low_sync"
	}
	return true, ""
}

// CalculateRewardPoints calculates reward points for a node
func CalculateRewardPoints(finalScore float64, successfulChecks int) float64 {
	return finalScore * float64(successfulChecks)
}

package domain

import (
	"time"
)

type NodeType string

const (
	NodeTypeLite NodeType = "lite"
	NodeTypeBob  NodeType = "bob"
)

// Node represents a monitored Qubic node
type Node struct {
	Operator          string     `json:"operator"`
	Type              NodeType   `json:"type"`
	Alias             *string    `json:"alias,omitempty"`
	CurrentIP         string     `json:"-"`
	Country           *string    `json:"country,omitempty"`
	CountryCode       *string    `json:"countryCode,omitempty"`
	Latitude          *float64   `json:"latitude,omitempty"`
	Longitude         *float64   `json:"longitude,omitempty"`
	FirstSeenAt       time.Time  `json:"firstSeenAt"`
	LastSeenAt        time.Time  `json:"lastSeenAt"`
	Flagged           bool       `json:"flagged"`
	FlaggedReason     *string    `json:"flaggedReason,omitempty"`
	TotalChecks       int        `json:"totalChecks"`
	SuccessfulChecks  int        `json:"successfulChecks"`
	SyncScoreSum      float64    `json:"syncScoreSum"`
	LastCheckAt       *time.Time `json:"lastCheckAt,omitempty"`
	LastSuccess       *bool      `json:"lastSuccess,omitempty"`
	LastFailureReason *string    `json:"lastFailureReason,omitempty"`
	LastTick          *uint32    `json:"lastTick,omitempty"`
	LastReferenceTick *uint32    `json:"lastReferenceTick,omitempty"`
}

// LiveScore represents calculated scores for a node
type LiveScore struct {
	UptimeScore     float64 `json:"uptimeScore"`
	SyncScore       float64 `json:"syncScore"`
	FinalScore      float64 `json:"finalScore"`
	RewardPoints    float64 `json:"rewardPoints"`
	EstimatedReward int64   `json:"estimatedReward,omitempty"`
}

// NodeWithScore combines a node with its live score
type NodeWithScore struct {
	Node
	LiveScore LiveScore `json:"liveScore"`
}

// CalculateLiveScore calculates the current score for a node
func (n *Node) CalculateLiveScore() LiveScore {
	return n.CalculateLiveScoreWithWeights(0.6, 0.4)
}

// CalculateLiveScoreWithWeights calculates live score with custom weights
func (n *Node) CalculateLiveScoreWithWeights(uptimeWeight, syncWeight float64) LiveScore {
	var uptimeScore, syncScore, finalScore float64

	if n.TotalChecks > 0 {
		uptimeScore = (float64(n.SuccessfulChecks) / float64(n.TotalChecks)) * 100
	}

	if n.SuccessfulChecks > 0 {
		syncScore = n.SyncScoreSum / float64(n.SuccessfulChecks)
	}

	finalScore = (uptimeScore * uptimeWeight) + (syncScore * syncWeight)
	rewardPoints := finalScore * float64(n.SuccessfulChecks)

	return LiveScore{
		UptimeScore:  uptimeScore,
		SyncScore:    syncScore,
		FinalScore:   finalScore,
		RewardPoints: rewardPoints,
	}
}

// LiveRewardConfig holds configuration for live reward calculation
type LiveRewardConfig struct {
	TotalPoolAmount  int64
	LitePoolPercent  float64
	BobPoolPercent   float64
	UptimeWeight     float64
	SyncWeight       float64
	MinUptimePercent float64
	MinSyncPercent   float64
	MinChecks        int
	UptimeEnabled    bool
	SyncEnabled      bool
	MinChecksEnabled bool
}

// CalculateLiveRewards calculates estimated rewards for all nodes
func CalculateLiveRewards(nodes []*Node, cfg LiveRewardConfig) map[string]LiveScore {
	results := make(map[string]LiveScore)

	// Calculate scores and separate by type
	var liteTotalPoints, bobTotalPoints float64
	liteScores := make(map[string]LiveScore)
	bobScores := make(map[string]LiveScore)

	for _, node := range nodes {
		score := node.CalculateLiveScoreWithWeights(cfg.UptimeWeight, cfg.SyncWeight)

		// Check eligibility
		eligible := true
		if cfg.MinChecksEnabled && node.TotalChecks < cfg.MinChecks {
			eligible = false
		}
		if cfg.UptimeEnabled && score.UptimeScore < cfg.MinUptimePercent {
			eligible = false
		}
		if cfg.SyncEnabled && score.SyncScore < cfg.MinSyncPercent {
			eligible = false
		}

		if node.Type == NodeTypeLite {
			liteScores[node.Operator] = score
			if eligible {
				liteTotalPoints += score.RewardPoints
			}
		} else {
			bobScores[node.Operator] = score
			if eligible {
				bobTotalPoints += score.RewardPoints
			}
		}
	}

	// Calculate pool amounts
	litePool := int64(float64(cfg.TotalPoolAmount) * cfg.LitePoolPercent / 100)
	bobPool := int64(float64(cfg.TotalPoolAmount) * cfg.BobPoolPercent / 100)

	// Calculate estimated rewards for lite nodes
	for op, score := range liteScores {
		if liteTotalPoints > 0 && score.RewardPoints > 0 {
			node := findNode(nodes, op)
			eligible := isEligible(node, score, cfg)
			if eligible {
				share := score.RewardPoints / liteTotalPoints
				score.EstimatedReward = int64(float64(litePool) * share)
			}
		}
		results[op] = score
	}

	// Calculate estimated rewards for bob nodes
	for op, score := range bobScores {
		if bobTotalPoints > 0 && score.RewardPoints > 0 {
			node := findNode(nodes, op)
			eligible := isEligible(node, score, cfg)
			if eligible {
				share := score.RewardPoints / bobTotalPoints
				score.EstimatedReward = int64(float64(bobPool) * share)
			}
		}
		results[op] = score
	}

	return results
}

func findNode(nodes []*Node, operator string) *Node {
	for _, n := range nodes {
		if n.Operator == operator {
			return n
		}
	}
	return nil
}

func isEligible(node *Node, score LiveScore, cfg LiveRewardConfig) bool {
	if node == nil {
		return false
	}
	if cfg.MinChecksEnabled && node.TotalChecks < cfg.MinChecks {
		return false
	}
	if cfg.UptimeEnabled && score.UptimeScore < cfg.MinUptimePercent {
		return false
	}
	if cfg.SyncEnabled && score.SyncScore < cfg.MinSyncPercent {
		return false
	}
	return true
}

// CheckResult represents the result of a node health check
type CheckResult struct {
	Operator      string
	NodeType      NodeType
	Success       bool
	FailureReason string
	SyncScore     float64
	NodeTick      uint32
	ReferenceTick uint32
	Timestamp     time.Time
}

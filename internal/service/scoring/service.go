package scoring

import (
	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
)

// calculates node scores
type Service struct {
	cfg *config.ScoringConfig
}

// creates a new scoring service
func NewService(cfg *config.ScoringConfig) *Service {
	return &Service{cfg: cfg}
}

// calculates all scores for a node
func (s *Service) CalculateScores(node *domain.Node) (uptimeScore, syncScore, finalScore, rewardPoints float64) {
	// Calculate uptime score
	if node.TotalChecks > 0 {
		uptimeScore = (float64(node.SuccessfulChecks) / float64(node.TotalChecks)) * 100
	}

	// Calculate average sync score
	if node.SuccessfulChecks > 0 {
		syncScore = node.SyncScoreSum / float64(node.SuccessfulChecks)
	}

	// Get normalized weights (only enabled metrics contribute)
	uptimeWeight, syncWeight := s.cfg.GetNormalizedWeights()

	// Calculate final score using normalized weights
	if s.cfg.Uptime.Enabled {
		finalScore += uptimeScore * uptimeWeight
	}
	if s.cfg.Sync.Enabled {
		finalScore += syncScore * syncWeight
	}

	// Calculate reward points
	rewardPoints = finalScore * float64(node.SuccessfulChecks)

	return
}

// calculates sync score for a single check
func (s *Service) CalculateSyncScoreForCheck(nodeTick, referenceTick uint32) float64 {
	if nodeTick >= referenceTick {
		return 100.0
	}

	ticksBehind := int(referenceTick - nodeTick)

	// Within buffer - full score
	if ticksBehind <= s.cfg.TickBuffer {
		return 100.0
	}

	// Beyond buffer - decay score
	excess := ticksBehind - s.cfg.TickBuffer
	score := 100.0 - (float64(excess) * s.cfg.DecayRate)

	if score < 0 {
		return 0
	}

	return score
}

// calculates reward amounts for eligible nodes
func (s *Service) CalculateRewardDistribution(results []*domain.EpochResult, poolAmount int64) map[string]int64 {
	rewards := make(map[string]int64)

	// Calculate total reward points for eligible nodes
	var totalPoints float64
	for _, r := range results {
		if r.Eligible {
			totalPoints += r.RewardPoints
		}
	}

	if totalPoints == 0 {
		return rewards
	}

	// Distribute rewards proportionally
	for _, r := range results {
		if r.Eligible {
			share := r.RewardPoints / totalPoints
			rewards[r.Operator] = int64(float64(poolAmount) * share)
		}
	}

	return rewards
}

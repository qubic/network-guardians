package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qubic/network-guardians/internal/cache"
	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/domain"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/epoch"
	"github.com/qubic/network-guardians/internal/service/reference"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	nodeRepo     *postgres.NodeRepository
	epochRepo    *postgres.EpochRepository
	referenceSvc *reference.Service
	epochSvc     *epoch.Service
	epochCfg     *config.EpochConfig
	scoringCfg   *config.ScoringConfig
	cache        *cache.Cache
}

func NewHandlers(
	nodeRepo *postgres.NodeRepository,
	epochRepo *postgres.EpochRepository,
	referenceSvc *reference.Service,
	epochSvc *epoch.Service,
	epochCfg *config.EpochConfig,
	scoringCfg *config.ScoringConfig,
	cache *cache.Cache,
) *Handlers {
	return &Handlers{
		nodeRepo:     nodeRepo,
		epochRepo:    epochRepo,
		referenceSvc: referenceSvc,
		epochSvc:     epochSvc,
		epochCfg:     epochCfg,
		scoringCfg:   scoringCfg,
		cache:        cache,
	}
}

// getLiveRewardConfig builds the live reward config from epoch and scoring configs
func (h *Handlers) getLiveRewardConfig() domain.LiveRewardConfig {
	uptimeWeight, syncWeight := h.scoringCfg.GetNormalizedWeights()
	return domain.LiveRewardConfig{
		TotalPoolAmount:  h.epochCfg.TotalPoolAmount,
		LitePoolPercent:  h.epochCfg.LitePoolPercent,
		BobPoolPercent:   h.epochCfg.BobPoolPercent,
		UptimeWeight:     uptimeWeight,
		SyncWeight:       syncWeight,
		MinUptimePercent: h.epochCfg.UptimeThreshold.Value,
		MinSyncPercent:   h.epochCfg.SyncThreshold.Value,
		MinChecks:        int(h.epochCfg.MinChecksThreshold.Value),
		UptimeEnabled:    h.epochCfg.UptimeThreshold.Enabled,
		SyncEnabled:      h.epochCfg.SyncThreshold.Enabled,
		MinChecksEnabled: h.epochCfg.MinChecksThreshold.Enabled,
	}
}

// EpochProgress contains epoch timing information
type EpochProgress struct {
	StartedAt            time.Time `json:"started_at"`
	CurrentTime          time.Time `json:"current_time"`
	EndsAt               time.Time `json:"ends_at"`
	TimeRemainingSeconds int64     `json:"time_remaining_seconds"`
	ProgressPercent      float64   `json:"progress_percent"`
}

// findPreviousWednesdayNoon finds the most recent Wednesday 12:00 UTC
func findPreviousWednesdayNoon(t time.Time) time.Time {
	t = t.UTC()
	noon := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)

	daysSinceWednesday := int(t.Weekday()) - int(time.Wednesday)
	if daysSinceWednesday < 0 {
		daysSinceWednesday += 7
	}

	if daysSinceWednesday == 0 && t.Before(noon) {
		daysSinceWednesday = 7
	}

	return noon.AddDate(0, 0, -daysSinceWednesday)
}

func findNextWednesdayNoon(t time.Time) time.Time {
	t = t.UTC()
	noon := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)

	daysUntilWednesday := int(time.Wednesday) - int(t.Weekday())
	if daysUntilWednesday <= 0 {
		daysUntilWednesday += 7
	}

	if t.Weekday() == time.Wednesday && t.Before(noon) {
		return noon
	}

	return noon.AddDate(0, 0, daysUntilWednesday)
}

// calculateEpochProgress calculates current epoch progress
func calculateEpochProgress() EpochProgress {
	now := time.Now().UTC()
	startedAt := findPreviousWednesdayNoon(now)
	endsAt := findNextWednesdayNoon(now)

	totalDuration := endsAt.Sub(startedAt).Seconds()
	elapsed := now.Sub(startedAt).Seconds()
	remaining := endsAt.Sub(now).Seconds()

	var progressPct float64
	if totalDuration > 0 {
		progressPct = (elapsed / totalDuration) * 100
		if progressPct > 100 {
			progressPct = 100
		}
	}

	return EpochProgress{
		StartedAt:            startedAt,
		CurrentTime:          now,
		EndsAt:               endsAt,
		TimeRemainingSeconds: int64(remaining),
		ProgressPercent:      progressPct,
	}
}

// Response helpers
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// NodeWithEligibility extends NodeWithScore with reward eligibility info
type NodeWithEligibility struct {
	domain.NodeWithScore
	EligibleForReward bool   `json:"eligibleForReward"`
	IneligibleReason  string `json:"ineligibleReason,omitempty"`
}

// ListNodes returns all nodes with live scores and reward eligibility info
func (h *Handlers) ListNodes(w http.ResponseWriter, r *http.Request) {
	cacheKey := "nodes:all"

	// Check cache
	if cached, found := h.cache.Get(cacheKey); found {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	nodes, err := h.nodeRepo.GetAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
		return
	}

	// Get nodes eligible for rewards (deduplicated by operator AND IP)
	eligibleNodes, _ := h.nodeRepo.GetEligibleForRewards(r.Context())

	// Calculate live rewards using ONLY eligible nodes
	liveRewardCfg := h.getLiveRewardConfig()
	liveScores := domain.CalculateLiveRewards(eligibleNodes, liveRewardCfg)

	// Build a set of eligible (operator:type) pairs for quick lookup
	eligibleSet := make(map[string]bool)
	for _, n := range eligibleNodes {
		eligibleSet[n.Operator+":"+string(n.Type)] = true
	}

	// Build eligibility config for threshold checks
	eligibilityCfg := domain.EligibilityConfig{
		UptimeEnabled:    h.epochCfg.UptimeThreshold.Enabled,
		MinUptimePercent: h.epochCfg.UptimeThreshold.Value,
		SyncEnabled:      h.epochCfg.SyncThreshold.Enabled,
		MinSyncPercent:   h.epochCfg.SyncThreshold.Value,
		MinChecksEnabled: h.epochCfg.MinChecksThreshold.Enabled,
		MinChecks:        int(h.epochCfg.MinChecksThreshold.Value),
	}

	// Build result with all nodes
	result := make([]NodeWithEligibility, 0, len(nodes))
	for _, node := range nodes {
		uptimeWeight, syncWeight := h.scoringCfg.GetNormalizedWeights()

		// Get score from live calculation only if this exact (operator, type) is eligible
		nodeKey := node.Operator + ":" + string(node.Type)
		score, inLiveScores := liveScores[node.Operator]
		if !inLiveScores || !eligibleSet[nodeKey] {
			score = node.CalculateLiveScoreWithWeights(uptimeWeight, syncWeight)
			score.EstimatedReward = 0
		}

		// Determine true eligibility: must be non-flagged, non-duplicate, AND meet thresholds
		var eligible bool
		var reason string

		if node.Flagged {
			reason = "flagged"
		} else if !eligibleSet[nodeKey] {
			reason = "duplicate_operator_or_ip"
		} else {
			thresholdOk, thresholdReason := domain.CheckEligibility(
				score.UptimeScore, score.SyncScore, node.TotalChecks, eligibilityCfg,
			)
			if thresholdOk {
				eligible = true
			} else {
				reason = thresholdReason
			}
		}

		info := NodeWithEligibility{
			NodeWithScore: domain.NodeWithScore{
				Node:      *node,
				LiveScore: score,
			},
			EligibleForReward: eligible,
			IneligibleReason:  reason,
		}

		result = append(result, info)
	}

	h.cache.Set(cacheKey, result)
	writeJSON(w, http.StatusOK, result)
}

// GetNode returns a single node by operator and type
func (h *Handlers) GetNode(w http.ResponseWriter, r *http.Request) {
	operator := chi.URLParam(r, "operator")
	if operator == "" {
		writeError(w, http.StatusBadRequest, "operator required")
		return
	}

	nodeType := chi.URLParam(r, "type")
	if nodeType != "lite" && nodeType != "bob" {
		writeError(w, http.StatusBadRequest, "type must be 'lite' or 'bob'")
		return
	}

	cacheKey := "nodes:" + operator + ":" + nodeType

	if cached, found := h.cache.Get(cacheKey); found {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// Get the requested node by composite key
	node, err := h.nodeRepo.GetByOperatorAndType(r.Context(), operator, domain.NodeType(nodeType))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch node")
		return
	}

	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	// Get all nodes for this operator
	allNodeTypes, _ := h.nodeRepo.GetAllByOperator(r.Context(), operator)

	// Get eligible nodes for reward calculation
	eligibleNodes, _ := h.nodeRepo.GetEligibleForRewards(r.Context())

	liveRewardCfg := h.getLiveRewardConfig()
	liveScores := domain.CalculateLiveRewards(eligibleNodes, liveRewardCfg)

	// Check if this exact (operator, type) is in the eligible set
	isEligibleNode := false
	for _, n := range eligibleNodes {
		if n.Operator == operator && string(n.Type) == nodeType {
			isEligibleNode = true
			break
		}
	}

	uptimeWeight, syncWeight := h.scoringCfg.GetNormalizedWeights()

	// Get score from live calculation only if this exact (operator, type) is eligible
	score, inLiveScores := liveScores[operator]
	if !inLiveScores || !isEligibleNode {
		score = node.CalculateLiveScoreWithWeights(uptimeWeight, syncWeight)
		score.EstimatedReward = 0
	}

	// Determine true eligibility
	var eligible bool
	var reason string

	if node.Flagged {
		reason = "flagged"
	} else if !isEligibleNode {
		reason = "duplicate_operator_or_ip"
	} else {
		eligibilityCfg := domain.EligibilityConfig{
			UptimeEnabled:    h.epochCfg.UptimeThreshold.Enabled,
			MinUptimePercent: h.epochCfg.UptimeThreshold.Value,
			SyncEnabled:      h.epochCfg.SyncThreshold.Enabled,
			MinSyncPercent:   h.epochCfg.SyncThreshold.Value,
			MinChecksEnabled: h.epochCfg.MinChecksThreshold.Enabled,
			MinChecks:        int(h.epochCfg.MinChecksThreshold.Value),
		}
		thresholdOk, thresholdReason := domain.CheckEligibility(
			score.UptimeScore, score.SyncScore, node.TotalChecks, eligibilityCfg,
		)
		if thresholdOk {
			eligible = true
		} else {
			reason = thresholdReason
		}
	}

	result := NodeWithEligibility{
		NodeWithScore: domain.NodeWithScore{
			Node:      *node,
			LiveScore: score,
		},
		EligibleForReward: eligible,
		IneligibleReason:  reason,
	}

	// Get epoch history
	epochs, _ := h.epochRepo.GetByOperator(r.Context(), operator)

	response := map[string]interface{}{
		"node":     result,
		"allTypes": allNodeTypes,
		"history":  epochs,
	}

	h.cache.Set(cacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

// GetLeaderboard returns current epoch rankings (deduplicated by operator AND IP)
func (h *Handlers) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	// Get type filter from query params
	typeFilter := r.URL.Query().Get("type")
	var filterNodeType *domain.NodeType
	if typeFilter == "lite" {
		t := domain.NodeTypeLite
		filterNodeType = &t
	} else if typeFilter == "bob" {
		t := domain.NodeTypeBob
		filterNodeType = &t
	}

	cacheKey := "leaderboard:current"
	if filterNodeType != nil {
		cacheKey = "leaderboard:current:" + string(*filterNodeType)
	}

	if cached, found := h.cache.Get(cacheKey); found {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// Get nodes eligible for rewards (deduplicated by operator AND IP)
	allNodes, err := h.nodeRepo.GetEligibleForRewards(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
		return
	}

	// Filter by type if specified
	var nodes []*domain.Node
	if filterNodeType != nil {
		for _, node := range allNodes {
			if node.Type == *filterNodeType {
				nodes = append(nodes, node)
			}
		}
	} else {
		nodes = allNodes
	}

	// Calculate live rewards for ALL eligible nodes
	liveRewardCfg := h.getLiveRewardConfig()
	liveScores := domain.CalculateLiveRewards(allNodes, liveRewardCfg)

	// Build rankings with live scores (only for filtered nodes)
	rankings := make([]domain.NodeWithScore, 0, len(nodes))
	for _, node := range nodes {
		rankings = append(rankings, domain.NodeWithScore{
			Node:      *node,
			LiveScore: liveScores[node.Operator],
		})
	}

	// Sort by final score (descending)
	for i := 0; i < len(rankings)-1; i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[j].LiveScore.FinalScore > rankings[i].LiveScore.FinalScore {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	// Get reference data
	tick, epoch, _, updatedAt := h.referenceSvc.GetReference().Get()

	// Get counts for info
	totalNodes, _ := h.nodeRepo.Count(r.Context())
	flaggedNodes, _ := h.nodeRepo.GetFlaggedCount(r.Context())
	activeNodes := totalNodes - flaggedNodes

	// Count by type from eligible nodes
	var liteCount, bobCount int
	for _, n := range allNodes {
		if n.Type == domain.NodeTypeLite {
			liteCount++
		} else {
			bobCount++
		}
	}

	// Calculate pool amounts
	totalPool := h.epochCfg.TotalPoolAmount
	litePool := int64(float64(totalPool) * h.epochCfg.LitePoolPercent / 100)
	bobPool := int64(float64(totalPool) * h.epochCfg.BobPoolPercent / 100)

	var poolAmount int64
	var poolName string
	if filterNodeType != nil {
		if *filterNodeType == domain.NodeTypeLite {
			poolAmount = litePool
			poolName = "lite"
		} else {
			poolAmount = bobPool
			poolName = "bob"
		}
	} else {
		poolAmount = totalPool
		poolName = "all"
	}

	response := map[string]interface{}{
		"rankings": rankings,
		"reference": map[string]interface{}{
			"tick":      tick,
			"epoch":     epoch,
			"updatedAt": updatedAt,
		},
		"info": map[string]interface{}{
			"totalNodes":         totalNodes,
			"activeNodes":        activeNodes,
			"flaggedNodes":       flaggedNodes,
			"eligibleForRewards": len(allNodes),
			"duplicatesExcluded": activeNodes - len(allNodes),
			"liteCount":          liteCount,
			"bobCount":           bobCount,
			"filteredCount":      len(nodes),
		},
		"pool": map[string]interface{}{
			"type":      poolName,
			"amount":    poolAmount,
			"totalPool": totalPool,
			"litePool":  litePool,
			"bobPool":   bobPool,
		},
	}

	h.cache.Set(cacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

// GetHistoricalLeaderboard returns rankings for a past epoch
func (h *Handlers) GetHistoricalLeaderboard(w http.ResponseWriter, r *http.Request) {
	epochStr := chi.URLParam(r, "epoch")
	epoch, err := strconv.ParseInt(epochStr, 10, 16)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epoch")
		return
	}

	cacheKey := "leaderboard:" + epochStr

	if cached, found := h.cache.Get(cacheKey); found {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	results, err := h.epochRepo.GetLeaderboard(r.Context(), int16(epoch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch leaderboard")
		return
	}

	stats, _ := h.epochRepo.GetEpochStats(r.Context(), int16(epoch))

	response := map[string]interface{}{
		"epoch":    epoch,
		"rankings": results,
		"stats":    stats,
	}

	h.cache.Set(cacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

// GetStats returns network summary statistics
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	cacheKey := "stats"

	if cached, found := h.cache.Get(cacheKey); found {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// Get node counts (total includes flagged)
	totalNodes, _ := h.nodeRepo.Count(r.Context())
	flaggedNodes, _ := h.nodeRepo.GetFlaggedCount(r.Context())

	// Get active (non-flagged) node counts
	activeNodes, _ := h.nodeRepo.CountActive(r.Context())
	activeLiteNodes, _ := h.nodeRepo.CountActiveByType(r.Context(), domain.NodeTypeLite)
	activeBobNodes, _ := h.nodeRepo.CountActiveByType(r.Context(), domain.NodeTypeBob)

	// Get reference data
	tick, epoch, _, updatedAt := h.referenceSvc.GetReference().Get()

	// Get latest epoch info
	latestEpoch, _ := h.epochRepo.GetLatestEpoch(r.Context())

	// Calculate pool amounts
	totalPool := h.epochCfg.TotalPoolAmount
	litePool := int64(float64(totalPool) * h.epochCfg.LitePoolPercent / 100)
	bobPool := int64(float64(totalPool) * h.epochCfg.BobPoolPercent / 100)

	// Calculate epoch progress
	epochProgress := calculateEpochProgress()

	// Get epoch phase info
	var epochPhaseInfo map[string]interface{}
	if h.epochSvc != nil {
		epochPhaseInfo = h.epochSvc.GetPhaseInfo()
	} else {
		epochPhaseInfo = map[string]interface{}{"phase": "active"}
	}

	response := map[string]interface{}{
		"totalNodes":   totalNodes,
		"activeNodes":  activeNodes,
		"flaggedNodes": flaggedNodes,
		"liteNodes":    activeLiteNodes,
		"bobNodes":     activeBobNodes,
		"reference": map[string]interface{}{
			"tick":      tick,
			"epoch":     epoch,
			"updatedAt": updatedAt,
		},
		"latestCompletedEpoch": latestEpoch,
		"epochRewards": map[string]interface{}{
			"totalPool": totalPool,
			"litePool":  litePool,
			"bobPool":   bobPool,
		},
		"epochProgress": map[string]interface{}{
			"started_at":             epochProgress.StartedAt,
			"current_time":           epochProgress.CurrentTime,
			"ends_at":                epochProgress.EndsAt,
			"time_remaining_seconds": epochProgress.TimeRemainingSeconds,
			"progress_percent":       epochProgress.ProgressPercent,
		},
		"epochPhase": epochPhaseInfo,
	}

	h.cache.Set(cacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	tick, epoch, _, updatedAt := h.referenceSvc.GetReference().Get()

	healthy := tick > 0 && !h.referenceSvc.GetReference().IsStale(120*time.Second)

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]interface{}{
		"healthy":     healthy,
		"tick":        tick,
		"epoch":       epoch,
		"lastUpdated": updatedAt,
	})
}

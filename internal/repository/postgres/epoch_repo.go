package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qubic/network-guardians/internal/domain"
)

// EpochRepository handles epoch result persistence
type EpochRepository struct {
	pool *pgxpool.Pool
}

// NewEpochRepository creates a new epoch repository
func NewEpochRepository(pool *pgxpool.Pool) *EpochRepository {
	return &EpochRepository{pool: pool}
}

// Insert inserts a new epoch result
func (r *EpochRepository) Insert(ctx context.Context, result *domain.EpochResult) error {
	query := `
		INSERT INTO epoch_results (
			operator, epoch, type, alias, total_checks, successful_checks,
			uptime_score, sync_score, final_score, eligible, disqualify_reason,
			reward_points, reward_amount, calculated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.pool.Exec(ctx, query,
		result.Operator,
		result.Epoch,
		result.Type,
		result.Alias,
		result.TotalChecks,
		result.SuccessfulChecks,
		result.UptimeScore,
		result.SyncScore,
		result.FinalScore,
		result.Eligible,
		result.DisqualifyReason,
		result.RewardPoints,
		result.RewardAmount,
		result.CalculatedAt,
	)

	return err
}

// InsertBatch inserts multiple epoch results in a single database transaction
// uses ON CONFLICT to make retries idempotent if a partial write occurred previously
// re-running will update the existing rows instead of failing on duplicate keys
func (r *EpochRepository) InsertBatch(ctx context.Context, results []*domain.EpochResult) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}

	query := `
		INSERT INTO epoch_results (
			operator, epoch, type, alias, total_checks, successful_checks,
			uptime_score, sync_score, final_score, eligible, disqualify_reason,
			reward_points, reward_amount, calculated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (epoch, operator) DO UPDATE SET
			type = EXCLUDED.type,
			alias = EXCLUDED.alias,
			total_checks = EXCLUDED.total_checks,
			successful_checks = EXCLUDED.successful_checks,
			uptime_score = EXCLUDED.uptime_score,
			sync_score = EXCLUDED.sync_score,
			final_score = EXCLUDED.final_score,
			eligible = EXCLUDED.eligible,
			disqualify_reason = EXCLUDED.disqualify_reason,
			reward_points = EXCLUDED.reward_points,
			reward_amount = EXCLUDED.reward_amount,
			calculated_at = EXCLUDED.calculated_at
	`

	for _, result := range results {
		batch.Queue(query,
			result.Operator,
			result.Epoch,
			result.Type,
			result.Alias,
			result.TotalChecks,
			result.SuccessfulChecks,
			result.UptimeScore,
			result.SyncScore,
			result.FinalScore,
			result.Eligible,
			result.DisqualifyReason,
			result.RewardPoints,
			result.RewardAmount,
			result.CalculatedAt,
		)
	}

	br := tx.SendBatch(ctx, batch)
	for range results {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("failed to insert epoch result: %w", err)
		}
	}
	br.Close()

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit epoch results: %w", err)
	}

	return nil
}

// InsertBatchWithRewards inserts epoch results and updates reward amounts atomically
func (r *EpochRepository) InsertBatchWithRewards(ctx context.Context, results []*domain.EpochResult, epoch int16, rewards map[string]int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Batch insert epoch results
	insertBatch := &pgx.Batch{}

	insertQuery := `
		INSERT INTO epoch_results (
			operator, epoch, type, alias, total_checks, successful_checks,
			uptime_score, sync_score, final_score, eligible, disqualify_reason,
			reward_points, reward_amount, calculated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (epoch, operator) DO UPDATE SET
			type = EXCLUDED.type,
			alias = EXCLUDED.alias,
			total_checks = EXCLUDED.total_checks,
			successful_checks = EXCLUDED.successful_checks,
			uptime_score = EXCLUDED.uptime_score,
			sync_score = EXCLUDED.sync_score,
			final_score = EXCLUDED.final_score,
			eligible = EXCLUDED.eligible,
			disqualify_reason = EXCLUDED.disqualify_reason,
			reward_points = EXCLUDED.reward_points,
			reward_amount = EXCLUDED.reward_amount,
			calculated_at = EXCLUDED.calculated_at
	`

	for _, result := range results {
		insertBatch.Queue(insertQuery,
			result.Operator,
			result.Epoch,
			result.Type,
			result.Alias,
			result.TotalChecks,
			result.SuccessfulChecks,
			result.UptimeScore,
			result.SyncScore,
			result.FinalScore,
			result.Eligible,
			result.DisqualifyReason,
			result.RewardPoints,
			result.RewardAmount,
			result.CalculatedAt,
		)
	}

	br := tx.SendBatch(ctx, insertBatch)
	for range results {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("failed to insert epoch result: %w", err)
		}
	}
	br.Close()

	// 2. Batch update reward amounts
	if len(rewards) > 0 {
		rewardBatch := &pgx.Batch{}
		rewardQuery := `UPDATE epoch_results SET reward_amount = $3 WHERE epoch = $1 AND operator = $2`

		for operator, amount := range rewards {
			rewardBatch.Queue(rewardQuery, epoch, operator, amount)
		}

		br2 := tx.SendBatch(ctx, rewardBatch)
		for range rewards {
			if _, err := br2.Exec(); err != nil {
				br2.Close()
				return fmt.Errorf("failed to update reward amount: %w", err)
			}
		}
		br2.Close()
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit epoch results with rewards: %w", err)
	}

	return nil
}

// GetByEpoch retrieves all results for an epoch
func (r *EpochRepository) GetByEpoch(ctx context.Context, epoch int16) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE epoch = $1
		ORDER BY final_score DESC
	`

	rows, err := r.pool.Query(ctx, query, epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetByOperator retrieves all epoch results for an operator
func (r *EpochRepository) GetByOperator(ctx context.Context, operator string) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE operator = $1
		ORDER BY epoch DESC
	`

	rows, err := r.pool.Query(ctx, query, operator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetByEpochAndOperator retrieves a specific epoch result
func (r *EpochRepository) GetByEpochAndOperator(ctx context.Context, epoch int16, operator string) (*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE epoch = $1 AND operator = $2
	`

	var result domain.EpochResult
	err := r.pool.QueryRow(ctx, query, epoch, operator).Scan(
		&result.Operator,
		&result.Epoch,
		&result.Type,
		&result.Alias,
		&result.TotalChecks,
		&result.SuccessfulChecks,
		&result.UptimeScore,
		&result.SyncScore,
		&result.FinalScore,
		&result.Eligible,
		&result.DisqualifyReason,
		&result.RewardPoints,
		&result.RewardAmount,
		&result.CalculatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetLeaderboard retrieves all eligible results for an epoch
func (r *EpochRepository) GetLeaderboard(ctx context.Context, epoch int16) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE epoch = $1 AND eligible = TRUE
		ORDER BY final_score DESC
	`

	rows, err := r.pool.Query(ctx, query, epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetLatestEpoch returns the most recent epoch number
func (r *EpochRepository) GetLatestEpoch(ctx context.Context) (int16, error) {
	var epoch int16
	err := r.pool.QueryRow(ctx, "SELECT COALESCE(MAX(epoch), 0) FROM epoch_results").Scan(&epoch)
	return epoch, err
}

// GetEpochStats calculates aggregate statistics for an epoch
func (r *EpochRepository) GetEpochStats(ctx context.Context, epoch int16) (*domain.EpochStats, error) {
	query := `
		SELECT
			$1::smallint as epoch,
			COUNT(*) as total_nodes,
			COUNT(*) FILTER (WHERE eligible = TRUE) as eligible_nodes,
			COUNT(*) FILTER (WHERE type = 'lite') as total_lite_nodes,
			COUNT(*) FILTER (WHERE type = 'bob') as total_bob_nodes,
			COALESCE(AVG(uptime_score), 0) as avg_uptime_score,
			COALESCE(AVG(sync_score), 0) as avg_sync_score,
			COALESCE(SUM(reward_amount), 0) as total_reward_pool
		FROM epoch_results
		WHERE epoch = $1
	`

	var stats domain.EpochStats
	err := r.pool.QueryRow(ctx, query, epoch).Scan(
		&stats.Epoch,
		&stats.TotalNodes,
		&stats.EligibleNodes,
		&stats.TotalLiteNodes,
		&stats.TotalBobNodes,
		&stats.AvgUptimeScore,
		&stats.AvgSyncScore,
		&stats.TotalRewardPool,
	)

	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// UpdateRewardAmounts updates reward amounts for eligible nodes in an epoch wrapped atomically
func (r *EpochRepository) UpdateRewardAmounts(ctx context.Context, epoch int16, rewards map[string]int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}

	query := `UPDATE epoch_results SET reward_amount = $3 WHERE epoch = $1 AND operator = $2`

	for operator, amount := range rewards {
		batch.Queue(query, epoch, operator, amount)
	}

	br := tx.SendBatch(ctx, batch)
	for range rewards {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("failed to update reward amount: %w", err)
		}
	}
	br.Close()

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit reward amounts: %w", err)
	}

	return nil
}

// GetEligibleByType retrieves eligible results by type for an epoch
func (r *EpochRepository) GetEligibleByType(ctx context.Context, epoch int16, nodeType domain.NodeType) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE epoch = $1 AND type = $2 AND eligible = TRUE
		ORDER BY final_score DESC
	`

	rows, err := r.pool.Query(ctx, query, epoch, nodeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetAllResults retrieves all epoch results
func (r *EpochRepository) GetAllResults(ctx context.Context) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		ORDER BY epoch DESC, final_score DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetDistinctEpochs returns a list of all distinct epochs with counts
func (r *EpochRepository) GetDistinctEpochs(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT epoch,
			   COUNT(*) as total_nodes,
			   COUNT(*) FILTER (WHERE eligible = TRUE) as eligible_nodes,
			   COALESCE(SUM(reward_amount), 0) as total_rewards,
			   MIN(calculated_at) as calculated_at
		FROM epoch_results
		GROUP BY epoch
		ORDER BY epoch DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var epochs []map[string]interface{}
	for rows.Next() {
		var epoch int16
		var totalNodes, eligibleNodes int
		var totalRewards int64
		var calculatedAt interface{}

		err := rows.Scan(&epoch, &totalNodes, &eligibleNodes, &totalRewards, &calculatedAt)
		if err != nil {
			return nil, err
		}

		epochs = append(epochs, map[string]interface{}{
			"epoch":         epoch,
			"totalNodes":    totalNodes,
			"eligibleNodes": eligibleNodes,
			"totalRewards":  totalRewards,
			"calculatedAt":  calculatedAt,
		})
	}

	return epochs, rows.Err()
}


// CountResults returns the total number of epoch results
func (r *EpochRepository) CountResults(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM epoch_results").Scan(&count)
	return count, err
}

// GetByEpochAndType retrieves results for an epoch filtered by node type
func (r *EpochRepository) GetByEpochAndType(ctx context.Context, epoch int16, nodeType domain.NodeType) ([]*domain.EpochResult, error) {
	query := `
		SELECT operator, epoch, type, alias, total_checks, successful_checks,
			   uptime_score, sync_score, final_score, eligible, disqualify_reason,
			   reward_points, reward_amount, calculated_at
		FROM epoch_results
		WHERE epoch = $1 AND type = $2
		ORDER BY final_score DESC
	`

	rows, err := r.pool.Query(ctx, query, epoch, nodeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// scanResults is a helper to scan rows into epoch results
func (r *EpochRepository) scanResults(rows pgx.Rows) ([]*domain.EpochResult, error) {
	var results []*domain.EpochResult

	for rows.Next() {
		var result domain.EpochResult
		err := rows.Scan(
			&result.Operator,
			&result.Epoch,
			&result.Type,
			&result.Alias,
			&result.TotalChecks,
			&result.SuccessfulChecks,
			&result.UptimeScore,
			&result.SyncScore,
			&result.FinalScore,
			&result.Eligible,
			&result.DisqualifyReason,
			&result.RewardPoints,
			&result.RewardAmount,
			&result.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}

		results = append(results, &result)
	}

	return results, rows.Err()
}

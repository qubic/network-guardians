package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qubic/network-guardians/internal/domain"
)

// NodeRepository handles node persistence
type NodeRepository struct {
	pool *pgxpool.Pool
}

// NewNodeRepository creates a new node repository
func NewNodeRepository(pool *pgxpool.Pool) *NodeRepository {
	return &NodeRepository{pool: pool}
}

// Upsert inserts or updates a node by operator and type (composite key)
func (r *NodeRepository) Upsert(ctx context.Context, node *domain.Node) error {
	query := `
		INSERT INTO nodes (
			operator, type, alias, current_ip, country, country_code,
			latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			total_checks, successful_checks, sync_score_sum,
			last_check_at, last_success, last_failure_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (operator, type) DO UPDATE SET
			alias = EXCLUDED.alias,
			current_ip = EXCLUDED.current_ip,
			country = EXCLUDED.country,
			country_code = EXCLUDED.country_code,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			last_seen_at = EXCLUDED.last_seen_at
	`

	_, err := r.pool.Exec(ctx, query,
		node.Operator,
		node.Type,
		node.Alias,
		node.CurrentIP,
		node.Country,
		node.CountryCode,
		node.Latitude,
		node.Longitude,
		node.FirstSeenAt,
		node.LastSeenAt,
		node.Flagged,
		node.FlaggedReason,
		node.TotalChecks,
		node.SuccessfulChecks,
		node.SyncScoreSum,
		node.LastCheckAt,
		node.LastSuccess,
		node.LastFailureReason,
	)

	return err
}

// UpsertFromDiscovery updates a node from discovery data (only updates IP and last_seen)
func (r *NodeRepository) UpsertFromDiscovery(ctx context.Context, operator string, nodeType domain.NodeType, alias *string, ip string, country *string, countryCode *string, latitude *float64, longitude *float64, lastSeenAt time.Time) error {
	query := `
		INSERT INTO nodes (operator, type, alias, current_ip, country, country_code, latitude, longitude, first_seen_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		ON CONFLICT (operator, type) DO UPDATE SET
			alias = COALESCE(EXCLUDED.alias, nodes.alias),
			current_ip = EXCLUDED.current_ip,
			country = COALESCE(EXCLUDED.country, nodes.country),
			country_code = COALESCE(EXCLUDED.country_code, nodes.country_code),
			latitude = COALESCE(EXCLUDED.latitude, nodes.latitude),
			longitude = COALESCE(EXCLUDED.longitude, nodes.longitude),
			last_seen_at = EXCLUDED.last_seen_at
	`

	_, err := r.pool.Exec(ctx, query, operator, nodeType, alias, ip, country, countryCode, latitude, longitude, lastSeenAt)
	return err
}

// GetByOperator retrieves a node by operator
func (r *NodeRepository) GetByOperator(ctx context.Context, operator string) (*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE operator = $1
		ORDER BY flagged ASC, last_seen_at DESC
		LIMIT 1
	`

	var node domain.Node
	var ip string

	err := r.pool.QueryRow(ctx, query, operator).Scan(
		&node.Operator,
		&node.Type,
		&node.Alias,
		&ip,
		&node.Country,
		&node.CountryCode,
		&node.Latitude,
		&node.Longitude,
		&node.FirstSeenAt,
		&node.LastSeenAt,
		&node.Flagged,
		&node.FlaggedReason,
		&node.TotalChecks,
		&node.SuccessfulChecks,
		&node.SyncScoreSum,
		&node.LastCheckAt,
		&node.LastSuccess,
		&node.LastFailureReason,
		&node.LastTick,
		&node.LastReferenceTick,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	node.CurrentIP = ip
	return &node, nil
}

// GetByOperatorAndType retrieves a node by operator and type (composite key)
func (r *NodeRepository) GetByOperatorAndType(ctx context.Context, operator string, nodeType domain.NodeType) (*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE operator = $1 AND type = $2
	`

	var node domain.Node
	var ip string

	err := r.pool.QueryRow(ctx, query, operator, nodeType).Scan(
		&node.Operator,
		&node.Type,
		&node.Alias,
		&ip,
		&node.Country,
		&node.CountryCode,
		&node.Latitude,
		&node.Longitude,
		&node.FirstSeenAt,
		&node.LastSeenAt,
		&node.Flagged,
		&node.FlaggedReason,
		&node.TotalChecks,
		&node.SuccessfulChecks,
		&node.SyncScoreSum,
		&node.LastCheckAt,
		&node.LastSuccess,
		&node.LastFailureReason,
		&node.LastTick,
		&node.LastReferenceTick,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	node.CurrentIP = ip
	return &node, nil
}

// GetAllByOperator retrieves all nodes for an operator (both types if they exist)
func (r *NodeRepository) GetAllByOperator(ctx context.Context, operator string) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE operator = $1
		ORDER BY last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query, operator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// GetAll retrieves all nodes
func (r *NodeRepository) GetAll(ctx context.Context) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		ORDER BY last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// GetActiveNodes retrieves all non-flagged nodes for checking
// all nodes are checked regardless of last_seen_at so that offline nodes
// accumulate failed checks to reflects actual availability
func (r *NodeRepository) GetActiveNodes(ctx context.Context) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE flagged = FALSE
		ORDER BY last_check_at ASC NULLS FIRST
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// UpdateCheckResult atomically updates node counters after a check
func (r *NodeRepository) UpdateCheckResult(ctx context.Context, result *domain.CheckResult) error {
	var query string
	var args []interface{}

	if result.Success {
		query = `
			UPDATE nodes SET
				total_checks = total_checks + 1,
				successful_checks = successful_checks + 1,
				sync_score_sum = sync_score_sum + $3,
				last_check_at = $4,
				last_success = TRUE,
				last_failure_reason = NULL,
				last_tick = $5,
				last_reference_tick = $6
			WHERE operator = $1 AND type = $2
		`
		args = []interface{}{result.Operator, result.NodeType, result.SyncScore, result.Timestamp, result.NodeTick, result.ReferenceTick}
	} else {
		query = `
			UPDATE nodes SET
				total_checks = total_checks + 1,
				last_check_at = $3,
				last_success = FALSE,
				last_failure_reason = $4
			WHERE operator = $1 AND type = $2
		`
		args = []interface{}{result.Operator, result.NodeType, result.Timestamp, result.FailureReason}
	}

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// ResetCounters resets all node counters (called at epoch transition)
// Also releases all claims and resets next_check_after to clean state for new epoch
func (r *NodeRepository) ResetCounters(ctx context.Context) error {
	query := `
		UPDATE nodes SET
			total_checks = 0,
			successful_checks = 0,
			sync_score_sum = 0,
			last_check_at = NULL,
			last_success = NULL,
			last_failure_reason = NULL,
			claimed_by = NULL,
			claimed_at = NULL,
			claim_expires_at = NULL,
			next_check_after = NULL,
			manual_override = FALSE
	`

	_, err := r.pool.Exec(ctx, query)
	return err
}

// GetNodesByType retrieves all nodes of a specific type
func (r *NodeRepository) GetNodesByType(ctx context.Context, nodeType domain.NodeType) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE type = $1
		ORDER BY last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query, nodeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// Count returns the total number of nodes
func (r *NodeRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes").Scan(&count)
	return count, err
}

// CountByType returns the count of nodes by type
func (r *NodeRepository) CountByType(ctx context.Context, nodeType domain.NodeType) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes WHERE type = $1", nodeType).Scan(&count)
	return count, err
}

func (r *NodeRepository) DeleteAll(ctx context.Context) (int64, error) {
	result, err := r.pool.Exec(ctx, "DELETE FROM nodes")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// GetOnlineCount returns count of nodes that passed their last check
func (r *NodeRepository) GetOnlineCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes WHERE last_success = TRUE").Scan(&count)
	return count, err
}

// GetFlaggedCount returns count of flagged nodes
func (r *NodeRepository) GetFlaggedCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes WHERE flagged = TRUE").Scan(&count)
	return count, err
}

// GetUniqueByIP returns non-flagged nodes deduplicated by IP (keeps the newest node per IP)
func (r *NodeRepository) GetUniqueByIP(ctx context.Context) ([]*domain.Node, error) {
	query := `
		SELECT DISTINCT ON (current_ip)
			operator, type, alias, host(current_ip), country, country_code,
			latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			total_checks, successful_checks, sync_score_sum,
			last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE flagged = FALSE
		ORDER BY current_ip, last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// GetEligibleForRewards returns non-flagged nodes deduplicated by BOTH operator AND IP
// ALWAYS one reward per operator, one reward per IP (newest registration wins)
func (r *NodeRepository) GetEligibleForRewards(ctx context.Context) ([]*domain.Node, error) {
	query := `
		WITH unique_by_operator AS (
			-- First: one node per operator (newest registration wins)
			SELECT DISTINCT ON (operator)
				operator, type, alias, current_ip, country, country_code,
				latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
				total_checks, successful_checks, sync_score_sum,
				last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
			FROM nodes
			WHERE flagged = FALSE
			ORDER BY operator, last_seen_at DESC
		)
		-- Then: one node per IP (newest registration wins)
		SELECT DISTINCT ON (current_ip)
			operator, type, alias, host(current_ip), country, country_code,
			latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			total_checks, successful_checks, sync_score_sum,
			last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM unique_by_operator
		ORDER BY current_ip, last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// GetDuplicateIPs returns a map of IPs that have multiple operators
func (r *NodeRepository) GetDuplicateIPs(ctx context.Context) (map[string][]string, error) {
	query := `
		SELECT host(current_ip), array_agg(operator ORDER BY last_seen_at DESC)
		FROM nodes
		GROUP BY current_ip
		HAVING COUNT(*) > 1
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	duplicates := make(map[string][]string)
	for rows.Next() {
		var ip string
		var operators []string
		if err := rows.Scan(&ip, &operators); err != nil {
			return nil, err
		}
		duplicates[ip] = operators
	}

	return duplicates, rows.Err()
}

// CountDuplicateIPs returns count of IPs with multiple operators
func (r *NodeRepository) CountDuplicateIPs(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT current_ip FROM nodes GROUP BY current_ip HAVING COUNT(*) > 1
		) t
	`).Scan(&count)
	return count, err
}

// CountUniqueByIP returns count of unique IPs
func (r *NodeRepository) CountUniqueByIP(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT current_ip) FROM nodes`).Scan(&count)
	return count, err
}

// UpdateCountry updates the country info for a node
func (r *NodeRepository) UpdateCountry(ctx context.Context, operator string, country string, countryCode string) error {
	query := `UPDATE nodes SET country = $2, country_code = $3 WHERE operator = $1`
	_, err := r.pool.Exec(ctx, query, operator, country, countryCode)
	return err
}

// GetAllActive returns all non-flagged nodes
func (r *NodeRepository) GetAllActive(ctx context.Context) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE flagged = FALSE
		ORDER BY last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// CountActive returns the count of non-flagged nodes
func (r *NodeRepository) CountActive(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes WHERE flagged = FALSE").Scan(&count)
	return count, err
}

// CountActiveByType returns the count of non-flagged nodes by type
func (r *NodeRepository) CountActiveByType(ctx context.Context, nodeType domain.NodeType) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nodes WHERE type = $1 AND flagged = FALSE", nodeType).Scan(&count)
	return count, err
}

// FlagDuplicateIPs flags all duplicate IP nodes except the primary (most recently seen)
func (r *NodeRepository) FlagDuplicateIPs(ctx context.Context) (int64, error) {
	// node with the latest last_seen_at per IP (most recent checkin wins)
	query := `
		WITH primary_nodes AS (
			SELECT DISTINCT ON (current_ip) operator, type
			FROM nodes
			ORDER BY current_ip, last_seen_at DESC, type ASC
		),
		duplicates AS (
			SELECT n.operator, n.type
			FROM nodes n
			INNER JOIN (
				SELECT current_ip
				FROM nodes
				GROUP BY current_ip
				HAVING COUNT(*) > 1
			) dup ON n.current_ip = dup.current_ip
			WHERE (n.operator, n.type) NOT IN (SELECT operator, type FROM primary_nodes)
			AND n.flagged = FALSE
			AND n.manual_override = FALSE
		)
		UPDATE nodes
		SET flagged = TRUE, flagged_reason = 'duplicate_ip'
		WHERE (operator, type) IN (SELECT operator, type FROM duplicates)
	`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// ClearDuplicateIPFlags clears the duplicate_ip flag from nodes no longer duplicates or have become the primary node (most recently seen) for their IP
func (r *NodeRepository) ClearDuplicateIPFlags(ctx context.Context) (int64, error) {
	// Clear flag from nodes that:
	// 1. were flagged as duplicate_ip but are now the only node on their IP
	// 2. are now the primary (most recently seen) for their IP
	query := `
		WITH primary_nodes AS (
			SELECT DISTINCT ON (current_ip) operator, type
			FROM nodes
			ORDER BY current_ip, last_seen_at DESC, type ASC
		),
		should_unflag AS (
			SELECT n.operator, n.type
			FROM nodes n
			WHERE n.flagged = TRUE
			AND n.flagged_reason = 'duplicate_ip'
			AND n.manual_override = FALSE
			AND (
				-- Node is now alone on its IP
				(SELECT COUNT(*) FROM nodes n2 WHERE n2.current_ip = n.current_ip) = 1
				OR
				-- Node is now the primary for its IP
				(n.operator, n.type) IN (SELECT operator, type FROM primary_nodes)
			)
		)
		UPDATE nodes
		SET flagged = FALSE, flagged_reason = NULL
		WHERE (operator, type) IN (SELECT operator, type FROM should_unflag)
	`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// FlagDuplicateOperators flags all duplicate operator nodes except the primary (most recently seen)
// only run ONE node (either lite or bob)
func (r *NodeRepository) FlagDuplicateOperators(ctx context.Context) (int64, error) {
	// This query flags all nodes where an operator has multiple nodes,
	// except for the node with the latest last_seen_at (most recent checkin wins)
	query := `
		WITH primary_nodes AS (
			SELECT DISTINCT ON (operator) operator, type
			FROM nodes
			ORDER BY operator, last_seen_at DESC, type ASC
		),
		duplicates AS (
			SELECT n.operator, n.type
			FROM nodes n
			INNER JOIN (
				SELECT operator
				FROM nodes
				GROUP BY operator
				HAVING COUNT(*) > 1
			) dup ON n.operator = dup.operator
			WHERE (n.operator, n.type) NOT IN (SELECT operator, type FROM primary_nodes)
			AND n.flagged = FALSE
			AND n.manual_override = FALSE
		)
		UPDATE nodes
		SET flagged = TRUE, flagged_reason = 'duplicate_operator'
		WHERE (operator, type) IN (SELECT operator, type FROM duplicates)
	`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// ClearDuplicateOperatorFlags clears the duplicate_operator flag from nodes no longer duplicates
// or have become the primary node (most recently seen) for their operator
func (r *NodeRepository) ClearDuplicateOperatorFlags(ctx context.Context) (int64, error) {
	// Clear flag from nodes that:
	// 1. were flagged as duplicate_operator but are now the only node for their operator
	// 2. are now the primary (most recently seen) for their operator
	query := `
		WITH primary_nodes AS (
			SELECT DISTINCT ON (operator) operator, type
			FROM nodes
			ORDER BY operator, last_seen_at DESC, type ASC
		),
		should_unflag AS (
			SELECT n.operator, n.type
			FROM nodes n
			WHERE n.flagged = TRUE
			AND n.flagged_reason = 'duplicate_operator'
			AND n.manual_override = FALSE
			AND (
				-- Node is now alone for this operator
				(SELECT COUNT(*) FROM nodes n2 WHERE n2.operator = n.operator) = 1
				OR
				-- Node is now the primary for this operator
				(n.operator, n.type) IN (SELECT operator, type FROM primary_nodes)
			)
		)
		UPDATE nodes
		SET flagged = FALSE, flagged_reason = NULL
		WHERE (operator, type) IN (SELECT operator, type FROM should_unflag)
	`

	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// ClaimNodes atomically claims a batch of nodes for checking
func (r *NodeRepository) ClaimNodes(ctx context.Context, checkerID string, limit int, claimTTL int) ([]*domain.Node, error) {
	query := `
		WITH claimable AS (
			SELECT operator, type FROM nodes
			WHERE flagged = FALSE
			  AND (claimed_by IS NULL OR claim_expires_at < NOW())
			  AND (next_check_after IS NULL OR next_check_after < NOW())
			ORDER BY RANDOM()
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE nodes n
		SET claimed_by = $1,
			claimed_at = NOW(),
			claim_expires_at = NOW() + INTERVAL '1 second' * $3
		FROM claimable c
		WHERE n.operator = c.operator AND n.type = c.type
		RETURNING n.operator, n.type, n.alias, host(n.current_ip), n.country, n.country_code,
				  n.latitude, n.longitude, n.first_seen_at, n.last_seen_at, n.flagged, n.flagged_reason,
				  n.total_checks, n.successful_checks, n.sync_score_sum,
				  n.last_check_at, n.last_success, n.last_failure_reason, n.last_tick, n.last_reference_tick
	`

	rows, err := r.pool.Query(ctx, query, checkerID, limit, claimTTL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// UpdateCheckResultAndRelease atomically updates check result -> sets next check time with jitter -> releases the claim
// The WHERE clause includes claimed_by = checkerID so that if the claim expired and another checker take over
func (r *NodeRepository) UpdateCheckResultAndRelease(ctx context.Context, checkerID string, result *domain.CheckResult, baseInterval int, jitterMax int) error {
	var query string
	var args []interface{}

	// next_check_after = NOW() + baseInterval + random(0, jitterMax) seconds
	if result.Success {
		query = `
			UPDATE nodes SET
				claimed_by = NULL,
				claimed_at = NULL,
				claim_expires_at = NULL,
				next_check_after = NOW() + INTERVAL '1 second' * $8 + INTERVAL '1 second' * floor(random() * $9),
				total_checks = total_checks + 1,
				successful_checks = successful_checks + 1,
				sync_score_sum = sync_score_sum + $4,
				last_check_at = $5,
				last_success = TRUE,
				last_failure_reason = NULL,
				last_tick = $6,
				last_reference_tick = $7
			WHERE operator = $1 AND type = $2 AND claimed_by = $3
		`
		args = []interface{}{result.Operator, result.NodeType, checkerID, result.SyncScore, result.Timestamp, result.NodeTick, result.ReferenceTick, baseInterval, jitterMax + 1}
	} else {
		query = `
			UPDATE nodes SET
				claimed_by = NULL,
				claimed_at = NULL,
				claim_expires_at = NULL,
				next_check_after = NOW() + INTERVAL '1 second' * $6 + INTERVAL '1 second' * floor(random() * $7),
				total_checks = total_checks + 1,
				last_check_at = $4,
				last_success = FALSE,
				last_failure_reason = $5
			WHERE operator = $1 AND type = $2 AND claimed_by = $3
		`
		args = []interface{}{result.Operator, result.NodeType, checkerID, result.Timestamp, result.FailureReason, baseInterval, jitterMax + 1}
	}

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// ReleaseExpiredClaims releases claims that have expired (for cleanup)
func (r *NodeRepository) ReleaseExpiredClaims(ctx context.Context) (int64, error) {
	query := `
		UPDATE nodes SET
			claimed_by = NULL,
			claimed_at = NULL,
			claim_expires_at = NULL
		WHERE claim_expires_at < NOW()
	`
	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// ReleaseAllClaims releases all claims (for epoch transition or shutdown)
func (r *NodeRepository) ReleaseAllClaims(ctx context.Context) (int64, error) {
	query := `
		UPDATE nodes SET
			claimed_by = NULL,
			claimed_at = NULL,
			claim_expires_at = NULL
		WHERE claimed_by IS NOT NULL
	`
	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// CheckerStats holds statistics for a checker instance
type CheckerStats struct {
	CheckerID     string
	ClaimedNodes  int
	ActiveClaims  int
	ExpiredClaims int
	LastClaimAt   *time.Time
	OldestClaimAt *time.Time
}

// GetCheckerStats returns statistics for all active checkers
func (r *NodeRepository) GetCheckerStats(ctx context.Context) ([]CheckerStats, error) {
	query := `
		SELECT
			claimed_by,
			COUNT(*) as claimed_nodes,
			COUNT(*) FILTER (WHERE claim_expires_at >= NOW()) as active_claims,
			COUNT(*) FILTER (WHERE claim_expires_at < NOW()) as expired_claims,
			MAX(claimed_at) as last_claim_at,
			MIN(claimed_at) as oldest_claim_at
		FROM nodes
		WHERE claimed_by IS NOT NULL
		GROUP BY claimed_by
		ORDER BY claimed_by
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []CheckerStats
	for rows.Next() {
		var s CheckerStats
		err := rows.Scan(
			&s.CheckerID,
			&s.ClaimedNodes,
			&s.ActiveClaims,
			&s.ExpiredClaims,
			&s.LastClaimAt,
			&s.OldestClaimAt,
		)
		if err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// GetClaimSummary returns overall claim statistics
func (r *NodeRepository) GetClaimSummary(ctx context.Context) (totalNodes int, claimedNodes int, pendingNodes int, err error) {
	query := `
		SELECT
			COUNT(*) as total_nodes,
			COUNT(*) FILTER (WHERE claimed_by IS NOT NULL) as claimed_nodes,
			COUNT(*) FILTER (WHERE flagged = FALSE AND (claimed_by IS NULL OR claim_expires_at < NOW()) AND (next_check_after IS NULL OR next_check_after < NOW())) as pending_nodes
		FROM nodes
	`
	err = r.pool.QueryRow(ctx, query).Scan(&totalNodes, &claimedNodes, &pendingNodes)
	return
}

// GetNodesWithoutCountry returns nodes that don't have country info
func (r *NodeRepository) GetNodesWithoutCountry(ctx context.Context) ([]*domain.Node, error) {
	query := `
		SELECT operator, type, alias, host(current_ip), country, country_code,
			   latitude, longitude, first_seen_at, last_seen_at, flagged, flagged_reason,
			   total_checks, successful_checks, sync_score_sum,
			   last_check_at, last_success, last_failure_reason, last_tick, last_reference_tick
		FROM nodes
		WHERE country IS NULL OR country_code IS NULL
		ORDER BY last_seen_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.Node
	for rows.Next() {
		var node domain.Node
		var ip string

		err := rows.Scan(
			&node.Operator,
			&node.Type,
			&node.Alias,
			&ip,
			&node.Country,
			&node.CountryCode,
			&node.Latitude,
			&node.Longitude,
			&node.FirstSeenAt,
			&node.LastSeenAt,
			&node.Flagged,
			&node.FlaggedReason,
			&node.TotalChecks,
			&node.SuccessfulChecks,
			&node.SyncScoreSum,
			&node.LastCheckAt,
			&node.LastSuccess,
			&node.LastFailureReason,
			&node.LastTick,
			&node.LastReferenceTick,
		)
		if err != nil {
			return nil, err
		}

		node.CurrentIP = ip
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

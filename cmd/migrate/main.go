package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/qubic/network-guardians/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	connString := cfg.Database.ConnectionString()
	fmt.Printf("Connecting to: %s:%d/%s\n", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close(ctx)

	fmt.Println("Connected successfully!")

	// Run db migrations
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "001_create_nodes",
			sql: `
CREATE TABLE IF NOT EXISTS nodes (
    operator VARCHAR(60) PRIMARY KEY,
    type VARCHAR(10) NOT NULL,
    alias VARCHAR(50),
    current_ip INET NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    flagged BOOLEAN NOT NULL DEFAULT FALSE,
    flagged_reason TEXT,
    total_checks INTEGER NOT NULL DEFAULT 0,
    successful_checks INTEGER NOT NULL DEFAULT 0,
    sync_score_sum DECIMAL(10,2) NOT NULL DEFAULT 0,
    last_check_at TIMESTAMPTZ,
    last_success BOOLEAN,
    last_failure_reason VARCHAR(50)
);

CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_flagged ON nodes(flagged);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen_at ON nodes(last_seen_at);
`,
		},
		{
			name: "002_create_epoch_results",
			sql: `
CREATE TABLE IF NOT EXISTS epoch_results (
    operator VARCHAR(60) NOT NULL,
    epoch SMALLINT NOT NULL,
    type VARCHAR(10) NOT NULL,
    alias VARCHAR(50),
    total_checks INTEGER NOT NULL,
    successful_checks INTEGER NOT NULL,
    uptime_score DECIMAL(5,2) NOT NULL,
    sync_score DECIMAL(5,2) NOT NULL,
    final_score DECIMAL(5,2) NOT NULL,
    eligible BOOLEAN NOT NULL,
    disqualify_reason VARCHAR(50),
    reward_points DECIMAL(15,2) NOT NULL,
    reward_amount BIGINT,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (epoch, operator)
);

CREATE INDEX IF NOT EXISTS idx_epoch_results_epoch ON epoch_results(epoch);
CREATE INDEX IF NOT EXISTS idx_epoch_results_operator ON epoch_results(operator);
CREATE INDEX IF NOT EXISTS idx_epoch_results_eligible ON epoch_results(epoch, eligible);
CREATE INDEX IF NOT EXISTS idx_epoch_results_final_score ON epoch_results(epoch, final_score DESC);
`,
		},
		{
			name: "003_drop_next_check_at",
			sql: `
DROP INDEX IF EXISTS idx_nodes_next_check_at;
ALTER TABLE nodes DROP COLUMN IF EXISTS next_check_at;
`,
		},
		{
			name: "004_add_country_columns",
			sql: `
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS country VARCHAR(100);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS country_code VARCHAR(2);
CREATE INDEX IF NOT EXISTS idx_nodes_country_code ON nodes(country_code);
`,
		},
		{
			name: "005_add_geo_coordinates",
			sql: `
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS latitude DECIMAL(10,6);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS longitude DECIMAL(10,6);
`,
		},
		{
			name: "006_composite_primary_key",
			sql: `
-- Change primary key from (operator) to (operator, type)
-- This allows the same operator to run both lite and bob nodes
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_pkey;
ALTER TABLE nodes ADD PRIMARY KEY (operator, type);

-- Add index for operator lookups
CREATE INDEX IF NOT EXISTS idx_nodes_operator ON nodes(operator);
`,
		},
		{
			name: "007_add_last_tick",
			sql: `
-- Add last_tick column to store the node's tick at last check
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_tick INTEGER;
`,
		},
		{
			name: "008_add_last_reference_tick",
			sql: `
-- Add last_reference_tick column to store the reference tick at check time
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_reference_tick INTEGER;
`,
		},
		{
			name: "009_add_distributed_checker_columns",
			sql: `
-- Add columns for distributed checker coordination
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS claimed_by VARCHAR(64);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS claim_expires_at TIMESTAMPTZ;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS next_check_after TIMESTAMPTZ;

-- Index for efficient claim queries
CREATE INDEX IF NOT EXISTS idx_nodes_claim_expires ON nodes (claim_expires_at)
    WHERE claimed_by IS NOT NULL;

-- Index for the claim query - nodes due for checking
CREATE INDEX IF NOT EXISTS idx_nodes_next_check ON nodes (next_check_after ASC NULLS FIRST)
    WHERE flagged = FALSE;
`,
		},
		{
			name: "010_add_manual_override",
			sql: `
-- Manual override prevents auto-flagging from overwriting admin flag/unflag decisions
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS manual_override BOOLEAN NOT NULL DEFAULT FALSE;
`,
		},
	}

	for _, m := range migrations {
		fmt.Printf("Running migration: %s... ", m.name)
		_, err := conn.Exec(ctx, m.sql)
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OK")
	}

	fmt.Println("\nAll migrations completed successfully!")
}

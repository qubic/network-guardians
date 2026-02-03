# Architecture

## Overview

Network Guardians monitors Qubic nodes through a distributed checker system. Multiple checker instances run across different regions, each validating nodes and reporting results. 
The API server aggregates this data and serves it to the frontend dashboard.

## How It Works

### Node Discovery

The discovery service polls an API where each nodes type check-in at startup and every 30mins. Each node is identified by its operator ID and enriched with GeoIP data for the map visualization.

### Distributed Checking

Checker instances run independently across regions. Each check connects to the node's endpoint (LITE on port 41841, BOB on port 40420), verifies it responds correctly, and measures how in-sync it is with the current network tick.

### Scoring

Scores are calculated from check results:

- **Uptime Score** = successful checks / total checks
- **Sync Score** = average sync metric across checks
- **Final Score** = (uptime × weight) + (sync × weight)

### Flagging

The flagging service automatically detects and flags nodes:
- Duplicate IPs (multiple nodes on same IP)
- Duplicate operators (same operator ID running multiple nodes of same type)
- Mid-epoch node type switch with same operator ID 

Flagged nodes are excluded from rewards. Only the most recent registration is eligible.

### API Server

The API server aggregates data from all services and serves it to the frontend dashboard:
- Node data with live scores
- Leaderboard rankings
- Epoch history and statistics
- Health and reference data

### Epoch Transitions

Epochs transition weekly on Wednesday at 12:00 UTC. During transition:

1. Checkers pause (grace period of one hour)
2. Final scores are calculated
3. Rewards are distributed based on eligibility thresholds
4. Node data is archived to `epochs` table
5. Fresh discovery cycle begins

### Reward Pools

Rewards are split between two pools.

- Bob Pool
- Lite Pool

The share between pools is configurable. Eligibility requires meeting minimum thresholds for checks, uptime, and sync score.


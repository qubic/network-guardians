# API Reference

Base URL: `/api/v1`

All responses are JSON.

## Endpoints

### List Nodes

```
GET /nodes
```

Returns all tracked nodes with live scores and reward eligibility.

**Response:**
```json
[
  {
    "operator": "ABCDEFGH...",
    "type": "lite",
    "alias": "my-node",
    "country": "France",
    "countryCode": "FR",
    "latitude": 48.8566,
    "longitude": 2.3522,
    "firstSeenAt": "2024-01-10T08:00:00Z",
    "lastSeenAt": "2024-01-15T10:30:00Z",
    "flagged": false,
    "flaggedReason": "duplicate_ip",
    "totalChecks": 150,
    "successfulChecks": 142,
    "syncScoreSum": 13400.5,
    "lastCheckAt": "2024-01-15T10:30:00Z",
    "lastSuccess": true,
    "lastFailureReason": "connection timeout",
    "lastTick": 22145678,
    "lastReferenceTick": 22145700,
    "liveScore": {
      "uptimeScore": 94.7,
      "syncScore": 89.2,
      "finalScore": 92.5,
      "rewardPoints": 13116.0,
      "estimatedReward": 125000
    },
    "eligibleForReward": true,
    "ineligibleReason": "duplicate_operator_or_ip"
  }
]
```

---

### Get Node

```
GET /nodes/{operator}
```

Returns details for a specific node including all node types for this operator and epoch history.

**Parameters:**
- `operator` - The node's operator ID

**Response:**
```json
{
  "node": {
    "operator": "ABCDEFGH...",
    "type": "lite",
    "alias": "my-node",
    "country": "France",
    "countryCode": "FR",
    "latitude": 48.8566,
    "longitude": 2.3522,
    "firstSeenAt": "2024-01-10T08:00:00Z",
    "lastSeenAt": "2024-01-15T10:30:00Z",
    "flagged": false,
    "totalChecks": 150,
    "successfulChecks": 142,
    "syncScoreSum": 13400.5,
    "lastCheckAt": "2024-01-15T10:30:00Z",
    "lastSuccess": true,
    "lastTick": 22145678,
    "lastReferenceTick": 22145700,
    "liveScore": {
      "uptimeScore": 94.7,
      "syncScore": 89.2,
      "finalScore": 92.5,
      "rewardPoints": 13116.0,
      "estimatedReward": 125000
    },
    "eligibleForReward": true,
    "ineligibleReason": "flagged"
  },
  "allTypes": [],
  "history": [
    {
      "operator": "ABCDEFGH...",
      "epoch": 149,
      "type": "lite",
      "alias": "my-node",
      "totalChecks": 10080,
      "successfulChecks": 9800,
      "uptimeScore": 97.2,
      "syncScore": 95.1,
      "finalScore": 96.4,
      "eligible": true,
      "rewardPoints": 944720.0,
      "rewardAmount": 250000,
      "calculatedAt": "2024-01-10T12:00:00Z"
    }
  ]
}
```

**Errors:**
- `404` - Node not found

---

### Current Leaderboard

```
GET /leaderboard
GET /leaderboard?type=lite
GET /leaderboard?type=bob
```

Returns nodes ranked by final score. Optionally filter by node type.

**Response:**
```json
{
  "rankings": [...],
  "reference": {
    "tick": 12345678,
    "epoch": 150,
    "updatedAt": "2024-01-15T10:30:00Z"
  },
  "info": {
    "totalNodes": 245,
    "activeNodes": 240,
    "flaggedNodes": 5,
    "eligibleForRewards": 210,
    "duplicatesExcluded": 30,
    "liteCount": 150,
    "bobCount": 60,
    "filteredCount": 150
  },
  "pool": {
    "type": "all",
    "amount": 100000000,
    "totalPool": 100000000,
    "litePool": 60000000,
    "bobPool": 40000000
  }
}
```

---

### Historical Leaderboard

```
GET /leaderboard/{epoch}
```

Returns final rankings for a past epoch.

**Parameters:**
- `epoch` - Epoch number

**Response:**
```json
{
  "epoch": 149,
  "rankings": [
    {
      "operator": "ABCDEFGH...",
      "epoch": 149,
      "type": "lite",
      "alias": "my-node",
      "totalChecks": 10080,
      "successfulChecks": 9800,
      "uptimeScore": 97.2,
      "syncScore": 95.1,
      "finalScore": 96.4,
      "eligible": true,
      "rewardPoints": 944720.0,
      "rewardAmount": 250000,
      "calculatedAt": "2024-01-10T12:00:00Z"
    }
  ],
  "stats": {
    "epoch": 149,
    "totalNodes": 245,
    "eligibleNodes": 210,
    "totalLiteNodes": 180,
    "totalBobNodes": 65,
    "avgUptimeScore": 91.3,
    "avgSyncScore": 87.5,
    "totalRewardPool": 750000000
  }
}
```

**Errors:**
- `404` - Epoch not found

---

### Network Stats

```
GET /stats
```

Returns network statistics and epoch progress.

**Response:**
```json
{
  "totalNodes": 245,
  "activeNodes": 240,
  "flaggedNodes": 5,
  "liteNodes": 180,
  "bobNodes": 60,
  "reference": {
    "tick": 12345678,
    "epoch": 150,
    "updatedAt": "2024-01-15T10:30:00Z"
  },
  "latestCompletedEpoch": 149,
  "epochRewards": {
    "totalPool": 100000000,
    "litePool": 60000000,
    "bobPool": 40000000
  },
  "epochProgress": {
    "started_at": "2024-01-10T12:00:00Z",
    "current_time": "2024-01-15T10:30:00Z",
    "ends_at": "2024-01-17T12:00:00Z",
    "time_remaining_seconds": 129600,
    "progress_percent": 75.5
  },
  "epochPhase": {
    "phase": "active"
  }
}
```

During a grace period, `epochPhase` includes additional fields:
```json
{
  "phase": "grace_period",
  "gracePeriodStarted": "2024-01-17T12:00:00Z",
  "gracePeriodEnds": "2024-01-17T13:00:00Z",
  "gracePeriodRemainingSeconds": 1800,
  "transitionEpoch": 151
}
```

## Error Responses

```json
{
  "error": "node not found"
}
```

- `400` - Bad request
- `404` - Not found
- `500` - Internal error

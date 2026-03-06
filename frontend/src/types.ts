export interface Stats {
  totalNodes: number
  activeNodes: number
  flaggedNodes: number
  liteNodes: number
  bobNodes: number
  reference: {
    tick: number
    epoch: number
    source: string
    updatedAt: string
  }
  latestCompletedEpoch: number
  epochRewards: {
    totalPool: number
    litePool: number
    bobPool: number
  }
  epochProgress: {
    started_at: string
    current_time: string
    ends_at: string
    time_remaining_seconds: number
    progress_percent: number
  }
}

export interface LiveScore {
  uptimeScore: number
  syncScore: number
  finalScore: number
  rewardPoints: number
  estimatedReward: number
}

export interface Node {
  operator: string
  type: 'lite' | 'bob'
  alias?: string
  country?: string
  countryCode?: string
  latitude?: number
  longitude?: number
  firstSeenAt: string
  lastSeenAt: string
  flagged: boolean
  flaggedReason?: string
  totalChecks: number
  successfulChecks: number
  syncScoreSum: number
  lastCheckAt?: string
  lastSuccess?: boolean
  lastFailureReason?: string
  lastTick?: number
  lastReferenceTick?: number
  liveScore: LiveScore
  eligibleForReward: boolean
  ineligibleReason?: string
}

// NodeDetails
export interface NodeDetailNode {
  operator: string
  type: 'lite' | 'bob'
  alias?: string
  country?: string
  countryCode?: string
  firstSeenAt: string
  lastSeenAt: string
  flagged: boolean
  flaggedReason?: string
  totalChecks: number
  successfulChecks: number
  syncScoreSum: number
  lastCheckAt?: string
  lastSuccess?: boolean
  lastFailureReason?: string
  lastTick?: number
  lastReferenceTick?: number
  liveScore: LiveScore
  eligibleForReward: boolean
  ineligibleReason?: string
}

export interface NodeDetail {
  node: NodeDetailNode
  allTypes?: Node[]
  history: EpochHistory[]
}

export interface EpochHistory {
  epoch: number
  uptimeScore: number
  syncScore: number
  finalScore: number
  eligible: boolean
  rewardPoints: number
  rewardAmount?: number
}

export interface LeaderboardEntry {
  operator: string
  type: 'lite' | 'bob'
  alias?: string
  country?: string
  countryCode?: string
  liveScore: LiveScore
}

export interface LeaderboardResponse {
  rankings: LeaderboardEntry[]
  reference: {
    tick: number
    epoch: number
    source: string
    updatedAt: string
  }
  info: {
    totalNodes: number
    activeNodes: number
    flaggedNodes: number
    eligibleForRewards: number
    duplicatesExcluded: number
    liteCount: number
    bobCount: number
    filteredCount: number
  }
  pool: {
    type: 'all' | 'lite' | 'bob'
    amount: number
    totalPool: number
    litePool: number
    bobPool: number
  }
}

export interface Health {
  healthy: boolean
  tick: number
  epoch: number
  lastUpdated: string
}

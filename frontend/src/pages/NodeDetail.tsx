import { useParams, Link } from 'react-router-dom'
import { useApi } from '../hooks/useApi'
import { NodeDetail as NodeDetailType, LeaderboardResponse } from '../types'
import Card from '../components/Card'
import Badge from '../components/Badge'
import Skeleton from '../components/Skeleton'

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

export default function NodeDetail() {
  const { operator, type } = useParams<{ operator: string; type: string }>()
  const { data, loading, error } = useApi<NodeDetailType>(
    `/api/v1/nodes/${operator}/${type}`,
    30000
  )
  const { data: leaderboard } = useApi<LeaderboardResponse>('/api/v1/leaderboard', 30000)

  // Find ranking position
  const rank = leaderboard?.rankings?.findIndex(e => e.operator === operator)
  const rankPosition = rank !== undefined && rank !== -1 ? rank + 1 : null
  const totalRanked = leaderboard?.rankings?.length ?? 0

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
        <Skeleton className="h-48" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="space-y-6">
        <Link to="/nodes" className="text-sm text-primary-30 hover:underline">
          &larr; Back to Nodes
        </Link>
        <Card className="p-8 text-center">
          <p className="text-error-40">
            {error || 'Node not found'}
          </p>
        </Card>
      </div>
    )
  }

  const { node, history } = data

  return (
    <div className="space-y-6">
      {/* Back Link */}
      <Link to="/nodes" className="text-13 text-primary-30 hover:underline">
        &larr; Back to Nodes
      </Link>

      {/* Page Header */}
      <div>
        <div className="flex flex-wrap items-center gap-4">
          <h1 className="font-space text-24 font-bold text-white">
            {node.alias || 'Node Details'}
          </h1>
          <Badge variant={node.type === 'lite' ? 'info' : 'warning'}>
            {node.type.charAt(0).toUpperCase() + node.type.slice(1)}
          </Badge>
          {node.flagged && <Badge variant="error">Flagged</Badge>}
          {!node.eligibleForReward && !node.flagged && node.ineligibleReason && (
            <Badge variant="warning">
              {node.ineligibleReason === 'duplicate_operator_or_ip' ? 'Duplicate'
                : node.ineligibleReason === 'insufficient_checks' ? 'Not Enough Checks'
                : node.ineligibleReason === 'low_uptime' ? 'Low Uptime'
                : node.ineligibleReason === 'low_sync' ? 'Low Sync'
                : 'Not Eligible'}
            </Badge>
          )}
        </div>
        <p className="mt-1 break-all font-space text-14 font-normal tracking-wide text-gray-50">
          {node.operator}
        </p>
      </div>

      {/* Main Info Grid */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* Basic Info */}
        <Card className="p-5">
          <h2 className="mb-4 font-space text-16 font-semibold text-white">
            Node Information
          </h2>
          <dl className="space-y-3">
            <div className="flex justify-between">
              <dt className="text-gray-50">Location</dt>
              <dd className="text-sm text-white">{node.country || '-'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-50">First Seen</dt>
              <dd className="text-sm text-white">{formatDate(node.firstSeenAt)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-50">Checks</dt>
              <dd className="text-sm text-white">
                {node.successfulChecks} / {node.totalChecks}
              </dd>
            </div>
            {node.lastTick && (
              <div className="flex justify-between">
                <dt className="text-gray-50">Last Tick</dt>
                <dd className="text-sm text-white">
                  {node.lastTick.toLocaleString()}
                  {node.lastReferenceTick && (
                    <span className="ml-2 text-xs text-gray-50">
                      ({node.lastTick >= node.lastReferenceTick
                        ? 'synced'
                        : `${(node.lastReferenceTick - node.lastTick).toLocaleString()} behind`})
                    </span>
                  )}
                </dd>
              </div>
            )}
            <div className="flex justify-between">
              <dt className="text-gray-50">Status</dt>
              <dd className="text-sm">
                {node.lastSuccess === true ? (
                  <span className="text-success-40">Online</span>
                ) : node.lastSuccess === false ? (
                  <span className="text-error-40">Offline</span>
                ) : (
                  <span className="text-gray-50">-</span>
                )}
              </dd>
            </div>
            {node.lastCheckAt && (
              <div className="flex justify-between">
                <dt className="text-gray-50">Last Check</dt>
                <dd className="text-sm text-white">{formatDate(node.lastCheckAt)}</dd>
              </div>
            )}
            {node.lastSuccess === false && node.lastFailureReason && (
              <div className="flex justify-between">
                <dt className="text-gray-50">Failure Reason</dt>
                <dd className="text-sm text-error-40">{node.lastFailureReason}</dd>
              </div>
            )}
            {node.flagged && node.flaggedReason && (
              <div className="flex justify-between">
                <dt className="text-gray-50">Flag Reason</dt>
                <dd className="text-sm text-error-40">{node.flaggedReason}</dd>
              </div>
            )}
          </dl>
        </Card>

        {/* Live Scores */}
        <Card className="p-5">
          <h2 className="mb-4 font-space text-16 font-semibold text-white">
            Live Scores
          </h2>
          <dl className="space-y-3">
            {rankPosition && (
              <div className="flex justify-between">
                <dt className="text-gray-50">Ranking</dt>
                <dd className="font-space text-lg font-bold text-primary-30">
                  #{rankPosition} <span className="text-sm font-normal text-gray-50">/ {totalRanked}</span>
                </dd>
              </div>
            )}
            <div className="flex justify-between">
              <dt className="text-gray-50">Uptime Score</dt>
              <dd className="text-sm font-medium text-white">
                {(node.liveScore?.uptimeScore ?? 0).toFixed(1)}%
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-50">Sync Score</dt>
              <dd className="text-sm font-medium text-white">
                {(node.liveScore?.syncScore ?? 0).toFixed(1)}%
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-50">Final Score</dt>
              <dd className="font-space text-lg font-bold text-primary-30">
                {(node.liveScore?.finalScore ?? 0).toFixed(1)}%
              </dd>
            </div>
            <div className="border-t border-primary-60 pt-3">
              <div className="flex justify-between">
                <dt className="text-gray-50">Reward Points</dt>
                <dd className="text-sm text-white">
                  {(node.liveScore?.rewardPoints ?? 0).toLocaleString(undefined, {
                    maximumFractionDigits: 0
                  })}
                </dd>
              </div>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-50">Estimated Reward</dt>
              <dd className="font-space text-lg font-bold text-success-40">
                {(node.liveScore?.estimatedReward ?? 0).toLocaleString()} QUS
              </dd>
            </div>
          </dl>
        </Card>
      </div>

      {/* Epoch History */}
      <Card className="overflow-hidden">
        <div className="border-b border-primary-60 bg-primary-80 px-5 py-3">
          <h2 className="font-space text-16 font-semibold text-white">
            Epoch History
          </h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-primary-60 bg-primary-80/50">
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-50">
                  Epoch
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Uptime
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Sync
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Final Score
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-50">
                  Eligible
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Points
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Reward
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-primary-60">
              {history?.length ? (
                history.map((epoch) => (
                  <tr
                    key={epoch.epoch}
                    className="transition-colors hover:bg-primary-60/50"
                  >
                    <td className="px-4 py-3 text-sm font-medium text-white">
                      {epoch.epoch}
                    </td>
                    <td className="px-4 py-3 text-right text-sm text-white">
                      {epoch.uptimeScore.toFixed(1)}%
                    </td>
                    <td className="px-4 py-3 text-right text-sm text-white">
                      {epoch.syncScore.toFixed(1)}%
                    </td>
                    <td className="px-4 py-3 text-right text-sm text-white">
                      {epoch.finalScore.toFixed(1)}%
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={epoch.eligible ? 'success' : 'error'}>
                        {epoch.eligible ? 'Yes' : 'No'}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-right text-sm text-white">
                      {epoch.rewardPoints.toLocaleString(undefined, {
                        maximumFractionDigits: 0
                      })}
                    </td>
                    <td className="px-4 py-3 text-right text-sm text-success-40">
                      {epoch.rewardAmount ? `${epoch.rewardAmount.toLocaleString()} QUS` : '-'}
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-gray-50">
                    No epoch history available
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
